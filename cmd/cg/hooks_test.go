package main_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cehbz/commit-gate/gatestate"
)

// runIn is like run() in main_test.go but runs the named program (relative
// to binDir, e.g. "hooks/commit-msg") with cwd set to dir, so gatestate.Open
// (which uses os.Getwd()) resolves against the target repo.
func runIn(t *testing.T, dir, stdin, prog string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(filepath.Join(binDir, prog), args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), errb.String(), code
}

func setHooksPath(t *testing.T, repo string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", repo, "config", "core.hooksPath", filepath.Join(binDir, "hooks")).CombinedOutput(); err != nil {
		t.Fatalf("git config core.hooksPath: %v %s", err, out)
	}
}

func writeAndAdd(t *testing.T, repo, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "add", name).CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, out)
	}
}

// gitCommit runs `git -C repo commit <args...>` and captures stdout/stderr/exit separately.
func gitCommit(t *testing.T, repo string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo, "commit"}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), errb.String(), code
}

// (a) commit rejected: stderr carries the exact block, and cg-pending is
// staged with the exact bytes git handed the hook.
func TestCommitMsgRejectsUnapprovedAndStagesPending(t *testing.T) {
	repo := mkrepoT(t)
	setHooksPath(t, repo)
	writeAndAdd(t, repo, "f", "x")

	_, errStr, code := gitCommit(t, repo, "-m", "feat: hello\n\nbody line\n")
	if code == 0 {
		t.Fatalf("expected commit to be rejected, got exit 0")
	}

	editMsg, err := os.ReadFile(filepath.Join(repo, ".git", "COMMIT_EDITMSG"))
	if err != nil {
		t.Fatalf("COMMIT_EDITMSG: %v", err)
	}
	hash := gatestate.CanonicalHash(string(editMsg))
	subject := gatestate.Subject(string(editMsg))
	want := "commit-gate: commit message not approved.\n" +
		"  subject: " + subject + "\n" +
		"  sha256 : " + hash + "\n" +
		"Have the user run:  approve   (with this exact message)\n" +
		"Human override:     git commit --no-verify\n"
	if !strings.Contains(errStr, want) {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant contains: %q", errStr, want)
	}

	pending, err := os.ReadFile(filepath.Join(repo, ".git", "cg-pending"))
	if err != nil {
		t.Fatalf("cg-pending not staged: %v", err)
	}
	if !bytes.Equal(pending, editMsg) {
		t.Fatalf("cg-pending bytes mismatch:\ngot:  %q\nwant: %q", pending, editMsg)
	}
}

// (b) approved commit consumes the manifest entry and succeeds; (c) amending
// HEAD with the identical message succeeds afterward with no approval left.
func TestCommitMsgApprovedConsumesManifestThenAmendSucceeds(t *testing.T) {
	repo := mkrepoT(t)
	setHooksPath(t, repo)
	writeAndAdd(t, repo, "f", "x")

	r, err := gatestate.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	msg := "feat: approved change\n\nbody\n"
	if err := gatestate.AppendApprovals(r.ManifestPath(), []string{msg}); err != nil {
		t.Fatal(err)
	}

	_, errStr, code := gitCommit(t, repo, "-m", msg)
	if code != 0 {
		t.Fatalf("expected approved commit to succeed: code=%d stderr=%s", code, errStr)
	}
	subj, err := gatestate.ApprovedSubjects(r.ManifestPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(subj) != 0 {
		t.Fatalf("manifest entry must be consumed, got %v", subj)
	}
	if _, err := os.Stat(r.PendingPath()); !os.IsNotExist(err) {
		t.Fatalf("cg-pending must be removed after approved commit")
	}

	// (c) amend with the same message: no approval available, must still succeed
	// because HeadMessage() now equals the proposed message.
	_, errStr2, code2 := gitCommit(t, repo, "--amend", "-m", msg)
	if code2 != 0 {
		t.Fatalf("expected amend with identical message to succeed: code=%d stderr=%s", code2, errStr2)
	}
}

// (d) a repo-local (native) commit-msg hook that exits 7 propagates exit 7,
// and the gate logic never runs (manifest untouched).
func TestCommitMsgLocalHookPropagatesExitCodeAndSkipsGate(t *testing.T) {
	repo := mkrepoT(t)

	hooksDir := filepath.Join(repo, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(hooksDir, "commit-msg")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	r, err := gatestate.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := gatestate.AppendApprovals(r.ManifestPath(), []string{"feat: preexisting\n"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(r.ManifestPath())
	if err != nil {
		t.Fatal(err)
	}

	msgFile := filepath.Join(t.TempDir(), "MSG")
	if err := os.WriteFile(msgFile, []byte("whatever\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, code := runIn(t, repo, "", "hooks/commit-msg", msgFile)
	if code != 7 {
		t.Fatalf("want propagated exit 7, got %d", code)
	}

	after, err := os.ReadFile(r.ManifestPath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("gate must never run when local hook fails: before=%q after=%q", before, after)
	}
}

// (e) pre-push blocks without a token and consumes one when present, invoking
// the hooks/pre-push symlink directly with fake remote args/stdin.
func TestPrePushBlocksWithoutTokenAndConsumesOne(t *testing.T) {
	repo := mkrepoT(t)
	r, err := gatestate.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	fakeStdin := "refs/heads/main aaaa000 refs/heads/main bbbb111\n"

	_, errStr, code := runIn(t, repo, fakeStdin, "hooks/pre-push", "origin", "git@example.com:x/y.git")
	if code != 1 {
		t.Fatalf("want exit 1 without token, got %d (stderr=%q)", code, errStr)
	}
	want := "commit-gate: push not approved.\n" +
		"Have the user run:  approve-push\n" +
		"Human override:     git push --no-verify\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}

	if err := os.MkdirAll(r.GateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.PushTokenPath(), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errStr2, code2 := runIn(t, repo, fakeStdin, "hooks/pre-push", "origin", "git@example.com:x/y.git")
	if code2 != 0 {
		t.Fatalf("want exit 0 with token present, got %d (stderr=%q)", code2, errStr2)
	}
	if _, err := os.Stat(r.PushTokenPath()); !os.IsNotExist(err) {
		t.Fatalf("push-token must be consumed (removed)")
	}
}

// Non-repo directories must exit 0 silently for both hooks.
func TestHooksNonRepoExit0(t *testing.T) {
	dir := t.TempDir()
	msgFile := filepath.Join(t.TempDir(), "MSG")
	os.WriteFile(msgFile, []byte("x\n"), 0o644)

	_, errStr, code := runIn(t, dir, "", "hooks/commit-msg", msgFile)
	if code != 0 || errStr != "" {
		t.Fatalf("commit-msg outside a repo must be silent exit 0: code=%d stderr=%q", code, errStr)
	}
	_, errStr, code = runIn(t, dir, "refs\n", "hooks/pre-push", "origin", "url")
	if code != 0 || errStr != "" {
		t.Fatalf("pre-push outside a repo must be silent exit 0: code=%d stderr=%q", code, errStr)
	}
}

// Missing message-file argument is a usage error, exit 1.
func TestCommitMsgMissingArgIsUsageError(t *testing.T) {
	repo := mkrepoT(t)
	_, errStr, code := runIn(t, repo, "", "hooks/commit-msg")
	if code != 1 || errStr == "" {
		t.Fatalf("missing arg must die usage exit 1: code=%d stderr=%q", code, errStr)
	}
}

// A repo opted out via commit-gate.disabled is inert (exit 0) even with an
// unapproved, non-HEAD message — matches tests/test_commit_msg.sh.
func TestCommitMsgDisabledRepoIsInert(t *testing.T) {
	repo := mkrepoT(t)
	if out, err := exec.Command("git", "-C", repo, "config", "commit-gate.disabled", "true").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v %s", err, out)
	}
	msgFile := filepath.Join(t.TempDir(), "MSG")
	if err := os.WriteFile(msgFile, []byte("feat: whatever\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errStr, code := runIn(t, repo, "", "hooks/commit-msg", msgFile)
	if code != 0 || errStr != "" {
		t.Fatalf("disabled repo must be inert: code=%d stderr=%q", code, errStr)
	}
}

// A repo opted out via commit-gate.disabled is inert (exit 0) for pre-push
// too, even without a push token — matches the disabled-repo semantics in
// tests/test_commit_msg.sh applied to the push side.
func TestPrePushDisabledRepoIsInert(t *testing.T) {
	repo := mkrepoT(t)
	if out, err := exec.Command("git", "-C", repo, "config", "commit-gate.disabled", "true").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v %s", err, out)
	}
	_, errStr, code := runIn(t, repo, "refs\n", "hooks/pre-push", "origin", "url")
	if code != 0 || errStr != "" {
		t.Fatalf("disabled repo must be inert: code=%d stderr=%q", code, errStr)
	}
}
