// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Package runtimepayload implements the self-contained runtime payload format.
package runtimepayload

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	PayloadVersion    = "20260909"
	manifestVersion   = 2
	containerHeader   = 64
	formatVersion     = uint32(1)
	maxContainerBytes = 32 << 20
	maxBundleBytes    = int64(24 << 20)
	maxFileBytes      = int64(16 << 20)
	maxFiles          = 2 // One target library and its manifest; no auxiliary files.
)

var (
	containerMagic       = [16]byte{'D', 'W', 'S', 'R', 'T', 'P', 'A', 'Y', 'L', 'O', 'A', 'D', '1'}
	ErrBundleUnavailable = errors.New("embedded runtime payload unavailable")
	readExecutableFile   = os.ReadFile
	writeExecutableFile  = os.WriteFile
	makeCacheDirectory   = os.MkdirAll
	makeCacheTemporary   = os.MkdirTemp
	extractPayload       = extractArchive
	validatePayloadRoot  = validateRoot
	publishPayload       = publishDirectory
	newPayloadGzipWriter = gzip.NewWriterLevel
	readPayloadDirectory = os.ReadDir
	lstatPayloadFile     = os.Lstat
	openPayloadFile      = func(path string) (io.ReadCloser, error) { return os.Open(path) }
	writePayloadHeader   = func(writer *tar.Writer, header *tar.Header) error { return writer.WriteHeader(header) }
	copyPayloadFile      = io.Copy
	closePayloadFile     = func(closer io.Closer) error { return closer.Close() }
	closePayloadTar      = func(writer *tar.Writer) error { return writer.Close() }
	closePayloadGzip     = func(writer *gzip.Writer) error { return writer.Close() }
	newPayloadGzipReader = func(input io.Reader) (io.ReadCloser, error) { return gzip.NewReader(input) }
	nextPayloadEntry     = func(reader *tar.Reader) (*tar.Header, error) { return reader.Next() }
	makePayloadDirectory = os.MkdirAll
	openPayloadOutput    = func(path string, flag int, mode fs.FileMode) (io.WriteCloser, error) {
		return os.OpenFile(path, flag, mode)
	}
	copyPayloadEntry       = io.CopyN
	closePayloadOutput     = func(closer io.Closer) error { return closer.Close() }
	regularPayloadFile     = isRegularFile
	openHashInput          = func(path string) (io.ReadCloser, error) { return os.Open(path) }
	readManifestFile       = os.ReadFile
	lstatPayloadTarget     = os.Lstat
	renamePayloadDirectory = os.Rename
	removePayloadDirectory = os.RemoveAll
)

// Descriptor identifies a fixed-capacity payload container inside a binary.
type Descriptor struct {
	Offset        int
	Size          int
	ContainerSize int
	SHA256        [sha256.Size]byte
}

type manifest struct {
	FormatVersion  int    `json:"format_version"`
	PayloadVersion string `json:"payload_version"`
	Target         string `json:"target"`
	Library        string `json:"library"`
	LibrarySHA256  string `json:"library_sha256"`
	// Only decoded to recognize ownership records from the retired format 1.
	PSFileCount      int    `json:"ps_file_count,omitempty"`
	PSManifestSHA256 string `json:"ps_manifest_sha256,omitempty"`
}

// Embedded returns the target-specific payload compiled into dws.
func Embedded() []byte { return embeddedPayload }

// BuildContainer creates a deterministic, fixed-capacity payload container.
func BuildContainer(payloadRoot string, capacity int) ([]byte, error) {
	if capacity < containerHeader || capacity > maxContainerBytes {
		return nil, fmt.Errorf("runtime payload container capacity %d is invalid", capacity)
	}
	var archive bytes.Buffer
	if err := writeArchive(&archive, payloadRoot); err != nil {
		return nil, err
	}
	if archive.Len() <= 0 || int64(archive.Len()) > maxBundleBytes || archive.Len()+containerHeader > capacity {
		return nil, fmt.Errorf("runtime payload size %d exceeds container capacity %d", archive.Len(), capacity)
	}
	result := make([]byte, capacity)
	copy(result[:16], containerMagic[:])
	binary.LittleEndian.PutUint64(result[16:24], uint64(archive.Len()))
	digest := sha256.Sum256(archive.Bytes())
	copy(result[24:56], digest[:])
	binary.LittleEndian.PutUint32(result[56:60], formatVersion)
	binary.LittleEndian.PutUint32(result[60:64], uint32(capacity))
	copy(result[containerHeader:], archive.Bytes())
	return result, nil
}

// WriteContainer writes a generated payload container to path.
func WriteContainer(path, payloadRoot string, capacity int) error {
	container, err := BuildContainer(payloadRoot, capacity)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create runtime payload asset directory: %w", err)
	}
	if err := os.WriteFile(path, container, 0o644); err != nil {
		return fmt.Errorf("write runtime payload asset: %w", err)
	}
	return nil
}

// Inspect validates a standalone container and returns its descriptor.
func Inspect(container []byte) (Descriptor, error) { return inspectAt(container, 0) }

// Find locates the valid payload container compiled into a binary. Marker
// constants elsewhere in the executable are ignored because their headers and
// archive checksums cannot validate as a complete container.
func Find(binaryData []byte) (Descriptor, error) {
	searchAt := 0
	for {
		relative := bytes.Index(binaryData[searchAt:], containerMagic[:])
		if relative < 0 {
			return Descriptor{}, ErrBundleUnavailable
		}
		offset := searchAt + relative
		if descriptor, err := inspectAt(binaryData, offset); err == nil {
			return descriptor, nil
		}
		searchAt = offset + 1
	}
}

func inspectAt(data []byte, offset int) (Descriptor, error) {
	if offset < 0 || offset+containerHeader > len(data) || !bytes.Equal(data[offset:offset+16], containerMagic[:]) {
		return Descriptor{}, ErrBundleUnavailable
	}
	header := data[offset : offset+containerHeader]
	if binary.LittleEndian.Uint32(header[56:60]) != formatVersion {
		return Descriptor{}, errors.New("unsupported runtime payload format")
	}
	size := int(binary.LittleEndian.Uint64(header[16:24]))
	capacity := int(binary.LittleEndian.Uint32(header[60:64]))
	if size <= 0 || int64(size) > maxBundleBytes || capacity < containerHeader+size || capacity > maxContainerBytes || offset+capacity > len(data) {
		return Descriptor{}, errors.New("invalid embedded runtime payload size")
	}
	descriptor := Descriptor{Offset: offset, Size: size, ContainerSize: capacity}
	copy(descriptor.SHA256[:], header[24:56])
	actual := sha256.Sum256(data[offset+containerHeader : offset+containerHeader+size])
	if actual != descriptor.SHA256 {
		return Descriptor{}, errors.New("embedded runtime payload checksum mismatch")
	}
	return descriptor, nil
}

// InjectFile replaces the compiled payload slot without changing binary size.
// It is used before the final platform code signature is applied.
func InjectFile(executablePath, payloadRoot string) error {
	data, err := readExecutableFile(executablePath)
	if err != nil {
		return fmt.Errorf("read executable: %w", err)
	}
	descriptor, err := Find(data)
	if err != nil {
		return err
	}
	container, err := BuildContainer(payloadRoot, descriptor.ContainerSize)
	if err != nil {
		return err
	}
	copy(data[descriptor.Offset:descriptor.Offset+descriptor.ContainerSize], container)
	if err := writeExecutableFile(executablePath, data, 0o755); err != nil {
		return fmt.Errorf("write executable: %w", err)
	}
	return nil
}

// Materialize verifies and extracts a container into a private,
// content-addressed cache, then returns the runtime library path.
func Materialize(container []byte, userCacheDir, targetOS, targetArch string) (string, error) {
	name, err := LibraryName(targetOS, targetArch)
	if err != nil {
		return "", err
	}
	descriptor, err := Inspect(container)
	if err != nil {
		return "", err
	}
	archive := container[containerHeader : containerHeader+descriptor.Size]
	expected, err := readArchiveManifest(bytes.NewReader(archive))
	if err != nil {
		return "", err
	}
	validate := func(path string) error {
		if err := validatePayloadRoot(path, targetOS, targetArch); err != nil {
			return err
		}
		// A cached manifest cannot authorize its own replacement checksums.
		actual, err := readManifest(path)
		if err != nil || actual != expected {
			return errors.New("runtime payload manifest does not match embedded bundle")
		}
		return nil
	}
	digest := hex.EncodeToString(descriptor.SHA256[:])
	parent := filepath.Join(userCacheDir, "dws", "runtime-context", PayloadVersion)
	root := filepath.Join(parent, digest)
	if err := validate(root); err == nil {
		return filepath.Join(root, name), nil
	}
	if err := makeCacheDirectory(parent, 0o700); err != nil {
		return "", fmt.Errorf("create runtime payload cache: %w", err)
	}
	temporary, err := makeCacheTemporary(parent, ".payload-*")
	if err != nil {
		return "", fmt.Errorf("stage runtime payload: %w", err)
	}
	defer os.RemoveAll(temporary)
	if err := extractPayload(bytes.NewReader(archive), temporary); err != nil {
		return "", err
	}
	if err := validate(temporary); err != nil {
		return "", fmt.Errorf("validate extracted runtime payload: %w", err)
	}
	if err := publishPayload(temporary, root, targetOS, targetArch); err != nil {
		return "", fmt.Errorf("publish runtime payload cache: %w", err)
	}
	if err := validate(root); err != nil {
		return "", fmt.Errorf("validate runtime payload cache: %w", err)
	}
	return filepath.Join(root, name), nil
}

// MaterializeFile locates and extracts a payload from a foreign target binary.
// Release validation uses this without executing the artifact.
func MaterializeFile(executablePath, userCacheDir, targetOS, targetArch string) (string, error) {
	data, err := readExecutableFile(executablePath)
	if err != nil {
		return "", fmt.Errorf("read executable payload: %w", err)
	}
	descriptor, err := Find(data)
	if err != nil {
		return "", err
	}
	return Materialize(data[descriptor.Offset:descriptor.Offset+descriptor.ContainerSize], userCacheDir, targetOS, targetArch)
}

// LibraryName returns the library shipped for a target.
func LibraryName(goos, goarch string) (string, error) {
	switch goos + "/" + goarch {
	case "darwin/amd64", "darwin/arm64":
		return "x7k2m9p4q1w8.dylib", nil
	case "linux/amd64", "linux/arm64":
		return "libx7k2m9p4q1w8.so", nil
	case "windows/amd64", "windows/arm64":
		return "x7k2m9p4q1w864.dll", nil
	default:
		return "", fmt.Errorf("unsupported runtime platform %s/%s", goos, goarch)
	}
}

func writeArchive(output io.Writer, root string) error {
	metadata, err := readManifest(root)
	if err != nil {
		return err
	}
	targetOS, targetArch, ok := strings.Cut(metadata.Target, "/")
	if !ok {
		return errors.New("invalid runtime payload target")
	}
	if err := validateRoot(root, targetOS, targetArch); err != nil {
		return fmt.Errorf("validate runtime payload source: %w", err)
	}
	gzipWriter, err := newPayloadGzipWriter(output, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("create runtime payload compressor: %w", err)
	}
	gzipWriter.Header.ModTime = time.Unix(0, 0).UTC()
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	paths := []string{"manifest.json", metadata.Library}
	sort.Strings(paths)
	for _, relative := range paths {
		path := filepath.Join(root, filepath.FromSlash(relative))
		info, err := lstatPayloadFile(path)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("invalid runtime payload file %s", relative)
		}
		header := &tar.Header{Name: relative, Mode: 0o600, Size: info.Size(), ModTime: time.Unix(0, 0).UTC(), Typeflag: tar.TypeReg}
		if relative == metadata.Library {
			header.Mode = 0o700
		}
		if err := writePayloadHeader(tarWriter, header); err != nil {
			return fmt.Errorf("write runtime payload header: %w", err)
		}
		file, err := openPayloadFile(path)
		if err != nil {
			return fmt.Errorf("open runtime payload file: %w", err)
		}
		_, copyErr := copyPayloadFile(tarWriter, file)
		closeErr := closePayloadFile(file)
		if copyErr != nil {
			return fmt.Errorf("write runtime payload file: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close runtime payload file: %w", closeErr)
		}
	}
	if err := closePayloadTar(tarWriter); err != nil {
		return fmt.Errorf("finalize runtime payload archive: %w", err)
	}
	if err := closePayloadGzip(gzipWriter); err != nil {
		return fmt.Errorf("finalize runtime payload compressor: %w", err)
	}
	return nil
}

func extractArchive(input io.Reader, root string) error {
	gzipReader, err := newPayloadGzipReader(input)
	if err != nil {
		return fmt.Errorf("open embedded runtime payload: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	seen := make(map[string]struct{})
	var total int64
	for count := 0; ; count++ {
		header, err := nextPayloadEntry(tarReader)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read embedded runtime payload: %w", err)
		}
		if count >= maxFiles {
			return errors.New("embedded runtime payload contains too many entries")
		}
		if header.Typeflag != tar.TypeReg || !validArchivePath(header.Name) {
			return fmt.Errorf("invalid embedded runtime payload entry %q", header.Name)
		}
		if _, duplicate := seen[header.Name]; duplicate {
			return fmt.Errorf("duplicate embedded runtime payload entry %q", header.Name)
		}
		seen[header.Name] = struct{}{}
		if header.Size < 0 || header.Size > maxFileBytes || total+header.Size > maxBundleBytes {
			return fmt.Errorf("invalid embedded runtime payload entry size for %q", header.Name)
		}
		total += header.Size
		destination := filepath.Join(root, filepath.FromSlash(header.Name))
		if err := makePayloadDirectory(filepath.Dir(destination), 0o700); err != nil {
			return fmt.Errorf("create runtime payload directory: %w", err)
		}
		mode := fs.FileMode(0o600)
		if strings.HasSuffix(header.Name, ".dylib") || strings.HasSuffix(header.Name, ".so") || strings.HasSuffix(header.Name, ".dll") {
			mode = 0o700
		}
		file, err := openPayloadOutput(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			return fmt.Errorf("create runtime payload file: %w", err)
		}
		_, copyErr := copyPayloadEntry(file, tarReader, header.Size)
		closeErr := closePayloadOutput(file)
		if copyErr != nil {
			return fmt.Errorf("extract runtime payload file: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close runtime payload file: %w", closeErr)
		}
	}
	return nil
}

func validArchivePath(name string) bool {
	return name == "manifest.json" || name == "x7k2m9p4q1w8.dylib" || name == "libx7k2m9p4q1w8.so" || name == "x7k2m9p4q1w864.dll"
}

func validateRoot(root, targetOS, targetArch string) error {
	metadata, err := readManifest(root)
	if err != nil {
		return err
	}
	// Build sources and private caches are dedicated bundle roots. Adjacent
	// publication validates only its owned library, leaving unrelated files alone.
	entries, err := readPayloadDirectory(root)
	if err != nil || len(entries) != maxFiles {
		return errors.New("runtime payload must contain only its library and manifest")
	}
	for _, entry := range entries {
		if entry.Name() != metadata.Library && entry.Name() != "manifest.json" {
			return errors.New("runtime payload contains an unexpected file")
		}
	}
	return validateRootManifest(root, metadata, targetOS, targetArch)
}

func validateRootManifest(root string, metadata manifest, targetOS, targetArch string) error {
	name, err := LibraryName(targetOS, targetArch)
	if err != nil {
		return err
	}
	if metadata.FormatVersion != manifestVersion || metadata.PayloadVersion != PayloadVersion || metadata.Target != targetOS+"/"+targetArch || metadata.Library != name {
		return errors.New("runtime payload manifest does not match target")
	}
	if metadata.PSFileCount != 0 || metadata.PSManifestSHA256 != "" {
		return errors.New("runtime payload manifest contains retired data files")
	}
	libraryPath := filepath.Join(root, name)
	if !regularPayloadFile(libraryPath) {
		return errors.New("runtime payload library is unavailable")
	}
	libraryDigest, err := hashFile(libraryPath)
	if err != nil || hex.EncodeToString(libraryDigest[:]) != metadata.LibrarySHA256 {
		return errors.New("runtime payload library checksum mismatch")
	}
	return nil
}

func readManifest(root string) (manifest, error) {
	path := filepath.Join(root, "manifest.json")
	if !regularPayloadFile(path) {
		return manifest{}, errors.New("runtime payload manifest is unavailable")
	}
	data, err := readManifestFile(path)
	if err != nil {
		return manifest{}, fmt.Errorf("read runtime payload manifest: %w", err)
	}
	var result manifest
	if err := json.Unmarshal(data, &result); err != nil {
		return manifest{}, fmt.Errorf("parse runtime payload manifest: %w", err)
	}
	return result, nil
}

func isRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func hashFile(path string) ([sha256.Size]byte, error) {
	file, err := openHashInput(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer file.Close()
	return hashReader(file)
}

func hashReader(reader io.Reader) ([sha256.Size]byte, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return [sha256.Size]byte{}, err
	}
	var result [sha256.Size]byte
	copy(result[:], hasher.Sum(nil))
	return result, nil
}

func publishDirectory(temporary, target, targetOS, targetArch string) error {
	if info, err := lstatPayloadTarget(target); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return errors.New("runtime payload cache target is unsafe")
	}
	if err := renamePayloadDirectory(temporary, target); err == nil {
		return nil
	}
	// Only another publisher of this exact bundle may win the race. A cache
	// manifest with replacement checksums must be repaired, even if self-consistent.
	expected, sourceErr := readManifest(temporary)
	actual, targetErr := readManifest(target)
	if sourceErr == nil && targetErr == nil && actual == expected && validateRoot(target, targetOS, targetArch) == nil {
		return nil
	}
	if info, err := lstatPayloadTarget(target); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("runtime payload cache target is unsafe")
		}
		if err := removePayloadDirectory(target); err != nil {
			return err
		}
	}
	return renamePayloadDirectory(temporary, target)
}
