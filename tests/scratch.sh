# Scratch stand-ins for the gate's public repository (DESIGN.md, Tests).
# Source after helpers.sh. A committed copy of the working tree stands in for
# the repository; its private global git config points the public address at
# it with url.<scratch>.insteadOf, so a build under test is an ordinary build.
PUBLIC_URL=https://github.com/cehbz/commit-gate.git

# The source tree: the parent of tests/, or of build/tests/ in a build tree.
SRC_ROOT="$(cd "$DIR/.." && pwd -P)"
[ -f "$SRC_ROOT/go.mod" ] || SRC_ROOT="$(cd "$DIR/../.." && pwd -P)"

# scratch_tree <dir>: copy the working tree, tracked files and untracked ones
# git doesn't ignore, into dir, so a dirty tree or one mid-plan copies whole.
scratch_tree() {
  ( cd "$SRC_ROOT" && git ls-files -co --exclude-standard -z \
      | while IFS= read -r -d '' f; do { [ -e "$f" ] || [ -L "$f" ]; } && printf '%s\0' "$f"; done \
      | tar --null -T - -cf - ) | tar -xf - -C "$1"
}

# scratch_commit <repo> <subject>: commit everything in repo as a machine
# committer, past the gate (DESIGN.md, Machine committer).
scratch_commit() {
  git -C "$1" add -A
  git -C "$1" -c core.hooksPath=/dev/null -c user.email=t@example.com -c user.name=Test commit -q -m "$2"
}

# scratch_build <src> <dir>: an ordinary build of src's cmd/cg, written to
# <dir>/commit-gate, outside src.
scratch_build() {
  mkdir -p "$2"
  ( cd "$1" && go build -o "$2/commit-gate" ./cmd/cg )
}

# build_commit <file> / build_dirty <file>: the commit a build records, and
# whether its source had uncommitted changes, read from the file.
build_commit() { go version -m "$1" | awk '$2 ~ /^vcs\.revision=/ { sub(/^vcs\.revision=/, "", $2); print $2 }'; }
build_dirty() { go version -m "$1" | awk '$2 ~ /^vcs\.modified=/ { sub(/^vcs\.modified=/, "", $2); print $2 }'; }

# scratch_home <tmp>: a new home directory under tmp, resolved, in $H.
scratch_home() { H="$(mktemp -d "$1/home.XXXXXX")"; H="$(cd "$H" && pwd -P)"; }

# snapshot <dir>: every path under dir, the content of every file and the
# target of every link, to compare before and after.
snapshot() {
  ( cd "$1" && find . | LC_ALL=C sort \
      && find . -type f -exec shasum {} + | LC_ALL=C sort \
      && find . -type l -exec sh -c 'for l; do printf "%s -> %s\n" "$l" "$(readlink "$l")"; done' sh {} + | LC_ALL=C sort )
}

# keep_go_caches: Go's module and build caches stay put when a command runs
# under a temporary HOME, and no module is fetched.
keep_go_caches() {
  GOMODCACHE="$(go env GOMODCACHE)"; GOCACHE="$(go env GOCACHE)"; GOPATH="$(go env GOPATH)"
  export GOMODCACHE GOCACHE GOPATH GOPROXY=off GOTOOLCHAIN=local
}
