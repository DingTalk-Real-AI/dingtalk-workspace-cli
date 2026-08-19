#!/usr/bin/env node

"use strict";

const fs = require("fs");
const path = require("path");
const childProcess = require("child_process");

const binaryPath = path.join(__dirname, "..", "vendor", process.platform === "win32" ? "dws.exe" : "dws");

// A package-managed Windows self-update renames the running executable to
// .old before npm/pnpm replaces the package. Recover it if installation was
// interrupted, and discard it only after the new binary proves executable.
const oldBinaryPath = `${binaryPath}.old`;
function restoreOldBinary() {
  try {
    if (fs.existsSync(binaryPath)) fs.rmSync(binaryPath, { force: true });
    fs.renameSync(oldBinaryPath, binaryPath);
    return true;
  } catch (_) {
    return false;
  }
}

if (process.platform === "win32" && fs.existsSync(oldBinaryPath)) {
  if (!fs.existsSync(binaryPath)) {
    restoreOldBinary();
  } else {
    const check = childProcess.spawnSync(binaryPath, ["version"], { stdio: "ignore", timeout: 10000 });
    if (check.status === 0 && !check.error) {
      try { fs.rmSync(oldBinaryPath, { force: true }); } catch (_) {}
    } else {
      restoreOldBinary();
    }
  }
}

if (!fs.existsSync(binaryPath)) {
  console.error(`dws binary not found at ${binaryPath}. Reinstall the package.`);
  process.exit(1);
}

const result = childProcess.spawnSync(binaryPath, process.argv.slice(2), {
  stdio: "inherit",
});

if (result.error) {
  console.error(result.error.message);
  process.exit(1);
}

process.exit(result.status === null ? 1 : result.status);
