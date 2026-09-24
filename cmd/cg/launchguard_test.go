// launchguard_test.go end-to-end tests the launch-guard wiring in
// cmdApprove/cmdApprovePush/cmdGateDisable through a REAL shell -c
// invocation (not the launchguard package's injected seam): the compiled
// binary's parent process really is `/bin/sh -c "<script>"`, and the check
// reads that parent's actual argv via the platform syscall path
// (kern.procargs2 on macOS, /proc/<ppid>/cmdline on Linux). This is the
// scenario the feature exists for: an agent-composed `!` command chain
// that also runs something else must be refused before any confirmation
// dialog or state change.
package main_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cehbz/commit-gate/gatestate"
)

// symlinkAs creates a symlink named name (in its own temp dir) pointing at
// the compiled test binary, so dispatch() sees the human-only basename
// (approve, approve-push, gate-disable) the real install.sh layout uses,
// rather than the `commit-gate <subcommand>` dispatch form the rest of this
// test package uses for convenience.
func symlinkAs(t *testing.T, name string) string {
	t.Helper()
	link := filepath.Join(t.TempDir(), name)
	if err := os.Symlink(bin, link); err != nil {
		t.Fatal(err)
	}
	return link
}

// runShellC runs /bin/sh -c script with dir as its working directory and
// stdin as its stdin, returning combined stdout+stderr and the exit code.
func runShellC(t *testing.T, dir, stdin, script string) (string, int) {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), code
}

func skipUnlessSupportedPlatform(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("launchguard's real parent-argv reading is only implemented on darwin/linux")
	}
}

func TestApproveRefusesWhenShellLaunchAlsoRunsInstallSh(t *testing.T) {
	skipUnlessSupportedPlatform(t)
	repo := mkrepoT(t)
	setHooksPath(t, repo)
	approve := symlinkAs(t, "approve")

	script := approve + " --yes && ./install.sh"
	out, code := runShellC(t, repo, "", script)
	if code == 0 {
		t.Fatalf("want non-zero exit, got 0; output=%q", out)
	}
	want := "commit-gate: approve refuses to run in a command that also runs ./install.sh"
	if !strings.Contains(out, want) {
		t.Fatalf("output must contain %q, got %q", want, out)
	}

	// And no approval must have been recorded (refused before any state
	// change).
	r, _ := gatestate.Open(repo)
	subj, err := gatestate.ApprovedSubjects(r.ManifestPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(subj) != 0 {
		t.Fatalf("want no approvals recorded, got %v", subj)
	}
}

func TestApprovePushRefusesWhenShellLaunchAlsoRunsRm(t *testing.T) {
	skipUnlessSupportedPlatform(t)
	repo := mkrepoT(t)
	setHooksPath(t, repo)
	approvePush := symlinkAs(t, "approve-push")

	// `;` does not short-circuit: rm still runs after approve-push refuses
	// (that's exactly the risk this feature warns about — it cannot stop a
	// sibling statement from running, only refuse its own state change).
	// Capture approve-push's own exit code via $ec rather than the whole
	// script's, which `;` makes rm's.
	script := approvePush + " --yes; ec=$?; rm -rf /tmp/should-not-run; exit $ec"
	out, code := runShellC(t, repo, "", script)
	if code == 0 {
		t.Fatalf("want non-zero exit, got 0; output=%q", out)
	}
	if !strings.Contains(out, "commit-gate: approve-push refuses to run in a command that also runs rm") {
		t.Fatalf("unexpected output: %q", out)
	}

	r, _ := gatestate.Open(repo)
	if _, err := os.Stat(r.PushTokenPath()); !os.IsNotExist(err) {
		t.Fatalf("push token must not have been written")
	}
}

func TestGateDisableRefusesWhenPipedToTee(t *testing.T) {
	skipUnlessSupportedPlatform(t)
	repo := mkrepoT(t)
	gateDisable := symlinkAs(t, "gate-disable")

	// A pipe starts both sides concurrently regardless of either's exit
	// code, so the overall script's exit code here is tee's, not
	// gate-disable's; what matters is that gate-disable refused (message)
	// and made no state change (checked below).
	script := gateDisable + " --yes | tee /tmp/should-not-run.log"
	out, _ := runShellC(t, repo, "", script)
	if !strings.Contains(out, "commit-gate: gate-disable refuses to run in a command that also runs tee") {
		t.Fatalf("unexpected output: %q", out)
	}
	if got := gitGetConfig(t, repo, "commit-gate.disabled"); got != "" {
		t.Fatalf("commit-gate.disabled must not have been set, got %q", got)
	}
}

// Positive control: a real `cd <repo> && approve` shell launch (the
// documented, supported pattern) must still work end-to-end through the
// real parent-argv syscall path, not just the launchguard package's fake
// seam.
func TestApproveAllowedWhenShellLaunchIsJustCdAndApprove(t *testing.T) {
	skipUnlessSupportedPlatform(t)
	repo := mkrepoT(t)
	setHooksPath(t, repo)
	approve := symlinkAs(t, "approve")
	elsewhere := t.TempDir()

	script := "cd " + repo + " && " + approve + " --yes"
	out, code := runShellC(t, elsewhere, "feat: shell-launched\n", script)
	if code != 0 {
		t.Fatalf("want exit 0, got %d; output=%q", code, out)
	}

	r, err := gatestate.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	subj, err := gatestate.ApprovedSubjects(r.ManifestPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(subj) != 1 || subj[0] != "feat: shell-launched" {
		t.Fatalf("want [feat: shell-launched], got %v", subj)
	}
}
