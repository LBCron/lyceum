---
id: 45073d-release-macos-15-runner-v1-follows-the-r
type: chore
title: 'release: macos-15 runner, v1 follows the rc until v1.0.0'
branch: chore/45073d-release-macos-15-runner-v1-follows-the-r
status: in-progress
gate: build
files: [.github/workflows/release.yml, check-action/run.sh, cmd/lyceum/testdata/script/ac181_check_action.txtar, README.md, lyceum/decisions.md]
approvals:
  - gate: plan
    hashes:
      lyceum/plan/45073d-release-macos-15-runner-v1-follows-the-r.md: sha256:e5ec125d168f8ee92c0228152d7b068f667302f0c97d7ee83c6223794129a434
    commit: 2802adb
    by: oritromax@gmail.com
    at: 2026-11-06T14:30:32Z
---

## What and why

Two release problems from `v1.0.0-rc.2`:

1. The darwin-arm64 verify job never started: GitHub no longer supports the `macos-14` runner
   (`The configuration 'macos-14-arm64-us-default' is not supported`).
2. `check-action@v1` cannot run before `v1.0.0`. GitHub needs a `v1` ref to fetch the action at all, the release
   job moves `v1` only on stable releases, and `run.sh` accepts only a stable `v1.x.y` behind `v1`. So every pull
   request, here and in any repo that installs an rc, fails the `lyceum` check until `v1.0.0`. A `v1` tagged by hand
   on an rc fails one step later (`tag v1 points at no v1.x.y release`) and turns off the no-major fallback.

Fix: while there is no stable `vN.x.y` yet, `vN` follows the newest verified prerelease; once a stable `vN.x.y`
exists, only stable releases move it, as today.

## Spec delta

## Steps

1. `release.yml`: darwin-arm64 verify on `macos-15`.
2. `release.yml` `major-tag`: also run for a prerelease tag, and move `vN` to it only when no stable `vN.x.y` tag
   exists and it is the highest `vN.*` prerelease; stable releases keep today's rule.
3. `check-action/run.sh` `major_release`: accept the tags at `vN`'s commit that are `vN.x.y` or `vN.x.y-<pre>`,
   preferring a stable one, then the highest by semver precedence.
4. `ac181_check_action.txtar`: new cases — `v1` on an rc commit with no stable tag runs that rc; `v1` on a commit
   carrying both `v1.0.0` and its rc runs `v1.0.0`. Show each new case fails on the old `run.sh`. No existing
   assertion weakened.
5. README CI section and a new `lyceum/decisions.md` row D2 recording the rule (revises the "never to a
   prerelease" line).
6. `lyceum test`; `lyceum close`. After merge the owner tags `v1.0.0-rc.3`; its release run moves `v1` to rc.3.

## Progress

## Log
