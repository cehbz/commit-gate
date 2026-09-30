package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cehbz/commit-gate/gatestate"
	"github.com/cehbz/commit-gate/harness"
	"github.com/cehbz/commit-gate/policy"
)

func installDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return filepath.Dir(exe), nil
}

func hooksFor(installDir string) string {
	h := filepath.Join(installDir, "hooks")
	if r, err := filepath.EvalSymlinks(h); err == nil {
		return r
	}
	return h
}

func liveHooksDir() string {
	d, err := installDir()
	if err != nil {
		return ""
	}
	return hooksFor(d)
}

// realResolve: absolute, cleaned, symlinks resolved via nearest existing ancestor.
func realResolve(cwd, path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	if !filepath.IsAbs(path) {
		base := cwd
		if base == "" {
			base, _ = os.Getwd()
		}
		path = filepath.Join(base, path)
	}
	path = filepath.Clean(path)
	// resolve through the nearest existing ancestor so unborn paths compare correctly
	probe, rest := path, ""
	for {
		if r, err := filepath.EvalSymlinks(probe); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return path
		}
		rest = filepath.Join(filepath.Base(probe), rest)
		probe = parent
	}
}

func cmdPrecheck() (code int) {
	defer func() {
		if r := recover(); r != nil {
			os.Stdout.Write(harness.Deny("commit-gate: internal error; denied fail-closed"))
			code = 0
		}
	}()
	in, err := harness.ParseInput(os.Stdin)
	if err != nil {
		os.Stdout.Write(harness.Deny("commit-gate: unreadable hook input; denied fail-closed"))
		return 0
	}
	inst, err := installDir()
	if err != nil {
		os.Stdout.Write(harness.Deny("commit-gate: cannot locate the live install; denied fail-closed"))
		return 0
	}
	hooks := hooksFor(inst)
	gated := false
	cwd := in.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if repo, err := gatestate.Open(cwd); err == nil {
		gated = !repo.Disabled() && repo.Enabled(hooks)
	}
	d := policy.Decide(policy.ToolCall{
		ToolName: in.ToolName,
		Command:  in.ToolInput.Command,
		FilePath: in.Path(),
		CWD:      cwd,
	}, policy.Ctx{InstallDir: inst, HooksDir: hooks, GatedCWD: gated, Resolve: realResolve})
	switch d.Kind {
	case policy.Deny:
		os.Stdout.Write(harness.Deny(d.Reason))
	case policy.AllowContext:
		os.Stdout.Write(harness.AllowContext(d.Reason))
	}
	return 0
}

// cmdSessioncheck is the SessionStart hook: an undecided repo (neither
// enabled nor opted out) is enabled, and the user (systemMessage) and the
// model (additionalContext) are told, including when enabling fails.
// Enabled, opted-out and non-repo directories stay silent.
func cmdSessioncheck() int {
	in, err := harness.ParseInput(os.Stdin)
	if err != nil {
		return 0 // sessioncheck is advisory; stay silent on garbage
	}
	cwd := in.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	repo, err := gatestate.Open(cwd)
	if err != nil {
		return 0
	}
	if repo.Disabled() || repo.Enabled(liveHooksDir()) {
		return 0
	}
	top, terr := repo.Toplevel()
	if terr != nil {
		top = cwd
	}
	if err := repo.Enable(liveHooksDir()); err != nil {
		os.Stdout.Write(harness.SessionNotice(
			"commit-gate could not be enabled in this repository ("+top+"): "+err.Error()+". A missing gate means commits are denied: do not commit or push here. The user was shown this failure and can run `gate-enable` to retry or `gate-disable` to opt out.",
			"commit-gate: could not enable in "+top+": "+err.Error()+". Commits here are denied until fixed: run gate-enable to retry, or gate-disable to opt out."))
		return 0
	}
	os.Stdout.Write(harness.SessionNotice(
		"commit-gate was enabled by default in this repository ("+top+"): commits and pushes here now need the user's recorded approval (workflow: ~/.claude/CLAUDE.md, \"Commits, pushes and publishing\"). The user was told they can opt out by running `gate-disable`.",
		"commit-gate: enabled in "+top+" (default). To opt out: gate-disable"))
	return 0
}
