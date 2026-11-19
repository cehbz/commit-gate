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

	// push --no-verify skips the pre-push hook (the only push approval gate).
	wantDeny(t, "git push --no-verify origin main", MsgNoVerify)
	wantDeny(t, "git push --no-verify", MsgNoVerify)
	// push -n is --dry-run (harmless), NOT no-verify; must not be denied.
	wantSilent(t, "git push -n origin main")
	wantSilent(t, "git push origin main")
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
	wantDeny(t, "git -c core.hooksPath=/dev/null commit -m x", MsgHooksPath) // a machine committer's bypass, denied to the agent
	wantSilent(t, "git config core.hooksPath /inst/hooks")                   // enable
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

func TestREnvConfigInjection(t *testing.T) {
	wantDeny(t, "GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=commit-gate.disabled GIT_CONFIG_VALUE_0=true git commit -F /tmp/m", MsgEnvConfig)
	wantDeny(t, "GIT_CONFIG_KEY_0=core.hooksPath git commit -F /tmp/m", MsgEnvConfig)
	wantDeny(t, "GIT_CONFIG_GLOBAL=/tmp/evil git push origin main", MsgEnvConfig)
	wantSilent(t, "FOO=bar git status")         // non-GIT_CONFIG env is fine
	wantSilent(t, "GIT_CONFIG_COUNT=1 echo hi") // not a git invocation
}

func TestR2R3CaseInsensitive(t *testing.T) {
	// git treats section/key names case-insensitively; the gate must too.
	wantDeny(t, "git config Core.HooksPath /evil", MsgHooksPath)
	wantDeny(t, "git config COMMIT-GATE.DISABLED true", MsgDisabled)
	wantDeny(t, "git -c Commit-Gate.Disabled=true commit -F /tmp/m", MsgDisabled)
}

func TestR3GlobalScopeAndSectionOps(t *testing.T) {
	// --global is a flag like any other: these were R3's already.
	wantDeny(t, "git config --global core.hooksPath /tmp/evil", MsgHooksPath)
	wantDeny(t, "git config --global --unset core.hooksPath", MsgHooksPath)
	wantDeny(t, "git config --global --unset-all core.hooksPath", MsgHooksPath)
	wantSilent(t, "git config --global core.hooksPath /inst/hooks") // enable
	wantSilent(t, "git config --global --get core.hooksPath")
	// The core section holds core.hooksPath: removing or renaming it drops
	// the value, and renaming another section to core can set it.
	wantDeny(t, "git config --global --remove-section core", MsgHooksPath)
	wantDeny(t, "git config --global --remove-section Core", MsgHooksPath)
	wantDeny(t, "git config --global --rename-section core old", MsgHooksPath)
	wantDeny(t, "git config --rename-section stash core", MsgHooksPath)
	wantSilent(t, "git config --global --remove-section alias")
	// A value-taking flag's value is not the key.
	wantDeny(t, "git config --global --comment why core.hooksPath /tmp/evil", MsgHooksPath)
}

func TestR2R3ConfigSubcommands(t *testing.T) {
	// git 2.46+: the subcommand is the first argument after config.
	wantDeny(t, "git config set --global core.hooksPath /tmp/evil", MsgHooksPath)
	wantDeny(t, "git config unset --global core.hooksPath", MsgHooksPath)
	wantDeny(t, "git config unset --global --value /inst/hooks core.hooksPath", MsgHooksPath)
	wantDeny(t, "git config remove-section --global core", MsgHooksPath)
	wantDeny(t, "git config rename-section --global core old", MsgHooksPath)
	wantDeny(t, "git config set commit-gate.disabled true", MsgDisabled)
	wantDeny(t, "git config unset commit-gate.disabled", MsgDisabled)
	// -t is --type's short form: its value is not the key, in either syntax.
	wantDeny(t, "git config --global -t path core.hooksPath /tmp/evil", MsgHooksPath)
	wantDeny(t, "git config set --global -t path core.hooksPath /tmp/evil", MsgHooksPath)
	wantDeny(t, "git config -t bool commit-gate.disabled true", MsgDisabled)
	wantDeny(t, "git config set -t bool commit-gate.disabled true", MsgDisabled)
	wantSilent(t, "git config set --global core.hooksPath /inst/hooks") // enable
	wantSilent(t, "git config get --global core.hooksPath")
	wantSilent(t, "git config get commit-gate.disabled")
	wantSilent(t, "git config list --global")
}

// A --file or -f naming the global git config, in any spelling of the path
// or the flag, is the global scope: the forms that drop or redirect
// core.hooksPath there are denied, and reads stay allowed. So is one naming
// .git/config.
func TestR3FileFlagNamingAProtectedConfig(t *testing.T) {
	for _, f := range []string{
		"--file=$HOME/.gitconfig", `"--file=$HOME/.gitconfig"`, "--file=${HOME}/.gitconfig",
		"--file $HOME/.gitconfig", "--file ${HOME}/.gitconfig", "--file ~/.gitconfig",
		"--file=/home/u/.gitconfig", "-f $HOME/.gitconfig", "-f$HOME/.gitconfig", "-f ~/.gitconfig",
		"--file=$XDG_CONFIG_HOME/git/config", "--file=/home/u/.config/git/config",
		"-f /home/u/.config/git/config", "--file=.git/config", "-f .git/config", "-f /r/.git/config",
	} {
		wantDeny(t, "git config "+f+" --unset core.hooksPath", MsgHooksPath)
		wantDeny(t, "git config "+f+" --unset-all core.hooksPath", MsgHooksPath)
		wantDeny(t, "git config "+f+" --remove-section core", MsgHooksPath)
		wantDeny(t, "git config "+f+" --rename-section core old", MsgHooksPath)
		wantDeny(t, "git config "+f+" core.hooksPath /evil", MsgHooksPath)
		wantDeny(t, "git config unset "+f+" core.hooksPath", MsgHooksPath)
		wantDeny(t, "git config remove-section "+f+" core", MsgHooksPath)
		wantDeny(t, "git config set "+f+" core.hooksPath /evil", MsgHooksPath)
		wantDeny(t, "git config "+f+" commit-gate.disabled true", MsgDisabled)
		wantSilent(t, "git config "+f+" --get core.hooksPath")
		wantSilent(t, "git config get "+f+" core.hooksPath")
		wantSilent(t, "git config "+f+" --list")
		wantSilent(t, "git config "+f+" core.hooksPath /inst/hooks") // the enable form
	}
}
