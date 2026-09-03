//go:build windows

package upgrade

import "golang.org/x/sys/windows"

func renameSkillPathNoReplaceAtomic(source, destination string) error {
	sourcePtr, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	destinationPtr, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFile(sourcePtr, destinationPtr)
}

// MoveFile without MOVEFILE_REPLACE_EXISTING already refuses an occupied
// destination on every supported Windows version, so there is nothing to
// degrade to.
func isNoReplaceRenameUnsupported(error) bool { return false }
