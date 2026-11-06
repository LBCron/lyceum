---
id: 878c57-release-check-record-v1-packaging-on-fou
type: doc
title: 'release-check record: v1 packaging on four platforms'
branch: docs/878c57-release-check-record-v1-packaging-on-fou
status: implemented
gate: close
approvals: []
---

## What and why

Record the v1 packaging ACs (AC83, AC84, AC85, AC93) as passed on all four v1 platforms, from the published
`v1.0.0-rc.5`, in a new latest release-check record that also carries the harness results, so release coverage is
complete for `v1.0.0`.

## Spec delta

## Steps

1. Write `release-checks/v1-2026-11-06.md`.
2. `LYCEUM_RELEASE_COVERAGE=1 go test -run TestACCoverage$ ./internal/acmap/`: AC83–AC85 and AC93 count as mapped;
   the one AC left, AC183, has no test at all and is a separate unit.
3. `lyceum test`; `lyceum close`.

## Progress

## Log
