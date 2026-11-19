package main_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkrepoAt(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "-q")
	return dir
}

func TestClearLocalHookspathRemovesGateDirsAndReportsOthers(t *testing.T) {
	root := t.TempDir()
	gate := filepath.Join(binDir, "hooks")
	stale := "/nonexistent/old/commit-gate/hooks"
	cur := mkrepoAt(t, filepath.Join(root, "a-current"))
	gitSetConfig(t, cur, "core.hooksPath", gate)
	husky := mkrepoAt(t, filepath.Join(root, "b-husky"))
	gitSetConfig(t, husky, "core.hooksPath", ".husky/_")
	old := mkrepoAt(t, filepath.Join(root, "c-stale"))
	gitSetConfig(t, old, "core.hooksPath", stale)
	mkrepoAt(t, filepath.Join(root, "d-plain"))
	vendored := mkrepoAt(t, filepath.Join(root, "e", "node_modules", "pkg"))
	gitSetConfig(t, vendored, "core.hooksPath", gate)

	out, errStr, code := run(t, "", "commit-gate", "clear-local-hookspath", root)
	want := "removed " + cur + ": core.hooksPath was " + gate + "\n" +
		"foreign " + husky + ": core.hooksPath is .husky/_ (left)\n" +
		"removed " + old + ": core.hooksPath was " + stale + "\n" +
		"commit-gate: 2 removed, 1 foreign\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d stderr=%q\ngot:\n%s\nwant:\n%s", code, errStr, out, want)
	}
	for _, r := range []string{cur, old} {
		if hp := localHooksPath(t, r); hp != "" {
			t.Fatalf("%s: core.hooksPath must be removed, got %q", r, hp)
		}
	}
	if hp := localHooksPath(t, husky); hp != ".husky/_" {
		t.Fatalf("a foreign core.hooksPath must stay: %q", hp)
	}
	if hp := localHooksPath(t, vendored); hp != gate {
		t.Fatalf("node_modules must be skipped: %q", hp)
	}
}

// An empty repo-local core.hooksPath names no gate directory: it is reported
// as foreign, quoted so it is visible, and left.
func TestClearLocalHookspathReportsAnEmptyValueAsForeign(t *testing.T) {
	root := t.TempDir()
	repo := mkrepoAt(t, filepath.Join(root, "r"))
	gitSetConfig(t, repo, "core.hooksPath", "")

	out, errStr, code := run(t, "", "commit-gate", "clear-local-hookspath", root)
	want := "foreign " + repo + `: core.hooksPath is "" (left)` + "\n" +
		"commit-gate: 0 removed, 1 foreign\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d stderr=%q\ngot:\n%s\nwant:\n%s", code, errStr, out, want)
	}
	if got := gitIn(t, repo, "config", "--local", "--get-all", "core.hooksPath"); got != "" {
		t.Fatalf("the empty value must stay: %q", got)
	}
}

// With no root the walk starts at $HOME, skips $HOME/Library, and reports a
// repo once however many worktrees share its config.
func TestClearLocalHookspathDefaultsToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	gate := filepath.Join(binDir, "hooks")
	main := mkrepoAt(t, filepath.Join(home, "p", "main"))
	c := gitIn(t, main, "-c", "user.email=t@example.com", "-c", "user.name=T",
		"commit-tree", "-m", "x", gitIn(t, main, "write-tree"))
	gitIn(t, main, "update-ref", "HEAD", c)
	gitIn(t, main, "worktree", "add", "-q", filepath.Join(home, "p", "wt"))
	gitSetConfig(t, main, "core.hooksPath", ".husky/_")
	lib := mkrepoAt(t, filepath.Join(home, "Library", "x"))
	gitSetConfig(t, lib, "core.hooksPath", gate)

	out, errStr, code := run(t, "", "commit-gate", "clear-local-hookspath")
	want := "foreign " + main + ": core.hooksPath is .husky/_ (left)\n" +
		"commit-gate: 0 removed, 1 foreign\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d stderr=%q\ngot:\n%s\nwant:\n%s", code, errStr, out, want)
	}
	if hp := localHooksPath(t, lib); hp != gate {
		t.Fatalf("$HOME/Library must be skipped: %q", hp)
	}
}

func TestClearLocalHookspathRefusesWithoutTheGlobalGate(t *testing.T) {
	noGlobalGate(t)
	root := t.TempDir()
	gate := filepath.Join(binDir, "hooks")
	repo := mkrepoAt(t, filepath.Join(root, "r"))
	gitSetConfig(t, repo, "core.hooksPath", gate)
	_, errStr, code := run(t, "", "commit-gate", "clear-local-hookspath", root)
	if code != 1 || !strings.Contains(errStr, "the global core.hooksPath is unset") {
		t.Fatalf("must refuse: code=%d stderr=%q", code, errStr)
	}
	if hp := localHooksPath(t, repo); hp != gate {
		t.Fatalf("a refusal must change nothing: %q", hp)
	}
}

func TestClearLocalHookspathReportsAnAbsoluteForeignValue(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir()
	repo := mkrepoAt(t, filepath.Join(root, "r"))
	gitSetConfig(t, repo, "core.hooksPath", elsewhere)
	out, errStr, code := run(t, "", "commit-gate", "clear-local-hookspath", root)
	want := "foreign " + repo + ": core.hooksPath is " + elsewhere + " (left)\n" +
		"commit-gate: 0 removed, 1 foreign\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d stderr=%q\ngot:\n%s\nwant:\n%s", code, errStr, out, want)
	}
	if hp := localHooksPath(t, repo); hp != elsewhere {
		t.Fatalf("a foreign core.hooksPath must stay: %q", hp)
	}
}

// git resolves a relative core.hooksPath against the repo's work tree, so
// "hooks" names <repo>/hooks wherever the one-off runs from, even from the
// directory whose hooks is the gate.
func TestClearLocalHookspathResolvesARelativeValueAgainstTheRepo(t *testing.T) {
	root := t.TempDir()
	repo := mkrepoAt(t, filepath.Join(root, "r"))
	gitSetConfig(t, repo, "core.hooksPath", "hooks")
	out, errStr, code := runIn(t, binDir, "", "commit-gate", "clear-local-hookspath", root)
	want := "foreign " + repo + ": core.hooksPath is hooks (left)\n" +
		"commit-gate: 0 removed, 1 foreign\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d stderr=%q\ngot:\n%s\nwant:\n%s", code, errStr, out, want)
	}
	if hp := localHooksPath(t, repo); hp != "hooks" {
		t.Fatalf("a relative value naming no gate must stay: %q", hp)
	}
}

func TestClearLocalHookspathReportsAMissingRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "typo")
	out, errStr, code := run(t, "", "commit-gate", "clear-local-hookspath", missing)
	if code == 0 || !strings.Contains(errStr, missing) || out != "commit-gate: 0 removed, 0 foreign, 1 error\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errStr)
	}
}

func TestClearLocalHookspathCountsAnUnreadableDirectory(t *testing.T) {
	root := t.TempDir()
	gate := filepath.Join(binDir, "hooks")
	repo := mkrepoAt(t, filepath.Join(root, "a"))
	gitSetConfig(t, repo, "core.hooksPath", gate)
	locked := filepath.Join(root, "b-locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	out, errStr, code := run(t, "", "commit-gate", "clear-local-hookspath", root)
	want := "removed " + repo + ": core.hooksPath was " + gate + "\n" +
		"commit-gate: 1 removed, 0 foreign, 1 error\n"
	if code == 0 || out != want || !strings.Contains(errStr, locked) {
		t.Fatalf("code=%d stderr=%q\ngot:\n%s\nwant:\n%s", code, errStr, out, want)
	}
}

func TestClearLocalHookspathCountsAFailedRead(t *testing.T) {
	root := t.TempDir()
	repo := mkrepoAt(t, filepath.Join(root, "r"))
	gitSetConfig(t, repo, "core.hooksPath", "~nosuchuser-commit-gate/hooks")
	out, errStr, code := run(t, "", "commit-gate", "clear-local-hookspath", root)
	if code == 0 || out != "commit-gate: 0 removed, 0 foreign, 1 error\n" || !strings.Contains(errStr, repo) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errStr)
	}
}

// A relative value that names the gate from the repo's work tree is a gate
// path, whatever directory the one-off runs from.
func TestClearLocalHookspathRemovesARelativeValueNamingTheGate(t *testing.T) {
	root := t.TempDir()
	repo := mkrepoAt(t, filepath.Join(root, "r"))
	rel, err := filepath.Rel(repo, filepath.Join(binDir, "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	gitSetConfig(t, repo, "core.hooksPath", rel)
	out, errStr, code := run(t, "", "commit-gate", "clear-local-hookspath", root)
	want := "removed " + repo + ": core.hooksPath was " + rel + "\n" +
		"commit-gate: 1 removed, 0 foreign\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d stderr=%q\ngot:\n%s\nwant:\n%s", code, errStr, out, want)
	}
}

// A root that is a symlink to a directory (/tmp on macOS) is walked, and its
// repos are reported under the root as given.
func TestClearLocalHookspathWalksASymlinkedRoot(t *testing.T) {
	real := t.TempDir()
	gate := filepath.Join(binDir, "hooks")
	mkrepoAt(t, filepath.Join(real, "r"))
	gitSetConfig(t, filepath.Join(real, "r"), "core.hooksPath", gate)
	root := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, root); err != nil {
		t.Fatal(err)
	}
	out, errStr, code := run(t, "", "commit-gate", "clear-local-hookspath", root)
	want := "removed " + filepath.Join(root, "r") + ": core.hooksPath was " + gate + "\n" +
		"commit-gate: 1 removed, 0 foreign\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d stderr=%q\ngot:\n%s\nwant:\n%s", code, errStr, out, want)
	}
}

func TestClearLocalHookspathReportsASymlinkToAFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(f, root); err != nil {
		t.Fatal(err)
	}
	out, errStr, code := run(t, "", "commit-gate", "clear-local-hookspath", root)
	if code == 0 || out != "commit-gate: 0 removed, 0 foreign, 1 error\n" || !strings.Contains(errStr, root+": not a directory") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errStr)
	}
}

// A root given as "." or with a trailing slash reports its repos at clean
// paths under it.
func TestClearLocalHookspathReportsCleanPathsForAnUncleanRoot(t *testing.T) {
	gate := filepath.Join(binDir, "hooks")
	base := t.TempDir()
	gitSetConfig(t, mkrepoAt(t, filepath.Join(base, "a")), "core.hooksPath", gate)
	out, errStr, code := runIn(t, base, "", "commit-gate", "clear-local-hookspath", ".")
	want := "removed a: core.hooksPath was " + gate + "\n" + "commit-gate: 1 removed, 0 foreign\n"
	if code != 0 || out != want {
		t.Fatalf("root .: code=%d stderr=%q\ngot:\n%s\nwant:\n%s", code, errStr, out, want)
	}
	repo := mkrepoAt(t, filepath.Join(t.TempDir(), "r"))
	gitSetConfig(t, repo, "core.hooksPath", gate)
	out, errStr, code = run(t, "", "commit-gate", "clear-local-hookspath", repo+"/")
	want = "removed " + repo + ": core.hooksPath was " + gate + "\n" + "commit-gate: 1 removed, 0 foreign\n"
	if code != 0 || out != want {
		t.Fatalf("trailing slash: code=%d stderr=%q\ngot:\n%s\nwant:\n%s", code, errStr, out, want)
	}
}
