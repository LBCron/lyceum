# Test runner and evidence

`lyceum test` runs the project's tests, records every run, and is the only source of evidence for mechanical gates.
`lyceum add test` (one new test) is a unit, defined in `units.md`.

## TESTING.md

Lives at the root and is always loaded. It is approval-gated: the agent drafts it, the owner approves
it with `approve testing`, and any later edit needs re-approval before mechanical gates can rely on it.

The frontmatter is the only part the CLI executes. The body is guidance for the agent and humans.

```yaml
---
lyceum: 1                      # schema version
suite: go test -count=1 ./...  # full suite: the only command that can clear a gate
single: go test -count=1 ./... -run {name}   # one named test; used by --repro
test_files: ["**/*_test.go"]       # globs for AC mapping
subsets:                       # names are whatever the owner defines
  unit: go test -count=1 ./... -short
  integration: go test ./... -run Integration
  changed: go test $(lyceum changed-files)       # helper
  last-failed: …
environments:                  # used by trace full-run
  test:
    url: http://localhost:8080
    up: docker compose -f compose.test.yml up -d
    seed: ./scripts/seed-test.sh
    reset: ./scripts/reset-test.sh
    down: docker compose -f compose.test.yml down
timeout: 15m                   # a run over this is killed and recorded as failed
---
```

When TESTING.md is missing, `lyceum test` prints this frontmatter schema (keys, one example line each, `{name}` in
`single:`) with its refusal, so an agent in a project that uses Lyceum has the format without reading the binary.

Ecosystem defaults ship as suggestions the agent offers while drafting, never as built-in CLI behaviour.
v1 ships suggestions for Go, JS/TS, Python, Rust, PHP and Ruby. Other languages work; the agent has no starter to offer.

`changed` cannot be computed generically, so it exists only if the owner defines it. A helper that prints
changed files on the branch (`lyceum changed-files`) is available for use in those commands.

## `lyceum test [subset]`

| Situation | Behaviour |
|---|---|
| No TESTING.md | Prints that this is the first run; the agent drafts TESTING.md from the detected frameworks without interviewing, and the owner approves one plain summary (`approve testing`). Questions come only if detection fails |
| TESTING.md exists, approved | Runs `suite` (bare) or the named subset; records the run |
| TESTING.md changed since approval | Runs and records, but the run cannot clear a gate |
| No tests exist in the project | Prompts to create them. On existing code these are characterization tests that pin current behaviour, right or wrong |
| The suite fails | Reports the failures, and the agent offers `add bug` for each new failure. A failing test is never edited |
| Unknown subset name | Refuses and lists the defined ones |

## Run records

Every run writes `.lyceum/runs/<ts>-<rand>.json`:

```json
{"ts":"…","command":"go test ./...","subset":null,"commit":"1a2b3c4","dirty":false,
 "exit":0,"duration_ms":81234,"output":"<ts>-<rand>.out","output_sha256":"…","testing_sha256":"…"}
```

The CLI spawns the command itself and captures stdout and stderr into the output file. The hash is of the
stored file, and hashes are never compared across runs because test output is not deterministic (from review).

A run is **gate evidence** only if: it ran `suite` (not a subset), the working tree was clean, TESTING.md matched
its approved hash, and the commit is the branch HEAD at clearing time. On a dirty tree the CLI
says to commit first, then run `lyceum test`.
The clearing run's record is inlined into the `gate-clear` event.

## AC mapping

An AC is mapped when its ID (`AC14`) appears as a whole word in a test name or a comment in a file that matches the
test-file patterns in TESTING.md frontmatter `test_files:` (a list of globs, filled in by the agent while drafting). `check` finds mappings by grep. The convention is documented per ecosystem.

A plan may list an AC in `acs_partial:` when it proves only part of it. Such an AC does not count as proven by that
unit: `verify` does not require it, and coverage and the unclaimed-AC warning keep it open until a closed plan lists
it in `acs:`. An AC row or bullet ending `(Deferred after v1, …)` is not required by release coverage.

## Mechanical gates

| Gate | Clears when |
|---|---|
| `verify` (feat) | Gate-evidence run exits 0 and every AC the unit added is mapped |
| `repro` (bug) | Test-first: `lyceum test --repro <test>` with a clean tree and a branch diff touching only test files; the CLI runs `single:` for that test and it fails, and the output mentions it. The test is fingerprinted and frozen until `verify` |
| `verify` (bug) | Gate-evidence run exits 0, which includes the repro test |
| `baseline` (refactor) | Gate-evidence run on the start commit exits 0, and the CLI fingerprints the test files |
| `verify` (refactor) | Gate-evidence run exits 0, test files match the baseline fingerprint or each diff is owner-approved, and the benchmark meets the metric if one was set |
| `run` (test) | The new test runs in a gate-evidence run and passes |

## Gate mechanics

- **Clearing:** `lyceum test` clears the unit's current mechanical gate itself when the run is gate evidence and the
  gate's condition holds, writing one `gate-clear` event with the run inlined. There is no separate clear command.
- **Work steps** (`build`, `diagnose`, `fix`, `draft`, `write`) end when the next mechanical gate clears in the same
  `lyceum test`; a passing gate-evidence run is the proof the work is done.
- **Repro fingerprint:** the content of each test file in the branch diff when `repro` clears. Later, a file still
  matches when every line it had at `repro` is still there, in order: new tests may be added to the same file, but
  changing or removing a line of the repro-time content needs owner approval (AC105).
- **Refactor baseline** runs at HEAD; the branch diff may touch only test files (characterization tests), the same
  rule as `repro`. The fingerprint is the sha256 of every file matching `test_files` at that commit.
- **Test-change approval** is `lyceum approve tests`, for two cases: a refactor at `verify` (test files that
  differ from the baseline fingerprint) and a bug after `repro` clears and before `cause` (changed frozen repro files).
  It hashes each changed test file; `verify` accepts a file if it matches its fingerprint or its latest approved hash.
  All approve rules apply (harness env, TTY, owner, typed confirmation).
  For refactor, a test file added after the baseline fingerprint counts as a change and needs approval too.
  Characterization tests added before `baseline` are approved the same way: with test files in the branch
  diff, `baseline` clears only after a counting `approve tests` covers them.
- **`approve cause`** refuses a dirty tree, so every approved fix file is committed and was in the suite run.
  `approve cause --no-repro` is confirmed by typing `cause`; it hashes nothing.
- A code commit on a bug after `verify` clears sends it back to `fix`: `verify` must clear again before
  `approve cause`. `approve tests` is also available for a refactor at `build`.
- `check` allows characterization tests while a refactor is at `plan`.
- **Refactor benchmark:** `intent` records `benchmark: <command>` in the plan; `verify` runs it and requires
  exit 0. The command decides pass or fail; Lyceum never parses numbers.
- **`approve cause`** and **`--no-repro`** are part of v1. The `cause` fix diff is every non-test file in the branch diff.
- **No tests (AC63)** means no file matches `test_files`: nothing runs and no passing run is recorded. One exception: a
  doc unit at `verify` whose branch diff has no code clears `verify` with `no_tests: true` on the `gate-clear` event.
- **Failure output (AC61):** the CLI prints the last 50 lines and the output file path; no per-framework parsing in v1.
- **Execution:** commands run via `sh -c`, output streamed live and captured; on timeout the whole process group is
  killed. Windows stays out of scope.
- `lyceum changed-files` is part of v1; `environments:` is read by `trace`.
- Unknown TESTING.md frontmatter keys produce a warning naming the key, never a refusal.
- A TESTING.md change while a mechanical gate relies on it is reported once, by the TESTING.md drift check; the
  approval-integrity check skips the `testing` approval in that case.

## Acceptance criteria

| # | Criterion |
|---|---|
| AC55 | Given no TESTING.md, when `lyceum test` runs, then it runs no tests and prints that TESTING.md must be drafted and approved |
| AC56 | Given an approved TESTING.md with `suite: go test ./...`, when `lyceum test` runs, then the CLI spawns exactly that command and writes a run record with exit code, commit, duration and output hash |
| AC57 | Given a subset `unit` defined in TESTING.md, when `lyceum test unit` runs, then the recorded run has `subset: unit` and it never clears a gate |
| AC58 | Given `lyceum test nope`, then it exits non-zero and lists the defined subsets |
| AC59 | Given a run with uncommitted changes, then the record has `dirty: true` and it cannot clear a gate |
| AC60 | Given TESTING.md edited after approval, when the suite passes, then no gate clears and the output names `approve testing` |
| AC61 | Given a failing suite, when `lyceum test` completes, then no test file was modified by the CLI and the output lists the failing tests |
| AC62 | Given a test named `TestExport_AC14`, when `check` maps ACs, then AC14 is mapped; given `AC140` only, then AC14 is not mapped |
| AC63 | Given a repo with no tests, when `lyceum test` runs, then it reports zero tests found and does not record a passing run |
| AC64 | Given 30 runs, then 30 run records exist under `.lyceum/runs/` and none is tracked by git |
| AC65 | Given a gate cleared by a run, then the `gate-clear` event contains that run's full record |
| AC103 | Given a bug branch whose diff against base touches `src/login.go`, when `lyceum test --repro TestLoginPlus` runs, then it refuses and names `src/login.go` |
| AC104 | Given a bug branch whose diff touches only `login_test.go`, when `--repro TestLoginPlus` runs and the test fails, then `repro` clears; when it passes, then `repro` does not clear |
| AC105 | Given `repro` cleared, when `login_test.go` changes afterwards without owner approval, then `verify` does not clear |
| AC112 | Given TESTING.md `timeout: 1s` and a test that sleeps 5s, when `lyceum test` runs, then the run is killed, recorded with a non-zero result, and cannot clear a gate |
| AC113 | Given a passing full-suite run recorded at commit A, when HEAD moves to commit B before the gate clears, then that run cannot clear the gate |
| AC116 | Given a release build (no test build tag), when `LYCEUM_NOW` or `LYCEUM_SEED` is set, then the binary ignores them: event timestamps come from the real clock and unit hashes are random |
| AC66 | Given each of the five tested harnesses, when its agent runs `lyceum approve`, then it is refused; the harness env-variable list in `enforcement.md` is verified against each harness and recorded in the docs |
| AC150 | Given a plan with `acs_partial: [AC9]` and a test naming AC9, when the unit closes, then release coverage and the unclaimed-AC warning still report AC9 until a closed plan lists it in `acs:` |
| AC151 | Given no TESTING.md, when `lyceum test` runs, then the output includes every frontmatter key with an example line, `single:` showing `{name}` |
| AC152 | Given `repro` cleared on `TestLoginPlus` in `login_test.go`, when a new test function is added to that file and no repro-time line changes, then `verify` is not blocked by the freeze; when a repro-time line changes, it is |
| AC153 | Given an AC row ending `(Deferred after v1, …)`, when release coverage runs, then that AC is not reported as unmapped |
| AC179 | Given a passing suite whose output has a Go `ok <pkg> (cached)` line, when `lyceum test` runs, then it warns in one line naming `-count=1` and the run still clears the gate; with no such line, no warning |
| AC180 | Given no TESTING.md, when `lyceum test` prints the schema, then its Go suggestion is `go test -count=1 ./...` |
