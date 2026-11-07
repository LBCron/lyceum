---
title: What is Lyceum
description: A spec-driven lifecycle for agentic coding, with approvals the agent can't pass and evidence the CLI records itself.
---

Lyceum is an open-source tool for spec-driven development with a coding agent. You stay in control through approvals, and it stays out of your way when you need speed: you only see the gates that need a human.

## The problem it addresses

Coding agents fill every gap in a request with their most plausible guess, and they report success fluently whether or not it happened. Lyceum puts a contract in front of the agent and an evidence check behind it. The human decides every unknown before code exists, approves at gates the agent cannot pass, and accepts "done" only on proof from a run the CLI performed itself.

## How the work is split

Lyceum ships as one CLI (`lyceum`) plus skills that any agent harness can load. All state lives in files in the repo, so switching harness mid-unit loses nothing.

| The CLI (`lyceum`, deterministic) | The agent (skills, any harness) |
|---|---|
| Creates files, assigns IDs, creates branches | Runs interviews |
| Owns every status and gate transition | Drafts plans, spec areas, records, TESTING.md |
| Runs tests and records the runs | Builds code inside an approved plan |
| Computes and checks hashes | Reads CLI output and acts on the printed next step |
| Refuses illegal transitions | Never runs `approve` and never edits CLI-owned fields |
| Records approvals, from the owner's terminal only | |

Every CLI command ends by printing where the work stands and the next action. That printed text is the handoff to the agent.

## Who does what

| Role | Who | What they do |
|---|---|---|
| Owner | The repo owner: a solo developer, or the lead of a small team | Approves every gate. The only identity whose approvals count |
| Teammate | Anyone else with repo access | Runs any command except a counting approval. Their approvals are recorded but do not clear gates |
| Agent | The coding agent in any supported harness | Runs every command except `approve`. Interviews, drafts, builds |

## Goals

- **No gate passes silently.** Every gate clearance is either a human approval bound to the hashes of what was approved, or a CLI-run, CLI-recorded result. Any bypass shows up in `lyceum check`, and CI blocks the merge.
- **Done means verifiable.** A gate never clears on the agent's report of a result, only on a run the CLI performed and recorded.
- **Context stays small.** The agent always loads `AGENTS.md`, `RULES.md` and `TESTING.md`. It loads the plan and the spec areas the plan names only when the work needs them.
- **Any harness.** No state lives in chat.
- **Greenfield and brownfield.** A new project starts with an interview, and an existing one grows its spec only where work happens.

Lyceum must not slow down a less experienced developer, or one on a deadline. The human sees only the gates that genuinely need a human; explanation, bookkeeping, drafting and routing are the agent's job.

## What it isn't

- **Not an agent or harness.** Your harness runs the agent. Lyceum provides the CLI, the skills, and the lifecycle checks.
- **Not a code generator or code analyser.** The agent reads and writes code. The CLI manages state, runs commands, and checks evidence.
- **Not a replacement for code review.** Gates prove the required approvals and evidence exist. They don't prove the code is correct or well designed.
- **Not a large-organisation approval system.** One configured owner clears judgment gates. There are no per-gate approvers and no custom gate lists per unit.
- **Not a security boundary on your machine.** Hooks and terminal checks stop ordinary agent behaviour, but any process running as your user can get around local controls. CI with branch protection is the enforcement; signed approval commits are an opt-in extra.
- **No telemetry.** The CLI opens no network connections except through `git`, `gh`/`glab`, and the commands your `TESTING.md` defines.
- **Not on Windows.** v1 supports Linux and macOS. macOS binaries aren't notarized.

Lyceum is MIT licensed.
