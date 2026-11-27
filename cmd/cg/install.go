// install.go implements `commit-gate install` (DESIGN.md, Install): it puts
// the running build live and wires the host to it, at fixed locations under
// the home directory, and refuses, changing nothing, unless the build is up
// to date.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cehbz/commit-gate/gatestate"
	"github.com/cehbz/commit-gate/release"
)

// cmdInstall implements `commit-gate install`.
func cmdInstall(_ []string) int {
	c := release.CheckBuild(release.ThisBuild(), release.PublicAddress)
	if c.State != release.UpToDate {
		if err := printCheck(os.Stderr, c); err != nil {
			return dieApprove("%s", err)
		}
		return dieApprove("install refuses: this build is %s, and only the target goes live; nothing changed", c.State)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return dieApprove("%s", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return dieApprove("%s", err)
	}
	l := release.Layout{Home: home}
	for _, step := range []func() error{
		func() error { return putLive(exe, l) },
		func() error { return linkNames(l) },
		func() error { return linkOnPath(l) },
		func() error { return gateEveryRepo(l) },
		func() error { return addClaudeHooks(l) },
	} {
		if err := step(); err != nil {
			return dieApprove("install: %s", err)
		}
	}
	return 0
}

// putLive makes the running build's file the live binary in one step: a
// copy written beside the live binary, then renamed over it, so the host
// runs the old build or the new one and never part of one.
func putLive(exe string, l release.Layout) error {
	if err := os.MkdirAll(l.Dir(), 0o755); err != nil {
		return err
	}
	src, err := os.Open(exe)
	if err != nil {
		return err
	}
	defer src.Close()
	tmp, err := os.CreateTemp(l.Dir(), ".commit-gate.new-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // gone once renamed
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), l.Live()); err != nil {
		return err
	}
	fmt.Printf("commit-gate: live: %s (%s)\n", l.Live(), release.ThisBuild())
	return nil
}

// linkNames links every hook name in the hook directory, and precheck and
// sessioncheck in the install directory, to the live binary.
func linkNames(l release.Layout) error {
	if err := os.MkdirAll(l.Hooks(), 0o755); err != nil {
		return err
	}
	for _, h := range hookNames() {
		if err := replaceLink("../commit-gate", filepath.Join(l.Hooks(), h)); err != nil {
			return err
		}
	}
	for _, n := range []string{"precheck", "sessioncheck"} {
		if err := replaceLink("commit-gate", filepath.Join(l.Dir(), n)); err != nil {
			return err
		}
	}
	return nil
}

// linkOnPath links commit-gate on PATH to the live binary.
func linkOnPath(l release.Layout) error {
	if err := os.MkdirAll(filepath.Dir(l.OnPath()), 0o755); err != nil {
		return err
	}
	if err := replaceLink(l.Live(), l.OnPath()); err != nil {
		return err
	}
	fmt.Printf("commit-gate: linked %s\n", l.OnPath())
	return nil
}

// replaceLink makes link a symlink to target in one step: a new link beside
// it, renamed over it.
func replaceLink(target, link string) error {
	tmp := link + ".new"
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, link)
}

// gateEveryRepo points the global core.hooksPath at the hook directory. A
// value that names another directory is left as it is and reported, with
// the command that gates every repo.
func gateEveryRepo(l release.Layout) error {
	hp, ok := gatestate.GlobalHooksPath("")
	switch {
	case !ok:
		if out, err := exec.Command("git", "config", "--global", "core.hooksPath", l.Hooks()).CombinedOutput(); err != nil {
			return fmt.Errorf("git config --global core.hooksPath: %v: %s", err, bytes.TrimSpace(out))
		}
		fmt.Printf("commit-gate: the global core.hooksPath is now %s\n", l.Hooks())
	case gatestate.SameDir(hp, l.Hooks()):
		fmt.Printf("commit-gate: the global core.hooksPath already names the gate: %s\n", hp)
	default:
		fmt.Fprintf(os.Stderr, "commit-gate: the global core.hooksPath is %s, not the gate; left as it is. To gate every repo: %s\n",
			hp, gatestate.GlobalGateFix("", l.Hooks()))
	}
	return nil
}

// addClaudeHooks adds Claude Code's PreToolUse hook running the precheck on
// Bash and the file-editing tools, and its SessionStart hook running the
// session check, to settings.json, each once, keeping everything else in it.
func addClaudeHooks(l release.Layout) error {
	path := l.Settings()
	settings := map[string]any{}
	b, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(b, &settings); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}
	precheck, sessioncheck := filepath.Join(l.Dir(), "precheck"), filepath.Join(l.Dir(), "sessioncheck")
	added := addHook(hooks, "PreToolUse", precheck, map[string]any{
		"matcher": "Bash|Write|Edit|MultiEdit|NotebookEdit",
		"hooks":   []any{map[string]any{"type": "command", "command": precheck, "statusMessage": "commit-gate precheck"}},
	})
	added = addHook(hooks, "SessionStart", sessioncheck, map[string]any{
		"hooks": []any{map[string]any{"type": "command", "command": sessioncheck}},
	}) || added
	if !added {
		fmt.Printf("commit-gate: Claude Code hooks in %s\n", path)
		return nil
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(settings); err != nil {
		return err
	}
	// WriteFile writes through a symlinked settings.json and keeps its mode.
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Printf("commit-gate: Claude Code hooks in %s\n", path)
	return nil
}

// addHook appends entry to hooks[event] unless a hook there already runs
// command, and reports whether it appended.
func addHook(hooks map[string]any, event, command string, entry map[string]any) bool {
	list, _ := hooks[event].([]any)
	for _, e := range list {
		m, _ := e.(map[string]any)
		inner, _ := m["hooks"].([]any)
		for _, h := range inner {
			if hm, _ := h.(map[string]any); hm["command"] == command {
				return false
			}
		}
	}
	hooks[event] = append(list, entry)
	return true
}
