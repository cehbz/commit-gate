package policy

import "testing"

// The agent installs only through a deploy: commit-gate approve-deploy and
// install are human-only, while version, make release and make deploy stay
// the agent's (DESIGN.md: Precheck, Protected set).
func TestPrecheckDeniesApproveDeployAndInstallAndLeavesVersionReleaseAndDeployToTheAgent(t *testing.T) {
	pending(t)
	wantDeny(t, "commit-gate approve-deploy", MsgHumanOnly)
	wantDeny(t, "commit-gate approve-deploy --yes", MsgHumanOnly)
	wantDeny(t, "commit-gate install", MsgHumanOnly)
	wantDeny(t, "~/.local/libexec/commit-gate/staging/commit-gate install", MsgHumanOnly)
	wantDeny(t, "/Users/x/.local/bin/commit-gate approve-deploy", MsgHumanOnly)
	wantDeny(t, "./build/commit-gate install", MsgHumanOnly)
	wantDeny(t, "cd ~/projects/commit-gate && commit-gate approve-deploy && make deploy", MsgHumanOnly)
	wantDeny(t, "bash -c 'commit-gate install'", MsgHumanOnly)
	wantDeny(t, "commit-gate 'install'", MsgHumanOnly)
	wantSilent(t, "commit-gate version")
	wantSilent(t, "make release VERSION=v1.2.0")
	wantSilent(t, "make deploy HOST=example")
	wantSilent(t, "make deploy")
	wantSilent(t, "git log --grep 'commit-gate install'")
}
