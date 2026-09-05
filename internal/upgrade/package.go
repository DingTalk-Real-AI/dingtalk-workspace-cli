// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/launcher"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/packagemanifest"
)

const (
	managedVersionsDir                      = "versions"
	managedCurrent                          = "current"
	managedWindowsCurrent                   = "current.txt"
	InstallationMetadataName                = "installation.json"
	InstallationMetadataFormatVersion       = 1
	maxArchiveEntries                       = 16384
	maxArchiveEntrySize               int64 = 4 << 30
	maxArchiveTotalSize               int64 = 9 << 30
)

// InstallationMetadata is written in <cli_root>/installation.json as exactly:
// {"format_version":1,"public_launcher":"<absolute>","platform":"unix|windows"}\n
type InstallationMetadata struct {
	FormatVersion  int    `json:"format_version"`
	PublicLauncher string `json:"public_launcher"`
	Platform       string `json:"platform"`
}

// Installation is the single resolved identity used by self-upgrade and rollback.
type Installation struct {
	PackageRoot     string
	LauncherPath    string
	CorePath        string
	InstallRoot     string
	VersionsDir     string
	CurrentPath     string
	PublicLauncher  string
	Manifest        packagemanifest.Manifest
	LegacyFlat      bool
	RunningPath     string
	GOOS            string
	GOARCH          string
	Edition         string
	TextPointer     bool
	PreviousVersion string
}

var ErrActivationStateUncertain = errors.New("activation state uncertain")

// ResolveInstallation validates launcher-provided identity before allowing a
// self-upgrade. Direct daemon/core execution remains valid for all other commands.
func ResolveInstallation(currentVersion, expectedEdition string) (Installation, error) {
	return resolveInstallationFor(currentVersion, expectedEdition, runtime.GOOS, runtime.GOARCH)
}

func resolveInstallationFor(currentVersion, expectedEdition, goos, goarch string) (Installation, error) {
	running, err := upgradeExecutable()
	if err != nil {
		return Installation{}, fmt.Errorf("locate running executable: %w", err)
	}
	running, err = filepath.Abs(running)
	if err != nil {
		return Installation{}, fmt.Errorf("canonicalize running executable: %w", err)
	}
	if manager := packageManagerForPath(running); manager != "" {
		return Installation{}, packageManagerUpgradeError(manager)
	}
	launcherPath := strings.TrimSpace(upgradeGetenv(launcher.EnvLauncherPath))
	coreDigest := strings.TrimSpace(upgradeGetenv(launcher.EnvCoreDigest))
	coreVersion := strings.TrimSpace(upgradeGetenv(launcher.EnvCoreVersion))
	set := 0
	for _, value := range []string{launcherPath, coreDigest, coreVersion} {
		if value != "" {
			set++
		}
	}
	if set == 0 {
		base := strings.ToLower(filepath.Base(running))
		if base == "dws" || base == "dws.exe" {
			root := filepath.Dir(running)
			cliRoot := filepath.Join(root, ".dws")
			return Installation{
				InstallRoot: cliRoot, VersionsDir: filepath.Join(cliRoot, managedVersionsDir),
				CurrentPath: filepath.Join(cliRoot, managedCurrent), PublicLauncher: running,
				LegacyFlat: true, RunningPath: running, GOOS: goos, GOARCH: goarch,
				Edition: expectedEdition, PreviousVersion: canonicalVersion(currentVersion),
			}, nil
		}
		return Installation{}, errors.New("self-upgrade requires the canonical dws launcher; invoke dws from its public install path or reinstall it")
	}
	if set != 3 {
		return Installation{}, errors.New("incomplete internal launcher identity; invoke the public dws launcher instead of libexec/dws-core")
	}
	if !filepath.IsAbs(launcherPath) || filepath.Clean(launcherPath) != launcherPath {
		return Installation{}, errors.New("internal launcher path is not canonical")
	}
	if coreVersion != canonicalVersion(currentVersion) {
		return Installation{}, fmt.Errorf("launcher/core version mismatch: launcher supplied %q, running core expects %q", coreVersion, canonicalVersion(currentVersion))
	}
	if !validDigest(coreDigest) {
		return Installation{}, errors.New("launcher supplied an invalid core SHA-256")
	}
	resolvedLauncher, err := upgradeEvalSymlinks(launcherPath)
	if err != nil || filepath.Clean(resolvedLauncher) != launcherPath {
		return Installation{}, errors.New("internal launcher path is a symlink or does not resolve to itself")
	}
	packageRoot := filepath.Dir(filepath.Dir(launcherPath))
	manifest, err := decodeManifest(filepath.Join(packageRoot, packagemanifest.ManifestName))
	if err != nil {
		return Installation{}, fmt.Errorf("validate installed package manifest: %w", err)
	}
	expected := packagemanifest.Identity{
		Release: packagemanifest.Release{Version: canonicalVersion(currentVersion), Commit: manifest.Release.Commit, Edition: expectedEdition},
		Target:  packagemanifest.Target{GOOS: goos, GOARCH: goarch},
	}
	manifest, err = packagemanifest.VerifyTree(packageRoot, expected)
	if err != nil {
		return Installation{}, fmt.Errorf("validate installed package: %w", err)
	}
	if manifest.Core.SHA256 != coreDigest {
		return Installation{}, errors.New("launcher core digest does not match package manifest")
	}
	launcherRel, coreRel, _ := packagemanifest.Paths(expected.Target)
	corePath := filepath.Join(packageRoot, filepath.FromSlash(coreRel))
	if err := sameRegularFile(running, corePath); err != nil {
		return Installation{}, fmt.Errorf("running process is not the launcher-selected package core: %w", err)
	}
	if err := sameRegularFile(launcherPath, filepath.Join(packageRoot, filepath.FromSlash(launcherRel))); err != nil {
		return Installation{}, fmt.Errorf("launcher identity mismatch: %w", err)
	}

	versionsDir := filepath.Dir(packageRoot)
	if filepath.Base(versionsDir) != managedVersionsDir {
		return Installation{}, errors.New("package is not in a managed versions directory; upgrade it with the installer or package manager that installed it")
	}
	installRoot := filepath.Dir(versionsDir)
	current := filepath.Join(installRoot, managedCurrent)
	textPointer := goos == "windows"
	if textPointer {
		current = filepath.Join(installRoot, managedWindowsCurrent)
	}
	if err := activationResolvesTo(current, packageRoot, textPointer); err != nil {
		return Installation{}, fmt.Errorf("invalid current activation pointer: %w", err)
	}
	public, err := locatePublicLauncher(installRoot, launcherPath, goos)
	if err != nil {
		return Installation{}, err
	}
	return Installation{
		PackageRoot: packageRoot, LauncherPath: launcherPath, CorePath: corePath,
		InstallRoot: installRoot, VersionsDir: versionsDir, CurrentPath: current,
		PublicLauncher: public, Manifest: manifest, RunningPath: running,
		GOOS: goos, GOARCH: goarch, Edition: expectedEdition, TextPointer: textPointer,
		PreviousVersion: manifest.Release.Version,
	}, nil
}

func packageManagerForPath(path string) string {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
	for index, part := range parts {
		lower := strings.ToLower(part)
		if lower == "node_modules" {
			return "npm"
		}
		if lower == "cellar" && index+1 < len(parts) && strings.HasPrefix(strings.ToLower(parts[index+1]), "dingtalk-workspace-cli") {
			return "homebrew"
		}
	}
	return ""
}

func packageManagerUpgradeError(manager string) error {
	if manager == "npm" {
		return errors.New("npm-managed DWS installation detected; self-upgrade will not modify node_modules. Run `npm install -g dingtalk-workspace-cli` (or append `@beta` for beta)")
	}
	return errors.New("Homebrew-managed DWS installation detected; self-upgrade will not modify the Cellar. Run `brew upgrade dingtalk-workspace-cli`")
}

func activationResolvesTo(pointer, packageRoot string, textPointer bool) error {
	if !textPointer {
		return symlinkResolvesTo(pointer, packageRoot)
	}
	info, err := os.Lstat(pointer)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > 1024 {
		if err == nil {
			err = errors.New("Windows activation pointer is not a small regular non-symlink file")
		}
		return err
	}
	data, err := os.ReadFile(pointer)
	if err != nil {
		return err
	}
	generation, err := parseWindowsPointer(data)
	if err != nil || generation != filepath.Base(packageRoot) {
		return errors.New("Windows activation pointer does not name the running package")
	}
	return nil
}

func parseWindowsPointer(data []byte) (string, error) {
	value := string(data)
	var name string
	if strings.HasSuffix(value, "\r\n") {
		name = strings.TrimSuffix(value, "\r\n")
	} else if strings.HasSuffix(value, "\n") {
		name = strings.TrimSuffix(value, "\n")
	} else {
		return "", errors.New("Windows activation pointer is not newline terminated")
	}
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00\r\n") {
		return "", errors.New("invalid Windows activation generation")
	}
	return name, nil
}

func locatePublicLauncher(installRoot, launcherPath, goos string) (string, error) {
	if metadata, found, err := readInstallationMetadata(installRoot); err != nil {
		return "", err
	} else if found {
		if err := validateInstallationMetadata(metadata, installRoot, launcherPath, goos); err != nil {
			return "", err
		}
		return metadata.PublicLauncher, nil
	}
	name := executableNameFor(goos, "dws")
	if goos == "windows" {
		name = "dws.cmd"
	}
	candidates := []string{
		filepath.Join(filepath.Dir(installRoot), name),
		filepath.Join(installRoot, "bin", name),
	}
	var matched string
	for _, candidate := range candidates {
		if goos == "windows" {
			info, err := os.Lstat(candidate)
			if err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
				data, readErr := os.ReadFile(candidate)
				expected := "@echo off\r\nsetlocal\r\nset \"DWS_ROOT=%~dp0.dws\"\r\nset /p DWS_PACKAGE=<\"%DWS_ROOT%\\current.txt\"\r\n\"%DWS_ROOT%\\versions\\%DWS_PACKAGE%\\bin\\dws.exe\" %*\r\nexit /b %ERRORLEVEL%\r\n"
				if readErr != nil || candidate != filepath.Join(filepath.Dir(installRoot), name) || string(data) != expected {
					continue
				}
				if matched != "" {
					return "", errors.New("ambiguous public launcher paths")
				}
				matched = candidate
			}
			continue
		}
		if symlinkResolvesTo(candidate, launcherPath) == nil {
			if matched != "" {
				return "", errors.New("ambiguous public launcher paths")
			}
			matched = candidate
		}
	}
	if matched == "" {
		return "", errors.New("cannot identify the public launcher from the managed package; custom DWS_CLI_ROOT installs require installer support")
	}
	return matched, nil
}

func readInstallationMetadata(installRoot string) (InstallationMetadata, bool, error) {
	path := filepath.Join(installRoot, InstallationMetadataName)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return InstallationMetadata{}, false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 4096 {
		if err == nil {
			err = errors.New("metadata is not a small regular non-symlink file")
		}
		return InstallationMetadata{}, true, fmt.Errorf("invalid installation metadata: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return InstallationMetadata{}, true, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var metadata InstallationMetadata
	if err := decoder.Decode(&metadata); err != nil {
		return InstallationMetadata{}, true, fmt.Errorf("decode installation metadata: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return InstallationMetadata{}, true, errors.New("installation metadata must contain exactly one JSON object")
	}
	var canonical bytes.Buffer
	encoder := json.NewEncoder(&canonical)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(metadata); err != nil {
		return InstallationMetadata{}, true, err
	}
	if !bytes.Equal(data, canonical.Bytes()) {
		return InstallationMetadata{}, true, errors.New("installation metadata is not canonical compact JSON followed by LF")
	}
	return metadata, true, nil
}

func validateInstallationMetadata(metadata InstallationMetadata, installRoot, launcherPath, goos string) error {
	if metadata.FormatVersion != InstallationMetadataFormatVersion {
		return fmt.Errorf("unsupported installation metadata format_version %d", metadata.FormatVersion)
	}
	expectedPlatform := "unix"
	if goos == "windows" {
		expectedPlatform = "windows"
	}
	if metadata.Platform != expectedPlatform {
		return fmt.Errorf("installation metadata platform %q does not match runtime %q", metadata.Platform, expectedPlatform)
	}
	if !filepath.IsAbs(metadata.PublicLauncher) || filepath.Clean(metadata.PublicLauncher) != metadata.PublicLauncher {
		return errors.New("installation metadata public_launcher is not a canonical absolute path")
	}
	if filepath.Base(metadata.PublicLauncher) == string(filepath.Separator) || filepath.Base(metadata.PublicLauncher) == "." {
		return errors.New("installation metadata public_launcher has no install name")
	}
	if expectedPlatform == "unix" {
		if err := symlinkResolvesTo(metadata.PublicLauncher, launcherPath); err != nil {
			return fmt.Errorf("metadata public launcher does not reach the current package: %w", err)
		}
		return nil
	}
	info, err := os.Lstat(metadata.PublicLauncher)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 4096 {
		return errors.New("metadata Windows public launcher is not a small regular exact shim")
	}
	body, err := os.ReadFile(metadata.PublicLauncher)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Ext(metadata.PublicLauncher), ".cmd") {
		return errors.New("metadata Windows public launcher does not have a .cmd install name")
	}
	root, err := filepath.Abs(installRoot)
	if err != nil || filepath.Clean(root) != installRoot {
		return errors.New("installation metadata directory is not canonical")
	}
	shimRoot := root
	defaultRoot := filepath.Join(filepath.Dir(metadata.PublicLauncher), ".dws")
	if sameCleanPath(defaultRoot, root) {
		shimRoot = "%~dp0.dws"
	}
	expected := "@echo off\r\nsetlocal\r\nset \"DWS_ROOT=" + shimRoot + "\"\r\nset /p DWS_PACKAGE=<\"%DWS_ROOT%\\current.txt\"\r\n\"%DWS_ROOT%\\versions\\%DWS_PACKAGE%\\bin\\dws.exe\" %*\r\nexit /b %ERRORLEVEL%\r\n"
	if string(body) != expected {
		return errors.New("metadata Windows public launcher does not exactly match the managed shim")
	}
	return nil
}

func decodeManifest(path string) (packagemanifest.Manifest, error) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		if err == nil {
			err = errors.New("manifest is not a regular non-symlink file")
		}
		return packagemanifest.Manifest{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return packagemanifest.Manifest{}, err
	}
	defer file.Close()
	return packagemanifest.Decode(file)
}

func sameRegularFile(left, right string) error {
	leftInfo, err := os.Stat(left)
	if err != nil {
		return err
	}
	rightInfo, err := os.Lstat(right)
	if err != nil {
		return err
	}
	if !leftInfo.Mode().IsRegular() || !rightInfo.Mode().IsRegular() || rightInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(leftInfo, rightInfo) {
		return errors.New("paths do not identify the same non-symlink regular file")
	}
	return nil
}

func symlinkResolvesTo(link, expected string) error {
	info, err := os.Lstat(link)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return errors.New("path is not a symbolic link")
	}
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		return err
	}
	resolved, _ = filepath.Abs(resolved)
	if canonicalExpected, err := filepath.EvalSymlinks(expected); err == nil {
		expected = canonicalExpected
	}
	expected, _ = filepath.Abs(expected)
	if filepath.Clean(resolved) != filepath.Clean(expected) {
		return fmt.Errorf("resolves to %s, want %s", resolved, expected)
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func canonicalVersion(version string) string {
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func executableNameFor(goos, base string) string {
	if goos == "windows" {
		return base + ".exe"
	}
	return base
}

// ExtractAndVerifyPackage extracts exactly one canonical package root and
// verifies it against identity. Archive entries are rejected, never skipped.
func ExtractAndVerifyPackage(archivePath, destination string, identity packagemanifest.Identity) (string, packagemanifest.Manifest, error) {
	if err := os.MkdirAll(destination, dirPermSecure); err != nil {
		return "", packagemanifest.Manifest{}, err
	}
	if strings.HasSuffix(strings.ToLower(archivePath), ".zip") {
		if err := extractStrictZip(archivePath, destination); err != nil {
			return "", packagemanifest.Manifest{}, err
		}
	} else {
		if err := extractStrictTarGz(archivePath, destination); err != nil {
			return "", packagemanifest.Manifest{}, err
		}
	}
	wrapper := fmt.Sprintf("dws-%s-%s-%s", identity.Release.Version, identity.Target.GOOS, identity.Target.GOARCH)
	legacyName := executableNameFor(identity.Target.GOOS, "dws")
	root := filepath.Join(destination, wrapper)
	entries, err := os.ReadDir(destination)
	if err != nil {
		return "", packagemanifest.Manifest{}, err
	}
	for _, entry := range entries {
		if entry.Name() == wrapper {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return "", packagemanifest.Manifest{}, errors.New("canonical package root is not a real directory")
			}
			continue
		}
		if entry.Name() == legacyName && entry.Type().IsRegular() {
			continue // authenticated against the canonical core after VerifyTree
		}
		if !archiveMetadataName(entry.Name()) || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return "", packagemanifest.Manifest{}, fmt.Errorf("unexpected archive-root entry: %s", entry.Name())
		}
	}
	manifest, err := packagemanifest.VerifyTree(root, identity)
	if err != nil {
		return "", packagemanifest.Manifest{}, fmt.Errorf("verify package tree: %w", err)
	}
	legacyPath := filepath.Join(destination, legacyName)
	if _, err := os.Lstat(legacyPath); err == nil {
		if err := packagemanifest.VerifyLegacyEntry(legacyPath, manifest); err != nil {
			return "", packagemanifest.Manifest{}, fmt.Errorf("verify legacy migration entry: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", packagemanifest.Manifest{}, err
	}
	return root, manifest, nil
}

func archiveMetadataName(name string) bool {
	switch name {
	case "LICENSE", "NOTICE", "README.md", "CHANGELOG.md":
		return true
	default:
		return false
	}
}

func cleanArchivePath(destination, name string) (string, error) {
	if name == "" || strings.ContainsRune(name, '\x00') || filepath.IsAbs(name) || strings.Contains(name, "\\") {
		return "", fmt.Errorf("invalid archive path %q", name)
	}
	for _, component := range strings.Split(strings.TrimSuffix(name, "/"), "/") {
		if component == "" || component == "." || component == ".." {
			return "", fmt.Errorf("non-canonical archive path %q", name)
		}
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive traversal path %q", name)
	}
	dest := filepath.Join(destination, clean)
	rel, err := filepath.Rel(destination, dest)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive traversal path %q", name)
	}
	return dest, nil
}

func extractStrictZip(archivePath, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer reader.Close()
	if err := validateStrictZipEntries(reader.File, destination); err != nil {
		return err
	}
	for _, entry := range reader.File {
		dest, err := cleanArchivePath(destination, entry.Name)
		if err != nil {
			return err
		}
		mode := entry.Mode()
		if mode.IsDir() {
			if err := os.MkdirAll(dest, dirPermShared); err != nil {
				return err
			}
			continue
		}
		_, err = writeArchiveFile(dest, mode.Perm(), func() (io.ReadCloser, error) { return entry.Open() }, int64(entry.UncompressedSize64))
		if err != nil {
			return err
		}
	}
	return nil
}

func validateStrictZipEntries(entries []*zip.File, destination string) error {
	if len(entries) > maxArchiveEntries {
		return fmt.Errorf("archive has too many entries: %d exceeds %d", len(entries), maxArchiveEntries)
	}
	seen := make(map[string]bool, len(entries))
	var total uint64
	for _, entry := range entries {
		dest, err := cleanArchivePath(destination, entry.Name)
		if err != nil {
			return err
		}
		key := filepath.Clean(dest)
		if seen[key] {
			return fmt.Errorf("duplicate archive entry: %s", entry.Name)
		}
		seen[key] = true
		mode := entry.Mode()
		if mode&os.ModeSymlink != 0 || (!mode.IsRegular() && !mode.IsDir()) {
			return fmt.Errorf("unsupported zip entry type: %s", entry.Name)
		}
		if mode.IsDir() {
			continue
		}
		if entry.UncompressedSize64 > uint64(maxArchiveEntrySize) {
			return fmt.Errorf("archive file is too large: %s", entry.Name)
		}
		if entry.UncompressedSize64 > uint64(maxArchiveTotalSize)-total {
			return fmt.Errorf("archive cumulative size exceeds %d bytes", maxArchiveTotalSize)
		}
		total += entry.UncompressedSize64
	}
	return nil
}

func extractStrictTarGz(archivePath, destination string) error {
	if err := validateStrictTarGz(archivePath, destination); err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	seen := map[string]bool{}
	var entries int
	var total int64
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		entries++
		if entries > maxArchiveEntries {
			return fmt.Errorf("archive has more than %d entries", maxArchiveEntries)
		}
		dest, err := cleanArchivePath(destination, header.Name)
		if err != nil {
			return err
		}
		key := filepath.Clean(dest)
		if seen[key] {
			return fmt.Errorf("duplicate archive entry: %s", header.Name)
		}
		seen[key] = true
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, os.FileMode(header.Mode).Perm()); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > maxArchiveEntrySize {
				return fmt.Errorf("invalid archive file size for %s", header.Name)
			}
			if header.Size > maxArchiveTotalSize-total {
				return fmt.Errorf("archive cumulative size exceeds %d bytes", maxArchiveTotalSize)
			}
			if err := writeTarFile(dest, os.FileMode(header.Mode).Perm(), io.LimitReader(reader, header.Size), header.Size); err != nil {
				return err
			}
			total += header.Size
		default:
			return fmt.Errorf("unsupported tar entry type for %s", header.Name)
		}
	}
}

func validateStrictTarGz(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	seen := map[string]bool{}
	var entries int
	var total int64
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		entries++
		if entries > maxArchiveEntries {
			return fmt.Errorf("archive has more than %d entries", maxArchiveEntries)
		}
		dest, err := cleanArchivePath(destination, header.Name)
		if err != nil {
			return err
		}
		key := filepath.Clean(dest)
		if seen[key] {
			return fmt.Errorf("duplicate archive entry: %s", header.Name)
		}
		seen[key] = true
		switch header.Typeflag {
		case tar.TypeDir:
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > maxArchiveEntrySize {
				return fmt.Errorf("invalid archive file size for %s", header.Name)
			}
			if header.Size > maxArchiveTotalSize-total {
				return fmt.Errorf("archive cumulative size exceeds %d bytes", maxArchiveTotalSize)
			}
			total += header.Size
		default:
			return fmt.Errorf("unsupported tar entry type for %s", header.Name)
		}
	}
}

func writeArchiveFile(destination string, mode os.FileMode, open func() (io.ReadCloser, error), expected int64) (int64, error) {
	reader, err := open()
	if err != nil {
		return 0, err
	}
	defer reader.Close()
	limited := io.LimitReader(reader, maxArchiveEntrySize+1)
	written, err := writeArchiveFileContents(destination, mode, limited, expected)
	if written > maxArchiveEntrySize {
		return written, fmt.Errorf("archive file exceeds %d bytes: %s", maxArchiveEntrySize, destination)
	}
	return written, err
}

func writeTarFile(destination string, mode os.FileMode, reader io.Reader, expected int64) error {
	_, err := writeArchiveFileContents(destination, mode, reader, expected)
	return err
}

func writeArchiveFileContents(destination string, mode os.FileMode, reader io.Reader, expected int64) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(destination), dirPermShared); err != nil {
		return 0, err
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return 0, err
	}
	written, copyErr := io.Copy(file, reader)
	syncErr := file.Sync()
	closeErr := file.Close()
	if copyErr != nil {
		return written, copyErr
	}
	if expected >= 0 && written != expected {
		return written, fmt.Errorf("archive entry size mismatch: wrote %d, want %d", written, expected)
	}
	return written, errors.Join(syncErr, closeErr)
}

// ActivatePackage places a verified package in an immutable generation and
// atomically switches current. The old current remains active on smoke failure.
func ActivatePackage(installation Installation, sourceRoot string, identity packagemanifest.Identity) (string, error) {
	if installation.LegacyFlat && installation.GOOS == "windows" {
		return "", errors.New("legacy Windows dws.exe cannot migrate while running; reinstall once with the canonical Windows installer, then dws upgrade can switch version directories without replacing a locked executable")
	}
	if _, err := packagemanifest.VerifyTree(sourceRoot, identity); err != nil {
		return "", fmt.Errorf("verify staged source package: %w", err)
	}
	if err := os.MkdirAll(installation.VersionsDir, dirPermSecure); err != nil {
		return "", err
	}
	generation := generationName(identity)
	destination := filepath.Join(installation.VersionsDir, generation)
	if _, err := os.Lstat(destination); err == nil {
		if _, verifyErr := packagemanifest.VerifyTree(destination, identity); verifyErr != nil {
			return "", fmt.Errorf("existing generation is invalid: %w", verifyErr)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	} else {
		stage, err := os.MkdirTemp(installation.VersionsDir, ".stage-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(stage)
		if err := copyPackageTree(sourceRoot, stage); err != nil {
			return "", fmt.Errorf("stage package tree: %w", err)
		}
		if _, err := packagemanifest.VerifyTree(stage, identity); err != nil {
			return "", fmt.Errorf("verify placed package: %w", err)
		}
		if err := os.Rename(stage, destination); err != nil {
			return "", fmt.Errorf("place package generation: %w", err)
		}
		if _, err := packagemanifest.VerifyTree(destination, identity); err != nil {
			return "", fmt.Errorf("verify final package generation: %w", err)
		}
	}

	previous := installation.PackageRoot
	if installation.LegacyFlat {
		legacyDir := filepath.Join(installation.VersionsDir, "legacy-flat-"+time.Now().UTC().Format("20060102-150405.000000000"))
		if err := os.Mkdir(legacyDir, dirPermSecure); err != nil {
			return "", err
		}
		legacyPath := filepath.Join(legacyDir, executableNameFor(installation.GOOS, "dws"))
		if err := copyFile(installation.RunningPath, legacyPath, filePermBinary); err != nil {
			return "", fmt.Errorf("preserve legacy flat executable: %w", err)
		}
		previous = legacyDir
	}
	if !installation.LegacyFlat && sameCleanPath(previous, destination) {
		return "", nil
	}
	if err := switchPointer(installation.CurrentPath, destination, installation.GOOS, installation.TextPointer); err != nil {
		return "", fmt.Errorf("activate package: %w", err)
	}
	if installation.LegacyFlat {
		launcherRel, _, _ := packagemanifest.Paths(identity.Target)
		target := filepath.Join(installation.CurrentPath, filepath.FromSlash(launcherRel))
		if err := replaceSymlink(installation.PublicLauncher, target); err != nil {
			activationErr := fmt.Errorf("publish canonical launcher: %w", err)
			if restoreErr := RestoreActivation(installation, previous); restoreErr != nil {
				return "", activationStateUncertain(activationErr, restoreErr)
			}
			return "", activationErr
		}
	}
	if err := smokeLauncher(installation.PublicLauncher, identity.Release.Version); err != nil {
		smokeErr := fmt.Errorf("new launcher smoke check failed: %w", err)
		if restoreErr := RestoreActivation(installation, previous); restoreErr != nil {
			return "", activationStateUncertain(smokeErr, restoreErr)
		}
		return "", fmt.Errorf("new launcher smoke check failed; previous activation restored: %w", err)
	}
	return previous, nil
}

// RestoreActivation restores the package generation returned by ActivatePackage
// and proves the restored public launcher by smoke testing it.
func RestoreActivation(installation Installation, previous string) error {
	if previous == "" {
		return nil
	}
	if installation.LegacyFlat {
		legacy := filepath.Join(previous, executableNameFor(installation.GOOS, "dws"))
		info, err := os.Lstat(legacy)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			if err == nil {
				err = errors.New("preserved legacy launcher is not a regular non-symlink file")
			}
			return err
		}
		temp := filepath.Join(filepath.Dir(installation.PublicLauncher), ".dws-legacy-restore-"+fmt.Sprint(os.Getpid()))
		_ = os.Remove(temp)
		if err := copyFile(legacy, temp, info.Mode().Perm()); err != nil {
			return fmt.Errorf("stage preserved legacy launcher: %w", err)
		}
		if err := replacePointerPath(temp, installation.PublicLauncher); err != nil {
			_ = os.Remove(temp)
			return fmt.Errorf("restore preserved legacy launcher: %w", err)
		}
		if err := os.Remove(installation.CurrentPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove migrated activation pointer: %w", err)
		}
	} else if err := switchPointer(installation.CurrentPath, previous, installation.GOOS, installation.TextPointer); err != nil {
		return fmt.Errorf("restore activation pointer: %w", err)
	}
	if err := smokeLauncher(installation.PublicLauncher, installation.PreviousVersion); err != nil {
		return fmt.Errorf("smoke restored launcher: %w", err)
	}
	return nil
}

func activationStateUncertain(operationErr, restoreErr error) error {
	return errors.Join(operationErr, fmt.Errorf("%w: restoration failed: %v", ErrActivationStateUncertain, restoreErr))
}

func sameCleanPath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}

func generationName(identity packagemanifest.Identity) string {
	commit := identity.Release.Commit
	if len(commit) > 16 {
		commit = commit[:16]
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	return replacer.Replace(identity.Release.Version + "-" + commit)
}

func copyPackageTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed: %s", rel)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if info.IsDir() {
			return os.Mkdir(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("special package entry is not allowed: %s", rel)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func switchPointer(pointer, destination, goos string, textPointer bool) error {
	if textPointer {
		return replaceTextPointer(pointer, filepath.Base(destination), goos)
	}
	return replaceSymlink(pointer, destination)
}

func replaceTextPointer(path, generation, goos string) error {
	if goos != "windows" {
		return errors.New("text activation pointers are Windows-only")
	}
	if err := os.MkdirAll(filepath.Dir(path), dirPermShared); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".current-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := io.WriteString(temp, generation+"\r\n"); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return replacePointerPath(tempPath, path)
}

func replaceSymlink(path, target string) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPermShared); err != nil {
		return err
	}
	temp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+fmt.Sprintf(".tmp-%d", os.Getpid()))
	_ = os.Remove(temp)
	relative, err := filepath.Rel(filepath.Dir(path), target)
	if err != nil {
		return err
	}
	if err := os.Symlink(relative, temp); err != nil {
		return err
	}
	if err := replacePointerPath(temp, path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	syncParentDir(path)
	return nil
}

func smokeLauncher(path, version string) error {
	for _, arguments := range [][]string{{"--version"}, {"version"}} {
		ctx, cancel := contextWithTimeout()
		var command *exec.Cmd
		if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(path), ".cmd") {
			line := `"` + path + `" ` + strings.Join(arguments, " ")
			command = exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", line)
		} else {
			command = exec.CommandContext(ctx, path, arguments...)
		}
		output, err := command.CombinedOutput()
		cancel()
		if err != nil {
			return fmt.Errorf("%s: %w: %s", strings.Join(arguments, " "), err, strings.TrimSpace(string(output)))
		}
		if !strings.Contains(string(output), strings.TrimPrefix(version, "v")) {
			return fmt.Errorf("%s returned unexpected version: %s", strings.Join(arguments, " "), strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}
