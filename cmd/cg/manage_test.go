// manage_test.go covers the five gate-management commands: approve-push,
// enable, disable, status and clear-approvals. Tests never omit
// --yes on commands that would otherwise invoke confirm.Confirm, since that
// would pop a real OS dialog / block on a tty in CI.
package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cehbz/commit-gate/gatestate"
)

// gitGetConfig returns the raw value of a git config key, or "" if unset.
func gitGetConfig(t *testing.T, repo, key string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "config", "--get", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

func gitSetConfig(t *testing.T, repo string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", repo, "config"}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git config %v: %v %s", args, err, out)
	}
}

func showToplevel(t *testing.T, repo string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("rev-parse --show-toplevel: %v", err)
	}
	return strings.TrimRight(string(out), "\n")
}

// --- commit-gate enable ---

func TestEnableInANeverEnabledRepoSetsNothingLocal(t *testing.T) {
	repo := mkrepoT(t)
	top := showToplevel(t, repo)
	out, errStr, code := runIn(t, repo, "", "commit-gate", "enable")
	if code != 0 || out != "commit-gate: enabled in "+top+"\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errStr)
	}
	if hp := localHooksPath(t, repo); hp != "" {
		t.Fatalf("the global core.hooksPath gates it; no repo-local one may be set: %q", hp)
	}
}

func TestEnableClearsAnOverride(t *testing.T) {
	repo := mkrepoT(t)
	gitSetConfig(t, repo, "core.hooksPath", t.TempDir())
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "enable"); code != 0 {
		t.Fatalf("commit-gate enable: code=%d stderr=%q", code, errStr)
	}
	if hp := localHooksPath(t, repo); hp != "" {
		t.Fatalf("the override must be cleared: %q", hp)
	}
	if out, _, _ := runIn(t, repo, "", "commit-gate", "status"); !strings.HasPrefix(out, "commit-gate: enabled\n") {
		t.Fatalf("commit-gate status: %q", out)
	}
}

// An empty repo-local core.hooksPath is an override (it turns hooks off), so
// commit-gate enable removes it like any other value.
func TestEnableClearsAnEmptyOverride(t *testing.T) {
	repo := mkrepoT(t)
	gitSetConfig(t, repo, "core.hooksPath", "")
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "enable"); code != 0 {
		t.Fatalf("commit-gate enable: code=%d stderr=%q", code, errStr)
	}
	if out, err := exec.Command("git", "-C", repo, "config", "--local", "--get-all", "core.hooksPath").Output(); err == nil {
		t.Fatalf("the empty override must be removed, not replaced: %q", out)
	}
	if out, _, _ := runIn(t, repo, "", "commit-gate", "status"); !strings.HasPrefix(out, "commit-gate: enabled\n") {
		t.Fatalf("commit-gate status: %q", out)
	}
}

func TestEnableClearsAnOptOut(t *testing.T) {
	repo := mkrepoT(t)
	gitSetConfig(t, repo, "commit-gate.disabled", "true")
	for i := 0; i < 2; i++ { // the second run finds nothing to clear
		if _, errStr, code := runIn(t, repo, "", "commit-gate", "enable"); code != 0 {
			t.Fatalf("commit-gate enable #%d: code=%d stderr=%q", i+1, code, errStr)
		}
	}
	if v := gitGetConfig(t, repo, "commit-gate.disabled"); v != "" {
		t.Fatalf("commit-gate.disabled must be unset, got %q", v)
	}
	if hp := localHooksPath(t, repo); hp != "" {
		t.Fatalf("no repo-local core.hooksPath may be set: %q", hp)
	}
}

// An opt-out from a scope commit-gate enable doesn't clear leaves the repo opted
// out, so commit-gate enable must not report it enabled.
func TestEnableReportsAnOptOutItCannotClear(t *testing.T) {
	repo := mkrepoT(t)
	t.Setenv("GIT_CONFIG_COUNT", "1") // git -c commit-gate.disabled=true
	t.Setenv("GIT_CONFIG_KEY_0", "commit-gate.disabled")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	out, errStr, code := runIn(t, repo, "", "commit-gate", "enable")
	want := "commit-gate: commit-gate.disabled is still true, from command config\n"
	if code != 1 || errStr != want {
		t.Fatalf("code=%d stdout=%q\ngot:  %q\nwant: %q", code, out, errStr, want)
	}
}

func TestEnableRefusesWithoutTheGlobalGate(t *testing.T) {
	noGlobalGate(t)
	repo := mkrepoT(t)
	gitSetConfig(t, repo, "core.hooksPath", liveHooks(t)) // enabled repo by repo
	_, errStr, code := runIn(t, repo, "", "commit-gate", "enable")
	want := "commit-gate: the global core.hooksPath is unset; to gate every repo: git config --global core.hooksPath " + liveHooks(t) + "\n"
	if code != 1 || errStr != want {
		t.Fatalf("code=%d\ngot:  %q\nwant: %q", code, errStr, want)
	}
	if hp := localHooksPath(t, repo); hp != liveHooks(t) {
		t.Fatalf("a refused commit-gate enable must change nothing: %q", hp)
	}
}

func TestEnableNonRepoDies(t *testing.T) {
	dir := t.TempDir()
	_, errStr, code := runIn(t, dir, "", "commit-gate", "enable")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: not inside a git repository\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}

// --- commit-gate disable ---

func TestDisableHappyPath(t *testing.T) {
	repo := mkrepoT(t)
	top := showToplevel(t, repo)

	out, errStr, code := runIn(t, repo, "", "commit-gate", "disable", "--yes")
	if code != 0 {
		t.Fatalf("commit-gate disable failed: code=%d stderr=%q", code, errStr)
	}
	want := "commit-gate: disabled (opted out) for " + top + "\n"
	if out != want {
		t.Fatalf("stdout mismatch:\ngot:  %q\nwant: %q", out, want)
	}
	if got := gitGetConfig(t, repo, "commit-gate.disabled"); got != "true" {
		t.Fatalf("commit-gate.disabled = %q, want true", got)
	}
}

func TestDisableNonRepoDies(t *testing.T) {
	dir := t.TempDir()
	_, errStr, code := runIn(t, dir, "", "commit-gate", "disable", "--yes")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: not inside a git repository\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}

// --- commit-gate status ---

// liveHooks is the hook directory the test binary resolves as its own.
func liveHooks(t *testing.T) string {
	t.Helper()
	h, err := filepath.EvalSymlinks(filepath.Join(binDir, "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestStatusUngatedNamesTheFix(t *testing.T) {
	noGlobalGate(t)
	repo := mkrepoT(t)
	out, errStr, code := runIn(t, repo, "", "commit-gate", "status")
	want := "commit-gate: ungated (the global core.hooksPath is unset; to gate every repo: git config --global core.hooksPath " + liveHooks(t) + ")\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d stderr=%q\ngot:  %q\nwant: %q", code, errStr, out, want)
	}
}

func TestStatusOverriddenNamesTheDirectory(t *testing.T) {
	repo := mkrepoT(t)
	elsewhere := t.TempDir()
	gitSetConfig(t, repo, "core.hooksPath", elsewhere)
	out, errStr, code := runIn(t, repo, "", "commit-gate", "status")
	want := "commit-gate: overridden (core.hooksPath is " + elsewhere + "; commit-gate enable clears it, commit-gate disable opts out)\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d stderr=%q\ngot:  %q\nwant: %q", code, errStr, out, want)
	}
}

func TestStatusEmptyLocalHooksPathIsOverridden(t *testing.T) {
	repo := mkrepoT(t)
	gitSetConfig(t, repo, "core.hooksPath", "")
	out, errStr, code := runIn(t, repo, "", "commit-gate", "status")
	want := "commit-gate: overridden (core.hooksPath is empty; commit-gate enable clears it, commit-gate disable opts out)\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d stderr=%q\ngot:  %q\nwant: %q", code, errStr, out, want)
	}
}

func TestStatusCommandScopeIsOverridden(t *testing.T) {
	repo := mkrepoT(t)
	elsewhere := t.TempDir()
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", elsewhere)
	out, errStr, code := runIn(t, repo, "", "commit-gate", "status")
	want := "commit-gate: overridden (core.hooksPath is " + elsewhere + ", from command config: git -c or GIT_CONFIG_* in the environment; removing it from the environment clears it, commit-gate disable opts out)\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d stderr=%q\ngot:  %q\nwant: %q", code, errStr, out, want)
	}
}

// A worktree-scope override names the command that clears it, as
// sessioncheck does: commit-gate enable clears only a repo-local one.
func TestStatusWorktreeScopeNamesItsRemedy(t *testing.T) {
	repo := mkrepoT(t)
	elsewhere := t.TempDir()
	gitSetConfig(t, repo, "extensions.worktreeConfig", "true")
	gitSetConfig(t, repo, "--worktree", "core.hooksPath", elsewhere)
	out, errStr, code := runIn(t, repo, "", "commit-gate", "status")
	want := "commit-gate: overridden (core.hooksPath is " + elsewhere + ", from worktree config; git config --worktree --unset core.hooksPath clears it, commit-gate disable opts out)\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d stderr=%q\ngot:  %q\nwant: %q", code, errStr, out, want)
	}
}

func TestStatusUnreadableHooksPathSaysWhy(t *testing.T) {
	repo := mkrepoT(t)
	gitSetConfig(t, repo, "core.hooksPath", "~nosuchuser-commit-gate/hooks")
	out, errStr, code := runIn(t, repo, "", "commit-gate", "status")
	if code != 1 || out != "" ||
		!strings.HasPrefix(errStr, "commit-gate: cannot read core.hooksPath: ") ||
		!strings.Contains(errStr, "~nosuchuser-commit-gate/hooks") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errStr)
	}
}

func TestStatusDisabled(t *testing.T) {
	repo := mkrepoT(t)
	gitSetConfig(t, repo, "commit-gate.disabled", "true")
	out, errStr, code := runIn(t, repo, "", "commit-gate", "status")
	if code != 0 {
		t.Fatalf("commit-gate status failed: code=%d stderr=%q", code, errStr)
	}
	if out != "commit-gate: opted-out\n" {
		t.Fatalf("stdout mismatch: %q", out)
	}
}

func TestStatusEnabledNoneAndNoToken(t *testing.T) {
	repo := mkrepoT(t)
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "enable"); code != 0 {
		t.Fatalf("commit-gate enable setup failed: %s", errStr)
	}
	out, errStr, code := runIn(t, repo, "", "commit-gate", "status")
	if code != 0 {
		t.Fatalf("commit-gate status failed: code=%d stderr=%q", code, errStr)
	}
	want := "commit-gate: enabled\napproved (pending): none\npush token: none\ndeploy token: none\n"
	if out != want {
		t.Fatalf("stdout mismatch:\ngot:  %q\nwant: %q", out, want)
	}
}

func TestStatusEnabledWithApprovalsAndToken(t *testing.T) {
	repo := mkrepoT(t)
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "enable"); code != 0 {
		t.Fatalf("commit-gate enable setup failed: %s", errStr)
	}
	r, err := gatestate.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := gatestate.AppendApprovals(r.ManifestPath(), []string{"feat: pending one"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.PushTokenPath(), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.DeployTokenPath(), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errStr, code := runIn(t, repo, "", "commit-gate", "status")
	if code != 0 {
		t.Fatalf("commit-gate status failed: code=%d stderr=%q", code, errStr)
	}
	want := "commit-gate: enabled\napproved (pending):\n  feat: pending one\npush token: present\ndeploy token: present\n"
	if out != want {
		t.Fatalf("stdout mismatch:\ngot:  %q\nwant: %q", out, want)
	}
}

func TestStatusNonRepoDies(t *testing.T) {
	dir := t.TempDir()
	_, errStr, code := runIn(t, dir, "", "commit-gate", "status")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: not inside a git repository\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}

// --- clear-approvals ---

func TestClearApprovalsHappyPath(t *testing.T) {
	repo := mkrepoT(t)
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "enable"); code != 0 {
		t.Fatalf("commit-gate enable setup failed: %s", errStr)
	}
	r, err := gatestate.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := gatestate.AppendApprovals(r.ManifestPath(), []string{"feat: to-clear"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.PushTokenPath(), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.DeployTokenPath(), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errStr, code := runIn(t, repo, "", "commit-gate", "clear-approvals")
	if code != 0 {
		t.Fatalf("clear-approvals failed: code=%d stderr=%q", code, errStr)
	}
	if out != "commit-gate: approvals, push token and deploy token cleared\n" {
		t.Fatalf("stdout mismatch: %q", out)
	}

	manifest, err := os.ReadFile(r.ManifestPath())
	if err != nil {
		t.Fatalf("manifest must still exist: %v", err)
	}
	if len(manifest) != 0 {
		t.Fatalf("manifest must be empty, got %q", manifest)
	}
	if _, err := os.Stat(r.PushTokenPath()); !os.IsNotExist(err) {
		t.Fatalf("push token must be removed")
	}
	if _, err := os.Stat(r.DeployTokenPath()); !os.IsNotExist(err) {
		t.Fatalf("deploy token must be removed")
	}
}

func TestClearApprovalsNotEnabledDies(t *testing.T) {
	noGlobalGate(t)
	repo := mkrepoT(t)
	_, errStr, code := runIn(t, repo, "", "commit-gate", "clear-approvals")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: repo not enabled\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}

func TestClearApprovalsNonRepoDies(t *testing.T) {
	dir := t.TempDir()
	_, errStr, code := runIn(t, dir, "", "commit-gate", "clear-approvals")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: not inside a git repository\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}

// --- approve-push ---

func TestApprovePushHappyPath(t *testing.T) {
	repo := mkrepoT(t)
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "enable"); code != 0 {
		t.Fatalf("commit-gate enable setup failed: %s", errStr)
	}

	out, errStr, code := runIn(t, repo, "", "commit-gate", "approve-push", "--yes")
	if code != 0 {
		t.Fatalf("approve-push failed: code=%d stderr=%q", code, errStr)
	}
	if out != "commit-gate: one push authorized\n" {
		t.Fatalf("stdout mismatch: %q", out)
	}

	r, err := gatestate.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.PushTokenPath()); err != nil {
		t.Fatalf("push token must exist: %v", err)
	}
}

func TestApprovePushNotEnabledDies(t *testing.T) {
	noGlobalGate(t)
	repo := mkrepoT(t)
	_, errStr, code := runIn(t, repo, "", "commit-gate", "approve-push", "--yes")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: repo not enabled — run: commit-gate enable\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}

func TestApprovePushDisabledRepoDies(t *testing.T) {
	repo := mkrepoT(t)
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "enable"); code != 0 {
		t.Fatalf("commit-gate enable setup failed: %s", errStr)
	}
	gitSetConfig(t, repo, "commit-gate.disabled", "true")

	_, errStr, code := runIn(t, repo, "", "commit-gate", "approve-push", "--yes")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: repo is opted out (commit-gate.disabled); run: commit-gate enable\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}

func TestApprovePushNonRepoDies(t *testing.T) {
	dir := t.TempDir()
	_, errStr, code := runIn(t, dir, "", "commit-gate", "approve-push", "--yes")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: not inside a git repository\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}
