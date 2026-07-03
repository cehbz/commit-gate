# commit-gate — TODO

Known limitations and follow-ups. Threat model is rationalization, not sabotage; the
PreToolUse precheck is an explicit Tier-1 best-effort deterrent, not a boundary.

## Verify empirically (against the running harness)
- Exact SessionStart hook JSON shapes
- Whether a missing/erroring PreToolUse command fails **closed**. If it fails open, a stale/broken precheck path silently lapses the deterrent.
- commit-msg behavior under `rebase` / `cherry-pick` / `-c` / `-C` (merge is confirmed).

## precheck (Tier-1 best-effort) — over-broad matches (observed live)
- Denies **reading** `core.hooksPath`: `grep -Eq 'core\.hooksPath'` matches `git config --get core.hooksPath`, not just a set/unset. Scope the deny to set/unset forms.
- The `commit-gate` matches are unanchored: the Bash write-pattern denies any command mentioning a `commit-gate` substring (incl. the gitignored design-doc filename); the Write/Edit branch denies any `commit-gate/` path segment — so the agent cannot edit the gate's own non-security docs (e.g. this TODO). Scope to gate state (`.git/commit-gate/`) + the scripts, or accept that gate maintenance is human-only.
- The core.hooksPath enable-allow is a substring match (`grep -qF`), so `<gate>/sub` is falsely allowed (a bypass).
- The global precheck has no per-repo opt-out (unlike the commit gate's `gate-disable`); its false-positives apply to the agent everywhere until removed from settings.json.

## installer
- Hook-merge idempotency is keyed on the absolute `$SCRIPT_DIR` path; if the repo moves, the entry can duplicate and the stale path may point at a missing precheck. Consider keying on a stable marker.

## ergonomics
- `approve --plan` parses only single-line `-m "…"`; multi-line / trailer messages need `-F` with the `@@COMMIT-GATE-SEP@@` separator.
- Tests leak temp dirs (no `trap … EXIT`); cosmetic.

## bang command flakiness
- cut and paste of ! commands seems unreliable, often (always?) needs hand editing before it works. Seems to echo just the path component?
- bare approve via the ! shell reported recorded 1 approval(s) and consumed cg-pending, but the manifest was never appended (0 bytes, mtime unchanged) — observed 2026-07-03 in the claude-knowledge repo; commit subsequently rejected.