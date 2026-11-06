#!/usr/bin/env node
"use strict";

// The npm shim (esbuild pattern): npm installs the native binary as the
// optional dependency @lyceum-spec/cli-<os>-<arch> matching this machine, and
// this script runs it with the same arguments, stdio, exit code and signal.

const { spawnSync } = require("node:child_process");
const os = require("node:os");

const SUPPORTED = ["linux-x64", "linux-arm64", "darwin-x64", "darwin-arm64"];

// platformPackage returns the package holding the binary for platform and
// arch (Node's process.platform and process.arch), or null if there is none.
function platformPackage(platform, arch) {
  const key = `${platform}-${arch}`;
  return SUPPORTED.includes(key) ? `@lyceum-spec/cli-${key}` : null;
}

// resolveBinary returns { path } of the binary for platform and arch, found
// with resolve (require.resolve), or { error } saying why there is none.
function resolveBinary(platform, arch, resolve) {
  const pkg = platformPackage(platform, arch);
  if (pkg === null) {
    return {
      error:
        `lyceum: ${platform}-${arch} is not supported.\n` +
        `Supported platforms: ${SUPPORTED.join(", ")}.`,
    };
  }
  try {
    return { path: resolve(`${pkg}/bin/lyceum`) };
  } catch (err) {
    if (err.code !== "MODULE_NOT_FOUND") throw err;
    return {
      error:
        `lyceum: the platform package ${pkg} is not installed.\n` +
        `npm leaves it out when optional dependencies are omitted (--omit=optional).\n` +
        `Reinstall lyceum without --omit=optional, e.g. npm i -g lyceum`,
    };
  }
}

function main() {
  const found = resolveBinary(process.platform, process.arch, require.resolve);
  if (found.error) {
    process.stderr.write(found.error + "\n");
    return 1;
  }
  const res = spawnSync(found.path, process.argv.slice(2), { stdio: "inherit" });
  if (res.error) {
    process.stderr.write(`lyceum: cannot run ${found.path}: ${res.error.message}\n`);
    return 1;
  }
  if (res.signal) {
    // Die of the same signal, so the caller sees what the binary saw. A signal
    // Node ignores (SIGPIPE) falls through to the shell's 128+n convention.
    process.kill(process.pid, res.signal);
    return 128 + os.constants.signals[res.signal];
  }
  return res.status;
}

if (require.main === module) {
  process.exitCode = main();
}

module.exports = { SUPPORTED, platformPackage, resolveBinary };
