// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Package buildversion owns the shared public version text, without build-state
// discovery, command construction or filesystem access.
package buildversion

func Format(version, commit, buildTime string) string {
	if buildTime != "unknown" || commit != "unknown" {
		return version + " (" + commit + ", " + buildTime + ")"
	}
	return version
}
