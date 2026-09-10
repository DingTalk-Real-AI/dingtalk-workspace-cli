//go:build windows && (amd64 || arm64)

package schemacache

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

const (
	stagingAttempts       = 16
	aggregateBufferSize   = 128 << 10
	lockfileExclusive     = 0x00000002
	lockfileFailImmediate = 0x00000001
	// Readers keep process-lifetime handles. Windows cannot POSIX-rename over
	// an open inode, so read handles must share write+delete:
	//   - os.WriteFile (repair / corrupt-recovery) needs FILE_SHARE_WRITE
	//   - Publish's MoveFileEx(REPLACE_EXISTING) needs FILE_SHARE_DELETE
	// Readers re-stat + re-digest before serving; same-UID mutate is already
	// in the threat model.
	secureShareRead = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE
	secureShareLock = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE
)

var (
	userCacheDir             = os.UserCacheDir
	programDataDir           = func() string { return os.Getenv("ProgramData") }
	platformIO     windowsIO = realWindowsIO{}
	currentGOOS              = runtime.GOOS
)

type windowsIO interface {
	attributes(path string) (uint32, error)
	mkdir(path string) error
	open(path string, access, share, disposition, flags uint32) (windows.Handle, error)
	info(h windows.Handle) (windows.ByHandleFileInformation, error)
	readAt(h windows.Handle, p []byte, offset int64) (int, error)
	write(h windows.Handle, p []byte) (int, error)
	flush(h windows.Handle) error
	close(h windows.Handle) error
	rename(oldpath, newpath string) error
	remove(path string) error
	lock(h windows.Handle) error
	unlock(h windows.Handle) error
	restrictACL(path string) error
	random(p []byte) (int, error)
}

type realWindowsIO struct{}

func (realWindowsIO) attributes(path string) (uint32, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	return windows.GetFileAttributes(p)
}

func (realWindowsIO) mkdir(path string) error { return os.Mkdir(path, 0o700) }

func (realWindowsIO) open(path string, access, share, disposition, flags uint32) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(p, access, share, nil, disposition, flags, 0)
}

func (realWindowsIO) info(h windows.Handle) (windows.ByHandleFileInformation, error) {
	var info windows.ByHandleFileInformation
	err := windows.GetFileInformationByHandle(h, &info)
	return info, err
}

func (realWindowsIO) readAt(h windows.Handle, p []byte, offset int64) (int, error) {
	var overlapped windows.Overlapped
	overlapped.Offset = uint32(offset)
	overlapped.OffsetHigh = uint32(offset >> 32)
	var done uint32
	err := windows.ReadFile(h, p, &done, &overlapped)
	return int(done), err
}

func (realWindowsIO) write(h windows.Handle, p []byte) (int, error) {
	var done uint32
	err := windows.WriteFile(h, p, &done, nil)
	return int(done), err
}

func (realWindowsIO) flush(h windows.Handle) error { return windows.FlushFileBuffers(h) }

func (realWindowsIO) close(h windows.Handle) error { return windows.CloseHandle(h) }

func (realWindowsIO) rename(oldpath, newpath string) error {
	return windows.Rename(oldpath, newpath)
}

func (realWindowsIO) remove(path string) error { return os.Remove(path) }

func (realWindowsIO) lock(h windows.Handle) error {
	ol := new(windows.Overlapped)
	return windows.LockFileEx(h, lockfileExclusive|lockfileFailImmediate, 0, 1, 0, ol)
}

func (realWindowsIO) unlock(h windows.Handle) error {
	ol := new(windows.Overlapped)
	return windows.UnlockFileEx(h, 0, 1, 0, ol)
}

func (realWindowsIO) restrictACL(path string) error { return restrictOwnerWrite(path) }

func (realWindowsIO) random(p []byte) (int, error) { return rand.Read(p) }

type windowsCache struct {
	mu       sync.RWMutex
	path     string
	edition  [32]byte
	counters *Counters
	ops      windowsIO
	closed   bool
	shared   bool
}

type fileState struct {
	volume uint32
	indexH uint32
	indexL uint32
	size   int64
	attrs  uint32
	nlink  uint32
}

// systemSchemaCacheBase is %ProgramData%\dws. The runtime never creates it;
// only the installer does. Layout under the base is dws\schema\<edition>\v1,
// matching the unix shared-cache contract.
func systemSchemaCacheBase() string {
	if currentGOOS != "windows" {
		return ""
	}
	base := strings.TrimSpace(programDataDir())
	if base == "" {
		return ""
	}
	base = filepath.Clean(base)
	if !filepath.IsAbs(base) {
		return ""
	}
	return filepath.Join(base, "dws")
}

func openPlatform(edition string, counters *Counters, noCreate bool) (backend, error) {
	digest := sha256.Sum256([]byte(edition))
	editionHex := hex.EncodeToString(digest[:])
	if override := os.Getenv("DWS_SCHEMA_CACHE_DIR"); override != "" {
		path, err := openCacheDirectory(override, editionHex, counters, platformIO, noCreate, true)
		if err != nil {
			return nil, err
		}
		return &windowsCache{path: path, edition: digest, counters: counters, ops: platformIO, shared: true}, nil
	}
	if systemBase := systemSchemaCacheBase(); systemBase != "" {
		if path, err := openCacheDirectory(systemBase, editionHex, counters, platformIO, true, true); err == nil {
			return &windowsCache{path: path, edition: digest, counters: counters, ops: platformIO, shared: true}, nil
		}
	}
	base, err := userCacheDir()
	if err != nil {
		return nil, fmt.Errorf("%w: user cache directory: %v", ErrDisabled, err)
	}
	path, err := openCacheDirectory(base, editionHex, counters, platformIO, noCreate, false)
	if err != nil {
		return nil, err
	}
	return &windowsCache{path: path, edition: digest, counters: counters, ops: platformIO, shared: false}, nil
}

func openCacheDirectory(base, editionHex string, counters *Counters, ops windowsIO, noCreate bool, shared bool) (string, error) {
	if !filepath.IsAbs(base) || filepath.Clean(base) != base {
		return "", fmt.Errorf("%w: cache base must be a clean absolute path", ErrUnsafePath)
	}
	current := filepath.VolumeName(base)
	if current == "" {
		return "", fmt.Errorf("%w: cache base missing volume", ErrUnsafePath)
	}
	current += `\`
	counters.rootOpenOps.Add(1)
	if err := validateAncestryPath(current, counters, ops); err != nil {
		return "", err
	}
	rest := strings.TrimPrefix(base, filepath.VolumeName(base))
	rest = strings.TrimPrefix(rest, `\`)
	var parts []string
	if rest != "" {
		parts = strings.Split(rest, `\`)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("%w: unsafe cache ancestry component", ErrUnsafePath)
		}
		next := filepath.Join(current, part)
		counters.rootOpenOps.Add(1)
		attrs, err := ops.attributes(next)
		if isNotFound(err) {
			if noCreate {
				return "", fmt.Errorf("%w: cache ancestry %s", ErrNotFound, part)
			}
			if err := validateAncestryPath(current, counters, ops); err != nil {
				return "", fmt.Errorf("%w: missing cache ancestry requires a safe parent", ErrUnsafePath)
			}
			counters.mkdirOps.Add(1)
			if mkdirErr := ops.mkdir(next); mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
				return "", fmt.Errorf("%w: create cache ancestry: %v", ErrUnsafePath, mkdirErr)
			}
			_ = ops.restrictACL(next)
			attrs, err = ops.attributes(next)
		}
		if err != nil {
			return "", fmt.Errorf("%w: open cache ancestry: %v", ErrUnsafePath, err)
		}
		if err := validateAttrsDirectory(attrs, false); err != nil {
			return "", err
		}
		current = next
	}
	for _, part := range []string{"dws", "schema", editionHex, "v1"} {
		next := filepath.Join(current, part)
		counters.rootOpenOps.Add(1)
		attrs, err := ops.attributes(next)
		if isNotFound(err) {
			if noCreate {
				return "", fmt.Errorf("%w: cache directory %s", ErrNotFound, part)
			}
			counters.mkdirOps.Add(1)
			if mkdirErr := ops.mkdir(next); mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
				return "", fmt.Errorf("%w: create cache directory: %v", ErrUnsafePath, mkdirErr)
			}
			if aclErr := ops.restrictACL(next); aclErr != nil {
				return "", fmt.Errorf("%w: restrict cache directory ACL: %v", ErrUnsafePath, aclErr)
			}
			attrs, err = ops.attributes(next)
		}
		if err != nil {
			return "", fmt.Errorf("%w: open cache directory: %v", ErrUnsafePath, err)
		}
		if err := validateAttrsDirectory(attrs, true); err != nil {
			return "", err
		}
		current = next
	}
	return current, nil
}

func isNotFound(err error) bool {
	return errors.Is(err, windows.ERROR_FILE_NOT_FOUND) ||
		errors.Is(err, windows.ERROR_PATH_NOT_FOUND) ||
		errors.Is(err, os.ErrNotExist)
}

func validateAncestryPath(path string, counters *Counters, ops windowsIO) error {
	counters.statOps.Add(1)
	attrs, err := ops.attributes(path)
	if err != nil {
		return fmt.Errorf("%w: cache ancestry: %v", ErrUnsafePath, err)
	}
	return validateAttrsDirectory(attrs, false)
}

func validateAttrsDirectory(attrs uint32, owned bool) error {
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("%w: cache path is a reparse point", ErrUnsafePath)
	}
	if attrs&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		if owned {
			return fmt.Errorf("%w: cache directory must be a directory", ErrUnsafePath)
		}
		return fmt.Errorf("%w: unsafe cache ancestry ownership or mode", ErrUnsafePath)
	}
	return nil
}

func validateCacheFile(info windows.ByHandleFileInformation, shared bool) error {
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("%w: cache file is a reparse point", ErrUnsafePath)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return fmt.Errorf("%w: cache file must be a single-link regular file", ErrUnsafePath)
	}
	if info.NumberOfLinks != 1 {
		return fmt.Errorf("%w: cache file must be a single-link regular file", ErrUnsafePath)
	}
	_ = shared
	return nil
}

func sameFileState(a, b fileState) bool {
	return a.volume == b.volume && a.indexH == b.indexH && a.indexL == b.indexL &&
		a.size == b.size && a.attrs == b.attrs && a.nlink == b.nlink
}

func fileStateFrom(info windows.ByHandleFileInformation) fileState {
	return fileState{
		volume: info.VolumeSerialNumber,
		indexH: info.FileIndexHigh,
		indexL: info.FileIndexLow,
		size:   int64(info.FileSizeHigh)<<32 | int64(info.FileSizeLow),
		attrs:  info.FileAttributes,
		nlink:  info.NumberOfLinks,
	}
}

func restrictOwnerWrite(path string) error {
	user, err := currentUserSID()
	if err != nil {
		return err
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	access := []windows.EXPLICIT_ACCESS{
		explicitAccess(user, windows.TRUSTEE_IS_USER),
		explicitAccess(system, windows.TRUSTEE_IS_USER),
	}
	acl, err := windows.ACLFromEntries(access, nil)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil,
	)
}

func explicitAccess(sid *windows.SID, trusteeType windows.TRUSTEE_TYPE) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  trusteeType,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}

func currentUserSID() (*windows.SID, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return nil, err
	}
	defer token.Close()
	tu, err := token.GetTokenUser()
	if err != nil {
		return nil, err
	}
	return tu.User.Sid, nil
}

func (c *windowsCache) directory() string { return c.path }

func (c *windowsCache) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *windowsCache) secureOpen(name string, access, share, disposition uint32) (windows.Handle, fileState, error) {
	path := filepath.Join(c.path, name)
	if filepath.Base(path) != name {
		return 0, fileState{}, fmt.Errorf("%w: %s", ErrUnsafePath, name)
	}
	fd, err := c.ops.open(path, access, share, disposition, windows.FILE_FLAG_OPEN_REPARSE_POINT)
	c.counters.fileOpenOps.Add(1)
	if err != nil {
		if isNotFound(err) {
			return 0, fileState{}, fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return 0, fileState{}, fmt.Errorf("%w: open %s: %v", ErrUnsafePath, name, err)
	}
	c.counters.statOps.Add(1)
	info, err := c.ops.info(fd)
	if err == nil {
		err = validateCacheFile(info, c.shared)
	}
	if err != nil {
		_ = c.ops.close(fd)
		c.counters.closeOps.Add(1)
		return 0, fileState{}, err
	}
	return fd, fileStateFrom(info), nil
}

func (c *windowsCache) readMeta(identity ExpectedIdentity, expected ArtifactExpectation) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return nil, ErrClosed
	}
	if !digestEqual(c.edition, identity.EditionSHA256) {
		return nil, ErrIdentityMismatch
	}
	fd, initial, err := c.secureOpen(metaFileName, windows.GENERIC_READ, secureShareRead, windows.OPEN_EXISTING)
	if err != nil {
		return nil, err
	}
	closeWith := func(resultErr error) error {
		c.counters.closeOps.Add(1)
		if closeErr := c.ops.close(fd); resultErr == nil && closeErr != nil {
			return fmt.Errorf("%w: close Meta: %v", ErrInvalidArtifact, closeErr)
		}
		return resultErr
	}
	wantSize, err := checkedFileSize(expected.EncodedLength)
	if err != nil || initial.size != wantSize {
		return nil, closeWith(fmt.Errorf("%w: Meta file size mismatch", ErrInvalidArtifact))
	}
	header := make([]byte, HeaderSize)
	if err := c.preadFull(fd, header, 0, readHeader); err != nil {
		return nil, closeWith(err)
	}
	envelope, err := ParseEnvelope(header)
	if err != nil {
		return nil, closeWith(err)
	}
	if err := envelope.authenticate(identity, expected); err != nil {
		return nil, closeWith(err)
	}
	payload := make([]byte, int(expected.EncodedLength))
	if err := c.preadFull(fd, payload, HeaderSize, readMetaPayload); err != nil {
		return nil, closeWith(err)
	}
	digest := sha256.Sum256(payload)
	if !digestEqual(digest, expected.EncodedSHA256) {
		return nil, closeWith(fmt.Errorf("%w: Meta payload digest mismatch", ErrIdentityMismatch))
	}
	c.counters.statOps.Add(1)
	info, err := c.ops.info(fd)
	if err != nil {
		return nil, closeWith(err)
	}
	if !sameFileState(initial, fileStateFrom(info)) {
		return nil, closeWith(fmt.Errorf("%w: Meta file changed during read", ErrInvalidArtifact))
	}
	if err := closeWith(nil); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *windowsCache) openRegistry(identity ExpectedIdentity, expected ArtifactExpectation) (registryBackend, error) {
	return c.openShard(identity, expected, registryFileName, readRegistryPayload)
}

func (c *windowsCache) openPayloads(identity ExpectedIdentity, expected ArtifactExpectation) (registryBackend, error) {
	return c.openShard(identity, expected, payloadFileName, readPayloadPayload)
}

func (c *windowsCache) openShard(identity ExpectedIdentity, expected ArtifactExpectation, name string, category readCategory) (registryBackend, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return nil, ErrClosed
	}
	if !digestEqual(c.edition, identity.EditionSHA256) {
		return nil, ErrIdentityMismatch
	}
	fd, initial, err := c.secureOpen(name, windows.GENERIC_READ, secureShareRead, windows.OPEN_EXISTING)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (registryBackend, error) {
		_ = c.ops.close(fd)
		c.counters.closeOps.Add(1)
		return nil, err
	}
	wantSize, err := checkedFileSize(expected.EncodedLength)
	if err != nil || initial.size != wantSize {
		return fail(fmt.Errorf("%w: %s file size mismatch", ErrInvalidArtifact, name))
	}
	header := make([]byte, HeaderSize)
	if err := c.preadFull(fd, header, 0, readHeader); err != nil {
		return fail(err)
	}
	envelope, err := ParseEnvelope(header)
	if err != nil {
		return fail(err)
	}
	if err := envelope.authenticate(identity, expected); err != nil {
		return fail(err)
	}
	c.counters.statOps.Add(1)
	info, err := c.ops.info(fd)
	if err != nil {
		return fail(err)
	}
	if !sameFileState(initial, fileStateFrom(info)) {
		return fail(fmt.Errorf("%w: %s file changed during open", ErrInvalidArtifact, name))
	}
	return &windowsRegistry{fd: fd, initial: initial, expected: expected, counters: c.counters, ops: c.ops, category: category}, nil
}

type readCategory uint8

const (
	readHeader readCategory = iota
	readMetaPayload
	readRegistryPayload
	readPayloadPayload
)

func (c *windowsCache) preadFull(fd windows.Handle, p []byte, offset int64, category readCategory) error {
	for len(p) > 0 {
		switch category {
		case readHeader:
			c.counters.headerReadOps.Add(1)
		case readMetaPayload:
			c.counters.metaPayloadReadOps.Add(1)
		case readRegistryPayload:
			c.counters.registryReadOps.Add(1)
		case readPayloadPayload:
			c.counters.payloadReadOps.Add(1)
		}
		n, err := c.ops.readAt(fd, p, offset)
		if n > 0 {
			if category == readMetaPayload {
				c.counters.metaPayloadReadBytes.Add(uint64(n))
			} else if category == readRegistryPayload {
				c.counters.registryReadBytes.Add(uint64(n))
			} else if category == readPayloadPayload {
				c.counters.payloadReadBytes.Add(uint64(n))
			}
			p = p[n:]
			offset += int64(n)
		}
		if err != nil && !errors.Is(err, windows.ERROR_IO_PENDING) {
			if n == 0 {
				if errors.Is(err, io.EOF) || errors.Is(err, windows.ERROR_HANDLE_EOF) {
					return fmt.Errorf("%w: short pread: %v", ErrInvalidArtifact, io.ErrUnexpectedEOF)
				}
				return fmt.Errorf("%w: pread: %v", ErrInvalidArtifact, err)
			}
		}
		if n == 0 {
			return fmt.Errorf("%w: short pread: %v", ErrInvalidArtifact, io.ErrUnexpectedEOF)
		}
	}
	return nil
}

type windowsRegistry struct {
	mu       sync.RWMutex
	fd       windows.Handle
	initial  fileState
	expected ArtifactExpectation
	counters *Counters
	ops      windowsIO
	category readCategory
	closed   bool
}

func (r *windowsRegistry) close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	r.counters.closeOps.Add(1)
	return r.ops.close(r.fd)
}

func (r *windowsRegistry) readRange(descriptor RangeDescriptor) ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, ErrClosed
	}
	if descriptor.Offset > r.expected.EncodedLength || descriptor.Length > r.expected.EncodedLength-descriptor.Offset {
		return nil, fmt.Errorf("%w: product range is outside Registry payload", ErrInvalidArtifact)
	}
	r.counters.statOps.Add(1)
	info, err := r.ops.info(r.fd)
	if err != nil {
		return nil, err
	}
	if !sameFileState(r.initial, fileStateFrom(info)) {
		return nil, fmt.Errorf("%w: Registry file changed before range read", ErrInvalidArtifact)
	}
	payload := make([]byte, int(descriptor.Length))
	reader := windowsCache{counters: r.counters, ops: r.ops}
	if err := reader.preadFull(r.fd, payload, int64(HeaderSize+descriptor.Offset), r.category); err != nil {
		return nil, err
	}
	if digest := sha256.Sum256(payload); !digestEqual(digest, descriptor.SHA256) {
		return nil, fmt.Errorf("%w: product range digest mismatch", ErrIdentityMismatch)
	}
	r.counters.statOps.Add(1)
	after, err := r.ops.info(r.fd)
	if err != nil {
		return nil, err
	}
	if !sameFileState(r.initial, fileStateFrom(after)) {
		return nil, fmt.Errorf("%w: Registry file changed during range read", ErrInvalidArtifact)
	}
	return payload, nil
}

func (r *windowsRegistry) validateAggregate() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return ErrClosed
	}
	r.counters.statOps.Add(1)
	info, err := r.ops.info(r.fd)
	if err != nil {
		return err
	}
	if !sameFileState(r.initial, fileStateFrom(info)) {
		return fmt.Errorf("%w: Registry file changed before aggregate read", ErrInvalidArtifact)
	}
	hash := sha256.New()
	buffer := make([]byte, aggregateBufferSize)
	reader := windowsCache{counters: r.counters, ops: r.ops}
	var offset uint64
	for offset < r.expected.EncodedLength {
		length := r.expected.EncodedLength - offset
		if length > uint64(len(buffer)) {
			length = uint64(len(buffer))
		}
		chunk := buffer[:int(length)]
		if err := reader.preadFull(r.fd, chunk, int64(HeaderSize+offset), r.category); err != nil {
			return err
		}
		_, _ = hash.Write(chunk)
		offset += length
	}
	var digest [32]byte
	copy(digest[:], hash.Sum(nil))
	if !digestEqual(digest, r.expected.EncodedSHA256) {
		return fmt.Errorf("%w: Registry aggregate digest mismatch", ErrIdentityMismatch)
	}
	r.counters.statOps.Add(1)
	after, err := r.ops.info(r.fd)
	if err != nil {
		return err
	}
	if !sameFileState(r.initial, fileStateFrom(after)) {
		return fmt.Errorf("%w: Registry file changed during aggregate read", ErrInvalidArtifact)
	}
	return nil
}

func (c *windowsCache) writeArtifact(identity ExpectedIdentity, artifact Artifact) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return ErrClosed
	}
	if !digestEqual(c.edition, identity.EditionSHA256) {
		return ErrIdentityMismatch
	}
	envelope, err := envelopeFrom(identity, artifact.Expectation)
	if err != nil {
		return err
	}
	header, _ := envelope.MarshalBinary()
	target := metaFileName
	if artifact.Expectation.Kind == KindRegistry {
		target = registryFileName
	} else if artifact.Expectation.Kind == KindPayloads {
		target = payloadFileName
	}
	return c.atomicReplace(target, header, artifact.Payload)
}

func (c *windowsCache) atomicReplace(target string, header, payload []byte) error {
	var randomBytes [16]byte
	var fd windows.Handle
	var staging string
	opened := false
	for attempt := 0; attempt < stagingAttempts; attempt++ {
		if _, err := io.ReadFull(randomReader{c.ops}, randomBytes[:]); err != nil {
			return fmt.Errorf("create staging name: %w", err)
		}
		staging = "." + target + "." + hex.EncodeToString(randomBytes[:]) + ".tmp"
		var err error
		fd, err = c.ops.open(
			filepath.Join(c.path, staging),
			windows.GENERIC_WRITE,
			0,
			windows.CREATE_NEW,
			windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		)
		c.counters.fileOpenOps.Add(1)
		if err == nil {
			opened = true
			break
		}
		if !errors.Is(err, windows.ERROR_FILE_EXISTS) && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create staging file: %w", err)
		}
	}
	if !opened {
		return fmt.Errorf("create staging file: exhausted %d attempts", stagingAttempts)
	}
	stagingPath := filepath.Join(c.path, staging)
	staged := true
	cleanup := func() {
		if opened {
			_ = c.ops.close(fd)
			c.counters.closeOps.Add(1)
			opened = false
		}
		if staged {
			_ = c.ops.remove(stagingPath)
			c.counters.removeOps.Add(1)
		}
	}
	c.counters.statOps.Add(1)
	info, err := c.ops.info(fd)
	if err == nil {
		err = validateCacheFile(info, c.shared)
	}
	if err != nil {
		cleanup()
		return err
	}
	if err := c.ops.restrictACL(stagingPath); err != nil {
		cleanup()
		return fmt.Errorf("restrict staging ACL: %w", err)
	}
	if err := c.writeFull(fd, header); err != nil {
		cleanup()
		return err
	}
	if err := c.writeFull(fd, payload); err != nil {
		cleanup()
		return err
	}
	c.counters.fileSyncOps.Add(1)
	if err := c.ops.flush(fd); err != nil {
		cleanup()
		return fmt.Errorf("sync staging file: %w", err)
	}
	c.counters.closeOps.Add(1)
	if err := c.ops.close(fd); err != nil {
		opened = false
		cleanup()
		return fmt.Errorf("close staging file: %w", err)
	}
	opened = false
	c.counters.renameOps.Add(1)
	if err := c.ops.rename(stagingPath, filepath.Join(c.path, target)); err != nil {
		cleanup()
		return fmt.Errorf("replace %s: %w", target, err)
	}
	staged = false
	c.counters.directorySyncOps.Add(1)
	return nil
}

type randomReader struct{ ops windowsIO }

func (r randomReader) Read(p []byte) (int, error) { return r.ops.random(p) }

func (c *windowsCache) writeFull(fd windows.Handle, p []byte) error {
	for len(p) > 0 {
		c.counters.writeOps.Add(1)
		n, err := c.ops.write(fd, p)
		if n > 0 {
			c.counters.writeBytes.Add(uint64(n))
			p = p[n:]
		}
		if err != nil {
			return fmt.Errorf("write staging file: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("write staging file: %w", io.ErrShortWrite)
		}
	}
	return nil
}

var localLocks sync.Map

type localLock struct{ token chan struct{} }

func localLockFor(path string) *localLock {
	created := &localLock{token: make(chan struct{}, 1)}
	created.token <- struct{}{}
	actual, _ := localLocks.LoadOrStore(path, created)
	return actual.(*localLock)
}

func (c *windowsCache) acquire(ctx context.Context, timeout time.Duration) (lockBackend, error) {
	if timeout < 0 {
		timeout = 0
	}
	deadline := time.Now().Add(timeout)
	local := localLockFor(c.path)
	if err := takeLocalLock(ctx, local, deadline); err != nil {
		return nil, err
	}
	releaseLocal := true
	defer func() {
		if releaseLocal {
			local.token <- struct{}{}
		}
	}()

	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, ErrClosed
	}
	fd, _, err := c.secureOpen(lockFileName, windows.GENERIC_READ|windows.GENERIC_WRITE, secureShareLock, windows.OPEN_ALWAYS)
	c.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	_ = c.ops.restrictACL(filepath.Join(c.path, lockFileName))
	closeFD := func() {
		_ = c.ops.close(fd)
		c.counters.closeOps.Add(1)
	}
	for {
		c.counters.lockAttempts.Add(1)
		err = c.ops.lock(fd)
		if err == nil {
			releaseLocal = false
			return &windowsLock{fd: fd, local: local, counters: c.counters, ops: c.ops}, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) && !errors.Is(err, windows.ERROR_IO_PENDING) {
			closeFD()
			return nil, fmt.Errorf("acquire schema cache lock: %w", err)
		}
		if err := waitForRetry(ctx, deadline); err != nil {
			closeFD()
			return nil, err
		}
	}
}

func takeLocalLock(ctx context.Context, lock *localLock, deadline time.Time) error {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		select {
		case <-lock.token:
			return nil
		default:
			return ErrLockTimeout
		}
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-lock.token:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrLockTimeout
	}
}

func waitForRetry(ctx context.Context, deadline time.Time) error {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return ErrLockTimeout
	}
	if remaining > 10*time.Millisecond {
		remaining = 10 * time.Millisecond
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		if time.Now().Before(deadline) {
			return nil
		}
		return ErrLockTimeout
	}
}

type windowsLock struct {
	fd       windows.Handle
	local    *localLock
	counters *Counters
	ops      windowsIO
}

func (l *windowsLock) release() error {
	unlockErr := l.ops.unlock(l.fd)
	l.counters.closeOps.Add(1)
	closeErr := l.ops.close(l.fd)
	l.local.token <- struct{}{}
	if unlockErr != nil {
		return fmt.Errorf("release schema cache lock: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close schema cache lock: %w", closeErr)
	}
	return nil
}
