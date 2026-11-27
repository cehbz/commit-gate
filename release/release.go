// Package release holds what the version check, release, deploy and install
// share (DESIGN.md: Build, Release, Target, Version check): a release is a
// vMAJOR.MINOR.PATCH tag on the gate's public repository, the target is the
// commit the highest one names, and a build is up to date when it is from
// the target.
package release

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
)

// PublicAddress is the gate's public repository, which every host can read
// without credentials.
const PublicAddress = "https://github.com/cehbz/commit-gate.git"

// Tag is a release tag: vMAJOR.MINOR.PATCH, with no suffix.
type Tag struct {
	Name                string
	Major, Minor, Patch int
}

var tagRe = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// ParseTag reads name as a release tag; ok is false for any other tag
// (latest, v1.0, v3, v2.0.0-rc1).
func ParseTag(name string) (t Tag, ok bool) {
	m := tagRe.FindStringSubmatch(name)
	if m == nil {
		return Tag{}, false
	}
	var v [3]int
	for i := range v {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return Tag{}, false
		}
		v[i] = n
	}
	return Tag{Name: name, Major: v[0], Minor: v[1], Patch: v[2]}, true
}

// Less orders release tags by semver: v1.9.0 before v1.10.0.
func (t Tag) Less(u Tag) bool {
	if t.Major != u.Major {
		return t.Major < u.Major
	}
	if t.Minor != u.Minor {
		return t.Minor < u.Minor
	}
	return t.Patch < u.Patch
}

// Target is the highest release tag on the public repository and the commit
// it names.
type Target struct {
	Tag    Tag
	Commit string
}

// ErrNoRelease is ReadTarget's error when the repository holds no release tag.
var ErrNoRelease = errors.New("no release tag")

// ReadTarget reads the target from the repository at url with git
// ls-remote, as git configured for the caller reads it, a url.insteadOf
// included. It never prompts for credentials.
func ReadTarget(url string) (Target, error) {
	cmd := exec.Command("git", "ls-remote", "--tags", url)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return Target{}, fmt.Errorf("git ls-remote --tags %s: %v: %s", url, err, strings.TrimSpace(errb.String()))
	}
	return highest(out.String())
}

// highest is the highest release tag in git ls-remote --tags output. An
// annotated tag names the commit on its peeled (^{}) line.
func highest(lsRemote string) (Target, error) {
	commits := map[string]string{}
	for _, line := range strings.Split(lsRemote, "\n") {
		hash, ref, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		name, ok := strings.CutPrefix(ref, "refs/tags/")
		if !ok {
			continue
		}
		if tagged, peeled := strings.CutSuffix(name, "^{}"); peeled {
			commits[tagged] = hash
		} else if _, seen := commits[name]; !seen {
			commits[name] = hash
		}
	}
	var best Target
	found := false
	for name, commit := range commits {
		if t, ok := ParseTag(name); ok && (!found || best.Tag.Less(t)) {
			best, found = Target{Tag: t, Commit: commit}, true
		}
	}
	if !found {
		return Target{}, ErrNoRelease
	}
	return best, nil
}

// Build is a build of the gate as its file records it: the commit it was
// built from, "" when it records none, and whether that source had
// uncommitted changes.
type Build struct {
	Commit string
	Dirty  bool
}

// ThisBuild is the running binary's build.
func ThisBuild() Build {
	var b Build
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return b
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			b.Commit = s.Value
		case "vcs.modified":
			b.Dirty = s.Value == "true"
		}
	}
	return b
}

// FromCommit reports whether the build is from a commit: it records one and
// its source had no uncommitted changes.
func (b Build) FromCommit() bool { return b.Commit != "" && !b.Dirty }

// String is the build as the version check reports it.
func (b Build) String() string {
	s := b.Commit
	if s == "" {
		s = "from no commit"
	}
	if b.Dirty {
		s += " (dirty)"
	}
	return s
}

// State is a build's standing against the target.
type State int

const (
	Unknown State = iota
	UpToDate
	OutOfDate
)

func (s State) String() string {
	switch s {
	case UpToDate:
		return "up to date"
	case OutOfDate:
		return "out of date"
	}
	return "unknown"
}

// Check is the version check of a build: the target it is measured
// against, or why there is none, and its state.
type Check struct {
	Build  Build
	Target Target
	Err    error // why the target can't be known; State is then Unknown
	State  State
}

// CheckBuild measures b against the target read from url: up to date when b
// is from the target's commit, out of date otherwise, unknown when the
// target can't be read or no release exists.
func CheckBuild(b Build, url string) Check {
	t, err := ReadTarget(url)
	if err != nil {
		return Check{Build: b, Err: err, State: Unknown}
	}
	if b.FromCommit() && b.Commit == t.Commit {
		return Check{Build: b, Target: t, State: UpToDate}
	}
	return Check{Build: b, Target: t, State: OutOfDate}
}
