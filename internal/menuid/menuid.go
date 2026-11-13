// Package menuid derives the public identifier under which a user's menu is
// published.
//
// The id is base62(HMAC-SHA256(secret, userID)[:12]). It is a keyed one-way
// function: without the secret it can be neither reversed nor recomputed, even
// though Telegram user IDs are a small, enumerable space. Earlier versions
// embedded the user ID in the id itself; this package deliberately has no
// decode function.
package menuid

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

const (
	// MinSecretLen is the minimum accepted length of the HMAC secret.
	MinSecretLen = 32

	// macBytes is how much of the HMAC output is kept: 96 bits keeps ids short
	// while making collisions between users negligible.
	macBytes = 12

	// Len is the fixed length of every id: the smallest n with 62^n >= 2^96.
	Len = 17

	alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	// domain separates this use of the secret from any other HMAC made with it.
	domain = "decarta/menu-id/v1\x00"
)

// ErrWeakSecret is returned by New when the secret is too short or is a
// publicly known value.
var ErrWeakSecret = errors.New("menuid: secret must be at least 32 characters and not a published default")

// legacyDefault was hard-coded in early versions of this repository and is
// therefore public; it must never be accepted again.
const legacyDefault = "demenu-secret-key-change-in-production"

// Generator derives ids from Telegram user IDs.
type Generator struct {
	key []byte
}

// New returns a Generator keyed with secret.
func New(secret string) (*Generator, error) {
	if len(secret) < MinSecretLen {
		return nil, fmt.Errorf("%w (got %d characters)", ErrWeakSecret, len(secret))
	}
	if secret == legacyDefault {
		return nil, ErrWeakSecret
	}
	return &Generator{key: []byte(secret)}, nil
}

// ID returns the public id for userID. It is deterministic for a given secret.
func (g *Generator) ID(userID int64) string {
	mac := hmac.New(sha256.New, g.key)
	mac.Write([]byte(domain))
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(userID)) //nolint:gosec // bit-for-bit reinterpretation is intended
	mac.Write(buf[:])
	return encodeBase62(mac.Sum(nil)[:macBytes])
}

// Valid reports whether s has the shape of an id. It says nothing about
// whether the id belongs to anyone; it exists so that ids can be used safely
// in file names and URL paths.
func Valid(s string) bool {
	if len(s) != Len {
		return false
	}
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(alphabet, s[i]) < 0 {
			return false
		}
	}
	return true
}

func encodeBase62(b []byte) string {
	n := new(big.Int).SetBytes(b)
	base := big.NewInt(int64(len(alphabet)))
	mod := new(big.Int)
	out := make([]byte, Len)
	for i := Len - 1; i >= 0; i-- {
		n.DivMod(n, base, mod)
		out[i] = alphabet[mod.Int64()]
	}
	return string(out)
}
