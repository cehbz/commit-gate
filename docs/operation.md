# Operating commit-gate

## Install
`./install.sh` builds the binary and wires it in; it is idempotent, so re-run it after a pull. It needs Go, `jq` and `git`. It changes these, and nothing else:
- `~/.local/libexec/commit-gate/`: the binary `commit-gate`, its `precheck` and `sessioncheck` links, and `hooks/`, a link to the binary for every name `commit-gate hook-names` prints.
- The global `core.hooksPath`, set to that `hooks/` directory. A global value naming another directory is reported, with the command that replaces it, and left as it is.
- `~/.local/bin`: `approve`, `approve-push`, `clear-approvals`, `gate-disable`, `gate-enable` and `gate-status`, each a link to the binary.
- `~/.claude/settings.json`: a PreToolUse hook running `precheck` on Bash and the file-editing tools, and a SessionStart hook running `sessioncheck`. The previous file is kept as `settings.json.bak`.

`CLAUDE_DIR`, `BIN_DIR` and `LIBEXEC` override the three locations. Changes to the source go live only when `install.sh` runs again.

## Hosts without Go
Cross-compile on a host with Go, `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o commit-gate ./cmd/cg` (`GOARCH=amd64` for x86-64), copy the binary over, and wire it by hand as `install.sh` would:
```bash
mkdir -p ~/.local/libexec/commit-gate/hooks ~/.local/bin
install -m 755 commit-gate ~/.local/libexec/commit-gate/commit-gate
cd ~/.local/libexec/commit-gate && ln -sf commit-gate precheck && ln -sf commit-gate sessioncheck
for h in $(./commit-gate hook-names); do ln -sf ../commit-gate "hooks/$h"; done
for n in approve approve-push clear-approvals gate-disable gate-enable gate-status; do ln -sf ~/.local/libexec/commit-gate/commit-gate ~/.local/bin/$n; done
git config --global core.hooksPath ~/.local/libexec/commit-gate/hooks
```
Add the two hooks to `~/.claude/settings.json` as `install.sh` does, if Claude Code runs there.

## Commands
- `approve [-F file | --plan plan.md] [--yes]`: approve commit messages in the current repo: a batch file whose messages are separated by lines reading `@@COMMIT-GATE-SEP@@`, the single-line `-m` messages of a plan, or, bare, the message the last rejected commit left pending.
- `approve-push [--yes]`: authorize one push from the current repo. Any later `approve` voids it.
- `gate-status`: the repo's state (enabled, opted out, overridden or ungated), with its outstanding approvals and push authorization when enabled.
- `gate-enable`: clear the repo's opt-out and repo-local `core.hooksPath`, so the global gate applies.
- `gate-disable [--yes]`: opt the repo out.
- `clear-approvals`: drop the repo's outstanding approvals and push authorization.
- `commit-gate hook-names`: every hook name the gate's hook directory holds.

`approve`, `approve-push` and `gate-disable` ask for confirmation: a dialog on macOS, a prompt on the terminal elsewhere, so on Linux they need a real terminal. Each refuses when the shell command it was launched from runs anything besides `cd` and these commands.

## Workflow
The agent commits first. The commit check rejects an unapproved message and prints its subject and hash. The user approves the exact message with `approve` in that repo and the agent commits again. For a push, the user runs `approve-push` after approving.
