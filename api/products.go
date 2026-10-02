package handler

import (
	"net/http"

	"github.com/kishin-dev/keyshin/lib/auth"
	"github.com/kishin-dev/keyshin/lib/httpx"
	"github.com/kishin-dev/keyshin/lib/store"
)

// Products handles /api/products (admin only).
//
//	GET            list products
//	POST           create a product
//	PATCH ?id=...  edit a product
func Products(w http.ResponseWriter, r *http.Request) {
	svc, _, ok := auth.Require(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		products, err := store.ListProducts(ctx, svc.Supa)
		if err != nil {
			store.WriteError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, products)

	case http.MethodPost:
		var in store.NewProduct
		if !httpx.DecodeJSON(w, r, &in) {
			return
		}
		p, err := store.CreateProduct(ctx, svc.Supa, in)
		if err != nil {
			store.WriteError(w, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, p)

	case http.MethodPatch:
		var in store.ProductPatch
		if !httpx.DecodeJSON(w, r, &in) {
			return
		}
		p, err := store.UpdateProduct(ctx, svc.Supa, r.URL.Query().Get("id"), in)
		if err != nil {
			store.WriteError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, p)

	default:
		w.Header().Set("Allow", "GET, POST, PATCH")
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
