#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; rc=0
for t in "$DIR"/test_*.sh; do echo "== $(basename "$t") =="; bash "$t" || rc=1; done
[ $rc -eq 0 ] && echo "ALL PASS" || { echo "SOME FAILED"; exit 1; }
