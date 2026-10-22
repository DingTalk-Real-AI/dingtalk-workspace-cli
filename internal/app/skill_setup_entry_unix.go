//go:build !windows

package app

import "os"

func skillSetupEntryIsDirLike(entry os.DirEntry) bool {
	return entry.IsDir() || entry.Type()&os.ModeSymlink != 0
}
