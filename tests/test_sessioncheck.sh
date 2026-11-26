#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"; . "$DIR/pending.sh"
SC="$DIR/../sessioncheck"; CG="$DIR/../commit-gate"
check() { printf '{"cwd":"%s"}' "$1" | "$SC"; }
quiet() { [ -z "$(check "$1")" ]; }

sessioncheck_is_quiet_in_a_repo_opted_out_by_disable() {
  optout="$(mkrepo)"; ( cd "$optout" && "$CG" disable --yes >/dev/null ) || fail 'commit-gate disable failed'
  quiet "$optout" || fail 'opted-out should be quiet'
}

enabled="$(mkrepo)"; quiet "$enabled" || fail 'enabled repo should be quiet'
pending sessioncheck_is_quiet_in_a_repo_opted_out_by_disable
nonrepo="$(mktemp -d)"; quiet "$nonrepo" || fail 'non-repo should be quiet'
overridden="$(mkrepo)"; elsewhere="$(mktemp -d)"; git -C "$overridden" config core.hooksPath "$elsewhere"
check "$overridden" | grep -q "is ungated: its core.hooksPath is $elsewhere" || fail 'overridden repo should be reported'
assert_eq "$(git -C "$overridden" config --local --get core.hooksPath)" "$elsewhere" 'sessioncheck must leave the override'

no_global_gate
fresh="$(mkrepo)"
check "$fresh" | grep -q 'the global core.hooksPath is unset' || fail 'missing global core.hooksPath should be reported'
[ -z "$(git -C "$fresh" config --local --get core.hooksPath)" ] || fail 'sessioncheck must not enable repo by repo'
echo "OK: test_sessioncheck"
