//go:build windows

package upgrade

import (
	"errors"

	"golang.org/x/sys/windows"
)

func isCrossDeviceError(err error) bool {
	return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE)
}
