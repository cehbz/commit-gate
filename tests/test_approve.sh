#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"
CG="$DIR/../commit-gate"

# A new repo is enabled: the global core.hooksPath names the gate.
repo="$(mkrepo)"; cd "$repo"
gate="$(cd "$(git rev-parse --git-common-dir)" && pwd)/commit-gate"

printf 'feat: one' | "$CG" approve --yes
grep -q "^$(printf 'feat: one' | "$CG" canonical)  feat: one\$" "$gate/approved" || fail 'stdin approve not recorded'

f="$(mktemp)"; printf 'feat: a\n@@COMMIT-GATE-SEP@@\nfeat: b\n' > "$f"
"$CG" approve --yes -F "$f"
grep -q "  feat: a\$" "$gate/approved" || fail 'batch a missing'
grep -q "  feat: b\$" "$gate/approved" || fail 'batch b missing'

"$CG" approve --yes -F <(printf 'feat: c')
grep -q "  feat: c\$" "$gate/approved" || fail 'process-sub input failed'

# bare approve reads the preloaded pending file and consumes it
common="$(cd "$(git rev-parse --git-common-dir)" && pwd)"
printf 'feat: pend' > "$common/cg-pending"
"$CG" approve --yes
grep -q "  feat: pend\$" "$gate/approved" || fail 'pending not recorded'
[ -e "$common/cg-pending" ] && fail 'pending not consumed' || true

# recording new approvals invalidates any live push token (stale-content push guard)
"$CG" approve-push --yes >/dev/null
[ -e "$gate/push-token" ] || fail 'precondition: token not written'
printf 'feat: after-token' | "$CG" approve --yes >/dev/null
[ -e "$gate/push-token" ] && fail 'push token survived new approvals' || true

repo2="$(mkrepo)"; git -C "$repo2" config core.hooksPath "$(mktemp -d)"; cd "$repo2"
if printf 'x' | "$CG" approve --yes 2>/dev/null; then fail 'approve allowed when not enabled'; fi

# approve resolves pending state relative to the CALLER's cwd, not a fixed target:
# running it from a DIFFERENT gated repo must not pick up another repo's pending message.
repoA="$(mkrepo)"
repoB="$(mkrepo)"
printf 'feat: only-in-A' > "$repoA/.git/cg-pending"
( cd "$repoB" && "$CG" approve --yes </dev/null 2>/dev/null ) && fail 'approve from repoB wrongly found a message'
[ -e "$repoA/.git/cg-pending" ] || fail 'repoA pending was wrongly consumed from repoB'
grep -q 'only-in-A' "$repoB/.git/commit-gate/approved" 2>/dev/null && fail 'repoA message wrongly recorded in repoB manifest' || true

echo "OK: test_approve"
