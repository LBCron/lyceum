---
name: lyceum-status
description: Use when starting a session or unsure what to do next in a Lyceum project, to find the unit in flight, its gate and the next action.
---

# lyceum-status

Run `lyceum status` first. Its printed next action wins over anything here or in another skill.

1. Read the output: the units (the one on this branch is marked), each unit's type, gate, last cleared gate and
   `NEXT`; pending approvals; any warning.
2. Tell the owner in one or two lines: the unit in flight, its gate, and the next action.
3. Load the skill for that unit's type and continue from its gate: `lyceum-feat`, `lyceum-bug`,
   `lyceum-refactor`, `lyceum-chore`, `lyceum-doc` or `lyceum-test`. A pending `project` approval means
   `lyceum-init`.
4. A next action that is an approval is the owner's: tell them the exact `lyceum approve <gate>` command to run in
   their own terminal, and stop. Never run it on your own initiative. One approval per command: the hook denies two in
   one shell call.
5. Nothing in flight: ask the owner what they want to do (a feature, a bug, a refactor, a chore, a doc or a test),
   and load that skill.

A warning (for example that a hook could not read a tool call) goes to the owner as is.
