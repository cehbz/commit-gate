package policy

import "testing"

func TestPrecheckDeniesCommitGateApproveApprovePushAndDisable(t *testing.T) {
	pending(t)
	wantDeny(t, "commit-gate approve --yes -F /tmp/x", MsgHumanOnly)
	wantDeny(t, "commit-gate approve", MsgHumanOnly)
	wantDeny(t, "commit-gate approve-push --yes", MsgHumanOnly)
	wantDeny(t, "commit-gate disable", MsgHumanOnly)
	wantDeny(t, "commit-gate disable --yes", MsgHumanOnly)
	wantDeny(t, "commit-gate 'approve'", MsgHumanOnly)
	wantDeny(t, `commit-gate app"rove"`, MsgHumanOnly)
}

func TestPrecheckDeniesAHumanOnlyCommandAnywhereInTheShellCommand(t *testing.T) {
	pending(t)
	wantDeny(t, "echo hi && commit-gate approve", MsgHumanOnly)
	wantDeny(t, "commit-gate approve; echo done", MsgHumanOnly)
	wantDeny(t, "foo&&commit-gate approve", MsgHumanOnly)
	wantDeny(t, "commit-gate approve|less", MsgHumanOnly)
	wantDeny(t, "cd ~/x && commit-gate approve -F f && commit-gate approve-push", MsgHumanOnly)
	wantDeny(t, "foo\ncommit-gate disable", MsgHumanOnly)
	wantDeny(t, "bash -c 'commit-gate approve --yes'", MsgHumanOnly)
	wantDeny(t, `bash -c "sh -c 'commit-gate disable'"`, MsgHumanOnly)
	wantDeny(t, "(commit-gate approve-push)", MsgHumanOnly)
	wantDeny(t, "echo $(commit-gate approve --yes)", MsgHumanOnly)
}

func TestPrecheckDeniesCommitGateNamedByItsPath(t *testing.T) {
	pending(t)
	wantDeny(t, "/Users/x/.local/bin/commit-gate approve --yes", MsgHumanOnly)
	wantDeny(t, "~/.local/libexec/commit-gate/commit-gate approve-push", MsgHumanOnly)
	wantDeny(t, "~/.local/bin/commit-gate disable", MsgHumanOnly)
	wantDeny(t, "./build/commit-gate approve", MsgHumanOnly)
}

func TestPrecheckRefusesAComputedCommitGateCommandWord(t *testing.T) {
	pending(t)
	wantDeny(t, `commit-gate "$c"`, MsgUnresolvable)
	wantDeny(t, `commit-gate $c --yes`, MsgUnresolvable)
	wantDeny(t, `commit-gate $(echo approve)`, MsgUnresolvable)
	wantDeny(t, "commit-gate `echo approve-push`", MsgUnresolvable)
	wantDeny(t, `/Users/x/.local/bin/commit-gate "$c"`, MsgUnresolvable)
}

func TestPrecheckRefusesAComputedCommandName(t *testing.T) {
	wantDeny(t, "$CMD --yes", MsgUnresolvable)
	wantDeny(t, `"$CG" approve`, MsgUnresolvable)
}

func TestPrecheckAllowsTheOtherCommitGateCommands(t *testing.T) {
	wantSilent(t, "commit-gate status")
	wantSilent(t, "commit-gate enable")
	wantSilent(t, "commit-gate canonical")
	wantSilent(t, "commit-gate clear-approvals")
	wantSilent(t, "commit-gate hook-names")
	wantSilent(t, "printf 'feat: x' | commit-gate canonical")
	wantSilent(t, "/Users/x/.local/bin/commit-gate status")
}

func TestPrecheckLeavesCommitGateCommandWordsInDataAlone(t *testing.T) {
	wantSilent(t, `git log --grep "commit-gate approve"`)
	wantSilent(t, "echo commit-gate approve")
	wantSilent(t, "grep 'commit-gate disable' notes.txt")
	wantSilent(t, "man commit-gate")
	wantSilent(t, "disapprove;foo")
}

// The old names are no longer commands: the precheck judges them as any
// other unknown command.
func TestPrecheckNoLongerDeniesTheOldCommandNames(t *testing.T) {
	pending(t)
	wantSilent(t, "approve --yes -F /tmp/x")
	wantSilent(t, "approve-push --yes")
	wantSilent(t, "gate-disable")
	wantSilent(t, "/Users/x/.local/bin/approve --yes")
}

func TestR5NonLiteralNameNoArgs(t *testing.T) {
	// a command whose entire name is non-literal, with NO other args, must still deny
	wantDeny(t, "$CMD", MsgUnresolvable)
	wantDeny(t, "$(which approve)", MsgUnresolvable)
	wantDeny(t, "`which approve`", MsgUnresolvable)
	wantDeny(t, "$(which approve) > /tmp/out", MsgUnresolvable)
	wantDeny(t, "{gate-disable,x}", MsgUnresolvable)
	// genuine redirect-only carriers must NOT be denied by R5
	wantSilent(t, "{ echo a; } > /tmp/f")
	wantSilent(t, "> /tmp/out")
}
