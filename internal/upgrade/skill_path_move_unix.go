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

func copySkillPathLink(target, dst string, _ os.FileMode) error {
	return skillPathSymlink(target, dst)
}

func upgradeEntryIsDirLike(entry os.DirEntry) bool {
	return entry.IsDir() || entry.Type()&os.ModeSymlink != 0
}

func createUpgradeSkillLink(_, relativeTarget, link string) error {
	return upgradeSymlink(relativeTarget, link)
}

func upgradeResolvePhysicalSkillRoot(path string) (string, error) {
	return upgradeEvalSymlinks(path)
}
