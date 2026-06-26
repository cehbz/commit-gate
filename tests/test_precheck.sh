#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"
PC="$DIR/../precheck"; GH="$(cd "$DIR/../hooks" && pwd)"
denies() { out="$("$PC")"; printf '%s' "$out" | grep -q '"permissionDecision":"deny"'; }
allows() { out="$("$PC")"; [ -z "$out" ]; }

echo '{"tool_name":"Bash","tool_input":{"command":"git commit -m x"}}' | allows || fail 'plain commit'
echo '{"tool_name":"Bash","tool_input":{"command":"git commit --no-verify -m x"}}' | denies || fail 'no-verify'
echo '{"tool_name":"Bash","tool_input":{"command":"git commit -n -m x"}}' | denies || fail '-n'
echo '{"tool_name":"Bash","tool_input":{"command":"echo -n hi"}}' | allows || fail 'echo -n fp'
echo '{"tool_name":"Bash","tool_input":{"command":"git config commit-gate.disabled true"}}' | denies || fail 'disable flag'
echo '{"tool_name":"Bash","tool_input":{"command":"git config core.hooksPath /tmp/evil"}}' | denies || fail 'hooksPath evil'
echo "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"git config core.hooksPath $GH\"}}" | allows || fail 'enable allowed'
echo '{"tool_name":"Bash","tool_input":{"command":"echo y >> .git/commit-gate/approved"}}' | denies || fail 'manifest write'
echo '{"tool_name":"Write","tool_input":{"file_path":"/x/.git/commit-gate/approved"}}' | denies || fail 'Write manifest'
echo '{"tool_name":"Edit","tool_input":{"file_path":"/Users/h/.claude/settings.json"}}' | denies || fail 'settings edit'
echo '{"tool_name":"Write","tool_input":{"file_path":"/x/.git/hooks/commit-msg"}}' | denies || fail 'native hook write'
echo '{"tool_name":"Write","tool_input":{"file_path":"/x/src/main.go"}}' | allows || fail 'normal write'
echo '{"tool_name":"Bash","tool_input":{"command":"approve --yes -F /tmp/x"}}' | denies || fail 'approve not denied'
echo '{"tool_name":"Bash","tool_input":{"command":"approve-push --yes"}}' | denies || fail 'approve-push not denied'
echo '{"tool_name":"Bash","tool_input":{"command":"gate-disable"}}' | denies || fail 'gate-disable not denied'
echo '{"tool_name":"Bash","tool_input":{"command":"gate-enable"}}' | allows || fail 'gate-enable false-denied'
echo '{"tool_name":"Bash","tool_input":{"command":"git log --grep approve"}}' | allows || fail 'approve-as-arg false-denied'
echo '{"tool_name":"Bash","tool_input":{"command":"clear-approvals"}}' | allows || fail 'clear-approvals false-denied'
echo "OK: test_precheck"
