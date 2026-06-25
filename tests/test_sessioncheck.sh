#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"
SC="$DIR/../sessioncheck"; BIN="$DIR/../bin"
offers() { out="$(printf '{"cwd":"%s"}' "$1" | "$SC")"; printf '%s' "$out" | grep -q additionalContext; }
quiet()  { out="$(printf '{"cwd":"%s"}' "$1" | "$SC")"; [ -z "$out" ]; }

undecided="$(mkrepo)"; offers "$undecided" || fail 'undecided should offer'
enabled="$(mkrepo)"; ( cd "$enabled" && "$BIN/enable" >/dev/null ); quiet "$enabled" || fail 'enabled should be quiet'
optout="$(mkrepo)"; ( cd "$optout" && "$BIN/disable" --yes >/dev/null ); quiet "$optout" || fail 'opted-out should be quiet'
nonrepo="$(mktemp -d)"; quiet "$nonrepo" || fail 'non-repo should be quiet'
echo "OK: test_sessioncheck"
