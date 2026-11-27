# Operating commit-gate

## Install
`./install.sh` builds the binary and wires it in; it is idempotent, so re-run it after a pull. It needs Go, `jq` and `git`. It changes these, and nothing else:
- `~/.local/libexec/commit-gate/`: the binary `commit-gate`, its `precheck` and `sessioncheck` links, and `hooks/`, a link to the binary for every name `commit-gate hook-names` prints.
- The global `core.hooksPath`, set to that `hooks/` directory. A global value naming another directory is reported, with the command that replaces it, and left as it is.
- `~/.local/bin`: `commit-gate`, a link to the binary.
- `~/.claude/settings.json`: a PreToolUse hook running `precheck` on Bash and the file-editing tools, and a SessionStart hook running `sessioncheck`. The previous file is kept as `settings.json.bak`.

`CLAUDE_DIR`, `BIN_DIR` and `LIBEXEC` override the three locations. Changes to the source go live only when `install.sh` runs again.

## Hosts without Go
Cross-compile on a host with Go, `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o commit-gate ./cmd/cg` (`GOARCH=amd64` for x86-64), copy the binary over, and wire it by hand as `install.sh` would:
```bash
mkdir -p ~/.local/libexec/commit-gate/hooks ~/.local/bin
install -m 755 commit-gate ~/.local/libexec/commit-gate/commit-gate
cd ~/.local/libexec/commit-gate && ln -sf commit-gate precheck && ln -sf commit-gate sessioncheck
for h in $(./commit-gate hook-names); do ln -sf ../commit-gate "hooks/$h"; done
ln -sf ~/.local/libexec/commit-gate/commit-gate ~/.local/bin/commit-gate
git config --global core.hooksPath ~/.local/libexec/commit-gate/hooks
```
Add the two hooks to `~/.claude/settings.json` as `install.sh` does, if Claude Code runs there.

## Commands
Every command is a word after `commit-gate`, the only name the gate puts on PATH. Any other word is an unknown command.
- `commit-gate approve [-F file | --plan plan.md] [--yes]`: approve commit messages in the current repo: a batch file whose messages are separated by lines reading `@@COMMIT-GATE-SEP@@`, the single-line `-m` messages of a plan, or, bare, the message the last rejected commit left pending.
- `commit-gate approve-push [--yes]`: authorize one push from the current repo. Any later `commit-gate approve` voids it.
- `commit-gate status`: the repo's state (enabled, opted out, overridden or ungated), with its outstanding approvals and push authorization when enabled.
- `commit-gate enable`: clear the repo's opt-out and repo-local `core.hooksPath`, so the global gate applies.
- `commit-gate disable [--yes]`: opt the repo out.
- `commit-gate clear-approvals`: drop the repo's outstanding approvals and push authorization.
- `commit-gate hook-names`: every hook name the gate's hook directory holds.
- `commit-gate canonical`: read a commit message on standard input and print its identity, the SHA-256 of its canonical form.
- `commit-gate version`: whether the build that runs it is the target, the commit the highest release tag on the public repository names. It prints the build's commit (`dirty` when built from changed source, `from no commit` when it records none), the target's tag and commit, the state (`up to date`, `out of date`, or `unknown` when the target can't be read or no release exists) and, when out of date, the fix. It exits non-zero unless the build is up to date.

`commit-gate approve`, `approve-push` and `disable` ask for confirmation: a dialog on macOS, a prompt on the terminal elsewhere, so on Linux they need a real terminal. Each refuses when the shell command it was launched from runs anything besides `cd` and `commit-gate`.

## Workflow
The agent commits first. The commit check rejects an unapproved message and prints its subject and hash. The user approves the exact message with `commit-gate approve` in that repo and the agent commits again. For a push, the user runs `commit-gate approve-push` after approving.
