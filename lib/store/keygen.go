package store

import (
	"crypto/rand"
	"strings"
)

// keyAlphabet leaves out 0, 1, I and O, which are easy to mix up when a
// customer types a key by hand. 32 symbols = 5 bits each.
const keyAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

// NewKey makes a random license key like KSHN-8F3A-Q2LM-7XWD-K9PT.
// The four groups carry 80 random bits, far too many to guess.
func NewKey(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString(prefix)
	for i, v := range b {
		if i%4 == 0 {
			sb.WriteByte('-')
		}
		// 256 is a multiple of 32, so masking keeps every symbol equally likely.
		sb.WriteByte(keyAlphabet[v&31])
	}
	return sb.String(), nil
}
