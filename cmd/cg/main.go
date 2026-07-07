// Command commit-gate is the single dispatch binary for the commit-gate tool.
// It is invoked either as `commit-gate <subcommand> [args]` or, in symlink mode,
// via a basename equal to one of the subcommand names (precheck, sessioncheck,
// commit-msg, pre-push, ...). The source package lives at cmd/cg but the built
// binary is always named "commit-gate"; dispatch reads filepath.Base(os.Args[0]),
// never the source directory.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	os.Exit(dispatch(os.Args))
}

func dispatch(args []string) int {
	name := filepath.Base(args[0])
	rest := args[1:]
	if name == "commit-gate" {
		if len(rest) == 0 {
			fmt.Fprintln(os.Stderr, "usage: commit-gate <precheck|sessioncheck|approve|approve-push|gate-enable|gate-disable|gate-status|clear-approvals|hook> ...")
			return 2
		}
		name, rest = rest[0], rest[1:]
		if name == "hook" {
			if len(rest) == 0 {
				fmt.Fprintln(os.Stderr, "usage: commit-gate hook <commit-msg|pre-push> ...")
				return 2
			}
			name, rest = rest[0], rest[1:]
		}
	}
	switch name {
	case "precheck":
		return cmdPrecheck()
	case "sessioncheck":
		return cmdSessioncheck()
	case "commit-msg":
		return cmdCommitMsg(rest)
	case "pre-push":
		return cmdPrePush(rest)
	case "approve":
		return cmdApprove(rest)
	case "approve-push":
		return cmdApprovePush(rest)
	case "gate-enable":
		return cmdGateEnable(rest)
	case "gate-disable":
		return cmdGateDisable(rest)
	case "gate-status":
		return cmdGateStatus(rest)
	case "clear-approvals":
		return cmdClearApprovals(rest)
	default:
		fmt.Fprintf(os.Stderr, "commit-gate: unknown command %q\n", name)
		return 2
	}
}
