// Package supa is a small client for the parts of Supabase KeyShin uses:
// Auth (sign in, refresh, look up the user, sign out) and the database's
// REST API (calling SQL functions and reading tables).
//
// It uses only the Go standard library and talks HTTPS, which suits Vercel's
// serverless functions better than holding open Postgres connections.
package supa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ErrNotConfigured is returned when the Supabase environment variables are missing.
var ErrNotConfigured = errors.New("supa: SUPABASE_URL, SUPABASE_PUBLISHABLE_KEY and SUPABASE_SECRET_KEY must be set")

// ErrInvalidCredentials means the email/password or a token was rejected.
var ErrInvalidCredentials = errors.New("supa: invalid credentials")

// Client talks to one Supabase project.
type Client struct {
	BaseURL string // e.g. https://abcd.supabase.co
	// PublishableKey (sb_publishable_... or the legacy anon key) is used for
	// Auth calls made on behalf of a user.
	PublishableKey string
	// SecretKey (sb_secret_... or the legacy service_role key) is used for
	// database access. It bypasses Row Level Security, so it only ever lives
	// in server environment variables.
	SecretKey string
	HTTP      *http.Client
}

// FromEnv builds a client from environment variables.
func FromEnv() (*Client, error) {
	c := &Client{
		BaseURL:        strings.TrimRight(os.Getenv("SUPABASE_URL"), "/"),
		PublishableKey: os.Getenv("SUPABASE_PUBLISHABLE_KEY"),
		SecretKey:      os.Getenv("SUPABASE_SECRET_KEY"),
		HTTP:           &http.Client{Timeout: 8 * time.Second},
	}
	if c.BaseURL == "" || c.PublishableKey == "" || c.SecretKey == "" {
		return nil, ErrNotConfigured
	}
	return c, nil
}

// APIError is a non-2xx response from Supabase.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("supabase: %d %s: %s", e.Status, e.Code, e.Message)
}

// setKey adds an API key to a request. New keys (sb_...) go only in the
// apikey header; legacy JWT keys also go in Authorization so the database
// runs with that key's role.
func setKey(req *http.Request, key string, bearer bool) {
	req.Header.Set("apikey", key)
	if bearer && !strings.HasPrefix(key, "sb_") {
		req.Header.Set("Authorization", "Bearer "+key)
	}
}

func (c *Client) do(req *http.Request, out any) (*http.Response, error) {
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return res, err
	}
	if res.StatusCode >= 300 {
		return res, parseError(res.StatusCode, body)
	}
	if out != nil && len(body) > 0 {
		if err := json.Unmarshal(body, out); err != nil {
			return res, fmt.Errorf("supabase: decoding response: %w", err)
		}
	}
	return res, nil
}

func parseError(status int, body []byte) error {
	// Auth and PostgREST use different error shapes; read whichever is there.
	var e struct {
		Code             any    `json:"code"`
		ErrorCode        string `json:"error_code"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		Msg              string `json:"msg"`
		Message          string `json:"message"`
	}
	_ = json.Unmarshal(body, &e)
	ae := &APIError{Status: status}
	switch {
	case e.ErrorCode != "":
		ae.Code = e.ErrorCode
	case e.Error != "":
		ae.Code = e.Error
	case e.Code != nil:
		ae.Code = fmt.Sprint(e.Code)
	}
	for _, m := range []string{e.Msg, e.Message, e.ErrorDescription} {
		if m != "" {
			ae.Message = m
			break
		}
	}
	if ae.Message == "" {
		ae.Message = http.StatusText(status)
	}
	return ae
}

func jsonBody(v any) (io.Reader, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

// ---------------------------------------------------------------- Auth

// User is a Supabase Auth user.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// Session is what Supabase Auth returns after signing in or refreshing.
type Session struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	User         User   `json:"user"`
}

func (c *Client) authRequest(ctx context.Context, method, path string, body any, bearer string) (*http.Request, error) {
	var r io.Reader
	if body != nil {
		var err error
		if r, err = jsonBody(body); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/auth/v1"+path, r)
	if err != nil {
		return nil, err
	}
	setKey(req, c.PublishableKey, bearer == "")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// asCredentialError turns "wrong password / bad token" responses into
// ErrInvalidCredentials and leaves real failures (Supabase down, bad config)
// as they are, so they can be logged and shown differently.
func asCredentialError(err error) error {
	var ae *APIError
	if errors.As(err, &ae) && (ae.Status == 400 || ae.Status == 401 || ae.Status == 403 || ae.Status == 404) {
		return ErrInvalidCredentials
	}
	return err
}

// SignInWithPassword exchanges an email and password for a session.
func (c *Client) SignInWithPassword(ctx context.Context, email, password string) (Session, error) {
	req, err := c.authRequest(ctx, http.MethodPost, "/token?grant_type=password",
		map[string]string{"email": email, "password": password}, "")
	if err != nil {
		return Session{}, err
	}
	var s Session
	if _, err := c.do(req, &s); err != nil {
		return Session{}, asCredentialError(err)
	}
	return s, nil
}

// Refresh exchanges a refresh token for a new session. Supabase rotates
// refresh tokens, so the returned RefreshToken replaces the old one.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (Session, error) {
	req, err := c.authRequest(ctx, http.MethodPost, "/token?grant_type=refresh_token",
		map[string]string{"refresh_token": refreshToken}, "")
	if err != nil {
		return Session{}, err
	}
	var s Session
	if _, err := c.do(req, &s); err != nil {
		return Session{}, asCredentialError(err)
	}
	return s, nil
}

// GetUser checks an access token with Supabase and returns its user. Asking
// Supabase (rather than only checking the token's signature) means a signed-
// out or deleted admin is rejected immediately.
func (c *Client) GetUser(ctx context.Context, accessToken string) (User, error) {
	req, err := c.authRequest(ctx, http.MethodGet, "/user", nil, accessToken)
	if err != nil {
		return User{}, err
	}
	var u User
	if _, err := c.do(req, &u); err != nil {
		return User{}, asCredentialError(err)
	}
	return u, nil
}

// SignOut revokes the session behind accessToken.
func (c *Client) SignOut(ctx context.Context, accessToken string) error {
	req, err := c.authRequest(ctx, http.MethodPost, "/logout?scope=local", nil, accessToken)
	if err != nil {
		return err
	}
	_, err = c.do(req, nil)
	return err
}

// ---------------------------------------------------------------- Database

func (c *Client) restRequest(ctx context.Context, method, path string, query url.Values, body any) (*http.Request, error) {
	u := c.BaseURL + "/rest/v1/" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var r io.Reader
	if body != nil {
		var err error
		if r, err = jsonBody(body); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return nil, err
	}
	setKey(req, c.SecretKey, true)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// RPC calls a SQL function: POST /rest/v1/rpc/<fn>.
func (c *Client) RPC(ctx context.Context, fn string, args, out any) error {
	if args == nil {
		args = struct{}{}
	}
	req, err := c.restRequest(ctx, http.MethodPost, "rpc/"+fn, nil, args)
	if err != nil {
		return err
	}
	_, err = c.do(req, out)
	return err
}

// Select reads rows from a table: GET /rest/v1/<table>?<query>.
// out should be a pointer to a slice.
func (c *Client) Select(ctx context.Context, table string, query url.Values, out any) error {
	req, err := c.restRequest(ctx, http.MethodGet, table, query, nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, out)
	return err
}
