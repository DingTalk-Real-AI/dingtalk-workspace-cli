//go:build !windows

package helpers

import (
	"os"
	"path/filepath"
	"testing"
)

// Unix 的 rename(2) 原生提供原子替换语义；该路径与 Windows 的
// MOVEFILE_REPLACE_EXISTING 同样服务 pull overwrite、sync remote-wins 与
// keep-both 的占位文件替换，因此两侧都需要平台覆盖。
func TestCrossPlatformCoverageDriveReplaceFileUnix_replacesExistingTarget(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.tmp")
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := driveReplaceFile(source, target); err != nil {
		t.Fatalf("replace existing target: %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "new" {
		t.Fatalf("target = %q, err=%v; want new content", got, err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source must be moved, stat err=%v", err)
	}
}
