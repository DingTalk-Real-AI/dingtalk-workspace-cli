//go:build windows

package junction

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

const maxMountPointReparseDataLength = 16*1024 - 8

type CleanupError struct {
	Path string
	Err  error
}

func (e *CleanupError) Error() string {
	return fmt.Sprintf("清理 junction 占位目录失败 %s: %v", e.Path, e.Err)
}

func (e *CleanupError) Unwrap() error {
	return e.Err
}

type Ops struct {
	Abs                func(string) (string, error)
	Mkdir              func(string, os.FileMode) error
	Remove             func(string) error
	UTF16PtrFromString func(string) (*uint16, error)
	CreateFile         func(*uint16, uint32, uint32, *windows.SecurityAttributes, uint32, uint32, windows.Handle) (windows.Handle, error)
	DeviceIoControl    func(windows.Handle, uint32, *byte, uint32, *byte, uint32, *uint32, *windows.Overlapped) error
}

func Create(target, link string, ops Ops) (err error) {
	if target == "" {
		return fmt.Errorf("junction target is empty")
	}
	if ops.Abs == nil || ops.Mkdir == nil || ops.Remove == nil || ops.UTF16PtrFromString == nil || ops.CreateFile == nil || ops.DeviceIoControl == nil {
		return errors.New("junction operations are incomplete")
	}

	target, err = ops.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve junction target: %w", err)
	}
	buffer, err := MountPointReparseBuffer(JunctionSubstituteName(target), target)
	if err != nil {
		return err
	}
	linkPath, err := ops.UTF16PtrFromString(link)
	if err != nil {
		return fmt.Errorf("encode junction path: %w", err)
	}
	if err := ops.Mkdir(link, 0o755); err != nil {
		return err
	}
	removeLink := true
	defer func() {
		if !removeLink {
			return
		}
		if rmErr := ops.Remove(link); rmErr != nil && !os.IsNotExist(rmErr) {
			err = errors.Join(err, &CleanupError{Path: link, Err: rmErr})
		}
	}()

	handle, err := ops.CreateFile(
		linkPath,
		windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return fmt.Errorf("open junction: %w", err)
	}
	defer windows.CloseHandle(handle)

	var returned uint32
	if err := ops.DeviceIoControl(
		handle,
		windows.FSCTL_SET_REPARSE_POINT,
		&buffer[0],
		uint32(len(buffer)),
		nil,
		0,
		&returned,
		nil,
	); err != nil {
		return fmt.Errorf("set junction reparse point: %w", err)
	}
	removeLink = false
	return nil
}

func JunctionSubstituteName(target string) string {
	var prefix, clean string
	switch {
	case strings.HasPrefix(target, `\\?\UNC\`):
		prefix = `\??\UNC\`
		clean = strings.TrimPrefix(target, `\\?\UNC\`)
	case strings.HasPrefix(target, `\\?\`):
		prefix = `\??\`
		clean = strings.TrimPrefix(target, `\\?\`)
	case strings.HasPrefix(target, `\\`):
		prefix = `\??\UNC\`
		clean = strings.TrimPrefix(target, `\\`)
	default:
		prefix = `\??\`
		clean = target
	}
	result := prefix + clean
	if !strings.HasSuffix(result, `\`) {
		result += `\`
	}
	return result
}

func MountPointReparseBuffer(substitute, printName string) ([]byte, error) {
	substituteUTF16, err := windows.UTF16FromString(substitute)
	if err != nil {
		return nil, fmt.Errorf("encode junction substitute name: %w", err)
	}
	printUTF16, err := windows.UTF16FromString(printName)
	if err != nil {
		return nil, fmt.Errorf("encode junction print name: %w", err)
	}

	substituteNameLength := 2 * (len(substituteUTF16) - 1)
	printNameOffset := 2 * len(substituteUTF16)
	printNameLength := 2 * (len(printUTF16) - 1)
	pathBufferLength := printNameOffset + 2*len(printUTF16)
	reparseDataLength := 8 + pathBufferLength
	if reparseDataLength > maxMountPointReparseDataLength {
		return nil, fmt.Errorf("junction path too long: reparse data length %d exceeds maximum %d", reparseDataLength, maxMountPointReparseDataLength)
	}

	buffer := make([]byte, 8+reparseDataLength)
	binary.LittleEndian.PutUint32(buffer[0:], windows.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(buffer[4:], uint16(reparseDataLength))
	binary.LittleEndian.PutUint16(buffer[6:], 0)
	binary.LittleEndian.PutUint16(buffer[8:], 0)
	binary.LittleEndian.PutUint16(buffer[10:], uint16(substituteNameLength))
	binary.LittleEndian.PutUint16(buffer[12:], uint16(printNameOffset))
	binary.LittleEndian.PutUint16(buffer[14:], uint16(printNameLength))

	pathBuffer := buffer[16:]
	for i, value := range substituteUTF16 {
		binary.LittleEndian.PutUint16(pathBuffer[i*2:], value)
	}
	for i, value := range printUTF16 {
		binary.LittleEndian.PutUint16(pathBuffer[printNameOffset+i*2:], value)
	}
	return buffer, nil
}
