package policy

import "testing"

func fileCall(tool, path string) ToolCall {
	return ToolCall{ToolName: tool, FilePath: path, CWD: "/w"}
}

func TestR6FileTools(t *testing.T) {
	cases := []struct {
		path, msg string
	}{
		{"/x/.git/commit-gate/approved", MsgEditGate},
		{"/inst/hooks/commit-msg", MsgEditGate},
		{"/x/.git/hooks/commit-msg", MsgEditHooks},
		{"/x/.git/config", MsgEditConfig},
		{"/Users/h/.claude/settings.json", MsgEditSettings},
	}
	for _, c := range cases {
		for _, tool := range []string{"Write", "Edit", "MultiEdit", "NotebookEdit"} {
			d := Decide(fileCall(tool, c.path), fakeCtx())
			if d.Kind != Deny || d.Reason != c.msg {
				t.Errorf("%s %s: %+v want %q", tool, c.path, d, c.msg)
			}
		}
	}
	// The gate SOURCE repo is not protected (boundary change vs bash).
	for _, p := range []string{"/Users/h/projects/commit-gate/policy/policy.go", "/x/src/main.go", ""} {
		if d := Decide(fileCall("Write", p), fakeCtx()); d.Kind != Silent {
			t.Errorf("Write %q: %+v want silent", p, d)
		}
	}
	if d := Decide(ToolCall{ToolName: "Grep"}, fakeCtx()); d.Kind != Silent {
		t.Errorf("unknown tool: %+v", d)
	}
}

func TestR7Reminder(t *testing.T) {
	gated := fakeCtx()
	gated.GatedCWD = true
	for _, cmd := range []string{"git commit -F /tmp/m", "git push origin main", "cd /r && git commit -F /tmp/m"} {
		d := Decide(ToolCall{ToolName: "Bash", Command: cmd, CWD: "/w"}, gated)
		if d.Kind != AllowContext || d.Reason != MsgReminder {
			t.Errorf("%q: %+v", cmd, d)
		}
	}
	// deny wins over reminder
	d := Decide(ToolCall{ToolName: "Bash", Command: "git commit -n -F /tmp/m", CWD: "/w"}, gated)
	if d.Kind != Deny {
		t.Errorf("deny must beat reminder: %+v", d)
	}
	// not gated, or not commit/push: silent
	if d := Decide(ToolCall{ToolName: "Bash", Command: "git commit -F /tmp/m"}, fakeCtx()); d.Kind != Silent {
		t.Errorf("ungated reminder: %+v", d)
	}
	if d := Decide(ToolCall{ToolName: "Bash", Command: "git status", CWD: "/w"}, gated); d.Kind != Silent {
		t.Errorf("git status: %+v", d)
	}
}
