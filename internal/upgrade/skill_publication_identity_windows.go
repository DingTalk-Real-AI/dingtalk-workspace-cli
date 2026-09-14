//go:build windows

package upgrade

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

func skillPathFileIncarnationImpl(info os.FileInfo) string {
	stat, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return fmt.Sprintf("unknown:%T", info.Sys())
	}
	return fmt.Sprintf("%d:%d", stat.CreationTime.HighDateTime, stat.CreationTime.LowDateTime)
}

// skillPathSameFileIdentity returns false because os.FileInfo on Windows does
// not expose the stable file ID (VolumeSerialNumber / FileIndex from
// GetFileInformationByHandle), which is the only primitive that distinguishes
// a recreated file from the original after NTFS file tunneling restores the
// creation time. Callers must use skillPathIdentityProven, which compares file
// IDs obtained via GetFileInformationByHandle.
func skillPathSameFileIdentityImpl(_, _ os.FileInfo) bool {
	return false
}

// skillPathFileIdentityImpl queries the stable volume serial number and file
// index via GetFileInformationByHandle. Unlike creation time (which NTFS file
// tunneling can restore for a recreated same-named object), the file index
// uniquely identifies the file on the volume for its lifetime.
//
// FILE_FLAG_OPEN_REPARSE_POINT opens symlinks themselves rather than following
// them to the target. This matters at publish time because a staged symlink
// carries a relative target computed for the final destination, which may not
// resolve from the staging directory. Opening the reparse point yields a file
// ID that is stable across the rename and is the correct identity to compare.
func skillPathFileIdentityImpl(path string) (id string) {
	if p, err := windows.UTF16PtrFromString(path); err == nil {
		if handle, err := windows.CreateFile(
			p, windows.FILE_READ_ATTRIBUTES,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil, windows.OPEN_EXISTING,
			windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0,
		); err == nil {
			defer windows.CloseHandle(handle)
			var info windows.ByHandleFileInformation
			if err := windows.GetFileInformationByHandle(handle, &info); err == nil {
				id = fmt.Sprintf("%d:%d:%d", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow)
			}
		}
	}
	return
}

// skillPathIdentityProven is the real identity proof on Windows. An empty
// expected value means the file ID could not be obtained at publish time, so
// identity cannot be proven and the caller must refuse the auto-delete.
func skillPathIdentityProven(_, _ os.FileInfo, expected, actual string) bool {
	return expected != "" && expected == actual
}
