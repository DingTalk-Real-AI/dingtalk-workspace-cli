#!/usr/bin/env node
"use strict";

// Explicit publication entry, never called by staging. Publish verified public
// resources before advancing the main package.
const fs = require("fs");
const path = require("path");
const child = require("child_process");
const crypto = require("crypto");

// Resolve the JavaScript CLI, never execute a .cmd through spawnSync. This
// preserves argument boundaries (including spaces/metacharacters) on Windows.
function npmEntrypoint() {
  if (process.env.npm_execpath && path.basename(process.env.npm_execpath) === "npm-cli.js" && fs.existsSync(process.env.npm_execpath)) return process.env.npm_execpath;
  const directories = (process.env.PATH || "").split(path.delimiter).filter(Boolean);
  for (const directory of directories) {
    const executable = path.join(directory, process.platform === "win32" ? "npm.cmd" : "npm");
    if (!fs.existsSync(executable)) continue;
    const resolved = fs.realpathSync(executable);
    if (path.basename(resolved) === "npm-cli.js") return resolved;
    const cli = path.join(path.dirname(resolved), "node_modules", "npm", "bin", "npm-cli.js");
    if (fs.existsSync(cli)) return cli;
  }
  throw new Error("Cannot locate npm-cli.js; install npm or run through npm exec");
}

function npm(args, cwd) {
  // POSIX version managers may expose an executable shim with no adjacent JS
  // entrypoint. Keep PATH resolution there; Windows requires Node + npm-cli.js.
  const windows = process.platform === "win32";
  const result = child.spawnSync(windows ? process.execPath : "npm", windows ? [npmEntrypoint(), ...args] : args, {
    cwd, encoding: "utf8", env: { ...process.env, NPM_CONFIG_AUDIT: "false", NPM_CONFIG_FUND: "false" },
    ...(args[0] === "publish" ? { stdio: "inherit" } : {}),
  });
  if (result.error) throw result.error;
  return result;
}
function parsed(result) {
  try { return JSON.parse(result.stdout); } catch { throw new Error("npm returned invalid JSON"); }
}
function errorCode(result) {
  try { return JSON.parse(result.stdout).error?.code || "unknown"; }
  catch { return result.stderr?.match(/npm (?:error|ERR!) code (\S+)/)?.[1] || "unknown"; }
}
async function main() {
  const [directory, action] = process.argv.slice(2);
  if (!directory || action !== "--publish") throw new Error("usage: publish-npm-splits.cjs <npm-split-directory> --publish");
  const root = path.resolve(directory);
  const index = JSON.parse(fs.readFileSync(path.join(root, "index.json"), "utf8"));
  if (index.format !== 1 || !Array.isArray(index.packages) || index.packages.length !== 7) throw new Error("incomplete split package index");
  if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-beta\.[1-9]\d*)?$/.test(index.version) ||
      index.prefix !== "dingtalk-workspace-cli" || index.edition !== "public" ||
      index.registry !== "https://registry.npmjs.org") throw new Error("only public DWS packages can be published to npmjs");
  const prepared = [];
  const seen = new Set();
  for (const entry of index.packages) {
    if (!/^(darwin|linux|win32)-(arm64|x64)$|^skills$/.test(entry.directory)) throw new Error("invalid split package directory");
    if (seen.has(entry.directory)) throw new Error("duplicate split package");
    seen.add(entry.directory);
    const dir = path.join(root, entry.directory);
    const manifest = JSON.parse(fs.readFileSync(path.join(dir, "package.json"), "utf8"));
    if (manifest.name !== entry.name || manifest.name !== `${index.prefix}-${entry.directory}` || manifest.version !== index.version ||
        manifest.scripts || manifest.dependencies || manifest.optionalDependencies || manifest.publishConfig?.registry !== index.registry ||
        manifest.dwsUpgrade?.edition !== index.edition || manifest.dwsUpgrade?.asset !== entry.asset ||
        manifest.dwsUpgrade?.sha256 !== entry.sha256 || manifest.dwsUpgrade?.size !== entry.size || manifest.dwsUpgrade?.format !== 1) throw new Error("split package manifest differs from publication index");
    if (!/^dws-(?:(?:darwin|linux)-(?:arm64|amd64)\.tar\.gz|windows-(?:arm64|amd64)\.zip|skills\.zip)$/.test(entry.asset)) throw new Error("unexpected upgrade asset");
    const asset = fs.readFileSync(path.join(dir, "assets", entry.asset));
    if (asset.length !== entry.size || crypto.createHash("sha256").update(asset).digest("hex") !== entry.sha256) throw new Error("split asset changed after staging");
    if (fs.readFileSync(path.join(dir, "assets", "checksums.txt"), "utf8") !== `${entry.sha256}  ${entry.asset}\n`) throw new Error("split checksums changed after staging");
    const packed = npm(["pack", dir, "--pack-destination", root, "--ignore-scripts", "--json"], root);
    if (packed.status !== 0) throw new Error(`npm pack failed for ${entry.name}`);
    const info = parsed(packed)[0];
    const allowed = new Set(["package.json", "assets/checksums.txt", `assets/${entry.asset}`]);
    if (!info?.integrity || !Array.isArray(info.files) || info.files.some(file => !allowed.has(file.path)) || info.files.length !== 3) throw new Error("unexpected files in split tarball");
    prepared.push({ entry, tarball: path.join(root, path.basename(info.filename)), integrity: info.integrity });
  }
  process.stdout.write(`Prepared ${prepared.length} packages for ${index.version}\n`);
  // Resolve every immutable name/version before the first external write.
  for (const item of prepared) {
    const spec = `${item.entry.name}@${index.version}`;
    const existing = npm(["view", spec, "dist.integrity", "--json", "--registry", index.registry], root);
    if (existing.status === 0) {
      if (parsed(existing) !== item.integrity) throw new Error(`published package differs: ${spec}`);
      item.published = true;
      continue;
    }
    if (errorCode(existing) !== "E404") throw new Error(`cannot determine publication state: ${spec} (${errorCode(existing)})`);
  }
  for (const item of prepared) {
    const spec = `${item.entry.name}@${index.version}`;
    // A previous accepted publish may become visible while this batch runs.
    if (!item.published) {
      const current = npm(["view", spec, "dist.integrity", "--json", "--registry", index.registry, "--prefer-online"], root);
      if (current.status === 0) {
        if (parsed(current) !== item.integrity) throw new Error(`published package differs: ${spec}`);
        item.published = true;
      } else if (errorCode(current) !== "E404") {
        throw new Error(`cannot determine publication state: ${spec} (${errorCode(current)})`);
      }
    }
    if (item.published) {
      process.stdout.write(`Already published: ${spec}\n`);
      continue;
    }
    process.stdout.write(`Publishing: ${spec}\n`);
    const result = npm(["publish", item.tarball, "--ignore-scripts", "--access", "public",
      "--tag", `release-${index.version}`, "--registry", index.registry], root);
    if (result.status !== 0) throw new Error(`publication failed: ${spec} (${errorCode(result)}); see npm's local log for authentication details`);
    process.stdout.write(`Submitted: ${spec}; waiting for registry availability\n`);
  }
  // npm may accept a package before its processing finishes. Submit the whole
  // set first, then verify exact-version integrity without republishing it.
  const deadline = Date.now() + 10 * 60 * 1000;
  while (prepared.some(item => !item.published)) {
    for (const item of prepared.filter(item => !item.published)) {
      const spec = `${item.entry.name}@${index.version}`;
      const delivered = npm(["view", spec, "dist.integrity", "--json", "--registry", index.registry, "--prefer-online"], root);
      if (delivered.status === 0) {
        if (parsed(delivered) !== item.integrity) throw new Error(`published package differs: ${spec}`);
        item.published = true;
        process.stdout.write(`Published and verified: ${spec}\n`);
      } else if (errorCode(delivered) !== "E404") {
        throw new Error(`cannot verify publication: ${spec} (${errorCode(delivered)}); retry to reconcile`);
      }
    }
    const pending = prepared.filter(item => !item.published);
    if (!pending.length) break;
    if (Date.now() >= deadline) throw new Error(`npm processing is still pending: ${pending.map(item => item.entry.name).join(", ")}; retry to reconcile`);
    await new Promise(resolve => setTimeout(resolve, 15000));
  }
}
if (require.main === module) main().catch(error => { process.stderr.write(error.message + "\n"); process.exitCode = 1; });
module.exports = { npmEntrypoint, npm };
