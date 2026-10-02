package handler

import (
	"net/http"
	"time"

	"github.com/kishin-dev/keyshin/lib/auth"
	"github.com/kishin-dev/keyshin/lib/httpx"
	"github.com/kishin-dev/keyshin/lib/store"
)

// Licenses handles /api/licenses (admin only).
//
//	GET ?q=&product=&state=&limit=&offset=  search licenses
//	GET ?id=...                              one license with its activations
//	POST                                     issue a license
//	PATCH ?id=...                            edit, revoke or restore a license
func Licenses(w http.ResponseWriter, r *http.Request) {
	svc, _, ok := auth.Require(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	q := r.URL.Query()

	switch r.Method {
	case http.MethodGet:
		if id := q.Get("id"); id != "" {
			lic, err := store.GetLicense(ctx, svc.Supa, id)
			if err != nil {
				store.WriteError(w, err)
				return
			}
			httpx.JSON(w, http.StatusOK, lic)
			return
		}
		res, err := store.SearchLicenses(ctx, svc.Supa, store.LicenseSearch{
			Query:     q.Get("q"),
			ProductID: q.Get("product"),
			State:     q.Get("state"),
			Limit:     httpx.QueryInt(r, "limit", 50),
			Offset:    httpx.QueryInt(r, "offset", 0),
		})
		if err != nil {
			store.WriteError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, res)

	case http.MethodPost:
		var in store.NewLicense
		if !httpx.DecodeJSON(w, r, &in) {
			return
		}
		lic, err := store.IssueLicense(ctx, svc.Supa, in, time.Now())
		if err != nil {
			store.WriteError(w, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, lic)

	case http.MethodPatch:
		var in store.LicensePatch
		if !httpx.DecodeJSON(w, r, &in) {
			return
		}
		lic, err := store.UpdateLicense(ctx, svc.Supa, q.Get("id"), in, time.Now())
		if err != nil {
			store.WriteError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, lic)

	default:
		w.Header().Set("Allow", "GET, POST, PATCH")
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
