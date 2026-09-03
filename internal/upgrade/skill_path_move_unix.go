//go:build !windows

package upgrade

import (
	"errors"
	"syscall"
)

func isCrossDeviceError(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}
