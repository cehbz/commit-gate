package policy

import "github.com/cehbz/commit-gate/shellscan"

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
		return decideFileTool(tc, ctx) // Task 9; stub returns silent() until then
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
	return reminderOrSilent(res, tc, ctx) // Task 9 fills in R7; until then return silent()
}

// TODO(task-9): reminderOrSilent is a stub — replace with R7 (AllowContext reminder
// on a real git commit/push in a gated repo). Currently always silent().
func reminderOrSilent(_ shellscan.Result, _ ToolCall, _ Ctx) Decision {
	return silent()
}

// TODO(task-9): decideFileTool is a stub — replace with R6 (file-tool write-target
// checks for gate state, native hooks, .git/config, settings.json). Currently
// always silent().
func decideFileTool(_ ToolCall, _ Ctx) Decision {
	return silent()
}
