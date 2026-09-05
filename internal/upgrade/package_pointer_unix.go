//go:build !windows

package upgrade

import "os"

func replacePointerPath(source, destination string) error {
	return os.Rename(source, destination)
}
