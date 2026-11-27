#!/bin/bash
# make deploy in a clone builds the target from a fresh copy of the public
# repository and puts it live on a host, under one deploy authorization
# (DESIGN.md, Deploy, Deploy authorization). A real host can't be reached from
# a test: a case deploys to the Mac, host mac, whose install directory is
# under a temporary home, with a scratch repository as the public address;
# any other host is a second temporary home, reached through stub ssh and scp
# first on PATH, and a host with no home there is unreachable.
set -eu
DIR="$(cd "$(dirname "$0")" && pwd -P)"; . "$DIR/helpers.sh"; . "$DIR/pending.sh"; . "$DIR/scratch.sh"
CG="$DIR/../commit-gate"
tmp="$(mktemp -d)"; tmp="$(cd "$tmp" && pwd -P)"; trap 'chmod -R u+w "$tmp" 2>/dev/null; rm -rf "$tmp"' EXIT
if [ "$(uname -s)" != Darwin ]; then echo "skip test_deploy: the deploy cases deliver to host mac"; exit 0; fi
keep_go_caches

# The stub ssh and scp, in $tmp/bin: host <name>'s home is $tmp/remotes/<name>.
# ssh logs its arguments to $tmp/ssh.log and runs its command in that home as
# a login there would, with that home's global git config; scp copies a file
# to a path under that home.
mkdir "$tmp/bin" "$tmp/remotes"; : > "$tmp/ssh.log"
cat > "$tmp/bin/ssh" <<'EOF'
#!/bin/sh
tmp="$(cd "$(dirname "$0")/.." && pwd -P)"
echo "$*" >> "$tmp/ssh.log"
home="$tmp/remotes/$1"; shift
[ -d "$home" ] || { echo "ssh: Could not resolve hostname" >&2; exit 255; }
cd "$home" && HOME="$home" GIT_CONFIG_GLOBAL="$home/.gitconfig" exec sh -c "$*"
EOF
cat > "$tmp/bin/scp" <<'EOF'
#!/bin/sh
tmp="$(cd "$(dirname "$0")/.." && pwd -P)"
[ "$1" != -q ] || shift
home="$tmp/remotes/${2%%:*}"
[ -d "$home" ] || { echo "ssh: Could not resolve hostname" >&2; exit 255; }
cp "$1" "$home/${2#*:}"
EOF
chmod +x "$tmp/bin/ssh" "$tmp/bin/scp"

# prepare, on first use: pub, the stand-in public repository, holding a
# committed copy of the working tree at c1, released as v1.0.0.
prepare() {
  [ -z "${prepared:-}" ] || return 0
  src="$tmp/src"; mkdir "$src"; scratch_tree "$src"; git -C "$src" init -q -b main
  scratch_commit "$src" 'c1'; c1="$(git -C "$src" rev-parse HEAD)"
  pub="$tmp/pub.git"; git clone -q --bare "$src" "$pub"; git -C "$pub" tag v1.0.0 "$c1"
  prepared=1
}

# new_host: a temporary home in $H for host mac, with no gate, whose global
# git config points the public address at pub; and a clone of pub in $clone.
# L is the host's install directory. The stub ssh's log starts empty.
new_host() {
  prepare; scratch_home "$tmp"; L="$H/.local/libexec/commit-gate"; : > "$tmp/ssh.log"
  printf '[url "%s"]\n\tinsteadOf = %s\n' "$pub" "$PUBLIC_URL" > "$H/.gitconfig"
  clone="$(mktemp -d "$tmp/clone.XXXXXX")"; as_host git clone -q "$pub" "$clone"
}
# new_remote <host>: a temporary home for host, reached through the stub ssh,
# with no gate and the public address pointed at pub, in $R.
new_remote() {
  R="$tmp/remotes/$1"; mkdir "$R"
  printf '[url "%s"]\n\tinsteadOf = %s\n' "$pub" "$PUBLIC_URL" > "$R/.gitconfig"
}
# clone_state: the clone's head, changes and gate state.
clone_state() { git -C "$clone" rev-parse HEAD; git -C "$clone" status --porcelain; ls -A "$clone/.git/commit-gate" 2>/dev/null || true; }
as_host() { HOME="$H" GIT_CONFIG_GLOBAL="$H/.gitconfig" "$@"; }
deploy() { ( cd "$clone" && PATH="$tmp/bin:$PATH" as_host make -s deploy "$@" ) > "$tmp/out" 2>&1; }
# authorize_deploy: the user's approve-deploy in the clone, which the gate
# under test enables through the suite's global config.
authorize_deploy() { ( cd "$clone" && HOME="$H" "$CG" approve-deploy --yes ) >/dev/null; }

# assert_target_live [<install dir>]: the host's live build, in L unless
# named, is the target's, from unchanged source.
assert_target_live() {
  local l="${1:-$L}"
  [ -x "$l/commit-gate" ] || fail "no live build in $l: $(cat "$tmp/out")"
  assert_eq "$(build_commit "$l/commit-gate")" "$c1" "the live build's commit"
  assert_eq "$(build_dirty "$l/commit-gate")" false 'the live build is from changed source'
}

# assert_refused_keeping_the_authorization <what>: deploy without a host
# refuses, reaches no host and leaves the deploy authorization live.
assert_refused_keeping_the_authorization() {
  if deploy; then fail "deploy went through: $1"; fi
  [ -e "$clone/.git/commit-gate/deploy-token" ] || fail "the refusal consumed the deploy authorization: $1: $(cat "$tmp/out")"
  [ ! -e "$L" ] || fail "a refused deploy installed on the Mac: $1"
  assert_eq "$(cat "$tmp/ssh.log")" '' "a refused deploy reached a host: $1"
}

# assert_deploy_refused: deploy refuses, naming commit-gate approve-deploy.
assert_deploy_refused() {
  if deploy HOST=mac; then fail "deploy went through: $1"; fi
  grep -qF 'commit-gate approve-deploy' "$tmp/out" || fail "the refusal does not name commit-gate approve-deploy: $(cat "$tmp/out")"
}

deploy_refuses_without_a_deploy_authorization_and_changes_nothing() {
  new_host
  home_before="$(snapshot "$H")"; clone_before="$(clone_state)"
  assert_deploy_refused 'no deploy authorization'
  assert_eq "$(snapshot "$H")" "$home_before" 'a refused deploy changed the host'
  assert_eq "$(clone_state)" "$clone_before" 'a refused deploy changed the clone'
}

deploy_puts_the_target_live_on_a_host_with_no_build() {
  new_host; authorize_deploy
  deploy HOST=mac || fail "deploy failed: $(cat "$tmp/out")"
  assert_target_live
  [ "$H/.local/bin/commit-gate" -ef "$L/commit-gate" ] || fail 'commit-gate on PATH is not the live build'
  assert_eq "$(as_host git config --global --type=path --get core.hooksPath)" "$L/hooks" 'the global core.hooksPath names the deployed gate'
  ( cd "$tmp" && as_host "$H/.local/bin/commit-gate" version ) > "$tmp/version" 2>&1 \
    || fail "the deployed build is not up to date: $(cat "$tmp/version")"
}

deploy_replaces_an_old_build_that_knows_no_version() {
  new_host; mkdir -p "$L"; printf '#!/bin/sh\nexit 1\n' > "$L/commit-gate"; chmod +x "$L/commit-gate"
  authorize_deploy
  deploy HOST=mac || fail "deploy over an old build failed: $(cat "$tmp/out")"
  assert_target_live
}

deploy_consumes_its_deploy_authorization() {
  new_host; authorize_deploy
  deploy HOST=mac || fail "deploy failed: $(cat "$tmp/out")"
  assert_deploy_refused 'a second deploy on one authorization'
}

deploy_consumes_its_deploy_authorization_when_the_host_fails() {
  new_host; mkdir -p "$L"; chmod 555 "$L"
  authorize_deploy
  if deploy HOST=mac; then chmod 755 "$L"; fail 'deploy to a host whose install directory is read-only went through'; fi
  chmod 755 "$L"
  assert_deploy_refused 'a deploy after a failed one, on one authorization'
}

deploy_builds_the_target_from_a_fresh_copy_whatever_the_clone_holds() {
  new_host
  echo unreleased >> "$clone/CHANGES"; scratch_commit "$clone" 'feat: unreleased'
  echo 'not committed' >> "$clone/README.md"
  before="$(git -C "$clone" rev-parse HEAD; git -C "$clone" status --porcelain)"
  authorize_deploy
  deploy HOST=mac || fail "deploy failed: $(cat "$tmp/out")"
  assert_target_live
  assert_eq "$(git -C "$clone" rev-parse HEAD; git -C "$clone" status --porcelain)" "$before" 'deploy changed the clone'
}

deploy_without_a_host_deploys_every_host_on_the_host_list() {
  new_host; printf 'mac\n' > "$clone/deploy.hosts"
  authorize_deploy
  deploy || fail "deploy of the host list failed: $(cat "$tmp/out")"
  assert_target_live
}

the_host_list_skips_comment_lines_and_blank_lines() {
  new_host; printf '# the hosts\n\nmac\n\n#example\n' > "$clone/deploy.hosts"
  authorize_deploy
  deploy || fail "deploy of a host list with comments failed: $(cat "$tmp/out")"
  assert_target_live
  assert_eq "$(cat "$tmp/ssh.log")" '' 'deploy reached a host other than mac'
}

deploy_without_a_host_or_a_host_list_refuses_and_the_authorization_stays_live() {
  new_host; authorize_deploy
  assert_refused_keeping_the_authorization 'no HOST and no host list'
}

deploy_of_a_host_list_naming_no_host_refuses_and_the_authorization_stays_live() {
  new_host; printf '# the hosts\n\n#mac\n\n' > "$clone/deploy.hosts"
  authorize_deploy
  assert_refused_keeping_the_authorization 'a host list of comments and blank lines'
}

deploy_puts_the_target_live_on_a_host_other_than_the_mac() {
  new_host; new_remote example; authorize_deploy
  deploy HOST=example || fail "deploy to host example failed: $(cat "$tmp/out")"
  assert_target_live "$R/.local/libexec/commit-gate"
  [ "$R/.local/bin/commit-gate" -ef "$R/.local/libexec/commit-gate/commit-gate" ] || fail "commit-gate on example's PATH is not the live build"
  [ ! -e "$L" ] || fail 'deploy to host example installed on the Mac'
}

a_failing_host_leaves_the_other_live_and_the_deploy_exits_non_zero() {
  new_host; new_remote remote1; printf 'down\nremote1\n' > "$clone/deploy.hosts"
  authorize_deploy
  if deploy; then fail 'deploy exited zero though host down is unreachable'; fi
  assert_target_live "$R/.local/libexec/commit-gate"
}

the_host_list_is_a_file_git_ignores() {
  new_host; printf 'mac\n' > "$clone/deploy.hosts"
  ( cd "$clone" && as_host git check-ignore -q deploy.hosts ) || fail 'git does not ignore the host list deploy.hosts'
}

deploy_refuses_without_a_deploy_authorization_and_changes_nothing
deploy_puts_the_target_live_on_a_host_with_no_build
deploy_replaces_an_old_build_that_knows_no_version
deploy_consumes_its_deploy_authorization
deploy_consumes_its_deploy_authorization_when_the_host_fails
deploy_builds_the_target_from_a_fresh_copy_whatever_the_clone_holds
deploy_without_a_host_deploys_every_host_on_the_host_list
the_host_list_skips_comment_lines_and_blank_lines
deploy_without_a_host_or_a_host_list_refuses_and_the_authorization_stays_live
deploy_of_a_host_list_naming_no_host_refuses_and_the_authorization_stays_live
deploy_puts_the_target_live_on_a_host_other_than_the_mac
a_failing_host_leaves_the_other_live_and_the_deploy_exits_non_zero
the_host_list_is_a_file_git_ignores
echo "OK: test_deploy"
