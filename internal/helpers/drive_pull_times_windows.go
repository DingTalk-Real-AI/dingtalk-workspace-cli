//go:build windows

package helpers

import (
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// setPullFileTimes 只通过已打开文件句柄更新访问/修改时间，不重新解析文件名。
func setPullFileTimes(file *os.File, modified time.Time) error {
	stamp := windows.NsecToFiletime(modified.UnixNano())
	return windows.SetFileTime(windows.Handle(file.Fd()), nil, &stamp, &stamp)
}
