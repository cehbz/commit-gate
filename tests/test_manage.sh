#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"; . "$DIR/pending.sh"
CG="$DIR/../commit-gate"

status_enable_clear_approvals_and_disable_manage_a_repo() {
  repo="$(mkrepo)"; cd "$repo"
  gate="$(cd "$(git rev-parse --git-common-dir)" && pwd)/commit-gate"

  "$CG" status | grep -q '^commit-gate: enabled$' || fail 'fresh repo should be enabled by the global core.hooksPath'
  elsewhere="$(mktemp -d)"; git config core.hooksPath "$elsewhere"
  "$CG" status | grep -q "^commit-gate: overridden (core.hooksPath is $elsewhere" || fail 'repo-local core.hooksPath should read as overridden'
  "$CG" enable >/dev/null
  [ -z "$(git config --local --get core.hooksPath)" ] || fail 'commit-gate enable did not clear the repo-local core.hooksPath'
  "$CG" status | grep -q '^commit-gate: enabled$' || fail 'should report enabled'
  mkdir -p "$gate"

  printf '%s  feat: x\n' 0000 > "$gate/approved"; : > "$gate/push-token"
  "$CG" clear-approvals >/dev/null
  [ -s "$gate/approved" ] && fail 'manifest not cleared' || true
  [ -e "$gate/push-token" ] && fail 'token not cleared' || true

  "$CG" disable --yes >/dev/null
  [ "$(git config --get commit-gate.disabled)" = true ] || fail 'commit-gate disable did not set flag'
  "$CG" status | grep -q opted-out || fail 'should report opted-out'
}
status_enable_clear_approvals_and_disable_manage_a_repo
echo "OK: test_manage"
