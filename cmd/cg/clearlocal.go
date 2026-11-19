// clearlocal.go is a one-off for moving a host from per-repo enabling to
// the global core.hooksPath: it removes each repo-local core.hooksPath
// naming a gate directory, current or stale, and reports every other
// repo-local core.hooksPath. Delete it once every host has run it.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cehbz/commit-gate/gatestate"
)

// isGateDir reports whether a core.hooksPath value names a commit-gate hook
// directory: the live one, or any path ending in commit-gate/hooks, which
// every install layout has used, including ones since removed.
func isGateDir(hp, live string) bool {
	if gatestate.SameDir(hp, live) {
		return true
	}
	c := filepath.Clean(hp)
	return filepath.Base(c) == "hooks" && filepath.Base(filepath.Dir(c)) == "commit-gate"
}

var skipNames = map[string]bool{"node_modules": true, ".cache": true, ".Trash": true}

// findRepos returns the directory holding each entry named .git under root
// (a directory, or a file in submodules and linked worktrees), in walk
// order. It follows no symlinks, doesn't descend into .git, and skips
// node_modules, .cache and .Trash anywhere and Library and go/pkg directly
// under root. Unreadable directories are skipped.
func findRepos(root string) []string {
	skipPaths := map[string]bool{filepath.Join(root, "Library"): true, filepath.Join(root, "go", "pkg"): true}
	var dirs []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == ".git" {
			dirs = append(dirs, filepath.Dir(path))
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() && path != root && (skipNames[d.Name()] || skipPaths[path]) {
			return fs.SkipDir
		}
		return nil
	})
	return dirs
}

// cmdClearLocalHookspath implements `commit-gate clear-local-hookspath
// [root...]` (default $HOME). It refuses, changing nothing, while the
// global core.hooksPath is not the gate: removing a repo-local gate path
// would then ungate the repo. An empty repo-local value names no gate
// directory, so it is reported as foreign, quoted.
func cmdClearLocalHookspath(args []string) int {
	hooks := liveHooksDir()
	if p := gatestate.GlobalGateProblem(hooks); p != "" {
		return dieApprove("%s; set it first, or this would ungate repos: git config --global core.hooksPath %s", p, hooks)
	}
	roots := args
	if len(roots) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return dieApprove("%s", err)
		}
		roots = []string{home}
	}
	seen := map[string]bool{}
	removed, foreign := 0, 0
	for _, root := range roots {
		for _, dir := range findRepos(root) {
			repo, err := gatestate.Open(dir)
			if err != nil || seen[repo.Common] {
				continue
			}
			seen[repo.Common] = true
			hp, scope, ok, err := repo.HooksPath()
			if err != nil {
				fmt.Fprintf(os.Stderr, "commit-gate: %s: %s\n", dir, err)
				continue
			}
			if !ok || scope != "local" {
				continue
			}
			if hp == "" {
				fmt.Printf("foreign %s: core.hooksPath is %q (left)\n", dir, hp)
				foreign++
				continue
			}
			if !isGateDir(hp, hooks) {
				fmt.Printf("foreign %s: core.hooksPath is %s (left)\n", dir, hp)
				foreign++
				continue
			}
			if err := repo.GitConfigUnset("core.hooksPath"); err != nil {
				fmt.Fprintf(os.Stderr, "commit-gate: %s: %s\n", dir, err)
				continue
			}
			fmt.Printf("removed %s: core.hooksPath was %s\n", dir, hp)
			removed++
		}
	}
	fmt.Printf("commit-gate: %d removed, %d foreign\n", removed, foreign)
	return 0
}
