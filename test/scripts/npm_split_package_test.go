package scripts_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNPMPlatformPackagesContainOnlyTheirOwnResources(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node required")
	}
	dist := t.TempDir()
	names := []string{"dws-darwin-arm64.tar.gz", "dws-darwin-amd64.tar.gz", "dws-linux-arm64.tar.gz", "dws-linux-amd64.tar.gz", "dws-windows-arm64.zip", "dws-windows-amd64.zip", "dws-skills.zip"}
	var checksums strings.Builder
	for _, name := range names {
		content := []byte("fixture for " + name)
		if err := os.WriteFile(filepath.Join(dist, name), content, 0600); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&checksums, "%x  %s\n", sha256.Sum256(content), name)
	}
	if err := os.WriteFile(filepath.Join(dist, "checksums.txt"), []byte(checksums.String()), 0600); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("../../scripts/release/stage-npm-splits.cjs")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, script, dist, "1.2.3")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("stage: %v %s", err, out)
	}
	var index struct {
		Packages []struct{ Name, Directory, Asset string } `json:"packages"`
	}
	data, err := os.ReadFile(filepath.Join(dist, "npm-split/index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Packages) != 7 {
		t.Fatalf("packages = %d", len(index.Packages))
	}
	for _, pkg := range index.Packages {
		if pkg.Name != "dingtalk-workspace-cli-"+pkg.Directory {
			t.Fatalf("name: %s", pkg.Name)
		}
		entries, err := os.ReadDir(filepath.Join(dist, "npm-split", pkg.Directory, "assets"))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			t.Fatalf("%s contains unrelated platform files", pkg.Name)
		}
		for _, entry := range entries {
			if entry.Name() != pkg.Asset && entry.Name() != "checksums.txt" {
				t.Fatalf("unexpected asset %s", entry.Name())
			}
		}
		manifestData, err := os.ReadFile(filepath.Join(dist, "npm-split", pkg.Directory, "package.json"))
		if err != nil {
			t.Fatal(err)
		}
		var manifest map[string]any
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest["scripts"] != nil {
			t.Fatal("split package has executable lifecycle scripts")
		}
		if pkg.Directory == "win32-x64" && (manifest["os"].([]any)[0] != "win32" || manifest["cpu"].([]any)[0] != "x64") {
			t.Fatal("wrong npm platform names")
		}
	}
	cmd = exec.Command(node, script, dist, "1.2.3-internal.1")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "only public") {
		t.Fatalf("private publication target accepted: %v %s", err, out)
	}
}
