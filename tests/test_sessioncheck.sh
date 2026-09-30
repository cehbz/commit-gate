#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"
SC="$DIR/../sessioncheck"; BIN="$DIR/../bin"; hooks="$(cd "$DIR/../hooks" && pwd -P)"
enables() { out="$(printf '{"cwd":"%s"}' "$1" | "$SC")"; printf '%s' "$out" | grep -q '"systemMessage":"commit-gate: enabled in ' &&
  printf '%s' "$out" | grep -q additionalContext && [ "$(cd "$(git -C "$1" config --get core.hooksPath)" && pwd -P)" = "$hooks" ]; }
quiet()  { out="$(printf '{"cwd":"%s"}' "$1" | "$SC")"; [ -z "$out" ]; }

undecided="$(mkrepo)"; enables "$undecided" || fail 'undecided should be enabled'
quiet "$undecided" || fail 'once enabled, should be quiet'
enabled="$(mkrepo)"; ( cd "$enabled" && "$BIN/gate-enable" >/dev/null ); quiet "$enabled" || fail 'enabled should be quiet'
optout="$(mkrepo)"; ( cd "$optout" && "$BIN/gate-disable" --yes >/dev/null ); quiet "$optout" || fail 'opted-out should be quiet'
[ -z "$(git -C "$optout" config --get core.hooksPath)" ] || fail 'opted-out should stay untouched'
foreign="$(mkrepo)"; git -C "$foreign" config core.hooksPath .husky/_
out="$(printf '{"cwd":"%s"}' "$foreign" | "$SC")"
printf '%s' "$out" | grep -q '"systemMessage":"commit-gate: not enabled in ' || fail 'foreign hooksPath should get the not-enabled notice'
assert_eq "$(git -C "$foreign" config --get core.hooksPath)" .husky/_ 'foreign hooksPath should stay untouched'
nonrepo="$(mktemp -d)"; quiet "$nonrepo" || fail 'non-repo should be quiet'
echo "OK: test_sessioncheck"
