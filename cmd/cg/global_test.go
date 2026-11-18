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

// gitIn runs git in repo, failing the test on error, and returns its trimmed stdout.
func gitIn(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, errb.String())
	}
	return strings.TrimRight(string(out), "\n")
}

// machineCommit commits what is staged the way a machine committer does,
// with core.hooksPath=/dev/null, so no hook runs.
func machineCommit(t *testing.T, repo, msg string) {
	t.Helper()
	gitIn(t, repo, "-c", "core.hooksPath=/dev/null", "commit", "-q", "--allow-empty", "-m", msg)
}

// localHooksPath is the repo-local core.hooksPath, or "" when unset.
func localHooksPath(t *testing.T, repo string) string {
	t.Helper()
	out, _ := exec.Command("git", "-C", repo, "config", "--local", "--get", "core.hooksPath").Output()
	return strings.TrimRight(string(out), "\n")
}

// assertGatesCommits: the repo rejects an unapproved commit, and admits it
// once the user approves the rejected message, with no repo-local
// core.hooksPath: the global one does the gating.
func assertGatesCommits(t *testing.T, repo string) {
	t.Helper()
	writeAndAdd(t, repo, "f", "x")
	_, errStr, code := gitCommit(t, repo, "-m", "feat: never enabled")
	if code == 0 || !strings.Contains(errStr, "commit-gate: commit message not approved.") {
		t.Fatalf("unapproved commit: code=%d stderr=%q", code, errStr)
	}
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "approve", "--yes"); code != 0 {
		t.Fatalf("approve: code=%d stderr=%q", code, errStr)
	}
	if _, errStr, code := gitCommit(t, repo, "-m", "feat: never enabled"); code != 0 {
		t.Fatalf("approved commit: code=%d stderr=%q", code, errStr)
	}
	if hp := localHooksPath(t, repo); hp != "" {
		t.Fatalf("a repo-local core.hooksPath appeared: %q", hp)
	}
}

func TestANeverEnabledInitRepoGatesCommits(t *testing.T) {
	assertGatesCommits(t, mkrepoT(t))
}

func TestANeverEnabledClonedRepoGatesCommits(t *testing.T) {
	src := mkrepoT(t)
	machineCommit(t, src, "init")
	dst := filepath.Join(t.TempDir(), "clone")
	if out, err := exec.Command("git", "clone", "-q", src, dst).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v %s", err, out)
	}
	gitIn(t, dst, "config", "user.email", "t@example.com")
	gitIn(t, dst, "config", "user.name", "Test")
	assertGatesCommits(t, dst)
}

func TestClearApprovalsCreatesTheManifestInANeverEnabledRepo(t *testing.T) {
	repo := mkrepoT(t)
	out, errStr, code := runIn(t, repo, "", "commit-gate", "clear-approvals")
	if code != 0 || out != "commit-gate: approvals and push token cleared\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errStr)
	}
	r, _ := gatestate.Open(repo)
	if b, err := os.ReadFile(r.ManifestPath()); err != nil || len(b) != 0 {
		t.Fatalf("manifest must exist and be empty: %q %v", b, err)
	}
}

func TestApprovePushWritesTheTokenInANeverEnabledRepo(t *testing.T) {
	repo := mkrepoT(t)
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "approve-push", "--yes"); code != 0 {
		t.Fatalf("approve-push: code=%d stderr=%q", code, errStr)
	}
	r, _ := gatestate.Open(repo)
	if _, err := os.Stat(r.PushTokenPath()); err != nil {
		t.Fatalf("push token must exist: %v", err)
	}
}

func TestAMachineCommitWithHooksPathDevNullGoesThrough(t *testing.T) {
	repo := mkrepoT(t)
	writeAndAdd(t, repo, "f", "x")
	cmd := exec.Command("git", "-C", repo, "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "auto: snapshot")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("machine commit refused: %v %s", err, out)
	}
	r, _ := gatestate.Open(repo)
	if _, err := os.Stat(r.PendingPath()); !os.IsNotExist(err) {
		t.Fatal("no hook may run for a machine commit, yet cg-pending exists")
	}
}

func TestAnUnapprovedMergeCommitIsRejected(t *testing.T) {
	repo := mkrepoT(t)
	machineCommit(t, repo, "base")
	trunk := gitIn(t, repo, "symbolic-ref", "--short", "HEAD")
	gitIn(t, repo, "checkout", "-q", "-b", "side")
	machineCommit(t, repo, "side")
	gitIn(t, repo, "checkout", "-q", trunk)
	machineCommit(t, repo, "trunk")
	out, err := exec.Command("git", "-C", repo, "merge", "--no-ff", "side", "-m", "Merge side").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "commit-gate: commit message not approved.") {
		t.Fatalf("unapproved merge must be rejected: err=%v out=%s", err, out)
	}
	r, _ := gatestate.Open(repo)
	if p, _ := os.ReadFile(r.PendingPath()); gatestate.Subject(string(p)) != "Merge side" {
		t.Fatalf("cg-pending must hold the merge message, got %q", p)
	}
}
