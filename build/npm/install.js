#!/usr/bin/env node

"use strict";

const fs = require("fs");
const crypto = require("crypto");
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

// backupStamp returns the UTC timestamp used for backup directory names,
// matching the shell installers' `date -u +%Y%m%d-%H%M%S` layout.
function backupStamp() {
  const d = new Date();
  const pad = (n) => String(n).padStart(2, "0");
  return (
    `${d.getUTCFullYear()}${pad(d.getUTCMonth() + 1)}${pad(d.getUTCDate())}` +
    `-${pad(d.getUTCHours())}${pad(d.getUTCMinutes())}${pad(d.getUTCSeconds())}`
  );
}

// backupAndRemoveSkillDir moves dir into <homeDir>/.dws/skill-backups/
// <stamp>/<rel-or-basename> instead of destroying it (non-interactive
// installs cannot confirm, so removals must stay reversible). Missing paths
// are a no-op success. On any backup failure the directory is left in place
// and false is returned so callers skip that target rather than silently
// deleting data.
function backupAndRemoveSkillDir(homeDir, dir, backups = null, renameFn = fs.renameSync) {
  if (!fs.existsSync(dir) || !fs.statSync(dir).isDirectory()) {
    return true;
  }
  const rel = path.relative(homeDir, dir);
  const name =
    rel && rel !== "." && !rel.startsWith("..") && !path.isAbsolute(rel)
      ? rel.split(path.sep).join("-")
      : path.basename(dir);
  const stamp = backupStamp();
  const backupRoot = path.join(homeDir, ".dws", "skill-backups");
  let targetRoot = path.join(backupRoot, stamp);
  let target = path.join(targetRoot, name);
  for (let i = 1; fs.existsSync(target); i++) {
    if (i > 1000) {
      console.warn(`⚠️  备份目录冲突，保留原目录 ${dir}`);
      return false;
    }
    targetRoot = path.join(backupRoot, `${stamp}-${i}`);
    target = path.join(targetRoot, name);
  }
  try {
    fs.mkdirSync(targetRoot, { recursive: true });
    renameFn(dir, target);
  } catch (err) {
    console.warn(`⚠️  备份失败，保留原目录 ${dir}: ${err.message}`);
    return false;
  }
  if (backups) {
    backups.push({ original: dir, backup: target });
  }
  console.log(`  × 已备份并移除 ${dir} → ${target}`);
  return true;
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

// publishCacheAtomically prepares a complete sibling tree before replacing a
// cache. If copying or publishing fails, the previous cache stays available.
// copyFn is injectable so the failure contract can be tested without relying
// on platform-specific permission behavior.
function publishCacheAtomically(sourceDir, cacheDir, copyFn = copyChildren) {
  const cacheParent = path.dirname(cacheDir);
  const cacheName = path.basename(cacheDir);
  fs.mkdirSync(cacheParent, { recursive: true });

  const stagedDir = fs.mkdtempSync(path.join(cacheParent, `.${cacheName}.tmp-`));
  let rollbackDir = "";
  let published = false;
  try {
    copyFn(sourceDir, stagedDir);

    if (fs.existsSync(cacheDir)) {
      rollbackDir = fs.mkdtempSync(path.join(cacheParent, `.${cacheName}.old-`));
      fs.rmSync(rollbackDir, { recursive: true, force: true });
      fs.renameSync(cacheDir, rollbackDir);
    }

    try {
      fs.renameSync(stagedDir, cacheDir);
      published = true;
    } catch (publishErr) {
      if (rollbackDir) {
        try {
          fs.renameSync(rollbackDir, cacheDir);
          rollbackDir = "";
        } catch (restoreErr) {
          throw new Error(
            `failed to publish cache ${cacheDir}: ${publishErr.message}; ` +
              `failed to restore previous cache from ${rollbackDir}: ${restoreErr.message}`,
          );
        }
      }
      throw publishErr;
    }

    if (rollbackDir) {
      try {
        fs.rmSync(rollbackDir, { recursive: true, force: true });
      } catch (cleanupErr) {
        console.warn(
          `⚠️  New cache is active, but old cache cleanup failed at ${rollbackDir}: ${cleanupErr.message}`,
        );
      }
      rollbackDir = "";
    }
  } finally {
    if (!published) {
      fs.rmSync(stagedDir, { recursive: true, force: true });
    }
  }
}

function installSkillsToHomes(skillRoot) {
  const homeDir = os.homedir();
  const managedNames = readManagedSkillNames(homeDir);
  let installed = 0;
  let attempted = 0;
  let failed = 0;

  const installToBase = (baseDir) => {
    const victims = [path.join(baseDir, "dws")];
    if (fs.existsSync(baseDir)) {
      for (const entry of fs.readdirSync(baseDir, { withFileTypes: true })) {
        if (entry.isDirectory() && isManagedMultiSkillDir(path.join(baseDir, entry.name), managedNames)) {
          victims.push(path.join(baseDir, entry.name));
        }
      }
    }
    try {
      publishManagedMonoSkillSetAtomically(homeDir, skillRoot, baseDir, victims);
    } catch (err) {
      console.warn(`⚠️  跳过 ${baseDir}（mono 集合发布失败，已回滚）: ${err.message}`);
      return false;
    }
    return true;
  };

  AGENT_DIRS.forEach((agentDir, index) => {
    const baseDir = path.join(homeDir, agentDir);
    const parentGate = path.dirname(baseDir);
    if (index > 0 && !fs.existsSync(parentGate)) {
      return;
    }
    attempted += 1;
    if (installToBase(baseDir)) {
      installed += 1;
    } else {
      failed += 1;
    }
  });

  if (attempted === 0) {
    if (installToBase(path.join(homeDir, ".agents", "skills"))) {
      installed += 1;
    } else {
      failed += 1;
    }
  }
  if (installed === 0) {
    throw new Error("未安装任何 mono Skill：所有检测到的 Agent 目标均失败");
  }
  if (failed > 0) {
    throw new Error(`有 ${failed} 个 Agent 目标安装 mono Skill 失败`);
  }
  fs.rmSync(path.join(skillStateDir(homeDir), "skills-state.json"), { force: true });
}

// multiTreeHasSkills mirrors multi_tree_has_skills in scripts/install.sh and
// Test-MultiTreeHasSkills in scripts/install.ps1: true only when the multi
// bundle carries at least one product skill (a subdir with SKILL.md). An
// empty or corrupt multi/ tree must never select the multi branch nor refresh
// the multi cache — installing it would wipe existing skills and lay down
// nothing.
function multiTreeHasSkills(dir) {
  if (!fs.existsSync(dir) || !fs.statSync(dir).isDirectory()) {
    return false;
  }
  return fs
    .readdirSync(dir, { withFileTypes: true })
    .some((e) => e.isDirectory() && fs.existsSync(path.join(dir, e.name, "SKILL.md")));
}

const MANAGED_SKILL_DIGEST_SCOPE = "skill-directory-v1";
// Frozen exact names shipped before centralized ownership metadata. Retired
// names stay here so old installs can be migrated without treating every
// dingtalk-* directory as DWS-owned.
const LEGACY_OFFICIAL_MULTI_SKILLS = new Set([
  "dingtalk-agoal", "dingtalk-aiapp", "dingtalk-aisearch", "dingtalk-aitable",
  "dingtalk-attendance", "dingtalk-calendar", "dingtalk-chat", "dingtalk-contact",
  "dingtalk-dev", "dingtalk-devapp", "dingtalk-devdoc", "dingtalk-ding",
  "dingtalk-doc", "dingtalk-drive", "dingtalk-event", "dingtalk-hrbrain",
  "dingtalk-live", "dingtalk-mail", "dingtalk-markdown", "dingtalk-minutes",
  "dingtalk-misc", "dingtalk-oa", "dingtalk-pat", "dingtalk-profile",
  "dingtalk-report", "dingtalk-shared", "dingtalk-sheet", "dingtalk-skill",
  "dingtalk-todo", "dingtalk-wiki", "dws-shared",
]);

function skillStateDir(homeDir) {
  return (process.env.DWS_CONFIG_DIR || "").trim() || path.join(homeDir, ".dws");
}

function readManagedSkillNames(homeDir) {
  try {
    const state = JSON.parse(fs.readFileSync(path.join(skillStateDir(homeDir), "skills-state.json"), "utf8"));
    return new Set((state.managed_skills || []).map((record) => record.name).filter(Boolean));
  } catch (_) {
    return new Set();
  }
}

function isManagedMultiSkillDir(dir, managedNames) {
  const name = path.basename(dir);
  return LEGACY_OFFICIAL_MULTI_SKILLS.has(name) || managedNames.has(name);
}

function skillDirectoryDigest(dir) {
  const files = [];
  const visit = (current, prefix) => {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const rel = prefix ? `${prefix}/${entry.name}` : entry.name;
      const full = path.join(current, entry.name);
      if (entry.isDirectory()) {
        visit(full, rel);
      } else {
        files.push({ rel, full });
      }
    }
  };
  visit(dir, "");
  files.sort((a, b) => Buffer.from(a.rel).compare(Buffer.from(b.rel)));
  const hash = crypto.createHash("sha256");
  for (const file of files) {
    hash.update(file.rel, "utf8");
    hash.update(Buffer.from([0]));
    hash.update(fs.readFileSync(file.full));
    hash.update(Buffer.from([0]));
  }
  return `sha256:${hash.digest("hex")}`;
}

// Publish a complete multi-skill set as one transaction. The entire new set
// is staged before any Agent-visible directory moves. If a later backup or
// publish fails, every partial publication is removed and all old directories
// are restored from their exact backup paths.
function publishManagedMultiSkillSetAtomically(
  homeDir,
  multiRoot,
  baseDir,
  skills,
  victims,
  options = {},
) {
  const copyFn = options.copyFn || copyChildren;
  const renameFn = options.renameFn || fs.renameSync;
  const removeFn = options.removeFn || ((dir) => fs.rmSync(dir, { recursive: true, force: true }));
  fs.mkdirSync(baseDir, { recursive: true });
  const stageRoot = fs.mkdtempSync(path.join(baseDir, ".dws-multi-set.tmp-"));
  const staged = [];
  const backups = [];
  const published = [];

  const restore = () => {
    const restoreErrors = [];
    for (let i = published.length - 1; i >= 0; i -= 1) {
      try {
        removeFn(published[i]);
      } catch (err) {
        restoreErrors.push(`remove ${published[i]}: ${err.message}`);
      }
    }
    for (let i = backups.length - 1; i >= 0; i -= 1) {
      const item = backups[i];
      try {
        fs.mkdirSync(path.dirname(item.original), { recursive: true });
        renameFn(item.backup, item.original);
      } catch (err) {
        restoreErrors.push(`restore ${item.original} from ${item.backup}: ${err.message}`);
      }
    }
    if (restoreErrors.length > 0) {
      throw new Error(restoreErrors.join("; "));
    }
  };

  try {
    for (const name of skills) {
      const stagedDir = path.join(stageRoot, name);
      copyFn(path.join(multiRoot, name), stagedDir);
      staged.push({ staged: stagedDir, dest: path.join(baseDir, name) });
    }

    const seen = new Set();
    for (const victim of victims) {
      const normalized = path.resolve(victim);
      if (seen.has(normalized)) {
        continue;
      }
      seen.add(normalized);
      if (!backupAndRemoveSkillDir(homeDir, victim, backups, renameFn)) {
        throw new Error(`failed to back up Skill directory ${victim}`);
      }
    }

    for (const item of staged) {
      renameFn(item.staged, item.dest);
      published.push(item.dest);
    }
  } catch (err) {
    try {
      restore();
    } catch (restoreErr) {
      throw new Error(`${err.message}; rollback failed: ${restoreErr.message}`);
    }
    throw err;
  } finally {
    removeFn(stageRoot);
  }
}

// Publish mono plus every mutually-exclusive managed multi victim as one
// transaction. The complete dws/ tree is staged before any live directory is
// moved; a later backup or publish failure restores the exact previous set.
function publishManagedMonoSkillSetAtomically(
  homeDir,
  monoRoot,
  baseDir,
  victims,
  options = {},
) {
  const copyFn = options.copyFn || copyChildren;
  const renameFn = options.renameFn || fs.renameSync;
  const removeFn = options.removeFn || ((dir) => fs.rmSync(dir, { recursive: true, force: true }));
  fs.mkdirSync(baseDir, { recursive: true });
  const stageRoot = fs.mkdtempSync(path.join(baseDir, ".dws-mono-set.tmp-"));
  const stagedDir = path.join(stageRoot, "dws");
  const destDir = path.join(baseDir, "dws");
  const backups = [];
  const published = [];

  const restore = () => {
    const restoreErrors = [];
    for (let i = published.length - 1; i >= 0; i -= 1) {
      try {
        removeFn(published[i]);
      } catch (err) {
        restoreErrors.push(`remove ${published[i]}: ${err.message}`);
      }
    }
    for (let i = backups.length - 1; i >= 0; i -= 1) {
      const item = backups[i];
      try {
        fs.mkdirSync(path.dirname(item.original), { recursive: true });
        renameFn(item.backup, item.original);
      } catch (err) {
        restoreErrors.push(`restore ${item.original} from ${item.backup}: ${err.message}`);
      }
    }
    if (restoreErrors.length > 0) {
      throw new Error(restoreErrors.join("; "));
    }
  };

  try {
    copyFn(monoRoot, stagedDir);

    const seen = new Set();
    for (const victim of victims) {
      const normalized = path.resolve(victim);
      if (seen.has(normalized)) {
        continue;
      }
      seen.add(normalized);
      if (!backupAndRemoveSkillDir(homeDir, victim, backups, renameFn)) {
        throw new Error(`failed to back up Skill directory ${victim}`);
      }
    }

    published.push(destDir);
    renameFn(stagedDir, destDir);
  } catch (err) {
    try {
      restore();
    } catch (restoreErr) {
      throw new Error(`${err.message}; rollback failed: ${restoreErr.message}`);
    }
    throw err;
  } finally {
    removeFn(stageRoot);
  }
}

function writeSkillsState(homeDir, multiRoot, skills) {
  const version = process.env.npm_package_version || process.env.DWS_PACKAGE_VERSION || "unknown";
  const managedSkills = [...skills].sort().map((name) => ({
    name,
    version,
    source: "npm-postinstall",
    digest: skillDirectoryDigest(path.join(multiRoot, name)),
    digest_scope: MANAGED_SKILL_DIGEST_SCOPE,
  }));
  const state = {
    version,
    official_skills: [...skills].sort(),
    updated_skills: [...skills].sort(),
    managed_skills: managedSkills,
    updated_at: new Date().toISOString(),
  };
  const stateDir = skillStateDir(homeDir);
  fs.mkdirSync(stateDir, { recursive: true });
  const stage = fs.mkdtempSync(path.join(stateDir, ".skills-state.tmp-"));
  const stagedFile = path.join(stage, "skills-state.json");
  const statePath = path.join(stateDir, "skills-state.json");
  const rollbackPath = path.join(stage, "skills-state.previous.json");
  let movedPrevious = false;
  let preserveRecovery = false;
  try {
    fs.writeFileSync(stagedFile, `${JSON.stringify(state, null, 2)}\n`, "utf8");
    if (fs.existsSync(statePath)) {
      fs.renameSync(statePath, rollbackPath);
      movedPrevious = true;
    }
    try {
      fs.renameSync(stagedFile, statePath);
    } catch (err) {
      if (movedPrevious && !fs.existsSync(statePath)) {
        try {
          fs.renameSync(rollbackPath, statePath);
          movedPrevious = false;
        } catch (restoreErr) {
          preserveRecovery = true;
          throw new Error(
            `publish skills state failed: ${err.message}; restore also failed: ${restoreErr.message}; previous state retained at ${rollbackPath}`,
          );
        }
      }
      throw err;
    }
  } finally {
    if (!preserveRecovery) {
      fs.rmSync(stage, { recursive: true, force: true });
    }
  }
}

// installMultiSkillsToHomes mirrors installSkillsToHomes for the multi bundle:
// every product skill becomes a sibling directory of the agent home. Mutual
// exclusion: the mono leftover (dws/) and stale, proven DWS-managed skills not
// present in the new bundle are removed first.
function installMultiSkillsToHomes(multiRoot) {
  const homeDir = os.homedir();
  const skills = fs
    .readdirSync(multiRoot, { withFileTypes: true })
    .filter((e) => e.isDirectory() && fs.existsSync(path.join(multiRoot, e.name, "SKILL.md")))
    .map((e) => e.name);
  if (skills.length === 0) {
    throw new Error(`no product skills found under ${multiRoot}`);
  }
  const skillSet = new Set(skills);
  const managedNames = readManagedSkillNames(homeDir);
  let installed = 0;
  let attempted = 0;
  let failed = 0;

  const installToBase = (baseDir) => {
    fs.mkdirSync(baseDir, { recursive: true });
    const victims = [path.join(baseDir, "dws")];
    // Mutual exclusion: include the mono leftover and stale managed skills in
    // the same transaction as every replaced bundled skill.
    for (const entry of fs.readdirSync(baseDir, { withFileTypes: true })) {
      if (
        entry.isDirectory() &&
        (LEGACY_OFFICIAL_MULTI_SKILLS.has(entry.name) || managedNames.has(entry.name)) &&
        !skillSet.has(entry.name)
      ) {
        victims.push(path.join(baseDir, entry.name));
      }
    }
    for (const name of skills) {
      victims.push(path.join(baseDir, name));
    }
    try {
      publishManagedMultiSkillSetAtomically(homeDir, multiRoot, baseDir, skills, victims);
    } catch (err) {
      console.warn(`⚠️  跳过 ${baseDir}（multi 集合发布失败，已回滚）: ${err.message}`);
      return false;
    }
    return true;
  };

  AGENT_DIRS.forEach((agentDir, index) => {
    const baseDir = path.join(homeDir, agentDir);
    const parentGate = path.dirname(baseDir);
    if (index > 0 && !fs.existsSync(parentGate)) {
      return;
    }
    attempted += 1;
    if (installToBase(baseDir)) {
      installed += 1;
    } else {
      failed += 1;
    }
  });

  if (attempted === 0) {
    if (installToBase(path.join(homeDir, ".agents", "skills"))) {
      installed += 1;
    } else {
      failed += 1;
    }
  }
  if (installed === 0) {
    throw new Error("未安装任何 multi Skill：所有检测到的 Agent 目标均失败");
  }
  if (failed > 0) {
    throw new Error(`有 ${failed} 个 Agent 目标安装 multi Skill 失败`);
  }
  writeSkillsState(homeDir, multiRoot, skills);
}

// resolveSkillMode mirrors scripts/install.sh: DWS_SKILL_MODE (mono|multi)
// wins; multi is the default. The --skill-mode flag accepts both the space
// form (`--skill-mode mono`) and the equals form (`--skill-mode=mono`).
function resolveSkillMode() {
  const raw = (process.env.DWS_SKILL_MODE || "").trim().toLowerCase();
  if (raw === "mono" || raw === "multi") {
    return raw;
  }
  if (raw !== "") {
    throw new Error(`invalid DWS_SKILL_MODE='${process.env.DWS_SKILL_MODE}'. Use 'mono' or 'multi'.`);
  }
  let fromFlag;
  const flagIndex = process.argv.indexOf("--skill-mode");
  if (flagIndex !== -1 && process.argv[flagIndex + 1]) {
    fromFlag = process.argv[flagIndex + 1];
  } else {
    const equalsArg = process.argv.find((arg) => arg.startsWith("--skill-mode="));
    if (equalsArg) {
      fromFlag = equalsArg.slice("--skill-mode=".length);
    }
  }
  if (fromFlag !== undefined) {
    const mode = fromFlag.trim().toLowerCase();
    if (mode === "mono" || mode === "multi") {
      return mode;
    }
    throw new Error(`invalid --skill-mode '${fromFlag}'. Use 'mono' or 'multi'.`);
  }
  return "multi";
}

// cacheUserSkills copies the mono and multi trees out of the freshly extracted
// dws-skills.zip into ~/.dws/skills/{mono,multi}/ so that `dws skill setup`
// can fall back to a user-local cache when --source is not provided. A cache
// is only refreshed when the new bundle actually carries that tree — an
// empty/corrupt multi/ (or a missing mono tree) must never wipe a previously
// good cache.
function cacheUserSkills(extractedSkillsRoot) {
  const cacheBase = path.join(os.homedir(), ".dws", "skills");

  const monoSource = fs.existsSync(path.join(extractedSkillsRoot, "mono", "SKILL.md"))
    ? path.join(extractedSkillsRoot, "mono")
    : extractedSkillsRoot;
  if (fs.existsSync(path.join(monoSource, "SKILL.md"))) {
    const monoCache = path.join(cacheBase, "mono");
    publishCacheAtomically(monoSource, monoCache);
  }

  const multiSource = path.join(extractedSkillsRoot, "multi");
  if (multiTreeHasSkills(multiSource)) {
    const multiCache = path.join(cacheBase, "multi");
    publishCacheAtomically(multiSource, multiCache);
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
  // A mono install requires an actual SKILL.md at the root of monoRoot. On a
  // multi-only zip monoRoot would degrade to the staging root and copy the
  // whole bundle (multi/ included) into a dws/ directory — skip instead.
  const monoHasSkill = fs.existsSync(path.join(monoRoot, "SKILL.md"));
  const multiRoot = path.join(skillsStaging, "multi");
  const skillMode = resolveSkillMode();
  if (skillMode === "multi" && multiTreeHasSkills(multiRoot)) {
    console.log(`Skill mode: multi — installing per-product skills`);
    installMultiSkillsToHomes(multiRoot);
  } else {
    if (skillMode === "multi") {
      console.log("multi skill tree not found or empty in bundle; falling back to mono.");
    }
    if (monoHasSkill) {
      installSkillsToHomes(monoRoot);
    } else {
      console.log("mono skill tree not found in bundle; skipping skill install.");
    }
  }
  cacheUserSkills(skillsStaging);
}

if (require.main === module) {
  main();
}

module.exports = {
  publishCacheAtomically,
  publishManagedMonoSkillSetAtomically,
  publishManagedMultiSkillSetAtomically,
};
