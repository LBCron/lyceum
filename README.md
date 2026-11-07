# Lyceum

Lyceum is a spec-driven development lifecycle for coding agents. It is one CLI (`lyceum`) plus a set of skills
that work in any agent harness that reads the Agent Skills standard.

## What Lyceum is

### The problem

A coding agent fills every gap in a request with its most plausible guess. It then reports success fluently,
whether or not the work happened. A plausible guess you never saw and a "tests pass" nobody ran look exactly like
real decisions and real results.

### What Lyceum does about it

Lyceum puts a contract in front of the agent and an evidence check behind it.

- **The contract.** Before code exists, the agent interviews you and drafts a spec and a plan. You approve them at
  gates the agent cannot pass. An approval is bound to the sha256 of every file it covers, so an edit after
  approval shows up.
- **The evidence.** A mechanical gate, such as `verify`, clears only on a test run the CLI performed and recorded
  itself. The agent's report of a result never clears a gate.
- **The check.** `lyceum check` recomputes approval hashes and gate rules. It runs from the pre-commit hook and in
  CI, so work that skipped a gate is caught before it merges.

### Philosophy

- **No gate passes silently.** Every gate clears by a human approval bound to what was approved, or by a
  CLI-recorded run. Any bypass shows up in `lyceum check`, and CI blocks the merge.
- **Done means verifiable.** "Done" is a recorded run, not a description.
- **All state lives in plain files in the repo.** Plans, specs, approvals and the event log are markdown, YAML and
  JSONL that diff and merge in git. Nothing lives in chat, so any supported harness works, and you can switch
  harness mid-unit without losing anything.
- **Small context.** The agent always loads `AGENTS.md`, `RULES.md` and `TESTING.md`, and loads a plan and its
  spec areas only when working on it.
- **Greenfield and brownfield.** A new project starts with an interview. An existing one grows its spec only where
  work happens.
- **Structure must not slow you down.** You see only the gates that need a human. Explanation, bookkeeping,
  drafting and routing are the agent's job, not extra commands for you.

### Who it is for

| Role | Who | What they do |
|---|---|---|
| Owner | A solo developer, or the lead of a small team | Approves gates. The only identity whose approvals count |
| Teammate | Anyone else with repo access | Runs any command. Their approvals are recorded but do not clear gates |
| Agent | The coding agent in a supported harness | Runs every command except `approve`. Interviews, drafts, builds |

## Install

With npm, run it once without installing:

```sh
npx lyceum init
```

or install it globally:

```sh
npm i -g lyceum
```

With Homebrew:

```sh
brew install oritromax/tap/lyceum
```

With Go:

```sh
go install github.com/oritromax/lyceum/cmd/lyceum@latest
```

Platforms: linux-x64, linux-arm64, darwin-x64, darwin-arm64. Windows is not supported. Binaries are also attached
to each GitHub release. The macOS binaries are not notarized, so after a browser download, run this once:

```sh
xattr -d com.apple.quarantine lyceum
```

## How to use it

This is a summary. The full specification starts at [`lyceum/SPEC.md`](lyceum/SPEC.md); the detail lives in:

| File | Covers |
|---|---|
| [`commands.md`](lyceum/spec/commands.md) | Every command and its syntax, `status`, `log`, `doctor` |
| [`units.md`](lyceum/spec/units.md) | Unit types, the plan file, gate lists, `close`, `amend` |
| [`records.md`](lyceum/spec/records.md) | `understand`, `trace`, `audit`, citations, freshness, `export` |
| [`enforcement.md`](lyceum/spec/enforcement.md) | `approve`, `check`, CI, harness hooks, signed approvals |
| [`test-runner.md`](lyceum/spec/test-runner.md) | `lyceum test`, the TESTING.md format, run records |
| [`distribution.md`](lyceum/spec/distribution.md) | `init`, harness support, packaging, `eject` |

Run bare `lyceum` to see the commands, and `lyceum <command> --help` for each one. Every command ends with a
`Next:` line naming the next action.

### Start a project

```sh
lyceum init
```

`init` writes `AGENTS.md`, `RULES.md`, `lyceum/` (spec index, config, event log) and `.gitignore` entries. It wires
the harnesses it detects, or the ones you name with `--harness` (`claude`, `codex`, `commandcode`, `opencode`,
`pi`), and offers the pre-commit hook and the CI workflow. It never overwrites an existing `AGENTS.md` or
`CLAUDE.md`; it appends a marked Lyceum section after asking. Commit what it wrote, then:

- **Greenfield** (no code yet): your agent's `lyceum-init` skill runs the project interview and drafts the first
  spec areas. You approve them with `lyceum approve project`.
- **Brownfield** (existing code): `init` offers an `understand` run as a baseline. The spec then grows one area at
  a time, as work touches it.

`TESTING.md` says how the project is tested. The first `lyceum test` has the agent draft it from the frameworks it
detects, and you approve it with `lyceum approve testing`. Mechanical gates rely on that approval.

### Understand existing code

| Command | What it does |
|---|---|
| `lyceum understand [hint]` | Explain the project or part of it |
| `lyceum trace [question]` | Follow one flow end to end, by reading the code or with a full run against a TESTING.md environment |
| `lyceum audit` | Run each ecosystem's audit tool and report vulnerable or outdated dependencies |
| `lyceum export <record> [path]` | Publish a record to `docs/` |

Inspect commands never edit code and are not units: no branch, plan or gates. The agent saves each answer as a
record under `lyceum/understand/`, `lyceum/trace/` or `lyceum/audit/`. A record cites the files it describes by
git hash, so `status` can tell you when one has drifted from the code. Findings are routed, not fixed: a
contradiction becomes an offer to start a bug or a feature.

### Change code: units

A unit is one piece of change work. It is one plan file (`lyceum/plan/<id>.md`) on one branch. Each type has a
fixed gate list. `J` gates need your approval; `M` gates clear on a run the CLI recorded.

| Start with | Gates in order | You approve |
|---|---|---|
| `lyceum add feat "<title>"` | `spec` J → `build` → `verify` M → `close` | `spec` (spec delta, ACs and plan, in one stop) |
| `lyceum add bug "<title>"` | `repro` M → `diagnose` → `fix` → `verify` M → `cause` J → `close` | `cause` (root cause and fix diff) |
| `lyceum refactor "<title>"` | `intent` J → `baseline` M → `plan` J → `build` → `verify` M → `close` | `intent`, `plan`, and any characterization tests or test-file changes |
| `lyceum chore "<title>"` | trivial: `build` → `close`; planned: `plan` J → `build` → `verify` M → `close` | nothing for a trivial chore; `plan` for a planned one |
| `lyceum add doc "<title>"` | `draft` → `verify` M → `close` | nothing |
| `lyceum add test "<title>"` | `write` → `run` M → `close` | nothing |

A bug starts test-first: `repro` needs a new test that fails before any fix exists. A trivial chore may not touch
code, a manifest, a lockfile or CI config; `lyceum plan` upgrades it to a planned chore. A test unit whose new test
fails is converted to a bug at `close`, with that test as the repro. The exact gate rules are in
[`units.md`](lyceum/spec/units.md).

### The day-to-day loop

```sh
lyceum status
```

`status` lists every open unit on this branch and others, its gate, the pending approvals, and the next action.
Your agent runs it at the start of each session too. Then:

1. **Start a unit**, for example `lyceum add feat "CSV export"`. Lyceum creates the plan file and the branch.
2. **Work with your agent.** It interviews you, drafts the spec delta and plan, and stops at the gate.
3. **Approve** in a real terminal the agent doesn't control: `lyceum approve`. Bare `approve` walks every pending
   gate. It shows what you are approving with its hashes and diff, and asks you to type the gate name.
4. **The agent builds.** Until the gate that opens code clears, the harness hook blocks the agent's code writes.
5. **Test** with `lyceum test`. It runs the suite from TESTING.md and records the run. A passing full run clears
   `verify`.
6. **Close** with `lyceum close`. It checks the last gate cleared, closes the unit, and opens a PR with `gh` or
   `glab` if one is set up; otherwise it prints the push and merge commands. You merge.

If the plan needs to change mid-build, `lyceum amend "<reason>"` reopens it: the approval goes back to pending and
`check` fails new code until you approve again. `lyceum plan` shows the current plan. `lyceum log` shows the
project history, and `lyceum doctor` checks the setup.

### Who does what

You decide the unknowns and approve. The agent does the rest: it interviews, drafts the spec and plan, writes the
code and tests, runs `lyceum test`, and keeps the plan's progress current. It does this through the skills `init`
installs in `.agents/skills/lyceum-*/`:

| Skill | Flow |
|---|---|
| `lyceum-status` | Orientation: run `status`, follow the next step |
| `lyceum-init` | The greenfield project interview |
| `lyceum-feat`, `lyceum-bug`, `lyceum-refactor`, `lyceum-chore`, `lyceum-doc`, `lyceum-test` | One per unit type |
| `lyceum-understand`, `lyceum-trace`, `lyceum-audit` | The inspect commands |

The agent cannot approve. `approve` refuses inside a known harness and without a terminal on stdin and stdout, and
the harness hook blocks any shell command that runs it. One opt-in exception: with `approvals.harness_prompt: allow`
in `lyceum/config.yaml`, Claude Code, OpenCode and Pi can run it when you ask, and the harness asks you to confirm
in its own prompt. The approval event records that route.

Tested end to end with **Claude Code, Codex, OpenCode, Pi and Command Code**. `init` installs the skills, slash
commands where the harness has them, the instructions pointer and a pre-tool hook for each. Codex and Pi load
project hooks only after you trust the project. Any other harness that reads the Agent Skills standard gets the
skills, with no guarantee.

## How it works with CI

Two layers run `lyceum check`:

- **The pre-commit hook** runs it on every commit as a warning. It prints the findings and never blocks the commit;
  a commit past a failing finding is recorded as a `bypass` event in the log.
- **CI** runs `lyceum check --ci`, which fails on any failing finding. It runs in any CI.

Local checks catch a sloppy or over-eager agent. CI plus branch protection catches the rest.

### The workflow `init` writes

When you accept the CI workflow, `init` writes `.github/workflows/lyceum.yml`, which runs the check on every pull
request:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0
- uses: oritromax/lyceum/check-action@v1
```

The Action downloads the release archive for the runner (Linux or macOS, X64 or ARM64) and `checksums.txt` from
this repo's GitHub release, refuses an archive whose sha256 does not match, and runs `lyceum check --ci` in the
workspace. The job fails exactly when the check does. Other runners fail with a message naming the platform.

On a pull request GitHub checks out a merge commit with a detached HEAD. `check --ci` takes the pull request's
branch from `GITHUB_HEAD_REF` and checks that branch's unit, so a closed unit there passes.

Require the check in branch protection. Without that, CI only reports.

### Which release runs

- `@v1` runs the release the `v1` tag points at. The release workflow moves `v1` only to the newest stable
  `v1.x.y` after its archives are verified, so `@v1` is the latest stable v1.
- `@v1.2.3` runs exactly that release.
- Any other ref (a branch, a commit) runs the release of the highest major tag.
- The `version` input (`latest`, `v1` or `v1.2.3`) overrides the ref.

## What it isn't

- **Not an agent or a harness.** Lyceum is a CLI and a set of skills. Your harness runs the agent.
- **Not a code generator.** The CLI never writes or reads your code for meaning. The agent does that.
- **Not a large-organisation workflow.** There are no roles, per-gate approvers or per-unit custom gate lists. One
  owner approves.
- **Not a replacement for code review.** Gates check that the spec, plan and evidence exist and were approved. They
  don't judge whether the code is good.
- **Not a security boundary on your machine.** Nothing that runs locally as the same OS user is absolute. CI with
  branch protection is the wall; signed approvals (opt-in) make an approval effectively unforgeable.
- **No telemetry.** The CLI sends no network traffic except through `git`, `gh`/`glab`, and the commands your
  TESTING.md defines.
- **Not on Windows.** Linux and macOS only.
- **Not notarized on macOS.** See the `xattr` note under Install.

## License

MIT
