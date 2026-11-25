#!/usr/bin/env node
"use strict";

// Bind every transferred byte and relative filename to a job output, outside
// the artifact itself. Only regular files/directories are accepted. This tool
// never loads package code, npm hooks or binaries from the transferred tree.
const fs = require("fs");
const path = require("path");
const crypto = require("crypto");
const root = path.resolve(process.argv[2]);
const hash = crypto.createHash("sha256");
function visit(relative) {
  const filename = path.join(root, relative);
  const stat = fs.lstatSync(filename);
  if (stat.isDirectory()) {
    for (const name of fs.readdirSync(filename).sort()) visit(path.join(relative, name));
  } else if (stat.isFile()) {
    const bytes = fs.readFileSync(filename);
    hash.update(JSON.stringify([relative.split(path.sep).join("/"), bytes.length,
      crypto.createHash("sha256").update(bytes).digest("hex")]) + "\n");
  } else {
    throw new Error("Non-regular file in npm handoff: " + relative);
  }
}
visit("");
const digest = hash.digest("hex");
if (process.argv[3] && digest !== process.argv[3]) throw new Error("npm handoff digest mismatch");
process.stdout.write(digest + "\n");
