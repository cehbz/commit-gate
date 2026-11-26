#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"; . "$DIR/pending.sh"
PC="$DIR/../precheck"; GH="$(cd "$DIR/../hooks" && pwd)"

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

gated="$(mkrepo)"   # enabled: the global core.hooksPath names the gate
ungated="$(mkrepo)"; git -C "$ungated" config commit-gate.disabled true

# check 1: --no-verify / -n, ANDed with a git-commit invocation
denies "git commit --no-verify -m x" || fail 'no-verify'
denies "git commit -n -m x" || fail '-n'
silent "git commit -many -m x" "$ungated" || fail '-n false-positive on -many'
silent "echo -n hi" "$ungated" || fail 'echo -n fp'
silent "git status; echo commit -n" "$ungated" || fail 'no-verify must not cross ;'
denies "git push --no-verify origin main" || fail 'push no-verify'
silent "git push -n origin main" "$ungated" || fail 'push -n is dry-run not no-verify'

# check 2: commit-gate.disabled mention
denies "git config commit-gate.disabled true" || fail 'disable flag'
silent "git config --get commit-gate.disabled" "$ungated" || fail 'disabled read false-denied'

# check 2b: GIT_CONFIG_* environment config injection (env-var equivalent of -c)
denies "GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=commit-gate.disabled GIT_CONFIG_VALUE_0=true git commit -m x" || fail 'GIT_CONFIG env disable'
silent "FOO=bar git status" "$ungated" || fail 'non-config env fine'

# check 3: core.hooksPath (enable exception; the compared path is quoted, so any
# regex-special character in it, e.g. a literal '.', is matched literally; and the
# match requires a real boundary right after the path, not just a prefix)
denies "git config core.hooksPath /tmp/evil" || fail 'hooksPath evil'
silent "git config core.hooksPath $GH" "$ungated" || fail 'enable allowed'
denies "git config core.hooksPath $GH/sub" || fail 'hooksPath prefix bypass'
silent "git config --get core.hooksPath" "$ungated" || fail 'hooksPath read false-denied'
denies "git -c core.hooksPath=/tmp/evil commit -m x" || fail 'per-invocation hooksPath override'

# check 4: write-verb + gate-state target
denies "echo y >> .git/commit-gate/approved" || fail 'manifest write'
silent "rm -rf /tmp/x; echo .git/config" "$ungated" || fail 'verb/target must not cross ;'
denies "rm -rf .git/commit-gate" || fail 'rm gate state'
silent "cat notes.txt | grep .git/config" "$ungated" || fail 'read mention false-denied'
silent 'cat <<EOF
rm -rf .git/commit-gate
EOF' "$ungated" || fail 'heredoc content false-denied'

# check 5: commit-gate approve / approve-push / disable, anchored to a command position
precheck_denies_commit_gate_approve_approve_push_and_disable() {
  denies "commit-gate approve --yes -F /tmp/x" || fail 'approve not denied'
  denies "commit-gate approve-push --yes" || fail 'approve-push not denied'
  denies "commit-gate disable" || fail 'disable not denied'
  denies "echo hi && commit-gate approve" || fail 'chained after && not denied'
  denies "commit-gate approve; echo done" || fail 'chained before ; not denied'
  denies "foo&&commit-gate approve" || fail 'chained, no space after && not denied'
  denies "commit-gate approve;echo x" || fail 'no space after ; not denied'
  denies "commit-gate approve&echo x" || fail 'no space after & not denied'
  denies "commit-gate approve|less" || fail 'no space after | not denied'
  denies "commit-gate disable;rm -rf /" || fail 'disable, no space after ; not denied'
  denies "$(printf 'foo\ncommit-gate disable')" || fail 'newline-separated disable not denied'
  denies "bash -c 'commit-gate approve --yes'" || fail 'approve inside -c not denied'
  denies "/Users/x/.local/bin/commit-gate approve --yes" || fail 'commit-gate by its path not denied'
  denies "$DIR/../commit-gate approve-push" || fail 'commit-gate by a relative path not denied'
}
precheck_refuses_a_computed_commit_gate_command_word() {
  denies 'commit-gate "$c"' || fail 'computed command word not refused'
  denies 'commit-gate $(echo approve) --yes' || fail 'substituted command word not refused'
}
precheck_no_longer_denies_the_old_command_names() {
  silent "approve --yes -F /tmp/x" "$ungated" || fail 'old approve still denied'
  silent "approve-push --yes" "$ungated" || fail 'old approve-push still denied'
  silent "gate-disable" "$ungated" || fail 'old gate-disable still denied'
}
pending precheck_denies_commit_gate_approve_approve_push_and_disable
pending precheck_refuses_a_computed_commit_gate_command_word
pending precheck_no_longer_denies_the_old_command_names
silent "commit-gate enable" "$ungated" || fail 'enable false-denied'
silent "commit-gate status" "$ungated" || fail 'status false-denied'
silent "commit-gate clear-approvals" "$ungated" || fail 'clear-approvals false-denied'
silent "commit-gate canonical < msg" "$ungated" || fail 'canonical false-denied'
silent "commit-gate hook-names" "$ungated" || fail 'hook-names false-denied'
silent "git log --grep 'commit-gate approve'" "$ungated" || fail 'approve-as-arg false-denied'
silent "grep 'commit-gate disable' notes.txt" "$ungated" || fail 'disable-as-arg false-denied'
silent "disapprove;foo" "$ungated" || fail 'disapprove false-denied even adjacent to ;'
denies 'echo "unclosed' || fail 'parse failure must deny (fail closed)'

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
out="$(jq -cn '{tool_name:"Write",tool_input:{file_path:"/Users/h/projects/commit-gate/policy/policy.go"}}' | "$PC")"
[ -z "$out" ] || fail 'gate SOURCE repo must be editable (installed-artifacts boundary)'

echo "OK: test_precheck"
