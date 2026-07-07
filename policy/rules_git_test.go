package policy

import "testing"

func TestR1NoVerify(t *testing.T) {
	wantDeny(t, "git commit --no-verify -m x", MsgNoVerify)
	wantDeny(t, "git commit -n -m x", MsgNoVerify)
	wantDeny(t, "cd /x && git commit -n", MsgNoVerify)
	wantSilent(t, "git commit -many -m x")                 // -many is not -n
	wantSilent(t, "echo -n hi")                            // not git commit
	wantSilent(t, "git status; echo commit -n")            // cross-statement: no contamination
	wantSilent(t, "echo --no-verify")                      // word in another invocation
	wantSilent(t, `git commit -m "use --no-verify never"`) // string content is data... -m VALUE is an arg; see note
}

func TestR2Disabled(t *testing.T) {
	wantDeny(t, "git config commit-gate.disabled true", MsgDisabled)
	wantDeny(t, "git config --unset commit-gate.disabled", MsgDisabled)
	wantDeny(t, "git -c commit-gate.disabled=true commit -F /tmp/m", MsgDisabled)
	wantSilent(t, "git config --get commit-gate.disabled") // reads allowed (behavior change)
	wantSilent(t, "grep commit-gate.disabled notes.txt")   // mention elsewhere is data
}

func TestR3HooksPath(t *testing.T) {
	wantDeny(t, "git config core.hooksPath /tmp/evil", MsgHooksPath)
	wantDeny(t, "git config core.hooksPath /inst/hooks/sub", MsgHooksPath) // prefix bypass
	wantDeny(t, "git config --unset core.hooksPath", MsgHooksPath)
	wantDeny(t, "git config core.hooksPath $DIR", MsgHooksPath) // unresolvable value in guarded position
	wantDeny(t, "git -c core.hooksPath=/tmp/evil commit -F /tmp/m", MsgHooksPath)
	wantSilent(t, "git config core.hooksPath /inst/hooks") // enable
	wantSilent(t, "git -c core.hooksPath=/inst/hooks commit -F /tmp/m")
	wantSilent(t, "git config --get core.hooksPath") // reads allowed (behavior change)
}

func TestR2R3FlagBoundaryBypasses(t *testing.T) {
	// -C's value token "config" must not shift the subcommand (bash caught these).
	wantDeny(t, "git -C config config core.hooksPath /evil", MsgHooksPath)
	wantDeny(t, "git -C config config commit-gate.disabled true", MsgDisabled)
	// --file/-f consume their value; the real key must still be seen.
	wantDeny(t, "git config --file /tmp/x commit-gate.disabled true", MsgDisabled)
	wantDeny(t, "git config -f /tmp/x core.hooksPath /evil", MsgHooksPath)
}

func TestR2R3CaseInsensitive(t *testing.T) {
	// git treats section/key names case-insensitively; the gate must too.
	wantDeny(t, "git config Core.HooksPath /evil", MsgHooksPath)
	wantDeny(t, "git config COMMIT-GATE.DISABLED true", MsgDisabled)
	wantDeny(t, "git -c Commit-Gate.Disabled=true commit -F /tmp/m", MsgDisabled)
}
