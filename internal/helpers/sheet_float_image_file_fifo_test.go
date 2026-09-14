//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package helpers

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestCrossPlatformCoverageFloatImageFIFORejectedBeforeOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	if _, err := openFloatImageLocalFile(path); err == nil || !strings.Contains(err.Error(), "普通文件") {
		t.Fatalf("FIFO error = %v", err)
	}
}
