---
title: Agents
description: The five tested agents, what init installs for each, and how the hook works.
---

| Agent | Tested version | Status |
|---|---|---|
| Claude Code | 2.1.318 | ✅ |
| Codex | 0.164.0 | ✅ |
| OpenCode | 2.0.18 | ✅ |
| Pi | 0.86.0 | ✅ |
| Command Code | 1.88.3 | ✅ |

Each was tested end to end in the v1 release check.

:::note
Any other agent that reads `AGENTS.md` and `.agents/skills/` can follow the workflow, but without a hook nothing stops it before a gate is approved; the check still catches it at commit and in CI.
:::

## Wiring an agent

`lyceum init` detects an agent when its binary is on your `PATH` or its project folder exists (`.claude/`, `.codex/`, `.opencode/`, `.pi/`, `.commandcode/`), and asks about each one it detects. To wire one without being asked:

```sh
lyceum init --harness claude
```

`--harness` is repeatable and accepts `claude`, `codex`, `commandcode`, `opencode` and `pi`.

For each agent it wires, `init` installs skills, slash commands where supported, and a hook. Skills go to `.agents/skills/lyceum-*/`, and to `.claude/skills/lyceum-*/` when Claude Code is wired. Wiring Claude Code also adds a one-line pointer to `AGENTS.md` in `CLAUDE.md`.

| Agent | Hook |
|---|---|
| Claude Code | `PreToolUse` in `.claude/settings.json` |
| Codex | `PreToolUse` in `.codex/hooks.json` |
| OpenCode | A plugin in `.opencode/plugins/` |
| Pi | An extension in `.pi/extensions/` |
| Command Code | `PreToolUse` in `.commandcode/settings.json` |

Existing settings files are merged: every existing key and value is kept.

:::caution
Codex and Pi run project hooks only after you trust the project in that agent. Until you do, the hook silently doesn't run. `init` prints the step for each.
:::

## Starting a skill

| Agent | Start a skill |
|---|---|
| Claude Code | `/lyceum-feat` |
| Codex | `$lyceum-feat` |
| Pi | `/skill:lyceum-feat` |
| OpenCode, Command Code | Ask for the skill by name |

The skills are `lyceum-feat`, `lyceum-bug`, `lyceum-doc`, `lyceum-test`, `lyceum-refactor`, `lyceum-chore`, `lyceum-init` (the new-project interview), `lyceum-status` (run `lyceum status` and follow the next step), and the inspect skills `lyceum-understand`, `lyceum-trace` and `lyceum-audit`.

## What the hook does

While the unit's code-opening gate is not cleared, the hook blocks writes to code. Test files are allowed at the gates that allow them. The plan's `files:` list is never a block, only a `check` warning. The hook also blocks any shell command that runs `lyceum approve` or `lyceum eject`. When it denies, the reason names the gate and the next step, and tells the agent not to retry.

The hook does not parse shell writes such as `printf hi > f`; those are left to `check`.

In Claude Code, OpenCode and Pi, the hook can instead ask you to confirm an approval in the agent's own prompt. See [Approvals](/concepts/approvals/#approving-from-inside-the-agent).

## Codex sandbox

Wiring Codex adds the repo's `.git` folder as a writable root in `.codex/config.toml`, so the agent can commit under `workspace-write`. That sandbox also keeps the network off, so a suite that binds ports can fail there and Codex asks to escalate on each run. You can set `network_access = true` under `[sandbox_workspace_write]` in `.codex/config.toml`, which opens outbound network to the agent too, or run such tests outside the sandbox.

## OpenCode server

When a re-run of `init` changes an existing OpenCode plugin, restart OpenCode's server (`opencode serve`): a running server keeps the plugin version it first loaded.

## Removing Lyceum

`lyceum eject` removes skills, slash commands, hooks, the CI workflow, the pre-commit hook and the appended Lyceum sections in `AGENTS.md` and `CLAUDE.md`. It leaves `lyceum/` and the root docs in place as plain markdown. Like `approve`, it runs only in your own terminal and is confirmed by typing `eject`.
