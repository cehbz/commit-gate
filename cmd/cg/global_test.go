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

// globalHooksPathViaInclude runs the test with a global git config that sets
// no core.hooksPath itself but includes a file setting it to dir.
func globalHooksPathViaInclude(t *testing.T, dir string) {
	t.Helper()
	d := t.TempDir()
	inc := filepath.Join(d, "hooks.inc")
	if err := os.WriteFile(inc, []byte("[core]\n\thooksPath = "+dir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(d, "gitconfig")
	if err := os.WriteFile(f, []byte("[include]\n\tpath = "+inc+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", f)
}

func TestSessioncheckReadsAnIncludedGlobalHooksPath(t *testing.T) {
	globalHooksPathViaInclude(t, liveHooks(t))
	if out, _ := runSessioncheck(t, mkrepoT(t)); out != "" {
		t.Fatalf("an included global core.hooksPath naming the gate must read as set: %q", out)
	}
}

func TestGateStatusReadsAnIncludedGlobalHooksPath(t *testing.T) {
	globalHooksPathViaInclude(t, liveHooks(t))
	out, errStr, code := runIn(t, mkrepoT(t), "", "commit-gate", "gate-status")
	if code != 0 || !strings.HasPrefix(out, "commit-gate: enabled\n") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errStr)
	}
}

func TestGateEnableReadsAnIncludedGlobalHooksPath(t *testing.T) {
	globalHooksPathViaInclude(t, liveHooks(t))
	repo := mkrepoT(t)
	gitSetConfig(t, repo, "core.hooksPath", t.TempDir())
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "gate-enable"); code != 0 {
		t.Fatalf("gate-enable: code=%d stderr=%q", code, errStr)
	}
	if hp := localHooksPath(t, repo); hp != "" {
		t.Fatalf("the override must be cleared: %q", hp)
	}
}

func TestClearLocalHookspathReadsAnIncludedGlobalHooksPath(t *testing.T) {
	globalHooksPathViaInclude(t, liveHooks(t))
	root := t.TempDir()
	repo := mkrepoAt(t, filepath.Join(root, "r"))
	gitSetConfig(t, repo, "core.hooksPath", liveHooks(t))
	out, errStr, code := run(t, "", "commit-gate", "clear-local-hookspath", root)
	if code != 0 || !strings.HasSuffix(out, "commit-gate: 1 removed, 0 foreign\n") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errStr)
	}
}

// globalHooksPathViaIncludeIf runs the test with a global git config that
// sets core.hooksPath to dir only in repo, through an includeIf "gitdir:".
func globalHooksPathViaIncludeIf(t *testing.T, repo, dir string) {
	t.Helper()
	top, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	inc := filepath.Join(d, "hooks.inc")
	if err := os.WriteFile(inc, []byte("[core]\n\thooksPath = "+dir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(d, "gitconfig")
	if err := os.WriteFile(f, []byte("[includeIf \"gitdir:"+top+"/\"]\n\tpath = "+inc+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", f)
}

func TestSessioncheckReadsAGlobalHooksPathFromAnIncludeIfGitdir(t *testing.T) {
	repo := mkrepoT(t)
	globalHooksPathViaIncludeIf(t, repo, liveHooks(t))
	if out, _ := runSessioncheck(t, repo); out != "" {
		t.Fatalf("the includeIf'd global core.hooksPath gates this repo: %q", out)
	}
}

func TestGateEnableReadsAGlobalHooksPathFromAnIncludeIfGitdir(t *testing.T) {
	repo := mkrepoT(t)
	globalHooksPathViaIncludeIf(t, repo, liveHooks(t))
	gitSetConfig(t, repo, "core.hooksPath", t.TempDir())
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "gate-enable"); code != 0 {
		t.Fatalf("gate-enable: code=%d stderr=%q", code, errStr)
	}
}

func TestGateStatusReadsAGlobalHooksPathFromAnIncludeIfGitdir(t *testing.T) {
	repo := mkrepoT(t)
	globalHooksPathViaIncludeIf(t, repo, liveHooks(t))
	out, errStr, code := runIn(t, repo, "", "commit-gate", "gate-status")
	if code != 0 || !strings.HasPrefix(out, "commit-gate: enabled\n") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errStr)
	}
}

// clear-local-hookspath removes a repo's gate path only where the global
// core.hooksPath gates that repo, and counts the others as errors.
func TestClearLocalHookspathReadsTheGlobalHooksPathPerRepo(t *testing.T) {
	root := t.TempDir()
	gated := mkrepoAt(t, filepath.Join(root, "a"))
	other := mkrepoAt(t, filepath.Join(root, "b"))
	globalHooksPathViaIncludeIf(t, gated, liveHooks(t))
	for _, r := range []string{gated, other} {
		gitSetConfig(t, r, "core.hooksPath", liveHooks(t))
	}
	out, errStr, code := run(t, "", "commit-gate", "clear-local-hookspath", root)
	want := "removed " + gated + ": core.hooksPath was " + liveHooks(t) + "\n" +
		"commit-gate: 1 removed, 0 foreign, 1 error\n"
	if code != 1 || out != want || !strings.Contains(errStr, other+": the global core.hooksPath is unset") {
		t.Fatalf("code=%d stderr=%q\ngot:\n%s\nwant:\n%s", code, errStr, out, want)
	}
	if hp := localHooksPath(t, other); hp != liveHooks(t) {
		t.Fatalf("a repo the global gate doesn't cover keeps its gate path: %q", hp)
	}
}

// Where an includeIf sets a foreign global core.hooksPath for a repo, the
// remedy names the file that sets it, which git config --global doesn't
// write.
func TestClearLocalHookspathNamesTheIncludedFileSettingAForeignValue(t *testing.T) {
	root := t.TempDir()
	repo := mkrepoAt(t, filepath.Join(root, "r"))
	elsewhere := t.TempDir()
	globalHooksPathViaIncludeIf(t, repo, elsewhere)
	inc := filepath.Join(filepath.Dir(os.Getenv("GIT_CONFIG_GLOBAL")), "hooks.inc")
	gitSetConfig(t, repo, "core.hooksPath", liveHooks(t))
	_, errStr, code := run(t, "", "commit-gate", "clear-local-hookspath", root)
	fix := "git config --file " + inc + " core.hooksPath " + liveHooks(t)
	if code != 1 || !strings.Contains(errStr, fix) || strings.Contains(errStr, "git config --global") {
		t.Fatalf("code=%d stderr=%q, want the fix %q", code, errStr, fix)
	}
}

// sessioncheck and gate-enable name the same remedy clear-local-hookspath
// does: the included file that sets a foreign global core.hooksPath.
func TestTheGlobalGateRemedyNamesTheIncludedFileEverywhere(t *testing.T) {
	repo := mkrepoT(t)
	globalHooksPathViaIncludeIf(t, repo, t.TempDir())
	inc := filepath.Join(filepath.Dir(os.Getenv("GIT_CONFIG_GLOBAL")), "hooks.inc")
	fix := "git config --file " + inc + " core.hooksPath " + liveHooks(t)
	_, so := runSessioncheck(t, repo)
	if !strings.Contains(so.SystemMessage, fix) || strings.Contains(so.SystemMessage, "git config --global") ||
		!strings.Contains(so.HookSpecificOutput.AdditionalContext, fix) {
		t.Fatalf("sessioncheck: %+v, want the fix %q", so, fix)
	}
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "gate-enable"); code != 1 || !strings.Contains(errStr, fix) {
		t.Fatalf("gate-enable: code=%d stderr=%q, want the fix %q", code, errStr, fix)
	}
	if out, _, _ := runIn(t, repo, "", "commit-gate", "gate-status"); !strings.Contains(out, fix) {
		t.Fatalf("gate-status: %q, want the fix %q", out, fix)
	}
}
