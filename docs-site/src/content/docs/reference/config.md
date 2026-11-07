---
title: Configuration
description: The keys in lyceum/config.yaml.
---

`lyceum init` writes `lyceum/config.yaml`. It is committed, so every clone sees the same owner and settings. The owner edits it.

| Key | Meaning | Default |
|---|---|---|
| `owner` | Owner identity: git email | `git config user.email` at `init` |
| `base_branch` | Branch units are cut from | The remote's default branch; with no remote, the current branch |
| `approvals.signed` | Require signed approval commits | `false` |
| `approvals.harness_prompt` | Allow the harness-prompt approval route where the harness supports it | `allow` |
| `harnesses` | Harnesses wired at `init` | detected |
| `hooks` | Harness hooks installed | `true` where supported |
| `understand.warn_threshold` | Scope above which `understand` warns about cost | 2,000 files or 200,000 lines |

## `owner`

Only approvals by this git email count. An approval by anyone else is written with `counts: false`, and the gate does not clear.

## `approvals.signed`

With `true`, each approval also creates a signed commit containing the event, and `check` verifies the signature against the owner's key.

## `approvals.harness_prompt`

With `allow`, in Claude Code, OpenCode and Pi you can tell the agent to run `lyceum approve` for you, and the agent's own approval prompt asks you to confirm. Set it to `deny` to turn the route off. `check --ci` then fails on a counting approval made through the harness prompt for the unit on the branch being checked; approvals of other units stay as they were.

See [Approvals](/concepts/approvals/#approving-from-inside-the-agent).

## `understand.warn_threshold`

Takes `files` and `lines`. Above either, `understand` warns about cost.

## Not in this file

How tests run lives in `TESTING.md`, not here. See [Acceptance criteria and evidence](/concepts/acceptance-criteria/#testingmd).
