---
title: Approvals
description: How judgment gates clear, what an approval hashes, and the harness-prompt route.
---

A judgment gate clears only when the owner approves it. Only the owner configured in `lyceum/config.yaml` can clear one: anyone else's approval is recorded but does not count.

```sh
lyceum approve spec
```

Bare `lyceum approve` lists every pending judgment gate for the current unit, plus `testing` if `TESTING.md` is unapproved or changed, and walks through them in one session.

## Your terminal only

`lyceum approve` only runs in your own terminal. Inside an agent's shell it refuses, so the agent can tell you which approval is due and the exact command to type, but it can't approve anything on its own.

`approve` refuses when:

- a known harness environment variable is set, or
- stdin and stdout are not terminals.

Run it in a separate terminal the agent doesn't control. The in-agent shell, such as Claude Code's `!` prefix, doesn't work.

## What happens when you approve

1. A summary shows the unit, the gate, the files to be approved with their sha256, and the diff since the last approval of this gate.
2. You type the gate name exactly to confirm.
3. For the gate that opens code (`spec` for feat, `plan` for refactor and planned chore), `approve` refuses if there are changes outside `lyceum/`, and lists them.
4. One `approve` event is written to `lyceum/events.jsonl` with the hash of each approved file, the commit, who approved and when. The approval commit contains the event, the plan and every approved file.

## What each approval hashes

| Gate | Files hashed |
|---|---|
| `spec` | The plan file, every area file it names, and `lyceum/SPEC.md` |
| `project` | Every registered area file and `lyceum/SPEC.md` |
| `plan` | The plan file |
| `intent` | The plan's `## What and why` section and its `goal:` and `benchmark:` fields |
| `cause` (bug) | The plan file and every file in the fix diff |
| `testing` | `TESTING.md` |
| `tests` | Each test file that differs from its fingerprint |

A plan file is hashed without its `## Progress` and `## Log` sections, which the agent keeps updating, and without `files:`, the planned scope.

`lyceum check` recomputes these hashes. A hashed file that changed after approval is drift: a pending re-approval, listed by `lyceum status`.

## Approving from inside the agent

In Claude Code, OpenCode and Pi you can tell the agent to run `lyceum approve` for you. The agent then asks you to confirm in its own approval prompt before the approval goes through. This is on by default.

Codex and Command Code have no such prompt, so their hooks keep denying `lyceum approve`.

The event records `via: harness-prompt`; approvals from a terminal record `via: terminal`. In auto-approve modes no prompt appears; that is your choice and is not blocked, but the event still says so.

This route reduces friction; it is not a security boundary. Set `approvals.harness_prompt: deny` in `lyceum/config.yaml` to turn it off. See [Configuration](/reference/config/).

## Signed approvals

With `approvals.signed: true`, each approval also creates a signed commit, and `check` verifies the signature against the owner's key.

## How far enforcement goes

Local checks catch a sloppy or over-eager agent. CI plus branch protection catches everything else. Nothing that runs locally as the same OS user is absolute.

| Layer | Mechanism |
|---|---|
| 1 | `AGENTS.md`, `RULES.md` and skills: stop at gates, never approve, never fake a terminal |
| 2 | The CLI refuses illegal transitions, such as `close` before `verify` |
| 3 | `approve` is human-only: terminal, typed confirmation, harness environment refusal |
| 4 | Approvals bound to file hashes; `check` recomputes them |
| 5 | Approving the gate that opens code refuses a dirty tree outside `lyceum/` |
| 6 | `check` in pre-commit (warn) and CI (fail) |
| 7 | Harness hooks block code writes until the code-opening gate is cleared |
| 8 | Signed approvals (opt-in) |

See [CI](/guides/ci/) for the last line of defence.
