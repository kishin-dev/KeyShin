package handler

import (
	"log"
	"net/http"

	"github.com/kishin-dev/keyshin/lib/auth"
	"github.com/kishin-dev/keyshin/lib/httpx"
	"github.com/kishin-dev/keyshin/lib/store"
)

// Stats handles GET /api/stats: the totals on the dashboard overview.
func Stats(w http.ResponseWriter, r *http.Request) {
	if !httpx.Method(w, r, http.MethodGet) {
		return
	}
	svc, _, ok := auth.Require(w, r)
	if !ok {
		return
	}
	stats, err := store.GetStats(r.Context(), svc.Supa)
	if err != nil {
		log.Printf("stats: %v", err)
		httpx.Error(w, http.StatusBadGateway, "could not load stats from the database")
		return
	}
	httpx.JSON(w, http.StatusOK, stats)
}
