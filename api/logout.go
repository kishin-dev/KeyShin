package handler

import (
	"net/http"
	"os"

	"github.com/kishin-dev/keyshin/lib/auth"
	"github.com/kishin-dev/keyshin/lib/httpx"
)

// Logout handles POST /api/logout.
func Logout(w http.ResponseWriter, r *http.Request) {
	if !httpx.Method(w, r, http.MethodPost) {
		return
	}
	svc, err := auth.FromEnv()
	if err != nil {
		// Still clear the cookies so logging out always works.
		svc = &auth.Service{Dev: os.Getenv("KEYSHIN_DEV") == "1"}
		svc.ClearSession(w)
	} else {
		svc.Logout(w, r)
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}
