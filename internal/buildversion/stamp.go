// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package buildversion

import (
	"crypto/sha256"
	"encoding/binary"
	"os"
	"sync"
)

// Stamp material for the running binary. Release builds inject version/commit/
// buildTime via ldflags on internal/app; app.SetVersion (and init) syncs them
// here. This is a binary seal, not a compile-time Schema declaration seal.
var (
	stampVersion   = "dev"
	stampCommit    = "unknown"
	stampBuildTime = "unknown"
)

// ExecutableMaterial supplies bytes folded into Digest when the stamp is still
// the default unstamped triple (dev/unknown/unknown). Production uses a
// process-cached lightweight replace-detector over os.Executable() metadata
// (path + size + mtime); tests may swap this seam.
//
// Behavior:
//   - Stamped builds (any non-default version/commit/buildTime): Digest is
//     stamp-only so identical release binaries share one seal regardless of
//     install path. Release ldflags stamp is the sole identity; no exe I/O.
//   - Unstamped local builds: Digest = hash(stamp || exe material) where exe
//     material is a metadata fingerprint (path/size/mtime), NOT a full-file
//     sha256. Short CLI invocations must not pay whole-binary I/O on every
//     process when productionSchemaCacheOptions loads identity. Replacing a
//     local go-build binary (A→B) still changes the seal via size/mtime/path
//     without calling Invalidate*.
//
// Residual risk: two different binaries that collide on path+size+mtime can
// share an unstamped seal; that edge case is accepted for startup cost.
var ExecutableMaterial = cachedExecutableMaterial

var (
	exeMaterialOnce   sync.Once
	exeMaterialCached []byte

	// Process seams for ExecutableMaterial fault paths (tests swap via testseam).
	osExecutable = os.Executable
	osStat       = os.Stat
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

func unstamped() bool {
	return stampVersion == "dev" && stampCommit == "unknown" && stampBuildTime == "unknown"
}

// Digest returns the binary-owned seal that persisted Schema cache identity
// sidecars must match. A sidecar alone cannot invent this value for a different
// binary. Release stamps use stamp-only material; unstamped builds also fold
// ExecutableMaterial so two different local binaries do not share a seal.
func Digest() [sha256.Size]byte {
	payload := make([]byte, 0, 96+len(stampVersion)+len(stampCommit)+len(stampBuildTime))
	payload = append(payload, "dws-binary-build-id-v1\x00"...)
	payload = append(payload, stampVersion...)
	payload = append(payload, 0)
	payload = append(payload, stampCommit...)
	payload = append(payload, 0)
	payload = append(payload, stampBuildTime...)
	if unstamped() {
		material := ExecutableMaterial()
		payload = append(payload, 0)
		payload = append(payload, "exe-material-v1\x00"...)
		payload = append(payload, material...)
	}
	return sha256.Sum256(payload)
}

func cachedExecutableMaterial() []byte {
	exeMaterialOnce.Do(func() {
		exeMaterialCached = computeExecutableMaterial()
	})
	return exeMaterialCached
}

// computeExecutableMaterial returns a lightweight stable replace-detector for
// the running binary. Unstamped Digests intentionally avoid hashing the entire
// os.Executable() contents: every short CLI process would otherwise pay full
// file I/O when loading schema-cache identity (sync.Once is per-process only).
func computeExecutableMaterial() []byte {
	path, err := osExecutable()
	if err != nil {
		return []byte("exe-unavailable")
	}
	return fingerprintStatOnly(path)
}

func fingerprintStatOnly(path string) []byte {
	h := sha256.New()
	h.Write([]byte(path))
	h.Write([]byte{0})
	info, err := osStat(path)
	if err != nil {
		return h.Sum(nil)
	}
	var sizeBuf [8]byte
	binary.LittleEndian.PutUint64(sizeBuf[:], uint64(info.Size()))
	h.Write(sizeBuf[:])
	var timeBuf [8]byte
	binary.LittleEndian.PutUint64(timeBuf[:], uint64(info.ModTime().UnixNano()))
	h.Write(timeBuf[:])
	return h.Sum(nil)
}

// CurrentStampForTest returns the active stamp triple for test restore.
func CurrentStampForTest() (version, commit, buildTime string) {
	return stampVersion, stampCommit, stampBuildTime
}
