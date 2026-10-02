package store

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/kishin-dev/keyshin/lib/httpx"
)

var (
	// ErrNotFound means the product, license or activation doesn't exist.
	ErrNotFound = errors.New("not found")
	uuidRe      = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// ValidationError is a problem with something the admin typed. Its message
// is shown in the dashboard as is.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// ConflictError means the change clashes with existing data, like a
// duplicate product slug.
type ConflictError struct {
	Field   string
	Message string
}

func (e *ConflictError) Error() string { return e.Message }

// IsUUID reports whether s looks like a database id.
func IsUUID(s string) bool { return uuidRe.MatchString(s) }

// WriteError sends the right HTTP response for an error returned by this
// package: 400 for bad input, 404, 409, and 502 for database failures.
func WriteError(w http.ResponseWriter, err error) {
	var ve *ValidationError
	var ce *ConflictError
	switch {
	case errors.As(err, &ve):
		httpx.JSON(w, http.StatusBadRequest, map[string]string{"error": ve.Message, "field": ve.Field})
	case errors.As(err, &ce):
		httpx.JSON(w, http.StatusConflict, map[string]string{"error": ce.Message, "field": ce.Field})
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not found")
	default:
		log.Printf("store: %v", err)
		httpx.Error(w, http.StatusBadGateway, "the database request failed")
	}
}

// Opt is a field in a PATCH request. It tells "not sent" (Set is false)
// apart from "sent as null" (Set is true, Value is nil), so a field can be
// cleared, e.g. removing a license's expiry date.
type Opt[T any] struct {
	Set   bool
	Value *T
}

func (o *Opt[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// cleanText trims s and turns an empty string into nil (stored as NULL).
func cleanText(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}
