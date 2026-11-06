# Lyceum

Spec-driven software development lifecycle for agentic development. One CLI (`lyceum`) plus skills
that work in any agent harness that reads the Agent Skills standard. Tested with Claude Code, Codex,
OpenCode, Pi and Command Code.

The agent writes specs, plans and code; you approve each gate yourself in a terminal. `lyceum check`
runs from the pre-commit hook and in CI, so work that skipped a gate is caught.

## Install

```sh
npx lyceum init                      # or: npm i -g lyceum
brew install oritromax/tap/lyceum
go install github.com/oritromax/lyceum/cmd/lyceum@latest
```

Platforms: linux-x64, linux-arm64, darwin-x64, darwin-arm64. Binaries are also attached to each GitHub
release; for a macOS browser download, run `xattr -d com.apple.quarantine lyceum` once.

## Quick start

```sh
lyceum init                    # wires your harness, writes AGENTS.md, RULES.md and lyceum/
lyceum add feat "<title>"      # then ask your agent to work the unit
lyceum status                  # where every unit stands and what comes next
lyceum approve                 # you, in a terminal: approve the pending gates
```

The full specification is in [`lyceum/SPEC.md`](lyceum/SPEC.md).

## CI

`lyceum init` writes `.github/workflows/lyceum.yml`, which runs the check on every pull request:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0
- uses: oritromax/lyceum/check-action@v1
```

The action downloads the release archive for the runner (Linux or macOS, X64 or ARM64) and `checksums.txt`
from this repo's GitHub release, refuses an archive whose sha256 does not match, and runs `lyceum check --ci`
in the workspace; the job fails exactly when the check does. Other runners fail with a message naming the
platform. Require the check in branch protection: without that, CI only reports.

Which release runs:

- `@v1` runs the release the `v1` tag points at. The release workflow moves `v1` to each new `v1.x.y` release
  after its archives are verified, never to a prerelease, so `@v1` is the latest stable v1.
- `@v1.2.3` (or `@v1.0.0-rc.1`) runs exactly that release.
- Any other ref (a branch, a commit) runs the release of the highest major tag.
- The `version` input (`latest`, `v1` or `v1.2.3`) overrides the ref.
- While this repo has no major tag (`v1`, `v2`, …) at all, a major (`@v1`), `latest` and a branch or commit
  run the newest release by version, prereleases included (`v1.0.0-rc.2` until `v1.0.0` is out). Once a major
  tag exists, the rules above apply.

## License

MIT
