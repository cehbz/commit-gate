package gatestate

import (
	"regexp"
	"testing"
)

var hexRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestCanonicalProperties(t *testing.T) {
	base := CanonicalHash("subj\n\nbody line")
	if !hexRe.MatchString(base) {
		t.Fatalf("not 64-hex: %q", base)
	}
	same := []string{
		"subj\r\n\r\nbody line",    // CRLF fold
		"subj  \n\t\nbody line\t ", // trailing ws per line
		"subj\n\nbody line\n\n\n",  // trailing blank lines
	}
	for _, m := range same {
		if CanonicalHash(m) != base {
			t.Errorf("hash(%q) != base", m)
		}
	}
	if CanonicalHash("subj\n\nbody line2") == base {
		t.Error("different bodies must differ")
	}
	if CanonicalHash("") == "" {
		t.Error("empty message still hashes")
	}
}
