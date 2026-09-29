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

func TestReminderNamesThePerRepoBatchFile(t *testing.T) {
	if !strings.Contains(MsgReminder, "/tmp/cg-batch-<repo>") {
		t.Errorf("reminder should name the per-repo batch file /tmp/cg-batch-<repo>: %q", MsgReminder)
	}
}
