---
name: lyceum-audit
description: Use when the owner wants to know whether a Lyceum project's dependencies are vulnerable or outdated, to run the audit, summarise it, and route what the owner acts on.
---

# lyceum-audit

Run `lyceum status` first. Its printed next action wins over anything here. Every `lyceum` command ends with a
`Next:` line; follow it. On a refusal, repeat its reason to the owner in plain words and do what it says.

audit only reports: it never changes packages, manifests, lockfiles or code. It is not a unit.

## 1. Run it

Run `lyceum audit`. It finds the manifests at the repo root, runs each ecosystem's own audit tool, and commits a
record in `lyceum/audit/` holding each tool's command, exit code and raw output, with an empty `## Summary`.
- A tool is missing: the refusal names how to install it. Ask the owner to install it, or to allow you to. Then
  run `lyceum audit` again.
- Files are staged: ask the owner what to do with them.
- No manifest it knows: tell the owner there is nothing to audit.

## 2. Summarise

Read the raw output in the record. Write the summary under its `## Summary`, in plain words:
- Per tool: each vulnerable package with its advisory, severity, installed version and fixed version; outdated
  packages if the tool reports them.
- Say plainly when a tool found nothing, and when a tool failed instead of reporting.
- Don't paste the raw output again, and leave the rest of the record as it is.

Then run `lyceum record lyceum/audit/<file>.md`. It checks the record and commits it.

## 3. Route what the owner acts on

Tell the owner the summary in a few lines and ask which findings to act on:
- Numbered questions, lettered options, your recommended option first, then stop and wait.
- Use the harness's structured-question tool when it has one.

Each finding the owner acts on becomes its own unit:
- An upgrade: `lyceum chore "Upgrade <package> to <version>"`, then the `lyceum-chore` skill.
- A defect the advisory points at in this project's code: `lyceum add bug "<title>"`, then the `lyceum-bug` skill.
Start none without the owner's say.

## Never

- Change packages, manifests, lockfiles or code here, or run a fix or upgrade command (such as `npm audit fix`).
  Only `lyceum/audit/` changes.
- Edit the raw output in the record.
- Fake a terminal, retry a refused command, or work around a hook refusal. A refusal means stop and tell the owner.
