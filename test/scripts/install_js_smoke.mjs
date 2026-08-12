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
 *   2. empty multi/ tree    — warns and falls back to mono instead of
 *                             crashing postinstall; a previously good multi
 *                             cache is NOT wiped.
 *   3. bogus mode           — DWS_SKILL_MODE=bogus exits non-zero with an
 *                             "invalid DWS_SKILL_MODE" error.
 *   4. multi-only zip, mono — mono install is skipped with a warning; the
 *                             staging root is NOT copied into a dws/ dir.
 *   5. multi backup failure — preserves mono, writes no multi skill, and
 *                             reports postinstall failure.
 *   6. mono backup failure  — preserves multi, writes no mono skill, and
 *                             reports postinstall failure.
 *   7. mono switch          — migrates only centrally owned multi Skills.
 *   8. cache copy failure   — preserves the previous complete cache.
 *   9. multi publish failure — restores the complete previous Skill set.
 *  10. multi backup failure  — restores every earlier successful backup.
 *  11. mono transaction failure — restores every managed multi Skill after
 *                                 later backup or mono publish failure.
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
  publishCacheAtomically,
  publishManagedMonoSkillSetAtomically,
  publishManagedMultiSkillSetAtomically,
} = require(installJsSource);
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

function runInstall(pkg, home, skillMode) {
  const env = { ...process.env, HOME: home };
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
              if (src.includes(".dws-multi-set.tmp-") && path.basename(src) === "dingtalk-second") {
                throw new Error("injected second publish failure");
              }
              originalRename(src, dest);
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
                src.includes(".dws-mono-set.tmp-") &&
                path.basename(src) === "dws"
              ) {
                throw new Error("injected mono publish failure");
              }
              originalRename(src, target);
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

for (const [name, fn] of scenarios) {
  process.stdout.write(`• ${name} ... `);
  fn();
  process.stdout.write("ok\n");
}
console.log(`OK — ${scenarios.length} install.js smoke scenarios passed`);
