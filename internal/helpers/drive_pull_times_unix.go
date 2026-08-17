//go:build !windows

package helpers

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// setPullFileTimes 只通过已打开文件描述符更新访问/修改时间，不重新解析文件名。
func setPullFileTimes(file *os.File, modified time.Time) error {
	stamp := unix.NsecToTimeval(modified.UnixNano())
	return unix.Futimes(int(file.Fd()), []unix.Timeval{stamp, stamp})
}
