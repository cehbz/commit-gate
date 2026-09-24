package launchguard

import (
	"errors"
	"strings"
	"testing"
)

// shQuote reproduces POSIX-shell single-quoting: wrap in '...', and turn
// every embedded single quote into the close-escape-reopen concatenation
// (close quote, backslash-quote, reopen quote) the task description calls
// out.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// wrapShellC reproduces the wrapper Claude Code's `!` command runs
// (observed via `ps -o args= -p $$`): a zsh -c script that sources the
// shell snapshot, clears a couple of options/aliases, then `eval`s the
// user's literal text.
func wrapShellC(userText string) []string {
	script := `source /Users/t/.claude/shell-snapshots/snapshot-zsh-abc.sh 2>/dev/null || true && ` +
		`setopt NO_EXTENDED_GLOB NO_BARE_GLOB_QUAL 2>/dev/null || true && ` +
		`{ \builtin unalias -- 'unsetenv'; \builtin unset -f -- 'unsetenv'; } >/dev/null 2>&1 || true && ` +
		`eval ` + shQuote(userText) + ` < /dev/null && pwd -P >| /tmp/claude-c5bc-cwd`
	return []string{"/bin/zsh", "-c", script}
}

func TestDecideRealWrapperAroundApprove(t *testing.T) {
	r := decide("approve", wrapShellC("approve --yes"), nil)
	if !r.Allow {
		t.Fatalf("want allow, got deny: %q", r.Reason)
	}
}

func TestDecideCdThenApprove(t *testing.T) {
	r := decide("approve", []string{"/bin/zsh", "-c", "cd ~/x && approve"}, nil)
	if !r.Allow {
		t.Fatalf("want allow, got deny: %q", r.Reason)
	}
}

func TestDecideCdThenApproveThenApprovePush(t *testing.T) {
	r := decide("approve", []string{"/bin/zsh", "-c", "cd ~/x && approve && approve-push"}, nil)
	if !r.Allow {
		t.Fatalf("want allow, got deny: %q", r.Reason)
	}
}

func TestDecideRefusesInstallShInChain(t *testing.T) {
	r := decide("approve", []string{"/bin/zsh", "-c", "approve && ./install.sh && approve-push"}, nil)
	if r.Allow {
		t.Fatal("want deny")
	}
	if !strings.Contains(r.Reason, "./install.sh") {
		t.Fatalf("reason must name ./install.sh, got %q", r.Reason)
	}
	if !strings.HasPrefix(r.Reason, "commit-gate: approve refuses to run") {
		t.Fatalf("reason has wrong shape: %q", r.Reason)
	}
}

func TestDecideRefusesSemicolonRm(t *testing.T) {
	r := decide("approve", []string{"/bin/zsh", "-c", "approve; rm -rf x"}, nil)
	if r.Allow {
		t.Fatal("want deny")
	}
	if !strings.Contains(r.Reason, "rm") {
		t.Fatalf("reason must name rm, got %q", r.Reason)
	}
}

func TestDecideRefusesPipeToTee(t *testing.T) {
	r := decide("approve", []string{"/bin/zsh", "-c", "approve | tee log"}, nil)
	if r.Allow {
		t.Fatal("want deny")
	}
	if !strings.Contains(r.Reason, "tee") {
		t.Fatalf("reason must name tee, got %q", r.Reason)
	}
}

func TestDecideRefusesCommandSubstitutionArgument(t *testing.T) {
	r := decide("approve", []string{"/bin/zsh", "-c", "approve $(echo x)"}, nil)
	if r.Allow {
		t.Fatal("want deny")
	}
	if r.Reason == "" {
		t.Fatal("want a non-empty reason")
	}
}

func TestDecideAllowsEmbeddedSingleQuoteResolvedThroughEval(t *testing.T) {
	// The user's own command safely quotes its apostrophe in DOUBLE quotes;
	// Claude's outer wrapper then single-quotes the WHOLE text for eval,
	// which escapes that embedded apostrophe as '\''. decide must resolve
	// both layers and still recognize this as a bare `approve`.
	userText := `approve -F "/tmp/it's messy.txt"`
	r := decide("approve", wrapShellC(userText), nil)
	if !r.Allow {
		t.Fatalf("want allow, got deny: %q", r.Reason)
	}
}

func TestDecideInteractiveParentAllows(t *testing.T) {
	r := decide("approve", []string{"-zsh"}, nil)
	if !r.Allow {
		t.Fatalf("want allow, got deny: %q", r.Reason)
	}
	r = decide("approve", []string{"zsh"}, nil)
	if !r.Allow {
		t.Fatalf("want allow, got deny: %q", r.Reason)
	}
	r = decide("approve", []string{"bash"}, nil)
	if !r.Allow {
		t.Fatalf("want allow, got deny: %q", r.Reason)
	}
}

func TestDecideUnreadableParentAllowsWithNote(t *testing.T) {
	r := decide("approve", nil, errors.New("permission denied"))
	if !r.Allow {
		t.Fatalf("want allow (fail open), got deny: %q", r.Reason)
	}
	if !strings.Contains(r.Note, "permission denied") {
		t.Fatalf("note must mention the underlying error, got %q", r.Note)
	}
}

func TestDecideUnparseableScriptRefuses(t *testing.T) {
	r := decide("approve", []string{"/bin/zsh", "-c", "approve && ("}, nil)
	if r.Allow {
		t.Fatal("want deny (fail closed) for an unparseable script")
	}
	if r.Reason == "" {
		t.Fatal("want a non-empty reason")
	}
}

func TestDecideDifferentCmdNameAppearsInMessage(t *testing.T) {
	r := decide("gate-disable", []string{"/bin/zsh", "-c", "gate-disable && rm -rf x"}, nil)
	if r.Allow {
		t.Fatal("want deny")
	}
	if !strings.Contains(r.Reason, "gate-disable refuses to run") {
		t.Fatalf("reason must name the guarded command, got %q", r.Reason)
	}
}

// Check wires GetParentArgv (the injectable seam) through to decide.
func TestCheckUsesGetParentArgvSeam(t *testing.T) {
	orig := GetParentArgv
	defer func() { GetParentArgv = orig }()

	GetParentArgv = func() ([]string, error) {
		return []string{"/bin/zsh", "-c", "approve && ./install.sh"}, nil
	}
	r := Check("approve")
	if r.Allow {
		t.Fatal("want deny")
	}
	if !strings.Contains(r.Reason, "./install.sh") {
		t.Fatalf("reason must name ./install.sh, got %q", r.Reason)
	}

	GetParentArgv = func() ([]string, error) { return []string{"-zsh"}, nil }
	r = Check("approve")
	if !r.Allow {
		t.Fatalf("want allow, got deny: %q", r.Reason)
	}
}

func TestDecideAllowsPathQualifiedCommitGateCommands(t *testing.T) {
	r := decide("approve", []string{"/bin/zsh", "-c", "/Users/t/.local/bin/approve && /Users/t/.local/bin/approve-push"}, nil)
	if !r.Allow {
		t.Fatalf("want allow, got deny: %q", r.Reason)
	}
}
