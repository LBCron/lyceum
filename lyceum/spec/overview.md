# Overview

## Brief

Coding agents fill every gap in a request with their most plausible guess, and they report success
fluently whether or not it happened. Lyceum puts a contract in front of the agent and an evidence check
behind it. The human decides every unknown before code exists, approves at gates the agent cannot pass,
and accepts "done" only on proof from a run the CLI performed itself.

Lyceum works in any agent harness that reads the Agent Skills standard, because all state lives in files
in the repo and the CLI is a plain binary.

## Users

| Role | Who | What they do |
|---|---|---|
| Owner | The repo owner: a solo developer, or the lead of a small team | Approves every gate. The only identity whose approvals count |
| Teammate | Anyone else with repo access | Runs any command except a counting approval. Their approvals are recorded but do not clear gates |
| Agent | The coding agent in any supported harness | Runs every command except `approve`. Interviews, drafts, builds |

Large organisations, role models and per-gate approvers are out of scope.

## Goals

- **GO1 (the invariant): no gate passes silently.** Every gate clearance is either a human approval bound to
  the hashes of what was approved, or a CLI-run, CLI-recorded result. Any bypass shows up in `lyceum check`,
  and CI blocks the merge.
- **GO2:** done means verifiable. A gate never clears on the agent's report of a result, only on a run
  the CLI performed and recorded.
- **GO3:** context stays small. The agent always loads AGENTS.md, RULES.md and TESTING.md. It loads the plan
  and the spec areas the plan names only when the work needs them.
- **GO4:** any harness. Switching harness mid-unit loses nothing, because no state lives in chat.
- **GO5:** greenfield and brownfield. A new project starts with an interview, and an existing one grows
  its spec only where work happens.

## Design principle

**Lyceum must not slow down a less experienced developer, or one on a deadline.** Structure should feel like help,
not a tax. So:
- The human sees only the gates that genuinely need a human. Everything else is mechanical or carried by the agent.
- Explanation, bookkeeping, drafting and routing are the agent's job, not extra commands or prompts for the human.
- When a rule adds a step, a prompt or a concept the user must learn, it needs a reason in GO1–GO5, or it goes.

## Scope

**In (v1):** every command in `commands.md`, freshness states, the signed-approval opt-in, harness hooks,
the CI check, npm/brew/`go install` distribution, and five tested harnesses.

**Out:**
- Roles, per-gate approvers and large-org workflows
- Per-unit tailored gate lists
- Harness transcripts as a `log` source
- macOS notarization

## Non-functional requirements

| # | Requirement |
|---|---|
| NF1 | The CLI is a single static binary with no runtime dependency (`CGO_ENABLED=0`) |
| NF2 | `lyceum status` and the hook entry point complete in under 50 ms on a repo with 200 plan files, measured cold on Linux x64; validated by benchmark during the build |
| NF3 | The CLI never sends network traffic except to run `git`, `gh`/`glab`, or a command the user's TESTING.md defines. No telemetry of any kind |
| NF4 | Every file Lyceum writes is plain text (markdown, YAML frontmatter, JSONL), so it diffs and merges in git |
| NF5 | Linux and macOS on x64 and arm64. Windows unsupported in v1 |
| NF6 | MIT licensed |

## Acceptance criteria

| # | Criterion |
|---|---|
| AC1 | Given a fresh Linux x64 machine with no Node, Go or Rust installed, when the user installs the release binary and runs `lyceum --version`, then it prints the version and exits 0 |
| AC2 | Given a repo using Lyceum, when any command runs, then no network connection is opened other than by a child `git`, `gh`/`glab`, or a TESTING.md-defined command (verified by running under a network-namespace or strace check) |
| AC3 | Given a repo with 200 plan files, when `lyceum status` runs cold, then it completes in under 50 ms (median of 10 runs) |
| AC4 | Given a unit started in one harness, when a different supported harness opens the repo and runs `lyceum status`, then it reports the same unit, gate and next action with no other input |

## Glossary

| Term | Meaning |
|---|---|
| Unit | One piece of change work (`feat`, `bug`, `refactor`, `chore`, `doc`, `test`). One unit = one plan file = one branch |
| Gate | A checkpoint in a unit's fixed gate list. **Judgment** gates clear by owner approval; **mechanical** gates clear on a CLI-recorded run |
| Plan | `lyceum/plan/<hash>-<slug>.md`, the unit itself, with its status in frontmatter |
| Area | One `lyceum/spec/<area>.md` file, registered in `lyceum/SPEC.md` |
| Record | A saved `understand`/`trace`/`audit` output, or an as-built spec section, with hashed citations |
| Inspect command | `understand`, `trace` or `audit`. Never edits code |
| As-built | A spec section describing what existing code does today, written before changing it |
