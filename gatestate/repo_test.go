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

// globalHooksPath runs the test with a global git config whose
// core.hooksPath is dir, or none when dir is "".
func globalHooksPath(t *testing.T, dir string) {
	t.Helper()
	body := ""
	if dir != "" {
		body = "[core]\n\thooksPath = " + dir + "\n"
	}
	f := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", f)
}

func TestStateTransitions(t *testing.T) {
	hooks := t.TempDir() // stands in for the live hooks dir
	globalHooksPath(t, hooks)
	r, _ := Open(mkrepo(t))
	if s := r.State(hooks); s != StateEnabled {
		t.Fatalf("a fresh repo under the global gate: %v", s)
	}
	r.GitConfigSet("commit-gate.disabled", "true")
	if s := r.State(hooks); s != StateDisabled {
		t.Fatalf("opt-out wins: %v", s)
	}
	r.GitConfigUnset("commit-gate.disabled")
	if err := r.GitConfigUnset("commit-gate.disabled"); err != nil {
		t.Fatalf("unset must be idempotent: %v", err)
	}
	elsewhere := t.TempDir()
	r.GitConfigSet("core.hooksPath", elsewhere)
	if s := r.State(hooks); s != StateOverridden {
		t.Fatalf("a repo-local core.hooksPath elsewhere: %v", s)
	}
	if hp, scope, ok, err := r.HooksPath(); err != nil || !ok || hp != elsewhere || scope != "local" {
		t.Fatalf("HooksPath = %q %q %v %v", hp, scope, ok, err)
	}
	r.GitConfigSet("core.hooksPath", hooks)
	if s := r.State(hooks); s != StateEnabled {
		t.Fatalf("a repo-local core.hooksPath naming the gate: %v", s)
	}
	globalHooksPath(t, "")
	if _, err := git(r.Dir, "config", "--local", "--unset", "core.hooksPath"); err != nil {
		t.Fatal(err)
	}
	if s := r.State(hooks); s != StateUngated {
		t.Fatalf("no core.hooksPath anywhere: %v", s)
	}
}

// commandHooksPath runs the test with a command-scope core.hooksPath of dir,
// as git -c core.hooksPath=dir sets it.
func commandHooksPath(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", dir)
}

func TestStateOverriddenAboveGlobal(t *testing.T) {
	hooks := t.TempDir()
	globalHooksPath(t, hooks)
	r, _ := Open(mkrepo(t))
	r.GitConfigSet("core.hooksPath", "")
	if s := r.State(hooks); s != StateOverridden {
		t.Fatalf("an empty repo-local core.hooksPath: %v", s)
	}
	if hp, scope, ok, err := r.HooksPath(); err != nil || !ok || hp != "" || scope != "local" {
		t.Fatalf("empty: HooksPath = %q %q %v %v", hp, scope, ok, err)
	}
	if _, err := git(r.Dir, "config", "--local", "--unset", "core.hooksPath"); err != nil {
		t.Fatal(err)
	}
	elsewhere := t.TempDir()
	commandHooksPath(t, elsewhere)
	if s := r.State(hooks); s != StateOverridden {
		t.Fatalf("a command-scope core.hooksPath elsewhere: %v", s)
	}
	if hp, scope, ok, err := r.HooksPath(); err != nil || !ok || hp != elsewhere || scope != "command" {
		t.Fatalf("command: HooksPath = %q %q %v %v", hp, scope, ok, err)
	}
	commandHooksPath(t, hooks)
	if s := r.State(hooks); s != StateEnabled {
		t.Fatalf("a command-scope core.hooksPath naming the gate: %v", s)
	}
}

func TestHooksPathReadFailure(t *testing.T) {
	hooks := t.TempDir()
	globalHooksPath(t, "")
	r, _ := Open(mkrepo(t))
	if _, _, ok, err := r.HooksPath(); ok || err != nil {
		t.Fatalf("unset is not a failure: ok=%v err=%v", ok, err)
	}
	globalHooksPath(t, hooks)
	r.GitConfigSet("core.hooksPath", "~nosuchuser-commit-gate/hooks")
	if _, _, ok, err := r.HooksPath(); ok || err == nil {
		t.Fatalf("an unexpandable value: ok=%v err=%v", ok, err)
	}
	if s := r.State(hooks); s != StateUngated {
		t.Fatalf("an unreadable core.hooksPath gates nothing: %v", s)
	}
}

func TestGlobalGateProblem(t *testing.T) {
	hooks := t.TempDir()
	globalHooksPath(t, "")
	if p := GlobalGateProblem(hooks); p != "the global core.hooksPath is unset" {
		t.Fatalf("unset: %q", p)
	}
	other := t.TempDir()
	globalHooksPath(t, other)
	if p := GlobalGateProblem(hooks); p != "the global core.hooksPath is "+other+", not the gate" {
		t.Fatalf("foreign: %q", p)
	}
	globalHooksPath(t, hooks)
	if p := GlobalGateProblem(hooks); p != "" {
		t.Fatalf("the gate: %q", p)
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
	if s := r.State(""); s != StateUngated {
		t.Fatalf("failed Enable must leave the repo ungated: %v", s)
	}
	if err := os.WriteFile(r.GateDir(), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.Enable(t.TempDir()); err == nil {
		t.Fatal("gate dir blocked by a file must error")
	}
}
