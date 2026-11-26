# commit-gate — TODO

Threat model is rationalization, not sabotage; the PreToolUse precheck is a best-effort
deterrent, not a boundary.

## Out of scope by design (documented, not forgotten)
Ways an agent could evade the gate that the parser deliberately does not chase — each is
deliberate indirection (sabotage), outside the rationalization threat model:
- Wrapper commands hide the real invocation: `env bash -c '…'`, `sudo bash -c '…'`,
  `xargs bash -c '…'`, `env VAR=val git …`. Only bash/sh/zsh `-c` payloads are unwrapped;
  a wrapper in front of them (or of git) is not followed.
- Combined short flags: `bash -lc '…'` is not recognized as a `-c` payload.
- Whole-`.git` destruction: `rm -rf .git` is not blocked — it destroys history, so it is
  sabotage rather than a way to sneak a commit past the gate.
- A `squash` in an interactive rebase lands a combined message with no commit-msg run: a
  deliberate interactive rebase.
- Replays run no commit-msg (git 2.39.5, 2.47.3, 2.54.0): a rebase's picks under both
  backends, `rebase --continue`, a clean cherry-pick, and a pull with `pull.rebase` and
  `rebase.autoStash`, which every host sets. commit-msg runs for a plain commit, a
  non-fast-forward merge and a `reword`; on 2.54.0 also for `cherry-pick --continue` after a
  conflict and for a conflicted merge finished with `git commit`.
- Indirect global-config writes: `include.path` or `includeIf.<cond>.path` naming a file that
  sets `core.hooksPath`; `git config --edit`/`-e` with an editor from `GIT_EDITOR`, `EDITOR` or
  `-c core.editor`.
- Writers outside R4's verbs: in-place editors other than `sed -i` (`perl -pi`, `python`),
  `ln`, `dd of=` and `install`.
- Directory destinations: `cp x ~`, `cp -t ~ x` and `mv x /r/.git` write
  `<dir>/<basename of the source>`, a protected file when the source is named `.gitconfig`,
  `config` or `hooks/<name>`; only the destination word is checked.
- Abbreviated long options, which git and GNU cp accept when unambiguous: `--unset-al`,
  `--remove-s`, `--typ path`, `--comm c` (the value is read as the key), `cp --target DIR`.
- `$XDG_CONFIG_HOME` spelled as a variable (`> $XDG_CONFIG_HOME/git/config`); only `$HOME`
  and `${HOME}` are resolved.
- `GIT_CONFIG_GLOBAL` set in the agent's environment: the file it names is the global config
  git reads, and the precheck protects only `~/.gitconfig` and `$XDG_CONFIG_HOME/git/config`.
- Plumbing: `git commit-tree` plus `git update-ref` makes a commit without running
  commit-msg, and the precheck allows both.
- Rewriting `cg-pending`, which is outside the protected set, so a bare `approve` records
  the rewritten message; its confirmation dialog shows subjects only, though the terminal
  prints the full messages.
Closing any of these means expanding the threat model from rationalization to sabotage.

## Precheck gaps
- Native hooks: the precheck denies a direct write into `.git/hooks` (cp, mv, rm, tee,
  `sed -i`, a redirect, Write, Edit) but not `ln`, nor a script that creates the hook, such
  as a repo's own `install.sh`.

## Verify empirically (against the running harness)
- Harness behavior when the PreToolUse command is missing or crashes: does the harness fail
  closed? The binary recovers-to-deny, but that only covers the case where it actually runs.

## Ergonomics
- `approve --plan` extracts only single-line `-m "…"` payloads; multi-line / trailer
  messages need `-F` with the `@@COMMIT-GATE-SEP@@` separator.

## Harness (not gate code)
- Pasting `!` commands is unreliable — often needs hand-editing before it runs, and appears
  to echo only the path component. Root cause not isolated.

