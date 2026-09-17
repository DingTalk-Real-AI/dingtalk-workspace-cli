// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"path/filepath"

	"github.com/spf13/cobra"
)

// detectNestedMultiSkillLayout detects the compatibility gap where an older
// running dws process replaces itself with a newer binary, but then installs
// the new release bundle with its legacy mono copier. That produces the
// impossible layout <agent>/dws/multi/<skill>/SKILL.md.
//
// Detection is deliberately read-only. Ordinary commands must not turn a
// compatibility warning into an unconfirmed, cross-Agent Skill refresh.
// A legitimate mono install has no dws/multi product tree and is ignored.
func detectNestedMultiSkillLayout() (bool, error) {
	home, err := skillSetupUserHomeDir()
	if err != nil {
		return false, err
	}
	for _, rel := range skillSetupAgentHomes {
		nested := filepath.Join(home, rel, "dws", "multi")
		if isSkillSourceRoot(nested, skillSetupModeMulti) {
			return true, nil
		}
	}
	return false, nil
}

func shouldDetectNestedSkillLayout(cmd *cobra.Command) bool {
	if cmd == nil {
		return true
	}
	if cmd.Name() == "upgrade" {
		return false
	}
	return !(cmd.Name() == "setup" && cmd.Parent() != nil && cmd.Parent().Name() == "skill")
}
