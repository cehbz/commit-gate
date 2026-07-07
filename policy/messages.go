package policy

const (
	MsgNoVerify     = `commit-gate: --no-verify/-n is not allowed for the agent. Get approval, or the human can run '! git commit --no-verify'.`
	MsgDisabled     = `commit-gate: the agent may not opt a repo out (commit-gate.disabled is human-only).`
	MsgHooksPath    = `commit-gate: the agent may only set core.hooksPath to the gate (enable); not elsewhere or --unset.`
	MsgWriteTarget  = `commit-gate: the agent may not modify gate state, native hooks, or .git/config.`
	MsgHumanOnly    = `commit-gate: the agent may not run approve/approve-push/gate-disable — these are human-only (run via ! or a terminal).`
	MsgUnresolvable = `commit-gate: unresolvable command in a guarded position — use a literal command name.`
	MsgParse        = `commit-gate: couldn't parse; simplify or split the command.`
	MsgEditGate     = `commit-gate: the agent may not edit gate state or scripts.`
	MsgEditHooks    = `commit-gate: the agent may not edit native git hooks.`
	MsgEditConfig   = `commit-gate: the agent may not edit .git/config.`
	MsgEditSettings = `commit-gate: the agent may not edit settings.json.`
	MsgReminder     = "commit-gate is enabled in this repo. Protocol (~/.claude/knowledge/projects/commit-gate.md): do NOT pre-ask. Write the message to a short /tmp path (e.g. /tmp/cgmsg), run `git commit -F <path>` (or push); if rejected it stages cg-pending — THEN show the message in chat and ask the user to run the bare `approve` (or `approve-push`) command. Batch plans: one `approve -F <file>` up front, messages separated by @@COMMIT-GATE-SEP@@."
)
