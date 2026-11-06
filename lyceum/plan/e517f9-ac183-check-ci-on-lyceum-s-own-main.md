---
id: e517f9-ac183-check-ci-on-lyceum-s-own-main
type: test
title: 'AC183: check --ci on Lyceum''s own main'
branch: test/e517f9-ac183-check-ci-on-lyceum-s-own-main
status: implemented
gate: close
approvals: []
---

## What and why
AC183 (distribution.md) was the only AC release coverage reported unmapped. Map it to a Go test that runs `lyceum check --ci`, built from the source under test, on Lyceum's own committed HEAD checked out as `main`. ci.yml runs `go test ./...` on pushes to main and PRs, so CI runs it with no workflow change.

## Spec delta

## Steps
- `cmd/lyceum/selfcheck_test.go`: `TestOwnMainPassesCheckCI_AC183` clones HEAD into a temp dir, `checkout -B main <sha>`, runs TestMain's `lyceum-release` binary with `check --ci`, `GITHUB_*`/`GIT_*` cleared; asserts exit 0 and `No findings`.
- Skips only when git is missing or the module root is not a git checkout root.
- Verify: passes here, passes from a depth-1 clone, fails on a committed breakage; release coverage passes.

## Progress
- [x] Test written and passing locally.

## Log
