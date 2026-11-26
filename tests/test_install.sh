#!/bin/bash
# install.sh builds the binary whatever directory it is called from, links
# every hook git runs, and points the global core.hooksPath at the gate
# unless it names another directory.
set -eu
DIR="$(cd "$(dirname "$0")" && pwd -P)"; . "$DIR/helpers.sh"; . "$DIR/pending.sh"
INSTALL="$DIR/../install.sh"
[ -f "$INSTALL" ] || INSTALL="$DIR/../../install.sh"   # run from build/tests
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
elsewhere="$(mkrepo)"
GIT_CONFIG_GLOBAL="$tmp/gitconfig"; export GIT_CONFIG_GLOBAL; : > "$GIT_CONFIG_GLOBAL"
run_install() {
  ( cd "$elsewhere" && CLAUDE_DIR="$tmp/claude" BIN_DIR="${bin_dir:-$tmp/bin}" LIBEXEC="$tmp/libexec" bash "$INSTALL" ) >"$tmp/out" 2>&1 \
    || fail "install.sh from another repo's directory: $(cat "$tmp/out")"
}

run_install
[ -x "$tmp/libexec/commit-gate" ] || fail "no binary built"
for h in $("$tmp/libexec/commit-gate" hook-names); do
  [ -L "$tmp/libexec/hooks/$h" ] || fail "hook $h not linked"
done
[ -L "$tmp/libexec/hooks/pre-commit" ] || fail 'pre-commit pass-through not linked'
assert_eq "$(git config --global --get core.hooksPath)" "$tmp/libexec/hooks" 'install.sh should set the global core.hooksPath'

run_install
grep -q 'already names the gate' "$tmp/out" || fail 'a re-run should keep the global core.hooksPath'
assert_eq "$(git config --global --get core.hooksPath)" "$tmp/libexec/hooks" 'a re-run should keep the global core.hooksPath'

other="$tmp/other-hooks"; mkdir -p "$other"; git config --global core.hooksPath "$other"
run_install
assert_eq "$(git config --global --get core.hooksPath)" "$other" 'install.sh must leave a foreign global core.hooksPath'
grep -q "core.hooksPath is $other, not the gate; left as it is" "$tmp/out" || fail 'a foreign global core.hooksPath should be reported'
grep -q "git config --global core.hooksPath $tmp/libexec/hooks" "$tmp/out" || fail 'the report should carry the fix'
# A global core.hooksPath set in a file the global config includes is set.
printf '[core]\n\thooksPath = %s\n' "$tmp/libexec/hooks" > "$tmp/hooks.inc"
printf '[include]\n\tpath = %s\n' "$tmp/hooks.inc" > "$GIT_CONFIG_GLOBAL"
run_install
grep -q 'already names the gate' "$tmp/out" || fail 'an included global core.hooksPath naming the gate should read as set'
[ -z "$(git config --global --no-includes --get-all core.hooksPath)" ] || fail 'install.sh must not add a second core.hooksPath beside an included one'

install_links_commit_gate_in_the_bin_dir_and_no_old_name() {
  bin_dir="$tmp/fresh-bin" run_install
  assert_eq "$(ls -A "$tmp/fresh-bin")" commit-gate 'a fresh bin directory should hold commit-gate alone'
  [ -L "$tmp/fresh-bin/commit-gate" ] || fail "commit-gate in the bin directory is not a link"
  [ "$tmp/fresh-bin/commit-gate" -ef "$tmp/libexec/commit-gate" ] || fail "the bin directory's commit-gate is not the installed binary"
}
pending install_links_commit_gate_in_the_bin_dir_and_no_old_name
rm -rf "$elsewhere"
echo "OK: test_install"
