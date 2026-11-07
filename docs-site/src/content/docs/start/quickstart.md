---
title: Quickstart
description: Set up Lyceum in a repo and take one feature from request to close.
---

## Run `init`

Run `init` at the root of your repo, new or existing:

```sh
lyceum init
```

If the folder isn't a git repo yet, `init` offers to run `git init` first.

### What `init` does

1. **Detects** whether the repo already has code, and which agents you use. An agent counts as detected when its binary is on your `PATH` or its project folder exists (`.claude/`, `.codex/`, `.opencode/`, `.pi/`, `.commandcode/`).
2. **Writes** the Lyceum files:

   | File | What it holds |
   |---|---|
   | `AGENTS.md` | The rules your agent reads first. |
   | `RULES.md` | Project rules. |
   | `lyceum/SPEC.md` | The spec index: one row per area. |
   | `lyceum/config.yaml` | Owner, base branch, approval settings. |
   | `lyceum/events.jsonl` | The event log for approvals and runs. Empty at first. |
   | `.gitattributes`, `.gitignore` | Merge rules for the event log, and Lyceum's local state. |

   An existing `AGENTS.md` or `CLAUDE.md` is never overwritten. `init` shows you its section, asks once, and appends it between `<!-- lyceum:start -->` and `<!-- lyceum:end -->` markers.
3. **Wires each agent you pick:** skills, slash commands and a hook that blocks code edits until the right gate is approved. Use `--harness <name>` to wire one without being asked.
4. **Offers** a pre-commit hook and a CI workflow. Both default to yes.

:::note
Codex and Pi run project hooks only after you trust the project in that agent. Until you do, the hook silently doesn't run. `init` prints the step for each.
:::

`TESTING.md` isn't written by `init`. The first `lyceum test` drafts it from the test frameworks it detects, and you approve it with `lyceum approve testing`.

Re-running `init` updates the skills and hooks to your installed version. It never touches your `lyceum/` content.

## New project

On a project with no code, the agent interviews you. It opens with one open question (what are you building, and for whom?), then numbered questions with options, at most two rounds. It writes spec areas under `lyceum/spec/`, each with acceptance criteria, and you approve the whole initial spec with:

```sh
lyceum approve project
```

Then start your first unit with `lyceum add feat "<title>"`. For an existing codebase, see [Existing projects](/guides/existing-projects/).

## Add a feature

Each piece of work is a **unit**: one plan file under `lyceum/plan/` and one branch. You can drive it from inside your agent or from your terminal.

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

`lyceum approve` only runs in your own terminal. Inside an agent's shell it refuses, so the agent can tell you which approval is due and the exact command to type, but it can't approve anything on its own. See [Approvals](/concepts/approvals/).

The examples use Claude Code's skill syntax. For other agents, see [Agents](/guides/agents/).

## Not sure what's next?

```sh
lyceum status
```

It names the unit in flight, its gate and the next action.
