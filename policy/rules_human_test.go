package policy

import "testing"

func TestR5HumanOnly(t *testing.T) {
	wantDeny(t, "approve --yes -F /tmp/x", MsgHumanOnly)
	wantDeny(t, "approve-push --yes", MsgHumanOnly)
	wantDeny(t, "gate-disable", MsgHumanOnly)
	wantDeny(t, "echo hi && approve", MsgHumanOnly)
	wantDeny(t, "approve; echo done", MsgHumanOnly)
	wantDeny(t, "foo&&approve", MsgHumanOnly)
	wantDeny(t, "approve|less", MsgHumanOnly)
	wantDeny(t, "foo\ngate-disable", MsgHumanOnly)                 // newline statement (behavior change)
	wantDeny(t, "bash -c 'approve --yes'", MsgHumanOnly)           // -c recursion (behavior change)
	wantDeny(t, "/Users/x/.local/bin/approve --yes", MsgHumanOnly) // path-qualified (tightening)
	wantDeny(t, "$CMD --yes", MsgUnresolvable)                     // bare-variable command name
	wantSilent(t, "git log --grep approve")
	wantSilent(t, "grep gate-disable notes.txt")
	wantSilent(t, "clear-approvals")
	wantSilent(t, "disapprove;foo")
	wantSilent(t, "gate-enable")
	wantSilent(t, "echo approve")
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
