---
id: ffa56b-readme-and-docs-describe-gitignore-and-g
type: doc
title: 'README and docs: describe .gitignore and .gitattributes accurately (#13)'
branch: docs/ffa56b-readme-and-docs-describe-gitignore-and-g
status: implemented
gate: close
approvals: []
---

## What and why

Issue #13: the README row for `.gitattributes`, `.gitignore` says "Lyceum's local state", which reads as if the
state is kept in `.gitignore`. It is not: the state is `lyceum/events.jsonl` and the plans, which are committed.
`.gitignore` lists `.lyceum/` (local runs and cache) and the files harnesses write on their own;
`.gitattributes` holds the merge rule for the event log (spec: distribution.md, AC9, AC176). The same row was
copied into the docs-site quickstart. Reword both.

## Spec delta

None.

## Steps

1. `README.md` and `docs-site/src/content/docs/start/quickstart.md`: reword the `.gitattributes`, `.gitignore`
   row to name the merge rule and the ignore entries.
2. `lyceum test`; then `lyceum close`.

## Progress

## Log
