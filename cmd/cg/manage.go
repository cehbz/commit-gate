// manage.go implements the six gate-management commands, each a word after
// commit-gate: approve-push, approve-deploy, enable, disable, status and
// clear-approvals.
//
// approve-push, approve-deploy and disable are human-only, run by pasting a
// `!` command an agent composed; like approve, they run launchguard.Check
// first and refuse if their launch command runs anything besides cd and
// commit-gate. See package launchguard.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/cehbz/commit-gate/confirm"
	"github.com/cehbz/commit-gate/gatestate"
)

// hasYes reports whether the first argument is exactly "--yes".
func hasYes(args []string) bool {
	return len(args) > 0 && args[0] == "--yes"
}

// authorization is a one-shot leave a human-only command grants in an
// enabled repo: a token file its consumer removes.
type authorization struct {
	word   string                       // the command word that grants it
	prompt string                       // the confirmation's question
	token  func(*gatestate.Repo) string // where the token lives
	done   string                       // the report once granted
}

var (
	pushAuthorization = authorization{
		word:   "approve-push",
		prompt: "commit-gate — authorize ONE push from this repo?",
		token:  (*gatestate.Repo).PushTokenPath,
		done:   "commit-gate: one push authorized",
	}
	deployAuthorization = authorization{
		word:   "approve-deploy",
		prompt: "commit-gate — authorize ONE deploy run from this clone?",
		token:  (*gatestate.Repo).DeployTokenPath,
		done:   "commit-gate: one deploy authorized",
	}
)

// cmdApprovePush implements `commit-gate approve-push`: grant one push.
func cmdApprovePush(args []string) int { return grant(pushAuthorization, args) }

// cmdApproveDeploy implements `commit-gate approve-deploy`: grant one deploy
// run from this clone, which its next make deploy consumes.
func cmdApproveDeploy(args []string) int { return grant(deployAuthorization, args) }

// grant runs the launch guard, requires an enabled repo, confirms unless
// --yes, writes a's token and reports it.
func grant(a authorization, args []string) int {
	if code := checkLaunchGuard(a.word); code != 0 {
		return code
	}
	cwd, _ := os.Getwd()
	repo, err := gatestate.Open(cwd)
	if err != nil {
		return dieApprove("not inside a git repository")
	}
	if repo.Disabled() {
		return dieApprove("repo is opted out (commit-gate.disabled); run: commit-gate enable")
	}
	if !repo.Enabled(liveHooksDir()) {
		return dieApprove("repo not enabled — run: commit-gate enable")
	}
	if !hasYes(args) {
		if err := confirm.Confirm(a.prompt); err != nil {
			return dieApprove("%s", err)
		}
	}
	if err := os.MkdirAll(repo.GateDir(), 0o755); err != nil {
		return dieApprove("%s", err)
	}
	if err := os.WriteFile(a.token(repo), []byte(time.Now().Format(time.RFC1123)+"\n"), 0o644); err != nil {
		return dieApprove("%s", err)
	}
	fmt.Println(a.done)
	return 0
}

// cmdEnable implements `commit-gate enable`: Repo.Enable with the
// live hooks dir, which clears the repo's opt-out and its repo-local
// core.hooksPath so the global core.hooksPath gates it.
func cmdEnable(_ []string) int {
	cwd, _ := os.Getwd()
	repo, err := gatestate.Open(cwd)
	if err != nil {
		return dieApprove("not inside a git repository")
	}
	if err := repo.Enable(liveHooksDir()); err != nil {
		return dieApprove("%s", err)
	}
	top, err := repo.Toplevel()
	if err != nil {
		return dieApprove("%s", err)
	}
	fmt.Printf("commit-gate: enabled in %s\n", top)
	return 0
}

// cmdDisable implements `commit-gate disable`: confirm unless
// --yes, then opt the repo out via commit-gate.disabled.
func cmdDisable(args []string) int {
	if code := checkLaunchGuard("disable"); code != 0 {
		return code
	}
	cwd, _ := os.Getwd()
	repo, err := gatestate.Open(cwd)
	if err != nil {
		return dieApprove("not inside a git repository")
	}
	if !hasYes(args) {
		if err := confirm.Confirm("commit-gate — opt this repo OUT (disable the gate)?"); err != nil {
			return dieApprove("%s", err)
		}
	}
	if err := repo.GitConfigSet("commit-gate.disabled", "true"); err != nil {
		return dieApprove("%s", err)
	}
	top, err := repo.Toplevel()
	if err != nil {
		return dieApprove("%s", err)
	}
	fmt.Printf("commit-gate: disabled (opted out) for %s\n", top)
	return 0
}

// cmdStatus implements `commit-gate status`: report the repo
// opted-out, enabled (with pending approvals and the push and deploy token states), overridden
// by a core.hooksPath above the global one (naming it, its scope when not
// repo-local, and what gates the repo), or ungated (naming why); an
// unreadable core.hooksPath is an error.
func cmdStatus(_ []string) int {
	cwd, _ := os.Getwd()
	repo, err := gatestate.Open(cwd)
	if err != nil {
		return dieApprove("not inside a git repository")
	}
	hooks := liveHooksDir()
	switch repo.State(hooks) {
	case gatestate.StateDisabled:
		fmt.Println("commit-gate: opted-out")
	case gatestate.StateOverridden:
		hp, scope, _, err := repo.HooksPath()
		if err != nil {
			return dieApprove("cannot read core.hooksPath: %s", err)
		}
		fmt.Println(describeOverride(hp, scope).status())
	case gatestate.StateUngated:
		hp, scope, ok, err := repo.HooksPath()
		p := gatestate.GlobalGateProblem(repo.Dir, hooks)
		switch {
		case err != nil:
			return dieApprove("cannot read core.hooksPath: %s", err)
		case p != "":
			fmt.Printf("commit-gate: ungated (%s; to gate every repo: %s)\n", p, gatestate.GlobalGateFix(repo.Dir, hooks))
		case ok:
			fmt.Printf("commit-gate: ungated (core.hooksPath is %s, from %s config)\n", hp, scope)
		default:
			fmt.Println("commit-gate: ungated (core.hooksPath is unset)")
		}
	case gatestate.StateEnabled:
		fmt.Println("commit-gate: enabled")
		subj, err := gatestate.ApprovedSubjects(repo.ManifestPath())
		if err != nil {
			return dieApprove("%s", err)
		}
		if len(subj) > 0 {
			fmt.Println("approved (pending):")
			for _, s := range subj {
				fmt.Println("  " + s)
			}
		} else {
			fmt.Println("approved (pending): none")
		}
		for _, t := range []struct{ name, path string }{
			{"push token", repo.PushTokenPath()},
			{"deploy token", repo.DeployTokenPath()},
		} {
			if _, err := os.Stat(t.path); err == nil {
				fmt.Println(t.name + ": present")
			} else {
				fmt.Println(t.name + ": none")
			}
		}
	}
	return 0
}

// cmdClearApprovals implements `commit-gate clear-approvals`: truncate the
// manifest and remove the push and deploy tokens.
func cmdClearApprovals(_ []string) int {
	cwd, _ := os.Getwd()
	repo, err := gatestate.Open(cwd)
	if err != nil {
		return dieApprove("not inside a git repository")
	}
	if !repo.Enabled(liveHooksDir()) {
		return dieApprove("repo not enabled")
	}
	if err := os.MkdirAll(repo.GateDir(), 0o755); err != nil {
		return dieApprove("%s", err)
	}
	if err := os.WriteFile(repo.ManifestPath(), []byte{}, 0o644); err != nil {
		return dieApprove("%s", err)
	}
	for _, token := range []string{repo.PushTokenPath(), repo.DeployTokenPath()} {
		if err := os.Remove(token); err != nil && !os.IsNotExist(err) {
			return dieApprove("%s", err)
		}
	}
	fmt.Println("commit-gate: approvals, push token and deploy token cleared")
	return 0
}
