"use strict";

// Tests for the npm shim, bin/lyceum.js: node --test npm/shim.test.js

const assert = require("node:assert/strict");
const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");

const shim = path.join(__dirname, "..", "bin", "lyceum.js");
const { SUPPORTED, platformPackage, resolveBinary } = require(shim);

const notFound = () => {
  const err = new Error("Cannot find module");
  err.code = "MODULE_NOT_FOUND";
  throw err;
};

test("each v1 platform maps to its package", () => {
  assert.deepEqual(SUPPORTED, ["linux-x64", "linux-arm64", "darwin-x64", "darwin-arm64"]);
  assert.equal(platformPackage("linux", "x64"), "@lyceum-spec/cli-linux-x64");
  assert.equal(platformPackage("linux", "arm64"), "@lyceum-spec/cli-linux-arm64");
  assert.equal(platformPackage("darwin", "x64"), "@lyceum-spec/cli-darwin-x64");
  assert.equal(platformPackage("darwin", "arm64"), "@lyceum-spec/cli-darwin-arm64");
});

test("the binary is resolved from the platform package", () => {
  const got = resolveBinary("darwin", "arm64", (id) => `/nm/${id}`);
  assert.deepEqual(got, { path: "/nm/@lyceum-spec/cli-darwin-arm64/bin/lyceum" });
});

test("AC84: a missing platform package is named, with the reinstall advice", () => {
  const got = resolveBinary("linux", "arm64", notFound);
  assert.match(got.error, /@lyceum-spec\/cli-linux-arm64 is not installed/);
  assert.match(got.error, /without --omit=optional/);
  assert.match(got.error, /npm i -g lyceum/);
});

test("an unsupported platform is refused, naming the supported ones", () => {
  for (const [platform, arch] of [["win32", "x64"], ["linux", "ia32"], ["freebsd", "x64"]]) {
    const got = resolveBinary(platform, arch, notFound);
    assert.match(got.error, new RegExp(`${platform}-${arch} is not supported`));
    assert.match(got.error, /linux-x64, linux-arm64, darwin-x64, darwin-arm64/);
  }
});

test("an unexpected resolve error is not hidden", () => {
  assert.throws(() => resolveBinary("linux", "x64", () => { throw new Error("boom"); }), /boom/);
});

// install lays out node_modules as npm does: the shim in lyceum/bin and, when
// binary is given, a fake platform binary (a shell script) in the package for
// this machine. It returns the shim's path.
function install(binary) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "lyceum-shim-"));
  const nm = path.join(root, "node_modules");
  fs.mkdirSync(path.join(nm, "lyceum", "bin"), { recursive: true });
  fs.copyFileSync(shim, path.join(nm, "lyceum", "bin", "lyceum.js"));
  if (binary !== undefined) {
    const bin = path.join(nm, platformPackage(process.platform, process.arch), "bin");
    fs.mkdirSync(bin, { recursive: true });
    fs.writeFileSync(path.join(bin, "lyceum"), `#!/bin/sh\n${binary}\n`, { mode: 0o755 });
  }
  return path.join(nm, "lyceum", "bin", "lyceum.js");
}

const native = platformPackage(process.platform, process.arch) !== null;

test("arguments, stdio and the exit code pass through", { skip: !native }, () => {
  const res = spawnSync(process.execPath, [install('echo "args:$*"; echo err >&2; exit 7'), "check", "--ci"], { encoding: "utf8" });
  assert.equal(res.stdout, "args:check --ci\n");
  assert.equal(res.stderr, "err\n");
  assert.equal(res.status, 7);
});

test("a terminating signal is re-raised", { skip: !native }, () => {
  const res = spawnSync(process.execPath, [install("kill -TERM $$")]);
  assert.equal(res.signal, "SIGTERM");
});

test("AC84: run without the platform package, the shim names it and exits 1", { skip: !native }, () => {
  const res = spawnSync(process.execPath, [install()], { encoding: "utf8" });
  assert.equal(res.status, 1);
  assert.match(res.stderr, new RegExp(`${platformPackage(process.platform, process.arch)} is not installed`));
  assert.match(res.stderr, /--omit=optional/);
});
