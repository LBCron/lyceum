---
title: Units and gates
description: Each piece of work is a unit with one plan file, one branch and a fixed list of gates.
---

A **unit** is one piece of change work. It lives in one plan file, `lyceum/plan/<hash>-<slug>.md`, and on one branch. The plan file is the unit: there is no other unit state.

Inspect commands (`understand`, `trace`, `audit`) are not units: they have no branch, plan or gates.

## Unit types

| Type | Started by | Purpose | Merges into the spec at close |
|---|---|---|---|
| `feat` | `lyceum add feat [title]` | New behaviour | Always |
| `bug` | `lyceum add bug [title]` | File a bug and run the full fix cycle | Only when the spec was silent or wrong: adds an edge case or AC |
| `refactor` | `lyceum refactor [title]` | Change structure, never behaviour | Citations only |
| `chore` | `lyceum chore [title]` | Maintenance: dependency updates, README, config | Never |
| `doc` | `lyceum add doc [title]` | A human doc under `docs/` | Never |
| `test` | `lyceum add test [title]` | Describe one test case; the agent picks its level and writes it | Never |

## IDs and branches

A unit ID is 6 random lowercase hex characters plus a slug derived from the title, for example `a3f9c2-csv-export`. Each unit has its own branch. The prefix follows the type: `feat/`, `fix/` (for `bug`), `refactor/`, `chore/`, `docs/`, `test/`, then the unit ID. A checkout has at most one unit in flight.

## Two kinds of gate

- **Judgment** gates clear by owner approval: `lyceum approve <gate>`. See [Approvals](/concepts/approvals/).
- **Mechanical** gates clear on a test run the CLI executed and recorded: `lyceum test`. See [Acceptance criteria and evidence](/concepts/acceptance-criteria/).

Work steps (`build`, `diagnose`, `fix`, `draft`, `write`) end when the next mechanical gate clears.

## Gate lists

Gate lists are fixed per type. There are no custom gate lists per unit.

| Type | Gates in order |
|---|---|
| `feat` | `spec` (judgment) → `build` → `verify` (mechanical) → `close` |
| `bug` | `repro` (mechanical) → `diagnose` → `fix` → `verify` (mechanical) → `cause` (judgment) → `close` |
| `refactor` | `intent` (judgment) → `baseline` (mechanical) → `plan` (judgment) → `build` → `verify` (mechanical) → `close` |
| `chore`, trivial | `build` → `close` |
| `chore`, planned | `plan` (judgment) → `build` → `verify` (mechanical) → `close` |
| `doc` | `draft` → `verify` (mechanical) → `close` |
| `test` | `write` → `run` (mechanical) → `close` |

### Feature

The `spec` gate covers the interview, the spec change, its acceptance criteria and the plan, drafted together and approved in one stop. `verify` needs the full suite to pass and every new acceptance criterion mapped to a test.

### Bug

The test comes first, and you approve after the fix, not before. At `repro` the agent writes a test that fails because of the bug, commits it, and runs `lyceum test --repro <test>`; the gate clears only if the test fails. The agent writes the root cause in the plan, then fixes the bug. `verify` needs the repro test and the full suite to pass. `lyceum approve cause` hashes the root cause and the fix diff together.

If no automated test can reproduce the bug, the agent explains why and stops. Only you can skip `repro`:

```sh
lyceum approve cause --no-repro "<reason>"
```

`verify` and `cause` are still required. A code commit after `verify` sends the bug back to `fix`.

### Refactor

You approve the intent and the plan. The full suite must pass on the start commit (`baseline`) and again at `verify`. If coverage is thin, characterization tests are added first, and you approve them. Any change to a test file after the baseline needs your approval with `lyceum approve tests`. If a refactor turns out to need a behaviour change, the agent stops and splits it out as a bug or feature.

### Chore

A trivial chore has no gates, but `check` fails one whose diff touches code, a manifest, a lockfile or CI config. `lyceum plan` upgrades it to a planned chore that you approve.

### Doc

Documentation only. No approval; the branch may change only non-code files.

### Test

Nothing needs your approval. If the new test fails on the current code, the unit stops and the agent offers to turn it into a bug. `lyceum close` then starts a bug at `diagnose`, with the test as its repro.

## The code-opening gate

Each type that changes application code has one gate that opens code: `spec` for feat, `repro` for bug (or its `--no-repro` skip), `plan` for refactor and planned chore. Until it clears, the harness hook blocks code writes and `check` flags code in the branch diff. Test files written during bug `repro` or refactor `baseline` are allowed before it.

## Status

The CLI moves `status` only at these points:

- `planned` → `in-progress` when the unit's code-opening gate clears (for doc and test units, when the unit is added; a trivial chore is `in-progress` from the start).
- `in-progress` → `implemented` when `verify` (or test's `run`) passes.
- `implemented` → `closed` at `close`.

## Close

`lyceum close` requires the unit's last gate before `close` to have cleared. It sets the status to `closed` and writes a `close` event; the plan file stays as history. With `gh` installed and a GitHub remote it opens a pull request (`glab` likewise for GitLab); otherwise it prints the exact push and merge commands. The merge itself is yours.

## Amend

`lyceum amend "<reason>"` reopens an approved plan mid-build. It invalidates the approval of the unit's code-opening gate and every later approval, and moves the gate back to it. `check` then fails new code changes until you re-approve. A bug has no plan to amend.

## The plan file

The CLI alone writes `id`, `type`, `branch`, `status`, `gate` and `approvals` in the plan's frontmatter. The agent writes the rest, including `files:` (the planned scope) and `acs:` (the acceptance criteria the unit adds). The body has `What and why`, `Spec delta`, `Steps`, `Progress` and `Log` sections.
