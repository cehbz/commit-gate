package main

// overrideText describes a core.hooksPath above the global one that names
// something other than the gate, and how to gate the repo, for commit-gate
// status and sessioncheck alike: commit-gate enable clears only a repo-local
// value.
type overrideText struct {
	value  string // the value, or "empty"
	where  string // "" when repo-local, else ", from <scope> config…"
	fix    string // the step that gates the repo, for sessioncheck's user
	clears string // the step, as "<step> clears it", for commit-gate status
	model  string // the override, as the model is told it
	told   string // what the user was told, as the model is told it
}

func describeOverride(hp, scope string) overrideText {
	t := overrideText{value: hp}
	if hp == "" {
		t.value = "empty"
	}
	switch scope {
	case "worktree":
		t.where = ", from worktree config"
		t.fix = "git config --worktree --unset core.hooksPath"
		t.clears = t.fix + " clears it"
		t.model = "its worktree core.hooksPath (" + t.value + ")"
		t.told = "The user was told `" + t.fix + "` gates it, stopping the hooks it names, and `commit-gate disable` opts out. Do not run either without their explicit yes."
	case "command":
		t.where = ", from command config: git -c or GIT_CONFIG_* in the environment"
		t.fix = "remove it from the environment Claude Code runs in"
		t.clears = "removing it from the environment clears it"
		t.model = "a core.hooksPath from git -c or GIT_CONFIG_* in the session's environment (" + t.value + ")"
		t.told = "The user was told to remove it from the environment Claude Code runs in, and that `commit-gate disable` opts out."
	default: // local
		t.fix = "commit-gate enable (clears that core.hooksPath)"
		t.clears = "commit-gate enable clears it"
		t.model = "its repo-local core.hooksPath (" + t.value + ")"
		t.told = "The user was told `commit-gate enable` gates it, clearing that core.hooksPath and so stopping the hooks it names, and `commit-gate disable` opts out. Do not run `commit-gate enable` without their explicit yes."
	}
	return t
}

// status is commit-gate status's line for the override.
func (t overrideText) status() string {
	return "commit-gate: overridden (core.hooksPath is " + t.value + t.where + "; " + t.clears + ", commit-gate disable opts out)"
}

// notice is sessioncheck's notice for the override in the repo at top: to
// the model (additionalContext) and to the user (systemMessage).
func (t overrideText) notice(top string) (ctx, user string) {
	return "commit-gate does not gate this repository (" + top + "): " + t.model +
			" overrides the gate, which counts as a missing gate: do not commit or push here. " + t.told,
		"commit-gate: " + top + " is ungated: its core.hooksPath is " + t.value + t.where +
			". To gate it: " + t.fix + ". To opt out: commit-gate disable"
}
