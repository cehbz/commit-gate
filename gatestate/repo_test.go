package gatestate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	commandConfig(t, "core.hooksPath", dir)
}

// commandConfig runs the test with a command-scope key=value, as git -c
// sets it.
func commandConfig(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", key)
	t.Setenv("GIT_CONFIG_VALUE_0", value)
}

// worktreeConfig sets key=value in the repo's worktree-scope config.
func worktreeConfig(t *testing.T, r *Repo, key, value string) {
	t.Helper()
	for _, args := range [][]string{
		{"config", "extensions.worktreeConfig", "true"},
		{"config", "--worktree", key, value},
	} {
		if _, err := git(r.Dir, args...); err != nil {
			t.Fatal(err)
		}
	}
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

func TestEnableClearsAnOverrideAndAnOptOut(t *testing.T) {
	hooks := t.TempDir()
	globalHooksPath(t, hooks)
	r, _ := Open(mkrepo(t))
	r.GitConfigSet("core.hooksPath", t.TempDir())
	r.GitConfigSet("commit-gate.disabled", "true")
	if err := r.Enable(hooks); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if s := r.State(hooks); s != StateEnabled {
		t.Fatalf("after Enable: %v", s)
	}
	if v, err := git(r.Dir, "config", "--local", "--get", "core.hooksPath"); err == nil {
		t.Fatalf("the repo-local core.hooksPath must be cleared, got %q", v)
	}
}

func TestEnableRefusesWithoutTheGlobalGate(t *testing.T) {
	hooks := t.TempDir()
	globalHooksPath(t, "")
	r, _ := Open(mkrepo(t))
	r.GitConfigSet("core.hooksPath", hooks) // enabled repo by repo
	err := r.Enable(hooks)
	if err == nil || !strings.Contains(err.Error(), "git config --global core.hooksPath "+hooks) {
		t.Fatalf("Enable must refuse, naming the fix: %v", err)
	}
	if v, _ := git(r.Dir, "config", "--local", "--get", "core.hooksPath"); v != hooks {
		t.Fatalf("a refused Enable must change nothing, core.hooksPath=%q", v)
	}
}

// An override Enable can't clear, here one from the command scope, leaves
// the repo ungated: Enable reports it rather than claiming success.
func TestEnableReportsAnOverrideItCannotClear(t *testing.T) {
	hooks := t.TempDir()
	globalHooksPath(t, hooks)
	elsewhere := t.TempDir()
	commandHooksPath(t, elsewhere)
	r, _ := Open(mkrepo(t))
	err := r.Enable(hooks)
	if want := `core.hooksPath is still "` + elsewhere + `", from command config`; err == nil || err.Error() != want {
		t.Fatalf("Enable must name the remaining override:\ngot:  %v\nwant: %s", err, want)
	}
}

func TestEnableQuotesAnEmptyOverride(t *testing.T) {
	hooks := t.TempDir()
	globalHooksPath(t, hooks)
	commandHooksPath(t, "")
	r, _ := Open(mkrepo(t))
	err := r.Enable(hooks)
	if want := `core.hooksPath is still "", from command config`; err == nil || err.Error() != want {
		t.Fatalf("got:  %v\nwant: %s", err, want)
	}
}

func TestEnableNamesTheClearingCommandForAWorktreeOverride(t *testing.T) {
	hooks := t.TempDir()
	globalHooksPath(t, hooks)
	r, _ := Open(mkrepo(t))
	elsewhere := t.TempDir()
	worktreeConfig(t, r, "core.hooksPath", elsewhere)
	err := r.Enable(hooks)
	want := `core.hooksPath is still "` + elsewhere + `", from worktree config; to clear it: git config --worktree --unset core.hooksPath`
	if err == nil || err.Error() != want {
		t.Fatalf("got:  %v\nwant: %s", err, want)
	}
}

// An opt-out Enable can't clear leaves the repo opted out: Enable reports
// it rather than claiming success.
func TestEnableReportsAnOptOutItCannotClear(t *testing.T) {
	hooks := t.TempDir()
	globalHooksPath(t, hooks)
	commandConfig(t, "commit-gate.disabled", "true")
	r, _ := Open(mkrepo(t))
	err := r.Enable(hooks)
	if want := "commit-gate.disabled is still true, from command config"; err == nil || err.Error() != want {
		t.Fatalf("got:  %v\nwant: %s", err, want)
	}
}

func TestEnableNamesTheClearingCommandForAWorktreeOptOut(t *testing.T) {
	hooks := t.TempDir()
	globalHooksPath(t, hooks)
	r, _ := Open(mkrepo(t))
	worktreeConfig(t, r, "commit-gate.disabled", "true")
	err := r.Enable(hooks)
	want := "commit-gate.disabled is still true, from worktree config; to clear it: git config --worktree --unset commit-gate.disabled"
	if err == nil || err.Error() != want {
		t.Fatalf("got:  %v\nwant: %s", err, want)
	}
}

func TestEnableReturnsAFailedRead(t *testing.T) {
	hooks := t.TempDir()
	globalHooksPath(t, hooks)
	r, _ := Open(mkrepo(t))
	commandHooksPath(t, "~nosuchuser-commit-gate/hooks") // git init would fail on it
	err := r.Enable(hooks)
	if err == nil || !strings.Contains(err.Error(), "nosuchuser-commit-gate") {
		t.Fatalf("Enable must return HooksPath's error: %v", err)
	}
}

func TestEnableFailures(t *testing.T) {
	r, _ := Open(mkrepo(t))
	if err := r.Enable(""); err == nil {
		t.Fatal("empty hooks dir must error")
	}
	if s := r.State(""); s != StateUngated {
		t.Fatalf("failed Enable must leave the repo ungated: %v", s)
	}
}
