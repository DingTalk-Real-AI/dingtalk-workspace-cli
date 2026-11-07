//go:build !windows

package upgrade

import (
	"os"
	"syscall"
	"testing"
)

func testCrossDeviceError() error {
	return &os.LinkError{Op: "rename", Old: "src", New: "dst", Err: syscall.EXDEV}
}

func testNoReplaceUnsupportedErrors() []error {
	return []error{syscall.EINVAL, syscall.ENOSYS, errNoReplaceRenameUnsupported}
}

func createTestDirLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
