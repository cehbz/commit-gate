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

// globalGitConfigs returns the resolved paths of the files git reads as the
// user's global config: ~/.gitconfig and $XDG_CONFIG_HOME/git/config, the
// latter under ~/.config when XDG_CONFIG_HOME is unset or empty, as git
// looks it up.
func globalGitConfigs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	return []string{
		realResolve("", filepath.Join(home, ".gitconfig")),
		realResolve("", filepath.Join(xdg, "git", "config")),
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
		gated = repo.State(hooks) == gatestate.StateEnabled
	}
	d := policy.Decide(policy.ToolCall{
		ToolName: in.ToolName,
		Command:  in.ToolInput.Command,
		FilePath: in.Path(),
		CWD:      cwd,
	}, policy.Ctx{InstallDir: inst, HooksDir: hooks, GlobalGitConfigs: globalGitConfigs(), GatedCWD: gated, Resolve: realResolve})
	switch d.Kind {
	case policy.Deny:
		os.Stdout.Write(harness.Deny(d.Reason))
	case policy.AllowContext:
		os.Stdout.Write(harness.AllowContext(d.Reason))
	}
	return 0
}

// cmdSessioncheck is the SessionStart hook. It changes nothing: it tells the
// user (systemMessage) and the model (additionalContext) when the global
// core.hooksPath is missing or not the gate, and when the session's repo
// overrides the gate with a repo-local core.hooksPath. Otherwise silent.
func cmdSessioncheck() int {
	in, err := harness.ParseInput(os.Stdin)
	if err != nil {
		return 0 // sessioncheck is advisory; stay silent on garbage
	}
	cwd := in.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	hooks := liveHooksDir()
	var ctx, user []string
	if p := gatestate.GlobalGateProblem(hooks); p != "" {
		ctx = append(ctx, "commit-gate does not gate every repository on this host: "+p+". A repository without its own gate setting is ungated, which counts as a missing gate: do not commit or push in it. The user was shown the fix: git config --global core.hooksPath "+hooks+".")
		user = append(user, "commit-gate: "+p+", so repos here are ungated. To gate every repo: git config --global core.hooksPath "+hooks)
	}
	if repo, err := gatestate.Open(cwd); err == nil && repo.State(hooks) == gatestate.StateOverridden {
		hp, _, _, _ := repo.HooksPath()
		top, terr := repo.Toplevel()
		if terr != nil {
			top = cwd
		}
		ctx = append(ctx, "commit-gate does not gate this repository ("+top+"): its repo-local core.hooksPath ("+hp+") overrides the gate, which counts as a missing gate: do not commit or push here. The user was told `gate-enable` gates it, clearing that core.hooksPath and so stopping the hooks it names, and `gate-disable` opts out. Do not run `gate-enable` without their explicit yes.")
		user = append(user, "commit-gate: "+top+" is ungated: its core.hooksPath is "+hp+". To gate it: gate-enable (clears that core.hooksPath). To opt out: gate-disable")
	}
	if len(ctx) == 0 {
		return 0
	}
	os.Stdout.Write(harness.SessionNotice(strings.Join(ctx, " "), strings.Join(user, "\n")))
	return 0
}
