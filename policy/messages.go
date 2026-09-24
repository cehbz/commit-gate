package policy

const (
	MsgNoVerify     = `commit-gate: --no-verify is not allowed for the agent. Get approval, or the human can re-run the command with --no-verify via '!'.`
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
	MsgEnvConfig    = `commit-gate: git config injection via GIT_CONFIG_* environment is not allowed for the agent.`
	MsgReminder     = "commit-gate is enabled in this repo. Workflow: ~/.claude/CLAUDE.md, \"Commits, pushes and publishing\". Commit first (`git commit -F /tmp/cgmsg`, no pre-ask); a rejection stages cg-pending: then show the message in chat and hand the user a paste-ready `! approve` (or `! approve-push`). Everything pending goes in one `/tmp/cg-batch` for `! approve -F /tmp/cg-batch`, messages separated by @@COMMIT-GATE-SEP@@."
)
