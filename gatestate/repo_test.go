package gatestate

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func mkrepo(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", append([]string{"-C", d}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return d
}

func TestOpenAndPaths(t *testing.T) {
	d := mkrepo(t)
	r, err := Open(d)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(d, ".git"))
	if r.Common != want {
		t.Fatalf("Common = %q want %q", r.Common, want)
	}
	if r.GateDir() != filepath.Join(want, "commit-gate") ||
		r.ManifestPath() != filepath.Join(want, "commit-gate", "approved") ||
		r.PendingPath() != filepath.Join(want, "cg-pending") ||
		r.PushTokenPath() != filepath.Join(want, "commit-gate", "push-token") {
		t.Fatal("path layout mismatch")
	}
	if _, err := Open(t.TempDir()); err == nil {
		t.Fatal("non-repo must error")
	}
}

func TestStateTransitions(t *testing.T) {
	d := mkrepo(t)
	hooks := t.TempDir() // stands in for the live hooks dir
	r, _ := Open(d)
	if s := r.State(hooks); s != StateUndecided {
		t.Fatalf("fresh repo: %v", s)
	}
	r.GitConfigSet("core.hooksPath", hooks)
	if s := r.State(hooks); s != StateEnabled {
		t.Fatalf("after enable: %v", s)
	}
	r.GitConfigSet("commit-gate.disabled", "true")
	if s := r.State(hooks); s != StateDisabled {
		t.Fatalf("disabled wins: %v", s)
	}
	r.GitConfigUnset("commit-gate.disabled")
	if err := r.GitConfigUnset("commit-gate.disabled"); err != nil {
		t.Fatalf("unset must be idempotent: %v", err)
	}
	if s := r.State(hooks); s != StateEnabled {
		t.Fatalf("after re-unset: %v", s)
	}
	// hooksPath somewhere else = undecided
	r.GitConfigSet("core.hooksPath", t.TempDir())
	if s := r.State(hooks); s != StateUndecided {
		t.Fatalf("foreign hooksPath: %v", s)
	}
}

func TestHeadMessage(t *testing.T) {
	d := mkrepo(t)
	r, _ := Open(d)
	if _, ok := r.HeadMessage(); ok {
		t.Fatal("no HEAD yet")
	}
	os.WriteFile(filepath.Join(d, "f"), []byte("x"), 0o644)
	exec.Command("git", "-C", d, "add", "f").Run()
	exec.Command("git", "-C", d, "commit", "-q", "-m", "subj\n\nbody").Run()
	msg, ok := r.HeadMessage()
	if !ok || CanonicalHash(msg) != CanonicalHash("subj\n\nbody") {
		t.Fatalf("head message: %q %v", msg, ok)
	}
}

func TestEnable(t *testing.T) {
	d := mkrepo(t)
	hooks := t.TempDir()
	r, _ := Open(d)
	r.GitConfigSet("commit-gate.disabled", "true")
	if err := r.Enable(hooks); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if s := r.State(hooks); s != StateEnabled {
		t.Fatalf("after Enable: %v", s)
	}
	if info, err := os.Stat(r.GateDir()); err != nil || !info.IsDir() {
		t.Fatalf("gate dir must exist: %v", err)
	}
	if err := os.WriteFile(r.ManifestPath(), []byte("abc  feat: keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.Enable(hooks); err != nil {
		t.Fatalf("re-Enable: %v", err)
	}
	if b, _ := os.ReadFile(r.ManifestPath()); string(b) != "abc  feat: keep\n" {
		t.Fatalf("re-Enable must keep the manifest, got %q", b)
	}
}

func TestEnableFailures(t *testing.T) {
	d := mkrepo(t)
	r, _ := Open(d)
	if err := r.Enable(""); err == nil {
		t.Fatal("empty hooks dir must error")
	}
	if s := r.State(""); s != StateUndecided {
		t.Fatalf("failed Enable must leave the repo undecided: %v", s)
	}
	if err := os.WriteFile(r.GateDir(), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.Enable(t.TempDir()); err == nil {
		t.Fatal("gate dir blocked by a file must error")
	}
}
