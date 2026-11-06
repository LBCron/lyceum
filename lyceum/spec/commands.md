# Command reference

`lyceum` is the root command. Bare `lyceum` prints the commands grouped as below.

## Lifecycle

| Command | Purpose | Defined in |
|---|---|---|
| `lyceum init` | Set up Lyceum in a repo; greenfield interview or brownfield baseline | distribution.md |
| `lyceum status [--json]` | Every unit on this branch and others, its gate and the next action; pending approvals; stale records | below |
| `lyceum check [--ci]` | Integrity and gate checks; warns locally, fails in CI | enforcement.md |
| `lyceum approve [gate]` | Owner-only approval from a real terminal; bare form walks every pending gate | enforcement.md |
| `lyceum plan [--new-area <name>]` | Show the plan, add an area, or upgrade a trivial chore | units.md |
| `lyceum amend [unit] [--intent] "<reason>"` | Reopen an approved plan mid-build | units.md |
| `lyceum close [unit]` | Fold the spec delta, close the unit, open a PR | units.md |
| `lyceum test [subset]` | Run the suite or a subset and record the run | test-runner.md |
| `lyceum log` | Project history | below |
| `lyceum eject` | Remove Lyceum's installed adapters, keep the records | distribution.md |
| `lyceum doctor [--json]` | Check the setup: the tool, and the repo's Lyceum integration | below |

## Change (units)

| Command | Purpose | Defined in |
|---|---|---|
| `lyceum add [feat\|bug\|doc\|test] [title]` | Start a unit of work: feat, bug, doc or test | units.md |
| `lyceum refactor [title]` | Change structure without changing behaviour | units.md |
| `lyceum chore [title]` | Maintenance: dependencies, config, README | units.md |

## Inspect (never edit code)

| Command | Purpose | Defined in |
|---|---|---|
| `lyceum understand [hint]` | Explain the project or part of it | records.md |
| `lyceum trace [question]` | Follow one flow end to end | records.md |
| `lyceum audit` | Report vulnerable or outdated dependencies | records.md |
| `lyceum export <record> [path]` | Publish a record to `docs/` | records.md |

## Internal (used by skills, hooks and CI, not listed in the menu)

| Command | Purpose |
|---|---|
| `lyceum gate <unit> <gate>` | Move a unit to a gate the CLI allows; refuses illegal transitions |
| `lyceum hook pre-tool --harness <name>` | Harness pre-tool hook entry point; reads the harness payload on stdin |
| `lyceum changed-files` | Files changed on this branch, for TESTING.md subset commands |
| `lyceum record <file>` | Fill in a record's id, date and cite hashes, validate it, and commit it |

## Menu, help and version

- Bare `lyceum` and `--help` print text only; cobra's `help` entry is not listed in the menu.
- `lyceum --version --json` prints `{"version":"<v>"}`.
- An unknown command or a bad flag is a refusal (exit 2) with a next step, e.g. `run "lyceum" to see the commands`.

## `lyceum status`

Reads every plan in the checkout and, with `git show`, open plans on other local and remote-tracking branches
(review). Shows, per unit: ID, type, branch, status, gate, last cleared gate, and the next action.
Also shows: pending approvals, an unapproved or changed TESTING.md, and the count of drifted or broken records.
Closed units are hidden unless `--all` is given. "Last cleared gate" is the gate before the current one in the
unit type's gate list (`close` once the unit is closed).

`status` also shows the repo-level gates, `project` and `testing`, each as approved (with the approval commit),
pending, or changed since approval. Its `Next:` line names the step that actually moves work on:

| State | `Next:` |
|---|---|
| Greenfield, `project` never approved | finish the project interview, then `lyceum approve project` |
| A repo-level approval is pending and the unit's next mechanical gate relies on it | `lyceum approve testing` (or `project`) |
| Unit at `close` | `lyceum close` |
| Bug at `diagnose` or `fix` | write the root cause, commit the fix, then `lyceum test`: `verify` clears on a passing run |
| Nothing in flight, nothing pending | the top `lyceum/LIBRARY.md` entry, else `lyceum add feat "<title>"` |

## `lyceum doctor`

Checks setup, never work. Outside a git repo it checks the tool only. In a git repo without Lyceum it checks the tool
and says the repo is not set up, with `lyceum init` as the next step (an interactive run offers to start it). In a
Lyceum repo it also checks the integration: installed skills match the binary, the wired harness hooks and the
pre-commit hook are present, `lyceum/config.yaml` and TESTING.md parse, and a CI workflow references an Action this
build can resolve, and `.gitignore` lists what harnesses write on their own. It reports and changes nothing there: `lyceum init` adds them. Every finding prints a `Next:`; `--json` prints one object with `state` and `findings`. No
short-name symlink in v1.

## `lyceum log`

Merges `lyceum/events.jsonl` with `git log` into one chronological view. Filters: `--unit <id>`, `--since <date>`,
`--type <event type>`. Commits whose message starts with `lyceum:` are hidden, since each is shown as its event.
Harness transcripts are not a source in v1.

## Acceptance criteria

| # | Criterion |
|---|---|
| AC95 | Given bare `lyceum`, then it prints the commands in the Lifecycle, Change and Inspect groups and exits 0 |
| AC96 | Given two open units, one on the current branch and one on another branch, when `status` runs, then both are listed with their gates and branches |
| AC97 | Given `status --json`, then the output is one valid JSON object with a `units` array |
| AC98 | Given events and commits on the same day, when `log` runs, then they are interleaved by timestamp |
| AC117 | Given `lyceum nope`, then it exits 2, says the command is unknown, and prints a `Next:` step; given `lyceum --version --json`, then stdout is exactly `{"version":…}` |
| AC99 | Given `log --unit a3f9c2-csv-export`, then only that unit's events and the commits on its branch are shown |
| AC157 | Given `approve project` and `approve testing` recorded and TESTING.md edited since, when `status` runs, then it shows `project` approved with its commit and `testing` changed since approval |
| AC158 | Given a greenfield `init` committed and no `project` approval, when `status` runs, then `Next:` names the project interview and `lyceum approve project`, not `lyceum add` |
| AC159 | Given a feat at `build` with TESTING.md unapproved, when `status` runs, then `Next:` names `lyceum approve testing`; given a unit at `close`, then `Next:` is `lyceum close` |
| AC160 | Given a bug at `diagnose`, when `status` runs, then `Next:` names `lyceum test` as the step that clears `verify` |
| AC161 | Given `doctor --json` outside a git repo, in a git repo without Lyceum, and in a Lyceum repo, then `state` is `no-repo`, `not-set-up` and `set-up` respectively, and the second has a finding whose next step is `lyceum init` |
| AC162 | Given a Lyceum repo whose installed `lyceum-feat` skill differs from the binary's copy, when `doctor` runs, then a finding names the skill with `lyceum init` as the next step |
| AC177 | Given a Lyceum repo whose `.gitignore` lacks a harness state path, when `doctor` runs, then a finding names the path with `lyceum init` as the next step, `.gitignore` is unchanged, and after `init` the finding is gone |
