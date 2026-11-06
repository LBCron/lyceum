# Architecture

## The split: the CLI keeps state, the agent thinks

| The CLI (`lyceum`, deterministic) | The agent (skills, any harness) |
|---|---|
| Creates files, assigns IDs, creates branches | Runs interviews |
| Owns every status and gate transition | Drafts plans, spec areas, records, TESTING.md |
| Runs tests and records the runs | Builds code inside an approved plan |
| Computes and checks hashes | Reads CLI output and acts on the printed next step |
| Refuses illegal transitions | Never runs `approve` and never edits CLI-owned fields |
| Records approvals, from the owner's terminal only | |

Every CLI command ends by printing where the work stands and the next action, including which skill to load.
That printed text is the handoff to the agent (direction v2). With `--json`, the same information is
emitted as one JSON object.

## File layout (in a project that uses Lyceum)

```
<root>/
  AGENTS.md            always loaded: what Lyceum is, how to find state, never approve
  RULES.md             always loaded: owner-owned principles and Always / Ask first / Never tiers
  TESTING.md           always loaded: frontmatter the CLI executes + guidance
  CLAUDE.md            one-line pointer to AGENTS.md
  .gitattributes       lyceum/events.jsonl merge=union
  lyceum/
    SPEC.md            area registry and index
    spec/<area>.md     project spec by area
    plan/<hash>-<slug>.md   units
    understand/<hash>-<slug>.md
    trace/<hash>-<slug>.md
    audit/<date>-<hash>.md
    events.jsonl       committed event log
    config.yaml        project config (see below)
  .lyceum/             gitignored: runs/, cache
  docs/                human docs: add doc output, exports
```

Inspect commands write only under `lyceum/` (and under `docs/` on an explicit export). They never write code.

## Config

`lyceum/config.yaml` is committed, so every clone sees the same owner and settings.

| Key | Meaning | Default |
|---|---|---|
| `owner` | Owner identity: git email | `git config user.email` at `init` |
| `base_branch` | Branch units are cut from | The remote's default branch; with no remote, the current branch |
| `approvals.signed` | Require signed approval commits | `false` |
| `approvals.harness_prompt` | Allow the harness-prompt approval route where the harness supports it | `allow` |
| `harnesses` | Harnesses wired at `init` | detected |
| `hooks` | Harness hooks installed | `true` where supported |
| `understand.warn_threshold` | Scope above which `understand` warns about cost | 2,000 files or 200,000 lines |

## IDs and branches

- A unit ID is 6 random lowercase hex characters plus a slug, e.g. `a3f9c2-csv-export`.
  The slug is derived from the title: lowercase, `[a-z0-9-]`, 40 characters max.
- On a collision with an existing ID in the checkout, the CLI regenerates the hash.
- Each unit has its own branch. The prefix follows the type: `feat/`, `fix/` (for `bug`), `refactor/`, `chore/`,
  `docs/`, `test/`, then the unit ID. This replaces the source method's single `feature/<name>` convention.
- Branch names never contain an agent name (source method git rules).
- `add`, `refactor` and `chore` create and check out the branch. A checkout has at most one unit in flight.
- **Worktrees are the agent's call.** Lyceum neither requires nor forbids them. A git worktree is its own
  checkout, so it holds at most one unit, with its own `lyceum/` files and `.lyceum/runs/`. The CLI must work
  correctly inside one: `.git` may be a file, refs are shared, and `status` lists units in every worktree
  (`git worktree list`). `lyceum approve <unit>` run from any checkout finds the unit's worktree and records the
  approval there, so the owner never has to `cd` into a worktree the agent created. **Deferred after v1:**
AC114 and AC115 stay in the spec, marked deferred, and release coverage does not require them.

## Logs

| Log | Path | Committed | Contents |
|---|---|---|---|
| Event log | `lyceum/events.jsonl` | yes, `merge=union` | `add`, `approve`, `gate-clear` (with the clearing run inlined), `gate-reopen`, `close` (with `converted_to` for a converted test unit), `amend` (`unit`, `gate`, `reason`, `invalidated`, `status`), `bypass` |
| Run log | `.lyceum/runs/<timestamp>-<id>.json` + output file | no | Every test or trace run: command, exit code, duration, commit, sha256 of the stored output file |

An event line is one JSON object with at least `ts`, `type`, `unit`, `actor`, `commit`, plus type-specific fields such as an approval's hashes.
The `add` event carries `unit_type` (the unit's type; `type` is the event type), `branch`, `gate` and `status`, so every CLI-owned plan field can be rebuilt from the log.
A `gate-reopen` event (`unit`, `gate`, `status`, `reason`, `commit`) records a gate moving back, e.g. a bug sent from `cause`
to `fix` by a code commit after `verify`; the CLI writes it the first time it notices, then moves the plan's `gate`, and `status` back to `in-progress`. `lyceum plan` upgrading a trivial chore writes one too: `gate: plan`, `status: planned`, `reason: chore upgraded to planned`.

## Acceptance criteria

| # | Criterion |
|---|---|
| AC5 | Given any command except the hidden helper `changed-files` (whose human output is only paths, for use in `$(…)`), when it finishes, then its last output names the unit (if any), the current gate and the next action; with `--json` the same fields are emitted as one JSON object |
| AC6 | Given `lyceum add feat "CSV export"`, when it runs, then a plan file `lyceum/plan/<6 hex>-csv-export.md` exists and branch `feat/<6 hex>-csv-export` is checked out |
| AC7 | Given a generated hash that matches an existing plan's hash, when `add` runs, then a different hash is generated and no file is overwritten |
| AC8 | Given a checkout with a unit whose status is not `closed` on the current branch, when `add` runs, then it refuses and names the open unit |
| AC9 | Given `init`, when it completes, then `.gitattributes` contains `lyceum/events.jsonl merge=union`, and `.lyceum/` is gitignored |
| AC10 | Given two branches that each append one event, when they are merged in either order, then the merge has no conflict and both lines are present |
| AC11 | Given 30 `lyceum test` runs, when `git status` is checked, then no tracked file changed as a result of the runs except at most one `gate-clear` event |
| AC12 | Given `understand`, `trace` or `audit`, when it runs, then no file outside `lyceum/` changes (and outside `docs/` when an export was confirmed), except what the project's own environment commands do under a confirmed `trace --run <env> --yes` |
| AC13 | Given `init` in a git repo, when it completes, then `lyceum/config.yaml` exists with `owner` equal to `git config user.email` |
| AC109 | Given uncommitted changes, when `add` runs, then it refuses, lists the changed files, and prints the commit or stash command |
| AC110 | Given a remote whose default branch is `main` while `dev` is checked out, when `add feat` runs, then the unit branch is cut from `main`; given `base_branch: dev` in config, then from `dev` |
| AC114 | Given a unit created inside a git worktree, when `lyceum status`, `test` and `check` run there, then they behave exactly as in the primary checkout, and `status` in the primary checkout lists that unit with its worktree path (Deferred after v1.) |
| AC115 | Given a unit at a judgment gate in a worktree, when the owner runs `lyceum approve <unit>` from the primary checkout, then the event is written in that worktree on the unit's branch (Deferred after v1.) |
| AC14 | Given `init` in a directory that is not a git repo, when it runs, then it offers to run `git init`, and runs it only after the user confirms |
