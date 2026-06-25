#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"
ROOT="$(cd "$DIR/../.." && pwd)"
home="$(mktemp -d)"; mkdir -p "$home/.claude"; echo '{}' > "$home/.claude/settings.json"

HOME="$home" CLAUDE_DIR="$home/.claude" BIN_DIR="$home/bin" bash "$ROOT/install.sh" >/dev/null
[ -L "$home/bin/approve" ] || fail 'approve not symlinked'
jq -e '.hooks.PreToolUse[]?.hooks[]?.command | select(test("commit-gate/precheck"))' "$home/.claude/settings.json" >/dev/null || fail 'precheck not merged'
jq -e '.hooks.SessionStart[]?.hooks[]?.command | select(test("commit-gate/sessioncheck"))' "$home/.claude/settings.json" >/dev/null || fail 'sessioncheck not merged'
[ -f "$home/.claude/settings.json.bak" ] || fail 'no backup'

HOME="$home" CLAUDE_DIR="$home/.claude" BIN_DIR="$home/bin" bash "$ROOT/install.sh" >/dev/null
n="$(jq '[.hooks.PreToolUse[]?.hooks[]? | select(.command|test("commit-gate/precheck"))]|length' "$home/.claude/settings.json")"
assert_eq "$n" "1" 'precheck duplicated on re-run'
echo "OK: test_install"
