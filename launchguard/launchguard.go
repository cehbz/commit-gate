// Package launchguard protects the human-only commands (approve,
// approve-push, gate-disable) against blind pastes: an agent-composed `!`
// command chain such as `approve && ./install.sh && approve-push` would, if
// pasted and confirmed once, run everything in the chain without individual
// review. Check inspects the argv of the shell that launched this process
// (its parent, read via os.Getppid()) and refuses to proceed when that
// launch command contains anything other than `cd` and commit-gate's own
// commands.
//
// This is not a defense against an adversary who controls the parent
// process's argv — it guards against an honest human pasting a chain they
// didn't fully read. Failures reading the parent's argv therefore fail
// open (the existing human-confirmation dialog still gates the action);
// failures parsing a launch script that WAS read fail closed.
package launchguard

import (
	"fmt"
	"strings"

	"github.com/cehbz/commit-gate/shellscan"
)

// allowed is the set of command basenames a human-confirmed command's
// launch chain may contain, besides itself. cd is allowed so `cd ~/repo &&
// approve` keeps working; the rest are commit-gate's own commands.
var allowed = map[string]bool{
	"cd":              true,
	"approve":         true,
	"approve-push":    true,
	"clear-approvals": true,
	"gate-status":     true,
	"gate-enable":     true,
	"gate-disable":    true,
}

// Result is the outcome of Check. Note and Reason are fully formatted,
// ready to print as-is.
type Result struct {
	Allow  bool
	Note   string // non-empty: an informational line to print regardless of Allow
	Reason string // non-empty when !Allow: print this and refuse
}

// GetParentArgv is the injectable seam returning the exact argv of the
// process that launched this one (os.Getppid()). Production wiring is
// platform-specific: parentargv_darwin.go (sysctl kern.procargs2),
// parentargv_linux.go (/proc/<ppid>/cmdline), parentargv_other.go (stub).
var GetParentArgv = defaultGetParentArgv

// Check inspects the launch command of the process running cmdName (one of
// "approve", "approve-push", "gate-disable") and reports whether it may
// proceed.
func Check(cmdName string) Result {
	argv, err := GetParentArgv()
	return decide(cmdName, argv, err)
}

// decide is the pure decision function behind Check, taking the parent
// argv (and any error reading it) directly so it can be table-tested
// without touching the GetParentArgv seam.
func decide(cmdName string, argv []string, argvErr error) Result {
	if argvErr != nil {
		return Result{Allow: true, Note: fmt.Sprintf(MsgUnreadableNote, argvErr)}
	}
	script, hasScript := scriptFromArgv(argv)
	if !hasScript {
		return Result{Allow: true} // interactive shell: no -c script to inspect
	}
	checked := script
	if payload, ok := evalLiteralPayload(script); ok {
		checked = payload
	}
	bad, parseErr := disallowed(checked)
	if parseErr {
		return Result{Reason: fmt.Sprintf(MsgParseErr, cmdName, cmdName)}
	}
	if len(bad) == 0 {
		return Result{Allow: true}
	}
	return Result{Reason: fmt.Sprintf(MsgDeny, cmdName, strings.Join(dedupe(bad), ", "), cmdName)}
}

// scriptFromArgv finds the shell -c script in argv, if any. Its absence
// means an interactive shell ("-zsh", "zsh", "bash", ...): there is no
// launch command to inspect.
func scriptFromArgv(argv []string) (string, bool) {
	for i, a := range argv {
		if a == "-c" && i+1 < len(argv) {
			return argv[i+1], true
		}
	}
	return "", false
}

// disallowed walks every simple command in checked (via shellscan, which
// already flattens &&/||/;/pipelines/subshells/blocks and recurses into
// command substitutions as ordinary statements) and reports the raw text
// of every one that is not an allowed commit-gate command, plus a marker
// for any unresolvable command name or argument (command substitution,
// parameter expansion, ...) even on an otherwise-allowed command: e.g. `cd
// $(approve)` must not slip through just because "approve" is allowed.
func disallowed(checked string) (bad []string, parseErr bool) {
	res := shellscan.Scan(checked)
	if res.ParseErr {
		return nil, true
	}
	for _, inv := range res.Invocations {
		if !inv.HasCommand {
			continue // synthetic redirect-only carrier
		}
		if !inv.Name.Literal {
			bad = append(bad, "<dynamically-computed command>")
			continue
		}
		bn := shellscan.Basename(inv.Name)
		if !allowed[bn] {
			bad = append(bad, inv.Name.Text)
			continue
		}
		for _, a := range inv.Args {
			if !a.Literal {
				bad = append(bad, fmt.Sprintf("%s <dynamic argument>", bn))
				break
			}
		}
	}
	return bad, false
}

// dedupe preserves order while dropping repeats.
func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
