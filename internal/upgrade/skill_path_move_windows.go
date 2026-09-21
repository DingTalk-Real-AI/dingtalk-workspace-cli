//go:build windows

package upgrade

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

func createSkillPathDirJunction(target, link string) (err error) {
	if target == "" {
		return fmt.Errorf("junction target is empty")
	}
	target, err = skillPathAbs(target)
	if err != nil {
		return fmt.Errorf("resolve junction target: %w", err)
	}
	targetPath := junctionSubstituteName(target)
	buffer, err := mountPointReparseBuffer(targetPath, target)
	if err != nil {
		return err
	}
	linkPath, err := windowsUTF16PtrFromString(link)
	if err != nil {
		return fmt.Errorf("encode junction path: %w", err)
	}

	if err := skillPathMkdir(link, 0o755); err != nil {
		return err
	}
	removeLink := true
	defer func() {
		if removeLink {
			if rmErr := windowsSkillPathRemove(link); rmErr != nil && !os.IsNotExist(rmErr) {
				cleanErr := fmt.Errorf("清理 junction 占位目录失败 %s: %w", link, rmErr)
				if err != nil {
					err = errors.Join(err, cleanErr)
				} else {
					err = cleanErr
				}
			}
		}
	}()

	var handle windows.Handle
	handle, err = windowsCreateFile(
		linkPath,
		windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		err = fmt.Errorf("open junction: %w", err)
		return err
	}
	defer windows.CloseHandle(handle)

	var returned uint32
	if devErr := windowsDeviceIoControl(
		handle,
		windows.FSCTL_SET_REPARSE_POINT,
		&buffer[0],
		uint32(len(buffer)),
		nil,
		0,
		&returned,
		nil,
	); devErr != nil {
		err = fmt.Errorf("set junction reparse point: %w", devErr)
		return err
	}
	removeLink = false
	return nil
}

func junctionSubstituteName(target string) string {
	var prefix, clean string
	if strings.HasPrefix(target, `\\?\UNC\`) {
		prefix = `\??\UNC\`
		clean = strings.TrimPrefix(target, `\\?\UNC\`)
	} else if strings.HasPrefix(target, `\\?\`) {
		prefix = `\??\`
		clean = strings.TrimPrefix(target, `\\?\`)
	} else if strings.HasPrefix(target, `\\`) {
		prefix = `\??\UNC\`
		clean = strings.TrimPrefix(target, `\\`)
	} else {
		prefix = `\??\`
		clean = target
	}
	result := prefix + clean
	if !strings.HasSuffix(result, `\`) {
		result += `\`
	}
	return result
}

func mountPointReparseBuffer(substitute, printName string) ([]byte, error) {
	substituteUTF16, err := windows.UTF16FromString(substitute)
	if err != nil {
		return nil, fmt.Errorf("encode junction substitute name: %w", err)
	}
	printUTF16, err := windows.UTF16FromString(printName)
	if err != nil {
		return nil, fmt.Errorf("encode junction print name: %w", err)
	}

	substituteNameLength := uint16(2 * (len(substituteUTF16) - 1))
	printNameOffset := uint16(2 * len(substituteUTF16))
	printNameLength := uint16(2 * (len(printUTF16) - 1))

	pathBufferLength := 2*len(substituteUTF16) + 2*len(printUTF16)
	reparseDataLength := uint16(8 + pathBufferLength)
	buffer := make([]byte, 8+int(reparseDataLength))

	binary.LittleEndian.PutUint32(buffer[0:], windows.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(buffer[4:], reparseDataLength)
	binary.LittleEndian.PutUint16(buffer[6:], 0) // Reserved
	binary.LittleEndian.PutUint16(buffer[8:], 0) // SubstituteNameOffset
	binary.LittleEndian.PutUint16(buffer[10:], substituteNameLength)
	binary.LittleEndian.PutUint16(buffer[12:], printNameOffset)
	binary.LittleEndian.PutUint16(buffer[14:], printNameLength)

	pathBuffer := buffer[16:]
	for i, value := range substituteUTF16 {
		binary.LittleEndian.PutUint16(pathBuffer[i*2:], value)
	}
	for i, value := range printUTF16 {
		binary.LittleEndian.PutUint16(pathBuffer[int(printNameOffset)+i*2:], value)
	}
	return buffer, nil
}
