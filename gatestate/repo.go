// gatestate/repo.go
package gatestate

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type State int

const (
	StateUndecided State = iota
	StateEnabled
	StateDisabled
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

func (r *Repo) Enabled(hooksDir string) bool {
	hp, err := git(r.Dir, "config", "--get", "core.hooksPath")
	if err != nil || hp == "" {
		return false
	}
	if !filepath.IsAbs(hp) {
		hp = filepath.Join(r.Dir, hp)
	}
	if rhp, err := filepath.EvalSymlinks(hp); err == nil {
		hp = rhp
	} else {
		return false // bash: cd fails -> not enabled
	}
	// Resolve hooksDir for comparison
	rhooksDir, err := filepath.EvalSymlinks(hooksDir)
	if err != nil {
		rhooksDir = hooksDir
	}
	return hp == rhooksDir
}

func (r *Repo) State(hooksDir string) State {
	if r.Disabled() {
		return StateDisabled
	}
	if r.Enabled(hooksDir) {
		return StateEnabled
	}
	return StateUndecided
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
