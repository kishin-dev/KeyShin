package store

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kishin-dev/keyshin/lib/supa"
)

// LicenseSearch filters the license list.
type LicenseSearch struct {
	Query     string // part of a key, customer name or email
	ProductID string
	State     string // "active", "expired", "revoked" or ""
	Limit     int
	Offset    int
}

// NewLicense is the input for issuing a license.
type NewLicense struct {
	ProductID      string     `json:"productId"`
	CustomerName   *string    `json:"customerName"`
	CustomerEmail  *string    `json:"customerEmail"`
	Note           *string    `json:"note"`
	MaxActivations int        `json:"maxActivations"`
	ExpiresAt      *time.Time `json:"expiresAt"`
}

// LicensePatch is the input for editing a license. Setting status to
// "revoked" revokes it; "active" restores it.
type LicensePatch struct {
	Status         Opt[string]    `json:"status"`
	CustomerName   Opt[string]    `json:"customerName"`
	CustomerEmail  Opt[string]    `json:"customerEmail"`
	Note           Opt[string]    `json:"note"`
	MaxActivations Opt[int]       `json:"maxActivations"`
	ExpiresAt      Opt[time.Time] `json:"expiresAt"`
}

const maxActivationsLimit = 10000

func checkCustomerName(s *string) (*string, error) {
	s = cleanText(s)
	if s != nil && utf8.RuneCountInString(*s) > 200 {
		return nil, invalid("customerName", "Keep the customer name under 200 characters.")
	}
	return s, nil
}

func checkEmail(s *string) (*string, error) {
	s = cleanText(s)
	if s == nil {
		return nil, nil
	}
	at := strings.LastIndexByte(*s, '@')
	if len(*s) > 254 || at < 1 || at == len(*s)-1 || strings.ContainsAny(*s, " \t\r\n") {
		return nil, invalid("customerEmail", "Enter a valid email address, or leave it empty.")
	}
	return s, nil
}

func checkNote(s *string) (*string, error) {
	s = cleanText(s)
	if s != nil && utf8.RuneCountInString(*s) > 2000 {
		return nil, invalid("note", "Keep the note under 2,000 characters.")
	}
	return s, nil
}

func checkMax(n int) error {
	if n < 1 || n > maxActivationsLimit {
		return invalid("maxActivations", "Machines allowed must be between 1 and 10,000.")
	}
	return nil
}

// SearchLicenses returns {"items": [...], "total": n} as built by the
// keyshin_search_licenses SQL function.
func SearchLicenses(ctx context.Context, c *supa.Client, s LicenseSearch) (json.RawMessage, error) {
	args := map[string]any{
		"p_query":  strings.TrimSpace(s.Query),
		"p_limit":  s.Limit,
		"p_offset": s.Offset,
	}
	if s.ProductID != "" {
		if !IsUUID(s.ProductID) {
			return nil, invalid("product", "Unknown product.")
		}
		args["p_product"] = s.ProductID
	}
	switch s.State {
	case "":
	case "active", "expired", "revoked":
		args["p_state"] = s.State
	default:
		return nil, invalid("state", "State must be active, expired or revoked.")
	}
	var out json.RawMessage
	err := c.RPC(ctx, "keyshin_search_licenses", args, &out)
	return out, err
}

// GetLicense returns one license with its product and activations.
func GetLicense(ctx context.Context, c *supa.Client, id string) (json.RawMessage, error) {
	if !IsUUID(id) {
		return nil, ErrNotFound
	}
	var out json.RawMessage
	if err := c.RPC(ctx, "keyshin_license", map[string]string{"p_id": id}, &out); err != nil {
		return nil, err
	}
	if len(out) == 0 || string(out) == "null" {
		return nil, ErrNotFound
	}
	return out, nil
}

// IssueLicense creates a license with a new random key and returns it.
func IssueLicense(ctx context.Context, c *supa.Client, in NewLicense, now time.Time) (json.RawMessage, error) {
	if !IsUUID(in.ProductID) {
		return nil, invalid("productId", "Choose a product.")
	}
	name, err := checkCustomerName(in.CustomerName)
	if err != nil {
		return nil, err
	}
	email, err := checkEmail(in.CustomerEmail)
	if err != nil {
		return nil, err
	}
	note, err := checkNote(in.Note)
	if err != nil {
		return nil, err
	}
	if in.MaxActivations == 0 {
		in.MaxActivations = 1
	}
	if err := checkMax(in.MaxActivations); err != nil {
		return nil, err
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(now) {
		return nil, invalid("expiresAt", "The expiry date must be in the future, or leave it empty for a lifetime license.")
	}

	var products []struct {
		KeyPrefix string `json:"key_prefix"`
	}
	q := url.Values{"select": {"key_prefix"}, "id": {"eq." + in.ProductID}}
	if err := c.Select(ctx, "products", q, &products); err != nil {
		return nil, err
	}
	if len(products) == 0 {
		return nil, invalid("productId", "That product no longer exists.")
	}

	// A duplicate key is astronomically unlikely, but retry if it happens.
	for attempt := 0; ; attempt++ {
		key, err := NewKey(products[0].KeyPrefix)
		if err != nil {
			return nil, err
		}
		var rows []struct {
			ID string `json:"id"`
		}
		err = c.Insert(ctx, "licenses", map[string]any{
			"product_id":      in.ProductID,
			"key":             key,
			"customer_name":   name,
			"customer_email":  email,
			"note":            note,
			"max_activations": in.MaxActivations,
			"expires_at":      in.ExpiresAt,
		}, &rows)
		if supa.IsCode(err, "23505") && attempt < 3 {
			continue
		}
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, ErrNotFound
		}
		return GetLicense(ctx, c, rows[0].ID)
	}
}

// UpdateLicense edits a license, including revoking and restoring it.
func UpdateLicense(ctx context.Context, c *supa.Client, id string, p LicensePatch, now time.Time) (json.RawMessage, error) {
	if !IsUUID(id) {
		return nil, ErrNotFound
	}
	patch := map[string]any{}
	if p.Status.Set {
		switch {
		case p.Status.Value != nil && *p.Status.Value == "revoked":
			patch["status"] = "revoked"
			patch["revoked_at"] = now
		case p.Status.Value != nil && *p.Status.Value == "active":
			patch["status"] = "active"
			patch["revoked_at"] = nil
		default:
			return nil, invalid("status", "Status must be active or revoked.")
		}
	}
	if p.CustomerName.Set {
		v, err := checkCustomerName(p.CustomerName.Value)
		if err != nil {
			return nil, err
		}
		patch["customer_name"] = v
	}
	if p.CustomerEmail.Set {
		v, err := checkEmail(p.CustomerEmail.Value)
		if err != nil {
			return nil, err
		}
		patch["customer_email"] = v
	}
	if p.Note.Set {
		v, err := checkNote(p.Note.Value)
		if err != nil {
			return nil, err
		}
		patch["note"] = v
	}
	if p.MaxActivations.Set {
		if p.MaxActivations.Value == nil {
			return nil, invalid("maxActivations", "Machines allowed must be between 1 and 10,000.")
		}
		if err := checkMax(*p.MaxActivations.Value); err != nil {
			return nil, err
		}
		patch["max_activations"] = *p.MaxActivations.Value
	}
	if p.ExpiresAt.Set {
		patch["expires_at"] = p.ExpiresAt.Value // nil removes the expiry
	}
	if len(patch) == 0 {
		return nil, invalid("", "Nothing to change.")
	}

	var rows []struct {
		ID string `json:"id"`
	}
	if err := c.Update(ctx, "licenses", url.Values{"id": {"eq." + id}, "select": {"id"}}, patch, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return GetLicense(ctx, c, id)
}

// DeleteActivation removes a machine from a license, freeing up a slot.
func DeleteActivation(ctx context.Context, c *supa.Client, id string) error {
	if !IsUUID(id) {
		return ErrNotFound
	}
	n, err := c.Delete(ctx, "activations", url.Values{"id": {"eq." + id}})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
