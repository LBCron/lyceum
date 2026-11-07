# Lyceum

Lyceum is an open-source tool for spec-driven development with a coding agent. You stay in control through
approvals, and it stays out of your way when you need speed: you only see the gates that need a human.

1. You describe what you want. The agent works out where it fits in your spec, or whether it's new.
2. It asks about the things that matter: numbered questions, lettered options, its recommendation first, or
   your own answer.
3. It proposes the spec change and plan. You approve it or send it back.
4. It builds, then works through gates. A judgment gate waits for your approval. A mechanical gate clears only
   on a test run the CLI executed and recorded itself.
5. Each acceptance criterion maps to a test, each approval is hashed against what you approved, and
   `lyceum check` enforces both in the commit hook and in CI.

The agent can say it's done. Only the CLI can record that it is.

## Works with

| Agent | Tested version | Status |
|---|---|---|
| Claude Code | 2.1.318 | ✅ |
| Codex | 0.164.0 | ✅ |
| OpenCode | 2.0.18 | ✅ |
| Pi | 0.86.0 | ✅ |
| Command Code | 1.88.3 | ✅ |

Each was tested end to end in the v1 release check. `lyceum init` finds the agents you use and installs
the skills, slash commands and a hook for each.

> [!NOTE]
> Any other agent that reads `AGENTS.md` and `.agents/skills/` can follow the workflow, but without a hook
> nothing stops it before a gate is approved; the check still catches it at commit and in CI.

## Install

Run it once without installing:

```sh
npx lyceum init
```

Or install it globally with npm:

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

Supported platforms are linux-x64, linux-arm64, darwin-x64 and darwin-arm64.

> [!WARNING]
> Windows isn't supported.

Each GitHub release also has the binaries attached. If you download one on macOS through a browser, run
`xattr -d com.apple.quarantine lyceum` once before using it.

## Start

Run `init` at the root of your repo, new or existing:

```sh
lyceum init
```

If the folder isn't a git repo yet, `init` offers to run `git init` first.

### What `init` does

1. **Detects** whether the repo already has code, and which agents you use. An agent counts as detected when
   its binary is on your `PATH` or its project folder exists (`.claude/`, `.codex/`, `.opencode/`, `.pi/`,
   `.commandcode/`).
2. **Writes** the Lyceum files:

   | File | What it holds |
   |---|---|
   | `AGENTS.md` | The rules your agent reads first. |
   | `RULES.md` | Project rules. |
   | `lyceum/SPEC.md` | The spec index: one row per area. |
   | `lyceum/config.yaml` | Owner, base branch, approval settings. |
   | `lyceum/events.jsonl` | The event log for approvals and runs. Empty at first. |
   | `.gitattributes`, `.gitignore` | Merge rules for the event log, and Lyceum's local state. |

   An existing `AGENTS.md` or `CLAUDE.md` is never overwritten. `init` shows you its section, asks once, and
   appends it between `<!-- lyceum:start -->` and `<!-- lyceum:end -->` markers.
3. **Wires each agent you pick:** skills, slash commands and a hook that blocks code edits until the right
   gate is approved. Use `--harness <name>` to wire one without being asked.
4. **Offers** a pre-commit hook and a CI workflow. Both default to yes.

> [!IMPORTANT]
> Codex and Pi run project hooks only after you trust the project in that agent. Until you do, the hook
> silently doesn't run. `init` prints the step for each.

`TESTING.md` isn't written by `init`. The first `lyceum test` drafts it from the test frameworks it
detects, and you approve it with `lyceum approve testing`.

Re-running `init` updates the skills and hooks to your installed version. It never touches your `lyceum/`
content.

### New project or existing code

| | New project (no code) | Existing code |
|---|---|---|
| Next step | The agent interviews you about the project | The agent offers `lyceum understand` to explain the code you have |
| The interview | It opens with one open question (what are you building, and for whom?), then numbered questions with options, at most two rounds | None |
| What it writes | Spec areas under `lyceum/spec/`, each with acceptance criteria | A baseline record under `lyceum/understand/`, if you accept |
| Your approval | `lyceum approve project`, covering the whole initial spec | No project-wide approval |
| How the spec grows | Units add acceptance criteria to existing areas, or register a new area with `lyceum plan --new-area` | One area at a time, as units touch them. Each new area starts with an as-built section written from the code, which you approve with the unit's plan. Untouched areas stay unspecified. |
| First unit | `lyceum add feat "<title>"` | Whatever you want to do next: a feature, a bug fix, a refactor |

## How to use it

Lyceum's documentation is its own spec. Start with the index, then go to the file you need:

| Topic | File |
|---|---|
| Index of every area | [`lyceum/SPEC.md`](lyceum/SPEC.md) |
| Goals, scope, who it's for | [`overview.md`](lyceum/spec/overview.md) |
| Every command and its `Next:` line | [`commands.md`](lyceum/spec/commands.md) |
| Unit types and their gates | [`units.md`](lyceum/spec/units.md) |
| `understand`, `trace`, `audit` records | [`records.md`](lyceum/spec/records.md) |
| Approvals, hooks, `check` | [`enforcement.md`](lyceum/spec/enforcement.md) |
| `TESTING.md` and recorded runs | [`test-runner.md`](lyceum/spec/test-runner.md) |
| Install, `init`, agent wiring | [`distribution.md`](lyceum/spec/distribution.md) |
| Files, config, event log | [`architecture.md`](lyceum/spec/architecture.md) |

Each piece of work is a **unit**: one plan file under `lyceum/plan/` and one branch. You can drive a unit
from inside your agent or from your terminal:

- **Inside the agent**, you start a skill and describe what you want. The agent runs the `lyceum` commands
  and asks you questions. When a gate needs your approval, it stops and tells you the exact
  `lyceum approve` command to run.
- **In your terminal**, you run each `lyceum` command yourself. The agent still does the writing between
  steps.

`lyceum approve` only runs in your own terminal. Inside an agent's shell it refuses, so the agent can
tell you which approval is due and the exact command to type, but it can't approve anything on its own.

The examples use Claude Code's skill syntax. In other agents:

| Agent | Start a skill |
|---|---|
| Claude Code | `/lyceum-feat` |
| Codex | `$lyceum-feat` |
| Pi | `/skill:lyceum-feat` |
| OpenCode, Command Code | Ask for the skill by name |

### Add a feature

Gates: `spec` (you approve) → `build` → `verify` (recorded run) → `close`.

| Step | Inside the agent | In your terminal |
|---|---|---|
| Start | You: `/lyceum-feat Export orders as CSV` | `lyceum add feat "Export orders as CSV"` |
| Interview, spec, plan | The agent asks its questions, then writes the spec change, acceptance criteria and plan | The same: your agent does the writing |
| Approve the spec | The agent stops and asks you to run `lyceum approve spec` in your terminal | `lyceum approve spec` |
| Build | The agent writes the code and tests | The same |
| Verify | The agent runs `lyceum test` | `lyceum test` |
| Close | The agent runs `lyceum close` | `lyceum close` |

`close` folds the spec change into the area files and opens a PR, or prints the push and merge commands.

### Fix a bug

Gates: `repro` (a recorded failing test) → `diagnose` → `fix` → `verify` (recorded run) → `cause`
(you approve) → `close`. The test comes first, and you approve after the fix, not before.

| Step | Inside the agent | In your terminal |
|---|---|---|
| Start | You: `/lyceum-bug Order totals ignore the discount on the last item` | `lyceum add bug "Order totals ignore the discount on the last item"` |
| Reproduce | The agent writes a test that fails because of the bug, commits it, and runs `lyceum test --repro <test>` | `lyceum test --repro TestOrderTotalDiscount` (clears only if the test fails) |
| Diagnose, fix | The agent writes the root cause in the plan, then fixes the bug | The same: your agent does the writing |
| Verify | The agent runs `lyceum test` | `lyceum test` |
| Approve the cause | The agent stops and asks you to run `lyceum approve cause`; the approval covers the root cause and the fix diff together | `lyceum approve cause` |
| Close | The agent runs `lyceum close` | `lyceum close` |

If no automated test can reproduce the bug, the agent explains why and stops. Only you can skip `repro`:
`lyceum approve cause --no-repro "<reason>"`. `verify` and `cause` are still required.

### Add a test

Gates: `write` → `run` (recorded run) → `close`. Nothing needs your approval.

| Step | Inside the agent | In your terminal |
|---|---|---|
| Start | You: `/lyceum-test Export rejects a date range longer than a year` | `lyceum add test "Export rejects a date range longer than a year"` |
| Write | The agent writes the one test and commits it | The same |
| Run | The agent runs `lyceum test` | `lyceum test` |
| Close | The agent runs `lyceum close` | `lyceum close` |

If the new test fails on the current code, the unit stops and the agent offers to turn it into a bug.
`lyceum close` then starts a bug at `diagnose`, with the test as its repro.

### Other unit types

| Type | Inside the agent | In your terminal | What it does |
|---|---|---|---|
| Refactor | `/lyceum-refactor <title>` | `lyceum refactor "<title>"` | Changes structure, not behaviour. You approve the intent and the plan, the suite must pass before and after, and any change to a test file needs your approval. |
| Chore | `/lyceum-chore <title>` | `lyceum chore "<title>"` | Maintenance work. A trivial chore has no gates, but `check` fails one that touches code, a manifest, a lockfile or CI config. `lyceum plan` upgrades it to a planned chore that you approve. |
| Doc | `/lyceum-doc <title>` | `lyceum add doc "<title>"` | Documentation only. No approval; the branch may change only non-code files. |

> [!NOTE]
> In Claude Code, OpenCode and Pi you can tell the agent to run `lyceum approve` for you. The agent then
> asks you to confirm in its own approval prompt before the approval goes through. This is on by default.
> Set `approvals.harness_prompt: deny` in `lyceum/config.yaml` to turn it off.

## Working with CI

| Layer | What it does |
|---|---|
| Pre-commit hook | Runs `lyceum check`, prints findings, and never blocks a commit. A commit past a failing finding is recorded as a `bypass` event, with a few exceptions such as approval drift and warning-only findings. |
| CI | Runs `lyceum check --ci` and fails the job on any failing finding. Warning-only findings, such as stale records, don't fail it. |
| Branch protection | Require the CI check so a failing PR can't merge. Without it, CI only reports. |

`check` covers approval hashes, gate rules, plan fields against the event log, the spec registry, and stale
test evidence. A passing suite doesn't stand in for a missing approval.

### GitHub Actions

If you accept CI during `init`, it writes `.github/workflows/lyceum.yml`, a pull-request job whose steps are:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0
- uses: oritromax/lyceum/check-action@v1
```

`@v1` runs the latest stable v1 release. For pinning a release, other CI systems, and how the Action
picks a release, see [`docs/ci.md`](docs/ci.md).

## What it isn't

- **Not an agent or harness.** Your harness runs the agent. Lyceum provides the CLI, the skills, and the
  lifecycle checks.
- **Not a code generator or code analyser.** The agent reads and writes code. The CLI manages state, runs
  commands, and checks evidence.
- **Not a replacement for code review.** Gates prove the required approvals and evidence exist. They don't
  prove the code is correct or well designed.
- **Not a large-organisation approval system.** One configured owner clears judgment gates. There are no
  per-gate approvers and no custom gate lists per unit.
- **Not a security boundary on your machine.** Hooks and terminal checks stop ordinary agent behaviour, but
  any process running as your user can get around local controls. CI with branch protection is the
  enforcement; signed approval commits are an opt-in extra.
- **No telemetry.** The CLI opens no network connections except through `git`, `gh`/`glab`, and the
  commands your `TESTING.md` defines.
- **Not on Windows.** v1 supports Linux and macOS. macOS binaries aren't notarized.

## License

MIT. See [`LICENSE`](LICENSE).
