package harness

import (
	"bytes"
	"strings"
	"testing"
)

func TestDenyShape(t *testing.T) {
	got := Deny(`x "quoted" y`)
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"x \"quoted\" y"}}` + "\n"
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestAllowContextShape(t *testing.T) {
	got := AllowContext("ctx")
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","additionalContext":"ctx"}}` + "\n"
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSessionContextShape(t *testing.T) {
	got := SessionContext("c")
	want := `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"c"}}` + "\n"
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSessionNoticeShape(t *testing.T) {
	got := SessionNotice("c", `u "q" && <x>`)
	want := `{"systemMessage":"u \"q\" && <x>","hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"c"}}` + "\n"
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestParseInput(t *testing.T) {
	in := `{"tool_name":"Bash","tool_input":{"command":"git status"},"cwd":"/x"}`
	got, err := ParseInput(strings.NewReader(in))
	if err != nil || got.ToolName != "Bash" || got.ToolInput.Command != "git status" || got.CWD != "/x" {
		t.Fatalf("got %+v err %v", got, err)
	}
	// Write-tool shape and notebook fallback
	in2 := `{"tool_name":"NotebookEdit","tool_input":{"notebook_path":"/n.ipynb"}}`
	got2, _ := ParseInput(strings.NewReader(in2))
	if got2.Path() != "/n.ipynb" {
		t.Fatalf("Path() = %q", got2.Path())
	}
	// Garbage input is an error, not a panic
	if _, err := ParseInput(bytes.NewReader([]byte("{not json"))); err == nil {
		t.Fatal("want error on bad json")
	}
}

func TestEmptyStringsKeepMandatedKeys(t *testing.T) {
	cases := []struct{ got, want string }{
		{string(Deny("")), `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":""}}` + "\n"},
		{string(AllowContext("")), `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","additionalContext":""}}` + "\n"},
		{string(SessionContext("")), `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":""}}` + "\n"},
	}
	for i, c := range cases {
		if c.got != c.want {
			t.Errorf("case %d: got %q want %q", i, c.got, c.want)
		}
	}
}

func TestNoHTMLEscaping(t *testing.T) {
	got := string(Deny(`a && b <x> "q"`))
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"a && b <x> \"q\""}}` + "\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPathPrefersFilePath(t *testing.T) {
	in := PreToolUseInput{ToolInput: ToolInput{FilePath: "/a", NotebookPath: "/b"}}
	if in.Path() != "/a" {
		t.Fatalf("Path() = %q", in.Path())
	}
}
