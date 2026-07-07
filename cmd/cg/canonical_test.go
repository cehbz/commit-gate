package main_test

import (
	"testing"
)

// TestCanonicalSubcommand mirrors bash lib/canonical: stdin message in,
// 64-hex sha256 (+ trailing newline) out, exit 0.
func TestCanonicalSubcommand(t *testing.T) {
	out, errStr, code := run(t, "feat: hello", "commit-gate", "canonical")
	if code != 0 {
		t.Fatalf("canonical failed: code=%d stderr=%q", code, errStr)
	}
	want := "93d453a2465815abe2767283e34931387395497416d2e016a28d19ba676361ff\n"
	if out != want {
		t.Fatalf("canonical output mismatch:\ngot:  %q\nwant: %q", out, want)
	}
}
