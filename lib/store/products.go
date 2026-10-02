package store

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kishin-dev/keyshin/lib/supa"
)

var (
	slugRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)
	prefixRe = regexp.MustCompile(`^[A-Z0-9]{2,8}$`)
)

// Product is something you sell access to.
type Product struct {
	ID             string    `json:"id"`
	Slug           string    `json:"slug"`
	Name           string    `json:"name"`
	Description    *string   `json:"description"`
	KeyPrefix      string    `json:"keyPrefix"`
	CreatedAt      time.Time `json:"createdAt"`
	ActiveLicenses int       `json:"activeLicenses"`
	TotalLicenses  int       `json:"totalLicenses"`
}

// productRow is a row of the products table as the REST API returns it.
type productRow struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	KeyPrefix   string    `json:"key_prefix"`
	CreatedAt   time.Time `json:"created_at"`
}

func (r productRow) product() Product {
	return Product{ID: r.ID, Slug: r.Slug, Name: r.Name, Description: r.Description, KeyPrefix: r.KeyPrefix, CreatedAt: r.CreatedAt}
}

// NewProduct is the input for creating a product.
type NewProduct struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description *string `json:"description"`
	KeyPrefix   string  `json:"keyPrefix"`
}

// ProductPatch is the input for editing a product. The slug can't be
// changed, because plugins already in use send it with every check.
type ProductPatch struct {
	Name        Opt[string] `json:"name"`
	Description Opt[string] `json:"description"`
	KeyPrefix   Opt[string] `json:"keyPrefix"`
}

func checkName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", invalid("name", "Enter a product name.")
	}
	if utf8.RuneCountInString(name) > 120 {
		return "", invalid("name", "Keep the name under 120 characters.")
	}
	return name, nil
}

func checkPrefix(p string) (string, error) {
	p = strings.ToUpper(strings.TrimSpace(p))
	if p == "" {
		return "KSHN", nil
	}
	if !prefixRe.MatchString(p) {
		return "", invalid("keyPrefix", "The key prefix must be 2–8 letters or numbers, like KSHN.")
	}
	return p, nil
}

func checkDescription(d *string) (*string, error) {
	d = cleanText(d)
	if d != nil && utf8.RuneCountInString(*d) > 1000 {
		return nil, invalid("description", "Keep the description under 1,000 characters.")
	}
	return d, nil
}

// ListProducts returns every product, newest first, with license counts.
func ListProducts(ctx context.Context, c *supa.Client) ([]Product, error) {
	var out []Product
	if err := c.RPC(ctx, "keyshin_products", nil, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Product{}
	}
	return out, nil
}

// CreateProduct adds a product.
func CreateProduct(ctx context.Context, c *supa.Client, in NewProduct) (Product, error) {
	name, err := checkName(in.Name)
	if err != nil {
		return Product{}, err
	}
	slug := strings.ToLower(strings.TrimSpace(in.Slug))
	if !slugRe.MatchString(slug) {
		return Product{}, invalid("slug", "The slug must be 2–64 lowercase letters, numbers or dashes, like discord-bot-pro.")
	}
	prefix, err := checkPrefix(in.KeyPrefix)
	if err != nil {
		return Product{}, err
	}
	desc, err := checkDescription(in.Description)
	if err != nil {
		return Product{}, err
	}

	var rows []productRow
	err = c.Insert(ctx, "products", map[string]any{
		"name": name, "slug": slug, "key_prefix": prefix, "description": desc,
	}, &rows)
	if supa.IsCode(err, "23505") {
		return Product{}, &ConflictError{Field: "slug", Message: "Another product already uses the slug “" + slug + "”."}
	}
	if err != nil {
		return Product{}, err
	}
	if len(rows) == 0 {
		return Product{}, ErrNotFound
	}
	return rows[0].product(), nil
}

// UpdateProduct edits a product's name, description or key prefix.
func UpdateProduct(ctx context.Context, c *supa.Client, id string, p ProductPatch) (Product, error) {
	if !IsUUID(id) {
		return Product{}, ErrNotFound
	}
	patch := map[string]any{}
	if p.Name.Set {
		if p.Name.Value == nil {
			return Product{}, invalid("name", "Enter a product name.")
		}
		name, err := checkName(*p.Name.Value)
		if err != nil {
			return Product{}, err
		}
		patch["name"] = name
	}
	if p.Description.Set {
		desc, err := checkDescription(p.Description.Value)
		if err != nil {
			return Product{}, err
		}
		patch["description"] = desc
	}
	if p.KeyPrefix.Set {
		var raw string
		if p.KeyPrefix.Value != nil {
			raw = *p.KeyPrefix.Value
		}
		prefix, err := checkPrefix(raw)
		if err != nil {
			return Product{}, err
		}
		patch["key_prefix"] = prefix
	}
	if len(patch) == 0 {
		return Product{}, invalid("", "Nothing to change.")
	}

	var rows []productRow
	if err := c.Update(ctx, "products", url.Values{"id": {"eq." + id}}, patch, &rows); err != nil {
		return Product{}, err
	}
	if len(rows) == 0 {
		return Product{}, ErrNotFound
	}
	return rows[0].product(), nil
}
