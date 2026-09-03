//go:build linux

package upgrade

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

func renameSkillPathNoReplaceAtomic(source, destination string) error {
	return unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE)
}

// rename(2) documents EINVAL as "the filesystem does not support one of the
// flags", which is what NFS, FUSE and overlayfs return; ENOSYS covers kernels
// without renameat2 at all.
func isNoReplaceRenameUnsupported(err error) bool {
	return errors.Is(err, syscall.EINVAL) ||
		errors.Is(err, syscall.ENOSYS) ||
		errors.Is(err, syscall.EOPNOTSUPP)
}
