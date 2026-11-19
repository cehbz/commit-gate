package policy

import "testing"

func TestR4RedirectTargets(t *testing.T) {
	wantDeny(t, "echo y >> .git/commit-gate/approved", MsgWriteTarget)
	wantDeny(t, "echo y > /repo/.git/config", MsgWriteTarget)
	wantDeny(t, "{ echo a; } >> /x/.git/hooks/pre-commit", MsgWriteTarget)
	wantDeny(t, "echo x > /inst/hooks/commit-msg", MsgWriteTarget) // under install dir
	wantSilent(t, "echo y > /tmp/out")
	wantSilent(t, "echo .git/config") // mention as data
}

func TestR4WriteVerbs(t *testing.T) {
	wantDeny(t, "rm -rf /x/.git/commit-gate", MsgWriteTarget)
	wantDeny(t, "mv /x/.git/hooks/pre-push /tmp/", MsgWriteTarget)
	wantDeny(t, "cp evil /x/.git/hooks/pre-push", MsgWriteTarget)
	wantDeny(t, "tee /x/.git/config < /tmp/x", MsgWriteTarget)
	wantDeny(t, "truncate -s 0 /x/.git/commit-gate/approved", MsgWriteTarget)
	wantDeny(t, "sed -i s/x/y/ /inst/commit-gate", MsgWriteTarget)
	wantSilent(t, "sed s/x/y/ /x/.git/config")       // no -i: read-only sed
	wantSilent(t, "rm -rf /tmp/x; echo .git/config") // verb and mention in different statements
	wantSilent(t, "cat /x/.git/config")              // reads are not writes
	wantSilent(t, `cat <<EOF
rm -rf /x/.git/commit-gate
EOF`) // heredoc content is data (behavior change vs bash)
	wantSilent(t, "git log --oneline -- .git/config") // args of non-write verbs
}

func TestR4SedLongOptionForms(t *testing.T) {
	// GNU =-attached in-place and script/file flags must not bypass R4.
	wantDeny(t, "sed --in-place=bak s/a/b/ /x/.git/config", MsgWriteTarget)
	wantDeny(t, "sed -i --file=script.sed /x/.git/config", MsgWriteTarget)
	wantDeny(t, "sed -i --expression=s/a/b/ /x/.git/config", MsgWriteTarget)
	// still-correct existing behavior (guard against regressions):
	wantDeny(t, "sed -i s/a/b/ /x/.git/config", MsgWriteTarget)
	wantSilent(t, "sed s/a/b/ /x/.git/config") // no in-place: read-only
}

func TestR4GlobalGitConfig(t *testing.T) {
	wantDeny(t, "echo '[core]' >> /home/u/.gitconfig", MsgWriteTarget)
	wantDeny(t, "sed -i /hooksPath/d /home/u/.gitconfig", MsgWriteTarget)
	wantDeny(t, "rm /home/u/.config/git/config", MsgWriteTarget)
	wantDeny(t, "mv /tmp/x /home/u/.gitconfig", MsgWriteTarget)
	wantDeny(t, "cp /tmp/x /home/u/.config/git/config", MsgWriteTarget)
	wantDeny(t, "tee /home/u/.gitconfig < /tmp/x", MsgWriteTarget)
	wantDeny(t, "truncate -s 0 /home/u/.gitconfig", MsgWriteTarget)
	wantSilent(t, "cat /home/u/.gitconfig")                 // reads stay allowed
	wantSilent(t, "sed -n /hooksPath/p /home/u/.gitconfig") // no -i: read-only sed
	wantSilent(t, "echo x > /home/u/.gitconfig.bak")        // not the config file
	wantSilent(t, "echo x > /home/u/.config/git/ignore")    // its neighbour
}

func TestR4HomeVariable(t *testing.T) {
	// $HOME is the agent's ordinary spelling of the home dir, as ~ is.
	wantDeny(t, "echo x >> $HOME/.gitconfig", MsgWriteTarget)
	wantDeny(t, `echo x >> "$HOME/.gitconfig"`, MsgWriteTarget)
	wantDeny(t, "echo x >> ${HOME}/.gitconfig", MsgWriteTarget)
	wantDeny(t, "sed -i /hooksPath/d $HOME/.gitconfig", MsgWriteTarget)
	wantSilent(t, "cat $HOME/.gitconfig")
	wantSilent(t, "echo x >> ${HOME:-/home/u}/.gitconfig.bak")
}

// cp writes only its destination: the last operand, or the directory -t or
// --target-directory names. Its sources are reads. mv removes its source,
// so every argument of mv stays checked.
func TestR4CpChecksOnlyItsDestination(t *testing.T) {
	// copying a protected file elsewhere is a read of it
	wantSilent(t, "cp ~/.gitconfig ~/.gitconfig.bak")
	wantSilent(t, "cp .git/config /tmp/x")
	wantSilent(t, "cp /home/u/.config/git/config /tmp/x")
	wantSilent(t, "cp -r /x/.git/hooks /tmp/h")
	wantSilent(t, "cp /inst/commit-gate /tmp/cg")
	wantSilent(t, "cp -t /tmp /home/u/.gitconfig")
	wantSilent(t, "cp -t/tmp /home/u/.gitconfig")
	wantSilent(t, "cp -pt /tmp /home/u/.gitconfig")
	wantSilent(t, "cp --target-directory=/tmp /x/.git/config")
	wantSilent(t, "cp --target-directory /tmp /x/.git/config")
	wantSilent(t, "cp --target-directory=$HOME/bak /x/.git/config")
	wantSilent(t, "cp -t$HOME/bak /home/u/.gitconfig")
	wantSilent(t, "cp -t$BAK ~/.gitconfig")
	wantSilent(t, `cp -t"$BAK" ~/.gitconfig`)
	wantSilent(t, "cp -t$(pwd) /home/u/.gitconfig")
	wantSilent(t, "cp -T /home/u/.gitconfig /tmp/x")
	wantSilent(t, "cp -S .orig /home/u/.gitconfig /tmp/x")
	wantSilent(t, "cp --suffix .orig /home/u/.gitconfig /tmp/x")
	wantSilent(t, "cp --backup=numbered /home/u/.gitconfig /tmp/x")
	// copying onto one is a write
	wantDeny(t, "cp x ~/.gitconfig", MsgWriteTarget)
	wantDeny(t, "cp x /r/.git/hooks/pre-push", MsgWriteTarget)
	wantDeny(t, "cp ~/.gitconfig.bak ~/.gitconfig", MsgWriteTarget)
	wantDeny(t, "cp -t /inst x", MsgWriteTarget)
	wantDeny(t, "cp -t/inst x", MsgWriteTarget)
	wantDeny(t, "cp -rt /x/.git/hooks pre-push", MsgWriteTarget)
	wantDeny(t, "cp --target-directory=/inst x", MsgWriteTarget)
	wantDeny(t, "cp --target-directory /x/.git/hooks pre-push", MsgWriteTarget)
	wantDeny(t, "cp --target-directory=$HOME/r/.git/hooks pre-push", MsgWriteTarget)
	wantDeny(t, "cp --target-directory=${HOME}/r/.git/hooks pre-push", MsgWriteTarget)
	wantDeny(t, "cp -t$HOME/r/.git/hooks pre-push", MsgWriteTarget)
	wantDeny(t, "cp -rt$HOME/r/.git/hooks pre-push", MsgWriteTarget)
	wantDeny(t, `cp "--target-directory=$HOME/r/.git/commit-gate" x`, MsgWriteTarget)
	wantDeny(t, "cp -T /tmp/x /home/u/.gitconfig", MsgWriteTarget)
	wantDeny(t, "cp -S .bak /tmp/x /home/u/.gitconfig", MsgWriteTarget)
	wantDeny(t, "cp -fS.bak /tmp/x /home/u/.gitconfig", MsgWriteTarget)
	wantDeny(t, "cp --suffix .bak /tmp/x /home/u/.gitconfig", MsgWriteTarget)
	wantDeny(t, "cp --suffix=.bak /tmp/x /home/u/.gitconfig", MsgWriteTarget)
	wantDeny(t, "cp --sparse always /tmp/x /home/u/.gitconfig", MsgWriteTarget)
	wantDeny(t, "cp --no-preserve mode /tmp/x /home/u/.gitconfig", MsgWriteTarget)
	wantDeny(t, "cp --backup=numbered /tmp/x /home/u/.gitconfig", MsgWriteTarget)
	wantDeny(t, "cp /tmp/x /home/u/.gitconfig -v", MsgWriteTarget)
	wantDeny(t, "cp -- /tmp/x /home/u/.gitconfig", MsgWriteTarget)
	// BSD cp ends options at the first operand: its last word is the target
	wantDeny(t, "cp /tmp/x -S /home/u/.gitconfig", MsgWriteTarget)
	// mv still checks its source
	wantDeny(t, "mv /home/u/.gitconfig /tmp/x", MsgWriteTarget)
	wantDeny(t, "mv -t /tmp /x/.git/hooks/pre-push", MsgWriteTarget)
	// and its target directory in every spelling
	wantDeny(t, "mv -t /r/.git/hooks x", MsgWriteTarget)
	wantDeny(t, "mv -t/r/.git/hooks x", MsgWriteTarget)
	wantDeny(t, "mv -ft/r/.git/hooks x", MsgWriteTarget)
	wantDeny(t, "mv --target-directory=/r/.git/hooks x", MsgWriteTarget)
	wantDeny(t, "mv --target-directory /r/.git/hooks x", MsgWriteTarget)
	wantDeny(t, "mv -t$HOME/r/.git/hooks x", MsgWriteTarget)
	wantDeny(t, "mv --target-directory=${HOME}/r/.git/hooks x", MsgWriteTarget)
	wantSilent(t, "mv -t/tmp x")
}

// An attached $HOME target names the install dir under the home directory;
// an attached expansion after -S leaves the destination the last operand.
func TestR4CpAttachedValueForms(t *testing.T) {
	ctx := fakeCtx()
	ctx.InstallDir = "/home/u/.local/libexec/commit-gate"
	d := Decide(ToolCall{ToolName: "Bash", Command: "cp -t$HOME/.local/libexec/commit-gate x", CWD: "/w"}, ctx)
	if d.Kind != Deny || d.Reason != MsgWriteTarget {
		t.Errorf("cp -t$HOME/<install dir>: got %+v", d)
	}
	wantDeny(t, "cp -S$SUF x ~/.gitconfig", MsgWriteTarget)
	wantDeny(t, `cp -S"$SUF" x /home/u/.gitconfig`, MsgWriteTarget)
	wantSilent(t, "cp -S$SUF ~/.gitconfig /tmp/x")
}
