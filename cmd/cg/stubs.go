package main

// TEMPORARY STUBS — Tasks 11-13 replace these with the real management/hook
// command implementations and DELETE this file. Each stub prints
// "commit-gate: not implemented" to stderr and returns exit code 1 so the
// dispatcher wires up cleanly while the real commands are still pending.
// Do NOT build on these; they intentionally do nothing.

import (
	"fmt"
	"os"
)

func notImplemented() int {
	fmt.Fprintln(os.Stderr, "commit-gate: not implemented")
	return 1
}

func cmdApprove(_ []string) int        { return notImplemented() } // Task 11
func cmdApprovePush(_ []string) int    { return notImplemented() } // Task 11
func cmdGateEnable(_ []string) int     { return notImplemented() } // Task 13
func cmdGateDisable(_ []string) int    { return notImplemented() } // Task 13
func cmdGateStatus(_ []string) int     { return notImplemented() } // Task 13
func cmdClearApprovals(_ []string) int { return notImplemented() } // Task 13
