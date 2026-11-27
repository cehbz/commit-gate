# Operating commit-gate

## Install
`commit-gate install` puts the build that runs it live and wires the host to it. It refuses, changing nothing, unless the build is up to date (`commit-gate version`), so only a build of the target goes live; it is safe to re-run. It changes these, and nothing else:
- `~/.local/libexec/commit-gate/`: the binary `commit-gate`, replaced in one step, its `precheck` and `sessioncheck` links, and `hooks/`, a link to the binary for every name `commit-gate hook-names` prints.
- The global `core.hooksPath`, set to that `hooks/` directory. A value naming another directory is reported, with the command that replaces it, and left as it is.
- `~/.local/bin`: `commit-gate`, a link to the binary.
- `~/.claude/settings.json`: a PreToolUse hook running `precheck` on Bash and the file-editing tools, and a SessionStart hook running `sessioncheck`, each added once; the rest of the file is kept.

The locations are fixed under the home directory, wherever the build that runs `install` sits. To install by hand, run a build of the target from a clean checkout of its tag: in the Mac's clone, with the target checked out and no uncommitted changes, `make build && build/commit-gate install`. The agent may not run `install`.

## Hosts without Go
Cross-compile the target on the Mac from a clean checkout of its tag, writing the build outside the checkout, `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ../commit-gate ./cmd/cg` (`GOARCH=amd64` for x86-64), copy the build to the host and run its `commit-gate install` there.

## Commands
Every command is a word after `commit-gate`, the only name the gate puts on PATH. Any other word is an unknown command.
- `commit-gate approve [-F file | --plan plan.md] [--yes]`: approve commit messages in the current repo: a batch file whose messages are separated by lines reading `@@COMMIT-GATE-SEP@@`, the single-line `-m` messages of a plan, or, bare, the message the last rejected commit left pending.
- `commit-gate approve-push [--yes]`: authorize one push from the current repo. Any later `commit-gate approve` voids it.
- `commit-gate approve-deploy [--yes]`: authorize one `make deploy` run from the current clone. A later `commit-gate approve` leaves it, since a deploy puts only the released target live.
- `commit-gate status`: the repo's state (enabled, opted out, overridden or ungated), with its outstanding approvals and push and deploy authorizations when enabled.
- `commit-gate enable`: clear the repo's opt-out and repo-local `core.hooksPath`, so the global gate applies.
- `commit-gate disable [--yes]`: opt the repo out.
- `commit-gate clear-approvals`: drop the repo's outstanding approvals and its push and deploy authorizations.
- `commit-gate hook-names`: every hook name the gate's hook directory holds.
- `commit-gate canonical`: read a commit message on standard input and print its identity, the SHA-256 of its canonical form.
- `commit-gate version`: whether the build that runs it is the target, the commit the highest release tag on the public repository names. It prints the build's commit (`dirty` when built from changed source, `from no commit` when it records none), the target's tag and commit, the state (`up to date`, `out of date`, or `unknown` when the target can't be read or no release exists) and, when out of date, the fix. It exits non-zero unless the build is up to date.
- `commit-gate install`: put the build that runs it live and wire the host to it (Install, above). It refuses, changing nothing, unless the build is up to date.

`commit-gate approve`, `approve-push`, `approve-deploy` and `disable` ask for confirmation: a dialog on macOS, a prompt on the terminal elsewhere, so on Linux they need a real terminal. Each refuses when the shell command it was launched from runs anything besides `cd` and `commit-gate`.

## Release
A release is a `vMAJOR.MINOR.PATCH` tag on the public repository; the highest one names the target, the commit every host should run. When the Mac's clone has commits since the target, the agent proposes the next version by semver (a breaking change to a command or the install raises MAJOR, a new behavior MINOR, a fix PATCH) and runs `make release VERSION=<tag>` in the clone. It pushes the head to origin's `main` and the tag on it in one atomic push, under one `commit-gate approve-push`, and tags the clone once the push lands. It refuses, changing nothing, when the clone has uncommitted changes, when origin's `main` has commits the head lacks, when the head is already the target, while the tree holds pending model-level tests, or when the tag is not above every release tag.

## Workflow
The agent commits first. The commit check rejects an unapproved message and prints its subject and hash. The user approves the exact message with `commit-gate approve` in that repo and the agent commits again. For a push, the user runs `commit-gate approve-push` after approving. To release, the agent runs `make release VERSION=<tag>` first; the gate rejects its push until the user runs `commit-gate approve-push`, which is the user's approval of the version, and the agent runs it again.
