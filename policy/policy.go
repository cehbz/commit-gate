package policy

import (
	"strings"

	"github.com/cehbz/commit-gate/shellscan"
)

type ToolCall struct {
	ToolName string
	Command  string // Bash
	FilePath string // Write/Edit/MultiEdit/NotebookEdit
	CWD      string
}

type Ctx struct {
	InstallDir string                        // resolved dir of the live binary
	HooksDir   string                        // resolved InstallDir/hooks
	GatedCWD   bool                          // cwd's repo is gated (computed by caller)
	Resolve    func(cwd, path string) string // abs+symlink resolution; injected (pure fake in tests)
}

type Kind int

const (
	Silent Kind = iota
	Deny
	AllowContext
)

type Decision struct {
	Kind   Kind
	Reason string // deny reason, or additionalContext text
}

func deny(msg string) Decision   { return Decision{Kind: Deny, Reason: msg} }
func silent() Decision           { return Decision{Kind: Silent} }
func remind(msg string) Decision { return Decision{Kind: AllowContext, Reason: msg} }

type bashRule func(inv shellscan.Invocation, tc ToolCall, ctx Ctx) (Decision, bool)

var bashRules = []bashRule{ruleNoVerify, ruleDisabledKey, ruleHooksPath, ruleWriteTargets, ruleHumanOnly}

func Decide(tc ToolCall, ctx Ctx) Decision {
	switch tc.ToolName {
	case "Bash":
		return decideBashCmd(tc, ctx)
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		return decideFileTool(tc, ctx) // R6
	default:
		return silent()
	}
}

func decideBashCmd(tc ToolCall, ctx Ctx) Decision {
	res := shellscan.Scan(tc.Command)
	if res.ParseErr {
		return deny(MsgParse)
	}
	for _, inv := range res.Invocations {
		for _, rule := range bashRules {
			if d, hit := rule(inv, tc, ctx); hit {
				return d
			}
		}
	}
	return reminderOrSilent(res, tc, ctx) // R7
}

// R7: a reminder (AllowContext, non-blocking) on a real git commit/push in a
// gated repo. Deny from R1-R5 always wins — this only runs when no bash rule
// already denied the command.
func reminderOrSilent(res shellscan.Result, tc ToolCall, ctx Ctx) Decision {
	if !ctx.GatedCWD {
		return silent()
	}
	for _, inv := range res.Invocations {
		if sub, ok := shellscan.GitSubcommand(inv); ok && (sub == "commit" || sub == "push") {
			return remind(MsgReminder)
		}
	}
	return silent()
}

// R6: Write/Edit/MultiEdit/NotebookEdit write-target checks. settings.json is
// checked before protectedClass since it isn't part of that (Bash-oriented)
// classification. The gate SOURCE repo is deliberately not protected here —
// only installed artifacts (install dir, gate state, native hooks, git config).
func decideFileTool(tc ToolCall, ctx Ctx) Decision {
	if tc.FilePath == "" {
		return silent()
	}
	resolved := ctx.Resolve(tc.CWD, tc.FilePath)
	parts := strings.Split(resolved, "/")
	if len(parts) >= 2 && parts[len(parts)-1] == "settings.json" && parts[len(parts)-2] == ".claude" {
		return deny(MsgEditSettings)
	}
	switch protectedClass(resolved, ctx) {
	case "install", "gatestate":
		return deny(MsgEditGate)
	case "githooks":
		return deny(MsgEditHooks)
	case "gitconfig":
		return deny(MsgEditConfig)
	}
	return silent()
}
