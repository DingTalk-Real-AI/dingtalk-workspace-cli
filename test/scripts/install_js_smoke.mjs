#!/usr/bin/env node
/**
 * install_js_smoke.mjs — smoke test for build/npm/install.js (npm postinstall).
 *
 * Runs the REAL build/npm/install.js against a staged fake package:
 *
 *   <tmp>/pkg/
 *     install.js                 (copied from build/npm/install.js)
 *     assets/
 *       dws-<os>-<arch>.tar.gz   (dummy archive holding a fake `dws` binary)
 *       dws-skills.zip           (tiny release-layout fixture built on the fly,
 *                                 NOT the real skills/ tree)
 *
 * Scenarios (each with an isolated fake HOME):
 *   1. multi install        — dingtalk-* and dws-shared land as sibling
 *                             skills, mono leftover dws/ and stale
 *                             dingtalk-* are removed, and the
 *                             ~/.dws/skills/{multi,mono} caches fill.
 *   2. universal topology   — canonical store installed, pre-existing private
 *                             copies retired and recoverable under
 *                             ~/.dws/skill-backups/<stamp>/<home-rel-name>.
 *   3. second run           — idempotent: an already-correct link is neither
 *                             re-backed-up nor rewritten.
 *   4. unretireable universal root — a cleanup-only failure WARNS; canonical
 *                             store, non-universal links and skills-state.json
 *                             are all still written and the install exits 0.
 *   5. empty multi/ tree    — warns and falls back to mono instead of
 *                             crashing postinstall; a previously good multi
 *                             cache is NOT wiped.
 *   6. bogus mode           — DWS_SKILL_MODE=bogus exits non-zero with an
 *                             "invalid DWS_SKILL_MODE" error.
 *   7. multi-only zip, mono — mono install is skipped with a warning; the
 *                             staging root is NOT copied into a dws/ dir.
 *   8. multi backup failure — preserves mono, writes no multi skill, and
 *                             reports postinstall failure.
 *   9. mono backup failure  — preserves multi, writes no mono skill, and
 *                             reports postinstall failure.
 *  10. mono switch          — migrates only centrally owned multi Skills.
 *  11. cache copy failure   — preserves the previous complete cache.
 *  12. no-replace publish   — an occupied destination that is not a victim is
 *                             refused, not replaced or linked into.
 *  13. simulated win32      — junction type + ABSOLUTE canonical target and
 *                             the rename publish path with its no-replace
 *                             guard, exercised by swapping process.platform
 *                             instead of requiring a Windows host.
 *  14. backup retention     — ~/.dws/skill-backups keeps the newest 5 stamps.
 *  15. unknown preservation — prune never removes non-DWS directories in skill-backups.
 *  16. multi publish failure — restores the complete previous Skill set.
 *  17. multi backup failure  — restores every earlier successful backup.
 *  18. mono transaction failure — restores every managed multi Skill after
 *                                 later backup or mono publish failure.
 *  19. copy publish no-replace  — mono/multi set publication refuses a
 *                                 concurrently occupied destination (atomic
 *                                 claim, no check-then-rename window) on
 *                                 POSIX and simulated win32.
 *  20. unmarked stamps       — stamp-shaped directories without the exact
 *                              ownership marker survive pruning and never
 *                              consume a keep slot.
 *  24. stamp-root cleanup    — a failed marker write removes an empty fresh
 *                              root (rmdir path) but never a non-empty
 *                              foreign root.
 *  25. rollback ENOENT gate  — a vanished child makes the destination
 *                              unverifiable (rollback refused), never
 *                              "destination already gone".
 *
 * Requirements: unix host with tar/zip/unzip on PATH (the same tools
 * install.js itself shells out to). Skips cleanly on win32.
 *
 * Usage (standalone; there is intentionally no Go test harness for the npm
 * installer — test/scripts/install_script_test.go only execs POSIX sh):
 *
 *   node test/scripts/install_js_smoke.mjs        # self-contained, <10s
 */

import assert from "node:assert/strict";
import childProcess from "node:child_process";
import fs from "node:fs";
import { createRequire } from "node:module";
import os from "node:os";
import path from "node:path";
import url from "node:url";

const PLATFORM_MAP = {
  "darwin-x64": "dws-darwin-amd64.tar.gz",
  "darwin-arm64": "dws-darwin-arm64.tar.gz",
  "linux-x64": "dws-linux-amd64.tar.gz",
  "linux-arm64": "dws-linux-arm64.tar.gz",
};

const repoRoot = path.resolve(path.dirname(url.fileURLToPath(import.meta.url)), "..", "..");
const installJsSource = path.join(repoRoot, "build", "npm", "install.js");
const require = createRequire(import.meta.url);
const {
  UPSTREAM_AGENTS,
  resolvedAgentTargets,
  agentTargetDetected,
  isSkillBackupStamp,
  pruneSkillBackups,
  backupAndRemoveSkillDir,
  movePathRecoverablySync,
  publishCacheAtomically,
  publishCanonicalLinksAtomically,
  publishManagedMonoSkillSetAtomically,
  publishManagedMultiSkillSetAtomically,
  recordSkillPathPublicationSync,
  rollbackPublishedSkillPath,
} = require(installJsSource);

assert.equal(UPSTREAM_AGENTS.length, 76, "the complete upstream Agent registry is pinned");
assert.equal(new Set(UPSTREAM_AGENTS.map(({ id }) => id)).size, 76, "upstream Agent IDs are unique");
assert.equal(UPSTREAM_AGENTS.filter(({ universal }) => universal).length, 19, "upstream universal classification is pinned");
assert.equal(UPSTREAM_AGENTS.filter(({ universal }) => !universal).length, 57, "upstream non-universal classification is pinned");
assert.equal(UPSTREAM_AGENTS.filter(({ agentDir }) => agentDir === null).length, 2, "no-global Agents are retained in the registry");
assert.equal(UPSTREAM_AGENTS.filter(({ agentDir }) => agentDir === ".agents/skills").length, 6, "canonical-direct Agents need no target");
assert.equal(resolvedAgentTargets(path.join(os.tmpdir(), "dws-registry-home")).length, 70, "65 upstream roots, qoderwork, and 4 migration roots are deduplicated");
const detectionHome = fs.mkdtempSync(path.join(os.tmpdir(), "dws-agent-detect-"));
fs.mkdirSync(path.join(detectionHome, ".config", "kimchi"), { recursive: true });
fs.mkdirSync(path.join(detectionHome, ".tabnine"), { recursive: true });
const detectionTargets = resolvedAgentTargets(detectionHome);
for (const id of ["kimchi", "tabnine-cli"]) {
  assert.equal(agentTargetDetected(detectionTargets.find((target) => target.id === id)), true, `${id} shallow install marker is detected`);
}
fs.rmSync(detectionHome, { recursive: true, force: true });
const assetName = PLATFORM_MAP[`${process.platform}-${process.arch}`];

if (process.platform === "win32" || !assetName) {
  console.log(`SKIP: install.js smoke test needs a unix host with tar/zip/unzip (got ${process.platform}-${process.arch})`);
  process.exit(0);
}
for (const tool of ["tar", "zip", "unzip"]) {
  try {
    childProcess.execFileSync("sh", ["-c", `command -v ${tool}`], { stdio: "ignore" });
  } catch {
    console.log(`SKIP: required tool not on PATH: ${tool}`);
    process.exit(0);
  }
}

function sh(command, args, options = {}) {
  childProcess.execFileSync(command, args, { stdio: "ignore", ...options });
}

function writeFile(filePath, content, mode = 0o644) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, content, { mode });
}

// Cross-surface backup ownership contract (mirrors internal/upgrade/paths.go,
// scripts/install.ps1, scripts/install-devapp.ps1, and scripts/install.sh):
// every DWS-created stamp root carries .dws-skill-backup whose entire content
// is exactly these bytes. Hardcoded here on purpose so drift from any writer
// fails the smoke test.
const SKILL_BACKUP_MARKER_FILE = ".dws-skill-backup";
const SKILL_BACKUP_MARKER_BODY = "dws skill backup v1\n";

function markBackupStamp(stampRoot) {
  writeFile(path.join(stampRoot, SKILL_BACKUP_MARKER_FILE), SKILL_BACKUP_MARKER_BODY);
}

function writeManagedState(home, names) {
  const state = {
    version: "old",
    official_skills: names,
    updated_skills: names,
    managed_skills: names.map((name) => ({
      name,
      version: "old",
      source: "test",
      digest: `sha256:${"0".repeat(64)}`,
      digest_scope: "skill-directory-v1",
    })),
    updated_at: "2026-01-01T00:00:00Z",
  };
  writeFile(path.join(home, ".dws", "skills-state.json"), `${JSON.stringify(state, null, 2)}\n`);
}

function crossDeviceError() {
  return Object.assign(new Error("injected cross-device rename"), { code: "EXDEV" });
}

// withSimulatedWin32 runs fn with process.platform === "win32" so the
// Windows-only publish branches (junction type, absolute link target, and
// the atomic junction creation at the destination) get real execution
// coverage on a unix host.
// fs.symlinkSync ignores the type argument off Windows, so the same call is
// safe here. Create every temp path BEFORE calling: os.tmpdir() follows the
// simulated platform.
function withSimulatedWin32(fn) {
  const original = process.platform;
  const set = (value) =>
    Object.defineProperty(process, "platform", { value, configurable: true, enumerable: true, writable: false });
  set("win32");
  try {
    return fn();
  } finally {
    set(original);
  }
}

function runCrossDeviceMoveContract() {
  const roots = [];
  const fixture = (name) => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), `dws-installjs-exdev-${name}-`));
    roots.push(root);
    const src = path.join(root, "external", "skill");
    const dest = path.join(root, "home", ".dws", "skill-backups", "stamp", "skill");
    writeFile(path.join(src, "SKILL.md"), "old skill\n", 0o640);
    return { root, src, dest };
  };
  const injectedRename = (source, target) => {
    if (path.basename(source) === "skill" && path.basename(target) === "skill") {
      throw crossDeviceError();
    }
    fs.renameSync(source, target);
  };
  try {
    {
      const { src, dest } = fixture("success");
      fs.symlinkSync("SKILL.md", path.join(src, "skill-link"));
      fs.symlinkSync("missing.md", path.join(src, "dangling-link"));
      movePathRecoverablySync(src, dest, { renameFn: injectedRename });
      assert.equal(fs.lstatSync(dest).isDirectory(), true);
      assert.equal(fs.statSync(path.join(dest, "SKILL.md")).mode & 0o777, 0o640);
      assert.equal(fs.lstatSync(path.join(dest, "skill-link")).isSymbolicLink(), true);
      assert.equal(fs.readlinkSync(path.join(dest, "skill-link")), "SKILL.md");
      assert.equal(fs.lstatSync(path.join(dest, "dangling-link")).isSymbolicLink(), true);
      assert.equal(fs.readlinkSync(path.join(dest, "dangling-link")), "missing.md");
      assert.equal(fs.existsSync(src), false, "verified EXDEV move removes source last");

      const restored = path.join(path.dirname(src), "restored-skill");
      movePathRecoverablySync(dest, restored, {
        renameFn(source, target) {
          if (source === dest && target === restored) throw crossDeviceError();
          fs.renameSync(source, target);
        },
      });
      assert.equal(fs.readFileSync(path.join(restored, "SKILL.md"), "utf8"), "old skill\n");
      assert.equal(fs.existsSync(dest), false, "EXDEV rollback consumes verified backup");
    }

    for (const [name, options] of [
      ["copy", { copyFn() { throw new Error("copy failed"); } }],
      ["verify", { verifyFn() { throw new Error("verify failed"); } }],
    ]) {
      const { root, src, dest } = fixture(name);
      assert.throws(
        () => movePathRecoverablySync(src, dest, { renameFn: injectedRename, ...options }),
        new RegExp(`${name} failed`),
      );
      assert.equal(fs.readFileSync(path.join(src, "SKILL.md"), "utf8"), "old skill\n");
      assert.equal(fs.existsSync(dest), false);
      const backupParent = path.dirname(dest);
      if (fs.existsSync(backupParent)) {
        assert.equal(
          fs.readdirSync(backupParent).some((entry) => entry.startsWith(".skill.cross-device-")),
          false,
          `${name} failure cleans destination-filesystem staging`,
        );
      }
      assert.ok(root);
    }

    {
      const { src, dest } = fixture("remove");
      assert.throws(
        () => movePathRecoverablySync(src, dest, {
          renameFn: injectedRename,
          removeFn(target) {
            if (target === src) throw new Error("remove failed");
            fs.rmSync(target, { recursive: true, force: true });
          },
        }),
        /both copies retained .*remove failed/,
      );
      assert.equal(fs.readFileSync(path.join(src, "SKILL.md"), "utf8"), "old skill\n");
      assert.equal(fs.readFileSync(path.join(dest, "SKILL.md"), "utf8"), "old skill\n");
    }

    {
      const { src, dest } = fixture("read-only");
      fs.chmodSync(src, 0o555);
      movePathRecoverablySync(src, dest, { renameFn: injectedRename });
      assert.equal(fs.statSync(dest).mode & 0o777, 0o555, "read-only directory mode is restored after publish");
      assert.equal(fs.readFileSync(path.join(dest, "SKILL.md"), "utf8"), "old skill\n");
      fs.chmodSync(dest, 0o755);
    }

    {
      const { src, dest } = fixture("read-only-remove-failure");
      fs.chmodSync(src, 0o555);
      assert.throws(
        () => movePathRecoverablySync(src, dest, {
          renameFn: injectedRename,
          removeFn(target) {
            if (target === src) throw new Error("remove failed");
            fs.rmSync(target, { recursive: true, force: true });
          },
        }),
        /both copies retained .*remove failed/,
      );
      assert.equal(fs.statSync(src).mode & 0o777, 0o555, "failed removal restores source directory mode");
      fs.chmodSync(src, 0o755);
      fs.chmodSync(dest, 0o755);
    }

    {
      const { src, dest } = fixture("permission");
      let copied = false;
      const permission = Object.assign(new Error("permission denied"), { code: "EACCES" });
      assert.throws(
        () => movePathRecoverablySync(src, dest, {
          renameFn() { throw permission; },
          copyFn() { copied = true; },
        }),
        /permission denied/,
      );
      assert.equal(copied, false, "EACCES must not be mistaken for EXDEV");
      assert.equal(fs.readFileSync(path.join(src, "SKILL.md"), "utf8"), "old skill\n");
    }

    {
      const { src, dest } = fixture("dest-verify");
      assert.throws(
        () => movePathRecoverablySync(src, dest, {
          renameFn: injectedRename,
          verifyFn(left, right) {
            if (right === dest) throw new Error("dest-verify failed");
          },
        }),
        /dest retracted|dest-verify failed/,
      );
      assert.equal(fs.readFileSync(path.join(src, "SKILL.md"), "utf8"), "old skill\n");
      assert.equal(fs.existsSync(dest), false, "post-occupy verify failure retracts dest");
    }

    {
      const { src, dest } = fixture("dest-chmod");
      assert.throws(
        () => movePathRecoverablySync(src, dest, {
          renameFn: injectedRename,
          chmodFn(target, mode) {
            if (target === dest) throw new Error("chmod failed");
            fs.chmodSync(target, mode);
          },
        }),
        /dest retracted|chmod failed/,
      );
      assert.equal(fs.readFileSync(path.join(src, "SKILL.md"), "utf8"), "old skill\n");
      assert.equal(fs.existsSync(dest), false, "post-occupy chmod failure retracts dest");
    }

    {
      const { src, dest } = fixture("dest-cleanup");
      assert.throws(
        () => movePathRecoverablySync(src, dest, {
          renameFn: injectedRename,
          removeFn(target) {
            if (target !== src && path.basename(target).startsWith(".skill.cross-device-")) {
              throw new Error("cleanup failed");
            }
            fs.rmSync(target, { recursive: true, force: true });
          },
        }),
        /dest retracted|cleanup failed/,
      );
      assert.equal(fs.readFileSync(path.join(src, "SKILL.md"), "utf8"), "old skill\n");
      assert.equal(fs.existsSync(dest), false, "post-occupy staging cleanup failure retracts dest");
    }

    {
      const { src, dest } = fixture("same-volume-occupied");
      writeFile(path.join(dest, "concurrent-user-data.txt"), "must survive\n");
      assert.throws(
        () => movePathRecoverablySync(src, dest),
        /already exists/,
      );
      assert.deepEqual(fs.readdirSync(dest), ["concurrent-user-data.txt"]);
      assert.equal(fs.readFileSync(path.join(dest, "concurrent-user-data.txt"), "utf8"), "must survive\n");
      assert.equal(fs.readFileSync(path.join(src, "SKILL.md"), "utf8"), "old skill\n");
    }

    {
      const root = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-dangling-collision-"));
      roots.push(root);
      const home = path.join(root, "home");
      const victim = path.join(home, ".agents", "skills", "dingtalk-chat");
      writeFile(path.join(victim, "SKILL.md"), "old\n");
      const stamp = "20260813-150000";
      const occupied = path.join(home, ".dws", "skill-backups", stamp, ".agents-skills-dingtalk-chat");
      fs.mkdirSync(path.dirname(occupied), { recursive: true });
      fs.symlinkSync("missing-backup", occupied);
      const backup = backupAndRemoveSkillDir(home, victim, null, { backupStampFn: () => stamp });
      assert.ok(backup.includes(`${stamp}-1`), "dangling backup target selects a numbered sibling");
      assert.equal(fs.readlinkSync(occupied), "missing-backup", "dangling collision is never overwritten");
    }

    {
      const root = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-link-rollback-"));
      roots.push(root);
      const home = path.join(root, "home");
      const canonical = path.join(home, ".agents", "skills");
      const base = path.join(home, ".cursor", "skills");
      const canonicalSkill = path.join(canonical, "dingtalk-chat");
      const victim = path.join(base, "dingtalk-chat");
      writeFile(path.join(canonicalSkill, "SKILL.md"), "new\n");
      writeFile(path.join(victim, "SKILL.md"), "old\n");
      assert.throws(
        () => publishCanonicalLinksAtomically(home, canonical, base, ["dingtalk-chat"], [victim], {
          renameFn(source, target) {
            if (source === victim || (source.includes(`${path.sep}skill-backups${path.sep}`) && target === victim)) {
              throw crossDeviceError();
            }
            fs.renameSync(source, target);
          },
		  publishSymlinkFn() { throw new Error("link publish failed"); },
        }),
        /link publish failed/,
      );
      assert.equal(fs.readFileSync(path.join(victim, "SKILL.md"), "utf8"), "old\n");
    }
  } finally {
    for (const root of roots) fs.rmSync(root, { recursive: true, force: true });
  }
}

// stagePkg builds a fake npm package whose assets/dws-skills.zip contains
// exactly the given zip entries ({ "relative/path": "content" }) plus any
// listed empty directories. Returns { tmp, pkg, home }.
function stagePkg(zipEntries, emptyDirs = []) {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-smoke-"));
  const pkg = path.join(tmp, "pkg");
  const assets = path.join(pkg, "assets");
  fs.mkdirSync(assets, { recursive: true });
  fs.copyFileSync(installJsSource, path.join(pkg, "install.js"));

  const binStage = path.join(tmp, "bin-stage");
  writeFile(path.join(binStage, "dws"), "#!/bin/sh\necho fake-dws\n", 0o755);
  sh("tar", ["-czf", path.join(assets, assetName), "-C", binStage, "."]);

  const zipStage = path.join(tmp, "zip-stage");
  for (const [rel, content] of Object.entries(zipEntries)) {
    writeFile(path.join(zipStage, rel), content);
  }
  for (const dir of emptyDirs) {
    fs.mkdirSync(path.join(zipStage, dir), { recursive: true });
  }
  sh("zip", ["-qr", path.join(assets, "dws-skills.zip"), "."], { cwd: zipStage });

  return { tmp, pkg, home: path.join(tmp, "home") };
}

function runInstall(pkg, home, skillMode, extraEnv = {}) {
  const env = { ...process.env, HOME: home, ...extraEnv };
  if (skillMode !== undefined) {
    env.DWS_SKILL_MODE = skillMode;
  } else {
    delete env.DWS_SKILL_MODE;
  }
  return childProcess.spawnSync(process.execPath, [path.join(pkg, "install.js")], {
    env,
    encoding: "utf8",
  });
}

const scenarios = [];
function scenario(name, fn) {
  scenarios.push([name, fn]);
}

scenario("multi install lays out sibling skills and caches", () => {
  const { tmp, pkg, home } = stagePkg({
    "SKILL.md": "# mono root copy\n",
    "mono/SKILL.md": "# mono fixture\n",
    "multi/dingtalk-test/SKILL.md": "# dingtalk-test\n",
    "multi/dingtalk-test/references/guide.md": "guide\n",
    "multi/dws-shared/SKILL.md": "# dws-shared\n",
  });
  try {
    // Pre-existing state the multi install must reconcile.
    writeFile(path.join(home, ".agents", "skills", "dws", "SKILL.md"), "old mono\n");
    writeFile(path.join(home, ".agents", "skills", "dingtalk-stale", "SKILL.md"), "stale\n");
    writeManagedState(home, ["dingtalk-stale"]);
    writeFile(path.join(home, ".agents", "skills", "dingtalk-custom", "SKILL.md"), "market skill\n");
    writeFile(path.join(home, ".agents", "skills", "other-skill", "SKILL.md"), "not dws\n");

    const res = runInstall(pkg, home, undefined); // default mode = multi
    assert.equal(res.status, 0, `exit=${res.status}\nstdout=${res.stdout}\nstderr=${res.stderr}`);
    assert.match(res.stdout, /Skill mode: multi/);

    const base = path.join(home, ".agents", "skills");
    assert.ok(fs.existsSync(path.join(base, "dingtalk-test", "SKILL.md")), "dingtalk-test installed");
    assert.ok(fs.existsSync(path.join(base, "dingtalk-test", "references", "guide.md")), "references copied");
    assert.ok(fs.existsSync(path.join(base, "dws-shared", "SKILL.md")), "dws-shared installed");
    assert.ok(!fs.existsSync(path.join(base, "dws")), "mono leftover removed");
    assert.ok(!fs.existsSync(path.join(base, "dingtalk-stale")), "stale skill removed");
    assert.equal(fs.readFileSync(path.join(base, "dingtalk-custom", "SKILL.md"), "utf8"), "market skill\n", "unregistered dingtalk-* skill preserved");
    const state = JSON.parse(fs.readFileSync(path.join(home, ".dws", "skills-state.json"), "utf8"));
    const provenance = state.managed_skills.find((record) => record.name === "dingtalk-test");
    assert.equal(provenance.source, "npm-postinstall");
    assert.match(provenance.digest, /^sha256:[0-9a-f]{64}$/);
    assert.equal(provenance.digest_scope, "skill-directory-v1");
    assert.ok(fs.existsSync(path.join(base, "other-skill", "SKILL.md")), "non-DWS skill preserved");

    assert.ok(fs.existsSync(path.join(home, ".dws", "skills", "multi", "dingtalk-test", "SKILL.md")), "multi cache filled");
    assert.equal(fs.readFileSync(path.join(home, ".dws", "skills", "mono", "SKILL.md"), "utf8"), "# mono fixture\n", "mono cache from mono/ tree");
    assert.ok(fs.existsSync(path.join(pkg, "vendor", "dws")), "binary installed into vendor/");
    assert.ok(!fs.existsSync(path.join(pkg, "vendor", ".dws-runtime")), "legacy runtime sidecar omitted");
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("pinned universal global topology installs canonical and retires private copies", () => {
  const { tmp, pkg, home } = stagePkg({
    "mono/SKILL.md": "# mono fixture\n",
    "multi/dingtalk-chat/SKILL.md": "# dingtalk-chat\n",
    "multi/dws-shared/SKILL.md": "# dws-shared\n",
  });
  try {
    // The pinned upstream installer treats global universal Agents as
    // canonical-only even when their registry also retains a distinct
    // globalSkillsDir for legacy discovery/removal. Exercise every unique
    // non-canonical universal root so this distinction cannot drift again.
    const retiredUniversalRoots = [
      ".config/agents/skills",
      ".gemini/antigravity/skills",
      ".gemini/antigravity-cli/skills",
      ".codex/skills",
      ".cursor/skills",
      ".deepagents/agent/skills",
      ".firebender/skills",
      ".gemini/skills",
      ".copilot/skills",
      ".config/opencode/skills",
    ];
    for (const root of retiredUniversalRoots) {
      writeFile(path.join(home, root, "dingtalk-chat", "SKILL.md"), `beta.6 copy in ${root}\n`);
    }
    writeFile(
      path.join(home, ".agents", "skills", "dws", "multi", "dingtalk-chat", "SKILL.md"),
      "old nested duplicate\n",
    );

    const res = runInstall(pkg, home, "multi");
    assert.equal(res.status, 0, `exit=${res.status}\nstdout=${res.stdout}\nstderr=${res.stderr}`);
    assert.ok(
      fs.existsSync(path.join(home, ".agents", "skills", "dingtalk-chat", "SKILL.md")),
      "universal canonical Skill installed",
    );
    for (const root of retiredUniversalRoots) {
      assert.ok(
        !fs.existsSync(path.join(home, root, "dingtalk-chat")),
        `beta.6 universal duplicate migrated from ${root}`,
      );
    }
    assert.ok(
      !fs.existsSync(path.join(home, ".agents", "skills", "dws")),
      "legacy nested duplicate retired",
    );
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("ZCode links its Agent root to the canonical store", () => {
  const { tmp, pkg, home } = stagePkg({
    "mono/SKILL.md": "# mono fixture\n",
    "multi/dingtalk-chat/SKILL.md": "# dingtalk-chat\n",
  });
  try {
    writeFile(path.join(home, ".zcode", "v2", "config.json"), "{}\n");
    writeFile(
      path.join(home, ".agents", "skills", "dws", "multi", "dingtalk-chat", "SKILL.md"),
      "old nested duplicate\n",
    );

    const res = runInstall(pkg, home, "multi");
    assert.equal(res.status, 0, `exit=${res.status}\nstdout=${res.stdout}\nstderr=${res.stderr}`);
    assert.ok(
      fs.existsSync(path.join(home, ".zcode", "skills", "dingtalk-chat", "SKILL.md")),
      "ZCode Skill resolves through canonical link",
    );
    assert.ok(fs.lstatSync(path.join(home, ".zcode", "skills", "dingtalk-chat")).isSymbolicLink());
    assert.ok(fs.existsSync(path.join(home, ".agents", "skills", "dingtalk-chat", "SKILL.md")));
    assert.ok(
      !fs.existsSync(path.join(home, ".agents", "skills", "dws")),
      "legacy generic duplicate retired",
    );
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("a second install run is idempotent and preserves correct links", () => {
  const { tmp, pkg, home } = stagePkg({
    "mono/SKILL.md": "# mono fixture\n",
    "multi/dingtalk-chat/SKILL.md": "# dingtalk-chat\n",
  });
  try {
    writeFile(path.join(home, ".zcode", "v2", "config.json"), "{}\n");

    const first = runInstall(pkg, home, "multi", { XDG_CONFIG_HOME: path.join(home, ".config") });
    assert.equal(first.status, 0, `exit=${first.status}\nstdout=${first.stdout}\nstderr=${first.stderr}`);
    const linked = path.join(home, ".zcode", "skills", "dingtalk-chat");
    const firstTarget = fs.readlinkSync(linked);
    const firstState = JSON.parse(fs.readFileSync(path.join(home, ".dws", "skills-state.json"), "utf8"));

    const second = runInstall(pkg, home, "multi", { XDG_CONFIG_HOME: path.join(home, ".config") });
    assert.equal(second.status, 0, `exit=${second.status}\nstdout=${second.stdout}\nstderr=${second.stderr}`);
    assert.equal(fs.lstatSync(linked).isSymbolicLink(), true, "an already-correct link stays a link");
    assert.equal(fs.readlinkSync(linked), firstTarget, "an already-correct link is not rewritten");
    assert.equal(fs.readFileSync(path.join(linked, "SKILL.md"), "utf8"), "# dingtalk-chat\n");
    assert.ok(
      !second.stdout.includes(`已备份并移除 ${linked}`),
      "an already-correct link must not be backed up again",
    );
    const secondState = JSON.parse(fs.readFileSync(path.join(home, ".dws", "skills-state.json"), "utf8"));
    assert.deepEqual(secondState.official_skills, firstState.official_skills, "state is stable across runs");
    assert.deepEqual(
      secondState.managed_skills.map((record) => record.digest),
      firstState.managed_skills.map((record) => record.digest),
      "re-installing the same bundle yields the same provenance digests",
    );
    assert.ok(
      fs.readdirSync(path.join(home, ".dws", "skill-backups")).length <= 5,
      "repeated installs keep the backup root bounded",
    );
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("unretireable universal copy warns but still completes the install", () => {
  const { tmp, pkg, home } = stagePkg({
    "mono/SKILL.md": "# mono fixture\n",
    "multi/dingtalk-chat/SKILL.md": "# dingtalk-chat\n",
  });
  const codexSkills = path.join(home, ".codex", "skills");
  try {
    // Non-universal Agent that must still receive its link.
    writeFile(path.join(home, ".zcode", "v2", "config.json"), "{}\n");
    // Universal Agent whose obsolete copy cannot be moved out: r-x on the
    // parent means rename() fails with EACCES.
    writeFile(path.join(codexSkills, "dingtalk-chat", "SKILL.md"), "old codex copy\n");
    fs.chmodSync(codexSkills, 0o500);

    const res = runInstall(pkg, home, "multi", {
      XDG_CONFIG_HOME: path.join(home, ".config"),
      CODEX_HOME: "",
    });
    assert.equal(
      res.status,
      0,
      `a cleanup-only failure must not fail the install\nexit=${res.status}\nstdout=${res.stdout}\nstderr=${res.stderr}`,
    );
    assert.match(res.stderr, /旧副本迁移失败/, "the retire failure is reported as a warning");
    assert.ok(
      fs.existsSync(path.join(home, ".agents", "skills", "dingtalk-chat", "SKILL.md")),
      "canonical store is still installed",
    );
    const linked = path.join(home, ".zcode", "skills", "dingtalk-chat");
    assert.equal(fs.lstatSync(linked).isSymbolicLink(), true, "non-universal link is still installed");
    assert.equal(fs.readFileSync(path.join(linked, "SKILL.md"), "utf8"), "# dingtalk-chat\n");
    assert.ok(
      fs.existsSync(path.join(home, ".dws", "skills-state.json")),
      "skills-state.json is written even when a retire failed",
    );
    assert.equal(
      fs.readFileSync(path.join(codexSkills, "dingtalk-chat", "SKILL.md"), "utf8"),
      "old codex copy\n",
      "a copy that cannot be backed up is never deleted",
    );
  } finally {
    try {
      fs.chmodSync(codexSkills, 0o700);
    } catch {}
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("upstream Agent roots honor XDG and custom homes with relative links", () => {
  const { tmp, pkg, home } = stagePkg({
    "mono/SKILL.md": "# mono fixture\n",
    "multi/dingtalk-chat/SKILL.md": "# dingtalk-chat\n",
  });
  const xdg = path.join(tmp, "xdg config");
  const autohand = path.join(tmp, "autohand home");
  const claude = path.join(tmp, "claude config");
  const codex = path.join(tmp, "codex home");
  const hermes = path.join(tmp, "hermes home");
  try {
    writeFile(path.join(xdg, "goose", "config.yaml"), "enabled: true\n");
    writeFile(path.join(xdg, "agents", "detection.marker"), "amp\n");
    writeFile(path.join(xdg, "agents", "skills", "dingtalk-chat", "SKILL.md"), "old amp copy\n");
    writeFile(path.join(autohand, "config.json"), "{}\n");
    writeFile(path.join(claude, "config.json"), "{}\n");
    writeFile(path.join(claude, "skills", "dingtalk-chat", "SKILL.md"), "old claude copy\n");
    writeFile(path.join(codex, "config.toml"), "model=test\n");
    writeFile(path.join(codex, "skills", "dingtalk-chat", "SKILL.md"), "old codex copy\n");
    writeFile(path.join(hermes, "config.json"), "{}\n");
    writeFile(path.join(hermes, "skills", "dingtalk-chat", "SKILL.md"), "old hermes copy\n");
    writeFile(path.join(home, ".qoderwork", "config.json"), "{}\n");
    writeFile(path.join(home, ".amp", "skills", "dingtalk-chat", "SKILL.md"), "old DWS path\n");

    const res = runInstall(pkg, home, "multi", {
      XDG_CONFIG_HOME: xdg,
      AUTOHAND_HOME: autohand,
      CLAUDE_CONFIG_DIR: claude,
      CODEX_HOME: codex,
      HERMES_HOME: hermes,
    });
    assert.equal(res.status, 0, `exit=${res.status}\nstdout=${res.stdout}\nstderr=${res.stderr}`);

    for (const linked of [
      path.join(xdg, "goose", "skills", "dingtalk-chat"),
      path.join(autohand, "skills", "dingtalk-chat"),
      path.join(claude, "skills", "dingtalk-chat"),
      path.join(hermes, "skills", "dingtalk-chat"),
      path.join(home, ".qoderwork", "skills", "dingtalk-chat"),
    ]) {
      assert.ok(fs.lstatSync(linked).isSymbolicLink(), `${linked} is linked`);
      assert.ok(!path.isAbsolute(fs.readlinkSync(linked)), `${linked} uses a relative link`);
      assert.equal(fs.readFileSync(path.join(linked, "SKILL.md"), "utf8"), "# dingtalk-chat\n");
    }
    const backupText = [];
    const backupNames = [];
    const collectBackupText = (dir) => {
      for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) {
          backupNames.push(entry.name);
          collectBackupText(full);
        } else if (entry.isFile()) backupText.push(fs.readFileSync(full, "utf8"));
      }
    };
    collectBackupText(path.join(home, ".dws", "skill-backups"));
    assert.ok(backupText.includes("old claude copy\n"), "CLAUDE_CONFIG_DIR content is recoverably backed up");
    assert.ok(backupText.includes("old hermes copy\n"), "HERMES_HOME content is recoverably backed up");
    // Retired universal copies are the PR's advertised recovery story: every
    // one of them must be readable back out of ~/.dws/skill-backups.
    assert.ok(backupText.includes("old amp copy\n"), "retired XDG agents copy is recoverable");
    assert.ok(backupText.includes("old codex copy\n"), "retired CODEX_HOME copy is recoverable");
    assert.ok(backupText.includes("old DWS path\n"), "retired legacy ~/.amp copy is recoverable");
    assert.ok(
      backupNames.includes(".amp-skills-dingtalk-chat"),
      `backup names record the HOME-relative origin, got ${backupNames.join(",")}`,
    );
    for (const retired of [
      path.join(xdg, "agents", "skills", "dingtalk-chat"),
      path.join(codex, "skills", "dingtalk-chat"),
      path.join(home, ".amp", "skills", "dingtalk-chat"),
    ]) {
      assert.ok(!fs.existsSync(retired), `legacy/universal duplicate retired: ${retired}`);
    }
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("empty multi/ tree falls back to mono and keeps the old multi cache", () => {
  const { tmp, pkg, home } = stagePkg({
    "SKILL.md": "# mono root copy\n",
    "mono/SKILL.md": "# mono fixture\n",
    // Corrupt multi tree: a product subdir without SKILL.md.
    "multi/dingtalk-broken/references/guide.md": "orphan\n",
  });
  try {
    writeFile(path.join(home, ".agents", "skills", "dws", "SKILL.md"), "old mono\n");
    // A previously good multi cache must survive an empty/corrupt bundle.
    writeFile(path.join(home, ".dws", "skills", "multi", "dingtalk-good", "SKILL.md"), "good cache\n");

    const res = runInstall(pkg, home, "multi");
    assert.equal(res.status, 0, `exit=${res.status}\nstdout=${res.stdout}\nstderr=${res.stderr}`);
    assert.match(res.stdout, /falling back to mono/);

    const base = path.join(home, ".agents", "skills");
    assert.equal(fs.readFileSync(path.join(base, "dws", "SKILL.md"), "utf8"), "# mono fixture\n", "mono installed from mono/ tree");
    assert.ok(!fs.existsSync(path.join(base, "dingtalk-broken")), "broken multi skill not installed");
    assert.equal(
      fs.readFileSync(path.join(home, ".dws", "skills", "multi", "dingtalk-good", "SKILL.md"), "utf8"),
      "good cache\n",
      "previously good multi cache must not be wiped",
    );
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("bogus DWS_SKILL_MODE fails fast with a clear error", () => {
  const { tmp, pkg, home } = stagePkg({
    "mono/SKILL.md": "# mono fixture\n",
    "multi/dingtalk-test/SKILL.md": "# dingtalk-test\n",
  });
  try {
    const res = runInstall(pkg, home, "bogus");
    assert.notEqual(res.status, 0, "bogus mode must exit non-zero");
    assert.match(res.stderr, /invalid DWS_SKILL_MODE/);
    assert.ok(!fs.existsSync(path.join(home, ".agents", "skills", "dingtalk-test")), "nothing installed on mode error");
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("multi-only zip in mono mode skips skill install instead of copying staging root", () => {
  const { tmp, pkg, home } = stagePkg({
    // No root SKILL.md and no mono/ tree — a multi-only release layout.
    "multi/dingtalk-test/SKILL.md": "# dingtalk-test\n",
    "multi/dws-shared/SKILL.md": "# dws-shared\n",
  });
  try {
    const res = runInstall(pkg, home, "mono");
    assert.equal(res.status, 0, `exit=${res.status}\nstdout=${res.stdout}\nstderr=${res.stderr}`);
    assert.match(res.stdout, /mono skill tree not found.*skipping skill install/s);
    const base = path.join(home, ".agents", "skills");
    assert.ok(!fs.existsSync(path.join(base, "dws")), "staging root must not be copied into dws/");
    assert.ok(!fs.existsSync(path.join(home, ".dws", "skills", "mono")), "mono cache not refreshed without a mono tree");
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("multi backup failure preserves mono and reports failure", () => {
  const { tmp, pkg, home } = stagePkg({
    "mono/SKILL.md": "# mono fixture\n",
    "multi/dingtalk-test/SKILL.md": "# dingtalk-test\n",
    "multi/dws-shared/SKILL.md": "# dws-shared\n",
  });
  try {
    const base = path.join(home, ".agents", "skills");
    writeFile(path.join(base, "dws", "SKILL.md"), "old mono\n");
    // Poison the backup root: mkdirSync(<file>/<stamp>) must fail.
    writeFile(path.join(home, ".dws", "skill-backups"), "not a directory\n");

    const res = runInstall(pkg, home, "multi");
    assert.notEqual(res.status, 0, `exit=${res.status}\nstdout=${res.stdout}\nstderr=${res.stderr}`);
    assert.match(res.stderr, /未安装任何 multi Skill/);
    assert.equal(fs.readFileSync(path.join(base, "dws", "SKILL.md"), "utf8"), "old mono\n");
    assert.ok(!fs.existsSync(path.join(base, "dingtalk-test")), "product skill not installed after cleanup failure");
    assert.ok(!fs.existsSync(path.join(base, "dws-shared")), "shared skill not installed after cleanup failure");
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("mono backup failure preserves multi and reports failure", () => {
  const { tmp, pkg, home } = stagePkg({
    "mono/SKILL.md": "# mono fixture\n",
    "multi/dingtalk-test/SKILL.md": "# dingtalk-test\n",
  });
  try {
    const base = path.join(home, ".agents", "skills");
    writeFile(path.join(base, "dingtalk-test", "SKILL.md"), "old multi\n");
    writeManagedState(home, ["dingtalk-test"]);
    writeFile(path.join(home, ".dws", "skill-backups"), "not a directory\n");

    const res = runInstall(pkg, home, "mono");
    assert.notEqual(res.status, 0, `exit=${res.status}\nstdout=${res.stdout}\nstderr=${res.stderr}`);
    assert.match(res.stderr, /未安装任何 mono Skill/);
    assert.equal(fs.readFileSync(path.join(base, "dingtalk-test", "SKILL.md"), "utf8"), "old multi\n");
    assert.ok(!fs.existsSync(path.join(base, "dws")), "mono not installed after cleanup failure");
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("mono switch migrates exact pre-state official skills", () => {
  const { tmp, pkg, home } = stagePkg({
    "mono/SKILL.md": "# mono fixture\n",
    "multi/dingtalk-test/SKILL.md": "# dingtalk-test\n",
  });
  try {
    const base = path.join(home, ".agents", "skills");
    writeFile(path.join(base, "dingtalk-aitable", "SKILL.md"), "legacy official\n");
    writeFile(path.join(base, "dingtalk-custom", "SKILL.md"), "market skill\n");
    writeManagedState(home, ["dingtalk-aitable"]);

    const res = runInstall(pkg, home, "mono");
    assert.equal(res.status, 0, `exit=${res.status}\nstdout=${res.stdout}\nstderr=${res.stderr}`);
    assert.ok(!fs.existsSync(path.join(base, "dingtalk-aitable")), "pre-state official skill removed");
    assert.equal(fs.readFileSync(path.join(base, "dingtalk-custom", "SKILL.md"), "utf8"), "market skill\n");
    assert.ok(fs.existsSync(path.join(base, "dws", "SKILL.md")), "mono installed");
    assert.ok(!fs.existsSync(path.join(home, ".dws", "skills-state.json")), "mono clears centralized multi state");
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("cache copy failure preserves the previous complete cache", () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-cache-"));
  const source = path.join(tmp, "source");
  const cache = path.join(tmp, "skills", "multi");
  try {
    writeFile(path.join(source, "dingtalk-new", "SKILL.md"), "new cache\n");
    writeFile(path.join(cache, "dingtalk-old", "SKILL.md"), "old cache\n");

    assert.throws(
      () =>
        publishCacheAtomically(source, cache, (_src, staged) => {
          writeFile(path.join(staged, "partial", "SKILL.md"), "partial\n");
          throw new Error("injected cache copy failure");
        }),
      /injected cache copy failure/,
    );
    assert.equal(fs.readFileSync(path.join(cache, "dingtalk-old", "SKILL.md"), "utf8"), "old cache\n");
    assert.ok(!fs.existsSync(path.join(cache, "dingtalk-new")), "failed refresh must not publish new cache");
    assert.ok(
      !fs.readdirSync(path.dirname(cache)).some((name) => name.startsWith(".multi.tmp-")),
      "failed refresh must clean its staging directory",
    );
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("canonical link publication never clobbers or deletes concurrent user data", () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-canonical-race-"));
  try {
    {
      // Real no-replace property: the destination is occupied by data this
      // transaction never backed up (it is not a victim), so the production
      // publisher must refuse rather than replace or link into it. No stub
      // creates or destroys anything here.
      const home = path.join(tmp, "no-clobber-home");
      const canonical = path.join(home, ".agents", "skills");
      const base = path.join(home, ".cursor", "skills");
      const destination = path.join(base, "dingtalk-chat");
      writeFile(path.join(canonical, "dingtalk-chat", "SKILL.md"), "new\n");
      writeFile(path.join(destination, "concurrent-user-data.txt"), "must survive\n");
      assert.throws(
        () => publishCanonicalLinksAtomically(home, canonical, base, ["dingtalk-chat"], []),
        /Skill publish destination already exists/,
      );
      assert.equal(fs.readFileSync(path.join(destination, "concurrent-user-data.txt"), "utf8"), "must survive\n");
      assert.deepEqual(
        fs.readdirSync(destination),
        ["concurrent-user-data.txt"],
        "refused publication must not leave anything inside the occupied destination",
      );
      assert.equal(fs.lstatSync(destination).isDirectory(), true, "the user's directory is still a directory");
      assert.ok(
        !fs.readdirSync(base).some((name) => name.startsWith(".dws-link-set.tmp-")),
        "refused publication cleans its staging root",
      );
    }

    {
      const home = path.join(tmp, "rollback-home");
      const canonical = path.join(home, ".agents", "skills");
      const base = path.join(home, ".cursor", "skills");
      const first = path.join(base, "dingtalk-first");
      const second = path.join(base, "dingtalk-second");
      for (const name of ["dingtalk-first", "dingtalk-second"]) {
        writeFile(path.join(canonical, name, "SKILL.md"), `new ${name}\n`);
        writeFile(path.join(base, name, "SKILL.md"), `old ${name}\n`);
      }
      let publishCalls = 0;
      assert.throws(
        () => publishCanonicalLinksAtomically(
          home,
          canonical,
          base,
          ["dingtalk-first", "dingtalk-second"],
          [first, second],
          {
            publishSymlinkFn(target, linkPath, type) {
              publishCalls += 1;
              if (publishCalls === 1) {
                fs.symlinkSync(target, linkPath, type);
                return;
              }
              fs.unlinkSync(first);
              writeFile(path.join(first, "concurrent-user-data.txt"), "must survive\n");
              throw new Error("injected second canonical publish failure");
            },
          },
        ),
        /rollback failed|refusing to delete non-transaction Skill/,
      );
      assert.equal(
        fs.readFileSync(path.join(first, "concurrent-user-data.txt"), "utf8"),
        "must survive\n",
        "unmatched quarantine must be restored onto dest",
      );
      assert.ok(
        !fs.existsSync(path.join(first, "SKILL.md")),
        "restored concurrent dest must not be replaced by the backup",
      );
      assert.equal(fs.readFileSync(path.join(second, "SKILL.md"), "utf8"), "old dingtalk-second\n");
      const retained = fs.readdirSync(base)
        .filter((name) => name.startsWith(".dingtalk-first.rollback-"))
        .map((name) => path.join(base, name, "payload", "concurrent-user-data.txt"))
        .filter((candidate) => fs.existsSync(candidate));
      assert.equal(retained.length, 0, "restored concurrent object must not stay hidden in quarantine");
    }

    {
      // Direct rollback of a recorded publication after a concurrent writer
      // replaced dest: ownership is proven at dest first, so the foreign
      // object is never claimed into quarantine.
      const dest = path.join(tmp, "direct-rollback", "dingtalk-chat");
      writeFile(path.join(dest, "SKILL.md"), "published\n");
      const publication = recordSkillPathPublicationSync(dest);
      fs.rmSync(dest, { recursive: true, force: true });
      writeFile(path.join(dest, "concurrent-user-data.txt"), "must survive\n");
      assert.throws(
        () => rollbackPublishedSkillPath(publication),
        /refusing to delete non-transaction Skill/,
      );
      assert.equal(
        fs.readFileSync(path.join(dest, "concurrent-user-data.txt"), "utf8"),
        "must survive\n",
        "concurrent replacement stays at the original destination",
      );
      assert.deepEqual(fs.readdirSync(dest), ["concurrent-user-data.txt"]);
      const parent = path.dirname(dest);
      assert.equal(
        fs.readdirSync(parent).some((name) => name.startsWith(".dingtalk-chat.rollback-")),
        false,
        "unowned dest must not be moved into quarantine",
      );
    }

    {
      // Dest matched at the pre-check, then the quarantined object no longer
      // matches: restore it onto dest with no-replace instead of deleting.
      const dest = path.join(tmp, "post-quarantine", "dingtalk-chat");
      writeFile(path.join(dest, "SKILL.md"), "published\n");
      const publication = recordSkillPathPublicationSync(dest);
      assert.throws(
        () => rollbackPublishedSkillPath(publication, {
          quarantineRenameFn(source, target) {
            fs.renameSync(source, target);
            fs.writeFileSync(path.join(target, "SKILL.md"), "mutated-in-quarantine\n");
          },
        }),
        /refusing to delete non-transaction Skill/,
      );
      assert.equal(
        fs.readFileSync(path.join(dest, "SKILL.md"), "utf8"),
        "mutated-in-quarantine\n",
        "post-quarantine mismatch is restored onto dest",
      );
      const parent = path.dirname(dest);
      assert.equal(
        fs.readdirSync(parent).some((name) => name.startsWith(".dingtalk-chat.rollback-")
          && fs.existsSync(path.join(parent, name, "payload", "SKILL.md"))),
        false,
        "restored dest must not stay hidden in quarantine",
      );
    }

    {
      // Injected race: a concurrent creator occupies the destination at the
      // instant of publication. Link creation must fail atomically with
      // EEXIST, leaving the foreign object and its contents completely
      // unchanged — nothing is replaced, and nothing is linked into it (the
      // old `ln -P` publish treated an occupied directory as a container and
      // left its stray link behind because the publication had not yet
      // entered the rollback list).
      const home = path.join(tmp, "injected-race-home");
      const canonical = path.join(home, ".agents", "skills");
      const base = path.join(home, ".codex", "skills");
      const dest = path.join(base, "dingtalk-chat");
      writeFile(path.join(canonical, "dingtalk-chat", "SKILL.md"), "new\n");
      assert.throws(
        () =>
          publishCanonicalLinksAtomically(home, canonical, base, ["dingtalk-chat"], [], {
            publishSymlinkFn(target, linkPath, type) {
              writeFile(path.join(linkPath, "concurrent-user-data.txt"), "must survive\n");
              fs.symlinkSync(target, linkPath, type);
            },
          }),
        /Skill publish destination already exists/,
      );
      assert.deepEqual(
        fs.readdirSync(dest),
        ["concurrent-user-data.txt"],
        "the foreign object is neither replaced nor linked into",
      );
      assert.equal(fs.readFileSync(path.join(dest, "concurrent-user-data.txt"), "utf8"), "must survive\n");
      assert.equal(fs.lstatSync(dest).isDirectory(), true, "the foreign directory is untouched");
      assert.ok(
        !fs.readdirSync(base).some((name) => name.startsWith(".dws-link-set.tmp-")),
        "refused publication cleans its staging root",
      );
    }
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("mono and multi set publication never clobbers or deletes concurrent user data", () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-copy-race-"));
  try {
    {
      // Real no-replace property for the multi copy publisher: the destination
      // is occupied by data this transaction never backed up (it is not a
      // victim), so publication must refuse rather than rename over it. The
      // claim (mkdir) is the existence check itself — a concurrent creator
      // either loses the claim race or is never touched.
      const home = path.join(tmp, "multi-home");
      const source = path.join(tmp, "multi-source");
      const base = path.join(home, ".cursor", "skills");
      writeFile(path.join(source, "dingtalk-first", "SKILL.md"), "new first\n");
      writeFile(path.join(source, "dingtalk-second", "SKILL.md"), "new second\n");
      writeFile(path.join(base, "dingtalk-second", "concurrent-user-data.txt"), "must survive\n");
      assert.throws(
        () => publishManagedMultiSkillSetAtomically(home, source, base, ["dingtalk-first", "dingtalk-second"], []),
        /Skill publish destination already exists/,
      );
      assert.deepEqual(
        fs.readdirSync(path.join(base, "dingtalk-second")),
        ["concurrent-user-data.txt"],
        "refused publication must not leave anything inside the occupied destination",
      );
      assert.equal(
        fs.readFileSync(path.join(base, "dingtalk-second", "concurrent-user-data.txt"), "utf8"),
        "must survive\n",
      );
      assert.ok(!fs.existsSync(path.join(base, "dingtalk-first")), "the earlier publication is rolled back");
      assert.ok(
        !fs.readdirSync(base).some((name) => name.startsWith(".dws-multi-set.tmp-")),
        "refused publication cleans its staging root",
      );
    }

    {
      // Same contract for the mono publisher (canonical publish and the
      // copy fallback both flow through it).
      const home = path.join(tmp, "mono-home");
      const source = path.join(tmp, "mono-source");
      const base = path.join(home, ".agents", "skills");
      writeFile(path.join(source, "SKILL.md"), "new mono\n");
      writeFile(path.join(base, "dws", "concurrent-user-data.txt"), "must survive\n");
      assert.throws(
        () => publishManagedMonoSkillSetAtomically(home, source, base, []),
        /Skill publish destination already exists/,
      );
      assert.deepEqual(fs.readdirSync(path.join(base, "dws")), ["concurrent-user-data.txt"]);
      assert.equal(fs.readFileSync(path.join(base, "dws", "concurrent-user-data.txt"), "utf8"), "must survive\n");
      assert.ok(
        !fs.readdirSync(base).some((name) => name.startsWith(".dws-mono-set.tmp-")),
        "refused publication cleans its staging root",
      );
    }

    {
      // Injected race: a concurrent creator occupies the destination the
      // instant before the claim. The old check-then-rename code replaced it;
      // the claim must now fail with EEXIST and leave the user object intact.
      const home = path.join(tmp, "race-home");
      const source = path.join(tmp, "race-source");
      const base = path.join(home, ".zcode", "skills");
      const secondDest = path.join(base, "dingtalk-second");
      writeFile(path.join(source, "dingtalk-first", "SKILL.md"), "new first\n");
      writeFile(path.join(source, "dingtalk-second", "SKILL.md"), "new second\n");
      assert.throws(
        () =>
          publishManagedMultiSkillSetAtomically(home, source, base, ["dingtalk-first", "dingtalk-second"], [], {
            publishMkdirFn(target) {
              if (target === secondDest) {
                writeFile(path.join(target, "concurrent-user-data.txt"), "must survive\n");
              }
              fs.mkdirSync(target);
            },
          }),
        /Skill publish destination already exists/,
      );
      assert.deepEqual(fs.readdirSync(secondDest), ["concurrent-user-data.txt"], "the concurrent object is neither replaced nor linked into");
      assert.equal(fs.readFileSync(path.join(secondDest, "concurrent-user-data.txt"), "utf8"), "must survive\n");
      assert.ok(!fs.existsSync(path.join(base, "dingtalk-first")), "the earlier publication is rolled back");
    }

    {
      // The guarantee holds on the simulated Windows publisher path as well
      // (libuv's rename replaces outright there, so the claim matters most).
      const home = path.join(tmp, "win32-home");
      const source = path.join(tmp, "win32-source");
      const base = path.join(home, ".qoder", "skills");
      writeFile(path.join(source, "dingtalk-first", "SKILL.md"), "new first\n");
      writeFile(path.join(base, "dingtalk-first", "concurrent-user-data.txt"), "must survive\n");
      withSimulatedWin32(() => {
        assert.throws(
          () => publishManagedMultiSkillSetAtomically(home, source, base, ["dingtalk-first"], []),
          /Skill publish destination already exists/,
        );
      });
      assert.deepEqual(fs.readdirSync(path.join(base, "dingtalk-first")), ["concurrent-user-data.txt"]);
      assert.equal(
        fs.readFileSync(path.join(base, "dingtalk-first", "concurrent-user-data.txt"), "utf8"),
        "must survive\n",
      );
    }
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("simulated win32 publishes junctions with absolute canonical targets", () => {
  // Temp roots are created before entering the simulated platform: os.tmpdir()
  // would otherwise resolve through Windows environment variables.
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-win32-link-"));
  try {
    const home = path.join(tmp, "home");
    const canonical = path.join(home, ".agents", "skills");
    const canonicalSkill = path.join(canonical, "dingtalk-chat");
    const base = path.join(home, ".zcode", "skills");
    const dest = path.join(base, "dingtalk-chat");
    writeFile(path.join(canonicalSkill, "SKILL.md"), "# dingtalk-chat\n");
    writeFile(path.join(dest, "SKILL.md"), "old zcode copy\n");

    const symlinkCalls = [];
    const publishCalls = [];
    withSimulatedWin32(() => {
      publishCanonicalLinksAtomically(home, canonical, base, ["dingtalk-chat"], [dest], {
        symlinkFn(target, linkPath, type) {
          symlinkCalls.push({ target, linkPath, type });
          fs.symlinkSync(target, linkPath, type);
        },
        publishSymlinkFn(target, linkPath, type) {
          publishCalls.push({ target, linkPath, type });
          fs.symlinkSync(target, linkPath, type);
        },
      });
    });

    assert.equal(symlinkCalls.length, 1, "one staged link per bundle skill");
    assert.equal(symlinkCalls[0].type, "junction", "Windows must stage junctions, not symlinks");
    assert.equal(path.isAbsolute(symlinkCalls[0].target), true, "junctions require an absolute target");
    assert.equal(symlinkCalls[0].target, fs.realpathSync(canonicalSkill), "junction targets the canonical store");
    assert.equal(publishCalls.length, 1, "win32 publishes by creating the junction directly at the destination");
    assert.equal(publishCalls[0].linkPath, dest);
    assert.equal(publishCalls[0].type, "junction");
    assert.equal(fs.readlinkSync(dest), fs.realpathSync(canonicalSkill), "published junction kept its absolute target");
    assert.equal(fs.readFileSync(path.join(dest, "SKILL.md"), "utf8"), "# dingtalk-chat\n");
    const backupRoot = path.join(home, ".dws", "skill-backups");
    const stamps = fs.readdirSync(backupRoot);
    assert.equal(stamps.length, 1, "the replaced copy produced exactly one backup stamp");
    assert.deepEqual(
      fs.readdirSync(path.join(backupRoot, stamps[0])).sort(),
      [SKILL_BACKUP_MARKER_FILE, ".zcode-skills-dingtalk-chat"],
      "the stamp root carries the ownership marker next to the backup payload",
    );
    assert.equal(
      fs.readFileSync(path.join(backupRoot, stamps[0], SKILL_BACKUP_MARKER_FILE), "utf8"),
      SKILL_BACKUP_MARKER_BODY,
      "backup creation stamps the exact cross-surface marker bytes",
    );
    assert.equal(
      fs.readFileSync(path.join(backupRoot, stamps[0], ".zcode-skills-dingtalk-chat", "SKILL.md"), "utf8"),
      "old zcode copy\n",
      "the replaced copy stays recoverable",
    );

    // FIX 5: an object appearing at the destination at publish time must never
    // be replaced — junction creation fails atomically with EEXIST (the old
    // win32 rename passed MOVEFILE_REPLACE_EXISTING and replaced it).
    const raceBase = path.join(home, ".qoder", "skills");
    const occupied = path.join(raceBase, "dingtalk-chat");
    withSimulatedWin32(() => {
      assert.throws(
        () =>
          publishCanonicalLinksAtomically(home, canonical, raceBase, ["dingtalk-chat"], [], {
            publishSymlinkFn(target, linkPath, type) {
              writeFile(path.join(linkPath, "concurrent-user-data.txt"), "must survive\n");
              fs.symlinkSync(target, linkPath, type);
            },
          }),
        /Skill publish destination already exists/,
      );
    });
    assert.deepEqual(fs.readdirSync(occupied), ["concurrent-user-data.txt"], "the concurrent object is neither replaced nor linked into");
    assert.equal(fs.readFileSync(path.join(occupied, "concurrent-user-data.txt"), "utf8"), "must survive\n");
    assert.equal(process.platform !== "win32", true, "the simulated platform is restored");
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("skill backups stay bounded to the newest five stamps", () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-backup-prune-"));
  const home = path.join(tmp, "home");
  const backupRoot = path.join(home, ".dws", "skill-backups");
  try {
    for (const stamp of [
      "20260101-000001", "20260101-000002", "20260101-000003",
      "20260101-000004", "20260101-000005", "20260101-000006", "20260101-000007",
    ]) {
      const stampRoot = path.join(backupRoot, stamp);
      writeFile(path.join(stampRoot, "placeholder", "SKILL.md"), `${stamp}\n`);
      markBackupStamp(stampRoot);
    }
    const victim = path.join(home, ".codex", "skills", "dingtalk-chat");
    writeFile(path.join(victim, "SKILL.md"), "old codex copy\n");

    const backup = backupAndRemoveSkillDir(home, victim, null, { backupStampFn: () => "20260102-000000" });

    const stamps = fs.readdirSync(backupRoot).sort();
    assert.equal(stamps.length, 5, `backup root must stay bounded, got ${stamps.join(",")}`);
    assert.ok(stamps.includes("20260102-000000"), "the backup from this run survives pruning");
    assert.ok(!stamps.includes("20260101-000001"), "the oldest stamps are pruned first");
    assert.ok(!stamps.includes("20260101-000003"), "pruning removes exactly the excess");
    assert.ok(stamps.includes("20260101-000004"), "the newest kept stamps are retained");
    assert.equal(path.basename(backup), ".codex-skills-dingtalk-chat");
    assert.equal(fs.readFileSync(path.join(backup, "SKILL.md"), "utf8"), "old codex copy\n");
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("prune preserves unknown directories in skill-backups", () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-prune-unknown-"));
  const home = path.join(tmp, "home");
  const backupRoot = path.join(home, ".dws", "skill-backups");
  try {
    for (const stamp of [
      "20260101-000001", "20260101-000002", "20260101-000003",
      "20260101-000004", "20260101-000005", "20260101-000006", "20260101-000007",
    ]) {
      const stampRoot = path.join(backupRoot, stamp);
      writeFile(path.join(stampRoot, "placeholder", "SKILL.md"), `${stamp}\n`);
      markBackupStamp(stampRoot);
    }
    const unknowns = [
      "user-personal-backup",
      "20260101-00000",      // too few digits in the time part
      "20260101-000000-abc", // non-numeric suffix
      "notes",
      "20260101",            // missing time portion
    ];
    for (const name of unknowns) {
      writeFile(path.join(backupRoot, name, "marker.txt"), `unknown:${name}\n`);
    }

    pruneSkillBackups(home);

    const remaining = fs.readdirSync(backupRoot).sort();
    const stamped = remaining.filter((n) => isSkillBackupStamp(n));
    assert.equal(stamped.length, 5, `stamped backups = ${stamped.length}, want 5`);
    assert.ok(!stamped.includes("20260101-000001"), "oldest stamps pruned first");
    assert.ok(!stamped.includes("20260101-000002"), "second-oldest pruned");
    assert.ok(stamped.includes("20260101-000005"), "newest kept stamps retained");
    for (const name of unknowns) {
      assert.ok(remaining.includes(name), `unknown directory ${name} must survive pruning`);
      assert.equal(
        fs.readFileSync(path.join(backupRoot, name, "marker.txt"), "utf8"),
        `unknown:${name}\n`,
        `unknown directory ${name} contents must be intact`,
      );
    }
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("prune preserves unmarked stamp directories without consuming keep slots", () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-prune-unmarked-"));
  const home = path.join(tmp, "home");
  const backupRoot = path.join(home, ".dws", "skill-backups");
  try {
    // Exactly SKILL_BACKUP_KEEP verified stamps: nothing may be pruned, even
    // though foreign stamp-shaped siblings would push the naive count past
    // the limit.
    const marked = ["20260101-000001", "20260101-000002", "20260101-000003", "20260101-000004", "20260101-000005"];
    for (const stamp of marked) {
      const stampRoot = path.join(backupRoot, stamp);
      writeFile(path.join(stampRoot, "placeholder", "SKILL.md"), `${stamp}\n`);
      markBackupStamp(stampRoot);
    }
    const foreign = [
      { stamp: "20260102-000001", body: null },              // no marker at all
      { stamp: "20260102-000002", body: "dws skill backup v2\n" }, // wrong body
      { stamp: "20260101-000000", body: "dws skill backup v1" },   // missing trailing LF
      { stamp: "20260101-000099", body: "dws skill backup v1\r\n" }, // CRLF is not the exact contract bytes
    ];
    for (const { stamp, body } of foreign) {
      const stampRoot = path.join(backupRoot, stamp);
      writeFile(path.join(stampRoot, "user-data.txt"), `foreign:${stamp}\n`);
      if (body !== null) writeFile(path.join(stampRoot, SKILL_BACKUP_MARKER_FILE), body);
    }

    pruneSkillBackups(home);

    let remaining = fs.readdirSync(backupRoot).sort();
    assert.deepEqual(
      remaining,
      [...marked, ...foreign.map(({ stamp }) => stamp)].sort(),
      "with exactly keep-many verified stamps, nothing is pruned and unmarked stamps never count",
    );

    // Force real pruning: three more verified stamps push marked history to
    // eight, so exactly the three oldest verified stamps go while every
    // unmarked sibling stays put (still not consuming a keep slot).
    for (const stamp of ["20260103-000001", "20260103-000002", "20260103-000003"]) {
      const stampRoot = path.join(backupRoot, stamp);
      writeFile(path.join(stampRoot, "placeholder", "SKILL.md"), `${stamp}\n`);
      markBackupStamp(stampRoot);
    }

    pruneSkillBackups(home);

    remaining = fs.readdirSync(backupRoot).sort();
    for (const pruned of marked.slice(0, 3)) {
      assert.ok(!remaining.includes(pruned), `oldest verified stamp ${pruned} is pruned`);
    }
    assert.deepEqual(
      remaining,
      [marked[3], marked[4], ...foreign.map(({ stamp }) => stamp), "20260103-000001", "20260103-000002", "20260103-000003"].sort(),
      "pruning keeps exactly the newest five verified stamps and preserves every unmarked sibling",
    );
    for (const { stamp } of foreign) {
      assert.equal(
        fs.readFileSync(path.join(backupRoot, stamp, "user-data.txt"), "utf8"),
        `foreign:${stamp}\n`,
        `unmarked stamp ${stamp} contents must be intact`,
      );
    }
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("multi set publish failure restores the complete previous set", () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-multi-set-"));
  const home = path.join(tmp, "home");
  const source = path.join(tmp, "multi");
  const base = path.join(home, ".agents", "skills");
  const first = path.join(base, "dingtalk-first");
  const second = path.join(base, "dingtalk-second");
  try {
    writeFile(path.join(source, "dingtalk-first", "SKILL.md"), "new first\n");
    writeFile(path.join(source, "dingtalk-second", "SKILL.md"), "new second\n");
    writeFile(path.join(first, "SKILL.md"), "old first\n");
    writeFile(path.join(second, "SKILL.md"), "old second\n");

    const originalRename = fs.renameSync;
    assert.throws(
      () =>
        publishManagedMultiSkillSetAtomically(
          home,
          source,
          base,
          ["dingtalk-first", "dingtalk-second"],
          [first, second],
          {
            renameFn(src, dest) {
              if (
                src === first ||
                src === second ||
                (src.includes(`${path.sep}skill-backups${path.sep}`) && (dest === first || dest === second))
              ) {
                throw crossDeviceError();
              }
              originalRename(src, dest);
            },
            // Child files publish through the link primitive now, so the
            // injected failure rides the link seam.
            publishLinkFn(src, dest) {
              if (src.includes(".dws-multi-set.tmp-") && src.includes(`${path.sep}dingtalk-second${path.sep}`)) {
                throw new Error("injected second publish failure");
              }
              return fs.linkSync(src, dest);
            },
          },
        ),
      /injected second publish failure/,
    );
    assert.equal(fs.readFileSync(path.join(first, "SKILL.md"), "utf8"), "old first\n");
    assert.equal(fs.readFileSync(path.join(second, "SKILL.md"), "utf8"), "old second\n");
    assert.ok(
      !fs.readdirSync(base).some((name) => name.startsWith(".dws-multi-set.tmp-")),
      "failed publish must clean the multi-set staging directory",
    );
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("multi set backup failure restores earlier backups", () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-multi-backup-"));
  const home = path.join(tmp, "home");
  const source = path.join(tmp, "multi");
  const base = path.join(home, ".agents", "skills");
  const first = path.join(base, "dingtalk-first");
  const second = path.join(base, "dingtalk-second");
  try {
    writeFile(path.join(source, "dingtalk-first", "SKILL.md"), "new first\n");
    writeFile(path.join(source, "dingtalk-second", "SKILL.md"), "new second\n");
    writeFile(path.join(first, "SKILL.md"), "old first\n");
    writeFile(path.join(second, "SKILL.md"), "old second\n");

    const originalRename = fs.renameSync;
    assert.throws(
      () =>
        publishManagedMultiSkillSetAtomically(
          home,
          source,
          base,
          ["dingtalk-first", "dingtalk-second"],
          [first, second],
          {
            renameFn(src, dest) {
              if (src === second) {
                throw new Error("injected second backup failure");
              }
              originalRename(src, dest);
            },
          },
        ),
      /failed to back up Skill directory/,
    );
    assert.equal(fs.readFileSync(path.join(first, "SKILL.md"), "utf8"), "old first\n");
    assert.equal(fs.readFileSync(path.join(second, "SKILL.md"), "utf8"), "old second\n");
    assert.ok(!fs.existsSync(path.join(base, "dingtalk-first", "new-only")));
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

for (const failureKind of ["backup", "publish"]) {
  scenario(`mono set ${failureKind} failure restores the complete previous set`, () => {
    const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-mono-set-"));
    const home = path.join(tmp, "home");
    const source = path.join(tmp, "mono");
    const base = path.join(home, ".agents", "skills");
    const first = path.join(base, "dingtalk-first");
    const second = path.join(base, "dingtalk-second");
    const dest = path.join(base, "dws");
    try {
      writeFile(path.join(source, "SKILL.md"), "new mono\n");
      writeFile(path.join(first, "SKILL.md"), "old first\n");
      writeFile(path.join(second, "SKILL.md"), "old second\n");

      const originalRename = fs.renameSync;
      assert.throws(
        () =>
          publishManagedMonoSkillSetAtomically(home, source, base, [dest, first, second], {
            renameFn(src, target) {
              if (failureKind === "backup" && src === second) {
                throw new Error("injected second backup failure");
              }
              if (
                failureKind === "publish" &&
                (src === first ||
                  src === second ||
                  (src.includes(`${path.sep}skill-backups${path.sep}`) && (target === first || target === second)))
              ) {
                throw crossDeviceError();
              }
              originalRename(src, target);
            },
            publishLinkFn(src, target) {
              if (failureKind === "publish" && src.includes(".dws-mono-set.tmp-")) {
                throw new Error("injected mono publish failure");
              }
              return fs.linkSync(src, target);
            },
          }),
        /injected|failed to back up/,
      );
      assert.equal(fs.readFileSync(path.join(first, "SKILL.md"), "utf8"), "old first\n");
      assert.equal(fs.readFileSync(path.join(second, "SKILL.md"), "utf8"), "old second\n");
      assert.ok(!fs.existsSync(dest), "failed mono transaction must not expose dws/");
      assert.ok(
        !fs.readdirSync(base).some((name) => name.startsWith(".dws-mono-set.tmp-")),
        "failed mono transaction must clean its staging directory",
      );
    } finally {
      fs.rmSync(tmp, { recursive: true, force: true });
    }
  });
}

// A stamp root whose ownership marker write fails must not linger: an empty
// fresh root is removed (rmdir, never a recursive delete), while a
// pre-existing non-empty root holding foreign data always survives.
scenario("a failed marker write never leaves an empty unowned stamp root", () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-stamp-cleanup-"));
  try {
    const home = path.join(tmp, "home");
    const victim = path.join(home, ".codex", "skills", "dingtalk-chat");
    writeFile(path.join(victim, "SKILL.md"), "old codex copy\n");

    // Fresh root: created by this call, then the marker write is poisoned so
    // the backup aborts before any payload moves.
    const stamp = "20260801-000000";
    const markerPath = path.join(home, ".dws", "skill-backups", stamp, SKILL_BACKUP_MARKER_FILE);
    const originalWriteFileSync = fs.writeFileSync;
    fs.writeFileSync = (target, data, options) => {
      if (target === markerPath) {
        throw Object.assign(new Error("injected marker write failure"), { code: "EACCES" });
      }
      return originalWriteFileSync(target, data, options);
    };
    try {
      assert.throws(
        () => backupAndRemoveSkillDir(home, victim, null, { backupStampFn: () => stamp }),
        /failed to back up Skill directory/,
      );
    } finally {
      fs.writeFileSync = originalWriteFileSync;
    }
    assert.equal(fs.readFileSync(path.join(victim, "SKILL.md"), "utf8"), "old codex copy\n", "the source stays in place");
    assert.equal(
      fs.existsSync(path.join(home, ".dws", "skill-backups", stamp)),
      false,
      "an empty unowned stamp root is cleaned up",
    );

    // Foreign non-empty root with the marker path occupied by a directory:
    // unusable, so the backup moves to a suffixed root and the foreign root
    // is never touched.
    const foreignStamp = "20260801-000001";
    const foreignRoot = path.join(home, ".dws", "skill-backups", foreignStamp);
    writeFile(path.join(foreignRoot, "user-data.txt"), "foreign\n");
    fs.mkdirSync(path.join(foreignRoot, SKILL_BACKUP_MARKER_FILE));
    const foreignBackup = backupAndRemoveSkillDir(home, victim, null, { backupStampFn: () => foreignStamp });
    assert.ok(foreignBackup.includes(`${foreignStamp}-1`), `the backup moves to a suffixed root: ${foreignBackup}`);
    assert.equal(fs.readFileSync(path.join(foreignRoot, "user-data.txt"), "utf8"), "foreign\n", "a non-empty foreign stamp root survives untouched");

    // Plain foreign root (ordinary files, no marker): must never be adopted —
    // the backup lands in the suffixed root instead, and the foreign root
    // never receives our marker.
    writeFile(path.join(victim, "SKILL.md"), "old codex copy\n");
    const plainStamp = "20260801-000002";
    const plainRoot = path.join(home, ".dws", "skill-backups", plainStamp);
    writeFile(path.join(plainRoot, "user-data.txt"), "foreign\n");
    const plainBackup = backupAndRemoveSkillDir(home, victim, null, { backupStampFn: () => plainStamp });
    assert.ok(plainBackup.includes("20260801-000002-1"), `the backup moves to a suffixed root: ${plainBackup}`);
    assert.equal(fs.readFileSync(path.join(plainRoot, "user-data.txt"), "utf8"), "foreign\n", "plain foreign data survives");
    assert.equal(fs.existsSync(path.join(plainRoot, SKILL_BACKUP_MARKER_FILE)), false, "the foreign root never receives our marker");
    assert.equal(fs.readFileSync(path.join(plainBackup, "SKILL.md"), "utf8"), "old codex copy\n", "the payload is intact in the suffixed root");
    assert.equal(
      fs.readFileSync(path.join(path.dirname(plainBackup), SKILL_BACKUP_MARKER_FILE), "utf8"),
      "dws skill backup v1\n",
      "the suffixed root carries the ownership marker",
    );
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

// ENOENT during the pre-quarantine identity walk means "already gone" only
// when the destination itself is missing; a concurrently removed CHILD must
// fail the rollback loudly instead of silently skipping it.
scenario("rollback refuses an unverifiable destination whose child vanished", () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-rollback-enoent-"));
  try {
    const dest = path.join(tmp, "skill");
    writeFile(path.join(dest, "SKILL.md"), "payload\n");
    writeFile(path.join(dest, "child.md"), "child\n");
    const publication = recordSkillPathPublicationSync(dest);

    // Remove the child while the pre-quarantine fingerprint walk sits between
    // its readdir and the per-child lstat, so the walk itself fails ENOENT.
    const firstChild = path.join(dest, "SKILL.md");
    const vanishedChild = path.join(dest, "child.md");
    const originalReadFileSync = fs.readFileSync;
    fs.readFileSync = (target, options) => {
      if (target === firstChild) {
        fs.rmSync(vanishedChild);
      }
      return originalReadFileSync(target, options);
    };
    try {
      assert.throws(
        () => rollbackPublishedSkillPath(publication),
        /refusing to delete unverifiable Skill/,
      );
    } finally {
      fs.readFileSync = originalReadFileSync;
    }
    assert.equal(fs.readFileSync(path.join(dest, "SKILL.md"), "utf8"), "payload\n", "the unverifiable destination is preserved");
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

scenario("per-child no-clobber refuses same-name concurrent entries after the claim", () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "dws-installjs-child-race-"));
  try {
    const buildFixture = (label) => {
      const home = path.join(tmp, label);
      const source = path.join(tmp, `${label}-src`);
      fs.mkdirSync(path.join(source, "dingtalk-chat", "references"), { recursive: true });
      writeFile(path.join(source, "dingtalk-chat", "SKILL.md"), "new chat\n");
      writeFile(path.join(source, "dingtalk-chat", "notes.md"), "notes\n");
      writeFile(path.join(source, "dingtalk-chat", "references", "guide.md"), "guide\n");
      fs.symlinkSync("SKILL.md", path.join(source, "dingtalk-chat", "rel-link"));
      return { home, source, base: path.join(home, ".zcode", "skills") };
    };

    {
      // Same-name concurrent FILE: the foreign file lands at
      // <claim>/SKILL.md between the claim and the per-child link primitive.
      const { home, source, base } = buildFixture("file-race");
      const dest = path.join(base, "dingtalk-chat");
      const planted = path.join(dest, "SKILL.md");
      assert.throws(
        () =>
          publishManagedMultiSkillSetAtomically(home, source, base, ["dingtalk-chat"], [], {
            publishLinkFn(src, target) {
              if (target === planted && !fs.existsSync(planted)) {
                writeFile(planted, "concurrent-user-edit\n");
              }
              return fs.linkSync(src, target);
            },
          }),
        /Skill move destination already exists/,
      );
      assert.equal(fs.readFileSync(planted, "utf8"), "concurrent-user-edit\n", "the concurrent file is never replaced");
      assert.deepEqual(
        fs.readdirSync(dest).sort(),
        ["SKILL.md"],
        "no other child may move once the concurrent entry is seen",
      );
      assert.ok(
        !fs.readdirSync(base).some((name) => name.startsWith(".dws-multi-set.tmp-")),
        "failed publish must clean the staging root",
      );
    }

    {
      // Same-name concurrent DIRECTORY at <claim>/references.
      const { home, source, base } = buildFixture("dir-race");
      const dest = path.join(base, "dingtalk-chat");
      const planted = path.join(dest, "references");
      assert.throws(
        () =>
          publishManagedMultiSkillSetAtomically(home, source, base, ["dingtalk-chat"], [], {
            publishMkdirFn(target) {
              if (target === planted && !fs.existsSync(planted)) {
                fs.mkdirSync(path.join(planted, "user-data"), { recursive: true });
                writeFile(path.join(planted, "user-data", "keep.txt"), "keep\n");
              }
              fs.mkdirSync(target, { mode: 0o700 });
            },
          }),
        /Skill move destination already exists/,
      );
      assert.equal(
        fs.readFileSync(path.join(planted, "user-data", "keep.txt"), "utf8"),
        "keep\n",
        "the concurrent directory survives byte-identical",
      );
      assert.ok(
        !fs.readdirSync(base).some((name) => name.startsWith(".dws-multi-set.tmp-")),
        "failed publish must clean the staging root",
      );
    }

    {
      // Same-name concurrent SYMLINK at <claim>/rel-link.
      const { home, source, base } = buildFixture("link-race");
      const dest = path.join(base, "dingtalk-chat");
      const planted = path.join(dest, "rel-link");
      assert.throws(
        () =>
          publishManagedMultiSkillSetAtomically(home, source, base, ["dingtalk-chat"], [], {
            publishSymlinkFn(target, linkname) {
              if (linkname === planted && !fs.existsSync(planted)) {
                fs.symlinkSync("foreign-target", planted);
              }
              return fs.symlinkSync(target, linkname);
            },
          }),
        /Skill move destination already exists/,
      );
      assert.equal(fs.readlinkSync(planted), "foreign-target", "the concurrent link is never replaced");
      assert.ok(
        !fs.readdirSync(base).some((name) => name.startsWith(".dws-multi-set.tmp-")),
        "failed publish must clean the staging root",
      );
    }
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});

process.stdout.write("• EXDEV copy/verify/remove and transaction rollback contract ... ");
runCrossDeviceMoveContract();
process.stdout.write("ok\n");

for (const [name, fn] of scenarios) {
  process.stdout.write(`• ${name} ... `);
  fn();
  process.stdout.write("ok\n");
}
console.log(`OK — ${scenarios.length} install.js smoke scenarios passed`);
