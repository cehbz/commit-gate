#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"
CANON="$DIR/../lib/canonical"
h() { printf '%b' "$1" | "$CANON"; }
assert_eq "$(h 'feat: hello')" '93d453a2465815abe2767283e34931387395497416d2e016a28d19ba676361ff' 'plain'
assert_eq "$(h 'feat: hello   ')" '93d453a2465815abe2767283e34931387395497416d2e016a28d19ba676361ff' 'trailing ws'
assert_eq "$(h 'feat: hello\r')" '93d453a2465815abe2767283e34931387395497416d2e016a28d19ba676361ff' 'CRLF'
assert_eq "$(h 'feat: hello\n\n\n')" '93d453a2465815abe2767283e34931387395497416d2e016a28d19ba676361ff' 'trailing blanks'
assert_eq "$(h 'feat: world')" '26945e41d62e9b53c842e8e082122b44f2825214cc3b565347f0facb848a0ef5' 'distinct'
echo "OK: test_canonical"
