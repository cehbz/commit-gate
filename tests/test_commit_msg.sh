#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"
HOOK="$DIR/../hooks/commit-msg"; CANON="$DIR/../lib/canonical"

repo="$(mkrepo)"; cd "$repo"
gate="$(cd "$(git rev-parse --git-common-dir)" && pwd)/commit-gate"; mkdir -p "$gate"

# Unapproved -> deny
: > "$gate/approved"; printf 'feat: alpha\n' > m.txt
if "$HOOK" m.txt; then fail 'unapproved admitted'; fi

# Approved -> admit + consume
printf '%s  feat: alpha\n' "$(printf 'feat: alpha' | "$CANON")" > "$gate/approved"
"$HOOK" m.txt || fail 'approved denied'
[ -s "$gate/approved" ] && fail 'not consumed' || true

# HEAD-amend rule: empty manifest, commit directly (no gating here), same message admitted
: > "$gate/approved"; git commit --allow-empty -q -m 'feat: beta'
printf 'feat: beta\n' > m2.txt
"$HOOK" m2.txt || fail 'HEAD-message amend denied'
printf 'feat: gamma\n' > m3.txt
if "$HOOK" m3.txt; then fail 'unapproved message admitted'; fi

# Opted out -> inert (exit 0) even with empty manifest
: > "$gate/approved"; git config commit-gate.disabled true
printf 'feat: whatever\n' > m4.txt
"$HOOK" m4.txt || fail 'disabled repo should be inert'
git config --unset commit-gate.disabled

# Chaining: a failing native hook aborts before the gate check
mkdir -p "$(git rev-parse --git-common-dir)/hooks"
printf '#!/bin/bash\nexit 7\n' > "$(git rev-parse --git-common-dir)/hooks/commit-msg"
chmod +x "$(git rev-parse --git-common-dir)/hooks/commit-msg"
if "$HOOK" m.txt; then fail 'native-hook failure not propagated'; fi
echo "OK: test_commit_msg"
