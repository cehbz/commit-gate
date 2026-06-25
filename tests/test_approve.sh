#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"
BIN="$DIR/../bin"; CANON="$DIR/../lib/canonical"

repo="$(mkrepo)"; cd "$repo"; "$BIN/enable" >/dev/null
gate="$(cd "$(git rev-parse --git-common-dir)" && pwd)/commit-gate"

printf 'feat: one' | "$BIN/approve" --yes
grep -q "^$(printf 'feat: one' | "$CANON")  feat: one\$" "$gate/approved" || fail 'stdin approve not recorded'

f="$(mktemp)"; printf 'feat: a\n@@COMMIT-GATE-SEP@@\nfeat: b\n' > "$f"
"$BIN/approve" --yes -F "$f"
grep -q "  feat: a\$" "$gate/approved" || fail 'batch a missing'
grep -q "  feat: b\$" "$gate/approved" || fail 'batch b missing'

# process substitution input works (-r, not -f)
"$BIN/approve" --yes -F <(printf 'feat: c')
grep -q "  feat: c\$" "$gate/approved" || fail 'process-sub input failed'

# not enabled -> error
repo2="$(mkrepo)"; cd "$repo2"
if printf 'x' | "$BIN/approve" --yes 2>/dev/null; then fail 'approve allowed when not enabled'; fi
echo "OK: test_approve"
