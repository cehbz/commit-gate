package main_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var (
	binDir string
	bin    string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "cgbin")
	if err != nil {
		panic(err)
	}
	bin = filepath.Join(dir, "commit-gate")
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		os.RemoveAll(dir)
		panic(string(out))
	}
	os.Mkdir(filepath.Join(dir, "hooks"), 0o755)
	for _, l := range []string{"precheck", "sessioncheck"} {
		os.Symlink(bin, filepath.Join(dir, l))
	}
	os.Symlink(bin, filepath.Join(dir, "hooks", "commit-msg"))
	os.Symlink(bin, filepath.Join(dir, "hooks", "pre-push"))
	binDir = dir
	// os.Exit does not run deferred funcs; remove the temp tree explicitly.
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func run(t *testing.T, stdin string, prog string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(filepath.Join(binDir, prog), args...)
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

// mkrepoT duplicates gatestate's mkrepo test helper for this external test package.
func mkrepoT(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", append([]string{"-C", d}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return d
}

func TestPrecheckDenyAndSilentAndExit0(t *testing.T) {
	out, _, code := run(t, `{"tool_name":"Bash","tool_input":{"command":"approve"}}`, "precheck")
	if code != 0 || !strings.Contains(out, `"permissionDecision":"deny"`) {
		t.Fatalf("deny: code=%d out=%q", code, out)
	}
	out, _, code = run(t, `{"tool_name":"Bash","tool_input":{"command":"git log --grep approve"}}`, "precheck")
	if code != 0 || out != "" {
		t.Fatalf("silent: code=%d out=%q", code, out)
	}
	out, _, code = run(t, `{not json`, "precheck")
	if code != 0 || !strings.Contains(out, `"deny"`) {
		t.Fatalf("bad json must fail closed: code=%d out=%q", code, out)
	}
}

func TestSessioncheckUndecidedRepo(t *testing.T) {
	repo := mkrepoT(t) // same helper pattern as gatestate tests; add it to this file
	in := `{"cwd":"` + repo + `"}`
	out, _, code := run(t, in, "sessioncheck")
	if code != 0 || !strings.Contains(out, "commit-gate is not configured in this repository") {
		t.Fatalf("undecided: code=%d out=%q", code, out)
	}
	out, _, _ = run(t, `{"cwd":"`+t.TempDir()+`"}`, "sessioncheck")
	if out != "" {
		t.Fatalf("non-repo must be silent: %q", out)
	}
}
