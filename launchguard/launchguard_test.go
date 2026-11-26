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
	pending(t)
	r := decide("approve", wrapShellC("commit-gate approve --yes"), nil)
	if !r.Allow {
		t.Fatalf("want allow, got deny: %q", r.Reason)
	}
}

func TestDecideCdThenApprove(t *testing.T) {
	pending(t)
	r := decide("approve", []string{"/bin/zsh", "-c", "cd ~/x && commit-gate approve"}, nil)
	if !r.Allow {
		t.Fatalf("want allow, got deny: %q", r.Reason)
	}
}

func TestDecideCdThenApproveThenApprovePush(t *testing.T) {
	pending(t)
	r := decide("approve", []string{"/bin/zsh", "-c", "cd x && commit-gate approve -F f && commit-gate approve-push"}, nil)
	if !r.Allow {
		t.Fatalf("want allow, got deny: %q", r.Reason)
	}
}

func TestDecideRefusesInstallShInChain(t *testing.T) {
	pending(t)
	r := decide("approve", []string{"/bin/zsh", "-c", "commit-gate approve && ./install.sh && commit-gate approve-push"}, nil)
	if r.Allow {
		t.Fatal("want deny")
	}
	if !strings.Contains(r.Reason, "also runs ./install.sh —") {
		t.Fatalf("reason must name ./install.sh alone, got %q", r.Reason)
	}
	if !strings.HasPrefix(r.Reason, "commit-gate: approve refuses to run") {
		t.Fatalf("reason has wrong shape: %q", r.Reason)
	}
}

func TestDecideRefusesSemicolonRm(t *testing.T) {
	pending(t)
	r := decide("approve", []string{"/bin/zsh", "-c", "commit-gate approve; rm -rf x"}, nil)
	if r.Allow {
		t.Fatal("want deny")
	}
	if !strings.Contains(r.Reason, "also runs rm —") {
		t.Fatalf("reason must name rm alone, got %q", r.Reason)
	}
}

func TestDecideRefusesPipeToTee(t *testing.T) {
	pending(t)
	r := decide("approve", []string{"/bin/zsh", "-c", "commit-gate approve | tee log"}, nil)
	if r.Allow {
		t.Fatal("want deny")
	}
	if !strings.Contains(r.Reason, "also runs tee —") {
		t.Fatalf("reason must name tee alone, got %q", r.Reason)
	}
}

func TestDecideRefusesCommandSubstitutionArgument(t *testing.T) {
	r := decide("approve", []string{"/bin/zsh", "-c", "commit-gate approve $(echo x)"}, nil)
	if r.Allow {
		t.Fatal("want deny")
	}
	if r.Reason == "" {
		t.Fatal("want a non-empty reason")
	}
}

func TestDecideAllowsEmbeddedSingleQuoteResolvedThroughEval(t *testing.T) {
	pending(t)
	// The user's own command safely quotes its apostrophe in DOUBLE quotes;
	// Claude's outer wrapper then single-quotes the WHOLE text for eval,
	// which escapes that embedded apostrophe as '\''. decide must resolve
	// both layers and still recognize this as a bare `commit-gate approve`.
	userText := `commit-gate approve -F "/tmp/it's messy.txt"`
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
	r := decide("approve", []string{"/bin/zsh", "-c", "commit-gate approve && ("}, nil)
	if r.Allow {
		t.Fatal("want deny (fail closed) for an unparseable script")
	}
	if r.Reason == "" {
		t.Fatal("want a non-empty reason")
	}
}

func TestDecideDifferentCmdNameAppearsInMessage(t *testing.T) {
	r := decide("disable", []string{"/bin/zsh", "-c", "commit-gate disable && rm -rf x"}, nil)
	if r.Allow {
		t.Fatal("want deny")
	}
	if !strings.Contains(r.Reason, "disable refuses to run") {
		t.Fatalf("reason must name the guarded command, got %q", r.Reason)
	}
}

// Check wires GetParentArgv (the injectable seam) through to decide.
func TestCheckUsesGetParentArgvSeam(t *testing.T) {
	orig := GetParentArgv
	defer func() { GetParentArgv = orig }()

	GetParentArgv = func() ([]string, error) {
		return []string{"/bin/zsh", "-c", "commit-gate approve && ./install.sh"}, nil
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
	pending(t)
	r := decide("approve", []string{"/bin/zsh", "-c", "/Users/t/.local/bin/commit-gate approve && ~/.local/libexec/commit-gate/commit-gate approve-push"}, nil)
	if !r.Allow {
		t.Fatalf("want allow, got deny: %q", r.Reason)
	}
}

func TestDecideAllowsAnyCommitGateCommandBesideCd(t *testing.T) {
	pending(t)
	script := "cd ~/x && commit-gate status && commit-gate clear-approvals && commit-gate approve -F f && commit-gate enable && commit-gate approve-push"
	r := decide("approve", []string{"/bin/zsh", "-c", script}, nil)
	if !r.Allow {
		t.Fatalf("want allow, got deny: %q", r.Reason)
	}
}

// The old names are not commit-gate: a chain that runs one runs something
// else.
func TestDecideRefusesAnOldCommandNameInTheChain(t *testing.T) {
	pending(t)
	for _, old := range []string{"approve-push", "approve", "gate-status", "clear-approvals"} {
		r := decide("approve", []string{"/bin/zsh", "-c", "cd x && commit-gate approve && " + old}, nil)
		if r.Allow {
			t.Fatalf("%s: want deny", old)
		}
		if !strings.Contains(r.Reason, "also runs "+old+" —") {
			t.Fatalf("reason must name %s alone, got %q", old, r.Reason)
		}
	}
}

func TestDecideRefusesAComputedCommitGateCommandWord(t *testing.T) {
	pending(t)
	r := decide("approve", []string{"/bin/zsh", "-c", `commit-gate approve && commit-gate "$c"`}, nil)
	if r.Allow {
		t.Fatal("want deny")
	}
	if !strings.Contains(r.Reason, "commit-gate <dynamic argument>") {
		t.Fatalf("reason must name the computed argument, got %q", r.Reason)
	}
}
