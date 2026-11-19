package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cehbz/commit-gate/gatestate"
)

// writeHook installs an executable sh script as the repo's own hook.
func writeHook(t *testing.T, repo, name, script string) {
	t.Helper()
	dir := filepath.Join(gitIn(t, repo, "rev-parse", "--absolute-git-dir"), "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestHookNamesListsEveryHookGitRuns(t *testing.T) {
	out, errStr, code := run(t, "", "commit-gate", "hook-names")
	// githooks(5), less fsmonitor-watchman, which git runs from the path
	// core.fsmonitor names, never from core.hooksPath.
	want := strings.Join([]string{
		"commit-msg", "pre-push",
		"applypatch-msg", "pre-applypatch", "post-applypatch",
		"pre-commit", "pre-merge-commit", "prepare-commit-msg", "post-commit",
		"pre-rebase", "post-checkout", "post-merge",
		"pre-receive", "update", "proc-receive", "post-receive", "post-update",
		"reference-transaction", "push-to-checkout", "pre-auto-gc", "post-rewrite",
		"sendemail-validate",
		"p4-changelist", "p4-prepare-changelist", "p4-post-changelist", "p4-pre-submit",
		"post-index-change",
	}, "\n") + "\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d stderr=%q\ngot:\n%s\nwant:\n%s", code, errStr, out, want)
	}
}

func TestARepoPreCommitHookRunsUnderTheGlobalGate(t *testing.T) {
	repo := mkrepoT(t)
	marker := filepath.Join(t.TempDir(), "ran")
	writeHook(t, repo, "pre-commit", "touch '"+marker+"'")
	writeAndAdd(t, repo, "f", "x")
	r, _ := gatestate.Open(repo)
	if err := gatestate.AppendApprovals(r.ManifestPath(), []string{"feat: with pre-commit"}); err != nil {
		t.Fatal(err)
	}
	if _, errStr, code := gitCommit(t, repo, "-m", "feat: with pre-commit"); code != 0 {
		t.Fatalf("approved commit: code=%d stderr=%q", code, errStr)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("the repo's own pre-commit did not run")
	}
}

func TestARepoPreCommitHookRefusalAbortsTheCommit(t *testing.T) {
	repo := mkrepoT(t)
	writeHook(t, repo, "pre-commit", "echo 'lint: refused' >&2; exit 3")
	writeAndAdd(t, repo, "f", "x")
	_, errStr, code := gitCommit(t, repo, "-m", "feat: refused")
	if code == 0 || !strings.Contains(errStr, "lint: refused") {
		t.Fatalf("the repo's pre-commit refusal must abort: code=%d stderr=%q", code, errStr)
	}
	r, _ := gatestate.Open(repo)
	if _, err := os.Stat(r.PendingPath()); !os.IsNotExist(err) {
		t.Fatal("commit-msg ran after pre-commit refused")
	}
}

// The KB repo's own commit-msg (hooks/kb-commit-check) refuses unvalidated
// commits; the gate chains to <common-dir>/hooks/commit-msg first.
func TestARepoCommitMsgHookRefusalIsHonouredUnderTheGlobalGate(t *testing.T) {
	repo := mkrepoT(t)
	writeHook(t, repo, "commit-msg", "echo 'kb-commit-check: needs validation' >&2; exit 1")
	writeAndAdd(t, repo, "f", "x")
	r, _ := gatestate.Open(repo)
	if err := gatestate.AppendApprovals(r.ManifestPath(), []string{"feat: unvalidated"}); err != nil {
		t.Fatal(err)
	}
	_, errStr, code := gitCommit(t, repo, "-m", "feat: unvalidated")
	if code == 0 || !strings.Contains(errStr, "kb-commit-check: needs validation") {
		t.Fatalf("the repo's commit-msg refusal must abort: code=%d stderr=%q", code, errStr)
	}
	if subj, _ := gatestate.ApprovedSubjects(r.ManifestPath()); len(subj) != 1 {
		t.Fatalf("the approval must survive the repo's own refusal: %v", subj)
	}
}

func TestARepoPostCheckoutHookRunsUnderTheGlobalGate(t *testing.T) {
	repo := mkrepoT(t)
	machineCommit(t, repo, "base")
	marker := filepath.Join(t.TempDir(), "ran")
	writeHook(t, repo, "post-checkout", "touch '"+marker+"'")
	gitIn(t, repo, "checkout", "-q", "-b", "side")
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("the repo's own post-checkout did not run")
	}
}

// updateInsteadPair returns a non-bare repo that accepts pushes to its
// checked-out branch (receive.denyCurrentBranch=updateInstead), its branch,
// and an opted-out clone of it holding one more commit, which adds new.txt.
func updateInsteadPair(t *testing.T) (dst, src, branch string) {
	t.Helper()
	dst = mkrepoT(t)
	writeAndAdd(t, dst, "tracked.txt", "v1\n")
	machineCommit(t, dst, "base")
	gitIn(t, dst, "config", "receive.denyCurrentBranch", "updateInstead")
	branch = gitIn(t, dst, "symbolic-ref", "--short", "HEAD")
	src = filepath.Join(t.TempDir(), "src")
	if out, err := exec.Command("git", "clone", "-q", dst, src).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v %s", err, out)
	}
	gitIn(t, src, "config", "user.email", "t@example.com")
	gitIn(t, src, "config", "user.name", "Test")
	gitIn(t, src, "config", "commit-gate.disabled", "true")
	writeAndAdd(t, src, "new.txt", "pushed\n")
	machineCommit(t, src, "add new.txt")
	return dst, src, branch
}

func pushTo(t *testing.T, src, branch string) (string, int) {
	t.Helper()
	out, err := exec.Command("git", "-C", src, "push", "-q", "origin", "HEAD:"+branch).CombinedOutput()
	if ee, ok := err.(*exec.ExitError); ok {
		return string(out), ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), 0
}

func TestAnUpdateInsteadPushUpdatesTheWorkTreeUnderTheGlobalGate(t *testing.T) {
	dst, src, branch := updateInsteadPair(t)
	if out, code := pushTo(t, src, branch); code != 0 {
		t.Fatalf("push: code=%d %s", code, out)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "new.txt")); err != nil || string(b) != "pushed\n" {
		t.Fatalf("the pushed commit must be checked out: %q %v", b, err)
	}
}

func TestAnUpdateInsteadPushIsRefusedOnADirtyWorkTree(t *testing.T) {
	dst, src, branch := updateInsteadPair(t)
	if err := os.WriteFile(filepath.Join(dst, "tracked.txt"), []byte("local edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, code := pushTo(t, src, branch); code == 0 {
		t.Fatalf("a push onto a dirty work tree must be refused: %s", out)
	}
	if _, err := os.Stat(filepath.Join(dst, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("a refused push must leave the work tree alone")
	}
}

func TestARepoPushToCheckoutHookRunsUnderTheGlobalGate(t *testing.T) {
	dst, src, branch := updateInsteadPair(t)
	marker := filepath.Join(t.TempDir(), "ran")
	writeHook(t, dst, "push-to-checkout", "touch '"+marker+"'; exit 1")
	if out, code := pushTo(t, src, branch); code == 0 {
		t.Fatalf("the repo's push-to-checkout refused, so the push must fail: %s", out)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("the repo's own push-to-checkout did not run")
	}
}

func TestAnUpdateInsteadPushIntoAnEmptyRepoUpdatesItsWorkTree(t *testing.T) {
	dst := mkrepoT(t)
	gitIn(t, dst, "config", "receive.denyCurrentBranch", "updateInstead")
	branch := gitIn(t, dst, "symbolic-ref", "--short", "HEAD")
	src := mkrepoT(t)
	gitIn(t, src, "config", "commit-gate.disabled", "true")
	gitIn(t, src, "remote", "add", "origin", dst)
	writeAndAdd(t, src, "new.txt", "pushed\n")
	machineCommit(t, src, "add new.txt")
	if out, code := pushTo(t, src, branch); code != 0 {
		t.Fatalf("push into an unborn branch: code=%d %s", code, out)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "new.txt")); err != nil || string(b) != "pushed\n" {
		t.Fatalf("the pushed commit must be checked out: %q %v", b, err)
	}
}

// approvedCommit stages a file and approves msg, so only a repo hook can
// stop the commit.
func approvedCommit(t *testing.T, repo, msg string) (string, int) {
	t.Helper()
	writeAndAdd(t, repo, "f", "x")
	r, _ := gatestate.Open(repo)
	if err := gatestate.AppendApprovals(r.ManifestPath(), []string{msg}); err != nil {
		t.Fatal(err)
	}
	_, errStr, code := gitCommit(t, repo, "-m", msg)
	return errStr, code
}

func assertNoCommit(t *testing.T, repo string) {
	t.Helper()
	if err := exec.Command("git", "-C", repo, "rev-parse", "-q", "--verify", "HEAD").Run(); err == nil {
		t.Fatal("a commit was made")
	}
}

func TestARepoPreCommitHookKilledBySignalAbortsTheCommit(t *testing.T) {
	repo := mkrepoT(t)
	writeHook(t, repo, "pre-commit", "kill -9 $$")
	if errStr, code := approvedCommit(t, repo, "feat: killed pre-commit"); code == 0 {
		t.Fatalf("a pre-commit killed by a signal must abort the commit: stderr=%q", errStr)
	}
	assertNoCommit(t, repo)
}

func TestARepoCommitMsgHookKilledBySignalAbortsTheCommit(t *testing.T) {
	repo := mkrepoT(t)
	writeHook(t, repo, "commit-msg", "kill -9 $$")
	if errStr, code := approvedCommit(t, repo, "feat: killed commit-msg"); code == 0 {
		t.Fatalf("a commit-msg killed by a signal must abort the commit: stderr=%q", errStr)
	}
	assertNoCommit(t, repo)
}

func TestARepoHookExitCodeComesThrough(t *testing.T) {
	repo := mkrepoT(t)
	writeHook(t, repo, "pre-commit", "exit 7")
	if _, errStr, code := runIn(t, repo, "", "hooks/pre-commit"); code != 7 {
		t.Fatalf("exit 7: code=%d stderr=%q", code, errStr)
	}
	writeHook(t, repo, "pre-commit", "kill -9 $$")
	if _, errStr, code := runIn(t, repo, "", "hooks/pre-commit"); code != 128+9 {
		t.Fatalf("killed by SIGKILL: code=%d stderr=%q", code, errStr)
	}
}

func TestARepoPostCheckoutHookGetsItsArguments(t *testing.T) {
	repo := mkrepoT(t)
	machineCommit(t, repo, "base")
	head := gitIn(t, repo, "rev-parse", "HEAD")
	out := filepath.Join(t.TempDir(), "args")
	writeHook(t, repo, "post-checkout", `printf '%s\n' "$@" > '`+out+`'`)
	gitIn(t, repo, "checkout", "-q", "-b", "side")
	b, err := os.ReadFile(out)
	if want := head + "\n" + head + "\n1\n"; err != nil || string(b) != want {
		t.Fatalf("post-checkout args: got %q (%v), want %q", b, err, want)
	}
}

func TestABareRepoPreReceiveHookGetsItsStdinInTheGitDir(t *testing.T) {
	bare := filepath.Join(t.TempDir(), "srv.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v %s", err, out)
	}
	rec := t.TempDir()
	writeHook(t, bare, "pre-receive", "pwd -P > '"+rec+"/pwd'; cat > '"+rec+"/stdin'")
	src := mkrepoT(t)
	gitIn(t, src, "config", "commit-gate.disabled", "true")
	machineCommit(t, src, "base")
	head := gitIn(t, src, "rev-parse", "HEAD")
	gitIn(t, src, "push", "-q", bare, "HEAD:refs/heads/main")
	stdin, _ := os.ReadFile(filepath.Join(rec, "stdin"))
	if want := head + " refs/heads/main\n"; !strings.HasSuffix(string(stdin), want) {
		t.Fatalf("pre-receive stdin: got %q, want a line ending %q", stdin, want)
	}
	pwd, _ := os.ReadFile(filepath.Join(rec, "pwd"))
	wantDir, _ := filepath.EvalSymlinks(bare)
	if strings.TrimSpace(string(pwd)) != wantDir {
		t.Fatalf("pre-receive cwd: got %q, want %q", pwd, wantDir)
	}
}

func TestAPassThroughOutsideARepoFails(t *testing.T) {
	_, errStr, code := runIn(t, t.TempDir(), "", "hooks/pre-commit")
	if code != 1 || !strings.Contains(errStr, "commit-gate:") {
		t.Fatalf("outside a repo: code=%d stderr=%q", code, errStr)
	}
}

// runHookNoGit runs the gate's hook link in dir with extra environment and
// a PATH holding no git, so the pass-through can find the repo's hook only
// without running git.
func runHookNoGit(t *testing.T, dir, name string, env ...string) int {
	t.Helper()
	cmd := exec.Command(filepath.Join(binDir, "hooks", name))
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "PATH=") && !strings.HasPrefix(kv, "GIT_DIR=") && !strings.HasPrefix(kv, "GIT_COMMON_DIR=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(append(cmd.Env, "PATH="+t.TempDir()), env...)
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return 0
}

func TestAPassThroughFindsTheRepoHookWithoutRunningGit(t *testing.T) {
	repo := mkrepoT(t)
	machineCommit(t, repo, "base")
	common := gitIn(t, repo, "rev-parse", "--absolute-git-dir")
	wt := filepath.Join(t.TempDir(), "wt")
	gitIn(t, repo, "worktree", "add", "-q", wt)
	wtGitDir := gitIn(t, wt, "rev-parse", "--absolute-git-dir")
	bare := filepath.Join(t.TempDir(), "srv.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v %s", err, out)
	}
	for _, c := range []struct {
		name, dir, hookRepo string
		env                 []string
	}{
		{"work tree", repo, repo, nil},
		{"linked worktree", wt, repo, nil},
		{"GIT_DIR", bare, bare, []string{"GIT_DIR=."}},
		{"GIT_DIR of a linked worktree", wt, repo, []string{"GIT_DIR=" + wtGitDir}},
		{"GIT_COMMON_DIR", wt, repo, []string{"GIT_DIR=" + wtGitDir, "GIT_COMMON_DIR=" + common}},
	} {
		t.Run(c.name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "ran")
			writeHook(t, c.hookRepo, "post-commit", "echo ran > '"+marker+"'")
			if code := runHookNoGit(t, c.dir, "post-commit", c.env...); code != 0 {
				t.Fatalf("code=%d", code)
			}
			if _, err := os.Stat(marker); err != nil {
				t.Fatal("the repo's own post-commit did not run")
			}
		})
	}
}

// writeBareHook installs an executable hook with no #! line, which git runs
// under /bin/sh when exec fails with ENOEXEC.
func writeBareHook(t *testing.T, repo, name, script string) {
	t.Helper()
	dir := filepath.Join(gitIn(t, repo, "rev-parse", "--absolute-git-dir"), "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestARepoHookWithNoShebangRunsUnderShAsAPassThrough(t *testing.T) {
	repo := mkrepoT(t)
	marker := filepath.Join(t.TempDir(), "ran")
	writeBareHook(t, repo, "pre-commit", "echo \"$0\" > '"+marker+"'")
	if errStr, code := approvedCommit(t, repo, "feat: bare pre-commit"); code != 0 {
		t.Fatalf("approved commit: code=%d stderr=%q", code, errStr)
	}
	if b, err := os.ReadFile(marker); err != nil || !strings.HasSuffix(strings.TrimSpace(string(b)), "hooks/pre-commit") {
		t.Fatalf("the repo's own pre-commit with no #! line did not run under sh: %q %v", b, err)
	}
}

func TestARepoCommitMsgHookWithNoShebangRunsUnderSh(t *testing.T) {
	repo := mkrepoT(t)
	writeBareHook(t, repo, "commit-msg", "grep -q 'feat:' \"$1\" || { echo 'bare: refused' >&2; exit 1; }")
	errStr, code := approvedCommit(t, repo, "chore: refused by the repo")
	if code == 0 || !strings.Contains(errStr, "bare: refused") {
		t.Fatalf("the repo's commit-msg refusal must abort: code=%d stderr=%q", code, errStr)
	}
	if errStr, code := approvedCommit(t, repo, "feat: admitted by the repo"); code != 0 {
		t.Fatalf("the repo's commit-msg must get the message file: code=%d stderr=%q", code, errStr)
	}
}
