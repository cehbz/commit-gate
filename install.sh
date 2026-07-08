#!/bin/bash
# Build the commit-gate Go binary, install its git-hook and PATH symlinks, and
# merge its PreToolUse/SessionStart hooks into Claude Code's settings.json.
# Idempotent: safe to re-run after a pull.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CLAUDE_DIR="${CLAUDE_DIR:-$HOME/.claude}"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
LIBEXEC="${LIBEXEC:-$HOME/.local/libexec/commit-gate}"
BIN="$LIBEXEC/commit-gate"

command -v go >/dev/null 2>&1 || { echo "commit-gate: Go is required to build" >&2; exit 1; }

mkdir -p "$LIBEXEC/hooks" "$BIN_DIR" "$CLAUDE_DIR"

# Single binary; every entry point is a symlink dispatched on argv[0] basename.
go build -o "$BIN" "$SCRIPT_DIR/cmd/cg"
echo "Built: $BIN"

ln -sf ../commit-gate "$LIBEXEC/hooks/commit-msg"
ln -sf ../commit-gate "$LIBEXEC/hooks/pre-push"
ln -sf commit-gate "$LIBEXEC/precheck"
ln -sf commit-gate "$LIBEXEC/sessioncheck"

for name in approve approve-push clear-approvals gate-disable gate-enable gate-status; do
  ln -sf "$BIN" "$BIN_DIR/$name"
  echo "Linked: $BIN_DIR/$name"
done

# Merge the two hooks into settings.json (idempotent), pointing at the binary's
# precheck/sessioncheck symlinks. core.hooksPath and gate-enable point at
# "$LIBEXEC/hooks"; the binary resolves its own install dir via os.Executable().
settings="$CLAUDE_DIR/settings.json"
precheck="$LIBEXEC/precheck"
sessioncheck="$LIBEXEC/sessioncheck"
[ -f "$settings" ] || echo '{}' > "$settings"
cp "$settings" "$settings.bak"
jq \
  --arg pre "$precheck" --arg ses "$sessioncheck" \
  '
  .hooks = (.hooks // {}) |
  .hooks.PreToolUse = (.hooks.PreToolUse // []) |
  .hooks.SessionStart = (.hooks.SessionStart // []) |
  (if any(.hooks.PreToolUse[]?.hooks[]?; .command == $pre) then . else
     .hooks.PreToolUse += [{matcher:"Bash|Write|Edit|MultiEdit|NotebookEdit",
       hooks:[{type:"command",command:$pre,statusMessage:"commit-gate precheck"}]}] end) |
  (if any(.hooks.SessionStart[]?.hooks[]?; .command == $ses) then . else
     .hooks.SessionStart += [{hooks:[{type:"command",command:$ses}]}] end)
  ' "$settings.bak" > "$settings"
echo "Merged commit-gate PreToolUse + SessionStart hooks into $settings"
