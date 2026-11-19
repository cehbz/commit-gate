package testenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// With git's hook variables aimed at a decoy repo, as when go test runs from
// a hook, the git a suite runs after ClearCallerRepo leaves the decoy alone.
func TestClearCallerRepoKeepsGitOffTheCallersRepo(t *testing.T) {
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global) // never the user's
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	decoy := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", decoy).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	cfg := filepath.Join(decoy, ".git", "config")
	before, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
	t.Setenv("GIT_WORK_TREE", decoy)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(decoy, ".git", "index"))
	ClearCallerRepo()
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "Test"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if after, _ := os.ReadFile(cfg); string(after) != string(before) {
		t.Fatalf("the caller's repo config changed:\n%s", after)
	}
}
