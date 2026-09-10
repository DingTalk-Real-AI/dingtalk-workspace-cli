// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package runtimepayload

import (
	"archive/tar"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/lock"
)

const ownershipName = ".dws-runtime-manifest.json"

// A pending record reserves only the fixed library and ps names. It is written
// before the first rename, so another process can recover after interruption.
// A ready record is committed last, after validation of the published files.
type ownership struct {
	Owner         string   `json:"owner"`
	State         string   `json:"state"`
	PayloadSHA256 string   `json:"payload_sha256"`
	Manifest      manifest `json:"manifest"`
}

var (
	renameAdjacent = os.Rename
	lstatAdjacent  = os.Lstat
	walkAdjacent   = filepath.WalkDir
)

// MaterializeAdjacent publishes only DWS-owned resources next to a resolved
// executable. A conflict or busy publisher is an error: the caller uses cache.
// No caller may load a library until this method has verified the whole bundle.
func MaterializeAdjacent(container []byte, root, targetOS, targetArch string) (string, error) {
	fail := func() (string, error) { return "", errors.New("adjacent runtime payload unavailable") }
	if !filepath.IsAbs(root) {
		return fail()
	}
	info, err := lstatAdjacent(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fail()
	}
	descriptor, err := Inspect(container)
	if err != nil {
		return fail()
	}
	name, err := LibraryName(targetOS, targetArch)
	if err != nil {
		return fail()
	}
	lockPath := filepath.Join(root, ".dws-runtime.lock")
	if info, err := lstatAdjacent(lockPath); err == nil && !info.Mode().IsRegular() {
		return fail()
	} else if err != nil && !os.IsNotExist(err) {
		return fail()
	}
	held, err := lock.TryAcquire(lockPath)
	if err != nil {
		return fail()
	}
	defer held.Close()

	current, owned, err := readOwnership(root, targetOS, targetArch)
	if err != nil {
		return fail()
	}
	// Never follow links, including links introduced inside a previously owned ps.
	for _, entry := range []string{name, "ps"} {
		path := filepath.Join(root, entry)
		info, err := lstatAdjacent(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !owned {
			return fail()
		}
		if (entry == "ps" && !info.IsDir()) || (entry != "ps" && !info.Mode().IsRegular()) {
			return fail()
		}
		if err := walkAdjacent(path, func(_ string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 || (!d.IsDir() && !d.Type().IsRegular()) {
				return errors.New("unsafe runtime resource")
			}
			return nil
		}); err != nil {
			return fail()
		}
	}

	digest := hex.EncodeToString(descriptor.SHA256[:])
	archive := container[containerHeader : containerHeader+descriptor.Size]
	if owned && current.State == "ready" && current.PayloadSHA256 == digest {
		// Read the trusted manifest without writing a second copy of the bundle.
		// A disk ownership record alone cannot authorize its own checksums.
		expected, err := readArchiveManifest(bytes.NewReader(archive))
		if err != nil {
			return fail()
		}
		if current.Manifest == expected && validateRootManifest(root, expected, targetOS, targetArch) == nil {
			return filepath.Join(root, name), nil
		}
	}

	// Only installation or repair stages and verifies a fresh embedded bundle.
	stage, err := makeCacheTemporary(root, ".dws-runtime-stage-*")
	if err != nil {
		return fail()
	}
	defer os.RemoveAll(stage)
	if err := extractPayload(bytes.NewReader(archive), stage); err != nil {
		return fail()
	}
	expected, err := readManifest(stage)
	if err != nil || validateRootManifest(stage, expected, targetOS, targetArch) != nil {
		return fail()
	}
	next := ownership{Owner: "dws.runtimepayload", State: "pending", PayloadSHA256: digest, Manifest: expected}
	if err := writeOwnership(root, stage, next); err != nil {
		return fail()
	}
	for _, entry := range []string{name, "ps"} {
		target := filepath.Join(root, entry)
		if _, err := lstatAdjacent(target); err == nil {
			// Move owned old data out of the way. A failed rename (e.g. a loaded DLL)
			// leaves the transaction pending and sends this invocation to cache.
			if err := renameAdjacent(target, filepath.Join(stage, "old-"+entry)); err != nil {
				return fail()
			}
		} else if !os.IsNotExist(err) {
			return fail()
		}
		if err := renameAdjacent(filepath.Join(stage, entry), target); err != nil {
			return fail()
		}
	}
	if validateRootManifest(root, expected, targetOS, targetArch) != nil {
		return fail()
	}
	next.State = "ready"
	if err := writeOwnership(root, stage, next); err != nil {
		return fail()
	}
	return filepath.Join(root, name), nil
}

// readArchiveManifest reads only as far as manifest.json from an archive whose
// checksum Inspect has verified. Bounds also apply to entries skipped before it.
// Cold publication still validates and extracts the complete archive.
func readArchiveManifest(input io.Reader) (manifest, error) {
	invalid := errors.New("invalid embedded runtime manifest")
	reader, err := newPayloadGzipReader(input)
	if err != nil {
		return manifest{}, invalid
	}
	defer reader.Close()
	archive := tar.NewReader(reader)
	var total int64
	for range maxFiles {
		header, err := nextPayloadEntry(archive)
		if err != nil || header.Typeflag != tar.TypeReg || !validArchivePath(header.Name) || header.Size < 0 || header.Size > maxFileBytes || total+header.Size > maxBundleBytes {
			return manifest{}, invalid
		}
		total += header.Size
		if header.Name != "manifest.json" {
			continue
		}
		if header.Size > 8192 {
			return manifest{}, invalid
		}
		data, err := io.ReadAll(archive)
		if err != nil {
			return manifest{}, invalid
		}
		var result manifest
		if json.Unmarshal(data, &result) != nil {
			return manifest{}, invalid
		}
		return result, nil
	}
	return manifest{}, invalid
}

func readOwnership(root, targetOS, targetArch string) (ownership, bool, error) {
	path := filepath.Join(root, ownershipName)
	info, err := lstatAdjacent(path)
	if os.IsNotExist(err) {
		return ownership{}, false, nil
	}
	invalid := errors.New("invalid runtime ownership")
	if err != nil || !info.Mode().IsRegular() || info.Size() > 8192 {
		return ownership{}, false, invalid
	}
	data, err := readManifestFile(path)
	if err != nil {
		return ownership{}, false, invalid
	}
	var value ownership
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF {
		return ownership{}, false, invalid
	}
	name, _ := LibraryName(targetOS, targetArch)
	m := value.Manifest
	if value.Owner != "dws.runtimepayload" || (value.State != "ready" && value.State != "pending") || m.FormatVersion != 1 || m.Library != name || m.Target != targetOS+"/"+targetArch || m.PSFileCount != 123 || len(m.PayloadVersion) != 8 || strings.Trim(m.PayloadVersion, "0123456789") != "" {
		return ownership{}, false, invalid
	}
	for _, digest := range []string{value.PayloadSHA256, m.LibrarySHA256, m.PSManifestSHA256} {
		raw, err := hex.DecodeString(digest)
		if err != nil || len(raw) != 32 {
			return ownership{}, false, invalid
		}
	}
	return value, true, nil
}

func writeOwnership(root, stage string, value ownership) error {
	// This concrete record contains only JSON primitives; marshaling cannot fail.
	data, _ := json.Marshal(value)
	path := filepath.Join(stage, "ownership.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return renameAdjacent(path, filepath.Join(root, ownershipName))
}
