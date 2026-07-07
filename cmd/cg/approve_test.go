package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cehbz/commit-gate/gatestate"
)

// (a) approve --yes with a stdin message records exactly one manifest line
// (hash + two spaces + subject).
func TestApproveYesStdinRecordsOneLine(t *testing.T) {
	repo := mkrepoT(t)
	setHooksPath(t, repo)

	out, errStr, code := runIn(t, repo, "feat: one", "commit-gate", "approve", "--yes")
	if code != 0 {
		t.Fatalf("approve failed: code=%d stdout=%q stderr=%q", code, out, errStr)
	}
	if !strings.Contains(out, "commit-gate: recorded 1 approval(s)") {
		t.Fatalf("missing recorded-count message: %q", out)
	}

	r, err := gatestate.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(r.ManifestPath())
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	want := gatestate.CanonicalHash("feat: one") + "  feat: one\n"
	if string(manifest) != want {
		t.Fatalf("manifest mismatch:\ngot:  %q\nwant: %q", manifest, want)
	}
}

// (b) -F with two messages separated by @@COMMIT-GATE-SEP@@ records both.
func TestApproveFileFlagRecordsBothMessages(t *testing.T) {
	repo := mkrepoT(t)
	setHooksPath(t, repo)

	f := filepath.Join(t.TempDir(), "msgs")
	if err := os.WriteFile(f, []byte("feat: a\n@@COMMIT-GATE-SEP@@\nfeat: b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, errStr, code := runIn(t, repo, "", "commit-gate", "approve", "--yes", "-F", f)
	if code != 0 {
		t.Fatalf("approve -F failed: code=%d stderr=%q", code, errStr)
	}

	r, _ := gatestate.Open(repo)
	subj, err := gatestate.ApprovedSubjects(r.ManifestPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(subj) != 2 || subj[0] != "feat: a" || subj[1] != "feat: b" {
		t.Fatalf("want [feat: a feat: b], got %v", subj)
	}
}

// (c) --plan extracts every `git commit ... -m "..."` / `-m '...'` payload.
func TestApprovePlanExtractsMPayloads(t *testing.T) {
	repo := mkrepoT(t)
	setHooksPath(t, repo)

	plan := filepath.Join(t.TempDir(), "plan.md")
	content := "Step 1: `git commit -m \"feat: alpha\"`\n" +
		"Step 2: git commit --amend -m 'feat: beta'\n"
	if err := os.WriteFile(plan, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, errStr, code := runIn(t, repo, "", "commit-gate", "approve", "--yes", "--plan", plan)
	if code != 0 {
		t.Fatalf("approve --plan failed: code=%d stderr=%q", code, errStr)
	}

	r, _ := gatestate.Open(repo)
	subj, err := gatestate.ApprovedSubjects(r.ManifestPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(subj) != 2 || subj[0] != "feat: alpha" || subj[1] != "feat: beta" {
		t.Fatalf("want [feat: alpha feat: beta], got %v", subj)
	}
}

// (d) bare approve (no -F/--plan) uses the preloaded cg-pending file and
// consumes (removes) it on success.
func TestApproveBareConsumesPending(t *testing.T) {
	repo := mkrepoT(t)
	setHooksPath(t, repo)

	r, _ := gatestate.Open(repo)
	if err := os.WriteFile(r.PendingPath(), []byte("feat: pend"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, errStr, code := runIn(t, repo, "", "commit-gate", "approve", "--yes")
	if code != 0 {
		t.Fatalf("bare approve failed: code=%d stderr=%q", code, errStr)
	}
	if _, err := os.Stat(r.PendingPath()); !os.IsNotExist(err) {
		t.Fatalf("pending file must be consumed")
	}
	subj, err := gatestate.ApprovedSubjects(r.ManifestPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(subj) != 1 || subj[0] != "feat: pend" {
		t.Fatalf("want [feat: pend], got %v", subj)
	}
}

// (e) cwd-sensitivity regression: approve run from a DIFFERENT gated repo
// must not pick up another repo's pending message.
func TestApproveDifferentRepoFindsNothing(t *testing.T) {
	repoA := mkrepoT(t)
	setHooksPath(t, repoA)
	repoB := mkrepoT(t)
	setHooksPath(t, repoB)

	rA, _ := gatestate.Open(repoA)
	if err := os.WriteFile(rA.PendingPath(), []byte("feat: only-in-A"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, errStr, code := runIn(t, repoB, "", "commit-gate", "approve", "--yes")
	if code == 0 {
		t.Fatalf("approve from repoB must fail, got exit 0")
	}
	if !strings.Contains(errStr, "commit-gate: no messages to approve") {
		t.Fatalf("want 'no messages to approve', got %q", errStr)
	}
	if _, err := os.Stat(rA.PendingPath()); err != nil {
		t.Fatalf("repoA pending must remain untouched: %v", err)
	}
	rB, _ := gatestate.Open(repoB)
	subj, _ := gatestate.ApprovedSubjects(rB.ManifestPath())
	if len(subj) != 0 {
		t.Fatalf("repoB manifest must stay empty, got %v", subj)
	}
}

// (f) an existing push token is invalidated (removed) with the exact
// message when new approvals are recorded.
func TestApprovePushTokenInvalidated(t *testing.T) {
	repo := mkrepoT(t)
	setHooksPath(t, repo)

	r, _ := gatestate.Open(repo)
	if err := os.MkdirAll(r.GateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.PushTokenPath(), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errStr, code := runIn(t, repo, "feat: after-token", "commit-gate", "approve", "--yes")
	if code != 0 {
		t.Fatalf("approve failed: code=%d stderr=%q", code, errStr)
	}
	want := "commit-gate: push token invalidated (approvals postdate it — re-run approve-push)"
	if !strings.Contains(out, want) {
		t.Fatalf("missing invalidation message, got %q", out)
	}
	if _, err := os.Stat(r.PushTokenPath()); !os.IsNotExist(err) {
		t.Fatalf("push token must be removed")
	}
}

// (g) an ungated repo (no core.hooksPath set) dies with the exact
// "repo not enabled" message, exit 1.
func TestApproveUngatedRepoDies(t *testing.T) {
	repo := mkrepoT(t)

	_, errStr, code := runIn(t, repo, "feat: x", "commit-gate", "approve", "--yes")
	if code != 1 {
		t.Fatalf("want exit 1, got %d (stderr=%q)", code, errStr)
	}
	want := "commit-gate: repo not enabled — run: gate-enable\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}

// Additional die-path coverage beyond the mandated (a)-(g): opted-out repo,
// non-repo directory, and bad usage all die with the exact expected text.
func TestApproveDisabledRepoDies(t *testing.T) {
	repo := mkrepoT(t)
	setHooksPath(t, repo)
	if out, err := exec.Command("git", "-C", repo, "config", "commit-gate.disabled", "true").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v %s", err, out)
	}
	_, errStr, code := runIn(t, repo, "feat: x", "commit-gate", "approve", "--yes")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: repo is opted out (commit-gate.disabled); run: gate-enable\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}

func TestApproveNonRepoDies(t *testing.T) {
	dir := t.TempDir()
	_, errStr, code := runIn(t, dir, "feat: x", "commit-gate", "approve", "--yes")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: not inside a git repository\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}

func TestApproveBadUsageDies(t *testing.T) {
	repo := mkrepoT(t)
	setHooksPath(t, repo)
	_, errStr, code := runIn(t, repo, "", "commit-gate", "approve", "--bogus")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: usage: approve [-F file | --plan plan.md] [--yes]   (bare: reads the preloaded pending file)\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}

// Zero-messages die path via an empty -F file.
func TestApproveNoMessagesDies(t *testing.T) {
	repo := mkrepoT(t)
	setHooksPath(t, repo)
	f := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(f, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errStr, code := runIn(t, repo, "", "commit-gate", "approve", "--yes", "-F", f)
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	want := "commit-gate: no messages to approve\n"
	if errStr != want {
		t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", errStr, want)
	}
}
