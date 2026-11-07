//go:build linux

package upgrade

import (
	"os"

	"golang.org/x/sys/unix"
)

// O_PATH 固定 inode 生命周期，不要求文件内容可读；O_NOFOLLOW 同样
// 支持悬空符号链接，避免误将链接目标当成发布对象。句柄不传给子进程。
func pinSkillPathIdentity(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "pin", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}
