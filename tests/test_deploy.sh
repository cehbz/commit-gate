#!/bin/bash
# make deploy in a clone builds the target from a fresh copy of the public
# repository and puts it live on a host, under one deploy authorization
# (DESIGN.md, Deploy, Deploy authorization). A real host can't be reached from
# a test: each case deploys to the Mac, host mac, whose install directory is
# under a temporary home, with a scratch repository as the public address.
set -eu
DIR="$(cd "$(dirname "$0")" && pwd -P)"; . "$DIR/helpers.sh"; . "$DIR/pending.sh"; . "$DIR/scratch.sh"
CG="$DIR/../commit-gate"
tmp="$(mktemp -d)"; tmp="$(cd "$tmp" && pwd -P)"; trap 'chmod -R u+w "$tmp" 2>/dev/null; rm -rf "$tmp"' EXIT
if [ "$(uname -s)" != Darwin ]; then echo "skip test_deploy: the deploy cases deliver to host mac"; exit 0; fi
keep_go_caches

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
# L is the host's install directory.
new_host() {
  prepare; scratch_home "$tmp"; L="$H/.local/libexec/commit-gate"
  printf '[url "%s"]\n\tinsteadOf = %s\n' "$pub" "$PUBLIC_URL" > "$H/.gitconfig"
  clone="$(mktemp -d "$tmp/clone.XXXXXX")"; as_host git clone -q "$pub" "$clone"
}
# clone_state: the clone's head, changes and gate state.
clone_state() { git -C "$clone" rev-parse HEAD; git -C "$clone" status --porcelain; ls -A "$clone/.git/commit-gate" 2>/dev/null || true; }
as_host() { HOME="$H" GIT_CONFIG_GLOBAL="$H/.gitconfig" "$@"; }
deploy() { ( cd "$clone" && as_host make -s deploy "$@" ) > "$tmp/out" 2>&1; }
# authorize_deploy: the user's approve-deploy in the clone, which the gate
# under test enables through the suite's global config.
authorize_deploy() { ( cd "$clone" && HOME="$H" "$CG" approve-deploy --yes ) >/dev/null; }

# assert_target_live: the host's live build is the target's, from unchanged source.
assert_target_live() {
  [ -x "$L/commit-gate" ] || fail "no live build on the host: $(cat "$tmp/out")"
  assert_eq "$(build_commit "$L/commit-gate")" "$c1" "the live build's commit"
  assert_eq "$(build_dirty "$L/commit-gate")" false 'the live build is from changed source'
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

the_host_list_is_a_file_git_ignores() {
  new_host; printf 'mac\n' > "$clone/deploy.hosts"
  ( cd "$clone" && as_host git check-ignore -q deploy.hosts ) || fail 'git does not ignore the host list deploy.hosts'
}

pending deploy_refuses_without_a_deploy_authorization_and_changes_nothing
pending deploy_puts_the_target_live_on_a_host_with_no_build
pending deploy_replaces_an_old_build_that_knows_no_version
pending deploy_consumes_its_deploy_authorization
pending deploy_consumes_its_deploy_authorization_when_the_host_fails
pending deploy_builds_the_target_from_a_fresh_copy_whatever_the_clone_holds
pending deploy_without_a_host_deploys_every_host_on_the_host_list
pending the_host_list_is_a_file_git_ignores
echo "OK: test_deploy"
