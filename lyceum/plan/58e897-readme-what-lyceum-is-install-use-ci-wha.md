---
id: 58e897-readme-what-lyceum-is-install-use-ci-wha
type: chore
title: 'README: what Lyceum is, install, use, CI, what it isn''t'
branch: chore/58e897-readme-what-lyceum-is-install-use-ci-wha
status: closed
gate: close
approvals: []
---

## What and why

Rewrite README.md from a bare quick start into: what Lyceum is (problem and philosophy, from overview.md), install,
how to use it (flows, unit gate table, skills, harnesses), CI, what it isn't, license. README only; no spec change.

## Spec delta

## Steps

1. Draft README.md from overview, units, commands, records, enforcement, test-runner and distribution areas.
2. Verify every command and flag against `lyceum --help` and per-command help; run the shown commands in a
   throwaway repo.
3. Condense the "which release runs" rules; drop the no-major-tag bullet (a `v1` tag exists, so `run.sh` never
   takes that path for users).

## Progress

- Step 1: done.
- Step 2: done (init, status, add feat, refactor, chore, understand, check, check --ci, test, doctor, log).
- Step 3: done.

## Log
