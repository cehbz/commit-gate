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
