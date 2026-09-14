//go:build windows

package upgrade

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func testCrossDeviceError() error {
	return &os.LinkError{Op: "rename", Old: "src", New: "dst", Err: windows.ERROR_NOT_SAME_DEVICE}
}

func testNoReplaceUnsupportedErrors() []error {
	return []error{errNoReplaceRenameUnsupported}
}

func TestCrossPlatformCoverageWindowsCrossDeviceError(t *testing.T) {
	if !isCrossDeviceError(testCrossDeviceError()) {
		t.Fatal("ERROR_NOT_SAME_DEVICE must enter the cross-device fallback")
	}
}
