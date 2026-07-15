package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyGiteeReleaseRequiresEveryAsset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		remote    string
		wantError bool
	}{
		{
			name:   "complete",
			remote: `{"assets":[{"name":"checksums.txt"},{"name":"dws-linux-amd64.tar.gz"},{"name":"dws-skills.zip"}]}`,
		},
		{
			name:      "missing remote asset",
			remote:    `{"assets":[{"name":"checksums.txt"},{"name":"dws-linux-amd64.tar.gz"}]}`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			distDir := filepath.Join(root, "dist")
			for _, name := range []string{"checksums.txt", "dws-linux-amd64.tar.gz", "dws-skills.zip"} {
				mustWriteFile(t, filepath.Join(distDir, name), []byte(name), 0o644)
			}
			stubRoot := filepath.Join(root, "stubs")
			mustWriteFile(t, filepath.Join(stubRoot, "curl"), []byte("#!/bin/sh\nprintf '%s' \"$FAKE_GITEE_RELEASE\"\n"), 0o755)

			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "verify-gitee-release.sh"))
			if err != nil {
				t.Fatalf("Abs(verify-gitee-release.sh) error = %v", err)
			}
			cmd := exec.Command("sh", scriptPath)
			cmd.Env = append(os.Environ(),
				"PATH="+stubRoot+":"+os.Getenv("PATH"),
				"VERSION=v1.2.3",
				"DIST_DIR="+distDir,
				"GITEE_REQUIRED_ASSETS=checksums.txt dws-linux-amd64.tar.gz dws-skills.zip",
				"GITEE_VERIFY_RETRIES=1",
				"FAKE_GITEE_RELEASE="+tt.remote,
			)
			output, err := cmd.CombinedOutput()
			if tt.wantError {
				if err == nil {
					t.Fatalf("verify-gitee-release.sh error = nil, want failure\noutput:\n%s", string(output))
				}
				if !strings.Contains(string(output), "dws-skills.zip(count=0)") {
					t.Fatalf("missing asset not reported:\n%s", string(output))
				}
				return
			}
			if err != nil {
				t.Fatalf("verify-gitee-release.sh error = %v\noutput:\n%s", err, string(output))
			}
			if !strings.Contains(string(output), "Gitee release v1.2.3 is complete") {
				t.Fatalf("success output missing completeness result:\n%s", string(output))
			}
		})
	}
}

func TestGiteePublishersRunCompletenessVerification(t *testing.T) {
	t.Parallel()
	for _, relPath := range []string{
		filepath.Join("..", "..", "scripts", "release", "sync-to-gitee.sh"),
		filepath.Join("..", "..", "scripts", "release", "publish-gitee-local.sh"),
		filepath.Join("..", "..", ".github", "workflows", "verify-gitee-release.yml"),
	} {
		data, err := os.ReadFile(relPath)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", relPath, err)
		}
		if !strings.Contains(string(data), "verify-gitee-release.sh") {
			t.Fatalf("%s does not run verify-gitee-release.sh", relPath)
		}
	}
}
