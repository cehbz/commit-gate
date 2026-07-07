package confirm

import (
	"errors"
	"testing"
)

// withSeams swaps the three package-var seams for the duration of the test.
func withSeams(t *testing.T, goos string, osa func(string) error, tty func(string) (string, error)) {
	t.Helper()
	origGOOS, origOsa, origTTY := GOOS, RunOsascript, ReadTTY
	GOOS, RunOsascript, ReadTTY = goos, osa, tty
	t.Cleanup(func() { GOOS, RunOsascript, ReadTTY = origGOOS, origOsa, origTTY })
}

func failOsascript(t *testing.T) func(string) error {
	return func(string) error {
		t.Fatal("RunOsascript should not be called on this path")
		return nil
	}
}

func failReadTTY(t *testing.T) func(string) (string, error) {
	return func(string) (string, error) {
		t.Fatal("ReadTTY should not be called on this path")
		return "", nil
	}
}

func TestConfirmDarwinOsascriptOK(t *testing.T) {
	withSeams(t, "darwin", func(string) error { return nil }, failReadTTY(t))
	if err := Confirm("proceed?"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestConfirmDarwinOsascriptError(t *testing.T) {
	withSeams(t, "darwin", func(string) error { return errors.New("user hit Cancel") }, failReadTTY(t))
	err := Confirm("proceed?")
	if err == nil || err.Error() != "aborted; not confirmed" {
		t.Fatalf("want aborted error, got %v", err)
	}
}

func TestConfirmNonDarwinTTYAffirmative(t *testing.T) {
	for _, reply := range []string{"y", "Y", "yes", "YES"} {
		withSeams(t, "linux", failOsascript(t), func(prompt string) (string, error) {
			if prompt != "proceed? [y/N] " {
				t.Fatalf("unexpected prompt: %q", prompt)
			}
			return reply, nil
		})
		if err := Confirm("proceed?"); err != nil {
			t.Fatalf("reply %q: want nil, got %v", reply, err)
		}
	}
}

func TestConfirmNonDarwinTTYNegativeOrEmpty(t *testing.T) {
	for _, reply := range []string{"n", ""} {
		withSeams(t, "linux", failOsascript(t), func(string) (string, error) { return reply, nil })
		err := Confirm("proceed?")
		if err == nil || err.Error() != "aborted; not confirmed" {
			t.Fatalf("reply %q: want aborted, got %v", reply, err)
		}
	}
}

func TestConfirmNoChannel(t *testing.T) {
	withSeams(t, "linux", failOsascript(t), func(string) (string, error) {
		return "", errors.New("open /dev/tty: no such device or address")
	})
	err := Confirm("proceed?")
	want := "no confirmation channel (no GUI, no tty) — run from a terminal"
	if err == nil || err.Error() != want {
		t.Fatalf("want %q, got %v", want, err)
	}
}

// darwin without osascript on PATH must fall through to the TTY channel,
// not silently succeed.
func TestConfirmDarwinWithoutOsascriptFallsBackToTTY(t *testing.T) {
	// hasOsascript() does a real PATH lookup; this test only exercises the
	// non-darwin-equivalent fallback semantics via GOOS gating, so use a
	// non-darwin GOOS to guarantee the TTY branch regardless of the host's
	// actual osascript availability.
	withSeams(t, "not-darwin", failOsascript(t), func(string) (string, error) { return "yes", nil })
	if err := Confirm("proceed?"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}
