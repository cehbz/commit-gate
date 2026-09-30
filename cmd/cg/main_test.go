package main_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cehbz/commit-gate/gatestate"
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

// sessionOut decodes sessioncheck's SessionStart JSON.
type sessionOut struct {
	SystemMessage      string `json:"systemMessage"`
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

func runSessioncheck(t *testing.T, dir string) (string, sessionOut) {
	t.Helper()
	out, _, code := run(t, `{"cwd":"`+dir+`"}`, "sessioncheck")
	if code != 0 {
		t.Fatalf("sessioncheck exit %d, out=%q", code, out)
	}
	var so sessionOut
	if out != "" {
		if err := json.Unmarshal([]byte(out), &so); err != nil {
			t.Fatalf("bad JSON %q: %v", out, err)
		}
	}
	return out, so
}

func TestSessioncheckEnablesUndecidedRepo(t *testing.T) {
	repo := mkrepoT(t)
	top := showToplevel(t, repo)
	_, so := runSessioncheck(t, repo)
	if want := "commit-gate: enabled in " + top + " (default). To opt out: gate-disable"; so.SystemMessage != want {
		t.Fatalf("systemMessage:\ngot:  %q\nwant: %q", so.SystemMessage, want)
	}
	ctx := so.HookSpecificOutput.AdditionalContext
	if so.HookSpecificOutput.HookEventName != "SessionStart" ||
		!strings.Contains(ctx, "enabled by default") || !strings.Contains(ctx, "gate-disable") || !strings.Contains(ctx, top) {
		t.Fatalf("additionalContext: %+v", so.HookSpecificOutput)
	}

	wantHooks, _ := filepath.EvalSymlinks(filepath.Join(binDir, "hooks"))
	if got, _ := filepath.EvalSymlinks(gitGetConfig(t, repo, "core.hooksPath")); got != wantHooks {
		t.Fatalf("core.hooksPath resolves to %q, want %q", got, wantHooks)
	}
	r, err := gatestate.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.ManifestPath()); err != nil {
		t.Fatalf("manifest must exist: %v", err)
	}

	if out, _ := runSessioncheck(t, repo); out != "" {
		t.Fatalf("now-enabled repo must be silent: %q", out)
	}
}

func TestSessioncheckLeavesOptedOutRepo(t *testing.T) {
	repo := mkrepoT(t)
	gitSetConfig(t, repo, "commit-gate.disabled", "true")
	if out, _ := runSessioncheck(t, repo); out != "" {
		t.Fatalf("opted-out repo must be silent: %q", out)
	}
	if hp := gitGetConfig(t, repo, "core.hooksPath"); hp != "" {
		t.Fatalf("opted-out repo must stay untouched, core.hooksPath=%q", hp)
	}
	if v := gitGetConfig(t, repo, "commit-gate.disabled"); v != "true" {
		t.Fatalf("opt-out must survive, commit-gate.disabled=%q", v)
	}
}

func TestSessioncheckReportsEnableFailure(t *testing.T) {
	repo := mkrepoT(t)
	top := showToplevel(t, repo)
	r, err := gatestate.Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.GateDir(), nil, 0o644); err != nil { // a file where the gate dir goes
		t.Fatal(err)
	}
	_, so := runSessioncheck(t, repo)
	if !strings.HasPrefix(so.SystemMessage, "commit-gate: could not enable in "+top+":") {
		t.Fatalf("systemMessage: %q", so.SystemMessage)
	}
	ctx := so.HookSpecificOutput.AdditionalContext
	if !strings.Contains(ctx, "could not be enabled") || !strings.Contains(ctx, "denied") {
		t.Fatalf("additionalContext: %q", ctx)
	}
}

func TestSessioncheckNonRepoSilent(t *testing.T) {
	if out, _ := runSessioncheck(t, t.TempDir()); out != "" {
		t.Fatalf("non-repo must be silent: %q", out)
	}
}
