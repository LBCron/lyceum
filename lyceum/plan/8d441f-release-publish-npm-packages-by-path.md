---
id: 8d441f-release-publish-npm-packages-by-path
type: chore
title: 'release: publish npm packages by ./ path'
branch: chore/8d441f-release-publish-npm-packages-by-path
status: in-progress
gate: build
files: [.github/workflows/release.yml]
approvals:
  - gate: plan
    hashes:
      lyceum/plan/8d441f-release-publish-npm-packages-by-path.md: sha256:0420994d557954646122c8c254ea0cb7fc3454221173d8997f6e6681d61ab83b
    commit: a85a22f
    by: oritromax@gmail.com
    at: 2026-11-06T14:08:09Z
---

## What and why

The `v1.0.0-rc.1` release run failed at "Publish to npm": `npm publish "npm-out/$pkg"` is read by npm as the
GitHub shorthand `npm-out/<pkg>` and fetched over SSH (`Permission denied (publickey)`), so no package was published.
A `./` prefix makes it a local folder. Reproduced locally: `npm publish npm-out/pkg --dry-run` fails with the same git
error, `npm publish ./npm-out/pkg --dry-run` packs.

## Spec delta

## Steps

1. In `.github/workflows/release.yml`, publish `./npm-out/$pkg` instead of `npm-out/$pkg`.
2. `lyceum test`; then `lyceum close`.
3. After merge, the owner tags `v1.0.0-rc.2` to rerun the release.

## Progress

## Log
