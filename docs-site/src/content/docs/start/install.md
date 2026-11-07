---
title: Install
description: Install Lyceum with npm, Homebrew, Go, or a release binary.
---

Run it once without installing:

```sh
npx lyceum init
```

Or install it globally with npm:

```sh
npm i -g lyceum
```

With Homebrew:

```sh
brew install oritromax/tap/lyceum
```

With Go:

```sh
go install github.com/oritromax/lyceum/cmd/lyceum@latest
```

## Platforms

Supported platforms are linux-x64, linux-arm64, darwin-x64 and darwin-arm64.

:::caution
Windows isn't supported.
:::

The CLI is a single static binary with no runtime dependency.

## Release downloads

Each GitHub release also has the binaries attached. If you download one on macOS through a browser, run this once before using it:

```sh
xattr -d com.apple.quarantine lyceum
```

macOS binaries aren't signed or notarized in v1.

## How the npm package works

The `lyceum` npm package is a thin shim. It declares one package per platform (`@lyceum-spec/cli-<os>-<arch>`) as optional dependencies and runs the matching binary.

## Next

Run `lyceum init` at the root of your repo. See the [Quickstart](/start/quickstart/).
