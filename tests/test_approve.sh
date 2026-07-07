#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"
BIN="$DIR/../bin"; CANON="$DIR/../lib/canonical"

repo="$(mkrepo)"; cd "$repo"; "$BIN/gate-enable" >/dev/null
gate="$(cd "$(git rev-parse --git-common-dir)" && pwd)/commit-gate"

printf 'feat: one' | "$BIN/approve" --yes
grep -q "^$(printf 'feat: one' | "$CANON")  feat: one\$" "$gate/approved" || fail 'stdin approve not recorded'

f="$(mktemp)"; printf 'feat: a\n@@COMMIT-GATE-SEP@@\nfeat: b\n' > "$f"
"$BIN/approve" --yes -F "$f"
grep -q "  feat: a\$" "$gate/approved" || fail 'batch a missing'
grep -q "  feat: b\$" "$gate/approved" || fail 'batch b missing'

"$BIN/approve" --yes -F <(printf 'feat: c')
grep -q "  feat: c\$" "$gate/approved" || fail 'process-sub input failed'

# bare approve reads the preloaded pending file and consumes it
common="$(cd "$(git rev-parse --git-common-dir)" && pwd)"
printf 'feat: pend' > "$common/cg-pending"
"$BIN/approve" --yes
grep -q "  feat: pend\$" "$gate/approved" || fail 'pending not recorded'
[ -e "$common/cg-pending" ] && fail 'pending not consumed' || true

# recording new approvals invalidates any live push token (stale-content push guard)
"$BIN/approve-push" --yes >/dev/null
[ -e "$gate/push-token" ] || fail 'precondition: token not written'
printf 'feat: after-token' | "$BIN/approve" --yes >/dev/null
[ -e "$gate/push-token" ] && fail 'push token survived new approvals' || true

repo2="$(mkrepo)"; cd "$repo2"
if printf 'x' | "$BIN/approve" --yes 2>/dev/null; then fail 'approve allowed when not enabled'; fi

# approve resolves pending state relative to the CALLER's cwd, not a fixed target:
# running it from a DIFFERENT gated repo must not pick up another repo's pending message.
repoA="$(mkrepo)"; ( cd "$repoA" && "$BIN/gate-enable" >/dev/null )
repoB="$(mkrepo)"; ( cd "$repoB" && "$BIN/gate-enable" >/dev/null )
printf 'feat: only-in-A' > "$repoA/.git/cg-pending"
( cd "$repoB" && "$BIN/approve" --yes </dev/null 2>/dev/null ) && fail 'approve from repoB wrongly found a message'
[ -e "$repoA/.git/cg-pending" ] || fail 'repoA pending was wrongly consumed from repoB'
grep -q 'only-in-A' "$repoB/.git/commit-gate/approved" 2>/dev/null && fail 'repoA message wrongly recorded in repoB manifest' || true

echo "OK: test_approve"
