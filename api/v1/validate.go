// Package handler holds version 1 of KeyShin's public API, the endpoints
// your plugins and services call. They don't need an admin login.
package handler

import (
	"log"
	"net/http"

	"github.com/kishin-dev/keyshin/lib/httpx"
	"github.com/kishin-dev/keyshin/lib/store"
	"github.com/kishin-dev/keyshin/lib/supa"
)

// Validate handles POST /api/v1/validate.
//
// Request:
//
//	{"key": "KSHN-....", "product": "your-product-slug",
//	 "fingerprint": "machine-id", "label": "optional readable name"}
//
// Response (always 200 when the check itself worked):
//
//	{"valid": true, "code": "valid", "message": "...", ...}
//	{"valid": false, "code": "revoked", "message": "..."}
//
// Codes: valid, not_found, wrong_product, revoked, expired, activation_limit.
func Validate(w http.ResponseWriter, r *http.Request) {
	// Let browser-based clients call this too. It returns nothing secret.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !httpx.Method(w, r, http.MethodPost) {
		return
	}

	c, err := supa.FromEnv()
	if err != nil {
		log.Println(err)
		httpx.Error(w, http.StatusInternalServerError, "server is not configured")
		return
	}

	var req store.ValidateRequest
	if !httpx.DecodeJSONLenient(w, r, &req) {
		return
	}
	res, err := store.ValidateLicense(r.Context(), c, req)
	if err != nil {
		store.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}
