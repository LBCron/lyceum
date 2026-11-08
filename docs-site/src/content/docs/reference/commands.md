---
title: Commands
description: What the lyceum commands do beyond their help text, and the internal commands.
---

Bare `lyceum` prints the commands grouped as Lifecycle, Change and Inspect. Every command ends by printing where the work stands and the next action.

The full list of commands and flags is the [CLI reference](/reference/cli/), generated from the binary. This page keeps what the help text can't say.

`--json` prints the outcome as one JSON object on stdout. `-v`, `--version` prints the version; with `--json` it prints `{"version":"<v>"}`. An unknown command or a bad flag is a refusal (exit 2) with a next step.

## Lifecycle

### `lyceum status`

Per unit it shows the ID, type, branch, status, gate, last cleared gate and next action. It also shows pending approvals, the repo-level `project` and `testing` gates, and the count of drifted or broken records.

### `lyceum check`

See [CI](/guides/ci/#what-check-looks-at) for what it checks.

### `lyceum approve`

Gates you can name: `spec`, `plan`, `intent`, `cause`, `tests`, `project`, `testing`. See [Approvals](/concepts/approvals/).

### `lyceum plan`

On a trivial chore, `lyceum plan` upgrades it to a planned chore.

### `lyceum test`

With a subset name, runs that subset from `TESTING.md`. A subset run is recorded but can't clear a gate.

### `lyceum log`

Merges `lyceum/events.jsonl` with `git log` into one chronological view.

### `lyceum doctor`

Checks setup, never work. In a Lyceum repo it also checks that installed skills match the binary, that the harness hooks and pre-commit hook are present, and that `lyceum/config.yaml` and `TESTING.md` parse. It reports and changes nothing.

## Change

`add`, `refactor` and `chore` each create the unit's plan file and branch and check the branch out. With no title, the agent's interview asks for one. See [Units and gates](/concepts/units-and-gates/).

## Inspect

Inspect commands never edit code.

## Internal

Used by skills, hooks and CI, and not listed in the menu or the CLI reference.

| Command | Purpose |
|---|---|
| `lyceum gate <unit> <gate>` | Move a unit to a gate the CLI allows; refuses illegal transitions |
| `lyceum hook pre-tool --harness <name>` | Harness pre-tool hook entry point; reads the harness payload on stdin |
| `lyceum changed-files` | Files changed on this branch, for `TESTING.md` subset commands |
| `lyceum record <file>` | Fill in a record's id, date and cite hashes, validate it, and commit it |
