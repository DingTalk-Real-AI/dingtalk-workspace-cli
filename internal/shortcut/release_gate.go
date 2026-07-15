// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package shortcut

func releaseHiddenKey(service, command string) string {
	return service + "\x00" + command
}

func applyReleaseGate(s Shortcut) Shortcut {
	if _, ok := ReleaseHiddenReason(s.Service, s.Command); ok {
		s.Hidden = true
	}
	return s
}

// ReleaseHiddenReason reports why a shortcut is hidden from public discovery in
// this release. Hidden shortcuts remain invocable by exact command path so the
// implementation can be kept and repaired in the next iteration.
func ReleaseHiddenReason(service, command string) (string, bool) {
	reason, ok := releaseHiddenShortcuts[releaseHiddenKey(service, command)]
	return reason, ok
}
