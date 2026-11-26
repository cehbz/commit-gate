#!/bin/bash
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"; . "$DIR/helpers.sh"; . "$DIR/pending.sh"
# PATH holds commit-gate alone, as install.sh's bin directory does.
path_dir="$(mktemp -d)"; ln -s "$DIR/../commit-gate" "$path_dir/commit-gate"; export PATH="$path_dir:$PATH"

an_approved_commit_and_push_go_through_with_commit_gate_on_path() {
  repo="$(mkrepo)"; cd "$repo"

  echo a > a.txt; git add a.txt
  if git commit -m "feat: add a" -m "Co-Authored-By: X <x@x>" >/dev/null 2>&1; then fail 'unapproved commit succeeded'; fi
  commit-gate approve --yes -F <(printf 'feat: add a\n\nCo-Authored-By: X <x@x>') >/dev/null
  git commit -m "feat: add a" -m "Co-Authored-By: X <x@x>" >/dev/null || fail 'approved commit blocked'

  echo a2 >> a.txt; git add a.txt
  git commit --amend --no-edit >/dev/null || fail 'amend blocked'

  echo b > b.txt; git add b.txt
  git commit --no-verify -m "wip" >/dev/null || fail 'human --no-verify blocked'

  bare="$(mktemp -d)"; git -C "$bare" init --bare -q; git remote add origin "$bare"
  if git push origin HEAD >/dev/null 2>&1; then fail 'push without token succeeded'; fi
  commit-gate approve-push --yes >/dev/null
  git push origin HEAD >/dev/null 2>&1 || fail 'approved push failed'
  gate="$(cd "$(git rev-parse --git-common-dir)" && pwd)/commit-gate"
  [ -e "$gate/push-token" ] && fail 'push token not consumed' || true
}

a_repo_starts_enabled_and_disable_makes_it_inert() {
  repo="$(mkrepo)"; cd "$repo"
  commit-gate status | grep -q '^commit-gate: enabled$' || fail 'should start enabled'
  commit-gate disable --yes >/dev/null
  echo c > c.txt; git add c.txt
  git commit -m "anything" >/dev/null || fail 'opted-out repo still gated'
}

an_approved_commit_and_push_go_through_with_commit_gate_on_path
pending a_repo_starts_enabled_and_disable_makes_it_inert
echo "OK: test_integration"
