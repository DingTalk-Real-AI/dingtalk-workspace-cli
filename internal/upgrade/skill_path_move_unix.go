//go:build !windows

package upgrade

import (
	"errors"
	"os"
	"syscall"
)

func isCrossDeviceError(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

func isSkillPathLink(mode os.FileMode) bool {
	return mode&os.ModeSymlink != 0
}
