#!/usr/bin/env node
"use strict";

// Additional data-only packages. The existing all-platform npm package stays
// byte-for-byte unchanged; old installation and sealed-version repair survive.
const fs = require("fs");
const path = require("path");
const crypto = require("crypto");

function stage(dist, version) {
  if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-beta\.[1-9]\d*)?$/.test(version)) {
    throw new Error("only public stable and beta versions can be staged");
  }
  const prefix = "dingtalk-workspace-cli";
  const registry = "https://registry.npmjs.org";
  const platforms = [
    ["darwin", "arm64", "darwin", "arm64"], ["darwin", "amd64", "darwin", "x64"],
    ["linux", "arm64", "linux", "arm64"], ["linux", "amd64", "linux", "x64"],
    ["windows", "arm64", "win32", "arm64"], ["windows", "amd64", "win32", "x64"],
  ];
  const assets = platforms.map(([goos, goarch, os, cpu]) => ({
    file: `dws-${goos}-${goarch}.${goos === "windows" ? "zip" : "tar.gz"}`,
    suffix: `${os}-${cpu}`, os, cpu,
  }));
  assets.push({ file: "dws-skills.zip", suffix: "skills" });
  const checksumText = fs.readFileSync(path.join(dist, "checksums.txt"), "utf8");
  const hashes = new Map(checksumText.trim().split(/\r?\n/).map(line => line.trim().split(/\s+/)));
  const prepared = assets.map(asset => {
    const data = fs.readFileSync(path.join(dist, asset.file));
    const sha256 = crypto.createHash("sha256").update(data).digest("hex");
    const expected = [...hashes].find(([hash, filename]) => filename === asset.file)?.[0];
    if (!expected || sha256 !== expected.toLowerCase()) throw new Error(`checksum mismatch: ${asset.file}`);
    return { ...asset, data, sha256 };
  });
  // Validate every input before replacing any previous staging output.
  const root = path.join(dist, "npm-split");
  fs.mkdirSync(root, { recursive: true });
  const packages = [];
  for (const asset of prepared) {
    const name = `${prefix}-${asset.suffix}`;
    const dir = path.join(root, asset.suffix);
    fs.rmSync(dir, { recursive: true, force: true });
    fs.mkdirSync(path.join(dir, "assets"), { recursive: true });
    fs.writeFileSync(path.join(dir, "assets", asset.file), asset.data);
    fs.writeFileSync(path.join(dir, "assets", "checksums.txt"), `${asset.sha256}  ${asset.file}\n`);
    const manifest = {
      name, version, description: `DWS upgrade resource: ${asset.suffix}`,
      license: "Apache-2.0", files: ["assets"],
      repository: { type: "git", url: "git+https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli.git" },
      ...(asset.os ? { os: [asset.os], cpu: [asset.cpu] } : {}),
      publishConfig: { registry, access: "public" },
      dwsUpgrade: { format: 1, edition: "public", asset: asset.file, sha256: asset.sha256, size: asset.data.length },
    };
    fs.writeFileSync(path.join(dir, "package.json"), JSON.stringify(manifest, null, 2) + "\n");
    packages.push({ name, directory: asset.suffix, asset: asset.file, sha256: asset.sha256, size: asset.data.length });
  }
  const index = { format: 1, version, prefix, registry, edition: "public", packages };
  fs.writeFileSync(path.join(root, "index.json"), JSON.stringify(index, null, 2) + "\n");
  return index;
}

if (require.main === module) {
  try {
    const [dist, rawVersion] = process.argv.slice(2);
    if (!dist || !rawVersion) throw new Error("usage: stage-npm-splits.cjs <dist> <version>");
    const version = rawVersion.replace(/^v/, "");
    const index = stage(path.resolve(dist), version);
    process.stdout.write(`Staged ${index.packages.length} split packages for ${version}\n`);
  } catch (error) { process.stderr.write(error.message + "\n"); process.exitCode = 1; }
}
module.exports = { stage };
