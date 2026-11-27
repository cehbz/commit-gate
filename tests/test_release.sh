#!/bin/bash
# make release VERSION=<tag> in a clone pushes the head and a release tag on
# it in one push, or refuses and changes nothing (DESIGN.md, Release). Each
# case runs make release in a scratch clone, holding the working tree without
# its model-level tests, against a scratch remote that the private global git
# config also names as the public address.
set -eu
DIR="$(cd "$(dirname "$0")" && pwd -P)"; . "$DIR/helpers.sh"; . "$DIR/pending.sh"; . "$DIR/scratch.sh"
CG="$DIR/../commit-gate"
tmp="$(mktemp -d)"; tmp="$(cd "$tmp" && pwd -P)"; trap 'rm -rf "$tmp"' EXIT
base_config="$GIT_CONFIG_GLOBAL"   # names the gate under test

# prepare, on first use: tree, the working tree without its model-level tests,
# so the scratch clone holds no pending test mid-plan.
prepare() {
  [ -z "${prepared:-}" ] || return 0
  tree="$tmp/tree"; mkdir "$tree"; scratch_tree "$tree"
  find "$tree" -name '*_test.go' ! -name pending_test.go -delete
  rm -f "$tree"/tests/test_*.sh
  prepared=1
}

# new_repos: a scratch remote and a clone of it holding one commit, the head
# of both, with no release tag; the public address names the remote.
new_repos() {
  prepare; GIT_CONFIG_GLOBAL="$base_config"
  remote="$(mktemp -d "$tmp/remote.XXXXXX")"; git init -q --bare -b main "$remote"
  clone="$(mktemp -d "$tmp/clone.XXXXXX")"; cp -R "$tree/." "$clone/"
  git -C "$clone" init -q -b main; scratch_commit "$clone" 'feat: first'
  git -C "$clone" remote add origin "$remote"
  git -C "$clone" -c core.hooksPath=/dev/null push -q -u origin main 2>/dev/null
  GIT_CONFIG_GLOBAL="$(mktemp "$tmp/gitconfig.XXXXXX")"; export GIT_CONFIG_GLOBAL
  cp "$base_config" "$GIT_CONFIG_GLOBAL"
  git config --global "url.$remote.insteadOf" "$PUBLIC_URL"
}

new_commit() { echo "$1" >> "$clone/CHANGES"; scratch_commit "$clone" "$1"; }
release() { ( cd "$clone" && make -s release "$@" ) > "$tmp/out" 2>&1; }
authorize_push() { ( cd "$clone" && "$CG" approve-push --yes ) >/dev/null; }
head() { git -C "$clone" rev-parse HEAD; }
remote_tag() { git -C "$remote" rev-parse -q --verify "refs/tags/$1^{commit}" || true; }
local_tag() { git -C "$clone" rev-parse -q --verify "refs/tags/$1^{commit}" || true; }

# state: the clone's branches, tags, head, changes and push authorization, and the remote's refs.
state() {
  git -C "$clone" for-each-ref refs/heads refs/tags; head; git -C "$clone" status --porcelain
  [ ! -e "$clone/.git/commit-gate/push-token" ] || echo 'push authorization live'
  echo remote; git -C "$remote" for-each-ref
}

# assert_release_refused <what> <make args>...: release refuses and changes nothing.
assert_release_refused() {
  local what="$1" before; shift; before="$(state)"
  if release "$@"; then fail "release went through: $what"; fi
  assert_eq "$(state)" "$before" "a refused release changed something: $what"
}

# assert_released <tag>: the remote's main and tag, and the clone's tag, name the head.
assert_released() {
  assert_eq "$(git -C "$remote" rev-parse refs/heads/main)" "$(head)" "the remote's main after release $1"
  assert_eq "$(remote_tag "$1")" "$(head)" "the remote's $1"
  assert_eq "$(local_tag "$1")" "$(head)" "the clone's $1 once the push landed"
}

release_pushes_the_head_and_its_tag_in_one_push_under_one_push_authorization() {
  new_repos; new_commit 'feat: second'; authorize_push
  release VERSION=v1.0.0 || fail "release failed: $(cat "$tmp/out")"
  assert_released v1.0.0
  [ ! -e "$clone/.git/commit-gate/push-token" ] || fail 'the push authorization was not consumed'
}

release_without_a_push_authorization_lands_nothing_and_makes_no_tag() {
  new_repos; new_commit 'feat: second'
  assert_release_refused 'no push authorization' VERSION=v1.0.0
  assert_eq "$(local_tag v1.0.0)" '' 'a refused push left a tag in the clone'
}

a_release_refused_for_its_push_goes_through_once_the_push_is_authorized() {
  new_repos; new_commit 'feat: second'
  if release VERSION=v1.0.0; then fail 'release went through without a push authorization'; fi
  authorize_push
  release VERSION=v1.0.0 || fail "release after the push authorization failed: $(cat "$tmp/out")"
  assert_released v1.0.0
}

a_release_whose_push_the_remote_rejects_makes_no_tag_anywhere() {
  new_repos; new_commit 'feat: second'
  printf '#!/bin/sh\n[ "$1" != refs/heads/main ]\n' > "$remote/hooks/update"; chmod +x "$remote/hooks/update"
  authorize_push
  if release VERSION=v1.0.0; then fail 'release went through though the remote rejected its head'; fi
  assert_eq "$(remote_tag v1.0.0)" '' 'the remote holds the tag of a rejected release'
  assert_eq "$(local_tag v1.0.0)" '' 'the clone holds the tag of a rejected release'
}

release_refuses_uncommitted_changes() {
  new_repos; new_commit 'feat: second'; authorize_push
  echo 'not committed' >> "$clone/README.md"
  assert_release_refused 'a changed file' VERSION=v1.0.0
  git -C "$clone" add README.md
  assert_release_refused 'a staged change' VERSION=v1.0.0
}

release_refuses_on_a_branch_other_than_main() {
  new_repos; git -C "$clone" checkout -q -b topic; new_commit 'feat: on topic'; authorize_push
  assert_release_refused 'the current branch is topic' VERSION=v1.0.0
}

release_refuses_on_a_detached_head() {
  new_repos; new_commit 'feat: second'; git -C "$clone" checkout -q --detach; authorize_push
  assert_release_refused 'a detached head' VERSION=v1.0.0
}

release_refuses_when_the_remote_has_commits_the_head_lacks() {
  new_repos; other="$(mktemp -d "$tmp/other.XXXXXX")"
  git clone -q "$remote" "$other"; echo elsewhere >> "$other/CHANGES"; scratch_commit "$other" 'feat: elsewhere'
  git -C "$other" -c core.hooksPath=/dev/null push -q origin main 2>/dev/null
  authorize_push
  assert_release_refused 'the head is behind the remote' VERSION=v1.0.0
  new_commit 'feat: here'
  assert_release_refused 'the head and the remote have diverged' VERSION=v1.0.0
}

release_refuses_when_the_head_is_the_targets_commit() {
  new_repos; new_commit 'feat: second'; authorize_push
  release VERSION=v1.0.0 || fail "first release failed: $(cat "$tmp/out")"
  authorize_push
  assert_release_refused 'no commits since the target' VERSION=v1.0.1
}

release_refuses_while_the_tree_holds_a_pending_go_test() {
  new_repos
  printf 'package main_test\n\nimport "testing"\n\nfunc TestSomethingNew(t *testing.T) {\n\tpending(t)\n}\n' > "$clone/cmd/cg/new_test.go"
  scratch_commit "$clone" 'test: a pending test'; authorize_push
  assert_release_refused 'a pending Go test' VERSION=v1.0.0
}

release_refuses_while_the_tree_holds_a_pending_acceptance_case() {
  new_repos
  printf '#!/bin/bash\n. "$DIR/pending.sh"\nsomething_new() { :; }\npending something_new\n' > "$clone/tests/test_new.sh"
  scratch_commit "$clone" 'test: a pending case'; authorize_push
  assert_release_refused 'a pending acceptance case' VERSION=v1.0.0
}

release_refuses_a_version_not_above_every_release_tag() {
  new_repos; new_commit 'feat: second'; authorize_push
  release VERSION=v1.2.0 || fail "first release failed: $(cat "$tmp/out")"
  new_commit 'feat: third'; authorize_push
  assert_release_refused 'a version equal to the highest' VERSION=v1.2.0
  assert_release_refused 'a version below the highest' VERSION=v1.1.9
  assert_release_refused 'a lower major version' VERSION=v0.9.0
}

release_refuses_a_version_that_is_not_a_release_tag() {
  new_repos; new_commit 'feat: second'; authorize_push
  assert_release_refused 'no VERSION'
  assert_release_refused 'no v' VERSION=1.0.0
  assert_release_refused 'two parts' VERSION=v1.0
  assert_release_refused 'a pre-release suffix' VERSION=v1.0.0-rc1
}

release_orders_versions_by_semver_not_by_text() {
  new_repos; new_commit 'feat: second'; authorize_push
  release VERSION=v1.9.0 || fail "release v1.9.0 failed: $(cat "$tmp/out")"
  new_commit 'feat: third'; authorize_push
  release VERSION=v1.10.0 || fail "release v1.10.0 after v1.9.0 failed: $(cat "$tmp/out")"
  assert_released v1.10.0
  new_commit 'feat: fourth'; authorize_push
  assert_release_refused 'v1.9.1 after v1.10.0' VERSION=v1.9.1
}

release_pushes_the_head_and_its_tag_in_one_push_under_one_push_authorization
release_without_a_push_authorization_lands_nothing_and_makes_no_tag
a_release_refused_for_its_push_goes_through_once_the_push_is_authorized
a_release_whose_push_the_remote_rejects_makes_no_tag_anywhere
release_refuses_uncommitted_changes
release_refuses_on_a_branch_other_than_main
release_refuses_on_a_detached_head
release_refuses_when_the_remote_has_commits_the_head_lacks
release_refuses_when_the_head_is_the_targets_commit
release_refuses_while_the_tree_holds_a_pending_go_test
release_refuses_while_the_tree_holds_a_pending_acceptance_case
release_refuses_a_version_not_above_every_release_tag
release_refuses_a_version_that_is_not_a_release_tag
release_orders_versions_by_semver_not_by_text
echo "OK: test_release"
