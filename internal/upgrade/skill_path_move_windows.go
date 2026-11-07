//go:build windows

package upgrade

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/junction"
	"golang.org/x/sys/windows"
)

var (
	skillPathAbs              = filepath.Abs
	windowsUTF16PtrFromString = windows.UTF16PtrFromString
	windowsCreateFile         = windows.CreateFile
	windowsDeviceIoControl    = windows.DeviceIoControl
	windowsSkillPathRemove    = skillPathRemove
)

func isCrossDeviceError(err error) bool {
	return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE)
}

func isSkillPathLink(mode os.FileMode) bool {
	return mode&os.ModeSymlink != 0 || mode&os.ModeIrregular != 0
}

func copySkillPathLink(target, dst string, mode os.FileMode) error {
	if mode&os.ModeSymlink != 0 {
		return skillPathSymlink(target, dst)
	}
	return createSkillPathDirJunction(target, dst)
}

func junctionSubstituteName(target string) string {
	return junction.JunctionSubstituteName(target)
}

func mountPointReparseBuffer(substitute, printName string) ([]byte, error) {
	return junction.MountPointReparseBuffer(substitute, printName)
}

func createSkillPathDirJunction(target, link string) error {
	return junction.Create(target, link, junction.Ops{
		Abs:                skillPathAbs,
		Mkdir:              skillPathMkdir,
		Remove:             windowsSkillPathRemove,
		UTF16PtrFromString: windowsUTF16PtrFromString,
		CreateFile:         windowsCreateFile,
		DeviceIoControl:    windowsDeviceIoControl,
	})
}

func upgradeEntryIsDirLike(entry os.DirEntry) bool {
	return entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || entry.Type()&os.ModeIrregular != 0
}

func createUpgradeSkillLink(realTarget, _, link string) error {
	return createSkillPathDirJunction(realTarget, link)
}

func upgradeResolvePhysicalSkillRoot(path string) (string, error) {
	info, err := upgradeLstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 && info.Mode()&os.ModeIrregular != 0 {
		target, err := upgradeReadlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		return upgradeEvalSymlinks(filepath.Clean(target))
	}
	return upgradeEvalSymlinks(path)
}
