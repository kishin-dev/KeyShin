package handler

import (
	"net/http"

	"github.com/kishin-dev/keyshin/lib/auth"
	"github.com/kishin-dev/keyshin/lib/httpx"
)

// Session handles GET /api/session. The dashboard calls it on load to check
// whether the admin is signed in, and goes to the login page if not.
func Session(w http.ResponseWriter, r *http.Request) {
	if !httpx.Method(w, r, http.MethodGet) {
		return
	}
	_, admin, ok := auth.Require(w, r)
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, admin)
}
