# Lyceum — decision log

Append-only. Rows are never edited or renumbered; a later decision that changes an earlier one says so
(`Revises Dn`, `Resolves Dn`). The spec (`lyceum/SPEC.md` and `lyceum/spec/`) is the current design;
this file is why it says what it says. The log starts at v1: the decisions before it are folded into the spec.

## Decision register

| # | Decision | Source |
|---|---|---|
| D1 | Lyceum v1: the first public version. Earlier design decisions are folded into the spec | owner, 2026-11-06 |
| D2 | `vN` follows the newest verified prerelease until a stable `vN.x.y` exists; from then on only stable releases move it, and `check-action` accepts a prerelease behind `vN` only when no stable release is there. Revises the release rule in the README and the release workflow that `v1` never moves to a prerelease | owner, 2026-11-06 |
