//go:build linux

package upgrade

import (
	"fmt"
	"os"
	"syscall"
)

func skillPathFileIncarnationImpl(info os.FileInfo) string {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Sprintf("unknown:%T", info.Sys())
	}
	return fmt.Sprintf("%d:%d:%d:%d", stat.Dev, stat.Ino, stat.Ctim.Sec, stat.Ctim.Nsec)
}

func skillPathSameFileIdentityImpl(left, right os.FileInfo) bool {
	return os.SameFile(left, right)
}

// skillPathFileIdentityImpl reports the device/inode pair from lstat. POSIX
// has no open-by-ID handle, but dev:ino identifies the object for its
// lifetime and is the same witness os.SameFile compares. It anchors the
// mkdir-claim identity of the child-move fallback: a wholesale replacement
// of the claimed destination reports a different identity, except in the
// classic inode-reuse race on recycled-inode filesystems (notably tmpfs),
// which the publication fingerprint backstop still catches. An unreadable
// or unrecognizable path reports "" and callers treat that as "no witness".
func skillPathFileIdentityImpl(path string) string {
	info, err := skillPathLstat(path)
	if err != nil {
		return ""
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino)
}

// skillPathIdentityProven stays SameFile-based on POSIX: the file ID above is
// the same dev:ino witness, so preferring it would add no strength while
// making the proof refuse publishes whose staged lstat (and therefore staged
// identity) could not be read.
func skillPathIdentityProven(staged, published os.FileInfo, _, _ string) bool {
	return skillPathSameFileIdentity(staged, published)
}
