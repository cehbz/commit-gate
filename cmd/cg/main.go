// Command commit-gate is the single binary behind every hook and command of
// the gate. Run as commit-gate, its first argument is a command word
// (approve, approve-push, clear-approvals, status, enable, disable,
// hook-names, canonical). Run under any other name, that name is a hook:
// precheck and sessioncheck for Claude Code, and in the hooks/ directory
// commit-msg and pre-push, which enforce, and every other hook git runs,
// which passes through to the repo's own .git/hooks/<name>. The source
// package lives at cmd/cg but the built binary is always named
// "commit-gate"; dispatch reads filepath.Base(os.Args[0]), never the source
// directory.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const commandUsage = "usage: commit-gate <approve|approve-push|clear-approvals|status|enable|disable|hook-names|canonical> ..."

func main() {
	os.Exit(dispatch(os.Args))
}

// dispatch runs a command when the binary is run as commit-gate, and the
// hook its name names otherwise.
func dispatch(args []string) int {
	name := filepath.Base(args[0])
	if name == "commit-gate" {
		return runCommand(args[1:])
	}
	return runHook(name, args[1:])
}

// runCommand runs the command word args[0] with the rest of args. A hook's
// name is not a command word.
func runCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, commandUsage)
		return 2
	}
	word, rest := args[0], args[1:]
	switch word {
	case "approve":
		return cmdApprove(rest)
	case "approve-push":
		return cmdApprovePush(rest)
	case "clear-approvals":
		return cmdClearApprovals(rest)
	case "status":
		return cmdStatus(rest)
	case "enable":
		return cmdEnable(rest)
	case "disable":
		return cmdDisable(rest)
	case "hook-names":
		return cmdHookNames()
	case "canonical":
		return cmdCanonical()
	}
	return unknownCommand(word)
}

// runHook runs the hook name: Claude Code's precheck and sessioncheck, git's
// commit-msg and pre-push checks, or a pass-through hook. A command word is
// not a hook's name.
func runHook(name string, args []string) int {
	switch name {
	case "precheck":
		return cmdPrecheck()
	case "sessioncheck":
		return cmdSessioncheck()
	case "commit-msg":
		return cmdCommitMsg(args)
	case "pre-push":
		return cmdPrePush(args)
	}
	if isPassThrough(name) {
		return cmdPassThrough(name, args)
	}
	return unknownCommand(name)
}

// unknownCommand reports an unknown command word or link name with the usage
// line and returns exit status 2.
func unknownCommand(name string) int {
	fmt.Fprintf(os.Stderr, "commit-gate: unknown command %q\n%s\n", name, commandUsage)
	return 2
}
