// Package httpx has small helpers shared by the API handlers.
package httpx

import (
	"encoding/json"
	"mime"
	"net/http"
	"strconv"
)

// JSON writes v as a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error writes {"error": msg}.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

// Method rejects requests that don't use the expected method.
// It returns false if the handler should stop.
func Method(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		w.Header().Set("Allow", method)
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return false
	}
	return true
}

// DecodeJSON reads a small JSON body into v. It requires the
// application/json content type, which a plain HTML form on another site
// cannot send, as an extra layer of CSRF protection.
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	return decode(w, r, v, true)
}

// DecodeJSONLenient is DecodeJSON but ignores fields it doesn't know, for
// the public API, so clients can send extra data without breaking.
func DecodeJSONLenient(w http.ResponseWriter, r *http.Request, v any) bool {
	return decode(w, r, v, false)
}

func decode(w http.ResponseWriter, r *http.Request, v any, strict bool) bool {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "application/json" {
		Error(w, http.StatusUnsupportedMediaType, "expected application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	dec := json.NewDecoder(r.Body)
	if strict {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(v); err != nil {
		Error(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// QueryInt reads an integer query parameter, or returns def.
func QueryInt(r *http.Request, name string, def int) int {
	if n, err := strconv.Atoi(r.URL.Query().Get(name)); err == nil {
		return n
	}
	return def
}
