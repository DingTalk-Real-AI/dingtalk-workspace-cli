package scripts_test

import (
	"os/exec"
	"path/filepath"
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
		for _, required := range []string{"DWS_VERIFY_NPM_TARBALL=", "verify-package-managers.sh", "Require unchanged verified tarballs", "actions/upload-artifact@v4", "digest: ${{ steps.package.outputs.digest }}"} {
			if !strings.Contains(preparation, required) {
				t.Errorf("%s missing %s", pair.preparation, required)
			}
		}
		for _, required := range []string{"id-token: write", "ref: ${{ github.sha }}", "actions/download-artifact@v4", "needs." + pair.preparation + ".outputs.digest", "npm-handoff-digest.cjs", "npm publish", "--ignore-scripts"} {
			if !strings.Contains(publisher, required) {
				t.Errorf("%s missing %s", pair.publisher, required)
			}
		}
		for _, forbidden := range []string{"verify-package-managers.sh", "pack-npm-package.sh", "npm install -g", "ref: ${{ needs.release-contract.outputs.release_commit }}", "ref: ${{ steps.authority.outputs.commit_sha }}"} {
			if strings.Contains(publisher, forbidden) {
				t.Errorf("publisher executes target code: %s", forbidden)
			}
		}
		if strings.Index(publisher, "Verify sealed npm handoff") > strings.Index(publisher, "npm publish") {
			t.Fatal("publication precedes handoff verification")
		}
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
const run=(expected)=>cp.spawnSync(process.execPath,[tool,root,...(expected?[expected]:[])],{encoding:'utf8'});
const before=run(); assert.strictEqual(before.status,0); const digest=before.stdout.trim();
assert.strictEqual(run(digest).status,0);
fs.writeFileSync(file,'changed tarball'); assert.notStrictEqual(run(digest).status,0);
fs.writeFileSync(file,'verified tarball'); fs.renameSync(file,path.join(root,'other.tgz')); assert.notStrictEqual(run(digest).status,0);
`
	if out, err := exec.Command(node, "-e", code, script, t.TempDir()).CombinedOutput(); err != nil {
		t.Fatalf("handoff: %v %s", err, out)
	}
}
