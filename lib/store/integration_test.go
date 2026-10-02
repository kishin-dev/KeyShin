package store_test

// Integration tests against a real Postgres + PostgREST with the KeyShin
// migrations applied. They're skipped unless these are set:
//
//	KEYSHIN_TEST_POSTGREST_URL  e.g. http://127.0.0.1:3001
//	KEYSHIN_TEST_SERVICE_JWT    a JWT with {"role":"service_role"}
//
// Use an empty, throwaway database: the tests create rows.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kishin-dev/keyshin/lib/store"
	"github.com/kishin-dev/keyshin/lib/supa"
)

func client(t *testing.T) *supa.Client {
	t.Helper()
	target, jwt := os.Getenv("KEYSHIN_TEST_POSTGREST_URL"), os.Getenv("KEYSHIN_TEST_SERVICE_JWT")
	if target == "" || jwt == "" {
		t.Skip("set KEYSHIN_TEST_POSTGREST_URL and KEYSHIN_TEST_SERVICE_JWT to run integration tests")
	}
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	// Supabase serves PostgREST under /rest/v1; a bare PostgREST serves it
	// at the root, so strip the prefix.
	proxy := httputil.NewSingleHostReverseProxy(u)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/rest/v1")
		r.Host = u.Host
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	// A legacy-style JWT key goes in Authorization, like a service_role key.
	return &supa.Client{BaseURL: srv.URL, PublishableKey: jwt, SecretKey: jwt, HTTP: http.DefaultClient}
}

var ctx = context.Background()

func uniq(prefix string) string { return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()%1e9) }

func newProduct(t *testing.T, c *supa.Client, prefix string) store.Product {
	t.Helper()
	p, err := store.CreateProduct(ctx, c, store.NewProduct{Name: "Test product", Slug: uniq("prod"), KeyPrefix: prefix})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

type license struct {
	ID             string `json:"id"`
	Key            string `json:"key"`
	Status         string `json:"status"`
	State          string `json:"state"`
	CustomerEmail  string `json:"customerEmail"`
	MaxActivations int    `json:"maxActivations"`
	ExpiresAt      *string
	Activations    []struct {
		ID          string `json:"id"`
		Fingerprint string `json:"fingerprint"`
		Label       string `json:"label"`
	} `json:"activations"`
}

func decodeLicense(t *testing.T, raw json.RawMessage) license {
	t.Helper()
	var l license
	if err := json.Unmarshal(raw, &l); err != nil {
		t.Fatalf("decoding %s: %v", raw, err)
	}
	return l
}

func ptr[T any](v T) *T { return &v }

func TestProducts(t *testing.T) {
	c := client(t)

	slug := uniq("bot")
	p, err := store.CreateProduct(ctx, c, store.NewProduct{Name: "  Discord Bot  ", Slug: strings.ToUpper(slug), KeyPrefix: "dbot"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Discord Bot" || p.Slug != slug || p.KeyPrefix != "DBOT" || p.ID == "" {
		t.Fatalf("created product not normalised: %+v", p)
	}

	_, err = store.CreateProduct(ctx, c, store.NewProduct{Name: "Dup", Slug: slug})
	var ce *store.ConflictError
	if !errors.As(err, &ce) || ce.Field != "slug" {
		t.Fatalf("duplicate slug: err = %v, want ConflictError on slug", err)
	}

	for name, in := range map[string]store.NewProduct{
		"no name":    {Slug: uniq("x")},
		"bad slug":   {Name: "X", Slug: "Has Spaces"},
		"bad prefix": {Name: "X", Slug: uniq("y"), KeyPrefix: "TOO-LONG-PREFIX"},
	} {
		var ve *store.ValidationError
		if _, err := store.CreateProduct(ctx, c, in); !errors.As(err, &ve) {
			t.Errorf("%s: err = %v, want ValidationError", name, err)
		}
	}

	var patch store.ProductPatch
	_ = json.Unmarshal([]byte(`{"name":"Discord Bot Pro","description":null,"keyPrefix":"PRO"}`), &patch)
	p2, err := store.UpdateProduct(ctx, c, p.ID, patch)
	if err != nil {
		t.Fatal(err)
	}
	if p2.Name != "Discord Bot Pro" || p2.KeyPrefix != "PRO" || p2.Slug != slug {
		t.Fatalf("update: %+v", p2)
	}
	if _, err := store.UpdateProduct(ctx, c, "00000000-0000-4000-8000-000000000000", patch); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update missing product: err = %v, want ErrNotFound", err)
	}

	list, err := store.ListProducts(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range list {
		found = found || x.ID == p.ID
	}
	if !found {
		t.Fatal("new product missing from list")
	}
}

func TestLicenseLifecycle(t *testing.T) {
	c := client(t)
	p := newProduct(t, c, "LIFE")
	now := time.Now()

	raw, err := store.IssueLicense(ctx, c, store.NewLicense{
		ProductID: p.ID, CustomerName: ptr("Ana"), CustomerEmail: ptr(" ana@example.com "), MaxActivations: 2,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	l := decodeLicense(t, raw)
	if !strings.HasPrefix(l.Key, "LIFE-") || len(l.Key) != len("LIFE-XXXX-XXXX-XXXX-XXXX") {
		t.Fatalf("unexpected key format %q", l.Key)
	}
	if l.State != "active" || l.CustomerEmail != "ana@example.com" || l.MaxActivations != 2 {
		t.Fatalf("issued license: %+v", l)
	}

	// Validate from two machines, then a third is refused.
	v := func(fp string) store.ValidateResult {
		t.Helper()
		res, err := store.ValidateLicense(ctx, c, store.ValidateRequest{Key: strings.ToLower(l.Key), Product: p.Slug, Fingerprint: fp, Label: "host-" + fp})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	if r := v("m1"); !r.Valid || !r.NewActivation || r.Message == "" {
		t.Fatalf("m1: %+v", r)
	}
	if r := v("m1"); !r.Valid || r.NewActivation {
		t.Fatalf("m1 again should reuse its slot: %+v", r)
	}
	v("m2")
	if r := v("m3"); r.Valid || r.Code != "activation_limit" {
		t.Fatalf("m3: %+v", r)
	}

	// Removing a machine frees a slot.
	l = decodeLicense(t, mustGet(t, c, l.ID))
	if len(l.Activations) != 2 {
		t.Fatalf("activations = %d, want 2", len(l.Activations))
	}
	if err := store.DeleteActivation(ctx, c, l.Activations[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteActivation(ctx, c, l.Activations[0].ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second delete: err = %v, want ErrNotFound", err)
	}
	if r := v("m3"); !r.Valid {
		t.Fatalf("m3 after freeing a slot: %+v", r)
	}

	// Revoke, then restore.
	var revoke, restore store.LicensePatch
	_ = json.Unmarshal([]byte(`{"status":"revoked","note":"refund"}`), &revoke)
	_ = json.Unmarshal([]byte(`{"status":"active"}`), &restore)
	if l = decodeLicense(t, must(store.UpdateLicense(ctx, c, l.ID, revoke, now))); l.State != "revoked" {
		t.Fatalf("after revoke: %+v", l)
	}
	if r := v("m1"); r.Valid || r.Code != "revoked" {
		t.Fatalf("revoked key validated: %+v", r)
	}
	if l = decodeLicense(t, must(store.UpdateLicense(ctx, c, l.ID, restore, now))); l.State != "active" {
		t.Fatalf("after restore: %+v", l)
	}
	if r := v("m1"); !r.Valid {
		t.Fatalf("restored key refused: %+v", r)
	}

	// Expire it, then remove the expiry.
	var expire, lifetime store.LicensePatch
	_ = json.Unmarshal([]byte(fmt.Sprintf(`{"expiresAt":%q}`, now.Add(-time.Hour).Format(time.RFC3339))), &expire)
	_ = json.Unmarshal([]byte(`{"expiresAt":null}`), &lifetime)
	store.UpdateLicense(ctx, c, l.ID, expire, now)
	if r := v("m1"); r.Code != "expired" {
		t.Fatalf("expired key: %+v", r)
	}
	store.UpdateLicense(ctx, c, l.ID, lifetime, now)
	if r := v("m1"); !r.Valid {
		t.Fatalf("lifetime key refused: %+v", r)
	}

	if r, _ := store.ValidateLicense(ctx, c, store.ValidateRequest{Key: l.Key, Product: "some-other-product"}); r.Code != "wrong_product" {
		t.Fatalf("wrong product: %+v", r)
	}
	if r, _ := store.ValidateLicense(ctx, c, store.ValidateRequest{Key: "NOPE-0000", Product: p.Slug}); r.Code != "not_found" {
		t.Fatalf("unknown key: %+v", r)
	}
}

func TestIssueValidation(t *testing.T) {
	c := client(t)
	p := newProduct(t, c, "VAL")
	now := time.Now()
	past := now.Add(-time.Minute)
	for name, in := range map[string]store.NewLicense{
		"no product":    {},
		"bad email":     {ProductID: p.ID, CustomerEmail: ptr("not-an-email")},
		"too many":      {ProductID: p.ID, MaxActivations: 20000},
		"expiry passed": {ProductID: p.ID, ExpiresAt: &past},
		"gone product":  {ProductID: "00000000-0000-4000-8000-000000000000"},
	} {
		var ve *store.ValidationError
		if _, err := store.IssueLicense(ctx, c, in, now); !errors.As(err, &ve) {
			t.Errorf("%s: err = %v, want ValidationError", name, err)
		}
	}
}

func TestSearch(t *testing.T) {
	c := client(t)
	p := newProduct(t, c, "SRCH")
	now := time.Now()
	email := uniq("findme") + "@example.com"
	store.IssueLicense(ctx, c, store.NewLicense{ProductID: p.ID, CustomerEmail: &email}, now)
	store.IssueLicense(ctx, c, store.NewLicense{ProductID: p.ID}, now)

	var res struct {
		Total int       `json:"total"`
		Items []license `json:"items"`
	}
	json.Unmarshal(must(store.SearchLicenses(ctx, c, store.LicenseSearch{Query: strings.ToUpper(email[:12])})), &res)
	if res.Total != 1 || res.Items[0].CustomerEmail != email {
		t.Fatalf("search by email: %+v", res)
	}
	json.Unmarshal(must(store.SearchLicenses(ctx, c, store.LicenseSearch{ProductID: p.ID, Limit: 1})), &res)
	if res.Total != 2 || len(res.Items) != 1 {
		t.Fatalf("paging: total=%d items=%d", res.Total, len(res.Items))
	}
	// Characters that are special in SQL LIKE or PostgREST filters are just text.
	json.Unmarshal(must(store.SearchLicenses(ctx, c, store.LicenseSearch{Query: "%,)(*"})), &res)
	if res.Total != 0 {
		t.Fatalf("special characters matched %d licenses", res.Total)
	}
	if _, err := store.SearchLicenses(ctx, c, store.LicenseSearch{State: "bogus"}); err == nil {
		t.Fatal("bad state accepted")
	}
}

func TestConcurrentActivations(t *testing.T) {
	c := client(t)
	p := newProduct(t, c, "RACE")
	l := decodeLicense(t, must(store.IssueLicense(ctx, c, store.NewLicense{ProductID: p.ID, MaxActivations: 3}, time.Now())))

	var wg sync.WaitGroup
	var mu sync.Mutex
	valid := 0
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, err := store.ValidateLicense(ctx, c, store.ValidateRequest{Key: l.Key, Product: p.Slug, Fingerprint: fmt.Sprintf("m%d", i)})
			if err != nil {
				t.Error(err)
				return
			}
			if r.Valid {
				mu.Lock()
				valid++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if valid != 3 {
		t.Fatalf("%d machines activated a 3-machine license", valid)
	}
}

func mustGet(t *testing.T, c *supa.Client, id string) json.RawMessage {
	t.Helper()
	raw, err := store.GetLicense(ctx, c, id)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func must(raw json.RawMessage, err error) json.RawMessage {
	if err != nil {
		panic(err)
	}
	return raw
}
