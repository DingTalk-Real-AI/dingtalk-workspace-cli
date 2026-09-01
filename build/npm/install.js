#!/usr/bin/env node

"use strict";

const fs = require("fs");
const crypto = require("crypto");
const os = require("os");
const path = require("path");
const childProcess = require("child_process");

// Exact vercel-labs/skills registry at the reference revision: 76 Agent IDs,
// including canonical-direct and no-global-directory entries. The resolver
// below intentionally deduplicates shared effective roots only after this
// authoritative enumeration has been classified.
const UPSTREAM_AGENTS = [
  ["aider-desk", false, ".aider-desk/skills"],
  ["amp", true, ".config/agents/skills"],
  ["antigravity", true, ".gemini/antigravity/skills"],
  ["antigravity-cli", true, ".gemini/antigravity-cli/skills"],
  ["astrbot", false, ".astrbot/data/skills"],
  ["autohand-code", false, ".autohand/skills"],
  ["augment", false, ".augment/skills"], ["bob", false, ".bob/skills"],
  ["claude-code", false, ".claude/skills"], ["openclaw", false, ".openclaw/skills"],
  ["cline", true, ".agents/skills"], ["codearts-agent", false, ".codeartsdoer/skills"],
  ["codebuddy", false, ".codebuddy/skills"], ["codemaker", false, ".codemaker/skills"],
  ["codestudio", false, ".codestudio/skills"], ["codex", true, ".codex/skills"],
  ["command-code", false, ".commandcode/skills"], ["continue", false, ".continue/skills"],
  ["cortex", false, ".snowflake/cortex/skills"], ["crush", false, ".config/crush/skills"],
  ["cursor", true, ".cursor/skills"], ["deepagents", true, ".deepagents/agent/skills"],
  ["devin", false, ".config/devin/skills"], ["dexto", true, ".agents/skills"],
  ["droid", false, ".factory/skills"], ["eve", false, null],
  ["firebender", true, ".firebender/skills"], ["forgecode", false, ".forge/skills"],
  ["gemini-cli", true, ".gemini/skills"], ["github-copilot", true, ".copilot/skills"],
  ["goose", false, ".config/goose/skills"], ["grok", false, ".grok/skills"],
  ["hermes-agent", false, ".hermes/skills"], ["inference-sh", false, ".inferencesh/skills"],
  ["jazz", false, ".jazz/skills"], ["junie", false, ".junie/skills"],
  ["iflow-cli", false, ".iflow/skills"], ["kilo", false, ".kilocode/skills"],
  ["kimchi", false, ".config/kimchi/harness/skills"], ["kimi-code-cli", true, ".agents/skills"],
  ["kiro-cli", false, ".kiro/skills"], ["kode", false, ".kode/skills"],
  ["lingma", false, ".lingma/skills"], ["loaf", true, ".agents/skills"],
  ["mcpjam", false, ".mcpjam/skills"], ["minimax-code", false, ".minimax/skills"],
  ["mistral-vibe", false, ".vibe/skills"], ["moxby", false, ".moxby/skills"],
  ["mux", false, ".mux/skills"], ["opencode", true, ".config/opencode/skills"],
  ["openhands", false, ".openhands/skills"], ["ona", false, ".ona/skills"],
  ["pi", false, ".pi/agent/skills"], ["qoder", false, ".qoder/skills"],
  ["qoder-cn", false, ".qoder-cn/skills"], ["qwen-code", false, ".qwen/skills"],
  ["replit", true, ".config/agents/skills"], ["reasonix", false, ".reasonix/skills"],
  ["rovodev", false, ".rovodev/skills"], ["roo", false, ".roo/skills"],
  ["tabnine-cli", false, ".tabnine/agent/skills"], ["terramind", false, ".terramind/skills"],
  ["tinycloud", false, ".tinycloud/skills"], ["trae", false, ".trae/skills"],
  ["trae-cn", false, ".trae-cn/skills"], ["warp", true, ".agents/skills"],
  ["windsurf", false, ".codeium/windsurf/skills"], ["zed", true, ".agents/skills"],
  ["zcode", false, ".zcode/skills"], ["zencoder", false, ".zencoder/skills"],
  ["zenflow", false, ".zencoder/skills"], ["neovate", false, ".neovate/skills"],
  ["pochi", false, ".pochi/skills"], ["promptscript", true, null],
  ["adal", false, ".adal/skills"], ["universal", true, ".config/agents/skills"],
].map(([id, universal, agentDir]) => ({ id, universal, agentDir }));

// Paths emitted by beta.6 or older DWS installers that do not correspond to
// the upstream registry. They are cleanup-only migration targets.
const LEGACY_UNIVERSAL_AGENT_DIRS = [
  ".github/skills", ".windsurf/skills", ".cline/skills", ".amp/skills",
];

function openClawSkillsDir(homeDir) {
  for (const name of [".openclaw", ".clawdbot", ".moltbot"]) {
    const root = path.join(homeDir, name);
    if (fs.existsSync(root)) return path.join(root, "skills");
  }
  return path.join(homeDir, ".openclaw", "skills");
}

function resolvedAgentTargets(homeDir) {
  const xdgConfigHome = (process.env.XDG_CONFIG_HOME || "").trim() || path.join(homeDir, ".config");
  const targets = [
    ...UPSTREAM_AGENTS.filter(({ agentDir }) => agentDir && agentDir !== ".agents/skills"),
    // DWS compatibility target; qoderwork is not in upstream's registry.
    { id: "dws-qoderwork", agentDir: ".qoderwork/skills", universal: false, extension: true },
    ...LEGACY_UNIVERSAL_AGENT_DIRS.map((agentDir) => ({ agentDir, universal: true, legacy: true })),
  ].map((target) => {
    const { agentDir } = target;
    let baseDir = path.join(homeDir, agentDir);
    if (agentDir === ".claude/skills" && (process.env.CLAUDE_CONFIG_DIR || "").trim()) {
      baseDir = path.join(process.env.CLAUDE_CONFIG_DIR.trim(), "skills");
    } else if (agentDir === ".codex/skills" && (process.env.CODEX_HOME || "").trim()) {
      baseDir = path.join(process.env.CODEX_HOME.trim(), "skills");
    } else if (agentDir === ".hermes/skills" && (process.env.HERMES_HOME || "").trim()) {
      baseDir = path.join(process.env.HERMES_HOME.trim(), "skills");
    } else if (agentDir === ".autohand/skills" && (process.env.AUTOHAND_HOME || "").trim()) {
      baseDir = path.join(process.env.AUTOHAND_HOME.trim(), "skills");
    } else if (agentDir === ".grok/skills" && (process.env.GROK_HOME || "").trim()) {
      baseDir = path.join(process.env.GROK_HOME.trim(), "skills");
    } else if (agentDir === ".vibe/skills" && (process.env.VIBE_HOME || "").trim()) {
      baseDir = path.join(process.env.VIBE_HOME.trim(), "skills");
    } else if (agentDir === ".openclaw/skills") {
      baseDir = openClawSkillsDir(homeDir);
    } else if (agentDir.startsWith(".config/")) {
      baseDir = path.join(xdgConfigHome, agentDir.slice(".config/".length));
    }
    return { ...target, baseDir };
  });
  const seen = new Set();
  return targets.filter((target) => {
    let key = path.resolve(target.baseDir);
    if (process.platform === "win32") key = key.toLowerCase();
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

function agentTargetDetected(target) {
  const parent = path.dirname(target.baseDir);
  switch (target.id) {
    case "kimchi":
    case "tabnine-cli":
      return fs.existsSync(path.dirname(parent));
    case "zcode":
      return fs.existsSync(parent) || fs.existsSync("/Applications/ZCode.app");
    case "minimax-code":
      return fs.existsSync(parent) || fs.existsSync("/Applications/MiniMax Code.app");
    default:
      return fs.existsSync(parent);
  }
}

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

// SKILL_BACKUP_KEEP bounds ~/.dws/skill-backups/ growth: only the newest
// stamped backup directories are kept. Mirrors skillBackupKeep and
// pruneSkillBackups in internal/upgrade/paths.go so every installer surface
// applies the same retention.
const SKILL_BACKUP_KEEP = 5;

// Backup roots this process created are never pruned: an in-flight
// transaction still needs them to roll back.
const currentRunBackupRoots = new Set();

// Ownership marker: every stamp root DWS creates carries .dws-skill-backup
// with exactly these bytes — the same bytes internal/upgrade/paths.go,
// install.sh, and the PowerShell installers write. A stamp-shaped name alone
// is not ownership proof, so pruning only deletes directories whose marker
// content matches byte-for-byte (cross-surface exact-LF contract; a
// PowerShell writer's hypothetical CRLF is normalized away on that surface,
// while every writer here emits LF).
const SKILL_BACKUP_MARKER_FILE = ".dws-skill-backup";
const SKILL_BACKUP_MARKER_BODY = "dws skill backup v1\n";

// writeSkillBackupMarker stamps a freshly created stamp root as DWS-owned.
function writeSkillBackupMarker(root) {
  fs.writeFileSync(path.join(root, SKILL_BACKUP_MARKER_FILE), SKILL_BACKUP_MARKER_BODY);
}

// skillBackupRootUsable reports whether a stamp root may be written into:
// either it does not exist yet (this transaction creates it fresh) or its
// ownership marker already verifies. A stamp-shaped directory created by
// anyone else must never be adopted — writing our marker into it would turn
// its foreign contents into prunable DWS backups.
function skillBackupRootUsable(root) {
  if (!pathExistsLexicallySync(root)) return true;
  // A root this very process created is ours by construction and stays
  // reusable even when its marker cannot be re-verified mid-run (for
  // example a permission failure after the first payload moved in).
  if (currentRunBackupRoots.has(root)) return true;
  return isProvenSkillBackupRoot(root);
}

// isProvenSkillBackupRoot reports whether a stamp root carries the ownership
// marker with the exact expected bytes; anything else is foreign data.
function isProvenSkillBackupRoot(root) {
  try {
    return fs.readFileSync(path.join(root, SKILL_BACKUP_MARKER_FILE), "utf8") === SKILL_BACKUP_MARKER_BODY;
  } catch {
    return false;
  }
}

// skillBackupStampPattern matches only directory names DWS itself writes:
// UTC YYYYmmdd-HHMMSS, with an optional -N collision suffix. Any other entry
// in the backup root is foreign (user data, unrelated tooling) and must be
// left untouched, so pruning is restricted to names it can prove DWS created.
const skillBackupStampPattern = /^[0-9]{8}-[0-9]{6}(-[0-9]+)?$/;

function isSkillBackupStamp(name) {
  return skillBackupStampPattern.test(name);
}

// pruneSkillBackups removes the oldest stamped backup directories once more
// than SKILL_BACKUP_KEEP remain. Only directories whose names match the DWS
// backup stamp format AND whose ownership marker verifies are candidates;
// unknown or unmarked directories are foreign data — preserved and never
// counted against SKILL_BACKUP_KEEP. Stamps sort lexicographically in
// chronological order (`YYYYmmdd-HHMMSS`), so name order is age order.
function pruneSkillBackups(homeDir) {
  const root = path.join(homeDir, ".dws", "skill-backups");
  let entries;
  try {
    entries = fs.readdirSync(root, { withFileTypes: true });
  } catch (err) {
    if (err && (err.code === "ENOENT" || err.code === "ENOTDIR")) return;
    throw err;
  }
  const names = entries
    .filter((entry) => entry.isDirectory() && isSkillBackupStamp(entry.name))
    .filter((entry) => isProvenSkillBackupRoot(path.join(root, entry.name)))
    .map((entry) => entry.name)
    .sort((a, b) => Buffer.from(a).compare(Buffer.from(b)));
  let excess = names.length - SKILL_BACKUP_KEEP;
  for (const name of names) {
    if (excess <= 0) break;
    const candidate = path.join(root, name);
    if (currentRunBackupRoots.has(candidate)) continue;
    fs.rmSync(candidate, { recursive: true, force: true });
    excess -= 1;
  }
}

function pathExistsLexicallySync(target) {
  try {
    fs.lstatSync(target);
    return true;
  } catch (err) {
    if (err && err.code === "ENOENT") return false;
    throw err;
  }
}

function copyPathLexicallySync(src, dest) {
  const info = fs.lstatSync(src);
  if (info.isDirectory()) {
    fs.mkdirSync(dest, { mode: 0o700 });
    for (const name of fs.readdirSync(src)) {
      copyPathLexicallySync(path.join(src, name), path.join(dest, name));
    }
    fs.chmodSync(dest, info.mode & 0o777);
    return;
  }
  if (info.isSymbolicLink()) {
    fs.symlinkSync(fs.readlinkSync(src), dest);
    return;
  }
  if (!info.isFile()) {
    throw new Error(`unsupported special Skill path ${src} (mode=${info.mode.toString(8)})`);
  }
  fs.copyFileSync(src, dest, fs.constants.COPYFILE_EXCL);
  fs.chmodSync(dest, info.mode & 0o777);
}

function restoreDirectoryModesSync(modes, chmodFn) {
  const failures = [];
  for (let i = modes.length - 1; i >= 0; i -= 1) {
    const item = modes[i];
    try {
      if (pathExistsLexicallySync(item.path)) chmodFn(item.path, item.mode);
    } catch (err) {
      failures.push(`restore source directory mode ${item.path}: ${err.message}`);
    }
  }
  if (failures.length > 0) throw new Error(failures.join("; "));
}

function prepareTreeRemovalSync(root, chmodFn) {
  const modes = [];
  const visit = (target) => {
    const info = fs.lstatSync(target);
    if (!info.isDirectory()) return;
    const mode = info.mode & 0o777;
    if ((mode & 0o700) !== 0o700) {
      chmodFn(target, mode | 0o700);
      modes.push({ path: target, mode });
    }
    for (const name of fs.readdirSync(target)) visit(path.join(target, name));
  };
  try {
    visit(root);
    return modes;
  } catch (err) {
    try {
      restoreDirectoryModesSync(modes, chmodFn);
    } catch (restoreErr) {
      throw new Error(`${err.message}; ${restoreErr.message}`);
    }
    throw err;
  }
}

function makeTreeWritableBestEffortSync(root) {
  try {
    const info = fs.lstatSync(root);
    if (!info.isDirectory()) return;
    try { fs.chmodSync(root, 0o700); } catch {}
    let names = [];
    try { names = fs.readdirSync(root); } catch {}
    for (const name of names) makeTreeWritableBestEffortSync(path.join(root, name));
  } catch {}
}

function removePublishedSourceSync(src, removeFn, chmodFn) {
  const modes = prepareTreeRemovalSync(src, chmodFn);
  try {
    removeFn(src);
    if (pathExistsLexicallySync(src)) throw new Error("source still exists after removal");
  } catch (err) {
    try {
      restoreDirectoryModesSync(modes, chmodFn);
    } catch (restoreErr) {
      throw new Error(`${err.message}; ${restoreErr.message}`);
    }
    throw err;
  }
}

function fileDigestSync(file) {
  return crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
}

function verifyPathCopySync(src, dest) {
  const srcInfo = fs.lstatSync(src);
  const destInfo = fs.lstatSync(dest);
  const srcType = srcInfo.mode & fs.constants.S_IFMT;
  const destType = destInfo.mode & fs.constants.S_IFMT;
  if (srcType !== destType) {
    throw new Error(`Skill path type mismatch: ${src} != ${dest}`);
  }
  if (srcInfo.isDirectory()) {
    if ((srcInfo.mode & 0o777) !== (destInfo.mode & 0o777)) {
      throw new Error(`Skill directory mode mismatch: ${src} != ${dest}`);
    }
    const bytewise = (left, right) => Buffer.from(left).compare(Buffer.from(right));
    const srcNames = fs.readdirSync(src).sort(bytewise);
    const destNames = fs.readdirSync(dest).sort(bytewise);
    if (srcNames.length !== destNames.length || srcNames.some((name, index) => name !== destNames[index])) {
      throw new Error(`Skill directory entries mismatch: ${src} != ${dest}`);
    }
    for (const name of srcNames) {
      verifyPathCopySync(path.join(src, name), path.join(dest, name));
    }
    return;
  }
  if (srcInfo.isSymbolicLink()) {
    if (fs.readlinkSync(src) !== fs.readlinkSync(dest)) {
      throw new Error(`Skill symlink target mismatch: ${src} != ${dest}`);
    }
    return;
  }
  if (!srcInfo.isFile()) {
    throw new Error(`unsupported special Skill path ${src} (mode=${srcInfo.mode.toString(8)})`);
  }
  if ((srcInfo.mode & 0o777) !== (destInfo.mode & 0o777)) {
    throw new Error(`Skill file mode mismatch: ${src} != ${dest}`);
  }
  if (srcInfo.size !== destInfo.size) {
    throw new Error(`Skill file size mismatch: ${src} != ${dest}`);
  }
  if (fileDigestSync(src) !== fileDigestSync(dest)) {
    throw new Error(`Skill file digest mismatch: ${src} != ${dest}`);
  }
}

// removeClaimIfEmptySync deletes a claimed destination only while it holds no
// entries. A foreign entry that appeared after the claim must never be
// removed: the rollback has no ownership proof for it.
function removeClaimIfEmptySync(destination) {
  try {
    fs.rmdirSync(destination);
  } catch (err) {
    if (err && (err.code === "ENOTEMPTY" || err.code === "EEXIST" || err.code === "EPERM")) {
      return;
    }
    throw err;
  }
}

function destinationExistsError(destination) {
  return Object.assign(new Error(`Skill move destination already exists: ${destination}`), { code: "EEXIST" });
}

// moveChildIntoClaimNoClobberSync moves one staged child into the claimed
// destination through an atomic no-clobber primitive: directories recursively
// claim with mkdir, regular files publish via fs.linkSync, and symlinks are
// recreated with fs.symlinkSync. Every primitive fails with EEXIST when the
// destination entry was taken concurrently, so no step can replace a foreign
// object the way a plain rename would.
function moveChildIntoClaimNoClobberSync(source, destination, fns) {
  const sourceStat = fs.lstatSync(source);
  if (sourceStat.isDirectory() && !sourceStat.isSymbolicLink()) {
    try {
      fns.mkdirFn(destination);
    } catch (err) {
      if (err && err.code === "EEXIST") throw destinationExistsError(destination);
      throw err;
    }
    moveChildrenIntoClaimSync(source, destination, sourceStat.mode & 0o777, fns);
    // The recursive move leaves an emptied shell at the source. rmdirSync
    // only deletes an empty directory, so clearing the shell keeps the
    // single-move outcome the parent's rollback expects; a shell that gained
    // a concurrent entry fails the removal and retracts the publication
    // instead of silently discarding it.
    try {
      fs.rmdirSync(source);
    } catch (err) {
      const rollbackErr = moveChildrenBackOutOfClaimSync(source, destination, fs.readdirSync(destination), fns);
      removeClaimIfEmptySync(destination);
      throw new Error(
        `Skill move state uncertain; source shell ${source} removal failed: ${err.message}` +
          (rollbackErr ? `; rollback failed: ${rollbackErr.message}` : ""),
      );
    }
    return;
  }
  if (sourceStat.isSymbolicLink()) {
    const target = fs.readlinkSync(source);
    try {
      fns.symlinkFn(target, destination);
    } catch (err) {
      if (err && err.code === "EEXIST") throw destinationExistsError(destination);
      throw err;
    }
    try {
      fs.unlinkSync(source);
    } catch (err) {
      let current = null;
      try {
        current = fs.readlinkSync(destination);
      } catch (_) {
        // Dest no longer carries our link; leave whatever replaced it.
      }
      if (current === target) fs.unlinkSync(destination);
      throw err;
    }
    return;
  }
  if (sourceStat.isFile()) {
    try {
      fns.linkFn(source, destination);
    } catch (err) {
      if (err && err.code === "EEXIST") throw destinationExistsError(destination);
      throw err;
    }
    try {
      fs.unlinkSync(source);
    } catch (err) {
      try {
        const destStat = fs.lstatSync(destination, { bigint: true });
        if (destStat.ino === sourceStat.ino && destStat.dev === sourceStat.dev) {
          fs.unlinkSync(destination);
        }
      } catch (_) {
        // Best effort; the error below names both retained paths.
      }
      throw err;
    }
    return;
  }
  throw new Error(`unsupported Skill path type for no-replace move: ${source}`);
}

// moveChildrenBackOutOfClaimSync returns the already relocated children to
// the source with plain renames — every one of them is an object this
// transaction published, so a replacing rename is safe — and removes the
// claim only while it is empty.
function moveChildrenBackOutOfClaimSync(source, destination, moved, fns) {
  let firstErr = null;
  for (let i = moved.length - 1; i >= 0; i -= 1) {
    try {
      fns.renameFn(path.join(destination, moved[i]), path.join(source, moved[i]));
    } catch (err) {
      firstErr = firstErr || err;
    }
  }
  try {
    removeClaimIfEmptySync(destination);
  } catch (err) {
    firstErr = firstErr || err;
  }
  return firstErr;
}

// moveChildrenIntoClaimSync populates an already-claimed destination from the
// staged source. The claim must still be empty when it runs (a concurrent
// writer landing between the claim and here aborts the publication with the
// destination retained), every child moves through an atomic no-clobber
// primitive, and a live re-read catches a different-named foreign entry that
// landed mid-move. Any failure returns the moved children to the source and
// removes the claim only while it is empty, so foreign data always survives.
function moveChildrenIntoClaimSync(source, destination, sourceMode, fns) {
  const claimEntries = fs.readdirSync(destination);
  if (claimEntries.length > 0) {
    throw destinationExistsError(destination);
  }
  const entries = fs.readdirSync(source);
  const moved = [];
  for (const name of entries) {
    try {
      moveChildIntoClaimNoClobberSync(path.join(source, name), path.join(destination, name), fns);
    } catch (err) {
      const rollbackErr = moveChildrenBackOutOfClaimSync(source, destination, moved, fns);
      if (rollbackErr) {
        throw new Error(`${err.message}; rollback failed: ${rollbackErr.message}`);
      }
      throw err;
    }
    moved.push(name);
  }
  const live = fs.readdirSync(destination);
  if (live.length > entries.length) {
    const abortErr = destinationExistsError(destination);
    const rollbackErr = moveChildrenBackOutOfClaimSync(source, destination, moved, fns);
    if (rollbackErr) {
      throw new Error(`${abortErr.message}; rollback failed: ${rollbackErr.message}`);
    }
    throw abortErr;
  }
  fns.chmodFn(destination, sourceMode);
}

function defaultClaimFns(renameFn) {
  return {
    mkdirFn: (target) => fs.mkdirSync(target, { mode: 0o700 }),
    renameFn: renameFn || fs.renameSync,
    linkFn: fs.linkSync,
    symlinkFn: fs.symlinkSync,
    chmodFn: fs.chmodSync,
  };
}

function renamePathNoReplaceSync(source, destination, renameFn = fs.renameSync) {
  const sourceStat = fs.lstatSync(source);
  if (sourceStat.isDirectory() && !sourceStat.isSymbolicLink()) {
    const fns = defaultClaimFns(renameFn);
    try {
      fns.mkdirFn(destination);
    } catch (err) {
      if (err && err.code === "EEXIST") {
        throw destinationExistsError(destination);
      }
      throw err;
    }
    moveChildrenIntoClaimSync(source, destination, sourceStat.mode & 0o777, fns);
    try {
      fs.rmdirSync(source);
    } catch (err) {
      const rollbackErr = moveChildrenBackOutOfClaimSync(source, destination, fs.readdirSync(destination), fns);
      if (rollbackErr) {
        throw new Error(`Skill move state uncertain; source ${source} and dest ${destination} retained: ${err.message}; retract failed: ${rollbackErr.message}`);
      }
      throw err;
    }
    return;
  }
  if (sourceStat.isSymbolicLink()) {
    try {
      fs.symlinkSync(fs.readlinkSync(source), destination);
    } catch (err) {
      if (err && err.code === "EEXIST") {
        throw destinationExistsError(destination);
      }
      throw err;
    }
    try {
      fs.unlinkSync(source);
    } catch (err) {
      try {
        fs.unlinkSync(destination);
      } catch (retractErr) {
        throw new Error(`Skill move state uncertain; source ${source} and dest ${destination} retained: ${err.message}; retract failed: ${retractErr.message}`);
      }
      throw err;
    }
    return;
  }
  if (sourceStat.isFile()) {
    try {
      fs.linkSync(source, destination);
    } catch (err) {
      if (err && err.code === "EEXIST") {
        throw destinationExistsError(destination);
      }
      throw err;
    }
    try {
      fs.unlinkSync(source);
    } catch (err) {
      try {
        const destStat = fs.lstatSync(destination);
        if (destStat.ino === sourceStat.ino && destStat.dev === sourceStat.dev) {
          fs.unlinkSync(destination);
        }
      } catch (retractErr) {
        throw new Error(`Skill move state uncertain; source ${source} and dest ${destination} retained: ${err.message}; retract failed: ${retractErr.message}`);
      }
      throw err;
    }
    return;
  }
  throw new Error(`unsupported Skill path type for no-replace move: ${source}`);
}

function recordSkillPathPublicationSync(destination) {
  const stat = fs.lstatSync(destination, { bigint: true });
  return {
    destination,
    device: stat.dev.toString(),
    inode: stat.ino.toString(),
    fingerprint: skillPathFingerprint(destination),
  };
}

function retractUnconfirmedSkillPublicationSync(src, dest, publication, cause, options = {}) {
  try {
    rollbackPublishedSkillPath(publication, {
      quarantineRenameFn: options.quarantineRenameFn,
      restoreRenameFn: options.restoreRenameFn || options.renameFn,
      removeFn: options.removeFn,
    });
  } catch (retractErr) {
    throw new Error(`Skill move state uncertain: ${cause.message}; failed to retract ${dest}: ${retractErr.message}; source ${src} and dest ${dest} retained`);
  }
  throw new Error(`Skill move failed, dest retracted, source retained (${src}): ${cause.message}`);
}

function movePathRecoverablySync(src, dest, options = {}) {
  const renameFn = options.renameFn || ((source, target) => renamePathNoReplaceSync(source, target));
  const copyFn = options.copyFn || copyPathLexicallySync;
  const verifyFn = options.verifyFn || verifyPathCopySync;
  const removeFn = options.removeFn || ((target) => fs.rmSync(target, { recursive: true, force: true }));
  const mkdirTempFn = options.mkdirTempFn || fs.mkdtempSync;
  const chmodFn = options.chmodFn || fs.chmodSync;
  if (pathExistsLexicallySync(dest)) {
    throw new Error(`move destination already exists: ${dest}`);
  }
  fs.mkdirSync(path.dirname(dest), { recursive: true });
  try {
    renameFn(src, dest);
    return;
  } catch (err) {
    if (!err || err.code !== "EXDEV") throw err;
  }

  const stageRoot = mkdirTempFn(path.join(path.dirname(dest), `.${path.basename(dest)}.cross-device-`));
  const stage = path.join(stageRoot, "payload");
  let stageCleaned = false;
  let destOccupied = false;
  try {
    copyFn(src, stage);
    verifyFn(src, stage);
    const stageInfo = fs.lstatSync(stage);
    const stageMode = stageInfo.mode & 0o777;
    if (stageInfo.isDirectory()) chmodFn(stage, stageMode | 0o700);
    // Dest publication is always no-replace. The injected renameFn is the
    // same-volume EXDEV probe; using it here would re-open a replacing
    // rename on Windows (MOVEFILE_REPLACE_EXISTING).
    renamePathNoReplaceSync(stage, dest, renameFn);
    destOccupied = true;
    let publication;
    try {
      publication = recordSkillPathPublicationSync(dest);
    } catch (recErr) {
      if (!pathExistsLexicallySync(dest)) throw recErr;
      throw new Error(`Skill move state uncertain: published dest ${dest} could not be recorded: ${recErr.message}; source ${src} and dest ${dest} retained`);
    }
    try {
      if (stageInfo.isDirectory()) {
        chmodFn(dest, stageMode);
        publication = recordSkillPathPublicationSync(dest);
      }
      verifyFn(src, dest);
      makeTreeWritableBestEffortSync(stageRoot);
      removeFn(stageRoot);
      stageCleaned = true;
    } catch (postErr) {
      retractUnconfirmedSkillPublicationSync(src, dest, publication, postErr, {
        removeFn,
      });
    }
    try {
      removePublishedSourceSync(src, removeFn, chmodFn);
    } catch (err) {
      throw new Error(`verified target published but source removal failed; both copies retained (${src}, ${dest}): ${err.message}`);
    }
    if (pathExistsLexicallySync(src)) {
      throw new Error(`verified target published but source still exists; both copies retained (${src}, ${dest})`);
    }
  } catch (err) {
    if (!stageCleaned) {
      try {
        makeTreeWritableBestEffortSync(stageRoot);
        removeFn(stageRoot);
        stageCleaned = true;
      } catch (cleanupErr) {
        if (!destOccupied) {
          throw new Error(`${err.message}; cross-device staging cleanup failed: ${cleanupErr.message}`);
        }
      }
    }
    throw err;
  }
}

function skillMoveOptions(options = {}) {
  return {
    renameFn: options.renameFn,
    copyFn: options.backupCopyFn,
    verifyFn: options.backupVerifyFn,
    removeFn: options.backupRemoveFn,
    mkdirTempFn: options.backupMkdirTempFn,
  };
}

// backupAndRemoveSkillDir moves dir into <homeDir>/.dws/skill-backups/
// <stamp>/<rel-or-basename> instead of destroying it (non-interactive
// installs cannot confirm, so removals must stay reversible). Missing paths
// are a no-op success. On any backup failure the directory is left in place
// and an error is returned so callers cannot silently delete data.
function backupAndRemoveSkillDir(homeDir, dir, backups = null, options = {}) {
  try {
    fs.lstatSync(dir);
  } catch (err) {
    if (err && err.code === "ENOENT") return "";
    throw err;
  }
  const rel = path.relative(homeDir, dir);
  const name =
    rel && rel !== "." && !rel.startsWith("..") && !path.isAbsolute(rel)
      ? rel.split(path.sep).join("-")
      : path.basename(dir);
  const stamp = (options.backupStampFn || backupStamp)();
  const backupRoot = path.join(homeDir, ".dws", "skill-backups");
  let targetRoot = path.join(backupRoot, stamp);
  let target = path.join(targetRoot, name);
  // Bump not only when the payload path is taken but also when the stamp
  // root exists without a verified ownership marker: a same-second foreign
  // directory must never be stamped DWS-owned and made prunable. A
  // marker-verified root from this run's same second stays reusable.
  for (let i = 1; pathExistsLexicallySync(target) || !skillBackupRootUsable(targetRoot); i++) {
    if (i > 1000) {
      throw new Error(`backup directory collision limit exceeded; source retained: ${dir}`);
    }
    targetRoot = path.join(backupRoot, `${stamp}-${i}`);
    target = path.join(targetRoot, name);
  }
  try {
    fs.mkdirSync(targetRoot, { recursive: true });
    // Stamp ownership immediately after creating the stamp root and before
    // the skill directory moves into it, so an interrupted backup can never
    // leave an unmarked (never-prunable) stamp behind.
    writeSkillBackupMarker(targetRoot);
  } catch (err) {
    // Never leave an empty unowned stamp root behind. rmdirSync removes an
    // empty root but deliberately fails on a non-empty one, so a pre-existing
    // root holding foreign data is never destroyed.
    try {
      fs.rmdirSync(targetRoot);
    } catch {
      // Non-empty pre-existing root: foreign data stays.
    }
    console.warn(`⚠️  备份失败，保留原目录 ${dir}: ${err.message}`);
    throw new Error(`failed to back up Skill directory ${dir}: ${err.message}`);
  }
  try {
    movePathRecoverablySync(dir, target, options);
  } catch (err) {
    console.warn(`⚠️  备份失败，保留原目录 ${dir}: ${err.message}`);
    throw new Error(`failed to back up Skill directory ${dir}: ${err.message}`);
  }
  if (backups) {
    backups.push({ original: dir, backup: target });
  }
  currentRunBackupRoots.add(targetRoot);
  // Keep the backup root bounded. The backup itself already succeeded, so a
  // prune failure is only a warning (matches Go's best-effort prune call).
  try {
    pruneSkillBackups(homeDir);
  } catch (err) {
    console.warn(`⚠️  旧备份清理失败（备份本身已成功）: ${err.message}`);
  }
  console.log(`  × 已备份并移除 ${dir} → ${target}`);
  return target;
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
    const runtimeSource = path.join(path.dirname(binaryPath), ".dws-runtime");
    if (!fs.existsSync(path.join(runtimeSource, "20260825", "manifest.json"))) {
      throw new Error(`runtime payload not found in archive ${archivePath}`);
    }
    const targetName = process.platform === "win32" ? "dws.exe" : "dws";
    const targetPath = path.join(destDir, targetName);
    fs.copyFileSync(binaryPath, targetPath);
    fs.cpSync(runtimeSource, path.join(destDir, ".dws-runtime"), { recursive: true, force: true });
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
        if ((entry.isDirectory() || entry.isSymbolicLink()) && isManagedMultiSkillDir(path.join(baseDir, entry.name), managedNames)) {
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

  const canonicalBase = path.join(homeDir, ".agents", "skills");
  attempted += 1;
  if (installToBase(canonicalBase)) {
    installed += 1;
  } else {
    failed += 1;
  }

  if (installed > 0) {
    for (const target of resolvedAgentTargets(homeDir)) {
      const { agentDir, baseDir, universal } = target;
      if (!agentTargetDetected(target) || samePhysicalDir(baseDir, canonicalBase)) continue;
      attempted += 1;
      if (universal) {
        // Cleanup-only migration: universal Agents read the canonical store
        // directly, so nothing is installed here. A retire failure leaves an
        // obsolete copy in place — worth a warning, never a reason to fail an
        // install whose canonical store and links all succeeded.
        try {
          retireManagedSkillRoot(homeDir, baseDir, managedNames);
        } catch (err) {
          console.warn(`⚠️  Agent Skill 旧副本迁移失败（不影响本次安装）${baseDir}: ${err.message}`);
        }
        continue;
      }
      const victims = [path.join(baseDir, "dws")];
      if (fs.existsSync(baseDir)) {
        for (const entry of fs.readdirSync(baseDir, { withFileTypes: true })) {
          if ((entry.isDirectory() || entry.isSymbolicLink()) && isManagedMultiSkillDir(path.join(baseDir, entry.name), managedNames)) {
            victims.push(path.join(baseDir, entry.name));
          }
        }
      }
      try {
        publishCanonicalLinksAtomically(homeDir, canonicalBase, baseDir, ["dws"], victims);
        installed += 1;
      } catch (linkErr) {
        if (installToBase(baseDir)) {
          console.log(`ℹ️  ${baseDir} 已自动使用兼容方式安装，可正常使用`);
          installed += 1;
        } else failed += 1;
      }
    }
  }
  if (installed === 0) {
    throw new Error("未安装任何 mono Skill：所有检测到的 Agent 目标均失败");
  }
  if (failed > 0) {
    throw new Error(`有 ${failed} 个 Agent 目标安装 mono Skill 失败`);
  }
  fs.rmSync(path.join(skillStateDir(homeDir), "skills-state.json"), { force: true });
  console.log(`✅ DWS Skills 安装完成`);
  console.log(`   统一安装位置：${canonicalBase}`);
  console.log(`   已自动适配本机上检测到的 Agent`);
  console.log(`ℹ️  下一步：请重启已打开的 Agent，使新 Skills 生效`);
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

function retireManagedSkillRoot(homeDir, baseDir, managedNames, options = {}) {
  const moveOptions = skillMoveOptions(options);
  const victims = [path.join(baseDir, "dws")];
  if (fs.existsSync(baseDir)) {
    for (const entry of fs.readdirSync(baseDir, { withFileTypes: true })) {
      if ((entry.isDirectory() || entry.isSymbolicLink()) && isManagedMultiSkillDir(path.join(baseDir, entry.name), managedNames)) {
        victims.push(path.join(baseDir, entry.name));
      }
    }
  }
  const backups = [];
  try {
    for (const victim of victims) {
      backupAndRemoveSkillDir(homeDir, victim, backups, moveOptions);
    }
  } catch (err) {
    const restoreErrors = [];
    for (let i = backups.length - 1; i >= 0; i -= 1) {
      try {
        fs.mkdirSync(path.dirname(backups[i].original), { recursive: true });
        movePathRecoverablySync(backups[i].backup, backups[i].original, moveOptions);
      } catch (restoreErr) {
        restoreErrors.push(`${backups[i].original}: ${restoreErr.message}`);
      }
    }
    if (restoreErrors.length > 0) {
      throw new Error(`${err.message}; Agent-root rollback failed: ${restoreErrors.join("; ")}`);
    }
    throw err;
  }
}

function samePhysicalDir(left, right) {
  try {
    const leftReal = fs.realpathSync(left);
    const rightReal = fs.realpathSync(right);
    return process.platform === "win32" ? leftReal.toLowerCase() === rightReal.toLowerCase() : leftReal === rightReal;
  } catch (_) {
    return false;
  }
}

function writeSkillFingerprintField(hash, value) {
  const bytes = Buffer.from(String(value), "utf8");
  hash.update(String(bytes.length), "utf8");
  hash.update(":", "utf8");
  hash.update(bytes);
}

function skillPathFingerprint(target) {
  const hash = crypto.createHash("sha256");
  const visit = (current) => {
    const stat = fs.lstatSync(current, { bigint: true });
    writeSkillFingerprintField(hash, Number(stat.mode & 0o170000n));
    writeSkillFingerprintField(hash, Number(stat.mode & 0o777n));
    if (stat.isDirectory()) {
      const names = fs.readdirSync(current).sort((a, b) => Buffer.from(a).compare(Buffer.from(b)));
      writeSkillFingerprintField(hash, names.length);
      for (const name of names) {
        writeSkillFingerprintField(hash, name);
        visit(path.join(current, name));
      }
      return;
    }
    if (stat.isSymbolicLink()) {
      writeSkillFingerprintField(hash, fs.readlinkSync(current));
      return;
    }
    if (stat.isFile()) {
      writeSkillFingerprintField(hash, stat.size.toString());
      hash.update(fs.readFileSync(current));
      return;
    }
    throw new Error(`unsupported Skill path type: ${current}`);
  };
  visit(target);
  return hash.digest("hex");
}

function skillPublicationMatches(target, publication) {
  const stat = fs.lstatSync(target, { bigint: true });
  return stat.dev.toString() === publication.device
    && stat.ino.toString() === publication.inode
    && skillPathFingerprint(target) === publication.fingerprint;
}

function removeSkillPathLexically(target) {
  const stat = fs.lstatSync(target);
  if (stat.isDirectory() && !stat.isSymbolicLink()) {
    fs.rmSync(target, { recursive: true, force: true });
  } else {
    fs.unlinkSync(target);
  }
}

function skillPathExistsLexically(target) {
  try {
    fs.lstatSync(target);
    return true;
  } catch (err) {
    if (err && err.code === "ENOENT") return false;
    throw err;
  }
}

function rollbackPublishedSkillPath(publication, options = {}) {
  // Prove ownership at dest first. Quarantining before that check would move
  // a concurrent replacement off its original path; if the later match fails
  // and restore cannot republish, user data stays hidden under .rollback-*.
  // Quarantine itself is a same-inode rename — a no-replace mkdir-claim
  // would mint a new identity and make this transaction's own dest look
  // foreign. A post-quarantine mismatch is restored with no-replace.
  const quarantineRenameFn = options.quarantineRenameFn || fs.renameSync;
  const restoreRenameFn = options.restoreRenameFn || options.renameFn || ((source, target) => renamePathNoReplaceSync(source, target));
  const removeFn = options.removeFn || removeSkillPathLexically;
  const destination = publication.destination;
  const quarantineRoot = fs.mkdtempSync(path.join(path.dirname(destination), `.${path.basename(destination)}.rollback-`));
  const quarantine = path.join(quarantineRoot, "payload");
  const cleanupRoot = () => {
    try {
      fs.rmSync(quarantineRoot, { recursive: true, force: true });
    } catch (cleanupErr) {
      throw new Error(`failed to clean Skill rollback quarantine ${quarantineRoot}: ${cleanupErr.message}`);
    }
  };
  try {
    if (!skillPublicationMatches(destination, publication)) {
      cleanupRoot();
      throw new Error(`refusing to delete non-transaction Skill ${destination}: concurrent object identity changed`);
    }
  } catch (err) {
    if (err && err.code === "ENOENT") {
      // ENOENT from the fingerprint walk means the destination itself is
      // gone only when it no longer exists lexically; a concurrently removed
      // CHILD produces the same error and must fall through to the refusal
      // below instead of skipping rollback.
      if (!pathExistsLexicallySync(destination)) {
        cleanupRoot();
        return;
      }
    }
    if (String(err.message || "").startsWith("refusing to delete non-transaction Skill")
        || String(err.message || "").startsWith("failed to clean Skill rollback quarantine")) {
      throw err;
    }
    cleanupRoot();
    throw new Error(`refusing to delete unverifiable Skill ${destination}: ${err.message}`);
  }

  try {
    quarantineRenameFn(destination, quarantine);
  } catch (err) {
    cleanupRoot();
    if (err && err.code === "ENOENT") return;
    throw new Error(`failed to quarantine published Skill ${destination}: ${err.message}`);
  }
  let owned = false;
  try {
    owned = skillPublicationMatches(quarantine, publication);
  } catch (err) {
    try {
      restoreRenameFn(quarantine, destination);
    } catch (restoreErr) {
      throw new Error(`refusing to delete non-transaction Skill ${destination}: ${err.message}; concurrent object retained at ${quarantine}; restore failed: ${restoreErr.message}`);
    }
    cleanupRoot();
    throw new Error(`refusing to delete non-transaction Skill ${destination}: ${err.message}`);
  }
  if (!owned) {
    try {
      restoreRenameFn(quarantine, destination);
    } catch (restoreErr) {
      throw new Error(`refusing to delete non-transaction Skill ${destination}: concurrent object identity changed; concurrent object retained at ${quarantine}; restore failed: ${restoreErr.message}`);
    }
    cleanupRoot();
    throw new Error(`refusing to delete non-transaction Skill ${destination}: concurrent object identity changed`);
  }
  removeFn(quarantine);
  cleanupRoot();
}

// Publish a staged Skill directory with a truly atomic no-replace claim.
// fs.mkdirSync fails with EEXIST when anything occupies the destination, so
// the claim itself is the existence check — there is no check-then-rename
// window a concurrent file, symlink, or empty directory could slip through
// (fs.renameSync replaces the destination outright on every platform,
// including Windows where libuv passes MOVEFILE_REPLACE_EXISTING). Children
// then move into the claimed directory one by one; each child rename targets
// a path inside a directory this transaction owns, so no step can replace a
// foreign object. The publication identity is the claim's: it is captured
// after the claim succeeds and re-verified by the caller once the move
// completes. On a failed child move the relocated children are moved back and
// only the (now empty) claim is removed, so the destination returns to the
// foreign object or to nothing — never to a partial publication.
function publishSkillDirNoReplace(staged, destination, options = {}) {
  const fns = {
    mkdirFn: options.publishMkdirFn || ((target) => fs.mkdirSync(target, { mode: 0o700 })),
    renameFn: options.publishRenameFn || fs.renameSync,
    linkFn: options.publishLinkFn || fs.linkSync,
    symlinkFn: options.publishSymlinkFn || fs.symlinkSync,
    chmodFn: options.publishChmodFn || fs.chmodSync,
  };
  const sourceStat = fs.lstatSync(staged);
  try {
    fns.mkdirFn(destination);
  } catch (err) {
    if (err && err.code === "EEXIST") {
      throw Object.assign(new Error(`Skill publish destination already exists: ${destination}`), { code: "EEXIST" });
    }
    throw err;
  }
  const claimStat = fs.lstatSync(destination, { bigint: true });
  const publication = {
    destination,
    device: claimStat.dev.toString(),
    inode: claimStat.ino.toString(),
    fingerprint: skillPathFingerprint(staged),
  };
  moveChildrenIntoClaimSync(staged, destination, sourceStat.mode & 0o7777, fns);
  return publication;
}

// Publish a staged canonical link by creating the link directly at the
// destination. Link creation is the atomic no-replace primitive on every
// platform: the kernel refuses with EEXIST when anything — file, symlink, or
// directory — already occupies the path, and unlike a rename (which Windows
// performs with MOVEFILE_REPLACE_EXISTING) or `ln -P source target` (which
// links INTO a directory that appeared at the target) it has no
// check-then-publish window and never treats the destination as a container.
// Staging still proves beforehand that links can be created on this
// filesystem, before any victim moves. The publication identity is captured
// from the destination after creation — no primitive carries the staged
// inode across this publish — and confirmation re-reads the live path, so a
// concurrent replacement surfaces before the publication enters the rollback
// list instead of being recorded as owned.
function publishCanonicalLinkNoReplace(staged, destination, options = {}) {
  const linkTarget = fs.readlinkSync(staged);
  const linkType = process.platform === "win32" ? "junction" : "dir";
  const symlinkFn = options.publishSymlinkFn || fs.symlinkSync;
  try {
    symlinkFn(linkTarget, destination, linkType);
  } catch (err) {
    if (err && err.code === "EEXIST") {
      throw Object.assign(new Error(`Skill publish destination already exists: ${destination}`), { code: "EEXIST" });
    }
    throw err;
  }
  let publication;
  try {
    publication = recordSkillPathPublicationSync(destination);
  } catch (recErr) {
    if (!skillPathExistsLexically(destination)) throw recErr;
    throw new Error(`Skill publish state uncertain: dest ${destination} occupied but unrecorded: ${recErr.message}`);
  }
  let destStat;
  try {
    destStat = fs.lstatSync(destination, { bigint: true });
  } catch (err) {
    throw new Error(`published Skill identity changed before confirmation: ${destination}: ${err.message}`);
  }
  if (!destStat.isSymbolicLink() || fs.readlinkSync(destination) !== linkTarget) {
    // Dest is not the link this transaction created. Leave the occupant.
    throw new Error(`published Skill identity changed before confirmation: ${destination}`);
  }
  const sameInode = publication
    && destStat.dev.toString() === publication.device
    && destStat.ino.toString() === publication.inode;
  if (!publication || !sameInode || !skillPublicationMatches(destination, publication)) {
    if (sameInode) {
      try {
        rollbackPublishedSkillPath(publication);
      } catch (retractErr) {
        throw new Error(`Skill publish state uncertain: published dest ${destination} confirmation failed; retract failed: ${retractErr.message}`);
      }
    }
    throw new Error(`published Skill identity changed before confirmation: ${destination}`);
  }
  return publication;
}

function publishCanonicalLinksAtomically(homeDir, canonicalBase, baseDir, names, victims, options = {}) {
  const moveOptions = skillMoveOptions(options);
  const publishRemoveFn = options.publishRemoveFn || removeSkillPathLexically;
  const rollbackRenameFn = options.rollbackRenameFn || ((source, target) => renamePathNoReplaceSync(source, target));
  // Injectable so the Windows junction type and absolute link target can be
  // asserted from a non-Windows host.
  const symlinkFn = options.symlinkFn || fs.symlinkSync;
  fs.mkdirSync(baseDir, { recursive: true });
  const realBaseDir = fs.realpathSync(baseDir);
  const stageRoot = fs.mkdtempSync(path.join(baseDir, ".dws-link-set.tmp-"));
  const staged = [];
  const backups = [];
  const published = [];
  const correctLinks = new Set();
  const restore = () => {
    const restoreErrors = [];
    for (let i = published.length - 1; i >= 0; i -= 1) {
      try {
        rollbackPublishedSkillPath(published[i], { restoreRenameFn: rollbackRenameFn, removeFn: publishRemoveFn });
      } catch (err) {
        restoreErrors.push(`remove ${published[i].destination}: ${err.message}`);
      }
    }
    for (let i = backups.length - 1; i >= 0; i -= 1) {
      try {
        fs.mkdirSync(path.dirname(backups[i].original), { recursive: true });
        movePathRecoverablySync(backups[i].backup, backups[i].original, moveOptions);
      } catch (err) {
        restoreErrors.push(`restore ${backups[i].original} from ${backups[i].backup}: ${err.message}`);
      }
    }
    if (restoreErrors.length > 0) {
      throw new Error(restoreErrors.join("; "));
    }
  };
  try {
    for (const name of names) {
      const target = path.join(canonicalBase, name);
      const dest = path.join(baseDir, name);
      if (samePhysicalDir(dest, target)) {
        correctLinks.add(path.resolve(dest));
        continue;
      }
      const stagedPath = path.join(stageRoot, name);
      const realTarget = fs.realpathSync(target);
      const linkTarget = process.platform === "win32" ? realTarget : path.relative(realBaseDir, realTarget);
      symlinkFn(linkTarget, stagedPath, process.platform === "win32" ? "junction" : "dir");
      staged.push({ staged: stagedPath, dest: path.join(baseDir, name) });
    }
    const seen = new Set();
    for (const victim of victims) {
      const normalized = path.resolve(victim);
      if (seen.has(normalized) || correctLinks.has(normalized)) continue;
      seen.add(normalized);
      backupAndRemoveSkillDir(homeDir, victim, backups, moveOptions);
    }
    for (const item of staged) {
      const publication = publishCanonicalLinkNoReplace(item.staged, item.dest, options);
      published.push(publication);
    }
  } catch (err) {
    try {
      restore();
    } catch (restoreErr) {
      throw new Error(`${err.message}; rollback failed: ${restoreErr.message}`);
    }
    throw err;
  } finally {
    fs.rmSync(stageRoot, { recursive: true, force: true });
  }
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
  const removeFn = options.removeFn || ((dir) => fs.rmSync(dir, { recursive: true, force: true }));
  const rollbackRenameFn = options.rollbackRenameFn || ((source, target) => renamePathNoReplaceSync(source, target));
  const moveOptions = skillMoveOptions(options);
  fs.mkdirSync(baseDir, { recursive: true });
  const stageRoot = fs.mkdtempSync(path.join(baseDir, ".dws-multi-set.tmp-"));
  const staged = [];
  const backups = [];
  const published = [];

  const restore = () => {
    const restoreErrors = [];
    for (let i = published.length - 1; i >= 0; i -= 1) {
      try {
        rollbackPublishedSkillPath(published[i], { restoreRenameFn: rollbackRenameFn, removeFn });
      } catch (err) {
        restoreErrors.push(`remove ${published[i].destination}: ${err.message}`);
      }
    }
    for (let i = backups.length - 1; i >= 0; i -= 1) {
      const item = backups[i];
      try {
        fs.mkdirSync(path.dirname(item.original), { recursive: true });
        movePathRecoverablySync(item.backup, item.original, moveOptions);
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
      backupAndRemoveSkillDir(homeDir, victim, backups, moveOptions);
    }

    for (const item of staged) {
      const publication = publishSkillDirNoReplace(item.staged, item.dest, options);
      if (!skillPublicationMatches(item.dest, publication)) {
        let live = null;
        try {
          live = fs.lstatSync(item.dest, { bigint: true });
        } catch (_) {
          live = null;
        }
        if (live && live.dev.toString() === publication.device && live.ino.toString() === publication.inode) {
          try {
            rollbackPublishedSkillPath(publication, { restoreRenameFn: rollbackRenameFn, removeFn });
          } catch (retractErr) {
            throw new Error(`Skill publish state uncertain: dest ${item.dest} confirmation failed; retract failed: ${retractErr.message}`);
          }
        }
        throw new Error(`published Skill identity changed before confirmation: ${item.dest}`);
      }
      published.push(publication);
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
  const removeFn = options.removeFn || ((dir) => fs.rmSync(dir, { recursive: true, force: true }));
  const rollbackRenameFn = options.rollbackRenameFn || ((source, target) => renamePathNoReplaceSync(source, target));
  const moveOptions = skillMoveOptions(options);
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
        rollbackPublishedSkillPath(published[i], { restoreRenameFn: rollbackRenameFn, removeFn });
      } catch (err) {
        restoreErrors.push(`remove ${published[i].destination}: ${err.message}`);
      }
    }
    for (let i = backups.length - 1; i >= 0; i -= 1) {
      const item = backups[i];
      try {
        fs.mkdirSync(path.dirname(item.original), { recursive: true });
        movePathRecoverablySync(item.backup, item.original, moveOptions);
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
      backupAndRemoveSkillDir(homeDir, victim, backups, moveOptions);
    }

    const publication = publishSkillDirNoReplace(stagedDir, destDir, options);
    if (!skillPublicationMatches(destDir, publication)) {
      let live = null;
      try {
        live = fs.lstatSync(destDir, { bigint: true });
      } catch (_) {
        live = null;
      }
      if (live && live.dev.toString() === publication.device && live.ino.toString() === publication.inode) {
        try {
          rollbackPublishedSkillPath(publication, { restoreRenameFn: rollbackRenameFn, removeFn });
        } catch (retractErr) {
          throw new Error(`Skill publish state uncertain: dest ${destDir} confirmation failed; retract failed: ${retractErr.message}`);
        }
      }
      throw new Error(`published Skill identity changed before confirmation: ${destDir}`);
    }
    published.push(publication);
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
        (entry.isDirectory() || entry.isSymbolicLink()) &&
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

  const canonicalBase = path.join(homeDir, ".agents", "skills");
  attempted += 1;
  if (installToBase(canonicalBase)) installed += 1;
  else failed += 1;

  if (installed > 0) {
    for (const target of resolvedAgentTargets(homeDir)) {
      const { agentDir, baseDir, universal } = target;
      if (!agentTargetDetected(target) || samePhysicalDir(baseDir, canonicalBase)) continue;
      attempted += 1;
      if (universal) {
        // Cleanup-only migration (see installSkillsToHomes): nothing is
        // installed in a universal root, so a retire failure must not fail an
        // otherwise complete install. Go records the same case as a failed
        // directory yet still returns success for the run.
        try {
          retireManagedSkillRoot(homeDir, baseDir, managedNames);
        } catch (err) {
          console.warn(`⚠️  Agent Skill 旧副本迁移失败（不影响本次安装）${baseDir}: ${err.message}`);
        }
        continue;
      }
      const victims = [path.join(baseDir, "dws")];
      if (fs.existsSync(baseDir)) {
        for (const entry of fs.readdirSync(baseDir, { withFileTypes: true })) {
          if (
            (entry.isDirectory() || entry.isSymbolicLink()) &&
            (LEGACY_OFFICIAL_MULTI_SKILLS.has(entry.name) || managedNames.has(entry.name)) &&
            !skillSet.has(entry.name)
          ) victims.push(path.join(baseDir, entry.name));
        }
      }
      for (const name of skills) victims.push(path.join(baseDir, name));
      try {
        publishCanonicalLinksAtomically(homeDir, canonicalBase, baseDir, skills, victims);
        installed += 1;
      } catch (linkErr) {
        if (installToBase(baseDir)) {
          console.log(`ℹ️  ${baseDir} 已自动使用兼容方式安装，可正常使用`);
          installed += 1;
        } else failed += 1;
      }
    }
  }
  if (installed === 0) {
    throw new Error("未安装任何 multi Skill：所有检测到的 Agent 目标均失败");
  }
  if (failed > 0) {
    throw new Error(`有 ${failed} 个 Agent 目标安装 multi Skill 失败`);
  }
  writeSkillsState(homeDir, multiRoot, skills);
  console.log(`✅ DWS Skills 安装完成`);
  console.log(`   统一安装位置：${canonicalBase}`);
  console.log(`   已自动适配本机上检测到的 Agent`);
  console.log(`ℹ️  下一步：请重启已打开的 Agent，使新 Skills 生效`);
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
  UPSTREAM_AGENTS,
  resolvedAgentTargets,
  agentTargetDetected,
  isSkillBackupStamp,
  pruneSkillBackups,
  backupAndRemoveSkillDir,
  copyPathLexicallySync,
  movePathRecoverablySync,
  publishCacheAtomically,
  publishCanonicalLinksAtomically,
  publishManagedMonoSkillSetAtomically,
  publishManagedMultiSkillSetAtomically,
  recordSkillPathPublicationSync,
  rollbackPublishedSkillPath,
  verifyPathCopySync,
};
