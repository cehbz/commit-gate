package launchguard

// MsgDeny is the refusal a human-confirmed command (commit-gate approve,
// approve-push, disable) prints when the shell command that launched it
// runs anything besides cd and commit-gate. Args: the command word being
// guarded, the comma-joined names of what else the launch chain runs, and
// the command word again.
const MsgDeny = "commit-gate: %s refuses to run in a command that also runs %s — run commit-gate %s on its own (cd is fine), and run the rest separately after reading it."

// MsgParseErr replaces MsgDeny when the launch command could be read but
// not parsed: fail closed rather than guess at its contents. Args: the
// command being guarded, twice.
const MsgParseErr = "commit-gate: %s refuses to run in a command commit-gate could not parse — run commit-gate %s on its own (cd is fine), and run the rest separately after reading it."

// MsgUnreadableNote is a one-line, non-fatal stderr note printed when the
// parent process's argv could not be read at all (permission, platform):
// the launch-chain check is skipped and the guarded command proceeds (fail
// open — this check guards against blind pastes, not an adversary; the
// existing confirm dialog still gates the action). Arg: the underlying
// error.
const MsgUnreadableNote = "commit-gate: launch-chain check skipped (could not read parent process: %v)"
