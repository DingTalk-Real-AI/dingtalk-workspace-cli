//go:build windows

package upgrade

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func isCrossDeviceError(err error) bool {
	return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE)
}

func isSkillPathLink(mode os.FileMode) bool {
	return mode&os.ModeSymlink != 0 || mode&os.ModeIrregular != 0
}
