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
Closing any of these means expanding the threat model from rationalization to sabotage.

## Verify empirically (against the running harness)
- Harness behavior when the PreToolUse command is missing or crashes: does the harness fail
  closed? The binary recovers-to-deny, but that only covers the case where it actually runs.
- commit-msg under `rebase` / `cherry-pick` (merge and amend are covered).

## Ergonomics
- `approve --plan` extracts only single-line `-m "…"` payloads; multi-line / trailer
  messages need `-F` with the `@@COMMIT-GATE-SEP@@` separator.

## Harness (not gate code)
- Pasting `!` commands is unreliable — often needs hand-editing before it runs, and appears
  to echo only the path component. Root cause not isolated.
