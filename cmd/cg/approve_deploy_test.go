// approve_deploy_test.go covers the Deploy authorization: commit-gate
// approve-deploy records one in an enabled repo after confirmation, behind
// the launch guard, as approve-push records a push authorization. Tests pass
// --yes, since confirmation would open a real dialog.
package main_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/cehbz/commit-gate/gatestate"
)

// deployAuthorized reports whether a deploy authorization is live in repo.
func deployAuthorized(t *testing.T, repo string) bool {
	t.Helper()
	r, err := gatestate.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(r.DeployTokenPath())
	return err == nil
}

func TestApproveDeployRecordsADeployAuthorizationInAnEnabledRepo(t *testing.T) {
	pending(t)
	repo := mkrepoT(t)
	_, errStr, code := runIn(t, repo, "", "commit-gate", "approve-deploy", "--yes")
	if code != 0 {
		t.Fatalf("commit-gate approve-deploy: code=%d stderr=%q", code, errStr)
	}
	if !deployAuthorized(t, repo) {
		t.Fatal("no deploy authorization recorded")
	}
}

func TestApproveDeployRefusesInARepoThatIsNotEnabled(t *testing.T) {
	pending(t)
	optedOut := mkrepoT(t)
	gitSetConfig(t, optedOut, "commit-gate.disabled", "true")
	overridden := mkrepoT(t)
	gitSetConfig(t, overridden, "core.hooksPath", t.TempDir())
	for name, repo := range map[string]string{"opted out": optedOut, "overridden": overridden} {
		_, errStr, code := runIn(t, repo, "", "commit-gate", "approve-deploy", "--yes")
		if code == 0 {
			t.Errorf("%s: approve-deploy must refuse, got exit 0; stderr=%q", name, errStr)
		}
		if deployAuthorized(t, repo) {
			t.Errorf("%s: a deploy authorization was recorded", name)
		}
	}
}

func TestApproveDeployRefusesOutsideARepository(t *testing.T) {
	pending(t)
	_, errStr, code := runIn(t, t.TempDir(), "", "commit-gate", "approve-deploy", "--yes")
	if code != 1 || errStr != "commit-gate: not inside a git repository\n" {
		t.Fatalf("code=%d stderr=%q", code, errStr)
	}
}

// The paste the launch guard exists for: the authorization and the deploy it
// admits in one chain.
func TestApproveDeployRefusesWhenTheShellLaunchAlsoRunsMakeDeploy(t *testing.T) {
	pending(t)
	skipUnlessSupportedPlatform(t)
	repo := mkrepoT(t)
	out, code := runShellC(t, repo, "", bin+" approve-deploy --yes && make deploy")
	if code == 0 {
		t.Fatalf("want non-zero exit, got 0; output=%q", out)
	}
	want := "approve-deploy refuses to run in a command that also runs make — run commit-gate approve-deploy on its own"
	if !strings.Contains(out, want) {
		t.Fatalf("output must contain %q, got %q", want, out)
	}
	if deployAuthorized(t, repo) {
		t.Fatal("a refused approve-deploy recorded a deploy authorization")
	}
}

func TestApproveDeployRunsWhenTheShellLaunchIsCdAndCommitGateAlone(t *testing.T) {
	pending(t)
	skipUnlessSupportedPlatform(t)
	repo := mkrepoT(t)
	out, code := runShellC(t, t.TempDir(), "", "cd "+repo+" && "+bin+" approve-deploy --yes")
	if code != 0 {
		t.Fatalf("want exit 0, got %d; output=%q", code, out)
	}
	if !deployAuthorized(t, repo) {
		t.Fatal("no deploy authorization recorded")
	}
}

func TestCommitGateWithNoCommandNamesVersionInstallAndApproveDeploy(t *testing.T) {
	pending(t)
	_, errStr, code := run(t, "", "commit-gate")
	if code == 0 {
		t.Fatalf("commit-gate with no command must fail, got exit 0; stderr=%q", errStr)
	}
	words := map[string]bool{}
	for _, w := range regexp.MustCompile(`[a-z][a-z-]*`).FindAllString(errStr, -1) {
		words[w] = true
	}
	for _, c := range []string{"version", "install", "approve-deploy"} {
		if !words[c] {
			t.Errorf("usage must name %q: %q", c, errStr)
		}
	}
}

func TestApproveLeavesALiveDeployAuthorizationLive(t *testing.T) {
	pending(t)
	repo := mkrepoT(t)
	if _, errStr, code := runIn(t, repo, "", "commit-gate", "approve-deploy", "--yes"); code != 0 {
		t.Fatalf("commit-gate approve-deploy: code=%d stderr=%q", code, errStr)
	}
	if _, errStr, code := runIn(t, repo, "feat: later\n", "commit-gate", "approve", "--yes"); code != 0 {
		t.Fatalf("commit-gate approve: code=%d stderr=%q", code, errStr)
	}
	if !deployAuthorized(t, repo) {
		t.Fatal("a new approval voided the deploy authorization")
	}
}
