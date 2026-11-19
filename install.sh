#!/bin/bash
# Build the commit-gate Go binary, install its git-hook and PATH symlinks,
# point the global core.hooksPath at the gate, and merge its PreToolUse and
# SessionStart hooks into Claude Code's settings.json.
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
# Go resolves the module from the working directory, not from the package path.
cd "$SCRIPT_DIR"
go build -o "$BIN" ./cmd/cg
echo "Built: $BIN"

# commit-msg and pre-push enforce; every other hook git runs passes through
# to the repo's own .git/hooks/<name>, so a global core.hooksPath naming this
# directory silences no repo's hooks.
for h in $("$BIN" hook-names); do ln -sf ../commit-gate "$LIBEXEC/hooks/$h"; done
ln -sf commit-gate "$LIBEXEC/precheck"
ln -sf commit-gate "$LIBEXEC/sessioncheck"

for name in approve approve-push clear-approvals gate-disable gate-enable gate-status; do
  ln -sf "$BIN" "$BIN_DIR/$name"
  echo "Linked: $BIN_DIR/$name"
done

# Gate every repo: the global core.hooksPath names the gate's hooks. One
# naming another directory is reported and left as it is.
hooks="$(cd "$LIBEXEC/hooks" && pwd -P)"
current="$(git config --global --includes --type=path --get core.hooksPath || true)"
if [ -z "$current" ]; then
  git config --global core.hooksPath "$LIBEXEC/hooks"
  echo "Set global core.hooksPath: $LIBEXEC/hooks"
elif [ "$(cd "$current" 2>/dev/null && pwd -P)" = "$hooks" ]; then
  echo "Global core.hooksPath already names the gate: $current"
else
  echo "commit-gate: the global core.hooksPath is $current, not the gate; left as it is. To gate every repo: git config --global core.hooksPath $LIBEXEC/hooks" >&2
fi

# Merge the two hooks into settings.json (idempotent), pointing at the binary's
# precheck/sessioncheck symlinks. The global core.hooksPath points at
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
