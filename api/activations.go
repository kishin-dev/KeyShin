package handler

import (
	"net/http"

	"github.com/kishin-dev/keyshin/lib/auth"
	"github.com/kishin-dev/keyshin/lib/httpx"
	"github.com/kishin-dev/keyshin/lib/store"
)

// Activations handles /api/activations (admin only).
//
//	DELETE ?id=...  remove a machine from its license, freeing the slot
func Activations(w http.ResponseWriter, r *http.Request) {
	if !httpx.Method(w, r, http.MethodDelete) {
		return
	}
	svc, _, ok := auth.Require(w, r)
	if !ok {
		return
	}
	if err := store.DeleteActivation(r.Context(), svc.Supa, r.URL.Query().Get("id")); err != nil {
		store.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}
