# Lyceum in CI

The README covers the basics. This page has the details.

## What the job does

`.github/workflows/lyceum.yml`, written by `lyceum init`, runs on every pull request. The
`oritromax/lyceum/check-action` step:

1. Picks a Lyceum release (see [Which release runs](#which-release-runs)).
2. Downloads that release's binary for the runner and verifies its SHA-256 checksum.
3. Runs `lyceum check --ci` in the workspace. The job fails when the check exits non-zero.

It runs on Linux and macOS runners, X64 and ARM64. Any other runner fails with a message naming the
unsupported OS or architecture.

`fetch-depth: 0` checks out full history, so the base branch is there for `check` to compare against. If
neither the base branch nor `origin/<base>` exists in the checkout, `check --ci` stops and says to fetch it.

## Pull requests

On `pull_request`, `actions/checkout` checks out a merge commit with a detached HEAD, so there's no branch
name. `check --ci` reads the branch from `GITHUB_HEAD_REF` instead and finds the PR's unit.

## Branch protection

The workflow only reports until you require it. In your repository settings, add a branch protection rule
(or ruleset) for your base branch and require the `check` status check from the `lyceum` workflow.

## Which release runs

| `uses:` ref | Release it runs |
|---|---|
| `@v1` | The release the `v1` tag was last moved to: the latest stable v1 |
| `@v1.2.3` | Exactly that release |
| Any other ref (a branch, a commit) | `latest`: the release behind the highest major tag |

The optional `version` input overrides the ref. It accepts `latest`, a major version (`v1`) or a release tag
(`v1.2.3`):

```yaml
- uses: oritromax/lyceum/check-action@v1
  with:
    version: v1.0.0
```

## Other CI systems

Install Lyceum on the runner (see the README's Install section) and run:

```sh
lyceum check --ci
```

The checkout needs the base branch (or `origin/<base>`) and the unit's branch name:

- Check out the PR's branch itself, not a detached commit. `check --ci` reads the unit from the current
  branch.
- If your CI only gives you a detached HEAD, set `GITHUB_HEAD_REF` to the PR's branch name. On a detached
  HEAD, that's where `check --ci` looks.
