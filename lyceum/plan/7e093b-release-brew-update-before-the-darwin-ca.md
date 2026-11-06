---
id: 7e093b-release-brew-update-before-the-darwin-ca
type: chore
title: 'release: brew update before the darwin cask install'
branch: chore/7e093b-release-brew-update-before-the-darwin-ca
status: implemented
gate: close
files: [.github/workflows/release.yml]
approvals:
  - gate: plan
    hashes:
      lyceum/plan/7e093b-release-brew-update-before-the-darwin-ca.md: sha256:48b1a0ff861803b879d2240174799fdfa9b845213cfaae12415bdbfcbb16ffff
    commit: 6cb1be8
    by: oritromax@gmail.com
    at: 2026-11-06T15:40:03Z
---

## What and why

The `v1.0.0-rc.4` darwin-x64 verify job failed to install the cask: `Cask 'lyceum' definition is invalid: invalid
'zap' stanza: wrong number of arguments (given 2, expected 1)`. goreleaser writes `zap :not_required`, which the
runner's preinstalled Homebrew (macos-15 image 2026-08-24) predates; the darwin-arm64 image (2026-11-04) accepts it.
The step sets `HOMEBREW_NO_AUTO_UPDATE=1`, so brew never updates first, unlike a user's `brew install`.

## Spec delta

## Steps

1. `release.yml` darwin brew step: drop `HOMEBREW_NO_AUTO_UPDATE=1` and run `brew update` before `brew tap-new`;
   keep `HOMEBREW_NO_INSTALL_CLEANUP=1`.
2. actionlint; `lyceum test`; `lyceum close`. After merge the owner tags `v1.0.0-rc.5`.

## Progress

## Log
