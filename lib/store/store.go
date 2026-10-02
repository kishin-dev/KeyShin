// Package store holds the database queries used by the API handlers.
package store

import (
	"context"

	"github.com/kishin-dev/keyshin/lib/supa"
)

// Stats are the totals shown on the dashboard overview.
type Stats struct {
	Products int `json:"products"`
	Active   int `json:"active"`
	Revoked  int `json:"revoked"`
}

// GetStats returns the dashboard totals.
func GetStats(ctx context.Context, c *supa.Client) (Stats, error) {
	var s Stats
	err := c.RPC(ctx, "keyshin_stats", nil, &s)
	return s, err
}
