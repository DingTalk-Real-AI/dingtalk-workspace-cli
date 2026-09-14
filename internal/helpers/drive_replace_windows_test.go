//go:build windows

package helpers

import (
	"os"
	"path/filepath"
	"testing"
)

// Windows 必须通过 MOVEFILE_REPLACE_EXISTING 原子替换既有非目录目标；该路径同时
// 服务 pull overwrite、sync remote-wins 与 keep-both 的占位文件替换。
func TestCrossPlatformCoverageDriveReplaceFileWindows_replacesExistingTarget(t *testing.T) {
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
