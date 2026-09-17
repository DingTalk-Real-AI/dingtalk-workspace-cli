//go:build darwin

package upgrade

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

func renameSkillPathNoReplaceAtomic(source, destination string) error {
	return unix.RenamexNp(source, destination, unix.RENAME_EXCL)
}

// Volumes whose filesystem does not implement renamex_np (several network and
// FUSE mounts) reject RENAME_EXCL instead of performing the rename.
func isNoReplaceRenameUnsupported(err error) bool {
	return errors.Is(err, syscall.ENOTSUP) ||
		errors.Is(err, syscall.EINVAL) ||
		errors.Is(err, syscall.ENOSYS)
}
