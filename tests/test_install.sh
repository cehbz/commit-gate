#!/bin/bash
# install.sh builds the binary whatever directory it is called from.
set -eu
DIR="$(cd "$(dirname "$0")" && pwd -P)"; . "$DIR/helpers.sh"
INSTALL="$DIR/../install.sh"
[ -f "$INSTALL" ] || INSTALL="$DIR/../../install.sh"   # run from build/tests
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
elsewhere="$(mkrepo)"
( cd "$elsewhere" && CLAUDE_DIR="$tmp/claude" BIN_DIR="$tmp/bin" LIBEXEC="$tmp/libexec" bash "$INSTALL" ) >"$tmp/out" 2>&1 \
  || fail "install.sh from another repo's directory: $(cat "$tmp/out")"
[ -x "$tmp/libexec/commit-gate" ] || fail "no binary built"
[ -L "$tmp/bin/approve" ] || fail "approve not linked"
rm -rf "$elsewhere"
echo "OK: test_install"
