#!/usr/bin/env node

"use strict";

const fs = require("fs");
const path = require("path");
const childProcess = require("child_process");

const binaryPath = path.join(__dirname, "..", "vendor", process.platform === "win32" ? "dws.exe" : "dws");

if (!fs.existsSync(binaryPath)) {
  console.error(`dws binary not found at ${binaryPath}. Reinstall the package.`);
  process.exit(1);
}

// Interactive commands must remain in the terminal's foreground session so
// prompts can use /dev/tty. Non-interactive launches use a separate process
// group, allowing a signal sent only to this wrapper to reach the full vendor
// process tree exactly once.
const isolateVendorProcessGroup = process.platform !== "win32" && !process.stdin.isTTY;

const child = childProcess.spawn(binaryPath, process.argv.slice(2), {
  stdio: "inherit",
  detached: isolateVendorProcessGroup,
});

let spawnFailed = false;
let forwardedSignal = null;
const forwardedSignals = ["SIGINT", "SIGTERM"];

function forwardSignal(signal) {
  forwardedSignal = signal;
  if (child.exitCode === null && child.signalCode === null) {
    if (process.platform === "win32") {
      child.kill(signal);
      return;
    }
    if (!isolateVendorProcessGroup) {
      // Ctrl-C is generated for the whole foreground process group, including
      // the vendor. SIGTERM is not terminal-generated and still needs an
      // explicit handoff when a process manager targets only this wrapper.
      if (signal === "SIGTERM") {
        child.kill(signal);
      }
      return;
    }
    try {
      // detached makes the vendor PID the leader of its POSIX process group.
      // Signal the whole group so any subprocesses inherit the same shutdown.
      process.kill(-child.pid, signal);
    } catch (error) {
      // The group may have completed between the state check and kill.
      if (error.code !== "ESRCH") {
        throw error;
      }
    }
  }
}

const signalHandlers = new Map(
  forwardedSignals.map((signal) => [signal, () => forwardSignal(signal)]),
);
for (const signal of forwardedSignals) {
  process.on(signal, signalHandlers.get(signal));
}

child.on("error", (error) => {
  spawnFailed = true;
  console.error(error.message);
});

child.on("close", (code, signal) => {
  for (const forwarded of forwardedSignals) {
    process.removeListener(forwarded, signalHandlers.get(forwarded));
  }

  const exitSignal = forwardedSignal || signal;
  if (exitSignal && process.platform !== "win32") {
    process.kill(process.pid, exitSignal);
    return;
  }
  process.exitCode = spawnFailed || code === null ? 1 : code;
});
