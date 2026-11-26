# commit-gate

Git hooks and Claude Code hooks that block any commit or push whose exact message a human hasn't approved.

## What
commit-gate makes approval of a commit a record the user writes, not a belief a coding agent holds. A commit goes through only when its exact message has been approved with `commit-gate approve`, and a push only when `commit-gate approve-push` has authorized it. It gates every repo on a host from the moment the repo exists, and keeps a Claude Code agent from loosening the gate or approving on the user's behalf.

## Why
Agents commit and push under my name, and nothing should land there without my approval of its exact text. Telling an agent to wait for approval isn't enough: an agent can talk itself into "that was approved" and commit anyway. The gate makes approval something only a human action can record, so that rationalization fails at the hook.

## How
One cgo-free Go binary, for macOS and Linux, serves every hook, by the name it is run as, and every command, as a word after `commit-gate`, the only name it puts on PATH. `install.sh` builds it, points the global `core.hooksPath` at its hook directory and adds two Claude Code hooks to `settings.json`. The `commit-msg` hook admits a commit whose canonical message hash is in the repo's approval list and consumes the approval; `pre-push` consumes a one-shot push authorization; every other hook passes through to the repo's own. `commit-gate approve` asks for confirmation in a macOS dialog or on the terminal, and refuses when it was pasted in a chain that runs anything else. The PreToolUse precheck parses each Bash command with [mvdan.cc/sh](https://github.com/mvdan/sh) and denies the agent's bypasses: `--no-verify`, opting a repo out, redirecting `core.hooksPath`, running the human-only commands, and writing gate state, hooks or git config. A SessionStart check reports a host or repo the gate doesn't cover. The domain model is in [DESIGN.md](DESIGN.md).

## How it differs
Telling an agent in its instructions to wait for approval doesn't work: the agent decides whether approval happened, and the reasoning that concludes "this was approved" is the reasoning that makes the commit. commit-gate takes that decision away from the agent. I haven't compared it with other commit gates or hook managers. Approaches considered and rejected on the way here: signing approvals through the 1Password SSH agent, which authorizes an application along with all its subprocesses, so one approval would let the agent sign a forged one silently; a confirmation prompt inside the hook, whose input the agent controls and can answer; a check in the agent's session alone, which can't see a commit run inside an ssh heredoc; hooks enabled repo by repo or through `init.templateDir`, which miss repos made before the change and which husky or `git lfs install` overwrite; and a precheck that matched command text, which couldn't tell a read from a write or one clause from the next, so the precheck now parses the shell. Moving the precheck into root-owned managed settings would stop the agent editing it; it is left out because the threat is rationalization, not sabotage.

## What it doesn't do
It defends against an agent rationalizing, not against sabotage: deliberate indirection such as a wrapper command (`env bash -c`), an interactive-rebase squash, or a replay (rebase picks, a clean cherry-pick, a rebasing pull), none of which runs `commit-msg`, is out of scope, and [TODO.md](TODO.md) lists each such gap. It doesn't gate a program that commits with `-c core.hooksPath=/dev/null` or through libgit2, which runs no hooks, nor a repo whose git doesn't read the user's global config, such as root's. It doesn't sign commits.

## More
- Install, the files it changes, the commands, and hosts without Go: [docs/operation.md](docs/operation.md)
- Install: `./install.sh` (needs Go, `jq` and `git`; safe to re-run after a pull)
- Unit and model-level tests: `make test`
- Acceptance suite against a build tree: `make accept`
- Known gaps and open work: [TODO.md](TODO.md)
