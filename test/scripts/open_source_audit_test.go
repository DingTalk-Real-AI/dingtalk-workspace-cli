package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicRepositoryAssetsExist(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}

	for _, rel := range []string{
		".github/workflows/ci.yml",
		".github/PULL_REQUEST_TEMPLATE.md",
		".env.example",
		"docs/architecture.md",
		"scripts/policy/open-source-audit.sh",
	} {
		full := filepath.Join(root, rel)
		if _, err := os.Stat(full); err != nil {
			t.Fatalf("Stat(%s) error = %v", full, err)
		}
	}
}

func TestOpenSourceAuditScriptPasses(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "policy", "check-open-source-assets.sh"))
	if err != nil {
		t.Fatalf("Abs(check-open-source-assets.sh) error = %v", err)
	}

	cmd := exec.Command("sh", scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check-open-source-assets.sh error = %v\noutput:\n%s", err, string(output))
	}
	if !strings.Contains(string(output), "open-source audit: ok") {
		t.Fatalf("audit output missing success marker:\n%s", string(output))
	}
}

func TestGeneratedDriftCheckPassesForCleanCheckout(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "policy", "check-generated-drift.sh"))
	if err != nil {
		t.Fatalf("Abs(check-generated-drift.sh) error = %v", err)
	}

	cmd := exec.Command("sh", scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check-generated-drift.sh error = %v\noutput:\n%s", err, string(output))
	}
	if !strings.Contains(string(output), "generated drift check: ok") {
		t.Fatalf("drift output missing success marker:\n%s", string(output))
	}
}

func TestEmbeddedHostCompatibilitySourcesAreRemoved(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}

	for _, rel := range []string{
		filepath.Join("internal", "hostcompat"),
		filepath.Join("internal", "app", "skill_command.go"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Fatalf("%s should not exist in OSS repository, stat err = %v", rel, err)
		}
	}

	forbiddenSnippets := []string{
		"buildMode",
		"EmbeddedMode",
		"WriteTokenMarker",
		"CleanTokenOnExpiry",
		"DeleteExeRelativeTokenOnAuthErr",
		"shouldDeleteEmbeddedTokenOnAuthError",
		"com.dingtalk.scenario.wukong",
		"认证信息已失效，请重新执行上一条命令（最多重试两次）",
	}

	err = filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, snippet := range forbiddenSnippets {
			if strings.Contains(text, snippet) {
				t.Fatalf("found forbidden OSS snippet %q in %s", snippet, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(internal) error = %v", err)
	}
}
