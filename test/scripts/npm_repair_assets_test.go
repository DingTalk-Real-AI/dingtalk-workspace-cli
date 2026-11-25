package scripts_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Run both real workflow gates in sequence. The sealed contract fixtures model
// the complete 8-asset v1.0.63-beta.3 and 9-asset v1.0.69 releases; no network or
// publication is used. Deliberately incompatible tooling must never be invoked.
func TestNPMRepairPreparesAssetsWithSealedVersionContract(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX release preparation")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node required")
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string                  `yaml:"name"`
				Run  string                  `yaml:"run"`
				With struct{ Script string } `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(readReleaseWorkflow(t)), &workflow); err != nil {
		t.Fatal(err)
	}
	authority, download := "", ""
	for _, step := range workflow.Jobs["prepare-npm-repair"].Steps {
		switch step.Name {
		case "Verify immutable release authority":
			authority = step.With.Script
		case "Download and verify GitHub release assets":
			download = step.Run
		}
	}
	if authority == "" || download == "" {
		t.Fatal("npm repair preparation gates missing")
	}
	helper, err := os.ReadFile(filepath.Join("..", "..", "scripts", "release", "verify-sealed-release-assets.cjs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"v1.0.63-beta.3", "v1.0.69"} {
		for _, mode := range []string{"valid", "wrong-layout", "duplicate", "unsafe-name", "extra-asset", "corrupt-bytes", "download-fails", "sealed-verifier-fails"} {
			t.Run(version+"/"+mode, func(t *testing.T) {
				root := t.TempDir()
				assets := append([]string{}, releasePlatformAssets...)
				assets = append(assets, "dws-skills.zip")
				if version == "v1.0.69" {
					assets = append(assets, "dws-openharmony-arm64.tar.gz")
				}
				expected := append(append([]string{}, assets...), "checksums.txt")
				sort.Strings(expected)
				if mode == "wrong-layout" {
					if version == "v1.0.69" {
						assets = assets[:len(assets)-1]
					} else {
						assets = append(assets, "dws-openharmony-arm64.tar.gz")
					}
				} else if mode == "extra-asset" {
					assets = append(assets, "dws-unknown.tar.gz")
				}
				fixture := filepath.Join(root, "download-fixture")
				var checksums strings.Builder
				for _, asset := range assets {
					body := []byte(version + ":" + asset)
					mustWriteFile(t, filepath.Join(fixture, asset), body, 0o644)
					fmt.Fprintf(&checksums, "%x  %s\n", sha256.Sum256(body), asset)
				}
				mustWriteFile(t, filepath.Join(fixture, "checksums.txt"), []byte(checksums.String()), 0o644)
				if mode == "corrupt-bytes" {
					mustWriteFile(t, filepath.Join(fixture, assets[0]), []byte("corrupt"), 0o644)
				}
				names := append(append([]string{}, assets...), "checksums.txt")
				if mode == "duplicate" {
					names = append(names, names[0])
				} else if mode == "unsafe-name" {
					names = append(names, "../outside.tar.gz")
				}
				var releaseAssets []map[string]string
				for _, name := range names {
					releaseAssets = append(releaseAssets, map[string]string{"name": name})
				}
				release, _ := json.Marshal(map[string]any{"tag_name": version, "draft": false, "immutable": true, "prerelease": strings.Contains(version, "-beta."), "assets": releaseAssets})
				command := exec.Command(node, "--input-type=module", "-e", giteeRepairAuthorityHarness)
				command.Env = append(os.Environ(), "REPAIR_AUTHORITY_SCRIPT="+authority, "REPAIR_AUTHORITY_RELEASE="+string(release))
				output, err := command.CombinedOutput()
				if err != nil {
					t.Fatalf("authority: %v\n%s", err, output)
				}
				var result struct {
					Failures []string          `json:"failures"`
					Outputs  map[string]string `json:"outputs"`
				}
				if err := json.Unmarshal(output, &result); err != nil {
					t.Fatal(err)
				}
				if mode == "duplicate" || mode == "unsafe-name" {
					if len(result.Failures) == 0 || len(result.Outputs) != 0 {
						t.Fatalf("unsafe download inventory accepted: %+v", result)
					}
					return
				}
				if len(result.Failures) != 0 || result.Outputs["asset_names"] == "" || result.Outputs["commit_sha"] != strings.Repeat("b", 40) {
					t.Fatalf("sealed release authority rejected: %+v", result)
				}
				tooling := filepath.Join(root, "tooling", "scripts", "release")
				mustWriteFile(t, filepath.Join(tooling, "verify-sealed-release-assets.cjs"), helper, 0o644)
				mustWriteFile(t, filepath.Join(tooling, "verify-release-artifacts.sh"), []byte("#!/bin/sh\nexit 97\n"), 0o755)
				sealed := `#!/bin/sh
set -eu
test "$1" = "$VERSION"
test -z "${ACTIONS_ID_TOKEN_REQUEST_URL:-}"
printf '%s\n' ` + "'" + strings.Join(expected, "\n") + "'" + ` > "$GITHUB_WORKSPACE/expected-assets"
find "$DWS_PACKAGE_DIST_DIR" -maxdepth 1 -type f -exec basename {} \; | LC_ALL=C sort > "$GITHUB_WORKSPACE/actual-assets"
diff -u "$GITHUB_WORKSPACE/expected-assets" "$GITHUB_WORKSPACE/actual-assets"
(cd "$DWS_PACKAGE_DIST_DIR" && shasum -a 256 -c checksums.txt)
test "$SCENARIO" != sealed-verifier-fails
printf '%s\n' "$VERSION" > "$GITHUB_WORKSPACE/verified-version"
`
				mustWriteFile(t, filepath.Join(root, "release-source", "scripts", "release", "verify-release-artifacts.sh"), []byte(sealed), 0o755)
				bin := filepath.Join(root, "bin")
				mustWriteFile(t, filepath.Join(bin, "gh"), []byte(`#!/bin/sh
set -eu
test "$1 $2 $3" = "release download $VERSION"
test "$SCENARIO" != download-fails
shift 3
while [ "$#" -gt 0 ]; do
  case "$1" in
    --pattern) echo 'must download the complete immutable inventory' >&2; exit 98 ;;
    --dir) shift; dist="$1" ;;
  esac
  shift
done
cp "$DOWNLOAD_FIXTURE"/* "$dist/"
`), 0o755)
				command = exec.Command("bash", "-c", download)
				command.Dir = root
				command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "TMPDIR="+root,
					"GITHUB_WORKSPACE="+root, "GITHUB_REPOSITORY=owner/repo", "VERSION="+version, "DOWNLOAD_FIXTURE="+fixture,
					"DWS_RELEASE_ASSET_NAMES="+result.Outputs["asset_names"], "ACTIONS_ID_TOKEN_REQUEST_URL=", "SCENARIO="+mode)
				output, err = command.CombinedOutput()
				if (err == nil) != (mode == "valid") {
					t.Fatalf("preparation mode=%s err=%v\n%s", mode, err, output)
				}
				verified, readErr := os.ReadFile(filepath.Join(root, "verified-version"))
				if mode == "valid" {
					if readErr != nil || strings.TrimSpace(string(verified)) != version {
						t.Fatalf("wrong sealed verifier result: %q %v", verified, readErr)
					}
				} else if !os.IsNotExist(readErr) {
					t.Fatal("failed preparation reached a verified handoff")
				}
			})
		}
	}
}
