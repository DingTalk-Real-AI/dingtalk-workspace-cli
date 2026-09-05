//go:build windows

package upgrade

import "golang.org/x/sys/windows"

// windows.Rename uses MoveFileEx with replacement and does not touch a running
// executable because activation changes only the directory pointer.
func replacePointerPath(source, destination string) error {
	return windows.Rename(source, destination)
}
