package gatestate

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var hexRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestCanonicalProperties(t *testing.T) {
	base := CanonicalHash("subj\n\nbody line")
	if !hexRe.MatchString(base) {
		t.Fatalf("not 64-hex: %q", base)
	}
	same := []string{
		"subj\r\n\r\nbody line",         // CRLF fold
		"subj  \n\t\nbody line\t ",       // trailing ws per line
		"subj\n\nbody line\n\n\n",        // trailing blank lines
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

// Cross-check against the bash reference while it is still in-tree (skips after Task 15).
func TestCanonicalMatchesBashReference(t *testing.T) {
	ref := filepath.Join("..", "lib", "canonical")
	if _, err := os.Stat(ref); err != nil {
		t.Skip("bash reference removed")
	}
	cases := []string{
		"one line",
		"subj\n\nbody with  spaces  \nand more\n",
		"crlf\r\nline\r\n",
		"trailing\n\n\n",
		"unicode: héllo — em dash\n",
		"tabs\t\nend\t",
	}
	for _, m := range cases {
		cmd := exec.Command("bash", ref)
		cmd.Stdin = strings.NewReader(m)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("bash canonical: %v", err)
		}
		want := strings.TrimSpace(string(out))
		if got := CanonicalHash(m); got != want {
			t.Errorf("CanonicalHash(%q) = %s, bash says %s", m, got, want)
		}
	}
}
