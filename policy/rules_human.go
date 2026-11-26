package policy

import "github.com/cehbz/commit-gate/shellscan"

// R5: commit-gate approve, approve-push and disable are human-only: the
// agent may not run them itself (the user runs them via `!` or a terminal).
// commit-gate counts on PATH or by its path, since shellscan.Basename strips
// the directory. A command word the precheck can't resolve
// (`commit-gate "$c"`) is refused, as a computed command name is.
var humanOnly = map[string]bool{"approve": true, "approve-push": true, "disable": true}

func ruleHumanOnly(inv shellscan.Invocation, _ ToolCall, _ Ctx) (Decision, bool) {
	if !inv.HasCommand {
		return Decision{}, false // synthetic redirect-only carrier, not a real command
	}
	if !inv.Name.Literal {
		return deny(MsgUnresolvable), true
	}
	if shellscan.Basename(inv.Name) != "commit-gate" || len(inv.Args) == 0 {
		return Decision{}, false
	}
	word := inv.Args[0]
	if !word.Literal {
		return deny(MsgUnresolvable), true
	}
	if humanOnly[word.Text] {
		return deny(MsgHumanOnly), true
	}
	return Decision{}, false
}
