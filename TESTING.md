---
lyceum: 1
suite: CGO_ENABLED=0 go test ./... -count=1
single: CGO_ENABLED=0 go test ./... -count=1 -run '{name}'
subsets:
  unit: CGO_ENABLED=0 go test ./internal/... -count=1
  ac: CGO_ENABLED=0 go test ./cmd/lyceum/... -count=1 -run TestScripts
  race: go test ./... -count=1 -race
  bench: CGO_ENABLED=0 go test ./... -count=1 -run '^$' -bench .
  shim: node --test npm/shim.test.js
test_files: ["**/*_test.go", "**/testdata/script/*.txtar"]
timeout: 20m
---

# TESTING.md: Lyceum

**Status: APPROVED by the owner (gate `testing`).** `lyceum test` runs exactly the frontmatter block above.

## 1. Principles

1. **Done means verifiable.** A task is done when the suite passes in a run whose real output is shown, and every
   AC the task claims is mapped to a passing test.
2. **The ACs are the contract.** Every AC in `lyceum/spec/` (release coverage leaves out rows withdrawn or deferred after v1, AC153) maps to at least one test whose name contains
   the AC ID. An AC without a test is an unmet AC.
3. **Real things, not mocks, for anything git or process related.** Every AC test runs the real built binary in a real
   temporary git repo with real child processes. Mocks are allowed only in unit tests of pure logic.
4. **A failing test stops the work.** It is reported with its output. It is never deleted, skipped, weakened, or
   rewritten to match the code. A test that contradicts the spec is a finding, and the owner decides which is wrong.

## 2. Levels

| Level | What | Where | Runs in |
|---|---|---|---|
| Unit | Pure logic: ID and slug generation, frontmatter parse/write, hashing, the code-path rule, gate-transition table, event encoding, freshness state computation | `internal/**/*_test.go` | `unit`, `suite` |
| AC scripts | One testscript per AC: builds nothing, drives the compiled `lyceum` binary inside a fresh temp git repo, asserts on stdout/stderr, exit codes, files and git state | `cmd/lyceum/testdata/script/acNNN_<slug>.txtar` | `ac`, `suite` |
| Benchmarks | NF2/AC3: `status` on a repo with 200 plans; hook entry point | `*_test.go` `Benchmark*` | `bench` (not in `suite`) |
| Platform | AC1, AC83–AC85, AC93: the release binary on each platform | release CI matrix | release only |
| Harness release checks | AC66, AC89–AC91: an end-to-end `feat` and `bug` in each of the five harnesses | `scripts/release-check*.sh`, results in `release-checks/<label>-YYYY-MM-DD.md` per release | release only, by hand |

Harness checks drive a real agent in a real harness, so they run outside the suite. They are run before each release,
and the release does not ship until all five pass.

## 3. AC naming and mapping

- Script files are named `ac<NNN>_<slug>.txtar`, e.g. `ac044_dirty_tree_refusal.txtar`, and start with a comment
  line `# AC44: <criterion summary>`. The literal `AC44` in that comment is what `check` maps.
- Unit tests that prove part of an AC name it: `TestCodePathRule_AC28`.
- One AC can have several scripts; one script proves exactly one AC.
- A script that proves no AC gets a plain descriptive name (e.g. `changed_files.txtar`) and no `# ACnn` line.
- `TestACCoverage` in `internal/acmap/coverage_test.go` parses `lyceum/spec/*.md` and fails if any AC has no script and no named test.
  It is the mechanical form of principle 2 and runs in `suite`.

## 4. The invariant: no gate passes silently (GO1)

Every bypass route in `lyceum/spec/enforcement.md` gets an attack script: it performs the bypass the way an agent
would, then asserts `lyceum check --ci` exits non-zero and names it. Minimum set:

| Attack | AC |
|---|---|
| Write code before the code gate is approved | AC47 |
| Edit the plan after approval | AC43 |
| Flip CLI-owned frontmatter by hand | AC17 |
| Run `approve` without a TTY | AC39 |
| Run `approve` with a harness env variable set | AC40 |
| Approve as a non-owner | AC45 |
| Edit TESTING.md after approval, then pass the suite | AC50, AC60 |
| Code on a branch with no unit | AC48 |
| Non-trivial chore with no plan | AC29 |
| Weaken the repro test after `repro` | AC105 |
| Clear a gate with a subset, a dirty-tree, or an old-commit run | AC57, AC59, AC113 |

**Guard tests.** Each protection also has a guard: the attack script runs once with the protection disabled through
a test-only build tag (`lyceumtest_noguard_<name>`) and must then *succeed*. If an attack script still fails with its
protection removed, it was failing for the wrong reason, and the guard test fails. This proves each script tests the
protection it names.

TTY tests use a real pseudo-terminal, never a fake `isatty` result. A script asserting what the terminal showed uses
the `ttyrun` command, which writes `$WORK/tty.out`: testscript's own `ttyout` can lose the end of the output,
because the pty is closed before it is drained.

**Portability.** The suite runs on Linux and macOS (NF5). Scripts use only POSIX tools and flags: no `sed -i`, no `\n` in
a `sed` replacement (the `linesub` command does that), no `sha256sum` (use `cksum`).

## 5. Determinism

- **Time** is injected: the binary reads `LYCEUM_NOW` (test builds only) instead of the clock. No sleeps in tests,
  except the timeout test (AC112), which uses a 1-second limit.
- **Randomness** (unit hashes) reads `LYCEUM_SEED` in test builds, so IDs are reproducible.
- Release builds ignore every `LYCEUM_*` test variable (AC116), so they can't be used to backdate events or fix IDs.
- **git identity** is set per temp repo (`user.name`, `user.email`); the global git config is ignored via
  `GIT_CONFIG_GLOBAL=/dev/null`.
- **Network:** AC tests run with no network access needed. AC2 (no network) is asserted with a network namespace
  on Linux.
- **Concurrency:** the `race` subset runs the suite under the race detector. Tests that exercise merges (AC10) run
  10 times in a loop; a single flake is a finding, not noise.

## 6. Coverage

- 100% of ACs mapped (enforced by `TestACCoverage`). This is the only coverage number that gates.
- An AC that only a live agent can prove (AC4, AC66, AC68, AC74, AC80, AC89, AC90, AC91) also counts as mapped when the latest
  `release-checks/<label>-YYYY-MM-DD.md` records it as passed on all five v1 harnesses.
- A packaging AC that only a release can prove (AC83, AC84, AC85, AC93) also counts as mapped when that latest record has
  `RESULT|<platform>|ACnn|PASS` for all four v1 platforms (linux-x64, linux-arm64, darwin-x64, darwin-arm64) and no
  FAIL for it; harness lines do not count for these. Nothing else counts either way.
- Every legal gate transition, and every illegal one refused, has a unit test (the transition table).
- Line coverage is reported, not targeted.

## 7. Running

| Purpose | Command |
|---|---|
| Full suite (the only gate evidence) | `CGO_ENABLED=0 go test ./... -count=1` |
| One test or AC | `CGO_ENABLED=0 go test ./... -count=1 -run 'TestScripts/ac044_'` |
| Unit only | `go test ./internal/... -count=1` |
| Race | `go test ./... -count=1 -race` |
| Benchmarks | `go test ./... -run '^$' -bench .` |
| npm shim (Node ≥ 18, not in the suite) | `node --test npm/shim.test.js` |

`-count=1` disables Go's test cache so every run actually runs.

## 8. Per-task loop

After every task: full suite, lint (`golangci-lint run`), then show the real output and name the ACs the task
proves. Only then is the task marked done.
