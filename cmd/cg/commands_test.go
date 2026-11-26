// commands_test.go covers the Command: every command is a word after the one
// command commit-gate, and runs in no other form.
package main_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cehbz/commit-gate/gatestate"
)

// runPath runs the program at path with cwd dir.
func runPath(t *testing.T, dir, stdin, path string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(path, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), errb.String(), code
}

// assertUnknownCommand: the run exited 2 naming an unknown command, and left
// the repo without an approval, a push authorization or an opt-out.
func assertUnknownCommand(t *testing.T, repo, what, stderr string, code int) {
	t.Helper()
	if code != 2 || !strings.Contains(stderr, "unknown command") {
		t.Errorf("%s: want exit 2 and an unknown command, got code=%d stderr=%q", what, code, stderr)
	}
	r, err := gatestate.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	if subj, _ := gatestate.ApprovedSubjects(r.ManifestPath()); len(subj) != 0 {
		t.Errorf("%s: recorded approvals %v", what, subj)
	}
	if _, err := os.Stat(r.PushTokenPath()); !os.IsNotExist(err) {
		t.Errorf("%s: wrote a push authorization", what)
	}
	if v := gitGetConfig(t, repo, "commit-gate.disabled"); v != "" {
		t.Errorf("%s: opted the repo out", what)
	}
}

func TestALinkNamedAfterACommandRunsNoCommand(t *testing.T) {
	pending(t)
	for _, name := range []string{
		"approve", "approve-push", "clear-approvals", "gate-enable", "gate-disable", "gate-status",
		"canonical", "hook-names", "status", "enable", "disable",
	} {
		repo := mkrepoT(t)
		link := filepath.Join(t.TempDir(), name)
		if err := os.Symlink(bin, link); err != nil {
			t.Fatal(err)
		}
		_, errStr, code := runPath(t, repo, "feat: by link\n", link, "--yes")
		assertUnknownCommand(t, repo, "a link named "+name, errStr, code)
	}
}

func TestTheOldCommandNamesAreNotCommandWords(t *testing.T) {
	pending(t)
	for _, word := range []string{"gate-enable", "gate-disable", "gate-status"} {
		repo := mkrepoT(t)
		_, errStr, code := runIn(t, repo, "", "commit-gate", word, "--yes")
		assertUnknownCommand(t, repo, "commit-gate "+word, errStr, code)
	}
}

func TestCommitGateHasNoHookForm(t *testing.T) {
	pending(t)
	msgFile := filepath.Join(t.TempDir(), "MSG")
	if err := os.WriteFile(msgFile, []byte("feat: through hook\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"hook", "approve", "--yes"},
		{"hook", "approve-push", "--yes"},
		{"hook", "commit-msg", msgFile},
		{"hook", "pre-commit"},
	} {
		repo := mkrepoT(t)
		_, errStr, code := runIn(t, repo, "feat: through hook\n", "commit-gate", args...)
		assertUnknownCommand(t, repo, "commit-gate "+strings.Join(args, " "), errStr, code)
	}
}

func TestCommitGateWithNoCommandNamesEveryCommandAndNoOldName(t *testing.T) {
	pending(t)
	_, errStr, code := run(t, "", "commit-gate")
	if code == 0 {
		t.Fatalf("commit-gate with no command must fail, got exit 0; stderr=%q", errStr)
	}
	words := map[string]bool{}
	for _, w := range regexp.MustCompile(`[a-z][a-z-]*`).FindAllString(errStr, -1) {
		words[w] = true
	}
	for _, c := range []string{"approve", "approve-push", "clear-approvals", "status", "enable", "disable", "hook-names", "canonical"} {
		if !words[c] {
			t.Errorf("usage must name %q: %q", c, errStr)
		}
	}
	for _, old := range []string{"gate-enable", "gate-disable", "gate-status"} {
		if words[old] {
			t.Errorf("usage must not name %q: %q", old, errStr)
		}
	}
}

func TestAHookNameAfterCommitGateIsAnUnknownCommand(t *testing.T) {
	pending(t)
	msgFile := filepath.Join(t.TempDir(), "MSG")
	if err := os.WriteFile(msgFile, []byte("feat: as a word\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"precheck"},
		{"sessioncheck"},
		{"commit-msg", msgFile},
		{"pre-push", "origin", "url"},
		{"pre-commit"},
		{"post-checkout", "0", "0", "1"},
	} {
		repo := mkrepoT(t)
		_, errStr, code := runIn(t, repo, "", "commit-gate", args...)
		assertUnknownCommand(t, repo, "commit-gate "+strings.Join(args, " "), errStr, code)
	}
}

// A copy of the binary under another name, or go run's cg, runs no command:
// command words are dispatched only under the name commit-gate.
func TestTheBinaryUnderAnotherNameRunsNoCommand(t *testing.T) {
	b, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	copied := filepath.Join(dir, "b1")
	if err := os.WriteFile(copied, b, 0o755); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(dir, "cg")
	if err := os.Symlink(bin, linked); err != nil {
		t.Fatal(err)
	}
	for _, prog := range []string{copied, linked} {
		for _, args := range [][]string{{"approve", "--yes"}, {"approve-push", "--yes"}, {"disable", "--yes"}, {"status"}} {
			repo := mkrepoT(t)
			_, errStr, code := runPath(t, repo, "feat: by another name\n", prog, args...)
			assertUnknownCommand(t, repo, filepath.Base(prog)+" "+strings.Join(args, " "), errStr, code)
		}
	}
}
