package store

import (
	"regexp"
	"testing"
)

func TestNewKey(t *testing.T) {
	re := regexp.MustCompile(`^KSHN(-[2-9A-HJ-NP-Z]{4}){4}$`)
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		k, err := NewKey("KSHN")
		if err != nil {
			t.Fatal(err)
		}
		if !re.MatchString(k) {
			t.Fatalf("bad key %q", k)
		}
		if seen[k] {
			t.Fatalf("duplicate key %q", k)
		}
		seen[k] = true
	}
}
