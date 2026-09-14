//go:build !linux && !darwin && !windows

package upgrade

import (
	"fmt"
	"os"
)

func skillPathFileIncarnationImpl(info os.FileInfo) string {
	return fmt.Sprintf("%d:%d:%s", info.ModTime().UnixNano(), info.Size(), info.Mode())
}

func skillPathSameFileIdentityImpl(left, right os.FileInfo) bool {
	return os.SameFile(left, right)
}

// skillPathFileIdentityImpl reports no identity on platforms whose stat
// layout we do not decode; the child-move claim witness degrades gracefully
// to the publication fingerprint backstop.
func skillPathFileIdentityImpl(_ string) string {
	return ""
}

func skillPathIdentityProven(staged, published os.FileInfo, _, _ string) bool {
	return skillPathSameFileIdentity(staged, published)
}
