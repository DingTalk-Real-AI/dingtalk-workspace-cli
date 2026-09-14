//go:build !linux && !darwin && !windows

package upgrade

func renameSkillPathNoReplaceAtomic(_, _ string) error {
	return errNoReplaceRenameUnsupported
}

func isNoReplaceRenameUnsupported(error) bool { return false }
