// gatestate/repo.go
package gatestate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type State int

const (
	// StateUngated: no override above the global config, and the effective
	// core.hooksPath is missing, unreadable or not the gate.
	StateUngated State = iota
	StateEnabled
	StateDisabled
	// StateOverridden: a scope above the global config (local, worktree,
	// command) sets core.hooksPath to something other than the gate, an
	// empty value included.
	StateOverridden
)

type Repo struct {
	Dir    string // the directory we were opened from
	Common string // absolute, symlink-resolved git common dir
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

func Open(dir string) (*Repo, error) {
	c, err := git(dir, "rev-parse", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("not inside a git repository")
	}
	if !filepath.IsAbs(c) {
		c = filepath.Join(dir, c)
	}
	if rc, err := filepath.EvalSymlinks(c); err == nil {
		c = rc
	}
	return &Repo{Dir: dir, Common: c}, nil
}

func (r *Repo) GateDir() string       { return filepath.Join(r.Common, "commit-gate") }
func (r *Repo) ManifestPath() string  { return filepath.Join(r.GateDir(), "approved") }
func (r *Repo) PendingPath() string   { return filepath.Join(r.Common, "cg-pending") }
func (r *Repo) PushTokenPath() string { return filepath.Join(r.GateDir(), "push-token") }

func (r *Repo) Toplevel() (string, error) { return git(r.Dir, "rev-parse", "--show-toplevel") }

func (r *Repo) Disabled() bool {
	v, err := git(r.Dir, "config", "--get", "commit-gate.disabled")
	return err == nil && v == "true"
}

// HooksPath returns the effective core.hooksPath, ~ expanded and possibly
// empty, and the scope git read it from: local, worktree, global, system or
// command. ok is false when no scope sets it; err reports a failed read.
func (r *Repo) HooksPath() (path, scope string, ok bool, err error) {
	out, err := git(r.Dir, "config", "--show-scope", "--type=path", "--get", "core.hooksPath")
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return "", "", false, nil // git config --get: the key is not set
	}
	if err != nil {
		return "", "", false, err
	}
	scope, path, found := strings.Cut(out, "\t")
	if !found {
		return "", "", false, fmt.Errorf("git config --show-scope: unexpected output %q", out)
	}
	return path, scope, true, nil
}

// aboveGlobal reports whether a core.hooksPath from scope overrides the
// global one.
func aboveGlobal(scope string) bool {
	return scope == "local" || scope == "worktree" || scope == "command"
}

// isGate reports whether a core.hooksPath value names hooksDir; a relative
// value resolves against the repo.
func (r *Repo) isGate(hp, hooksDir string) bool {
	if hp == "" || hooksDir == "" {
		return false
	}
	if !filepath.IsAbs(hp) {
		hp = filepath.Join(r.Dir, hp)
	}
	return SameDir(hp, hooksDir)
}

// SameDir reports whether a and b name one directory after symlink
// resolution; an a that does not resolve names nothing.
func SameDir(a, b string) bool {
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}
	if rb, err := filepath.EvalSymlinks(b); err == nil {
		b = rb
	}
	return ra == b
}

// Enabled reports whether the effective core.hooksPath, from any scope, is
// the gate.
func (r *Repo) Enabled(hooksDir string) bool {
	hp, _, ok, _ := r.HooksPath()
	return ok && r.isGate(hp, hooksDir)
}

// State: opted out wins; else enabled when the effective core.hooksPath is
// the gate (a value above the global one naming the gate included); else
// overridden when a value above the global one names anything else, empty
// included; else ungated, an unreadable value included.
func (r *Repo) State(hooksDir string) State {
	if r.Disabled() {
		return StateDisabled
	}
	hp, scope, ok, _ := r.HooksPath() // ok is false on a failed read
	switch {
	case !ok:
		return StateUngated
	case r.isGate(hp, hooksDir):
		return StateEnabled
	case aboveGlobal(scope):
		return StateOverridden
	}
	return StateUngated
}

// GlobalHooksPath returns the core.hooksPath in the user's global git
// config (~/.gitconfig, or $GIT_CONFIG_GLOBAL), ~ expanded.
func GlobalHooksPath() (string, bool) {
	out, err := git("/", "config", "--global", "--type=path", "--get", "core.hooksPath")
	if err != nil || out == "" {
		return "", false
	}
	return out, true
}

// GlobalGateProblem says why the global core.hooksPath doesn't name
// hooksDir, or returns "" when it does.
func GlobalGateProblem(hooksDir string) string {
	hp, ok := GlobalHooksPath()
	switch {
	case !ok:
		return "the global core.hooksPath is unset"
	case !SameDir(hp, hooksDir):
		return "the global core.hooksPath is " + hp + ", not the gate"
	}
	return ""
}

func (r *Repo) HeadMessage() (string, bool) {
	if _, err := git(r.Dir, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		return "", false
	}
	msg, err := git(r.Dir, "log", "-1", "--format=%B", "HEAD")
	if err != nil {
		return "", false
	}
	return msg, true
}

func (r *Repo) GitConfigSet(key, value string) error {
	_, err := git(r.Dir, "config", key, value)
	return err
}

func (r *Repo) GitConfigUnset(key string) error {
	if _, err := git(r.Dir, "config", "--unset", key); err != nil {
		if _, geterr := git(r.Dir, "config", "--get", key); geterr != nil {
			return nil // already absent
		}
		return err
	}
	return nil
}

// Enable gates the repo: core.hooksPath points at hooksDir, any opt-out is
// cleared, and the gate dir and manifest exist (an existing manifest is kept).
func (r *Repo) Enable(hooksDir string) error {
	if hooksDir == "" {
		return fmt.Errorf("live hooks dir unknown")
	}
	if err := r.GitConfigSet("core.hooksPath", hooksDir); err != nil {
		return err
	}
	if err := r.GitConfigUnset("commit-gate.disabled"); err != nil {
		return err
	}
	if err := os.MkdirAll(r.GateDir(), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(r.ManifestPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
