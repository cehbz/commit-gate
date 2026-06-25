#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"
BIN="$DIR/../bin"; export PATH="$BIN:$PATH"

repo="$(mkrepo)"; cd "$repo"
"$DIR/../bin/gate-status" | grep -q undecided || fail 'should start undecided'
gate-enable >/dev/null

echo a > a.txt; git add a.txt
if git commit -m "feat: add a" -m "Co-Authored-By: X <x@x>" >/dev/null 2>&1; then fail 'unapproved commit succeeded'; fi
approve --yes -F <(printf 'feat: add a\n\nCo-Authored-By: X <x@x>') >/dev/null
git commit -m "feat: add a" -m "Co-Authored-By: X <x@x>" >/dev/null || fail 'approved commit blocked'

echo a2 >> a.txt; git add a.txt
git commit --amend --no-edit >/dev/null || fail 'amend blocked'

echo b > b.txt; git add b.txt
git commit --no-verify -m "wip" >/dev/null || fail 'human --no-verify blocked'

bare="$(mktemp -d)"; git -C "$bare" init --bare -q; git remote add origin "$bare"
if git push origin HEAD >/dev/null 2>&1; then fail 'push without token succeeded'; fi
approve-push --yes >/dev/null
git push origin HEAD >/dev/null 2>&1 || fail 'approved push failed'
gate="$(cd "$(git rev-parse --git-common-dir)" && pwd)/commit-gate"
[ -e "$gate/push-token" ] && fail 'push token not consumed' || true

# opt-out makes the repo inert
gate-disable --yes >/dev/null
echo c > c.txt; git add c.txt
git commit -m "anything" >/dev/null || fail 'opted-out repo still gated'
echo "OK: test_integration"
