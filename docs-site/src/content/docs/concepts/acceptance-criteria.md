---
title: Acceptance criteria and evidence
description: How acceptance criteria map to tests, and what counts as a run that can clear a gate.
---

## Acceptance criteria

The spec lives in `lyceum/spec/<area>.md`, indexed by `lyceum/SPEC.md`. Each area carries acceptance criteria, numbered globally: `AC1`, `AC2`, and so on. Numbers are never reused, and withdrawn criteria stay in place, marked withdrawn.

A feature's `spec` gate adds criteria to an area, and its plan lists them in `acs:`. A plan may list a criterion in `acs_partial:` when it proves only part of it; that criterion stays open until a closed plan lists it in `acs:`.

## Mapping a criterion to a test

A criterion is mapped when its ID (`AC14`) appears as a whole word in a test name or a comment in a file that matches the `test_files:` patterns in `TESTING.md`. `check` finds mappings by grep.

A feature's `verify` gate needs every criterion the unit added to be mapped.

## `TESTING.md`

`TESTING.md` lives at the repo root. The agent drafts it on the first `lyceum test`, from the test frameworks it detects, and you approve it with `lyceum approve testing`. Any later edit needs re-approval before mechanical gates can rely on it.

Its frontmatter is the only part the CLI executes:

| Key | What it is |
|---|---|
| `suite` | The full suite: the only command that can clear a gate |
| `single` | One named test, with `{name}` in place of the test; used by `--repro` |
| `test_files` | Globs for test files, used for mapping |
| `subsets` | Named commands you define, run with `lyceum test <subset>` |
| `environments` | Environments for a full-run `trace` |
| `timeout` | A run over this is killed and recorded as failed |

The body is guidance for the agent and humans.

## Gate evidence

Every `lyceum test` run is recorded under `.lyceum/runs/`, with the command, exit code, duration, commit and a hash of the captured output. A run is **gate evidence** only if:

- it ran `suite`, not a subset,
- the working tree was clean,
- `TESTING.md` matched its approved hash, and
- the commit is the branch HEAD.

`lyceum test` clears the unit's current mechanical gate itself when the run is gate evidence and the gate's condition holds. There is no separate clear command.

## Mechanical gates

| Gate | Clears when |
|---|---|
| `verify` (feat) | A gate-evidence run exits 0 and every criterion the unit added is mapped |
| `repro` (bug) | `lyceum test --repro <test>` on a clean tree, with a branch diff touching only test files: the test fails |
| `verify` (bug) | A gate-evidence run exits 0, which includes the repro test |
| `baseline` (refactor) | A gate-evidence run on the start commit exits 0, and the CLI fingerprints the test files |
| `verify` (refactor) | A gate-evidence run exits 0, and test files match the baseline or each change is owner-approved |
| `run` (test) | The new test runs in a gate-evidence run and passes |

Once `repro` clears, the repro test is frozen: new tests may be added to the same file, but changing or removing a line of it needs owner approval.

## Stale evidence

If a feature, refactor, planned chore or doc has a code commit after its `verify` cleared, the run no longer describes HEAD. Local `check` warns, `check --ci` fails, and `close` refuses until a new gate-evidence run at HEAD passes.

## A failing suite

When the suite fails, `lyceum test` prints the last 50 lines and the path of the full output. The agent offers `add bug` for each new failure. A failing test is never edited.
