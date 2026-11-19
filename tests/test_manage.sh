#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"
BIN="$DIR/../bin"

repo="$(mkrepo)"; cd "$repo"
gate="$(cd "$(git rev-parse --git-common-dir)" && pwd)/commit-gate"

"$BIN/gate-status" | grep -q '^commit-gate: enabled$' || fail 'fresh repo should be enabled by the global core.hooksPath'
elsewhere="$(mktemp -d)"; git config core.hooksPath "$elsewhere"
"$BIN/gate-status" | grep -q "^commit-gate: overridden (core.hooksPath is $elsewhere" || fail 'repo-local core.hooksPath should read as overridden'
"$BIN/gate-enable" >/dev/null
[ -z "$(git config --local --get core.hooksPath)" ] || fail 'gate-enable did not clear the repo-local core.hooksPath'
"$BIN/gate-status" | grep -q '^commit-gate: enabled$' || fail 'should report enabled'
mkdir -p "$gate"

printf '%s  feat: x\n' 0000 > "$gate/approved"; : > "$gate/push-token"
"$BIN/clear-approvals" >/dev/null
[ -s "$gate/approved" ] && fail 'manifest not cleared' || true
[ -e "$gate/push-token" ] && fail 'token not cleared' || true

"$BIN/gate-disable" --yes >/dev/null
[ "$(git config --get commit-gate.disabled)" = true ] || fail 'gate-disable did not set flag'
"$BIN/gate-status" | grep -q opted-out || fail 'should report opted-out'
echo "OK: test_manage"
