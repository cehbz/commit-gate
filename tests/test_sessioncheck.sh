#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"
SC="$DIR/../sessioncheck"; BIN="$DIR/../bin"
check() { printf '{"cwd":"%s"}' "$1" | "$SC"; }
quiet() { [ -z "$(check "$1")" ]; }

enabled="$(mkrepo)"; quiet "$enabled" || fail 'enabled repo should be quiet'
optout="$(mkrepo)"; ( cd "$optout" && "$BIN/gate-disable" --yes >/dev/null ); quiet "$optout" || fail 'opted-out should be quiet'
nonrepo="$(mktemp -d)"; quiet "$nonrepo" || fail 'non-repo should be quiet'
overridden="$(mkrepo)"; elsewhere="$(mktemp -d)"; git -C "$overridden" config core.hooksPath "$elsewhere"
check "$overridden" | grep -q "is ungated: its core.hooksPath is $elsewhere" || fail 'overridden repo should be reported'
assert_eq "$(git -C "$overridden" config --local --get core.hooksPath)" "$elsewhere" 'sessioncheck must leave the override'

no_global_gate
fresh="$(mkrepo)"
check "$fresh" | grep -q 'the global core.hooksPath is unset' || fail 'missing global core.hooksPath should be reported'
[ -z "$(git -C "$fresh" config --local --get core.hooksPath)" ] || fail 'sessioncheck must not enable repo by repo'
echo "OK: test_sessioncheck"
