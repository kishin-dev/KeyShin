package handler

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/kishin-dev/keyshin/lib/auth"
	"github.com/kishin-dev/keyshin/lib/httpx"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login handles POST /api/login.
func Login(w http.ResponseWriter, r *http.Request) {
	if !httpx.Method(w, r, http.MethodPost) {
		return
	}
	svc, err := auth.FromEnv()
	if err != nil {
		log.Println(err)
		httpx.Error(w, http.StatusInternalServerError, "server is not configured")
		return
	}

	var req loginRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	admin, err := svc.Login(r.Context(), w, req.Username, req.Password)
	switch {
	case err == nil:
		httpx.JSON(w, http.StatusOK, admin)
	case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrNotAdmin):
		// Same answer for "no such user" and "wrong password", so the
		// response doesn't reveal which usernames exist. The pause slows
		// down guessing on top of Supabase's own rate limits.
		time.Sleep(400 * time.Millisecond)
		httpx.Error(w, http.StatusUnauthorized, "invalid username or password")
	default:
		log.Printf("login: %v", err)
		httpx.Error(w, http.StatusBadGateway, "could not reach the authentication service")
	}
}
