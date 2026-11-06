---
id: 7da1dd-release-cask-postflight-brew-install-che
type: chore
title: 'release: cask postflight, brew install checked before the tap'
branch: chore/7da1dd-release-cask-postflight-brew-install-che
status: in-progress
gate: build
files: [.goreleaser.yaml, .github/workflows/release.yml]
approvals:
  - gate: plan
    hashes:
      lyceum/plan/7da1dd-release-cask-postflight-brew-install-che.md: sha256:7a1683944a56e702902697d9cbe8c2f997099862ff857fed791b8dc48a18f713
    commit: 62f6af7
    by: oritromax@gmail.com
    at: 2026-11-06T14:59:23Z
---

## What and why

The `v1.0.0-rc.3` cask does not load, so `brew tap oritromax/tap` fails for everyone (`invalid syntax in tap!`).
`.goreleaser.yaml` sets `hooks.post.install_steps`, which renders `postflight_steps do`; that block accepts only
Homebrew's step commands, so `system_command` and `staged_path` are undefined in it. Tested on Homebrew 7.0.12
(darwin-arm64) from a local tap:

- block removed: installs, but the binary keeps `com.apple.quarantine` and `lyceum --version` is killed (exit 137);
- `postflight do` with the same `xattr` call (goreleaser `hooks.post.install`): quarantine removed, `--version`
  prints `lyceum 1.0.0-rc.3`, uninstall clean.

The release also pushes the cask to the tap before anything has installed it, so a broken cask reaches users first.

## Spec delta

## Steps

1. `.goreleaser.yaml`: `hooks.post.install_steps` becomes `hooks.post.install` (same `xattr` body), and the cask
   is generated but not pushed (`skip_upload: true`).
2. `release.yml` `release` job: upload goreleaser's generated `Casks/lyceum.rb` as a workflow artifact.
3. `release.yml` darwin verify jobs: after the archive check, download that artifact, install it from a local tap
   (`brew tap-new`, cask copied in), assert the installed binary has no `com.apple.quarantine` and that
   `lyceum --version` prints `lyceum <version>`, then uninstall. Linux verify jobs unchanged.
4. `release.yml`: new `tap` job after `verify` that commits the artifact cask to `oritromax/homebrew-tap`
   `Casks/lyceum.rb` with `HOMEBREW_TAP_TOKEN` (commit message `lyceum <tag>`), scoped to that secret only.
   `major-tag` order unchanged.
5. Checks: `goreleaser check`; a snapshot release renders `postflight do` (not `postflight_steps`); actionlint;
   the step-3 install script run by hand on the Mac mini against the snapshot cask pointed at the rc.3 archives.
6. `lyceum test`; `lyceum close`. After merge the owner tags `v1.0.0-rc.4`; then `brew install
   oritromax/tap/lyceum` is checked on the Mac mini.

## Progress

- [x] 1. `hooks.post.install` (renders `postflight do`), `skip_upload: true`; cask lands in
  `dist/homebrew/Casks/lyceum.rb`. Tap token is only read on upload, so goreleaser no longer gets it.
- [x] 2. `release` uploads that file as artifact `cask`.
- [x] 3. darwin verify: brew install from local tap, no quarantine, `--version`, uninstall.
- [x] 4. `tap` job (`permissions: {}`, PAT in step env) commits `Casks/lyceum.rb`; `major-tag` needs `[verify, tap]`.
- [ ] 5. `goreleaser check` exits 2 (deprecated `hooks.post.install`, config valid); snapshot renders `postflight do`,
  no `postflight_steps`; actionlint clean. Mac-mini hand run: owner.

## Log
