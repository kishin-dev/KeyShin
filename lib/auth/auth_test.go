package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	handler "github.com/kishin-dev/keyshin/api"
	"github.com/kishin-dev/keyshin/lib/auth"
	"github.com/kishin-dev/keyshin/lib/supafake"
)

// client is a tiny cookie-keeping browser for driving the API handlers.
type client struct {
	t       *testing.T
	cookies map[string]*http.Cookie
}

func (c *client) call(h http.HandlerFunc, method, body string) (int, map[string]any) {
	c.t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, "/", nil)
	}
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.MaxAge < 0 {
			delete(c.cookies, ck.Name)
		} else {
			c.cookies[ck.Name] = ck
		}
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func setup(t *testing.T, dev bool) (*supafake.Server, *client) {
	fake := supafake.New()
	t.Cleanup(fake.Close)
	t.Setenv("SUPABASE_URL", fake.URL)
	t.Setenv("SUPABASE_PUBLISHABLE_KEY", supafake.PublishableKey)
	t.Setenv("SUPABASE_SECRET_KEY", supafake.SecretKey)
	if dev {
		t.Setenv("KEYSHIN_DEV", "1")
	} else {
		t.Setenv("KEYSHIN_DEV", "")
	}
	id := fake.AddUser("bartol@example.com", "s3cret-pass")
	fake.AddAdmin("bartol", id)
	fake.AddUser("stranger@example.com", "stranger-pass") // Supabase user, not an admin
	return fake, &client{t: t, cookies: map[string]*http.Cookie{}}
}

func noCache(t *testing.T) {
	old := auth.SessionCacheTTL
	auth.SessionCacheTTL = 0
	t.Cleanup(func() { auth.SessionCacheTTL = old })
}

func TestLoginFlow(t *testing.T) {
	noCache(t) // this test checks token refresh, which the cache would skip
	fake, c := setup(t, true)

	if code, _ := c.call(handler.Session, "GET", ""); code != 401 {
		t.Fatalf("session before login = %d, want 401", code)
	}

	code, body := c.call(handler.Login, "POST", `{"username":" Bartol ","password":"s3cret-pass"}`)
	if code != 200 || body["username"] != "bartol" {
		t.Fatalf("login = %d %v", code, body)
	}
	for _, name := range []string{"ks_access", "ks_refresh"} {
		ck := c.cookies[name]
		if ck == nil || !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode {
			t.Fatalf("cookie %s missing or not HttpOnly/Strict: %+v", name, ck)
		}
	}

	if code, body := c.call(handler.Session, "GET", ""); code != 200 || body["username"] != "bartol" {
		t.Fatalf("session after login = %d %v", code, body)
	}

	fake.Stats["products"] = 3
	if code, body := c.call(handler.Stats, "GET", ""); code != 200 || body["products"] != float64(3) {
		t.Fatalf("stats = %d %v", code, body)
	}

	// Access token expires after an hour: the refresh token renews it.
	oldRefresh := c.cookies["ks_refresh"].Value
	fake.ExpireAccessTokens()
	if code, _ := c.call(handler.Session, "GET", ""); code != 200 {
		t.Fatalf("session after access expiry = %d, want 200 via refresh", code)
	}
	if c.cookies["ks_refresh"].Value == oldRefresh {
		t.Fatal("refresh token was not rotated")
	}

	if code, _ := c.call(handler.Logout, "POST", ""); code != 200 {
		t.Fatalf("logout = %d", code)
	}
	if len(c.cookies) != 0 {
		t.Fatalf("cookies left after logout: %v", c.cookies)
	}
	if code, _ := c.call(handler.Session, "GET", ""); code != 401 {
		t.Fatalf("session after logout = %d, want 401", code)
	}
}

func TestLogoutRevokesStolenCookies(t *testing.T) {
	_, c := setup(t, true)
	c.call(handler.Login, "POST", `{"username":"bartol","password":"s3cret-pass"}`)
	stolen := &client{t: t, cookies: map[string]*http.Cookie{}}
	for k, v := range c.cookies {
		stolen.cookies[k] = v
	}
	c.call(handler.Logout, "POST", "")
	if code, _ := stolen.call(handler.Session, "GET", ""); code != 401 {
		t.Fatalf("copied cookies still work after logout: %d", code)
	}
}

func TestLoginRejected(t *testing.T) {
	_, c := setup(t, true)
	cases := map[string]string{
		"wrong password":     `{"username":"bartol","password":"nope"}`,
		"unknown user":       `{"username":"ghost","password":"s3cret-pass"}`,
		"non-admin by email": `{"username":"stranger@example.com","password":"stranger-pass"}`,
		"empty password":     `{"username":"bartol","password":""}`,
		"invalid username":   `{"username":"a","password":"x"}`,
	}
	for name, body := range cases {
		code, out := c.call(handler.Login, "POST", body)
		if code != 401 || out["error"] != "invalid username or password" {
			t.Errorf("%s: got %d %v, want 401 with generic message", name, code, out)
		}
	}
	if len(c.cookies) != 0 {
		t.Errorf("rejected logins set cookies: %v", c.cookies)
	}
}

func TestRemovedAdminLosesAccess(t *testing.T) {
	noCache(t) // without the short cache, removal takes effect immediately
	fake, c := setup(t, true)
	c.call(handler.Login, "POST", `{"username":"bartol","password":"s3cret-pass"}`)
	fake.RemoveAdmin("bartol")
	if code, _ := c.call(handler.Session, "GET", ""); code != 401 {
		t.Fatalf("removed admin still signed in: %d", code)
	}
	if len(c.cookies) != 0 {
		t.Fatal("cookies not cleared for removed admin")
	}
}

func TestProductionCookies(t *testing.T) {
	_, c := setup(t, false)
	c.call(handler.Login, "POST", `{"username":"bartol","password":"s3cret-pass"}`)
	ck := c.cookies["__Host-ks_access"]
	if ck == nil || !ck.Secure || ck.Path != "/" || ck.Domain != "" {
		t.Fatalf("production cookie should be __Host- prefixed and Secure: %+v", ck)
	}
}

func TestStatsRequiresLogin(t *testing.T) {
	_, c := setup(t, true)
	if code, _ := c.call(handler.Stats, "GET", ""); code != 401 {
		t.Fatalf("stats without login = %d, want 401", code)
	}
	c.cookies["ks_access"] = &http.Cookie{Name: "ks_access", Value: "forged"}
	if code, _ := c.call(handler.Stats, "GET", ""); code != 401 {
		t.Fatalf("stats with forged cookie = %d, want 401", code)
	}
}

func TestLoginRequiresJSON(t *testing.T) {
	setup(t, true)
	req := httptest.NewRequest("POST", "/", strings.NewReader("username=bartol&password=s3cret-pass"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.Login(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("form post = %d, want 415", rec.Code)
	}
}

func TestSessionCacheSkipsSupabase(t *testing.T) {
	fake, c := setup(t, true)
	c.call(handler.Login, "POST", `{"username":"bartol","password":"s3cret-pass"}`)
	c.call(handler.Session, "GET", "") // verifies with Supabase and caches
	before := fake.Requests
	for i := 0; i < 5; i++ {
		if code, _ := c.call(handler.Session, "GET", ""); code != 200 {
			t.Fatalf("cached session check = %d", code)
		}
	}
	if fake.Requests != before {
		t.Fatalf("cached checks made %d Supabase requests, want 0", fake.Requests-before)
	}
	// Logging out clears the cache entry, so copied cookies stop working.
	stolen := &client{t: t, cookies: map[string]*http.Cookie{}}
	for k, v := range c.cookies {
		stolen.cookies[k] = v
	}
	c.call(handler.Logout, "POST", "")
	if code, _ := stolen.call(handler.Session, "GET", ""); code != 401 {
		t.Fatalf("session after logout = %d, want 401", code)
	}
}
