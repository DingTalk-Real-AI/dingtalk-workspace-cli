// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package buildversion

import "crypto/sha256"

// Stamp material for the running binary. Release builds inject version/commit/
// buildTime via ldflags on internal/app; app.SetVersion (and init) syncs them
// here. This is a binary seal, not a compile-time Schema declaration seal.
var (
	stampVersion   = "dev"
	stampCommit    = "unknown"
	stampBuildTime = "unknown"
)

// Set updates the binary stamp material. Empty inputs leave the prior value.
func Set(version, commit, buildTime string) {
	if version != "" {
		stampVersion = version
	}
	if commit != "" {
		stampCommit = commit
	}
	if buildTime != "" {
		stampBuildTime = buildTime
	}
}

// Digest returns the binary-owned seal that persisted Schema cache identity
// sidecars must match. A sidecar alone cannot invent this value for a different
// binary: it is derived from this process's version/commit/buildTime stamp.
func Digest() [sha256.Size]byte {
	payload := make([]byte, 0, 64+len(stampVersion)+len(stampCommit)+len(stampBuildTime))
	payload = append(payload, "dws-binary-build-id-v1\x00"...)
	payload = append(payload, stampVersion...)
	payload = append(payload, 0)
	payload = append(payload, stampCommit...)
	payload = append(payload, 0)
	payload = append(payload, stampBuildTime...)
	return sha256.Sum256(payload)
}
