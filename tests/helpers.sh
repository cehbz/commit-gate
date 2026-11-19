fail() { echo "FAIL: $*" >&2; exit 1; }
assert_eq() { [ "$1" = "$2" ] || fail "${3:-assert_eq}: got [$1] want [$2]"; }
mkrepo() {
  local d; d="$(mktemp -d)"
  git -C "$d" init -q
  git -C "$d" config user.email t@example.com
  git -C "$d" config user.name 'Test'
  printf '%s' "$d"
}
# Every git a test runs reads a temporary global config, never the user's:
# its core.hooksPath names the gate under test, as install.sh makes the
# user's ~/.gitconfig do.
# Run from a hook, git's exported repository, index and config variables
# would aim the tests' git at the caller's repo (the Go suites' copy of this
# list is internal/testenv.CallerRepoEnv).
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_COMMON_DIR GIT_OBJECT_DIRECTORY \
  GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX GIT_CONFIG_PARAMETERS GIT_CONFIG_COUNT
export GIT_CONFIG_NOSYSTEM=1
GIT_CONFIG_GLOBAL="$(mktemp)"; export GIT_CONFIG_GLOBAL
gate_hooks="$(cd "$DIR/../hooks" 2>/dev/null && pwd -P)" || gate_hooks=""
[ -z "$gate_hooks" ] || printf '[core]\n\thooksPath = %s\n' "$gate_hooks" > "$GIT_CONFIG_GLOBAL"
# no_global_gate: the rest of the test runs on a host whose global git
# config names no core.hooksPath.
no_global_gate() { : > "$GIT_CONFIG_GLOBAL"; }
