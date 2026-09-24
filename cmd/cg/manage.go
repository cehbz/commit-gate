// manage.go implements the five gate-management commands: approve-push,
// gate-enable, gate-disable, gate-status, clear-approvals. Behavioral
// contract = reference files bin/approve-push, bin/gate-enable,
// bin/gate-disable, bin/gate-status, bin/clear-approvals (sourcing
// lib/common's require_enabled/confirm/die/gate_dir/is_enabled/is_disabled).
//
// approve-push and gate-disable are human-only, run by pasting a `!`
// command an agent composed; like approve, they run launchguard.Check
// first and refuse if their launch command contains anything besides cd
// and commit-gate's own commands. See package launchguard.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/cehbz/commit-gate/confirm"
	"github.com/cehbz/commit-gate/gatestate"
)

// hasYes reports whether the first argument is exactly "--yes", matching
// bash's `[ "${1:-}" = "--yes" ]` check.
func hasYes(args []string) bool {
	return len(args) > 0 && args[0] == "--yes"
}

// cmdApprovePush implements the `approve-push` command: require_enabled;
// confirm unless --yes; write a push token; report success.
func cmdApprovePush(args []string) int {
	if code := checkLaunchGuard("approve-push"); code != 0 {
		return code
	}
	cwd, _ := os.Getwd()
	repo, err := gatestate.Open(cwd)
	if err != nil {
		return dieApprove("not inside a git repository")
	}
	if repo.Disabled() {
		return dieApprove("repo is opted out (commit-gate.disabled); run: gate-enable")
	}
	if !repo.Enabled(liveHooksDir()) {
		return dieApprove("repo not enabled — run: gate-enable")
	}
	if !hasYes(args) {
		if err := confirm.Confirm("commit-gate — authorize ONE push from this repo?"); err != nil {
			return dieApprove("%s", err)
		}
	}
	if err := os.MkdirAll(repo.GateDir(), 0o755); err != nil {
		return dieApprove("%s", err)
	}
	if err := os.WriteFile(repo.PushTokenPath(), []byte(time.Now().Format(time.RFC1123)+"\n"), 0o644); err != nil {
		return dieApprove("%s", err)
	}
	fmt.Println("commit-gate: one push authorized")
	return 0
}

// cmdGateEnable implements the `gate-enable` command: point core.hooksPath
// at the live hooks dir, clear any opt-out, and ensure the gate dir and
// manifest exist.
func cmdGateEnable(_ []string) int {
	cwd, _ := os.Getwd()
	repo, err := gatestate.Open(cwd)
	if err != nil {
		return dieApprove("not inside a git repository")
	}
	if err := repo.GitConfigSet("core.hooksPath", liveHooksDir()); err != nil {
		return dieApprove("%s", err)
	}
	if err := repo.GitConfigUnset("commit-gate.disabled"); err != nil {
		return dieApprove("%s", err)
	}
	if err := os.MkdirAll(repo.GateDir(), 0o755); err != nil {
		return dieApprove("%s", err)
	}
	f, err := os.OpenFile(repo.ManifestPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return dieApprove("%s", err)
	}
	f.Close()
	top, err := repo.Toplevel()
	if err != nil {
		return dieApprove("%s", err)
	}
	fmt.Printf("commit-gate: enabled in %s\n", top)
	return 0
}

// cmdGateDisable implements the `gate-disable` command: confirm unless
// --yes, then opt the repo out via commit-gate.disabled.
func cmdGateDisable(args []string) int {
	if code := checkLaunchGuard("gate-disable"); code != 0 {
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

// cmdGateStatus implements the `gate-status` command: report opted-out,
// enabled (with pending approvals + push token state), or undecided.
func cmdGateStatus(_ []string) int {
	cwd, _ := os.Getwd()
	repo, err := gatestate.Open(cwd)
	if err != nil {
		return dieApprove("not inside a git repository")
	}
	if repo.Disabled() {
		fmt.Println("commit-gate: opted-out")
		return 0
	}
	if repo.Enabled(liveHooksDir()) {
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
		if _, err := os.Stat(repo.PushTokenPath()); err == nil {
			fmt.Println("push token: present")
		} else {
			fmt.Println("push token: none")
		}
		return 0
	}
	fmt.Println("commit-gate: undecided (run gate-enable to gate this repo, or gate-disable to opt out)")
	return 0
}

// cmdClearApprovals implements the `clear-approvals` command: truncate the
// manifest and remove the push token.
func cmdClearApprovals(_ []string) int {
	cwd, _ := os.Getwd()
	repo, err := gatestate.Open(cwd)
	if err != nil {
		return dieApprove("not inside a git repository")
	}
	if info, err := os.Stat(repo.GateDir()); err != nil || !info.IsDir() {
		return dieApprove("repo not enabled")
	}
	if err := os.WriteFile(repo.ManifestPath(), []byte{}, 0o644); err != nil {
		return dieApprove("%s", err)
	}
	if err := os.Remove(repo.PushTokenPath()); err != nil && !os.IsNotExist(err) {
		return dieApprove("%s", err)
	}
	fmt.Println("commit-gate: approvals and push token cleared")
	return 0
}
