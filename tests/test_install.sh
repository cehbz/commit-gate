#!/bin/bash
# commit-gate install puts an up-to-date build live and wires the host to it
# at fixed locations under the home directory (DESIGN.md, Install). The
# stand-in target is a committed, tagged copy of the working tree, which the
# private global git config of each temporary home names as the public
# address; the build under test is an ordinary build of that copy.
set -eu
DIR="$(cd "$(dirname "$0")" && pwd -P)"; . "$DIR/helpers.sh"; . "$DIR/pending.sh"; . "$DIR/scratch.sh"
tmp="$(mktemp -d)"; tmp="$(cd "$tmp" && pwd -P)"; trap 'chmod -R u+w "$tmp" 2>/dev/null; rm -rf "$tmp"' EXIT

# prepare, on first use: src, a copy of the working tree committed as c1 and
# then c2; b1, a build of c1; bdirty, a build of c1 with an uncommitted change.
prepare() {
  [ -z "${prepared:-}" ] || return 0
  src="$tmp/src"; mkdir "$src"; scratch_tree "$src"; git -C "$src" init -q -b main
  scratch_commit "$src" 'c1'; c1="$(git -C "$src" rev-parse HEAD)"
  scratch_build "$src" "$tmp/b1"
  echo 'a change' >> "$src/README.md"; scratch_build "$src" "$tmp/bdirty"; git -C "$src" checkout -q README.md
  echo 'c2' >> "$src/README.md"; scratch_commit "$src" 'c2'; c2="$(git -C "$src" rev-parse HEAD)"
  [ "$(build_commit "$tmp/b1/commit-gate")" = "$c1" ] && [ "$(build_dirty "$tmp/b1/commit-gate")" = false ] \
    && [ "$(build_dirty "$tmp/bdirty/commit-gate")" = true ] || fail 'setup: the scratch builds do not record their source'
  prepared=1
}

# publish <tag>=<commit>...: a new stand-in public repository holding src's
# commits and those tags, in $pub.
publish() {
  pub="$(mktemp -d "$tmp/pub.XXXXXX")"; git clone -q --bare "$src" "$pub"
  local tc; for tc in "$@"; do git -C "$pub" tag "${tc%%=*}" "${tc#*=}"; done
}

# new_home <repository>: a new temporary home in $H, whose global git config
# points the public address at repository and names no core.hooksPath: a host
# with no gate. L is its install directory.
new_home() {
  scratch_home "$tmp"; L="$H/.local/libexec/commit-gate"
  printf '[url "%s"]\n\tinsteadOf = %s\n' "$1" "$PUBLIC_URL" > "$H/.gitconfig"
}
home_git() { HOME="$H" GIT_CONFIG_GLOBAL="$H/.gitconfig" git "$@"; }

# run_install <build>: run the build's install under the home $H, from a
# directory outside any repository; its output is in $tmp/out.
run_install() { ( cd "$tmp" && HOME="$H" GIT_CONFIG_GLOBAL="$H/.gitconfig" "$1" install ) > "$tmp/out" 2>&1; }

# hook_count <event> <command>: how many of settings.json's <event> hooks run command.
hook_count() { jq --arg c "$2" "[.hooks.$1[]?.hooks[]? | select(.command == \$c)] | length" "$H/.claude/settings.json"; }

assert_installed() {  # the build $1 is live in $L and the host is wired to it
  [ -f "$L/commit-gate" ] && [ ! -L "$L/commit-gate" ] && [ -x "$L/commit-gate" ] || fail "no live binary in $L: $(cat "$tmp/out")"
  cmp -s "$L/commit-gate" "$1" || fail 'the live binary is not the build that ran install'
  local l h
  for l in precheck sessioncheck; do [ "$L/$l" -ef "$L/commit-gate" ] || fail "$L/$l does not run the live binary"; done
  for h in $("$L/commit-gate" hook-names); do [ "$L/hooks/$h" -ef "$L/commit-gate" ] || fail "hook $h is not linked to the live binary"; done
  [ -L "$H/.local/bin/commit-gate" ] && [ "$H/.local/bin/commit-gate" -ef "$L/commit-gate" ] || fail 'commit-gate on PATH is not a link to the live binary'
  [ -f "$H/.claude/settings.json" ] || fail 'no ~/.claude/settings.json'
  assert_eq "$(hook_count PreToolUse "$L/precheck")" 1 'one PreToolUse hook running the precheck'
  assert_eq "$(hook_count SessionStart "$L/sessioncheck")" 1 'one SessionStart hook running the session check'
  assert_eq "$(jq -r --arg c "$L/precheck" '.hooks.PreToolUse[] | select(any(.hooks[]?; .command == $c)) | .matcher | split("|") | contains(["Bash","Write","Edit","MultiEdit","NotebookEdit"])' "$H/.claude/settings.json")" true \
    'the precheck runs on Bash and the file-editing tools'
}

# assert_install_refused <build> <what>: install refuses and changes nothing under home.
assert_install_refused() {
  local before; before="$(snapshot "$H")"
  if run_install "$1"; then fail "install of $2 went through"; fi
  assert_eq "$(snapshot "$H")" "$before" "a refused install ($2) changed the home directory"
}

install_puts_an_up_to_date_build_live_and_wires_the_host_under_home() {
  prepare; publish v1.0.0="$c1"; new_home "$pub"
  run_install "$tmp/b1/commit-gate" || fail "install of an up-to-date build failed: $(cat "$tmp/out")"
  assert_installed "$tmp/b1/commit-gate"
  assert_eq "$(home_git config --global --type=path --get core.hooksPath)" "$L/hooks" 'the global core.hooksPath names the gate'
}

install_takes_no_location_from_where_the_running_build_sits() {
  prepare; publish v1.0.0="$c1"; new_home "$pub"
  away="$(mktemp -d "$tmp/away.XXXXXX")/deep"; mkdir -p "$away"; cp "$tmp/b1/commit-gate" "$away/"
  run_install "$away/commit-gate" || fail "install from $away failed: $(cat "$tmp/out")"
  assert_eq "$(ls -A "$away")" commit-gate 'install wrote beside the running build'
  assert_installed "$away/commit-gate"
  case "$(readlink "$H/.local/bin/commit-gate")" in "$away"*) fail 'commit-gate on PATH names the running build';; esac
}

install_from_staging_puts_the_build_live_in_the_install_directory() {
  prepare; publish v1.0.0="$c1"; new_home "$pub"
  mkdir -p "$L/staging"; cp "$tmp/b1/commit-gate" "$L/staging/commit-gate"
  run_install "$L/staging/commit-gate" || fail "install from staging failed: $(cat "$tmp/out")"
  assert_installed "$tmp/b1/commit-gate"
  [ ! -e "$L/staging/hooks" ] || fail 'install linked hooks in staging'
  assert_eq "$(home_git config --global --type=path --get core.hooksPath)" "$L/hooks" 'the global core.hooksPath names the install directory, not staging'
  case "$(readlink "$H/.local/bin/commit-gate")" in *staging*) fail 'commit-gate on PATH names the staged build';; esac
}

install_replaces_the_live_binary_in_one_step() {
  prepare; publish v1.0.0="$c1"; new_home "$pub"
  mkdir -p "$L"; printf '#!/bin/sh\nexit 1\n' > "$L/commit-gate"; chmod +x "$L/commit-gate"
  ln "$L/commit-gate" "$H/old-build"   # the old live file, by another name
  run_install "$tmp/b1/commit-gate" || fail "install over an old build failed: $(cat "$tmp/out")"
  assert_installed "$tmp/b1/commit-gate"
  assert_eq "$(cat "$H/old-build")" "$(printf '#!/bin/sh\nexit 1')" 'the old live binary was rewritten in place, not replaced in one step'
}

install_leaves_a_foreign_global_hooks_path_and_reports_it() {
  prepare; publish v1.0.0="$c1"; new_home "$pub"
  other="$H/other-hooks"; mkdir -p "$other"; home_git config --global core.hooksPath "$other"
  run_install "$tmp/b1/commit-gate" || true
  assert_eq "$(home_git config --global --get-all core.hooksPath)" "$other" 'install must leave a foreign global core.hooksPath'
  grep -qF "$other" "$tmp/out" || fail "the foreign global core.hooksPath is not reported: $(cat "$tmp/out")"
  grep -qF "git config --global core.hooksPath $L/hooks" "$tmp/out" || fail "the report lacks the command that gates every repo: $(cat "$tmp/out")"
}

install_run_again_keeps_one_hooks_path_and_one_of_each_claude_hook() {
  prepare; publish v1.0.0="$c1"; new_home "$pub"
  run_install "$tmp/b1/commit-gate" || fail "first install failed: $(cat "$tmp/out")"
  run_install "$tmp/b1/commit-gate" || fail "second install failed: $(cat "$tmp/out")"
  assert_installed "$tmp/b1/commit-gate"
  assert_eq "$(home_git config --global --get-all core.hooksPath | wc -l | tr -d ' ')" 1 'one global core.hooksPath after a second install'
}

install_reads_an_included_global_hooks_path_naming_the_gate_as_set() {
  prepare; publish v1.0.0="$c1"; new_home "$pub"
  printf '[core]\n\thooksPath = %s\n' "$L/hooks" > "$H/hooks.inc"; home_git config --global include.path "$H/hooks.inc"
  run_install "$tmp/b1/commit-gate" || fail "install failed: $(cat "$tmp/out")"
  assert_installed "$tmp/b1/commit-gate"
  [ -z "$(home_git config --global --no-includes --get-all core.hooksPath)" ] || fail 'install added a second core.hooksPath beside the included one'
}

install_keeps_the_rest_of_claude_settings() {
  prepare; publish v1.0.0="$c1"; new_home "$pub"
  mkdir -p "$H/.claude"
  printf '%s\n' '{"model":"opus","hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"/other/hook"}]}]}}' > "$H/.claude/settings.json"
  run_install "$tmp/b1/commit-gate" || fail "install failed: $(cat "$tmp/out")"
  assert_installed "$tmp/b1/commit-gate"
  assert_eq "$(jq -r .model "$H/.claude/settings.json")" opus 'install dropped a setting'
  assert_eq "$(hook_count PreToolUse /other/hook)" 1 'install dropped another PreToolUse hook'
}

install_links_commit_gate_alone_in_a_fresh_bin_directory() {
  prepare; publish v1.0.0="$c1"; new_home "$pub"
  run_install "$tmp/b1/commit-gate" || fail "install failed: $(cat "$tmp/out")"
  assert_eq "$(ls -A "$H/.local/bin")" commit-gate 'a fresh bin directory should hold commit-gate alone'
}

install_refuses_an_out_of_date_build_and_changes_nothing() {
  prepare; publish v1.0.0="$c1" v1.1.0="$c2"; new_home "$pub"
  assert_install_refused "$tmp/b1/commit-gate" 'a build of an earlier release'
}

install_refuses_a_dirty_build_and_changes_nothing() {
  prepare; publish v1.0.0="$c1"; new_home "$pub"
  assert_install_refused "$tmp/bdirty/commit-gate" 'a build from changed source'
}

install_refuses_when_the_target_cant_be_read_and_changes_nothing() {
  prepare; new_home "$tmp/no-such-repository.git"
  assert_install_refused "$tmp/b1/commit-gate" 'a build whose target cannot be read'
}

install_refuses_when_no_release_exists_and_changes_nothing() {
  prepare; publish; new_home "$pub"
  assert_install_refused "$tmp/b1/commit-gate" 'a build with no release to measure against'
}

install_puts_an_up_to_date_build_live_and_wires_the_host_under_home
install_takes_no_location_from_where_the_running_build_sits
install_from_staging_puts_the_build_live_in_the_install_directory
install_replaces_the_live_binary_in_one_step
install_leaves_a_foreign_global_hooks_path_and_reports_it
install_run_again_keeps_one_hooks_path_and_one_of_each_claude_hook
install_reads_an_included_global_hooks_path_naming_the_gate_as_set
install_keeps_the_rest_of_claude_settings
install_links_commit_gate_alone_in_a_fresh_bin_directory
install_refuses_an_out_of_date_build_and_changes_nothing
install_refuses_a_dirty_build_and_changes_nothing
install_refuses_when_the_target_cant_be_read_and_changes_nothing
install_refuses_when_no_release_exists_and_changes_nothing
echo "OK: test_install"
