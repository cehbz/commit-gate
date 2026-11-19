// Package testenv holds what the test suites of packages that run git share.
package testenv

import "os"

// CallerRepoEnv are the variables git exports to a hook that aim a git
// command at a repository, index or config. Run from a hook (go test in a
// pre-commit), they would point a suite's git init and git config, and the
// gate binary under test, at the caller's repo. tests/helpers.sh unsets the
// same list for the shell suite.
var CallerRepoEnv = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR",
	"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_PREFIX",
	"GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT",
}

// ClearCallerRepo unsets CallerRepoEnv; a TestMain calls it first.
func ClearCallerRepo() {
	for _, v := range CallerRepoEnv {
		os.Unsetenv(v)
	}
}
