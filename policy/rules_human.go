package policy

import "github.com/cehbz/commit-gate/shellscan"

// R5: approve / approve-push / gate-disable are human-only — the agent may
// never invoke them itself (they must be run via `!` or a human terminal).
// Path-qualified invocations count (deliberate tightening vs. bash, which
// matched on bare command word only): shellscan.Basename strips the
// directory, so /Users/x/.local/bin/approve is caught too.
var humanOnly = map[string]bool{"approve": true, "approve-push": true, "gate-disable": true}

func ruleHumanOnly(inv shellscan.Invocation, _ ToolCall, _ Ctx) (Decision, bool) {
	if !inv.HasCommand {
		return Decision{}, false // synthetic redirect-only carrier, not a real command
	}
	if !inv.Name.Literal {
		return deny(MsgUnresolvable), true
	}
	if humanOnly[shellscan.Basename(inv.Name)] {
		return deny(MsgHumanOnly), true
	}
	return Decision{}, false
}
