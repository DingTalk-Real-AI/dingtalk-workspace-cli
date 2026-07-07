// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"strconv"
	"strings"
)

// CompareVersions compares two semver strings (e.g. "1.0.5", "v1.0.6-beta").
// Returns -1 if a < b, 0 if equal, 1 if a > b.
// Stable releases sort after prereleases with the same numeric version.
func CompareVersions(a, b string) int {
	va := parseVersion(a)
	vb := parseVersion(b)
	for i := 0; i < 3; i++ {
		if va.parts[i] < vb.parts[i] {
			return -1
		}
		if va.parts[i] > vb.parts[i] {
			return 1
		}
	}
	return comparePrerelease(va.prerelease, vb.prerelease)
}

// NeedsUpgrade returns true when remoteVersion is newer than currentVersion.
func NeedsUpgrade(currentVersion, remoteVersion string) bool {
	return CompareVersions(currentVersion, remoteVersion) < 0
}

type parsedVersion struct {
	parts      [3]int
	prerelease string
}

func parseVersion(v string) parsedVersion {
	v = strings.TrimPrefix(v, "v")
	var prerelease string
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		prerelease = v[idx+1:]
		v = v[:idx]
	}
	parts := strings.SplitN(v, ".", 3)
	var result parsedVersion
	for i, p := range parts {
		if i < 3 {
			result.parts[i], _ = strconv.Atoi(p)
		}
	}
	result.prerelease = prerelease
	return result
}

func comparePrerelease(a, b string) int {
	if a == "" && b == "" {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}

	ap := strings.Split(a, ".")
	bp := strings.Split(b, ".")
	max := len(ap)
	if len(bp) > max {
		max = len(bp)
	}
	for i := 0; i < max; i++ {
		if i >= len(ap) {
			return -1
		}
		if i >= len(bp) {
			return 1
		}
		if c := comparePrereleaseIdentifier(ap[i], bp[i]); c != 0 {
			return c
		}
	}
	return 0
}

func comparePrereleaseIdentifier(a, b string) int {
	ai, aNum := parseNumericIdentifier(a)
	bi, bNum := parseNumericIdentifier(b)
	switch {
	case aNum && bNum:
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
		return 0
	case aNum:
		return -1
	case bNum:
		return 1
	default:
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}
}

func parseNumericIdentifier(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	v, err := strconv.Atoi(s)
	return v, err == nil
}
