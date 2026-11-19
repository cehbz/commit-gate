package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cehbz/commit-gate/gatestate"
)

// gateHooks are the hooks the gate enforces.
var gateHooks = []string{"commit-msg", "pre-push"}

// passThroughHooks are every other hook git runs from core.hooksPath
// (githooks(5)); each runs the repo's own <common-dir>/hooks/<name> and
// nothing else. fsmonitor-watchman is not one: git runs it from the path
// core.fsmonitor names.
var passThroughHooks = []string{
	"applypatch-msg", "pre-applypatch", "post-applypatch",
	"pre-commit", "pre-merge-commit", "prepare-commit-msg", "post-commit",
	"pre-rebase", "post-checkout", "post-merge",
	"pre-receive", "update", "proc-receive", "post-receive", "post-update",
	"reference-transaction", "push-to-checkout", "pre-auto-gc", "post-rewrite",
	"sendemail-validate",
	"p4-changelist", "p4-prepare-changelist", "p4-post-changelist", "p4-pre-submit",
	"post-index-change",
}

func isPassThrough(name string) bool {
	for _, h := range passThroughHooks {
		if h == name {
			return true
		}
	}
	return false
}

// cmdHookNames prints every hook name the gate's hook directory holds, one
// per line: install.sh, the Makefile and the tests link each to the binary.
func cmdHookNames() int {
	for _, h := range gateHooks {
		fmt.Println(h)
	}
	for _, h := range passThroughHooks {
		fmt.Println(h)
	}
	return 0
}

// cmdPassThrough runs the repo's own hook of this name with git's arguments
// and stdin, and exits with its code. Without one it exits 0, except
// push-to-checkout: its presence in core.hooksPath turns off git's own
// updateInstead work-tree update, so without the repo's own hook it does
// that update itself.
func cmdPassThrough(name string, args []string) int {
	repo, err := hookRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "commit-gate: %s: %s\n", name, err)
		return 1
	}
	if _, ok := localHook(repo, name); !ok {
		if name == "push-to-checkout" {
			return pushToDeploy(args)
		}
		return 0
	}
	if code := chainLocal(repo, name, args); code != -1 {
		return code
	}
	return 0
}

// hookRepo finds the repo a pass-through runs for without running git, since
// hooks such as reference-transaction and post-index-change run many times
// per command: from GIT_COMMON_DIR or GIT_DIR when git exports them, else
// from the .git directory or gitfile in the directory git runs the hook in.
// gatestate.Open resolves anything else.
func hookRepo() (*gatestate.Repo, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if common := commonDir(cwd); common != "" {
		return &gatestate.Repo{Dir: cwd, Common: common}, nil
	}
	return gatestate.Open(cwd)
}

// commonDir is the git common dir named by the environment or by cwd's .git,
// or "" when neither names one.
func commonDir(cwd string) string {
	if c := os.Getenv("GIT_COMMON_DIR"); c != "" {
		return absIn(cwd, c)
	}
	gitDir := os.Getenv("GIT_DIR")
	if gitDir != "" {
		gitDir = absIn(cwd, gitDir)
	} else if gitDir = dotGit(cwd); gitDir == "" {
		return ""
	}
	// A linked worktree's git dir names the common dir in its commondir file.
	if b, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		return absIn(gitDir, strings.TrimSpace(string(b)))
	}
	return gitDir
}

// dotGit is the git dir of the work tree rooted at dir: its .git directory,
// or the path a .git file's "gitdir: " line names.
func dotGit(dir string) string {
	p := filepath.Join(dir, ".git")
	info, err := os.Stat(p)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return p
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(b), "\n")
	target, ok := strings.CutPrefix(strings.TrimSpace(line), "gitdir: ")
	if !ok {
		return ""
	}
	return absIn(dir, target)
}

func absIn(base, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(base, p)
}

// pushToDeploy is git's push_to_deploy (builtin/receive-pack.c): refuse
// unless the work tree and index match HEAD (the empty tree on an unborn
// branch), then check out the pushed commit. git runs the hook with GIT_DIR
// and GIT_WORK_TREE set, so these git commands act on the receiving repo.
func pushToDeploy(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "commit-gate: usage: push-to-checkout <commit>")
		return 1
	}
	base := "HEAD"
	if exec.Command("git", "rev-parse", "--verify", "-q", "HEAD").Run() != nil {
		out, err := exec.Command("git", "hash-object", "-t", "tree", "/dev/null").Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "commit-gate: %s\n", err)
			return 1
		}
		base = strings.TrimSpace(string(out))
	}
	steps := []struct {
		args    []string
		refusal string
	}{
		{[]string{"update-index", "-q", "--ignore-submodules", "--refresh"}, "Up-to-date check failed"},
		{[]string{"diff-files", "--quiet", "--ignore-submodules", "--"}, "Working directory has unstaged changes"},
		{[]string{"diff-index", "--quiet", "--cached", "--ignore-submodules", base, "--"}, "Working directory has staged changes"},
		{[]string{"read-tree", "-u", "-m", args[0]}, "Could not update working tree to new HEAD"},
	}
	for _, s := range steps {
		cmd := exec.Command("git", s.args...)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "commit-gate: "+s.refusal)
			return 1
		}
	}
	return 0
}
