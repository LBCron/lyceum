---
id: 32a937-release-pin-goreleaser-to-v2-20-0
type: chore
title: 'release: pin goreleaser to v2.20.0'
branch: chore/32a937-release-pin-goreleaser-to-v2-20-0
status: implemented
gate: close
files: [.github/workflows/release.yml]
approvals:
  - gate: plan
    hashes:
      lyceum/plan/32a937-release-pin-goreleaser-to-v2-20-0.md: sha256:9dd89d694202ca0978e49c023cf9090b79e3b460683829f66d8b363ac103161c
    commit: d806835
    by: oritromax@gmail.com
    at: 2026-11-06T16:56:40Z
---

## What and why

`release.yml` runs goreleaser `~> v2`, so each release takes the newest v2. The cask hook uses
`hooks.post.install`, which v2.20.0 marks deprecated (`goreleaser check` exits 2 on it) and is the only form that
renders a working `postflight do`; a later v2 that drops it would break the cask without a change here. Pin the
version the release was verified with, and upgrade it deliberately.

This is also the first pull request since the AC184 fix shipped in a release, so its `lyceum` check is the live
proof that `check --ci` finds the unit on a pull request's merge checkout.

## Spec delta

## Steps

1. `release.yml`: goreleaser-action `version: "~> v2"` becomes `version: v2.20.0`.
2. actionlint; `lyceum test`; `lyceum close`; the pull request's `lyceum` check must pass.

## Progress

## Log
