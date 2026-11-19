// gatestate/repo.go
package gatestate

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
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
	path, scope, _, ok, err = readHooksPath(r.Dir)
	return path, scope, ok, err
}

// readHooksPath is the one reader of core.hooksPath, so the effective and
// the global values can't be read differently: run in dir, so an includeIf
// "gitdir:" matches as it does for git there, with includes followed. opts
// narrows it to one scope (--global); none reads the effective value. origin
// is the file that sets it, "" when it isn't a file.
func readHooksPath(dir string, opts ...string) (path, scope, origin string, ok bool, err error) {
	return readHooksPathAs(dir, append(opts, "--includes")...)
}

// readHooksPathAs reads core.hooksPath with opts as given, --includes or
// --no-includes among them.
func readHooksPathAs(dir string, opts ...string) (path, scope, origin string, ok bool, err error) {
	args := append(append([]string{"config"}, opts...), "--show-scope", "--show-origin", "--type=path", "--get", "core.hooksPath")
	out, err := git(dir, args...)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return "", "", "", false, nil // git config --get: the key is not set
	}
	if err != nil {
		return "", "", "", false, err
	}
	scope, rest, found := strings.Cut(out, "\t")
	from, path, found2 := strings.Cut(rest, "\t")
	if !found || !found2 {
		return "", "", "", false, fmt.Errorf("git config --show-scope --show-origin: unexpected output %q", out)
	}
	if f, isFile := strings.CutPrefix(from, "file:"); isFile {
		if uq, err := strconv.Unquote(f); err == nil {
			f = uq // git quotes a path with special characters
		}
		origin = f
	}
	return path, scope, origin, true, nil
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

// GlobalHooksPath returns the core.hooksPath the user's global git config
// (~/.gitconfig, or $GIT_CONFIG_GLOBAL) sets for git run in dir, ~
// expanded: the files it includes are read, and an includeIf "gitdir:"
// applies when dir is in a repo it matches. dir "" is read as "/", in no
// repo.
func GlobalHooksPath(dir string) (string, bool) {
	if dir == "" {
		dir = "/"
	}
	hp, _, _, ok, err := readHooksPath(dir, "--global")
	if err != nil || !ok || hp == "" {
		return "", false
	}
	return hp, true
}

// GlobalGateProblem says why the global core.hooksPath, as git run in dir
// reads it, doesn't name hooksDir, or returns "" when it does.
func GlobalGateProblem(dir, hooksDir string) string {
	hp, ok := GlobalHooksPath(dir)
	switch {
	case !ok:
		return "the global core.hooksPath is unset"
	case !SameDir(hp, hooksDir):
		return "the global core.hooksPath is " + hp + ", not the gate"
	}
	return ""
}

// GlobalGateFix is the one remedy for a global core.hooksPath that isn't
// the gate, for every command that reports one: the command that makes the
// value git reads in dir name hooksDir, its paths shell-quoted so it pastes.
// It writes the file that sets the value, decided by origin alone, an
// empty value included: an included file when an include or includeIf sets
// it, which git config --global would not override, else the global config.
func GlobalGateFix(dir, hooksDir string) string {
	global := "git config --global core.hooksPath " + shellQuote(hooksDir)
	if dir == "" {
		dir = "/"
	}
	_, _, origin, ok, err := readHooksPath(dir, "--global")
	if err != nil || !ok || origin == "" {
		return global
	}
	if _, _, top, ok, err := readHooksPathAs(dir, "--global", "--no-includes"); err == nil && ok && top == origin {
		return global // the global config itself sets it
	}
	return "git config --file " + shellQuote(origin) + " core.hooksPath " + shellQuote(hooksDir)
}

// shellQuote returns s as one shell word: as it is when it holds only
// [A-Za-z0-9/._~+-], else single-quoted, a ' inside closed, escaped and
// reopened.
func shellQuote(s string) string {
	plain := s != ""
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("/._~+-", c)) {
			plain = false
			break
		}
	}
	if plain {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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

// GitConfigUnset removes every repo-local value of key; an absent key is no
// error. Values from other scopes stay.
func (r *Repo) GitConfigUnset(key string) error {
	if _, err := git(r.Dir, "config", "--local", "--unset-all", key); err != nil {
		if _, geterr := git(r.Dir, "config", "--local", "--get-all", key); geterr != nil {
			return nil // already absent
		}
		return err
	}
	return nil
}

// Enable gates the repo under the global gate: it clears a repo-local
// opt-out and core.hooksPath. While the global core.hooksPath is not the
// gate it refuses and changes nothing, since clearing a repo-local value
// would then leave the repo less gated. An opt-out or override from
// another scope survives; Enable reports it.
func (r *Repo) Enable(hooksDir string) error {
	if hooksDir == "" {
		return fmt.Errorf("live hooks dir unknown")
	}
	if p := GlobalGateProblem(r.Dir, hooksDir); p != "" {
		return fmt.Errorf("%s; to gate every repo: %s", p, GlobalGateFix(r.Dir, hooksDir))
	}
	if err := r.GitConfigUnset("core.hooksPath"); err != nil {
		return err
	}
	if err := r.GitConfigUnset("commit-gate.disabled"); err != nil {
		return err
	}
	switch r.State(hooksDir) {
	case StateEnabled:
		return nil
	case StateDisabled:
		scope, err := r.configScope("commit-gate.disabled")
		if err != nil {
			return err
		}
		return remains("commit-gate.disabled is still true", "commit-gate.disabled", scope)
	}
	hp, scope, ok, err := r.HooksPath()
	switch {
	case err != nil:
		return err
	case !ok:
		return fmt.Errorf("core.hooksPath is unset")
	}
	return remains(fmt.Sprintf("core.hooksPath is still %q", hp), "core.hooksPath", scope)
}

// configScope returns the scope git reads key's effective value from.
func (r *Repo) configScope(key string) (string, error) {
	out, err := git(r.Dir, "config", "--show-scope", "--get", key)
	if err != nil {
		return "", err
	}
	scope, _, _ := strings.Cut(out, "\t")
	return scope, nil
}

// remains reports a value of key that Enable could not clear, naming the
// clearing command when it is in the worktree config.
func remains(what, key, scope string) error {
	msg := what + ", from " + scope + " config"
	if scope == "worktree" {
		msg += "; to clear it: git config --worktree --unset " + key
	}
	return errors.New(msg)
}
