---
title: Existing projects
description: Adopt Lyceum in a codebase that already exists, growing the spec only where work happens.
---

Run `init` at the root of the repo:

```sh
lyceum init
```

On existing code there is no project interview and no project-wide approval. The spec grows one area at a time, as units touch them.

| | New project (no code) | Existing code |
|---|---|---|
| Next step | The agent interviews you about the project | The agent offers `lyceum understand` to explain the code you have |
| The interview | It opens with one open question (what are you building, and for whom?), then numbered questions with options, at most two rounds | None |
| What it writes | Spec areas under `lyceum/spec/`, each with acceptance criteria | A baseline record under `lyceum/understand/`, if you accept |
| Your approval | `lyceum approve project`, covering the whole initial spec | No project-wide approval |
| How the spec grows | Units add acceptance criteria to existing areas, or register a new area with `lyceum plan --new-area` | One area at a time, as units touch them. Each new area starts with an as-built section written from the code, which you approve with the unit's plan. Untouched areas stay unspecified. |
| First unit | `lyceum add feat "<title>"` | Whatever you want to do next: a feature, a bug fix, a refactor |

## The baseline: `understand`

`lyceum understand` explains the project or part of it. The agent reads the code and saves the answer as a record under `lyceum/understand/`. Inspect commands never edit code and are not units: no branch, plan or gates.

A record cites the files it was written from. The CLI hashes those citations, so it can tell when a record no longer matches the code:

| State | Meaning |
|---|---|
| fresh | All cited hashes match |
| drifted | A cited file or folder changed |
| broken | A cited path no longer exists |
| missing | No record matches |
| invalid | The record can't be parsed, or its citations have no hashes yet |

Drift means "possibly stale", not "wrong". `lyceum status` lists drifted and broken records, and `check` warns on them.

Records from `understand` seed as-built spec sections later. When the answer is useful to humans, `lyceum export` copies it to `docs/`.

## Growing the spec

When a unit's plan names an area that has no spec file, the agent first writes that area's as-built section from the code, citing files, and starting from a matching `understand` record if one exists. You approve it together with the plan.

## TESTING.md on existing code

The first `lyceum test` drafts `TESTING.md` from the test frameworks it detects. If the project has no tests, it prompts to create them; on existing code these are characterization tests that pin current behaviour, right or wrong.

## Other inspect commands

- `lyceum trace "<question>"` follows one flow end to end, by reading the code or with a full run against an environment defined in `TESTING.md`.
- `lyceum audit` runs each detected ecosystem's standard audit tool and reports vulnerable or outdated dependencies. It never changes packages or code.

Findings from inspect commands are routed, never fixed: the agent offers a bug, a feature or a chore.
