//go:build !windows

package upgrade

import (
	"os"
	"syscall"
)

func testCrossDeviceError() error {
	return &os.LinkError{Op: "rename", Old: "src", New: "dst", Err: syscall.EXDEV}
}

func testNoReplaceUnsupportedErrors() []error {
	return []error{syscall.EINVAL, syscall.ENOSYS, errNoReplaceRenameUnsupported}
}
