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
// order, and every error the walk met: a root that doesn't exist or isn't
// a directory, and each directory it couldn't read. A root that is a
// symlink is resolved and its repos reported under the root as given;
// below the root it follows no symlinks. It doesn't descend into .git, and
// skips node_modules, .cache and .Trash anywhere and Library and go/pkg
// directly under root.
func findRepos(root string) ([]string, []error) {
	walkRoot := root
	if fi, err := os.Lstat(root); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
		r, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, []error{err}
		}
		walkRoot = r
	}
	skipPaths := map[string]bool{filepath.Join(walkRoot, "Library"): true, filepath.Join(walkRoot, "go", "pkg"): true}
	var dirs []string
	var errs []error
	filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if path == walkRoot && !d.IsDir() {
			errs = append(errs, fmt.Errorf("%s: not a directory", root))
			return nil
		}
		if d.Name() == ".git" {
			dir := filepath.Dir(path)
			if walkRoot != root { // report it under the symlink given
				rel, _ := filepath.Rel(walkRoot, dir)
				dir = filepath.Join(root, rel)
			}
			dirs = append(dirs, dir)
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() && path != walkRoot && (skipNames[d.Name()] || skipPaths[path]) {
			return fs.SkipDir
		}
		return nil
	})
	return dirs, errs
}

// cmdClearLocalHookspath implements `commit-gate clear-local-hookspath
// [root...]` (default $HOME). It leaves a repo's gate path, as an error,
// while the global core.hooksPath git reads in that repo is not the gate:
// removing it would then ungate the repo. A relative value is resolved against the
// repo's work tree, as git resolves it. An empty value names no gate
// directory, so it is reported as foreign, quoted. Every error it meets
// goes to stderr and into the summary, and any makes it exit 1.
func cmdClearLocalHookspath(args []string) int {
	hooks := liveHooksDir()
	roots := args
	if len(roots) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return dieApprove("%s", err)
		}
		roots = []string{home}
	}
	seen := map[string]bool{}
	removed, foreign, failed := 0, 0, 0
	fail := func(err error) {
		fmt.Fprintf(os.Stderr, "commit-gate: %s\n", err)
		failed++
	}
	for _, root := range roots {
		dirs, errs := findRepos(root)
		for _, err := range errs {
			fail(err)
		}
		for _, dir := range dirs {
			repo, err := gatestate.Open(dir)
			if err != nil {
				fail(fmt.Errorf("%s: %w", dir, err))
				continue
			}
			if seen[repo.Common] {
				continue
			}
			seen[repo.Common] = true
			hp, scope, ok, err := repo.HooksPath()
			if err != nil {
				fail(fmt.Errorf("%s: %w", dir, err))
				continue
			}
			if !ok || scope != "local" {
				continue
			}
			// dir holds the .git entry: it is the work tree's top.
			resolved := hp
			if hp != "" && !filepath.IsAbs(hp) {
				resolved = filepath.Join(dir, hp)
			}
			if hp == "" || !isGateDir(resolved, hooks) {
				shown := hp
				if hp == "" {
					shown = `""`
				}
				fmt.Printf("foreign %s: core.hooksPath is %s (left)\n", dir, shown)
				foreign++
				continue
			}
			if p := gatestate.GlobalGateProblem(dir, hooks); p != "" {
				fail(fmt.Errorf("%s: %s; its gate path is left, or this would ungate it: %s", dir, p, gatestate.GlobalGateFix(dir, hooks)))
				continue
			}
			if err := repo.GitConfigUnset("core.hooksPath"); err != nil {
				fail(fmt.Errorf("%s: %w", dir, err))
				continue
			}
			fmt.Printf("removed %s: core.hooksPath was %s\n", dir, hp)
			removed++
		}
	}
	if failed == 0 {
		fmt.Printf("commit-gate: %d removed, %d foreign\n", removed, foreign)
		return 0
	}
	noun := "errors"
	if failed == 1 {
		noun = "error"
	}
	fmt.Printf("commit-gate: %d removed, %d foreign, %d %s\n", removed, foreign, failed, noun)
	return 1
}
