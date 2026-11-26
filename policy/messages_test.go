package policy

import (
	"strings"
	"testing"
)

func TestReminderPointsAtTheAlwaysLoadedWorkflow(t *testing.T) {
	if !strings.Contains(MsgReminder, "~/.claude/CLAUDE.md") {
		t.Errorf("reminder should name ~/.claude/CLAUDE.md as the workflow source: %q", MsgReminder)
	}
	if strings.Contains(MsgReminder, "projects/commit-gate.md") {
		t.Errorf("reminder still cites the KB node as the protocol: %q", MsgReminder)
	}
}

func TestReminderNamesThePerSessionBatchFile(t *testing.T) {
	for _, want := range []string{"/tmp/cg-batch-<repo>-<sid>", "$CLAUDE_CODE_SESSION_ID", "approve -F"} {
		if !strings.Contains(MsgReminder, want) {
			t.Errorf("reminder should name %q: %q", want, MsgReminder)
		}
	}
	// Shared across sessions: /tmp/cgmsg by every session, cg-pending (read by
	// bare approve) by every session in the repo.
	for _, stale := range []string{"/tmp/cgmsg", "`! approve`"} {
		if strings.Contains(MsgReminder, stale) {
			t.Errorf("reminder still names the shared %q: %q", stale, MsgReminder)
		}
	}
}

func TestReminderHandsOverTheCommitGateCommands(t *testing.T) {
	for _, want := range []string{"`! commit-gate approve -F /tmp/cg-batch-<repo>-<sid>`", "commit-gate approve-push"} {
		if !strings.Contains(MsgReminder, want) {
			t.Errorf("reminder should name %q: %q", want, MsgReminder)
		}
	}
	for _, old := range []string{"! approve -F", "! approve-push"} {
		if strings.Contains(MsgReminder, old) {
			t.Errorf("reminder still names the old command %q: %q", old, MsgReminder)
		}
	}
}

func TestTheHumanOnlyDenialNamesTheCommitGateCommands(t *testing.T) {
	for _, want := range []string{"commit-gate approve", "disable"} {
		if !strings.Contains(MsgHumanOnly, want) {
			t.Errorf("denial should name %q: %q", want, MsgHumanOnly)
		}
	}
	if strings.Contains(MsgHumanOnly, "gate-disable") {
		t.Errorf("denial still names the old gate-disable: %q", MsgHumanOnly)
	}
}
