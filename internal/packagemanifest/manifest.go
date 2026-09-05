// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Package packagemanifest defines and verifies the finalized DWS package layout.
package packagemanifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

const (
	// LayoutVersion is the only package layout understood by this package.
	LayoutVersion = 1
	// ManifestName is fixed at the package root.
	ManifestName = "package-manifest.json"
	// MaxExecutableSize bounds both declared and hashed executable sizes.
	MaxExecutableSize int64 = 4 << 30
	maxManifestSize         = 64 << 10
)

// Release identifies the exact build represented by a package.
type Release struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Edition string `json:"edition"`
}

// Target identifies the package's Go target.
type Target struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
}

// Identity is supplied independently by a trusted caller when verifying.
type Identity struct {
	Release Release
	Target  Target
}

// FileIdentity binds one fixed-layout executable to its bytes and relevant mode.
// Mode is the exact Unix permission mode, or zero for a Windows target.
type FileIdentity struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
}

// Manifest is the canonical package manifest. Trust comes from the verified
// release archive and, on macOS, the independently signed executables.
type Manifest struct {
	LayoutVersion int          `json:"layout_version"`
	Release       Release      `json:"release"`
	Target        Target       `json:"target"`
	Launcher      FileIdentity `json:"launcher"`
	Core          FileIdentity `json:"core"`
}

// Paths returns the only launcher and core paths valid for target.
func Paths(target Target) (launcher, core string, err error) {
	if err := validateTarget(target); err != nil {
		return "", "", err
	}
	suffix := ""
	if target.GOOS == "windows" {
		suffix = ".exe"
	}
	return path.Join("bin", "dws"+suffix), path.Join("libexec", "dws-core"+suffix), nil
}

// Build hashes the already-finalized executables without changing them.
func Build(root string, identity Identity) (Manifest, error) {
	if err := validateIdentity(identity); err != nil {
		return Manifest{}, err
	}
	launcherPath, corePath, _ := Paths(identity.Target)
	launcherFile, launcherInfo, err := openRegular(filepath.Join(root, launcherPath), "launcher")
	if err != nil {
		return Manifest{}, err
	}
	defer launcherFile.Close()
	coreFile, coreInfo, err := openRegular(filepath.Join(root, corePath), "core")
	if err != nil {
		return Manifest{}, err
	}
	defer coreFile.Close()
	if os.SameFile(launcherInfo, coreInfo) {
		return Manifest{}, errors.New("launcher and core refer to the same file")
	}
	launcher, err := identityFromOpenFile(launcherFile, launcherInfo, identity.Target, launcherPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("launcher: %w", err)
	}
	core, err := identityFromOpenFile(coreFile, coreInfo, identity.Target, corePath)
	if err != nil {
		return Manifest{}, fmt.Errorf("core: %w", err)
	}
	manifest := Manifest{
		LayoutVersion: LayoutVersion,
		Release:       identity.Release,
		Target:        identity.Target,
		Launcher:      launcher,
		Core:          core,
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// MarshalCanonical emits compact JSON in stable field order with one final newline.
func MarshalCanonical(manifest Manifest) ([]byte, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(manifest); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// Decode strictly decodes one manifest, rejecting duplicate/unknown fields and trailing data.
func Decode(reader io.Reader) (Manifest, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxManifestSize+1))
	if err != nil {
		return Manifest{}, err
	}
	if len(data) > maxManifestSize {
		return Manifest{}, fmt.Errorf("manifest exceeds %d bytes", maxManifestSize)
	}
	if err := rejectDuplicateKeys(data); err != nil {
		return Manifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Manifest{}, errors.New("decode manifest: trailing JSON value")
		}
		return Manifest{}, fmt.Errorf("decode manifest trailing data: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Validate checks all closed manifest enums, identities, sizes, hashes, and modes.
func (manifest Manifest) Validate() error {
	if manifest.LayoutVersion != LayoutVersion {
		return fmt.Errorf("unsupported layout version %d", manifest.LayoutVersion)
	}
	identity := Identity{Release: manifest.Release, Target: manifest.Target}
	if err := validateIdentity(identity); err != nil {
		return err
	}
	if err := validateFileIdentity("launcher", manifest.Launcher, manifest.Target); err != nil {
		return err
	}
	return validateFileIdentity("core", manifest.Core, manifest.Target)
}

// VerifyTree verifies the complete fixed-layout tree against a trusted identity.
func VerifyTree(root string, expected Identity) (Manifest, error) {
	if err := validateIdentity(expected); err != nil {
		return Manifest{}, fmt.Errorf("expected identity: %w", err)
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return Manifest{}, fmt.Errorf("package root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return Manifest{}, errors.New("package root is not a real directory")
	}

	manifestFile, manifestInfo, err := openRegular(filepath.Join(root, ManifestName), "manifest")
	if err != nil {
		return Manifest{}, err
	}
	defer manifestFile.Close()
	if runtime.GOOS != "windows" && manifestInfo.Mode().Perm() != 0o644 {
		return Manifest{}, fmt.Errorf("manifest mode is %04o, want 0644", manifestInfo.Mode().Perm())
	}
	manifest, err := Decode(manifestFile)
	if err != nil {
		return Manifest{}, err
	}
	if manifest.Release != expected.Release {
		return Manifest{}, fmt.Errorf("release identity mismatch: got %+v, want %+v", manifest.Release, expected.Release)
	}
	if manifest.Target != expected.Target {
		return Manifest{}, fmt.Errorf("target mismatch: got %+v, want %+v", manifest.Target, expected.Target)
	}

	launcherPath, corePath, _ := Paths(expected.Target)
	launcherFile, launcherInfo, err := openRegular(filepath.Join(root, launcherPath), "launcher")
	if err != nil {
		return Manifest{}, err
	}
	defer launcherFile.Close()
	coreFile, coreInfo, err := openRegular(filepath.Join(root, corePath), "core")
	if err != nil {
		return Manifest{}, err
	}
	defer coreFile.Close()
	if os.SameFile(launcherInfo, coreInfo) {
		return Manifest{}, errors.New("launcher and core refer to the same file")
	}
	if err := verifyOpenFile("launcher", launcherFile, launcherInfo, manifest.Launcher, expected.Target); err != nil {
		return Manifest{}, err
	}
	if err := verifyOpenFile("core", coreFile, coreInfo, manifest.Core, expected.Target); err != nil {
		return Manifest{}, err
	}
	if err := verifyExactTree(root, launcherPath, corePath); err != nil {
		return Manifest{}, err
	}
	if err := unchangedOpenFile(manifestFile, manifestInfo); err != nil {
		return Manifest{}, fmt.Errorf("manifest changed during verification: %w", err)
	}
	return manifest, nil
}

// VerifyLegacyEntry authenticates the optional archive-root executable for
// already-shipped flat-binary upgraders. It must be an exact copy of the final
// core, never the launcher (which cannot run without its package siblings).
func VerifyLegacyEntry(path string, manifest Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	file, info, err := openRegular(path, "legacy upgrade entry")
	if err != nil {
		return err
	}
	defer file.Close()
	return verifyOpenFile("legacy upgrade entry", file, info, manifest.Core, manifest.Target)
}

func validateIdentity(identity Identity) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "release version", value: identity.Release.Version},
		{name: "release commit", value: identity.Release.Commit},
		{name: "release edition", value: identity.Release.Edition},
	} {
		name, value := field.name, field.value
		if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
			return fmt.Errorf("invalid %s", name)
		}
		for _, character := range value {
			if unicode.IsControl(character) {
				return fmt.Errorf("invalid %s", name)
			}
		}
	}
	return validateTarget(identity.Target)
}

func validateTarget(target Target) error {
	validTargets := map[string]bool{
		"darwin/amd64": true, "darwin/arm64": true,
		"linux/amd64": true, "linux/arm64": true,
		"windows/amd64": true, "windows/arm64": true,
	}
	if !validTargets[target.GOOS+"/"+target.GOARCH] {
		return fmt.Errorf("invalid target %q/%q", target.GOOS, target.GOARCH)
	}
	return nil
}

func validateFileIdentity(name string, identity FileIdentity, target Target) error {
	launcherPath, corePath, err := Paths(target)
	if err != nil {
		return err
	}
	expectedPath := launcherPath
	if name == "core" {
		expectedPath = corePath
	}
	if identity.Path != expectedPath {
		return fmt.Errorf("%s path is %q, want %q", name, identity.Path, expectedPath)
	}
	if identity.Size <= 0 || identity.Size > MaxExecutableSize {
		return fmt.Errorf("%s size %d is outside 1..%d", name, identity.Size, MaxExecutableSize)
	}
	if len(identity.SHA256) != sha256.Size*2 {
		return fmt.Errorf("%s SHA-256 must be 64 lowercase hexadecimal characters", name)
	}
	if _, err := hex.DecodeString(identity.SHA256); err != nil || strings.ToLower(identity.SHA256) != identity.SHA256 {
		return fmt.Errorf("%s SHA-256 must be 64 lowercase hexadecimal characters", name)
	}
	if target.GOOS == "windows" {
		if identity.Mode != 0 {
			return fmt.Errorf("%s mode must be zero for a Windows target", name)
		}
		return nil
	}
	if identity.Mode > 0o777 || identity.Mode&0o111 == 0 {
		return fmt.Errorf("%s mode %04o is not an executable Unix permission mode", name, identity.Mode)
	}
	return nil
}

func openRegular(path, name string) (*os.File, fs.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", name, err)
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s is not a regular non-symlink file", name)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", name, err)
	}
	opened, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, fmt.Errorf("stat open %s: %w", name, err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		file.Close()
		return nil, nil, fmt.Errorf("%s changed while opening", name)
	}
	return file, opened, nil
}

func identityFromOpenFile(file *os.File, before fs.FileInfo, target Target, fixedPath string) (FileIdentity, error) {
	if before.Size() <= 0 || before.Size() > MaxExecutableSize {
		return FileIdentity{}, fmt.Errorf("size %d is outside 1..%d", before.Size(), MaxExecutableSize)
	}
	if target.GOOS != "windows" && before.Mode().Perm()&0o111 == 0 {
		return FileIdentity{}, fmt.Errorf("mode %04o is not executable", before.Mode().Perm())
	}
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, MaxExecutableSize+1))
	if err != nil {
		return FileIdentity{}, err
	}
	if written != before.Size() {
		return FileIdentity{}, fmt.Errorf("size changed while hashing: read %d, initially %d", written, before.Size())
	}
	if err := unchangedOpenFile(file, before); err != nil {
		return FileIdentity{}, err
	}
	mode := uint32(0)
	if target.GOOS != "windows" {
		mode = uint32(before.Mode().Perm())
	}
	return FileIdentity{Path: fixedPath, SHA256: hex.EncodeToString(hash.Sum(nil)), Size: written, Mode: mode}, nil
}

func verifyOpenFile(name string, file *os.File, before fs.FileInfo, expected FileIdentity, target Target) error {
	actual, err := identityFromOpenFile(file, before, target, expected.Path)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if actual.Size != expected.Size {
		return fmt.Errorf("%s size mismatch: got %d, want %d", name, actual.Size, expected.Size)
	}
	if actual.Mode != expected.Mode {
		return fmt.Errorf("%s mode mismatch: got %04o, want %04o", name, actual.Mode, expected.Mode)
	}
	if actual.SHA256 != expected.SHA256 {
		return fmt.Errorf("%s SHA-256 mismatch", name)
	}
	return nil
}

func unchangedOpenFile(file *os.File, before fs.FileInfo) error {
	after, err := file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		return errors.New("file changed while open")
	}
	return nil
}

func verifyExactTree(root, launcherPath, corePath string) error {
	allowed := map[string]bool{
		".":                          true,
		"bin":                        true,
		"libexec":                    true,
		ManifestName:                 true,
		filepath.Clean(launcherPath): true,
		filepath.Clean(corePath):     true,
	}
	seen := make(map[string]bool, len(allowed))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.Clean(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed: %s", relative)
		}
		if !allowed[relative] {
			return fmt.Errorf("unexpected package entry: %s", relative)
		}
		seen[relative] = true
		return nil
	})
	if err != nil {
		return err
	}
	for path := range allowed {
		if !seen[path] {
			return fmt.Errorf("missing package entry: %s", path)
		}
	}
	return nil
}

func rejectDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := scanJSONValue(decoder, 0); err != nil {
		return fmt.Errorf("decode manifest: %w", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return errors.New("decode manifest: trailing JSON value")
		}
		return fmt.Errorf("decode manifest: %w", err)
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 16 {
		return errors.New("JSON nesting exceeds limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delimiter != '{' && delimiter != '[' {
		return errors.New("unexpected closing JSON delimiter")
	}
	seen := map[string]struct{}{}
	for decoder.More() {
		if delimiter == '{' {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate object key %q", key)
			}
			seen[key] = struct{}{}
		}
		if err := scanJSONValue(decoder, depth+1); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}
