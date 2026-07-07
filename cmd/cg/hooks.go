// hooks.go implements the two native git hook subcommands: commit-msg and
// pre-push. These are invoked by git itself (via core.hooksPath pointing at
// this binary's hooks/ directory), not by the Claude Code harness. Behavior
// is a direct transcription of hooks/commit-msg and hooks/pre-push (see
// lib/common for chain_local/gate_dir/is_disabled).
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cehbz/commit-gate/gatestate"
)

// chainLocal runs the repo-local hook <repo.Common>/hooks/<name> — the
// NATIVE git hook location, independent of whatever core.hooksPath is
// configured to — if it exists and is executable, passing args and
// inheriting stdio, with Dir set to the repo's working directory. It
// returns -1 to mean "continue" (no local hook present, or the local hook
// ran and exited 0), or the child's exit code to propagate immediately.
func chainLocal(repo *gatestate.Repo, name string, args []string) int {
	path := filepath.Join(repo.Common, "hooks", name)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return -1
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = repo.Dir
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return 1 // couldn't even run it; fail closed like the bash `|| exit $?`
	}
	return -1
}

// cmdCommitMsg implements the commit-msg git hook.
func cmdCommitMsg(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "commit-gate: usage: commit-msg <message-file>")
		return 1
	}
	msgFile := args[0]

	cwd, _ := os.Getwd()
	repo, err := gatestate.Open(cwd)
	if err != nil {
		return 0 // not a repo
	}

	if code := chainLocal(repo, "commit-msg", args); code != -1 {
		return code
	}

	if repo.Disabled() {
		return 0
	}

	content, err := os.ReadFile(msgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "commit-gate: %s\n", err)
		return 1
	}
	msg := string(content)
	hash := gatestate.CanonicalHash(msg)

	if ok, _ := gatestate.ConsumeApproval(repo.ManifestPath(), hash); ok {
		os.Remove(repo.PendingPath())
		return 0
	}

	if headMsg, has := repo.HeadMessage(); has && gatestate.CanonicalHash(headMsg) == hash {
		os.Remove(repo.PendingPath())
		return 0
	}

	// Stage the rejected message for the human's bare `approve`.
	if err := os.WriteFile(repo.PendingPath(), content, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "commit-gate: %s\n", err)
		return 1
	}
	subject := gatestate.Subject(msg)
	fmt.Fprintf(os.Stderr,
		"commit-gate: commit message not approved.\n"+
			"  subject: %s\n"+
			"  sha256 : %s\n"+
			"Have the user run:  approve   (with this exact message)\n"+
			"Human override:     git commit --no-verify\n",
		subject, hash)
	return 1
}

// cmdPrePush implements the pre-push git hook.
func cmdPrePush(args []string) int {
	cwd, _ := os.Getwd()
	repo, err := gatestate.Open(cwd)
	if err != nil {
		return 0 // not a repo
	}

	if code := chainLocal(repo, "pre-push", args); code != -1 {
		return code
	}

	if repo.Disabled() {
		return 0
	}

	token := repo.PushTokenPath()
	if _, err := os.Stat(token); err == nil {
		os.Remove(token)
		return 0
	}

	fmt.Fprint(os.Stderr,
		"commit-gate: push not approved.\n"+
			"Have the user run:  approve-push\n"+
			"Human override:     git push --no-verify\n")
	return 1
}
