//go:build windows

package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/junction"
	"golang.org/x/sys/windows"
)

var (
	filepathAbs               = filepath.Abs
	windowsUTF16PtrFromString = windows.UTF16PtrFromString
	windowsCreateFile         = windows.CreateFile
	windowsDeviceIoControl    = windows.DeviceIoControl
	windowsOsRemove           = os.Remove
)

func createSkillSetupDirLink(target, link string) error {
	err := junction.Create(target, link, junction.Ops{
		Abs:                filepathAbs,
		Mkdir:              os.Mkdir,
		Remove:             windowsOsRemove,
		UTF16PtrFromString: windowsUTF16PtrFromString,
		CreateFile:         windowsCreateFile,
		DeviceIoControl:    windowsDeviceIoControl,
	})
	var cleanupErr *junction.CleanupError
	if !errors.As(err, &cleanupErr) {
		return err
	}
	return errors.Join(err, &skillSetupStagingCleanupError{
		Path: cleanupErr.Path,
		Err:  fmt.Errorf("清理 junction 占位目录失败 %s: %w", cleanupErr.Path, cleanupErr.Err),
	})
}

func junctionSubstituteName(target string) string {
	return junction.JunctionSubstituteName(target)
}

func mountPointReparseBuffer(substitute, printName string) ([]byte, error) {
	return junction.MountPointReparseBuffer(substitute, printName)
}

func skillSetupLinkTarget(realTarget, relativeTarget string) string {
	return realTarget
}

func isSkillSetupCurrentCanonicalAdapter(path, canonicalTarget string) bool {
	info, err := skillSetupLstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular == 0 {
		// Junctions report ModeIrregular; symlinks report ModeSymlink and must
		// be upgraded; ordinary directories are not adapters.
		return false
	}
	linkTarget, err := skillSetupReadlink(path)
	if err != nil {
		return false
	}
	// filepath.EvalSymlinks does not follow junctions (they surface as
	// ModeIrregular, not ModeSymlink), so physical equality must be checked
	// on the junction's stored target. Both sides resolve through EvalSymlinks
	// so 8.3 short path components and symlinked parents normalize identically.
	realCanonical, err := skillSetupEvalSymlinks(canonicalTarget)
	if err != nil {
		return false
	}
	realLink, err := skillSetupEvalSymlinks(filepath.Clean(linkTarget))
	if err != nil {
		// The stored target no longer resolves; the junction must be rebuilt.
		return false
	}
	if !sameSkillSetupPath(realLink, realCanonical) {
		return false
	}
	return validateSkillSetupLink(path) == nil
}
