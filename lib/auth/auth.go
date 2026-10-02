// Package auth handles admin sign-in for the KeyShin dashboard using
// Supabase Auth.
//
// How it works:
//   - Admins sign in with a username. The username is looked up in the
//     admins table to find the Supabase Auth email, and Supabase checks the
//     password. Being a Supabase user is not enough: only users listed in the
//     admins table can sign in.
//   - Supabase's access and refresh tokens are kept in HttpOnly cookies, so
//     JavaScript in the browser can never read them.
//   - Every request checks the access token with Supabase. When it has
//     expired (after an hour), the refresh token quietly gets a new one.
package auth

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/kishin-dev/keyshin/lib/httpx"
	"github.com/kishin-dev/keyshin/lib/supa"
)

// IdleTimeout is how long an admin stays signed in without using the dashboard.
const IdleTimeout = 12 * time.Hour

var (
	// ErrInvalidCredentials means the username or password was wrong.
	ErrInvalidCredentials = errors.New("auth: invalid username or password")
	// ErrNotSignedIn means there's no valid session.
	ErrNotSignedIn = errors.New("auth: not signed in")
	// ErrNotAdmin means the Supabase user isn't in the admins table.
	ErrNotAdmin = errors.New("auth: user is not an admin")
)

var (
	usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{2,31}$`)
	uuidRe     = regexp.MustCompile(`^[0-9a-fA-F-]{36}$`)
)

// Admin is a signed-in dashboard admin.
type Admin struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// Service signs admins in and out.
type Service struct {
	Supa *supa.Client
	// Dev drops the Secure flag and __Host- prefix from cookies so they work
	// over plain http://localhost.
	Dev bool
}

// FromEnv builds the service from environment variables.
func FromEnv() (*Service, error) {
	c, err := supa.FromEnv()
	if err != nil {
		return nil, err
	}
	return &Service{Supa: c, Dev: os.Getenv("KEYSHIN_DEV") == "1"}, nil
}

// ---------------------------------------------------------------- cookies

func (s *Service) cookieName(base string) string {
	if s.Dev {
		return "ks_" + base
	}
	// __Host- cookies must be Secure, Path=/ and have no Domain, so they
	// can't be set or overwritten by any other subdomain of kishin.lol.
	return "__Host-ks_" + base
}

func (s *Service) setCookie(w http.ResponseWriter, base, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName(base),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   !s.Dev,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Service) storeSession(w http.ResponseWriter, sess supa.Session) {
	age := int(IdleTimeout.Seconds())
	s.setCookie(w, "access", sess.AccessToken, age)
	s.setCookie(w, "refresh", sess.RefreshToken, age)
}

// ClearSession removes the session cookies.
func (s *Service) ClearSession(w http.ResponseWriter) {
	s.setCookie(w, "access", "", -1)
	s.setCookie(w, "refresh", "", -1)
}

func (s *Service) cookie(r *http.Request, base string) string {
	if c, err := r.Cookie(s.cookieName(base)); err == nil {
		return c.Value
	}
	return ""
}

// ---------------------------------------------------------------- sign in

// Login checks a username and password and, if they're right, starts a
// session by setting cookies on w.
func (s *Service) Login(ctx context.Context, w http.ResponseWriter, username, password string) (Admin, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if !usernameRe.MatchString(username) || password == "" {
		return Admin{}, ErrInvalidCredentials
	}

	var email *string
	if err := s.Supa.RPC(ctx, "keyshin_admin_email", map[string]string{"p_username": username}, &email); err != nil {
		return Admin{}, err
	}
	if email == nil || *email == "" {
		return Admin{}, ErrInvalidCredentials
	}

	sess, err := s.Supa.SignInWithPassword(ctx, *email, password)
	if errors.Is(err, supa.ErrInvalidCredentials) {
		return Admin{}, ErrInvalidCredentials
	}
	if err != nil {
		return Admin{}, err
	}

	admin, err := s.adminByUserID(ctx, sess.User.ID)
	if err != nil {
		return Admin{}, err
	}
	admin.Email = sess.User.Email
	s.storeSession(w, sess)
	return admin, nil
}

// Authenticate returns the admin signed in on r. It refreshes an expired
// access token when it can and writes the new cookies to w.
func (s *Service) Authenticate(w http.ResponseWriter, r *http.Request) (Admin, error) {
	ctx := r.Context()
	access, refresh := s.cookie(r, "access"), s.cookie(r, "refresh")
	if access == "" && refresh == "" {
		return Admin{}, ErrNotSignedIn
	}
	if admin, ok := cacheGet(access, time.Now()); ok {
		return admin, nil
	}

	var user supa.User
	var err error = supa.ErrInvalidCredentials
	if access != "" {
		user, err = s.Supa.GetUser(ctx, access)
	}
	if errors.Is(err, supa.ErrInvalidCredentials) {
		if refresh == "" {
			s.ClearSession(w)
			return Admin{}, ErrNotSignedIn
		}
		sess, rerr := s.Supa.Refresh(ctx, refresh)
		if errors.Is(rerr, supa.ErrInvalidCredentials) {
			s.ClearSession(w)
			return Admin{}, ErrNotSignedIn
		}
		if rerr != nil {
			return Admin{}, rerr
		}
		s.storeSession(w, sess)
		user, err = sess.User, nil
		access = sess.AccessToken
	}
	if err != nil {
		return Admin{}, err
	}

	admin, err := s.adminByUserID(ctx, user.ID)
	if errors.Is(err, ErrNotAdmin) {
		s.ClearSession(w)
	}
	if err != nil {
		return Admin{}, err
	}
	admin.Email = user.Email
	cachePut(access, admin, time.Now())
	return admin, nil
}

// Logout ends the session with Supabase and clears the cookies.
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	if access := s.cookie(r, "access"); access != "" {
		cacheDelete(access)
		if err := s.Supa.SignOut(r.Context(), access); err != nil && !errors.Is(err, context.Canceled) {
			// The cookies are cleared either way; log in case Supabase is down.
			log.Printf("auth: sign out: %v", err)
		}
	}
	s.ClearSession(w)
}

func (s *Service) adminByUserID(ctx context.Context, userID string) (Admin, error) {
	if !uuidRe.MatchString(userID) {
		return Admin{}, ErrNotAdmin
	}
	var rows []struct {
		Username string `json:"username"`
	}
	q := url.Values{"select": {"username"}, "user_id": {"eq." + userID}, "limit": {"1"}}
	if err := s.Supa.Select(ctx, "admins", q, &rows); err != nil {
		return Admin{}, err
	}
	if len(rows) == 0 {
		return Admin{}, ErrNotAdmin
	}
	return Admin{UserID: userID, Username: rows[0].Username}, nil
}

// ---------------------------------------------------------------- handlers

// Require is the first line of every admin-only API handler. It writes an
// error response and returns ok=false if the request isn't from an admin.
//
//	svc, admin, ok := auth.Require(w, r)
//	if !ok { return }
func Require(w http.ResponseWriter, r *http.Request) (svc *Service, admin Admin, ok bool) {
	svc, err := FromEnv()
	if err != nil {
		log.Println(err)
		httpx.Error(w, http.StatusInternalServerError, "server is not configured")
		return nil, Admin{}, false
	}
	admin, err = svc.Authenticate(w, r)
	switch {
	case err == nil:
		return svc, admin, true
	case errors.Is(err, ErrNotSignedIn), errors.Is(err, ErrNotAdmin):
		httpx.Error(w, http.StatusUnauthorized, "not signed in")
	default:
		log.Printf("auth: %v", err)
		httpx.Error(w, http.StatusBadGateway, "could not reach the authentication service")
	}
	return nil, Admin{}, false
}
