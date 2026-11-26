#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"
CG="$DIR/../commit-gate"; HOOK="$DIR/../hooks/pre-push"

# A new repo is enabled: the global core.hooksPath names the gate.
repo="$(mkrepo)"; cd "$repo"
gate="$(cd "$(git rev-parse --git-common-dir)" && pwd)/commit-gate"

if echo | "$HOOK" origin /dev/null; then fail 'push admitted without token'; fi
"$CG" approve-push --yes >/dev/null
[ -e "$gate/push-token" ] || fail 'token not written'
echo | "$HOOK" origin /dev/null || fail 'push denied with token'
[ -e "$gate/push-token" ] && fail 'token not consumed' || true
if echo | "$HOOK" origin /dev/null; then fail 'token reused'; fi
echo "OK: test_push"
