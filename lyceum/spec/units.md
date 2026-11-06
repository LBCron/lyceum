# Units of work

A unit is one piece of change work. It lives in one plan file and on one branch. Inspect
commands (`understand`, `trace`, `audit`) are not units: they have no branch, plan or gates.

## Unit types

| Type | Started by | Purpose | Merges into the spec at close |
|---|---|---|---|
| `feat` | `lyceum add feat [title]` | New behaviour | Always |
| `bug` | `lyceum add bug [title]` | File a bug and run the full fix cycle | Only when the spec was silent or wrong: adds an edge case or AC |
| `refactor` | `lyceum refactor [title]` | Change structure, never behaviour | Citations only |
| `chore` | `lyceum chore [title]` | Maintenance: dependency updates, README, config | Never |
| `doc` | `lyceum add doc [title]` | A human doc under `docs/` | Never |
| `test` | `lyceum add test [title]` | Describe one test case; the agent picks its level and writes it | Never |

Bare `lyceum add` offers the four `add` types. With no title, the agent's interview asks for one.

## The plan file

`lyceum/plan/<hash>-<slug>.md`. It is the unit: there is no other unit state.

```yaml
---
id: a3f9c2-csv-export          # CLI-owned
type: feat                     # CLI-owned
title: CSV export
branch: feat/a3f9c2-csv-export # CLI-owned
status: planned                # CLI-owned: planned | in-progress | implemented | closed
gate: plan                     # CLI-owned: current gate
areas: [orders]                # agent-written; each must exist in the registry
files: [src/orders/export.ts, src/orders/routes.ts]   # agent-written: planned scope
acs: [AC14, AC15]              # agent-written: ACs this unit adds; verify requires each mapped
acs_partial: []                # agent-written: ACs this unit proves only in part; they stay open
approvals:                     # CLI-owned, mirrors events.jsonl
  - gate: plan
    hashes: {lyceum/plan/a3f9c2-csv-export.md: "sha256:…"}
    commit: 1a2b3c4
    by: owner@example.com
    at: 2026-10-23T10:12:00Z
    # counts: false            # written only when the approver is not the owner
---
## What and why          (agent: summary of the change; points at the spec delta, never copies the spec)
## Spec delta            (agent: which areas and ACs changed; the change itself is written in the area files)
## Steps                 (agent: ordered chunks, each with the ACs it proves)
## Progress              (agent: per step, done / in progress / not started, with run references)
## Log                   (agent: decisions made and owner input received during the unit)
```

Body sections follow the plan shape set by the owner: explain the feature, which files change, a summary
of progress, and documenting it in the spec (plan discussion).

**Ownership:** the CLI alone writes the CLI-owned fields. If `check` sees a CLI-owned field change
without a matching event, that is a bypass.
`check` finds the current unit through the log as well as the plan: if the current branch matches an `add` event's
`branch` but no plan claims it, or the plan's fields disagree, that is a finding, never "no unit".

## Gate lists (fixed per type)

`M` = mechanical (clears on a CLI-recorded run). `J` = judgment (clears by owner approval).

| Type | Gates in order |
|---|---|
| `feat` | `spec` J (interview → spec delta + ACs + plan, drafted together and approved in one stop; brownfield adds as-built sections for missing areas) → `build` (steps) → `verify` M (full suite passes, every new AC mapped to a test) → `close` |
| `bug` | `repro` M (test-first: a new test fails before any fix exists) → `diagnose` (root cause written in the plan) → `fix` → `verify` M (repro test passes and the full suite passes) → `cause` J: `approve cause` on the branch, hashing the root cause and the fix diff together, before the PR → `close`. A spec AC the bug adds is written into its area file (and the registry range) before `approve cause`, which hashes that area file. Untestable bugs: `approve cause --no-repro "<reason>"` at `repro` records the reason and skips `repro` only; the bug moves to `diagnose`, and `verify` and the final `cause` J are still required. No plan gate |
| `refactor` | `intent` J (what, into what, why, plus a metric if the goal is performance) → `baseline` M (the full suite passes on the start commit; if coverage is thin, characterization tests are added first, and the owner approves them J) → `plan` J → `build` (steps; each step runs a subset and stays green, never clearing a gate) → `verify` M (full suite passes; test files are unchanged since the baseline or every diff is owner-approved J; benchmark if the goal is performance) → `close` |
| `chore` | Two tiers. **Trivial:** `build` → `close`, with no up-front gate; `check` fails the branch if its diff touches code, a manifest, a lockfile or CI config without an approved plan. **Planned:** `lyceum plan` moves the chore to `plan` J → `build` → `verify` M (full suite passes) → `close` |
| `doc` | `draft` → `verify` M (the branch diff has only non-code files, and the full suite passes or the project has no tests at all; running the commands the doc shows is the agent's job in the doc skill) → `close`. No human gate |
| `test` | `write` → `run` M (the new test passes) → `close`. No human gate. If the new test fails on current code, the unit stops and offers conversion to `bug` with the test as the repro |

**Conditional approvals:** refactor's characterization-test approval (inside `baseline`) and test-diff approval
(inside `verify`) are conditions on those gates, not separate gates. The gate does not clear until the required
approval is recorded.

**Status transitions.** The CLI moves `status` only at these points: `planned` → `in-progress` when the
unit's code-opening gate clears (for doc and test units, which have none, when the unit is added); `in-progress` →
`implemented` when `verify` (or test's `run`) passes; `implemented` → `closed` at `close`. A trivial chore is
`in-progress` from `add`. Each move is recorded in the event that caused it (`status` field).

**Code-opening gate:** `spec` for feat, `repro` for bug (or its `--no-repro` skip), `plan` for refactor and
planned chore. Test files written during bug `repro` or refactor `baseline` are allowed before it.

**The floor:** every type that changes application code passes through an approved `plan` or `spec`/`intent` gate and a mechanical
`verify` before it can close. `bug` meets the floor through the
mechanical `repro` gate plus the `cause` approval. `doc` and `test` change only docs or tests and have mechanical gates only.

## Refactor boundaries

- If a refactor turns out to need a behaviour change (a bug, an edge case, an interface change), the agent stops
  and splits it out as `add bug` or `add feat`. A refactor PR never contains a behaviour change.
- Changing a public surface (API route, CLI flag, exported function, DB schema) needs explicit owner approval, or
  the unit is reclassified as `feat`. The plan lists each change under `public_surface:` (agent field) and
  `approve plan` hashes them with the plan. Spotting a public-surface change is the agent's job; the CLI cannot
  detect one in an arbitrary language. `verify` refuses when `public_surface:` changed after `approve plan`.

## Close

`lyceum close [unit]`:
1. Requires the unit's last gate before `close` cleared: `verify` for feat, refactor and planned chore and doc,
   `cause` for bug, `run` for test. A trivial chore needs nothing but `check` passing its diff.
2. For `feat`, checks that every area the plan's `## Spec delta` names is in the `spec` approval's hashed files.
   The spec text was written into the area files at the `spec` gate and approved there, so `close` copies nothing.
For `refactor`, refuses any spec diff other than `cites:` frontmatter.
3. If TESTING.md changed in this unit, refuses until the owner re-runs `approve testing`.
4. Sets status `closed`, writes a `close` event and commits the plan with it. The plan file stays as history.
5. With `gh` installed and a GitHub remote, runs `gh pr create` from the unit branch to the base branch;
   `glab` likewise for GitLab. Otherwise it prints the exact push and merge commands. The merge itself is the
   owner's, and git history records it; no merge event is written.

A **test unit whose test fails**: `close` converts it to a bug instead of closing it. It writes an `add`
event for a new bug unit on the same branch with the failing test recorded as the repro, so the bug starts past
`repro` at `diagnose`, and a `close` event for the test unit with `converted_to: <bug id>`.

For `bug`, `close` requires the `cause` approval. For `feat`, the PR merge carries no extra approval: its judgment gates are already recorded.

## Amend

`lyceum amend [unit] "<reason>"` reopens an approved plan mid-build:
1. Invalidates the approval of the unit's code-opening gate (`spec` for feat, `plan` for refactor and planned chore)
   and every later approval, and moves the gate back to it; status goes back to `planned`. A refactor's `intent`
   stays approved; `amend --intent "<reason>"` goes back to `intent` instead, for when the intent itself changed.
2. Writes an `amend` event (`unit`, `gate`, `reason`, `invalidated`, `status`) and appends the reason to the plan's
   `## Log`.
3. `check` then fails new code changes until the owner re-approves that gate.
Working out what the change touches (areas, ACs, tests, steps) is the agent's job, guided by the skill. A bug has
no plan to amend: `amend` on a bug refuses and points at `add bug` for a new cause.

## Plan command

`lyceum plan` shows the current unit's plan. `lyceum plan --new-area <name>` registers a new area after printing
the existing ones. When `lyceum/spec/<name>.md` already exists with no registry row, it registers that file and
leaves it unchanged. The agent writes registry rows and AC ranges; `check` validates them. `lyceum plan` on a trivial chore upgrades it to a planned chore and moves it to `plan` J.
Re-planning an approved unit is `amend`; there is no `plan --edit`.

## Starting refactor and chore units

`lyceum refactor "<title>"` and `lyceum chore "<title>"` start a unit exactly as `add` does (ID, branch, plan
file, `add` event); a refactor starts at `intent`, a chore as a trivial chore at `build`.

## Roadmap

`lyceum/LIBRARY.md` is the ordered list of likely next units. The agent keeps it and reorders it freely; nothing
hashes it and the owner never approves it. Each entry is one list item, `- <type>: <title>`, optionally followed by
` (AC12, AC13)` naming the ACs it will claim. When no unit is in flight and nothing is pending, `status` prints the top
entry as its `Next:` line (`lyceum add feat "<title>"`, or `lyceum refactor` / `lyceum chore` for those types).

## Concurrency

- Parallel work happens on other branches or other clones, never in one checkout.
- `add` and `close` read `lyceum/plan/` on other local and remote-tracking branches with `git show`, and warn when
  another open unit names the same area. They never block.
- Spec conflicts are resolved in git like any other conflict.

## Interviews

Interview length scales with the change; when the request already answers everything, the agent asks nothing and drafts. Every skill that interviews uses one format: numbered questions, lettered options, recommended option first,
then the agent stops and waits. Where the harness has a structured-question tool, the skill uses it instead.
Interview rules from the source method apply: never ask what the request already answers, options are
concrete alternatives, at most two clarification rounds per document, anything still open becomes
`[NEEDS CLARIFICATION]`.

**After a gate clears.** Any question about whether to proceed is a structured yes/no (yes starts the next
step, no opens a discussion), never an open prose question. The next step is the one `status` prints.

## Brownfield spec growth

When a plan names an area that has no spec file, the `spec` gate (feat) or `plan` gate (refactor, chore) first
has the agent write that area's as-built section from the code, citing files and starting from a matching
`understand` record if one exists. The owner approves it together with the plan. Untouched areas stay unspecified.

## Greenfield

`init` on an empty project runs the project interview and writes the initial `lyceum/spec/<area>.md` files,
approved together as gate `project` J. At greenfield the agent writes the area registry rows in `lyceum/SPEC.md`
itself, since no unit exists yet; `approve project` hashes `SPEC.md` with the area files. After that only the CLI
changes the registry (`plan --new-area`). The first slice of work is then an `add feat`. Security, deploy and UI
are optional areas.

## Acceptance criteria

| # | Criterion |
|---|---|
| AC15 | Given bare `lyceum add`, when it runs interactively, then it offers exactly `feat`, `bug`, `doc`, `test` |
| AC16 | Given `add feat`, when it completes, then the plan has `status: planned`, `gate: spec`, and an `add` event exists in `events.jsonl` |
| AC106 | Given a `feat` whose spec delta and plan are drafted, when the owner runs `approve spec` once, then the build gate opens and the event hashes the plan and every named area file; no separate `approve plan` is required |
| AC107 | Given a bug at `repro` with no repro test, when the owner runs `approve cause --no-repro "visual only"`, then `repro` is skipped, the event records the reason, the bug moves to `diagnose`, and `verify` (full-suite pass) and the final `approve cause` are still required before `close` |
| AC17 | Given a plan, when any CLI-owned frontmatter field differs from what the event log implies, then `check` reports a bypass naming the field |
| AC18 | Given a `feat` at gate `spec`, when `lyceum test` passes, then the gate does not change (judgment gates never clear on runs) |
| AC19 | Given a `feat` with `spec` and `plan` approved and the steps built, when `lyceum test` runs the full suite and it passes and every new AC ID is found in a test, then `verify` clears and a `gate-clear` event with the run inlined is written |
| AC20 | Given a `feat` at `verify`, when the full suite passes but AC17 is not found in any test, then `verify` does not clear and the output names AC17 |
| AC21 | Given an `add bug` at `repro`, when the agent's new test is run by the CLI at HEAD, with a branch diff that touches only test files, and fails, then `repro` clears with no human approval |
| AC22 | Given an `add bug` at `repro`, when the new test passes at HEAD, with a branch diff that touches only test files, then `repro` does not clear and the output says the test does not reproduce the bug |
| AC23 | Given a `bug` unit, when it reaches `close`, then no `plan` approval was ever required |
| AC24 | Given a `refactor` at `baseline`, when the full suite fails on the start commit, then `baseline` does not clear |
| AC25 | Given a `refactor` that passed `baseline`, when a test file's assertions change and the owner has not approved that diff, then `verify` does not clear and the diff is shown |
| AC26 | Given a `refactor` step, when the agent runs `lyceum test changed`, then a run is recorded and no gate clears |
| AC27 | Given a `refactor` whose stated goal is performance, when `intent` is approved without a metric in the plan, then the CLI refuses the approval and names the missing metric |
| AC28 | Given a `chore` branch whose diff touches only `*.md` files, when `check --ci` runs, then it passes with no plan approval |
| AC29 | Given a `chore` branch whose diff touches `package.json`, when `check --ci` runs and no approved plan exists, then it fails |
| AC30 | Given an `add test` unit, when the new test fails against current code, then the agent stops and offers conversion to `add bug` and the test file is unchanged |
| AC31 | Given `close` on a `feat`, when it completes, then every area the plan names was in the `spec` approval's hashed files, the plan has `status: closed`, and a `close` event exists |
| AC32 | Given `close` on a `refactor`, when the area file's diff includes anything other than `cites:` frontmatter, then `close` refuses |
| AC33 | Given `close` when TESTING.md changed in the unit and is not re-approved, then `close` refuses and names `approve testing` |
| AC34 | Given `close` with `gh` installed and a GitHub remote, then a PR is opened; given no `gh`, then the exact next command is printed |
| AC35 | Given `amend` on a unit at `build`, when it completes, then the affected approvals are marked invalid, the gate moves back, and `check` fails new code changes until re-approval |
| AC36 | Given another branch with an open unit naming area `orders`, when `add` creates a unit and the plan names `orders`, then a warning names that branch and the command still succeeds |
| AC37 | Given a plan naming an area not in the registry, when the plan gate is approved, then the approval is refused and the output lists existing areas and `--new-area` |
| AC38 | Given `init` on an empty directory, when the project interview completes and the owner approves `project`, then at least one `lyceum/spec/<area>.md` exists and is registered in `lyceum/SPEC.md` |
| AC100 | Given a `refactor` unit, when the agent finds a needed behaviour change, then it stops, offers `add bug` or `add feat`, and the refactor branch's diff contains no test assertion change the owner has not approved and no change to spec behaviour text |
| AC102 | Given a `bug` with `verify` cleared and no `approve cause` event, when `check --ci` runs on its branch, then it fails and names `approve cause`; after the owner approves, the event hashes the plan and every file in the fix diff |
| AC101 | Given a `refactor` whose plan's `public_surface:` list changed after `approve plan`, when `verify` would clear, then it does not clear and the output names the re-approval |
| AC124 | Given a test unit whose test fails, when `close` runs, then the test unit is closed with `converted_to`, a bug unit exists on the same branch with that test as its repro, and the bug is at `diagnose` |
| AC125 | Given a unit whose last pre-close gate is not cleared, when `close` runs, then it refuses and names that gate |
| AC126 | Given `close` with no `gh` or `glab` or no remote, then it prints the push and merge commands and writes no merge event |
| AC127 | Given `lyceum refactor "Split parser"`, then a refactor unit exists at `intent` on a `refactor/` branch; given `lyceum chore "Bump deps"`, then a chore unit exists at `build` |
| AC128 | Given `lyceum plan` on a trivial chore, then it moves to `plan` and `check` requires the plan approval before code |
| AC129 | Given a doc unit whose branch diff touches a code file, when `lyceum test` passes, then `verify` does not clear and names the file |
| AC130 | Given a refactor with `intent` approved, when the agent writes `## Steps` and `public_surface:` and `check` runs, then there is no approval-integrity finding; when `## What and why` or `goal:` changes, then `check` names the `intent` approval |
| AC131 | Given a refactor at `build`, when `amend --intent "<reason>"` runs, then `intent` and every later approval are invalidated and the gate is `intent`; without `--intent`, `intent` still counts and the gate is `plan` |
| AC132 | Given a project where no file matches `test_files` and a doc unit at `verify` whose diff has no code, when `lyceum test` runs, then `verify` clears with `no_tests: true`; given the same with a code file in the diff, then it refuses and names the file; a feat in the same project still refuses |
| AC154 | Given `lyceum/spec/billing.md` exists and no registry row names it, when `plan --new-area billing` runs, then the row is added, the file is unchanged, and the command does not refuse |
| AC155 | Given no unit in flight, nothing pending, and `lyceum/LIBRARY.md` whose first entry is `- feat: CSV export`, when `status` runs, then `Next:` is `lyceum add feat "CSV export"` |
| AC156 | Given AC9 in an area file, no closed plan listing it in `acs:` and no LIBRARY.md entry naming it, when `check --ci` runs, then a warning names AC9 and the exit code is unaffected by it |
| AC168 | Given a chore moved to `plan` by `lyceum plan` and its plan approved, when the build is committed and `lyceum test` passes, then `verify` clears and `close` succeeds; until `verify` clears, `close` refuses |
