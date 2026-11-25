package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNPMReleaseHandoffSeparatesPublishingIdentity(t *testing.T) {
	workflow := readReleaseWorkflow(t)
	for _, pair := range []struct{ publisher, preparation, end string }{
		{"publish-channels", "prepare-npm", "mirror-gitee-release"},
		{"repair-npm", "prepare-npm-repair", "prepare-npm"},
	} {
		publisher := releaseWorkflowSection(t, workflow, "  "+pair.publisher+":\n", "\n  "+pair.end+":\n")
		end := "prepare-npm-repair"
		if pair.preparation == end {
			end = "coverage-baseline-confirmation"
		}
		preparation := releaseWorkflowSection(t, workflow, "  "+pair.preparation+":\n", "\n  "+end+":\n")
		if strings.Contains(preparation, "id-token: write") || strings.Contains(preparation, "npm publish") {
			t.Fatal("installation verification holds publication authority")
		}
		for _, required := range []string{"DWS_VERIFY_NPM_TARBALL=", "verify-sealed-npm-package.sh", "Require unchanged verified tarballs", "actions/upload-artifact@v4", "digest: ${{ steps.package.outputs.digest }}", "retention-days: 31"} {
			if !strings.Contains(preparation, required) {
				t.Errorf("%s missing %s", pair.preparation, required)
			}
		}
		for _, required := range []string{"id-token: write", "ref: ${{ github.sha }}", "actions/download-artifact@v4", "needs." + pair.preparation + ".outputs.digest", "npm-handoff-digest.cjs", "npm publish", "--ignore-scripts", "verify-npm-publication-authority.cjs"} {
			if !strings.Contains(publisher, required) {
				t.Errorf("%s missing %s", pair.publisher, required)
			}
		}
		for _, forbidden := range []string{"verify-package-managers.sh", "verify-sealed-npm-package.sh", "pack-npm-package.sh", "npm install -g", "ref: ${{ needs.release-contract.outputs.release_commit }}", "ref: ${{ steps.authority.outputs.commit_sha }}"} {
			if strings.Contains(publisher, forbidden) {
				t.Errorf("publisher executes target code: %s", forbidden)
			}
		}
		if strings.Index(publisher, "Verify sealed npm handoff") > strings.Index(publisher, "npm publish") {
			t.Fatal("publication precedes handoff verification")
		}
		if strings.Index(publisher, "Revalidate publication authority") > strings.Index(publisher, "npm publish") {
			t.Fatal("publication precedes live withdrawal/tag/release verification")
		}
	}
}

func TestNPMPublicationRevalidatesWithdrawalAndSealedIdentity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node required")
	}
	script, err := filepath.Abs("../../scripts/release/verify-npm-publication-authority.cjs")
	if err != nil {
		t.Fatal(err)
	}
	code := `
const assert=require('assert'),verify=require(process.argv[1]);
const commit='a'.repeat(40),tagObject='b'.repeat(40);
async function run(mode,version='v1.2.3') {
  const github={rest:{git:{
    async getRef({ref}) {
      if(ref.startsWith('tags/withdrawn/')) {
        if(mode==='withdrawn') return {data:{}};
        throw Object.assign(new Error('ref unavailable'),{status:mode==='denied'?403:404});
      }
      return {data:{object:{type:mode==='lightweight'?'commit':'tag',sha:mode==='tag-changed'?'c'.repeat(40):tagObject}}};
    },
    async getTag() {return {data:{tag:version,object:{type:'commit',sha:mode==='commit-changed'?'d'.repeat(40):commit}}};}
  },repos:{async getReleaseByTag() {return {data:{tag_name:mode==='wrong-release'?'v0.0.0':version,draft:mode==='draft',immutable:mode!=='mutable',prerelease:mode==='wrong-channel'?!version.includes('-beta.'):version.includes('-beta.')}};}}}};
  return verify({github,context:{repo:{owner:'owner',repo:'repo'}},version,commit,tagObject:mode==='missing-seal'?'':tagObject});
}
(async()=>{
  await run('valid'); await run('valid','v1.2.3-beta.1');
  for(const mode of ['withdrawn','denied','lightweight','tag-changed','commit-changed','wrong-release','draft','mutable','wrong-channel','missing-seal']) {
    await assert.rejects(()=>run(mode),undefined,mode+' must reject a stale handoff');
  }
})().catch(e=>{console.error(e);process.exitCode=1});
`
	if out, err := exec.Command(node, "-e", code, script).CombinedOutput(); err != nil {
		t.Fatalf("publication authority: %v %s", err, out)
	}
}

func TestNPMPosixShimPreservesArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable shim")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node required")
	}
	script, err := filepath.Abs("../../scripts/release/publish-npm-splits.cjs")
	if err != nil {
		t.Fatal(err)
	}
	code := `
const fs=require('fs'),path=require('path'),assert=require('assert');
const root=process.argv[2];
fs.writeFileSync(path.join(root,'npm'),'#!/bin/sh\nprintf "%s\\n" "$@"\n',{mode:0o755});
process.env.PATH=root; delete process.env.npm_execpath;
const {npm}=require(process.argv[1]);
const args=['pack','a path & literal $(no-shell).tgz','--ignore-scripts'];
const result=npm(args,root);
assert.strictEqual(result.status,0);assert.deepStrictEqual(result.stdout.trimEnd().split('\n'),args);
`
	if out, err := exec.Command(node, "-e", code, script, t.TempDir()).CombinedOutput(); err != nil {
		t.Fatalf("POSIX npm shim: %v %s", err, out)
	}
}

func TestNPMSealedVerifierUsesHistoricalContractAndExactTarball(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX release preparation")
	}
	script, err := filepath.Abs("../../scripts/release/verify-sealed-npm-package.sh")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	source := filepath.Join(root, "sealed source")
	dist := filepath.Join(root, "dist")
	stage := filepath.Join(dist, "npm", "dingtalk-workspace-cli")
	bin := filepath.Join(root, "bin")
	for _, directory := range []string{filepath.Join(source, "scripts", "release"), stage, bin} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tarball := filepath.Join(dist, "npm-publish.tgz")
	mustWriteFile(t, tarball, []byte("the original sealed tarball"), 0o644)
	mustWriteFile(t, filepath.Join(stage, "package.json"), []byte(`{"version":"1.0.68"}`), 0o644)
	mustWriteFile(t, filepath.Join(source, "scripts", "release", "verify-package-managers.sh"), []byte(`#!/bin/sh
set -eu
test "$1" = --npm-only
test "$2" = --expected-version
test "$3" = v1.0.68
stage="$DWS_PACKAGE_DIST_DIR/npm/dingtalk-workspace-cli"
name="$(cd "$stage" && npm pack --silent)"
HOME="$VERIFY_TEST_HOME" npm install -g "$stage/$name"
test -f "$VERIFY_TEST_HOME/.legacy-agent/skills/verified"
test ! -e "$VERIFY_TEST_HOME/.workbuddy"
rm "$stage/$name"
`), 0o755)
	mustWriteFile(t, filepath.Join(bin, "npm"), []byte(`#!/bin/sh
set -eu
test "$1" = install
cmp -s "$3" "$DWS_VERIFY_FIXED_TARBALL"
mkdir -p "$HOME/.legacy-agent/skills"
touch "$HOME/.legacy-agent/skills/verified"
`), 0o755)
	cmd := exec.Command("sh", script, source, "v1.0.68")
	cmd.Env = []string{
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TMPDIR=" + root, "DWS_PACKAGE_DIST_DIR=" + dist,
		"DWS_VERIFY_NPM_TARBALL=" + tarball, "VERIFY_TEST_HOME=" + filepath.Join(root, "home"),
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("historical install contract: %v %s", err, out)
	}
	if got, err := os.ReadFile(tarball); err != nil || string(got) != "the original sealed tarball" {
		t.Fatalf("sealed tarball changed: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(stage, "dws-verified-input.tgz")); !os.IsNotExist(err) {
		t.Fatalf("verification copy was not removed: %v", err)
	}
}

func TestNPMMainAnchorAndHistoricalStaging(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node required")
	}
	script, err := filepath.Abs("../../scripts/release/stage-npm-splits.cjs")
	if err != nil {
		t.Fatal(err)
	}
	code := `
const fs=require('fs'), path=require('path'), crypto=require('crypto'), assert=require('assert');
const {anchorMain}=require(process.argv[1]), root=process.argv[2];
const dir=path.join(root,'npm','dingtalk-workspace-cli'); fs.mkdirSync(dir,{recursive:true});
const file=path.join(dir,'package.json');
const original='{ "name": "dingtalk-workspace-cli", "version": "1.2.3" }\n';
fs.writeFileSync(file,original);
assert.strictEqual(anchorMain(root),false);
assert.strictEqual(fs.readFileSync(file,'utf8'),original); // frozen bytes unchanged, even without resources
const asset='dws-skills.zip', bytes=Buffer.from('official release'), sha=crypto.createHash('sha256').update(bytes).digest('hex');
fs.writeFileSync(path.join(root,asset),bytes); fs.writeFileSync(path.join(root,'checksums.txt'),sha+'  '+asset+'\n');
fs.writeFileSync(file,JSON.stringify({name:'dingtalk-workspace-cli',version:'1.2.4',dwsUpgrade:{format:1,edition:'public',assets:{}}}));
assert.strictEqual(anchorMain(root),true);
assert.deepStrictEqual(JSON.parse(fs.readFileSync(file)).dwsUpgrade.assets,{[asset]:{sha256:sha,size:bytes.length}});
fs.writeFileSync(path.join(root,asset),'different');
assert.throws(()=>anchorMain(root),/checksum mismatch/);
`
	if out, err := exec.Command(node, "-e", code, script, t.TempDir()).CombinedOutput(); err != nil {
		t.Fatalf("anchor: %v %s", err, out)
	}
}

func TestNPMWindowsEntrypointPreservesArguments(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node required")
	}
	script, err := filepath.Abs("../../scripts/release/publish-npm-splits.cjs")
	if err != nil {
		t.Fatal(err)
	}
	code := `
const fs=require('fs'),path=require('path'),assert=require('assert');
const root=path.join(process.argv[2],'node with spaces & signs');
fs.mkdirSync(path.join(root,'node_modules','npm','bin'),{recursive:true});
fs.writeFileSync(path.join(root,'npm.cmd'),'exit /b 99');
const cli=path.join(root,'node_modules','npm','bin','npm-cli.js');
fs.writeFileSync(cli,'process.stdout.write(JSON.stringify(process.argv.slice(2)))');
Object.defineProperty(process,'platform',{value:'win32'}); process.env.PATH=root; delete process.env.npm_execpath;
const {npmEntrypoint,npm}=require(process.argv[1]);
assert.strictEqual(npmEntrypoint(),cli);
const args=['pack','a path & literal $(no-shell).tgz','--ignore-scripts'];
const result=npm(args,root); assert.strictEqual(result.status,0); assert.deepStrictEqual(JSON.parse(result.stdout),args);
`
	if out, err := exec.Command(node, "-e", code, script, t.TempDir()).CombinedOutput(); err != nil {
		t.Fatalf("Windows npm: %v %s", err, out)
	}
}

func TestNPMHandoffRejectsChangedBytesAndNames(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node required")
	}
	script, err := filepath.Abs("../../scripts/release/npm-handoff-digest.cjs")
	if err != nil {
		t.Fatal(err)
	}
	code := `
const fs=require('fs'),path=require('path'),assert=require('assert'),cp=require('child_process');
const tool=process.argv[1],root=process.argv[2],file=path.join(root,'npm-publish.tgz');
fs.writeFileSync(file,'verified tarball');
const installers=path.join(root,'oss-installers');fs.mkdirSync(installers);
const installer=path.join(installers,'install.sh');fs.writeFileSync(installer,'sealed installer');
const run=(expected)=>cp.spawnSync(process.execPath,[tool,root,...(expected?[expected]:[])],{encoding:'utf8'});
const before=run(); assert.strictEqual(before.status,0); const digest=before.stdout.trim();
assert.strictEqual(run(digest).status,0);
fs.writeFileSync(installer,'current tooling installer'); assert.notStrictEqual(run(digest).status,0);
fs.writeFileSync(installer,'sealed installer');
fs.writeFileSync(file,'changed tarball'); assert.notStrictEqual(run(digest).status,0);
fs.writeFileSync(file,'verified tarball'); fs.renameSync(file,path.join(root,'other.tgz')); assert.notStrictEqual(run(digest).status,0);
`
	if out, err := exec.Command(node, "-e", code, script, t.TempDir()).CombinedOutput(); err != nil {
		t.Fatalf("handoff: %v %s", err, out)
	}
}
