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
	MsgReminder     = "commit-gate is enabled in this repo. Workflow: ~/.claude/CLAUDE.md, \"Commits, pushes and publishing\". Messages go in this session's batch file `/tmp/cg-batch-<repo>-<sid>` (the repo directory's name; the last four hex digits of $CLAUDE_CODE_SESSION_ID), separated by lines reading @@COMMIT-GATE-SEP@@. Commit first (`git commit -F` with the message from that file, no pre-ask); on rejection show the messages in chat and hand the user a paste-ready `! approve -F /tmp/cg-batch-<repo>-<sid>` with the path spelled out (then `! approve-push` to push). Delete the file once its last message has committed."
)
