#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"
PC="$DIR/../precheck"; GH="$(cd "$DIR/../hooks" && pwd)"; BIN="$DIR/../bin"

run() {  # $1=command, $2=cwd (optional)
  if [ -n "${2:-}" ]; then
    jq -cn --arg c "$1" --arg d "$2" '{tool_name:"Bash",tool_input:{command:$c},cwd:$d}' | "$PC"
  else
    jq -cn --arg c "$1" '{tool_name:"Bash",tool_input:{command:$c}}' | "$PC"
  fi
}
denies()  { out="$(run "$1" "${2:-}")"; printf '%s' "$out" | grep -q '"permissionDecision":"deny"'; }
silent()  { out="$(run "$1" "${2:-}")"; [ -z "$out" ]; }
reminds() { out="$(run "$1" "${2:-}")"; printf '%s' "$out" | grep -q '"permissionDecision":"allow"'; }

gated="$(mkrepo)"; ( cd "$gated" && "$BIN/gate-enable" >/dev/null )
ungated="$(mkrepo)"

# check 1: --no-verify / -n, ANDed with a git-commit invocation
denies "git commit --no-verify -m x" || fail 'no-verify'
denies "git commit -n -m x" || fail '-n'
silent "git commit -many -m x" "$ungated" || fail '-n false-positive on -many'
silent "echo -n hi" "$ungated" || fail 'echo -n fp'
silent "git status; echo commit -n" "$ungated" || fail 'no-verify must not cross ;'

# check 2: commit-gate.disabled mention
denies "git config commit-gate.disabled true" || fail 'disable flag'

# check 3: core.hooksPath (enable exception; the compared path is quoted, so any
# regex-special character in it, e.g. a literal '.', is matched literally; and the
# match requires a real boundary right after the path, not just a prefix)
denies "git config core.hooksPath /tmp/evil" || fail 'hooksPath evil'
silent "git config core.hooksPath $GH" "$ungated" || fail 'enable allowed'
denies "git config core.hooksPath $GH/sub" || fail 'hooksPath prefix bypass'

# check 4: write-verb + gate-state target
denies "echo y >> .git/commit-gate/approved" || fail 'manifest write'
silent "rm -rf /tmp/x; echo .git/config" "$ungated" || fail 'verb/target must not cross ;'

# check 5: approve / approve-push / gate-disable, anchored to a command position
denies "approve --yes -F /tmp/x" || fail 'approve not denied'
denies "approve-push --yes" || fail 'approve-push not denied'
denies "gate-disable" || fail 'gate-disable not denied'
denies "echo hi && approve" || fail 'chained after && not denied'
denies "approve; echo done" || fail 'chained before ; not denied'
denies "foo&&approve" || fail 'chained, no space after && not denied'
denies "approve;echo x" || fail 'no space after ; not denied'
denies "approve&echo x" || fail 'no space after & not denied'
denies "approve|less" || fail 'no space after | not denied'
denies "gate-disable;rm -rf /" || fail 'gate-disable, no space after ; not denied'
silent "gate-enable" "$ungated" || fail 'gate-enable false-denied'
silent "git log --grep approve" "$ungated" || fail 'approve-as-arg false-denied'
silent "grep gate-disable notes.txt" "$ungated" || fail 'gate-disable-as-arg false-denied'
silent "clear-approvals" "$ungated" || fail 'clear-approvals false-denied'
silent "disapprove;foo" "$ungated" || fail 'disapprove false-denied even adjacent to ;'

# check 6: reminder on a real git commit/push, only in a gated repo
reminds "git commit -m x" "$gated" || fail 'reminder missing on commit in gated repo'
reminds "git push origin main" "$gated" || fail 'reminder missing on push in gated repo'
silent "git commit -m x" "$ungated" || fail 'reminder wrongly fired in ungated repo'
silent "git status" "$gated" || fail 'reminder wrongly fired on non-commit/push'

# Write/Edit branch (path-based, unaffected by the Bash-branch fixes above)
out="$(jq -cn '{tool_name:"Write",tool_input:{file_path:"/x/.git/commit-gate/approved"}}' | "$PC")"
printf '%s' "$out" | grep -q '"permissionDecision":"deny"' || fail 'Write manifest'
out="$(jq -cn '{tool_name:"Edit",tool_input:{file_path:"/Users/h/.claude/settings.json"}}' | "$PC")"
printf '%s' "$out" | grep -q '"permissionDecision":"deny"' || fail 'settings edit'
out="$(jq -cn '{tool_name:"Write",tool_input:{file_path:"/x/.git/hooks/commit-msg"}}' | "$PC")"
printf '%s' "$out" | grep -q '"permissionDecision":"deny"' || fail 'native hook write'
out="$(jq -cn '{tool_name:"Write",tool_input:{file_path:"/x/src/main.go"}}' | "$PC")"
[ -z "$out" ] || fail 'normal write'

echo "OK: test_precheck"
