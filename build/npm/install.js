#!/usr/bin/env node

"use strict";

const fs = require("fs");
const os = require("os");
const path = require("path");
const childProcess = require("child_process");

// Canonical list: keep scripts/install.sh, scripts/install.ps1, scripts/install-skills.sh in sync.
const AGENT_DIRS = [
  ".agents/skills",
  ".claude/skills",
  ".cursor/skills",
  ".qoder/skills",
  ".qoderwork/skills",
  ".gemini/skills",
  ".codex/skills",
  ".github/skills",
  ".windsurf/skills",
  ".augment/skills",
  ".cline/skills",
  ".amp/skills",
  ".kiro/skills",
  ".trae/skills",
  ".openclaw/skills",
  ".hermes/skills",
];

const PLATFORM_MAP = {
  "darwin-x64": "dws-darwin-amd64.tar.gz",
  "darwin-arm64": "dws-darwin-arm64.tar.gz",
  "linux-x64": "dws-linux-amd64.tar.gz",
  "linux-arm64": "dws-linux-arm64.tar.gz",
  "win32-x64": "dws-windows-amd64.zip",
  "win32-arm64": "dws-windows-arm64.zip",
};

function run(command, args) {
  childProcess.execFileSync(command, args, { stdio: "inherit" });
}

function ensureCleanDir(dir) {
  fs.rmSync(dir, { recursive: true, force: true });
  fs.mkdirSync(dir, { recursive: true });
}

function findBinary(root) {
  const entries = fs.readdirSync(root, { withFileTypes: true });
  for (const entry of entries) {
    const entryPath = path.join(root, entry.name);
    if (entry.isDirectory()) {
      const nested = findBinary(entryPath);
      if (nested) {
        return nested;
      }
      continue;
    }
    if (entry.name === "dws" || entry.name === "dws.exe") {
      return entryPath;
    }
  }
  return "";
}

function extractArchive(archivePath, destDir) {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "dws-npm-bin-"));
  try {
    if (archivePath.endsWith(".tar.gz")) {
      run("tar", ["-xzf", archivePath, "-C", tmpDir]);
    } else if (process.platform === "win32") {
      run("powershell.exe", [
        "-NoLogo",
        "-NoProfile",
        "-Command",
        `Expand-Archive -Path '${archivePath.replace(/'/g, "''")}' -DestinationPath '${tmpDir.replace(/'/g, "''")}' -Force`,
      ]);
    } else {
      run("unzip", ["-q", archivePath, "-d", tmpDir]);
    }

    const binaryPath = findBinary(tmpDir);
    if (!binaryPath) {
      throw new Error(`dws binary not found in archive ${archivePath}`);
    }

    ensureCleanDir(destDir);
    const targetName = process.platform === "win32" ? "dws.exe" : "dws";
    const targetPath = path.join(destDir, targetName);
    fs.copyFileSync(binaryPath, targetPath);
    if (process.platform !== "win32") {
      fs.chmodSync(targetPath, 0o755);
    }
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

function extractSkills(zipPath, destDir) {
  ensureCleanDir(destDir);
  if (process.platform === "win32") {
    run("powershell.exe", [
      "-NoLogo",
      "-NoProfile",
      "-Command",
      `Expand-Archive -Path '${zipPath.replace(/'/g, "''")}' -DestinationPath '${destDir.replace(/'/g, "''")}' -Force`,
    ]);
    return;
  }
  run("unzip", ["-q", zipPath, "-d", destDir]);
}

function copyChildren(srcDir, destDir) {
  fs.mkdirSync(destDir, { recursive: true });
  for (const entry of fs.readdirSync(srcDir)) {
    fs.cpSync(path.join(srcDir, entry), path.join(destDir, entry), { recursive: true, force: true });
  }
}

function installSkillsToHomes(skillRoot) {
  const homeDir = os.homedir();
  let installed = 0;

  AGENT_DIRS.forEach((agentDir, index) => {
    const baseDir = path.join(homeDir, agentDir);
    const parentGate = path.dirname(baseDir);
    if (index > 0 && !fs.existsSync(parentGate)) {
      return;
    }
    const destDir = path.join(baseDir, "dws");
    fs.rmSync(destDir, { recursive: true, force: true });
    copyChildren(skillRoot, destDir);
    installed += 1;
  });

  if (installed === 0) {
    copyChildren(skillRoot, path.join(homeDir, ".agents", "skills", "dws"));
  }
}

// cacheUserSkills copies the mono and multi trees out of the freshly extracted
// dws-skills.zip into ~/.dws/skills/{mono,multi}/ so that `dws skill setup`
// can fall back to a user-local cache when --source is not provided. mono is
// already installed into agent homes by installSkillsToHomes; the cache is
// purely a source-of-truth for the setup command.
function cacheUserSkills(extractedSkillsRoot) {
  const cacheBase = path.join(os.homedir(), ".dws", "skills");

  const monoSource = fs.existsSync(path.join(extractedSkillsRoot, "mono", "SKILL.md"))
    ? path.join(extractedSkillsRoot, "mono")
    : extractedSkillsRoot;
  const monoCache = path.join(cacheBase, "mono");
  fs.rmSync(monoCache, { recursive: true, force: true });
  copyChildren(monoSource, monoCache);

  const multiSource = path.join(extractedSkillsRoot, "multi");
  if (fs.existsSync(multiSource) && fs.statSync(multiSource).isDirectory()) {
    const multiCache = path.join(cacheBase, "multi");
    fs.rmSync(multiCache, { recursive: true, force: true });
    copyChildren(multiSource, multiCache);
  }
}

function main() {
  const packageRoot = __dirname;
  const assetsDir = path.join(packageRoot, "assets");
  const vendorDir = path.join(packageRoot, "vendor");
  // Extract dws-skills.zip into a staging directory so we can split mono/
  // (installed to agent homes) from multi/ (cached for later setup use).
  const skillsStaging = path.join(packageRoot, "share", "skills");
  const assetName = PLATFORM_MAP[`${process.platform}-${process.arch}`];
  if (!assetName) {
    throw new Error(`unsupported platform: ${process.platform}/${process.arch}`);
  }

  const archivePath = path.join(assetsDir, assetName);
  const skillsPath = path.join(assetsDir, "dws-skills.zip");
  if (!fs.existsSync(archivePath)) {
    throw new Error(`missing platform archive: ${archivePath}`);
  }
  if (!fs.existsSync(skillsPath)) {
    throw new Error(`missing skills archive: ${skillsPath}`);
  }

  extractArchive(archivePath, vendorDir);
  extractSkills(skillsPath, skillsStaging);

  // For backward compatibility, the zip root carries a copy of mono content
  // (SKILL.md + references/ + scripts/). Prefer the explicit mono/ subdir
  // when present; fall back to the staging root otherwise.
  const monoRoot = fs.existsSync(path.join(skillsStaging, "mono", "SKILL.md"))
    ? path.join(skillsStaging, "mono")
    : skillsStaging;
  installSkillsToHomes(monoRoot);
  cacheUserSkills(skillsStaging);
}

main();
