//go:build !windows

package app

import "os"

func createSkillSetupDirLink(target, link string) error {
	return os.Symlink(target, link)
}

func skillSetupLinkTarget(realTarget, relativeTarget string) string {
	return relativeTarget
}
