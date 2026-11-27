// version_test.go covers the Version check: commit-gate version measures the
// build it runs against the target, the commit the highest release tag on
// the public address names. A committed copy of the working tree is the
// scratch source of the builds under test, and each test points the public
// address at a scratch repository holding the release tags it needs
// (url.<scratch>.insteadOf in its private global git config), so every build
// under test is an ordinary build.
package main_test

import (
	"debug/buildinfo"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

const publicAddress = "https://github.com/cehbz/commit-gate.git"

// scratchBuilds are builds of the scratch source: c1 and then c2 are its
// commits.
type scratchBuilds struct {
	src      string // the scratch source repository
	c1, c2   string
	b1, b2   string // builds of c1 and c2
	dirty    string // a build of c1 with an uncommitted change
	noCommit string // a build of c2 that records no commit
}

var (
	scratchOnce sync.Once
	scratch     scratchBuilds
	scratchErr  string
)

// builds returns the scratch builds, made on first use under binDir, which
// TestMain removes.
func builds(t *testing.T) scratchBuilds {
	t.Helper()
	scratchOnce.Do(func() { scratchErr = makeScratchBuilds() })
	if scratchErr != "" {
		t.Fatal(scratchErr)
	}
	return scratch
}

func makeScratchBuilds() string {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return err.Error()
	}
	dir := filepath.Join(binDir, "scratch")
	src := filepath.Join(dir, "src")
	if err := copyWorkingTree(root, src); err != nil {
		return err.Error()
	}
	git := func(args ...string) (string, error) {
		out, err := exec.Command("git", append([]string{"-C", src}, args...)...).CombinedOutput()
		if err != nil {
			return "", &cmdError{"git " + strings.Join(args, " "), string(out), err}
		}
		return strings.TrimSpace(string(out)), nil
	}
	commit := func(msg string) (string, error) {
		if _, err := git("add", "-A"); err != nil {
			return "", err
		}
		// a machine committer's commit, past the gate under test
		if _, err := git("-c", "core.hooksPath=/dev/null", "-c", "user.email=t@example.com", "-c", "user.name=Test", "commit", "-q", "-m", msg); err != nil {
			return "", err
		}
		return git("rev-parse", "HEAD")
	}
	build := func(name string, flags ...string) (string, error) {
		out := filepath.Join(dir, name, "commit-gate")
		cmd := exec.Command("go", append(append([]string{"build"}, flags...), "-o", out, "./cmd/cg")...)
		cmd.Dir = src
		if b, err := cmd.CombinedOutput(); err != nil {
			return "", &cmdError{"go build " + name, string(b), err}
		}
		return out, nil
	}
	s := scratchBuilds{src: src}
	if _, err := git("init", "-q", "-b", "main"); err != nil {
		return err.Error()
	}
	readme := filepath.Join(src, "README.md")
	steps := []func() error{
		func() (err error) { s.c1, err = commit("c1"); return },
		func() (err error) { s.b1, err = build("b1"); return },
		func() error { return appendTo(readme, "a change\n") },
		func() (err error) { s.dirty, err = build("dirty"); return },
		func() error { _, err := git("checkout", "-q", "README.md"); return err },
		func() error { return appendTo(readme, "c2\n") },
		func() (err error) { s.c2, err = commit("c2"); return },
		func() (err error) { s.b2, err = build("b2"); return },
		func() (err error) { s.noCommit, err = build("nocommit", "-buildvcs=false"); return },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err.Error()
		}
	}
	scratch = s
	return ""
}

type cmdError struct {
	what, out string
	err       error
}

func (e *cmdError) Error() string { return e.what + ": " + e.err.Error() + ": " + e.out }

func appendTo(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// copyWorkingTree copies root's working tree, tracked files and untracked
// ones git doesn't ignore, to dst.
func copyWorkingTree(root, dst string) error {
	out, err := exec.Command("git", "-C", root, "ls-files", "-co", "--exclude-standard", "-z").Output()
	if err != nil {
		return err
	}
	for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		from, to := filepath.Join(root, rel), filepath.Join(dst, rel)
		fi, err := os.Lstat(from)
		if os.IsNotExist(err) {
			continue // deleted from the working tree
		} else if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(from)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, to); err != nil {
				return err
			}
			continue
		}
		b, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		if err := os.WriteFile(to, b, fi.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

// release is a tag and the commit it names on the stand-in public repository.
type release struct{ tag, commit string }

// publishAt points the public address, for the rest of the test, at url in
// a private global git config that also names the gate under test.
func publishAt(t *testing.T, url string) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "gitconfig")
	cfg := "[core]\n\thooksPath = " + filepath.Join(binDir, "hooks") + "\n[url \"" + url + "\"]\n\tinsteadOf = " + publicAddress + "\n"
	if err := os.WriteFile(f, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", f)
}

// publish makes a stand-in public repository holding the scratch source's
// commits and the given tags, lightweight or annotated, and points the
// public address at it.
func publish(t *testing.T, annotated bool, releases ...release) {
	t.Helper()
	s := builds(t)
	pub := filepath.Join(t.TempDir(), "pub.git")
	if out, err := exec.Command("git", "clone", "-q", "--bare", s.src, pub).CombinedOutput(); err != nil {
		t.Fatalf("git clone --bare: %v %s", err, out)
	}
	for _, r := range releases {
		args := []string{"-C", pub, "-c", "user.email=t@example.com", "-c", "user.name=Test", "tag"}
		if annotated {
			args = append(args, "-a", "-m", "release "+r.tag)
		}
		if out, err := exec.Command("git", append(args, r.tag, r.commit)...).CombinedOutput(); err != nil {
			t.Fatalf("git tag %s: %v %s", r.tag, err, out)
		}
	}
	publishAt(t, pub)
}

// version runs build's commit-gate version and returns its combined output
// and exit status.
func version(t *testing.T, build string) (string, int) {
	t.Helper()
	out, errStr, code := runPath(t, t.TempDir(), "", build, "version")
	return out + errStr, code
}

// deployHost is this host's name in a deploy: its short host name, or mac
// on the Mac.
func deployHost(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "darwin" {
		return "mac"
	}
	h, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	return strings.SplitN(h, ".", 2)[0]
}

func assertUpToDate(t *testing.T, out string, code int, build, tag, target string) {
	t.Helper()
	if code != 0 {
		t.Errorf("an up-to-date build must exit 0, got %d: %q", code, out)
	}
	for _, want := range []string{build, tag, target, "up to date"} {
		if !strings.Contains(out, want) {
			t.Errorf("output must contain %q: %q", want, out)
		}
	}
	if strings.Contains(out, "out of date") || strings.Contains(out, "approve-deploy") {
		t.Errorf("an up-to-date build needs no fix: %q", out)
	}
}

// assertOutOfDate: the report names the state and the two-command fix, in order.
func assertOutOfDate(t *testing.T, out string, code int, tag, target string) {
	t.Helper()
	if code == 0 {
		t.Errorf("an out-of-date build must exit non-zero: %q", out)
	}
	for _, want := range []string{tag, target, "out of date"} {
		if !strings.Contains(out, want) {
			t.Errorf("output must contain %q: %q", want, out)
		}
	}
	if strings.Contains(out, "up to date") {
		t.Errorf("an out-of-date build is reported up to date: %q", out)
	}
	approve, deploy := strings.Index(out, "commit-gate approve-deploy"), strings.Index(out, "make deploy HOST="+deployHost(t))
	if approve < 0 || deploy < 0 || approve > deploy {
		t.Errorf("the fix must be commit-gate approve-deploy, then make deploy HOST=%s: %q", deployHost(t), out)
	}
}

func assertUnknown(t *testing.T, out string, code int) {
	t.Helper()
	if code == 0 {
		t.Errorf("an unknown state must exit non-zero: %q", out)
	}
	if !strings.Contains(out, "unknown") {
		t.Errorf("output must name the state unknown: %q", out)
	}
	if strings.Contains(out, "up to date") || strings.Contains(out, "out of date") {
		t.Errorf("a state that can't be known is reported known: %q", out)
	}
}

func TestVersionReportsABuildFromTheTargetUpToDate(t *testing.T) {
	s := builds(t)
	publish(t, false, release{"v1.0.0", s.c1})
	out, code := version(t, s.b1)
	assertUpToDate(t, out, code, s.c1, "v1.0.0", s.c1)
}

func TestVersionReportsABuildOfAnEarlierReleaseOutOfDateWithTheTwoCommandFix(t *testing.T) {
	s := builds(t)
	publish(t, false, release{"v1.0.0", s.c1}, release{"v1.1.0", s.c2})
	out, code := version(t, s.b1)
	assertOutOfDate(t, out, code, "v1.1.0", s.c2)
	if !strings.Contains(out, s.c1) {
		t.Errorf("output must name the build's commit %s: %q", s.c1, out)
	}
}

func TestVersionReportsABuildAheadOfTheTargetOutOfDate(t *testing.T) {
	s := builds(t)
	publish(t, false, release{"v1.0.0", s.c1})
	out, code := version(t, s.b2)
	assertOutOfDate(t, out, code, "v1.0.0", s.c1)
}

func TestVersionReportsABuildFromChangedSourceOutOfDate(t *testing.T) {
	s := builds(t)
	publish(t, false, release{"v1.0.0", s.c1})
	out, code := version(t, s.dirty)
	assertOutOfDate(t, out, code, "v1.0.0", s.c1)
}

func TestVersionReportsABuildThatRecordsNoCommitOutOfDate(t *testing.T) {
	s := builds(t)
	publish(t, false, release{"v1.1.0", s.c2})
	out, code := version(t, s.noCommit)
	assertOutOfDate(t, out, code, "v1.1.0", s.c2)
}

func TestVersionReportsTheCommitAndDirtyFlagItsFileRecords(t *testing.T) {
	s := builds(t)
	publish(t, false, release{"v1.0.0", s.c1})
	for _, c := range []struct {
		build                 string
		wantCommit, wantDirty string
	}{{s.b1, s.c1, "false"}, {s.dirty, s.c1, "true"}} {
		info, err := buildinfo.ReadFile(c.build)
		if err != nil {
			t.Fatal(err)
		}
		recorded := map[string]string{}
		for _, kv := range info.Settings {
			recorded[kv.Key] = kv.Value
		}
		if recorded["vcs.revision"] != c.wantCommit || recorded["vcs.modified"] != c.wantDirty {
			t.Fatalf("%s records commit %q, modified %q; want %q, %q", c.build, recorded["vcs.revision"], recorded["vcs.modified"], c.wantCommit, c.wantDirty)
		}
		out, _ := version(t, c.build)
		if !strings.Contains(out, c.wantCommit) {
			t.Errorf("%s: output must name the recorded commit %s: %q", c.build, c.wantCommit, out)
		}
		if dirty := strings.Contains(out, "dirty"); dirty != (c.wantDirty == "true") {
			t.Errorf("%s: output names the build dirty=%v, its file records modified=%s: %q", c.build, dirty, c.wantDirty, out)
		}
	}
}

func TestVersionReportsUnknownWhenTheTargetCantBeRead(t *testing.T) {
	s := builds(t)
	publishAt(t, filepath.Join(t.TempDir(), "no-such-repository.git"))
	out, code := version(t, s.b1)
	assertUnknown(t, out, code)
}

func TestVersionReportsUnknownWhenNoReleaseExists(t *testing.T) {
	s := builds(t)
	publish(t, false)
	out, code := version(t, s.b1)
	assertUnknown(t, out, code)

	publish(t, false, release{"latest", s.c1}, release{"v1.0", s.c1}, release{"v1.0.0-rc1", s.c1})
	out, code = version(t, s.b1)
	assertUnknown(t, out, code)
}

func TestVersionTakesTheHighestReleaseTagBySemverNotByText(t *testing.T) {
	s := builds(t)
	publish(t, false, release{"v1.9.0", s.c1}, release{"v1.10.0", s.c2})
	out, code := version(t, s.b2)
	assertUpToDate(t, out, code, s.c2, "v1.10.0", s.c2)
	out, code = version(t, s.b1)
	assertOutOfDate(t, out, code, "v1.10.0", s.c2)
}

func TestVersionIgnoresTagsThatAreNotReleases(t *testing.T) {
	s := builds(t)
	publish(t, false, release{"v1.0.0", s.c1}, release{"v2.0.0-rc1", s.c2}, release{"latest", s.c2}, release{"v3", s.c2})
	out, code := version(t, s.b1)
	assertUpToDate(t, out, code, s.c1, "v1.0.0", s.c1)
}

func TestVersionReadsAnAnnotatedReleaseTagAsTheCommitItNames(t *testing.T) {
	s := builds(t)
	publish(t, true, release{"v1.0.0", s.c1})
	out, code := version(t, s.b1)
	assertUpToDate(t, out, code, s.c1, "v1.0.0", s.c1)
}
