package store

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/kishin-dev/keyshin/lib/supa"
)

// ValidateRequest is what a plugin sends to check a license.
type ValidateRequest struct {
	Key         string `json:"key"`
	Product     string `json:"product"`
	Fingerprint string `json:"fingerprint"`
	Label       string `json:"label"`
}

// ValidateResult is KeyShin's answer about a license.
type ValidateResult struct {
	Valid          bool    `json:"valid"`
	Code           string  `json:"code"`
	Message        string  `json:"message"`
	Product        string  `json:"product,omitempty"`
	ExpiresAt      *string `json:"expiresAt,omitempty"`
	MaxActivations *int    `json:"maxActivations,omitempty"`
	Activations    *int    `json:"activations,omitempty"`
	NewActivation  bool    `json:"newActivation,omitempty"`
}

// Messages for each result code, written for the customer: a plugin can
// show them directly.
var validateMessages = map[string]string{
	"valid":            "License is valid.",
	"not_found":        "This license key doesn't exist. Check it was copied correctly.",
	"wrong_product":    "This license key is for a different product.",
	"revoked":          "This license has been revoked. Contact the seller if you think this is a mistake.",
	"expired":          "This license has expired.",
	"activation_limit": "This license is already active on the maximum number of machines. Remove one from your license, or contact the seller.",
}

// ValidateLicense checks a key and records the machine using it.
func ValidateLicense(ctx context.Context, c *supa.Client, req ValidateRequest) (ValidateResult, error) {
	key := strings.TrimSpace(req.Key)
	product := strings.TrimSpace(req.Product)
	fp := strings.TrimSpace(req.Fingerprint)
	label := strings.TrimSpace(req.Label)

	switch {
	case key == "":
		return ValidateResult{}, invalid("key", "key is required")
	case product == "":
		return ValidateResult{}, invalid("product", "product is required (the product's slug)")
	case len(key) > 64:
		return ValidateResult{}, invalid("key", "key is too long")
	case len(product) > 64:
		return ValidateResult{}, invalid("product", "product is too long")
	case len(fp) > 200:
		return ValidateResult{}, invalid("fingerprint", "fingerprint must be at most 200 characters")
	case utf8.RuneCountInString(label) > 120:
		return ValidateResult{}, invalid("label", "label must be at most 120 characters")
	}

	args := map[string]any{"p_key": key, "p_product": product}
	if fp != "" {
		args["p_fingerprint"] = fp
	}
	if label != "" {
		args["p_label"] = label
	}

	var res ValidateResult
	if err := c.RPC(ctx, "keyshin_validate", args, &res); err != nil {
		return ValidateResult{}, err
	}
	res.Message = validateMessages[res.Code]
	return res, nil
}
