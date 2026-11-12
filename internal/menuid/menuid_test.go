package menuid

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

const (
	secretA = "0123456789abcdef0123456789abcdef"
	secretB = "fedcba9876543210fedcba9876543210"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{"empty", "", true},
		{"short", "short-secret", true},
		{"published legacy default", legacyDefault, true},
		{"31 chars", strings.Repeat("a", 31), true},
		{"32 chars", strings.Repeat("a", 32), false},
		{"long", strings.Repeat("a", 128), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.secret)
			if (err != nil) != tt.wantErr {
				t.Fatalf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrWeakSecret) {
				t.Fatalf("New() error = %v, want ErrWeakSecret", err)
			}
		})
	}
}

func TestID(t *testing.T) {
	a := mustNew(t, secretA)
	b := mustNew(t, secretB)

	tests := []struct {
		name   string
		userID int64
	}{
		{"zero", 0},
		{"one", 1},
		{"typical", 1532825339},
		{"large", 1<<52 - 1},
		{"max", 1<<63 - 1},
		{"negative (group-style id)", -1001234567890},
	}
	seen := map[string]string{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := a.ID(tt.userID)
			if !Valid(id) {
				t.Fatalf("ID(%d) = %q is not a valid id", tt.userID, id)
			}
			if again := a.ID(tt.userID); again != id {
				t.Errorf("not deterministic: %q then %q", id, again)
			}
			if other := b.ID(tt.userID); other == id {
				t.Errorf("id does not depend on the secret: %q", id)
			}
			if prev, dup := seen[id]; dup {
				t.Errorf("collision between %s and %s", prev, tt.name)
			}
			seen[id] = tt.name
		})
	}
}

// The previous scheme put base62(userID) in the first 7 characters. Guard
// against any encoding of the user ID leaking into the id.
func TestIDDoesNotEmbedUserID(t *testing.T) {
	g := mustNew(t, secretA)
	for _, userID := range []int64{7, 123456789, 1532825339, 9876543210} {
		id := g.ID(userID)
		for _, enc := range []string{
			strconv.FormatInt(userID, 10),
			strconv.FormatInt(userID, 16),
			strconv.FormatInt(userID, 36),
			legacyBase62(uint64(userID)),
		} {
			if len(enc) >= 4 && strings.Contains(strings.ToLower(id), strings.ToLower(enc)) {
				t.Errorf("ID(%d) = %q contains encoding %q of the user id", userID, id, enc)
			}
		}
	}
}

// Neighbouring user IDs must not produce related ids.
func TestIDAvalanche(t *testing.T) {
	g := mustNew(t, secretA)
	x, y := g.ID(1000), g.ID(1001)
	same := 0
	for i := range x {
		if x[i] == y[i] {
			same++
		}
	}
	if same > Len/2 {
		t.Errorf("ids for adjacent users share %d/%d positions: %q %q", same, Len, x, y)
	}
}

func TestValid(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{strings.Repeat("a", Len), true},
		{"0Aa1Bb2Cc3Dd4Ee5F", true},
		{"", false},
		{strings.Repeat("a", Len-1), false},
		{strings.Repeat("a", Len+1), false},
		{"../../etc/passwd0", false},
		{"aaaaaaaa/aaaaaaaa", false},
		{"aaaaaaaa.aaaaaaaa", false},
		{"aaaaaaaa aaaaaaaa", false},
		{"aaaaaaaa\x00aaaaaaaa", false},
		{"ááááááááá", false},
	}
	for _, tt := range tests {
		if got := Valid(tt.in); got != tt.want {
			t.Errorf("Valid(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func mustNew(t *testing.T, secret string) *Generator {
	t.Helper()
	g, err := New(secret)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func legacyBase62(n uint64) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{alphabet[n%62]}, out...)
		n /= 62
	}
	return string(out)
}
