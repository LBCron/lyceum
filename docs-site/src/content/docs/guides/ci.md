---
title: CI
description: Run lyceum check in CI, require it in branch protection, and pick which release runs.
---

| Layer | What it does |
|---|---|
| Pre-commit hook | Runs `lyceum check`, prints findings, and never blocks a commit. A commit past a failing finding is recorded as a `bypass` event, with a few exceptions such as approval drift and warning-only findings. |
| CI | Runs `lyceum check --ci` and fails the job on any failing finding. Warning-only findings, such as stale records, don't fail it. |
| Branch protection | Require the CI check so a failing PR can't merge. Without it, CI only reports. |

`check` covers approval hashes, gate rules, plan fields against the event log, the spec registry, and stale test evidence. A passing suite doesn't stand in for a missing approval.

## GitHub Actions

If you accept CI during `init`, it writes `.github/workflows/lyceum.yml`, a pull-request job whose steps are:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0
- uses: oritromax/lyceum/check-action@v1
```

The `oritromax/lyceum/check-action` step:

1. Picks a Lyceum release (see [Which release runs](#which-release-runs)).
2. Downloads that release's binary for the runner and verifies its SHA-256 checksum.
3. Runs `lyceum check --ci` in the workspace. The job fails when the check exits non-zero.

It runs on Linux and macOS runners, X64 and ARM64. Any other runner fails with a message naming the unsupported OS or architecture.

`fetch-depth: 0` checks out full history, so the base branch is there for `check` to compare against. If neither the base branch nor `origin/<base>` exists in the checkout, `check --ci` stops and says to fetch it.

### Pull requests

On `pull_request`, `actions/checkout` checks out a merge commit with a detached HEAD, so there's no branch name. `check --ci` reads the branch from `GITHUB_HEAD_REF` instead and finds the PR's unit.

## Branch protection

The workflow only reports until you require it. In your repository settings, add a branch protection rule (or ruleset) for your base branch and require the `check` status check from the `lyceum` workflow.

## Which release runs

| `uses:` ref | Release it runs |
|---|---|
| `@v1` | The release the `v1` tag was last moved to: the latest stable v1 |
| `@v1.2.3` | Exactly that release |
| Any other ref (a branch, a commit) | `latest`: the release behind the highest major tag |

The optional `version` input overrides the ref. It accepts `latest`, a major version (`v1`) or a release tag (`v1.2.3`):

```yaml
- uses: oritromax/lyceum/check-action@v1
  with:
    version: v1.0.0
```

## Other CI systems

[Install Lyceum](/start/install/) on the runner and run:

```sh
lyceum check --ci
```

The checkout needs the base branch (or `origin/<base>`) and the unit's branch name:

- Check out the PR's branch itself, not a detached commit. `check --ci` reads the unit from the current branch.
- If your CI only gives you a detached HEAD, set `GITHUB_HEAD_REF` to the PR's branch name. On a detached HEAD, that's where `check --ci` looks.

## What `check` looks at

| Check | Fails when |
|---|---|
| Approval integrity | A hashed file differs from its hash in the approval relied on for it |
| Stale evidence | A unit whose `verify` cleared has a later commit changing a code file |
| Registry | An area file in `lyceum/spec/` has no row in `lyceum/SPEC.md`, or a row's AC range omits an AC defined in its file |
| Unclaimed ACs | An AC that no closed plan lists in `acs:` and no `lyceum/LIBRARY.md` entry names (warning only) |
| CLI-owned fields | Plan frontmatter disagrees with the event log |
| Gate before code | The branch diff touches code while the unit's code-opening gate is not cleared |
| Scope | Changed code files are outside the plan's `files:` list (warning only) |
| Chore | The diff touches code, a manifest, a lockfile or CI config with no approved plan |
| No unit | The branch has no unit and the diff touches code, a manifest, a lockfile or CI config |
| TESTING.md | Changed since its approval while a mechanical gate relies on it |
| Owner | A gate is cleared by an approval that doesn't count |
| Reopen | A bug at `cause` has a code commit after its `verify` cleared |
| Stale records | Records whose citations changed (warning only) |

"Code, manifest, lockfile or CI config" is any path outside `lyceum/`, `docs/` and `*.md`/`*.txt` at any depth. CI config such as `.github/workflows/*.yml` counts as code.
