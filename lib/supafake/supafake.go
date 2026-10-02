// Package supafake is an in-memory stand-in for the parts of Supabase that
// KeyShin uses. It's for tests and for trying the dashboard locally without
// a real project:
//
//	go run ./cmd/dev -fake
package supafake

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

const (
	PublishableKey = "sb_publishable_fake"
	SecretKey      = "sb_secret_fake"
)

type user struct {
	id, email, password string
}

type session struct {
	userID  string
	expired bool
}

// Server is a fake Supabase project.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	n        int
	users    map[string]*user  // by email
	admins   map[string]string // username -> user id
	access   map[string]*session
	refresh  map[string]string // refresh token -> user id
	Stats    map[string]int
	Requests int
}

// New starts a fake Supabase server. Call Close when done.
func New() *Server {
	s := &Server{
		users:   map[string]*user{},
		admins:  map[string]string{},
		access:  map[string]*session{},
		refresh: map[string]string{},
		Stats:   map[string]int{"products": 0, "active": 0, "revoked": 0},
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

// AddUser creates an Auth user and returns its id.
func (s *Server) AddUser(email, password string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
	id := fmt.Sprintf("00000000-0000-4000-8000-%012d", s.n)
	s.users[email] = &user{id: id, email: email, password: password}
	return id
}

// AddAdmin gives a user a dashboard username.
func (s *Server) AddAdmin(username, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.admins[username] = userID
}

// RemoveAdmin takes away dashboard access.
func (s *Server) RemoveAdmin(username string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.admins, username)
}

// ExpireAccessTokens makes every current access token expired, as happens
// an hour after it was issued.
func (s *Server) ExpireAccessTokens() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.access {
		sess.expired = true
	}
}

func (s *Server) newSession(u *user) map[string]any {
	s.n++
	at, rt := fmt.Sprintf("at-%d", s.n), fmt.Sprintf("rt-%d", s.n)
	s.access[at] = &session{userID: u.id}
	s.refresh[rt] = u.id
	return map[string]any{
		"access_token": at, "refresh_token": rt, "expires_in": 3600, "token_type": "bearer",
		"user": map[string]string{"id": u.id, "email": u.email},
	}
}

func (s *Server) userByID(id string) *user {
	for _, u := range s.users {
		if u.id == id {
			return u
		}
	}
	return nil
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Requests++

	key := r.Header.Get("apikey")
	if key != PublishableKey && key != SecretKey {
		reply(w, 401, map[string]string{"message": "Invalid API key"})
		return
	}
	// New-style keys must never be sent as a bearer token.
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer sb_") {
		reply(w, 401, map[string]string{"message": "sb_ keys belong in the apikey header"})
		return
	}
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	var body map[string]string
	_ = json.NewDecoder(r.Body).Decode(&body)

	switch {
	case r.URL.Path == "/auth/v1/token" && r.URL.Query().Get("grant_type") == "password":
		u := s.users[body["email"]]
		if u == nil || u.password != body["password"] {
			reply(w, 400, map[string]string{"error_code": "invalid_credentials", "msg": "Invalid login credentials"})
			return
		}
		reply(w, 200, s.newSession(u))

	case r.URL.Path == "/auth/v1/token" && r.URL.Query().Get("grant_type") == "refresh_token":
		id, ok := s.refresh[body["refresh_token"]]
		if !ok {
			reply(w, 400, map[string]string{"error_code": "refresh_token_not_found", "msg": "Invalid Refresh Token"})
			return
		}
		delete(s.refresh, body["refresh_token"]) // rotation: old token is single-use
		reply(w, 200, s.newSession(s.userByID(id)))

	case r.URL.Path == "/auth/v1/user":
		sess := s.access[bearer]
		if sess == nil || sess.expired {
			reply(w, 403, map[string]any{"code": 403, "error_code": "bad_jwt", "msg": "invalid JWT"})
			return
		}
		u := s.userByID(sess.userID)
		reply(w, 200, map[string]string{"id": u.id, "email": u.email})

	case r.URL.Path == "/auth/v1/logout":
		if sess := s.access[bearer]; sess != nil {
			for t, a := range s.access {
				if a.userID == sess.userID {
					delete(s.access, t)
				}
			}
			for t, id := range s.refresh {
				if id == sess.userID {
					delete(s.refresh, t)
				}
			}
		}
		w.WriteHeader(204)

	case strings.HasPrefix(r.URL.Path, "/rest/v1/"):
		if key != SecretKey {
			reply(w, 401, map[string]string{"code": "42501", "message": "permission denied"})
			return
		}
		s.rest(w, r, body)

	default:
		reply(w, 404, map[string]string{"message": "not found"})
	}
}

func (s *Server) rest(w http.ResponseWriter, r *http.Request, body map[string]string) {
	switch r.URL.Path {
	case "/rest/v1/rpc/keyshin_admin_email":
		name := strings.ToLower(strings.TrimSpace(body["p_username"]))
		if id, ok := s.admins[name]; ok {
			reply(w, 200, s.userByID(id).email)
			return
		}
		reply(w, 200, nil)
	case "/rest/v1/rpc/keyshin_stats":
		reply(w, 200, s.Stats)
	case "/rest/v1/admins":
		id := strings.TrimPrefix(r.URL.Query().Get("user_id"), "eq.")
		rows := []map[string]string{}
		for name, uid := range s.admins {
			if uid == id {
				rows = append(rows, map[string]string{"username": name})
			}
		}
		reply(w, 200, rows)
	default:
		reply(w, 404, map[string]string{"message": "not found"})
	}
}
