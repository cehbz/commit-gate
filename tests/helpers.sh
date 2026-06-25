fail() { echo "FAIL: $*" >&2; exit 1; }
assert_eq() { [ "$1" = "$2" ] || fail "${3:-assert_eq}: got [$1] want [$2]"; }
mkrepo() {
  local d; d="$(mktemp -d)"
  git -C "$d" init -q
  git -C "$d" config user.email t@example.com
  git -C "$d" config user.name 'Test'
  printf '%s' "$d"
}
