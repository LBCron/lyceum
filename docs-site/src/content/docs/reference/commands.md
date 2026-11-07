---
title: Commands
description: Every lyceum command, its arguments and flags.
---

Bare `lyceum` prints the commands grouped as below. Every command ends by printing where the work stands and the next action.

## Global flags

| Flag | Meaning |
|---|---|
| `--json` | Print the outcome as one JSON object on stdout |
| `-h`, `--help` | Help for any command |
| `-v`, `--version` | The version. With `--json`, prints `{"version":"<v>"}` |

An unknown command or a bad flag is a refusal (exit 2) with a next step.

## Lifecycle

| Command | Purpose |
|---|---|
| `lyceum init` | Set up Lyceum in a repo; greenfield interview or brownfield baseline |
| `lyceum status` | Every unit on this branch and others, its gate and the next action; pending approvals; stale records |
| `lyceum check [--ci]` | Integrity and gate checks; warns locally, fails in CI |
| `lyceum approve [gate]` | Owner-only approval from a real terminal; bare form walks every pending gate |
| `lyceum plan [--new-area <name>]` | Show the plan, add an area, or upgrade a trivial chore |
| `lyceum amend [unit] "<reason>"` | Reopen an approved plan mid-build |
| `lyceum close [unit]` | Fold the spec delta, close the unit, open a PR |
| `lyceum test [subset]` | Run the suite or a subset and record the run |
| `lyceum log` | Project history |
| `lyceum eject` | Remove Lyceum's installed adapters, keep the records |
| `lyceum doctor` | Check the setup: the tool, and the repo's Lyceum integration |

### `lyceum init`

| Flag | Meaning |
|---|---|
| `--harness <name>` | Wire this harness without asking (repeatable): `claude`, `codex`, `commandcode`, `opencode`, `pi` |

### `lyceum status`

| Flag | Meaning |
|---|---|
| `--all` | Show closed units too |

Per unit it shows the ID, type, branch, status, gate, last cleared gate and next action. It also shows pending approvals, the repo-level `project` and `testing` gates, and the count of drifted or broken records.

### `lyceum check`

| Flag | Meaning |
|---|---|
| `--ci` | Fail on any failing finding and write no events |

See [CI](/guides/ci/#what-check-looks-at) for what it checks.

### `lyceum approve`

| Flag | Meaning |
|---|---|
| `--no-repro <reason>` | With `approve cause`, while a bug is at `repro`: skip `repro` for a bug that can't have a repro test, recording the reason |

Gates you can name: `spec`, `plan`, `intent`, `cause`, `tests`, `project`, `testing`. See [Approvals](/concepts/approvals/).

### `lyceum plan`

| Flag | Meaning |
|---|---|
| `--new-area <name>` | Register a new spec area: list the registered ones, add its registry row and create its area file |

On a trivial chore, `lyceum plan` upgrades it to a planned chore.

### `lyceum amend`

| Flag | Meaning |
|---|---|
| `--intent` | On a refactor: go back to `intent`, not `plan`, when the intent itself changed |

### `lyceum test`

| Flag | Meaning |
|---|---|
| `--repro <test>` | Run one named test with `TESTING.md`'s `single:` command to clear a bug's `repro` gate; it must fail |

With a subset name, runs that subset from `TESTING.md`. A subset run is recorded but can't clear a gate.

### `lyceum log`

Merges `lyceum/events.jsonl` with `git log` into one chronological view.

| Flag | Meaning |
|---|---|
| `--since <date>` | Only entries from this date (`YYYY-MM-DD`, UTC) on |
| `--type <type>` | Only events of this type, and no commits |
| `--unit <id>` | Only this unit's events and the commits on its branch |

### `lyceum doctor`

Checks setup, never work. In a Lyceum repo it also checks that installed skills match the binary, that the harness hooks and pre-commit hook are present, and that `lyceum/config.yaml` and `TESTING.md` parse. It reports and changes nothing.

## Change

| Command | Purpose |
|---|---|
| `lyceum add [feat\|bug\|doc\|test] [title]` | Start a unit of work: feat, bug, doc or test |
| `lyceum refactor [title]` | Change structure without changing behaviour |
| `lyceum chore [title]` | Maintenance: dependencies, config, README |

Each creates the unit's plan file and branch and checks the branch out. With no title, the agent's interview asks for one. See [Units and gates](/concepts/units-and-gates/).

## Inspect

Inspect commands never edit code.

| Command | Purpose |
|---|---|
| `lyceum understand [hint]` | Explain the project or part of it |
| `lyceum trace [question]` | Follow one flow end to end |
| `lyceum audit` | Report vulnerable or outdated dependencies |
| `lyceum export <record> [path]` | Publish a record to `docs/` |

### `lyceum trace`

| Flag | Meaning |
|---|---|
| `--run <env>` | Full run against this `TESTING.md` environment: up, seed, the trigger, reset, down |
| `--trigger <command>` | With `--run`: the command that triggers the flow |
| `--yes` | With `--run`: run the commands; without it they are only printed |

## Internal

Used by skills, hooks and CI, and not listed in the menu.

| Command | Purpose |
|---|---|
| `lyceum gate <unit> <gate>` | Move a unit to a gate the CLI allows; refuses illegal transitions |
| `lyceum hook pre-tool --harness <name>` | Harness pre-tool hook entry point; reads the harness payload on stdin |
| `lyceum changed-files` | Files changed on this branch, for `TESTING.md` subset commands |
| `lyceum record <file>` | Fill in a record's id, date and cite hashes, validate it, and commit it |
