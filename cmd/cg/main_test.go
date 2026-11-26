package main_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cehbz/commit-gate/internal/testenv"
)

var (
	binDir string
	bin    string
)

func TestMain(m *testing.M) {
	testenv.ClearCallerRepo()
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
	names, err := exec.Command(bin, "hook-names").Output()
	if err != nil {
		os.RemoveAll(dir)
		panic(err)
	}
	for _, h := range strings.Fields(string(names)) {
		os.Symlink(bin, filepath.Join(dir, "hooks", h))
	}
	binDir = dir
	// Every git the suite runs, and every git the gate binary runs, reads this
	// global config instead of the user's: core.hooksPath names the gate under
	// test, as install.sh makes the user's ~/.gitconfig do.
	globalCfg := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(globalCfg, []byte("[core]\n\thooksPath = "+filepath.Join(dir, "hooks")+"\n"), 0o644); err != nil {
		os.RemoveAll(dir)
		panic(err)
	}
	os.Setenv("GIT_CONFIG_GLOBAL", globalCfg)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
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
	out, _, code := run(t, `{"tool_name":"Bash","tool_input":{"command":"commit-gate approve"}}`, "precheck")
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

// setGlobalHooksPath runs the test with a global git config whose
// core.hooksPath is dir.
func setGlobalHooksPath(t *testing.T, dir string) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(f, []byte("[core]\n\thooksPath = "+dir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", f)
}

func TestSessioncheckIsSilentWhenTheGlobalGateCoversTheRepo(t *testing.T) {
	if out, _ := runSessioncheck(t, mkrepoT(t)); out != "" {
		t.Fatalf("must be silent: %q", out)
	}
}

func TestSessioncheckReportsAMissingGlobalHooksPath(t *testing.T) {
	noGlobalGate(t)
	repo := mkrepoT(t)
	_, so := runSessioncheck(t, repo)
	want := "commit-gate: the global core.hooksPath is unset, so repos here are ungated. To gate every repo: git config --global core.hooksPath " + liveHooks(t)
	if so.SystemMessage != want {
		t.Fatalf("systemMessage:\ngot:  %q\nwant: %q", so.SystemMessage, want)
	}
	if ctx := so.HookSpecificOutput.AdditionalContext; !strings.Contains(ctx, "do not commit or push") {
		t.Fatalf("additionalContext: %q", ctx)
	}
	if hp := localHooksPath(t, repo); hp != "" {
		t.Fatalf("sessioncheck must not enable repo by repo: %q", hp)
	}
}

func TestSessioncheckReportsAForeignGlobalHooksPathOutsideAnyRepo(t *testing.T) {
	other := t.TempDir()
	setGlobalHooksPath(t, other)
	_, so := runSessioncheck(t, t.TempDir())
	want := "commit-gate: the global core.hooksPath is " + other + ", not the gate, so repos here are ungated. To gate every repo: git config --global core.hooksPath " + liveHooks(t)
	if so.SystemMessage != want {
		t.Fatalf("systemMessage:\ngot:  %q\nwant: %q", so.SystemMessage, want)
	}
}

func TestSessioncheckReportsAnOverriddenRepo(t *testing.T) {
	repo := mkrepoT(t)
	top := showToplevel(t, repo)
	elsewhere := t.TempDir()
	gitSetConfig(t, repo, "core.hooksPath", elsewhere)
	_, so := runSessioncheck(t, repo)
	want := "commit-gate: " + top + " is ungated: its core.hooksPath is " + elsewhere + ". To gate it: commit-gate enable (clears that core.hooksPath). To opt out: commit-gate disable"
	if so.SystemMessage != want {
		t.Fatalf("systemMessage:\ngot:  %q\nwant: %q", so.SystemMessage, want)
	}
	ctx := so.HookSpecificOutput.AdditionalContext
	if !strings.Contains(ctx, "do not commit or push here") || !strings.Contains(ctx, "explicit yes") ||
		!strings.Contains(ctx, "`commit-gate enable` gates it") {
		t.Fatalf("additionalContext: %q", ctx)
	}
	if hp := localHooksPath(t, repo); hp != elsewhere {
		t.Fatalf("sessioncheck must leave the override: %q", hp)
	}
}

func TestSessioncheckLeavesOptedOutRepo(t *testing.T) {
	repo := mkrepoT(t)
	gitSetConfig(t, repo, "commit-gate.disabled", "true")
	if out, _ := runSessioncheck(t, repo); out != "" {
		t.Fatalf("opted-out repo must be silent: %q", out)
	}
	if hp := localHooksPath(t, repo); hp != "" {
		t.Fatalf("opted-out repo must stay untouched, core.hooksPath=%q", hp)
	}
	if v := gitGetConfig(t, repo, "commit-gate.disabled"); v != "true" {
		t.Fatalf("opt-out must survive, commit-gate.disabled=%q", v)
	}
}

func TestSessioncheckNonRepoSilent(t *testing.T) {
	if out, _ := runSessioncheck(t, t.TempDir()); out != "" {
		t.Fatalf("non-repo must be silent: %q", out)
	}
}

// noGlobalGate runs the test on a host whose global git config names no
// core.hooksPath.
func noGlobalGate(t *testing.T) {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
}

// An empty repo-local core.hooksPath is named as empty, as commit-gate status names
// it, not printed as a blank.
func TestSessioncheckNamesAnEmptyOverride(t *testing.T) {
	repo := mkrepoT(t)
	top := showToplevel(t, repo)
	gitSetConfig(t, repo, "core.hooksPath", "")
	_, so := runSessioncheck(t, repo)
	want := "commit-gate: " + top + " is ungated: its core.hooksPath is empty. To gate it: commit-gate enable (clears that core.hooksPath). To opt out: commit-gate disable"
	if so.SystemMessage != want {
		t.Fatalf("systemMessage:\ngot:  %q\nwant: %q", so.SystemMessage, want)
	}
	ctx := so.HookSpecificOutput.AdditionalContext
	if !strings.Contains(ctx, "its repo-local core.hooksPath (empty) overrides the gate") {
		t.Fatalf("additionalContext: %q", ctx)
	}
}

// A worktree-scope override is not repo-local and commit-gate enable doesn't clear
// it: the notice names its scope and the command that does.
func TestSessioncheckNamesAWorktreeOverrideAndItsRemedy(t *testing.T) {
	repo := mkrepoT(t)
	top := showToplevel(t, repo)
	elsewhere := t.TempDir()
	gitSetConfig(t, repo, "extensions.worktreeConfig", "true")
	gitSetConfig(t, repo, "--worktree", "core.hooksPath", elsewhere)
	_, so := runSessioncheck(t, repo)
	want := "commit-gate: " + top + " is ungated: its core.hooksPath is " + elsewhere + ", from worktree config. To gate it: git config --worktree --unset core.hooksPath. To opt out: commit-gate disable"
	if so.SystemMessage != want {
		t.Fatalf("systemMessage:\ngot:  %q\nwant: %q", so.SystemMessage, want)
	}
	ctx := so.HookSpecificOutput.AdditionalContext
	if strings.Contains(ctx, "repo-local") || strings.Contains(ctx, "`commit-gate enable` gates it") ||
		!strings.Contains(ctx, "do not commit or push here") || !strings.Contains(ctx, "explicit yes") {
		t.Fatalf("additionalContext: %q", ctx)
	}
}

// A command-scope override comes from the session's environment (git -c
// passed down, GIT_CONFIG_*): no config file holds it.
func TestSessioncheckNamesACommandScopeOverride(t *testing.T) {
	repo := mkrepoT(t)
	top := showToplevel(t, repo)
	elsewhere := t.TempDir()
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", elsewhere)
	_, so := runSessioncheck(t, repo)
	want := "commit-gate: " + top + " is ungated: its core.hooksPath is " + elsewhere + ", from command config: git -c or GIT_CONFIG_* in the environment. To gate it: remove it from the environment Claude Code runs in. To opt out: commit-gate disable"
	if so.SystemMessage != want {
		t.Fatalf("systemMessage:\ngot:  %q\nwant: %q", so.SystemMessage, want)
	}
	ctx := so.HookSpecificOutput.AdditionalContext
	if strings.Contains(ctx, "repo-local") || strings.Contains(ctx, "`commit-gate enable` gates it") ||
		!strings.Contains(ctx, "do not commit or push here") {
		t.Fatalf("additionalContext: %q", ctx)
	}
}
