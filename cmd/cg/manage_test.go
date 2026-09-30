// manage_test.go covers the five gate-management commands: approve-push,
// gate-enable, gate-disable, gate-status, clear-approvals. Behavioral
// contract = reference files bin/approve-push, bin/gate-enable,
// bin/gate-disable, bin/gate-status, bin/clear-approvals. Tests never omit
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

// --- gate-enable ---

func TestGateEnableHappyPath(t *testing.T) {
	repo := mkrepoT(t)
	top := showToplevel(t, repo)

	out, errStr, code := runIn(t, repo, "", "commit-gate", "gate-enable")
	if code != 0 {
		t.Fatalf("gate-enable failed: code=%d stdout=%q stderr=%q", code, out, errStr)
	}
	want := "commit-gate: enabled in " + top + "\n"
	if out != want {
		t.Fatalf("stdout mismatch:\ngot:  %q\nwant: %q", out, want)
	}

	wantHooks, err := filepath.EvalSymlinks(filepath.Join(binDir, "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	got := gitGetConfig(t, repo, "core.hooksPath")
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("core.hooksPath %q does not resolve: %v", got, err)
	}
	if gotResolved != wantHooks {
		t.Fatalf("core.hooksPath = %q (resolved %q), want %q", got, gotResolved, wantHooks)
	}

	r, err := gatestate.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(r.GateDir()); err != nil || !info.IsDir() {
		t.Fatalf("gate dir must exist: %v", err)
	}
	if _, err := os.Stat(r.ManifestPath()); err != nil {
		t.Fatalf("manifest must exist: %v", err)
	}
}

func TestGateEnableUnsetsDisabledIdempotently(t *testing.T) {
	repo := mkrepoT(t)
	gitSetConfig(t, repo, "commit-gate.disabled", "true")

	_, errStr, code := runIn(t, repo, "", "commit-gate", "gate-enable")
	if code != 0 {
		t.Fatalf("gate-enable failed: code=%d stderr=%q", code, errStr)
	}
	if got := gitGetConfig(t, repo, "commit-gate.disabled"); got != "" {
		t.Fatalf("commit-gate.disabled must be unset, got %q", got)
	}

	// Running again with no commit-gate.disabled present must still succeed
	// (idempotent unset).
	_, errStr2, code2 := runIn(t, repo, "", "commit-gate", "gate-enable")
	if code2 != 0 {
		t.Fatalf("second gate-enable failed: code=%d stderr=%q", code2, errStr2)
	}
}

func TestGateEnableNonRepoDies(t *testing.T) {
	dir := t.TempDir()
	_, errStr, code := runIn(t, dir, "", "commit-gate", "gate-enable")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: not inside a git repository\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}

// --- gate-disable ---

func TestGateDisableHappyPath(t *testing.T) {
	repo := mkrepoT(t)
	top := showToplevel(t, repo)

	out, errStr, code := runIn(t, repo, "", "commit-gate", "gate-disable", "--yes")
	if code != 0 {
		t.Fatalf("gate-disable failed: code=%d stderr=%q", code, errStr)
	}
	want := "commit-gate: disabled (opted out) for " + top + "\n"
	if out != want {
		t.Fatalf("stdout mismatch:\ngot:  %q\nwant: %q", out, want)
	}
	if got := gitGetConfig(t, repo, "commit-gate.disabled"); got != "true" {
		t.Fatalf("commit-gate.disabled = %q, want true", got)
	}
}

func TestGateDisableNonRepoDies(t *testing.T) {
	dir := t.TempDir()
	_, errStr, code := runIn(t, dir, "", "commit-gate", "gate-disable", "--yes")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: not inside a git repository\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}

// --- gate-status ---

func TestGateStatusUndecided(t *testing.T) {
	repo := mkrepoT(t)
	out, errStr, code := runIn(t, repo, "", "commit-gate", "gate-status")
	if code != 0 {
		t.Fatalf("gate-status failed: code=%d stderr=%q", code, errStr)
	}
	want := "commit-gate: undecided (gated at the next Claude Code session start unless opted out: gate-disable)\n"
	if out != want {
		t.Fatalf("stdout mismatch:\ngot:  %q\nwant: %q", out, want)
	}
}

func TestGateStatusUndecidedForeignHooksPath(t *testing.T) {
	repo := mkrepoT(t)
	gitSetConfig(t, repo, "core.hooksPath", ".husky/_")
	out, errStr, code := runIn(t, repo, "", "commit-gate", "gate-status")
	if code != 0 {
		t.Fatalf("gate-status failed: code=%d stderr=%q", code, errStr)
	}
	want := "commit-gate: undecided (core.hooksPath is .husky/_, so not gated automatically; gate-enable replaces it, gate-disable opts out)\n"
	if out != want {
		t.Fatalf("stdout mismatch:\ngot:  %q\nwant: %q", out, want)
	}
}

func TestGateStatusDisabled(t *testing.T) {
	repo := mkrepoT(t)
	gitSetConfig(t, repo, "commit-gate.disabled", "true")
	out, errStr, code := runIn(t, repo, "", "commit-gate", "gate-status")
	if code != 0 {
		t.Fatalf("gate-status failed: code=%d stderr=%q", code, errStr)
	}
	if out != "commit-gate: opted-out\n" {
		t.Fatalf("stdout mismatch: %q", out)
	}
}

func TestGateStatusEnabledNoneAndNoToken(t *testing.T) {
	repo := mkrepoT(t)
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "gate-enable"); code != 0 {
		t.Fatalf("gate-enable setup failed: %s", errStr)
	}
	out, errStr, code := runIn(t, repo, "", "commit-gate", "gate-status")
	if code != 0 {
		t.Fatalf("gate-status failed: code=%d stderr=%q", code, errStr)
	}
	want := "commit-gate: enabled\napproved (pending): none\npush token: none\n"
	if out != want {
		t.Fatalf("stdout mismatch:\ngot:  %q\nwant: %q", out, want)
	}
}

func TestGateStatusEnabledWithApprovalsAndToken(t *testing.T) {
	repo := mkrepoT(t)
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "gate-enable"); code != 0 {
		t.Fatalf("gate-enable setup failed: %s", errStr)
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

	out, errStr, code := runIn(t, repo, "", "commit-gate", "gate-status")
	if code != 0 {
		t.Fatalf("gate-status failed: code=%d stderr=%q", code, errStr)
	}
	want := "commit-gate: enabled\napproved (pending):\n  feat: pending one\npush token: present\n"
	if out != want {
		t.Fatalf("stdout mismatch:\ngot:  %q\nwant: %q", out, want)
	}
}

func TestGateStatusNonRepoDies(t *testing.T) {
	dir := t.TempDir()
	_, errStr, code := runIn(t, dir, "", "commit-gate", "gate-status")
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
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "gate-enable"); code != 0 {
		t.Fatalf("gate-enable setup failed: %s", errStr)
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

	out, errStr, code := runIn(t, repo, "", "commit-gate", "clear-approvals")
	if code != 0 {
		t.Fatalf("clear-approvals failed: code=%d stderr=%q", code, errStr)
	}
	if out != "commit-gate: approvals and push token cleared\n" {
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
}

func TestClearApprovalsNotEnabledDies(t *testing.T) {
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
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "gate-enable"); code != 0 {
		t.Fatalf("gate-enable setup failed: %s", errStr)
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
	repo := mkrepoT(t)
	_, errStr, code := runIn(t, repo, "", "commit-gate", "approve-push", "--yes")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: repo not enabled — run: gate-enable\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}

func TestApprovePushDisabledRepoDies(t *testing.T) {
	repo := mkrepoT(t)
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "gate-enable"); code != 0 {
		t.Fatalf("gate-enable setup failed: %s", errStr)
	}
	gitSetConfig(t, repo, "commit-gate.disabled", "true")

	_, errStr, code := runIn(t, repo, "", "commit-gate", "approve-push", "--yes")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: repo is opted out (commit-gate.disabled); run: gate-enable\n"
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
