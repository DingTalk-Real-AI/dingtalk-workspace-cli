//go:build !windows

package app

import "os"

func createSkillSetupDirLink(target, link string) error {
	return os.Symlink(target, link)
}

func skillSetupLinkTarget(realTarget, relativeTarget string) string {
	return relativeTarget
}

func isSkillSetupCurrentCanonicalAdapter(path, canonicalTarget string) bool {
	if !samePhysicalSkillSetupPath(path, canonicalTarget) {
		return false
	}
	info, err := skillSetupLstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	return validateSkillSetupLink(path) == nil
}
