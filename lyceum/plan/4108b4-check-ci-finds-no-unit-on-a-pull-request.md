---
id: 4108b4-check-ci-finds-no-unit-on-a-pull-request
type: bug
title: check --ci finds no unit on a pull request's merge checkout
branch: fix/4108b4-check-ci-finds-no-unit-on-a-pull-request
status: implemented
gate: cause
files: [internal/cli/check.go]
acs: [AC184]
approvals: []
---

## What and why

- Symptom: on a GitHub `pull_request` event, `actions/checkout` checks out the PR's merge commit with HEAD detached.
  `lyceum check --ci` then finds no unit and fails `no-unit` on a PR that changes code, even though the PR's branch
  has a (closed) unit. Seen on PR #5: `fail: no-unit: … .github/workflows/release.yml, .goreleaser.yaml`.
- Expected (owner, option (a)): in CI, with HEAD detached and `GITHUB_HEAD_REF` set (GitHub sets it only on
  `pull_request` events), `check` takes `GITHUB_HEAD_REF` as the branch. Local `check` is unchanged; a detached HEAD
  in CI with no `GITHUB_HEAD_REF` behaves as today.
- Trigger: `git fetch origin pull/5/merge && git checkout FETCH_HEAD`, then `lyceum check --ci`.
- Repro test: `TestScripts/ac184_ci_pr_merge_checkout`.

## Root cause

`runCheck` (`internal/cli/check.go`) takes the branch only from `gitx.Branch` (`git branch --show-current`), which is
`""` on a detached HEAD, and never reads `GITHUB_HEAD_REF`. `openUnit`, `loggedUnit` and `closedUnit` all match the
unit by that branch name, so on a merge checkout no plan matches, `ctx.Plan` is nil, and `NoUnit` fails on the code in
the branch diff. The repro script builds a closed bug on `fix/6ae678-login-plus`, merges it into `main` on a detached
HEAD (main unchanged, like `refs/pull/N/merge`), sets `GITHUB_HEAD_REF`, and asserts `check --ci` passes.

Base for the branch diff: unaffected and right. `baseRef` uses `base_branch` (this repo: `main`), else
`gitx.DefaultBranch`; it never reads `GITHUB_BASE_REF`, and with neither it refuses on a detached HEAD, as before. The
diff is `<base>...HEAD`, from the merge base of `origin/main` and the merge commit. That merge base is the merge
commit's first parent (or `main` itself), so the diff is exactly what the PR adds to `main`.

## Spec delta

- AC184 (enforcement): `check --ci` on a detached merge checkout takes `GITHUB_HEAD_REF` as the branch; locally, or
  with no `GITHUB_HEAD_REF`, a detached HEAD still has no branch. Registry range updated in `lyceum/SPEC.md`.

## Steps

- In `runCheck`, with `--ci` and an empty branch, use `GITHUB_HEAD_REF`.

## Progress

- [x] repro: `ac184_ci_pr_merge_checkout` fails on the old code
- [x] diagnose
- [x] fix
- [ ] verify

## Log
