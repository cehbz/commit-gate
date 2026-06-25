#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"
BIN="$DIR/../bin"; hooks="$(cd "$DIR/../hooks" && pwd)"

repo="$(mkrepo)"; cd "$repo"
gate="$(cd "$(git rev-parse --git-common-dir)" && pwd)/commit-gate"

"$BIN/gate-status" | grep -q undecided || fail 'fresh repo should be undecided'
"$BIN/gate-enable" >/dev/null
[ "$(git config --get core.hooksPath)" = "$hooks" ] || fail 'gate-enable did not set local hooksPath'
[ -d "$gate" ] || fail 'gate-enable did not create gate dir'
"$BIN/gate-status" | grep -q enabled || fail 'should report enabled'

printf '%s  feat: x\n' 0000 > "$gate/approved"; : > "$gate/push-token"
"$BIN/clear-approvals" >/dev/null
[ -s "$gate/approved" ] && fail 'manifest not cleared' || true
[ -e "$gate/push-token" ] && fail 'token not cleared' || true

"$BIN/gate-disable" --yes >/dev/null
[ "$(git config --get commit-gate.disabled)" = true ] || fail 'gate-disable did not set flag'
"$BIN/gate-status" | grep -q opted-out || fail 'should report opted-out'
echo "OK: test_manage"
