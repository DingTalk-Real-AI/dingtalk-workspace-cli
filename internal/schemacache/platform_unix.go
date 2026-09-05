//go:build (darwin && arm64) || (linux && amd64)

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
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const (
	secureDirectoryFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC
	secureReadFlags      = unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
	secureLockFlags      = unix.O_CREAT | unix.O_RDWR | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
	stagingAttempts      = 16
	aggregateBufferSize  = 128 << 10
)

var (
	userCacheDir        = os.UserCacheDir
	platformIO   unixIO = realUnixIO{}
)

type unixIO interface {
	open(path string, flags int, mode uint32) (int, error)
	openat(dirfd int, path string, flags int, mode uint32) (int, error)
	mkdirat(dirfd int, path string, mode uint32) error
	fstat(fd int, stat *unix.Stat_t) error
	pread(fd int, p []byte, offset int64) (int, error)
	write(fd int, p []byte) (int, error)
	fsync(fd int) error
	close(fd int) error
	renameat(olddirfd int, oldpath string, newdirfd int, newpath string) error
	unlinkat(dirfd int, path string, flags int) error
	flock(fd int, how int) error
	random(p []byte) (int, error)
}

type realUnixIO struct{}

func (realUnixIO) open(path string, flags int, mode uint32) (int, error) {
	return unix.Open(path, flags, mode)
}
func (realUnixIO) openat(dirfd int, path string, flags int, mode uint32) (int, error) {
	return unix.Openat(dirfd, path, flags, mode)
}
func (realUnixIO) mkdirat(dirfd int, path string, mode uint32) error {
	return unix.Mkdirat(dirfd, path, mode)
}
func (realUnixIO) fstat(fd int, stat *unix.Stat_t) error { return unix.Fstat(fd, stat) }
func (realUnixIO) pread(fd int, p []byte, offset int64) (int, error) {
	return unix.Pread(fd, p, offset)
}
func (realUnixIO) write(fd int, p []byte) (int, error) { return unix.Write(fd, p) }
func (realUnixIO) fsync(fd int) error                  { return unix.Fsync(fd) }
func (realUnixIO) close(fd int) error                  { return unix.Close(fd) }
func (realUnixIO) renameat(olddirfd int, oldpath string, newdirfd int, newpath string) error {
	return unix.Renameat(olddirfd, oldpath, newdirfd, newpath)
}
func (realUnixIO) unlinkat(dirfd int, path string, flags int) error {
	return unix.Unlinkat(dirfd, path, flags)
}
func (realUnixIO) flock(fd int, how int) error  { return unix.Flock(fd, how) }
func (realUnixIO) random(p []byte) (int, error) { return rand.Read(p) }

type unixCache struct {
	mu       sync.RWMutex
	dirfd    int
	path     string
	edition  [32]byte
	counters *Counters
	ops      unixIO
	closed   bool
}

type fileState struct {
	dev   uint64
	ino   uint64
	size  int64
	mode  uint32
	uid   uint32
	nlink uint64
}

func openPlatform(edition string, counters *Counters) (backend, error) {
	base, err := userCacheDir()
	if err != nil {
		return nil, fmt.Errorf("%w: user cache directory: %v", ErrDisabled, err)
	}
	digest := sha256.Sum256([]byte(edition))
	dirfd, path, err := openCacheDirectory(base, hex.EncodeToString(digest[:]), counters, platformIO)
	if err != nil {
		return nil, err
	}
	return &unixCache{dirfd: dirfd, path: path, edition: digest, counters: counters, ops: platformIO}, nil
}

func openCacheDirectory(base, editionHex string, counters *Counters, ops unixIO) (int, string, error) {
	if !filepath.IsAbs(base) || filepath.Clean(base) != base {
		return -1, "", fmt.Errorf("%w: cache base must be a clean absolute path", ErrUnsafePath)
	}
	rootfd, err := ops.open(string(filepath.Separator), secureDirectoryFlags, 0)
	counters.rootOpenOps.Add(1)
	if err != nil {
		return -1, "", fmt.Errorf("%w: open filesystem root: %v", ErrUnsafePath, err)
	}
	current := rootfd
	closeCurrent := func() {
		if current >= 0 {
			_ = ops.close(current)
			counters.closeOps.Add(1)
			current = -1
		}
	}
	if err := validateAncestryDirectory(current, counters, ops); err != nil {
		closeCurrent()
		return -1, "", err
	}

	baseParts := strings.Split(strings.TrimPrefix(base, string(filepath.Separator)), string(filepath.Separator))
	if len(baseParts) == 1 && baseParts[0] == "" {
		baseParts = nil
	}
	for _, part := range baseParts {
		if part == "" || part == "." || part == ".." {
			closeCurrent()
			return -1, "", fmt.Errorf("%w: invalid cache base component", ErrUnsafePath)
		}
		next, openErr := ops.openat(current, part, secureDirectoryFlags, 0)
		counters.rootOpenOps.Add(1)
		if errors.Is(openErr, unix.ENOENT) {
			// os.UserCacheDir returns a location, not an existing directory.
			// Bootstrap it only below an already authenticated user-owned
			// parent; never create missing system ancestry on the user's behalf.
			parent, statErr := statFD(current, counters, ops)
			if statErr != nil || parent.uid != uint32(unix.Geteuid()) {
				closeCurrent()
				return -1, "", fmt.Errorf("%w: missing cache ancestry requires a user-owned parent", ErrUnsafePath)
			}
			counters.mkdirOps.Add(1)
			if mkdirErr := ops.mkdirat(current, part, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				closeCurrent()
				return -1, "", fmt.Errorf("%w: create cache ancestry: %v", ErrUnsafePath, mkdirErr)
			}
			next, openErr = ops.openat(current, part, secureDirectoryFlags, 0)
			counters.rootOpenOps.Add(1)
		}
		if openErr != nil {
			closeCurrent()
			return -1, "", fmt.Errorf("%w: open cache ancestry: %v", ErrUnsafePath, openErr)
		}
		if err := validateAncestryDirectory(next, counters, ops); err != nil {
			_ = ops.close(next)
			counters.closeOps.Add(1)
			closeCurrent()
			return -1, "", err
		}
		_ = ops.close(current)
		counters.closeOps.Add(1)
		current = next
	}

	for _, part := range []string{"dws", "schema", editionHex, "v1"} {
		next, openErr := ops.openat(current, part, secureDirectoryFlags, 0)
		counters.rootOpenOps.Add(1)
		if errors.Is(openErr, unix.ENOENT) {
			counters.mkdirOps.Add(1)
			if mkdirErr := ops.mkdirat(current, part, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				closeCurrent()
				return -1, "", fmt.Errorf("%w: create cache directory: %v", ErrUnsafePath, mkdirErr)
			}
			next, openErr = ops.openat(current, part, secureDirectoryFlags, 0)
			counters.rootOpenOps.Add(1)
		}
		if openErr != nil {
			closeCurrent()
			return -1, "", fmt.Errorf("%w: open cache directory: %v", ErrUnsafePath, openErr)
		}
		if err := validateOwnedDirectory(next, counters, ops); err != nil {
			_ = ops.close(next)
			counters.closeOps.Add(1)
			closeCurrent()
			return -1, "", err
		}
		_ = ops.close(current)
		counters.closeOps.Add(1)
		current = next
	}
	return current, filepath.Join(base, "dws", "schema", editionHex, "v1"), nil
}

func validateAncestryDirectory(fd int, counters *Counters, ops unixIO) error {
	state, err := statFD(fd, counters, ops)
	if err != nil {
		return err
	}
	uid := uint32(unix.Geteuid())
	if state.mode&unix.S_IFMT != unix.S_IFDIR || (state.uid != 0 && state.uid != uid) || state.mode&0o022 != 0 {
		return fmt.Errorf("%w: unsafe cache ancestry ownership or mode", ErrUnsafePath)
	}
	return nil
}

func validateOwnedDirectory(fd int, counters *Counters, ops unixIO) error {
	state, err := statFD(fd, counters, ops)
	if err != nil {
		return err
	}
	if state.mode&unix.S_IFMT != unix.S_IFDIR || state.uid != uint32(unix.Geteuid()) || state.mode&0o7777 != 0o700 {
		return fmt.Errorf("%w: cache directory must be owned by the current user with mode 0700", ErrUnsafePath)
	}
	return nil
}

func statFD(fd int, counters *Counters, ops unixIO) (fileState, error) {
	var stat unix.Stat_t
	counters.statOps.Add(1)
	if err := ops.fstat(fd, &stat); err != nil {
		return fileState{}, fmt.Errorf("%w: fstat: %v", ErrInvalidArtifact, err)
	}
	return fileState{
		dev:   uint64(stat.Dev),
		ino:   uint64(stat.Ino),
		size:  stat.Size,
		mode:  uint32(stat.Mode),
		uid:   stat.Uid,
		nlink: uint64(stat.Nlink),
	}, nil
}

func validateCacheFile(state fileState) error {
	if state.mode&unix.S_IFMT != unix.S_IFREG || state.uid != uint32(unix.Geteuid()) ||
		state.mode&0o7777 != 0o600 || state.nlink != 1 {
		return fmt.Errorf("%w: cache file must be a single-link regular file owned by the current user with mode 0600", ErrUnsafePath)
	}
	return nil
}

func sameFileState(a, b fileState) bool {
	return a.dev == b.dev && a.ino == b.ino && a.size == b.size && a.mode == b.mode &&
		a.uid == b.uid && a.nlink == b.nlink
}

func (c *unixCache) directory() string { return c.path }

func (c *unixCache) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	c.counters.closeOps.Add(1)
	return c.ops.close(c.dirfd)
}

func (c *unixCache) secureOpen(name string, flags int, mode uint32) (int, fileState, error) {
	fd, err := c.ops.openat(c.dirfd, name, flags, mode)
	c.counters.fileOpenOps.Add(1)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return -1, fileState{}, fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return -1, fileState{}, fmt.Errorf("%w: open %s: %v", ErrUnsafePath, name, err)
	}
	state, err := statFD(fd, c.counters, c.ops)
	if err == nil {
		err = validateCacheFile(state)
	}
	if err != nil {
		_ = c.ops.close(fd)
		c.counters.closeOps.Add(1)
		return -1, fileState{}, err
	}
	return fd, state, nil
}

func (c *unixCache) readMeta(identity ExpectedIdentity, expected ArtifactExpectation) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return nil, ErrClosed
	}
	if !digestEqual(c.edition, identity.EditionSHA256) {
		return nil, ErrIdentityMismatch
	}
	fd, initial, err := c.secureOpen(metaFileName, secureReadFlags, 0)
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
	final, err := statFD(fd, c.counters, c.ops)
	if err != nil {
		return nil, closeWith(err)
	}
	if !sameFileState(initial, final) {
		return nil, closeWith(fmt.Errorf("%w: Meta file changed during read", ErrInvalidArtifact))
	}
	if err := closeWith(nil); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *unixCache) openRegistry(identity ExpectedIdentity, expected ArtifactExpectation) (registryBackend, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return nil, ErrClosed
	}
	if !digestEqual(c.edition, identity.EditionSHA256) {
		return nil, ErrIdentityMismatch
	}
	fd, initial, err := c.secureOpen(registryFileName, secureReadFlags, 0)
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
		return fail(fmt.Errorf("%w: Registry file size mismatch", ErrInvalidArtifact))
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
	final, err := statFD(fd, c.counters, c.ops)
	if err != nil {
		return fail(err)
	}
	if !sameFileState(initial, final) {
		return fail(fmt.Errorf("%w: Registry file changed during open", ErrInvalidArtifact))
	}
	return &unixRegistry{fd: fd, initial: initial, expected: expected, counters: c.counters, ops: c.ops}, nil
}

type readCategory uint8

const (
	readHeader readCategory = iota
	readMetaPayload
	readRegistryPayload
)

func (c *unixCache) preadFull(fd int, p []byte, offset int64, category readCategory) error {
	for len(p) > 0 {
		switch category {
		case readHeader:
			c.counters.headerReadOps.Add(1)
		case readMetaPayload:
			c.counters.metaPayloadReadOps.Add(1)
		case readRegistryPayload:
			c.counters.registryReadOps.Add(1)
		}
		n, err := c.ops.pread(fd, p, offset)
		if n > 0 {
			if category == readMetaPayload {
				c.counters.metaPayloadReadBytes.Add(uint64(n))
			} else if category == readRegistryPayload {
				c.counters.registryReadBytes.Add(uint64(n))
			}
			p = p[n:]
			offset += int64(n)
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%w: pread: %v", ErrInvalidArtifact, err)
		}
		if n == 0 {
			return fmt.Errorf("%w: short pread: %v", ErrInvalidArtifact, io.ErrUnexpectedEOF)
		}
	}
	return nil
}

type unixRegistry struct {
	mu       sync.RWMutex
	fd       int
	initial  fileState
	expected ArtifactExpectation
	counters *Counters
	ops      unixIO
	closed   bool
}

func (r *unixRegistry) close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	r.counters.closeOps.Add(1)
	return r.ops.close(r.fd)
}

func (r *unixRegistry) readRange(descriptor RangeDescriptor) ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, ErrClosed
	}
	if descriptor.Offset > r.expected.EncodedLength || descriptor.Length > r.expected.EncodedLength-descriptor.Offset {
		return nil, fmt.Errorf("%w: product range is outside Registry payload", ErrInvalidArtifact)
	}
	before, err := statFD(r.fd, r.counters, r.ops)
	if err != nil {
		return nil, err
	}
	if !sameFileState(r.initial, before) {
		return nil, fmt.Errorf("%w: Registry file changed before range read", ErrInvalidArtifact)
	}
	payload := make([]byte, int(descriptor.Length))
	reader := unixCache{counters: r.counters, ops: r.ops}
	if err := reader.preadFull(r.fd, payload, int64(HeaderSize+descriptor.Offset), readRegistryPayload); err != nil {
		return nil, err
	}
	if digest := sha256.Sum256(payload); !digestEqual(digest, descriptor.SHA256) {
		return nil, fmt.Errorf("%w: product range digest mismatch", ErrIdentityMismatch)
	}
	after, err := statFD(r.fd, r.counters, r.ops)
	if err != nil {
		return nil, err
	}
	if !sameFileState(r.initial, after) {
		return nil, fmt.Errorf("%w: Registry file changed during range read", ErrInvalidArtifact)
	}
	return payload, nil
}

func (r *unixRegistry) validateAggregate() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return ErrClosed
	}
	before, err := statFD(r.fd, r.counters, r.ops)
	if err != nil {
		return err
	}
	if !sameFileState(r.initial, before) {
		return fmt.Errorf("%w: Registry file changed before aggregate read", ErrInvalidArtifact)
	}
	hash := sha256.New()
	buffer := make([]byte, aggregateBufferSize)
	reader := unixCache{counters: r.counters, ops: r.ops}
	var offset uint64
	for offset < r.expected.EncodedLength {
		length := r.expected.EncodedLength - offset
		if length > uint64(len(buffer)) {
			length = uint64(len(buffer))
		}
		chunk := buffer[:int(length)]
		if err := reader.preadFull(r.fd, chunk, int64(HeaderSize+offset), readRegistryPayload); err != nil {
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
	after, err := statFD(r.fd, r.counters, r.ops)
	if err != nil {
		return err
	}
	if !sameFileState(r.initial, after) {
		return fmt.Errorf("%w: Registry file changed during aggregate read", ErrInvalidArtifact)
	}
	return nil
}

func (c *unixCache) writeArtifact(identity ExpectedIdentity, artifact Artifact) error {
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
	header, err := envelope.MarshalBinary()
	if err != nil {
		return err
	}
	target := metaFileName
	if artifact.Expectation.Kind == KindRegistry {
		target = registryFileName
	}
	return c.atomicReplace(target, header, artifact.Payload)
}

func (c *unixCache) atomicReplace(target string, header, payload []byte) error {
	var randomBytes [16]byte
	var fd = -1
	var staging string
	for attempt := 0; attempt < stagingAttempts; attempt++ {
		if _, err := io.ReadFull(randomReader{c.ops}, randomBytes[:]); err != nil {
			return fmt.Errorf("create staging name: %w", err)
		}
		staging = "." + target + "." + hex.EncodeToString(randomBytes[:]) + ".tmp"
		var err error
		fd, err = c.ops.openat(c.dirfd, staging, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0o600)
		c.counters.fileOpenOps.Add(1)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EEXIST) {
			return fmt.Errorf("create staging file: %w", err)
		}
	}
	if fd < 0 {
		return fmt.Errorf("create staging file: exhausted %d attempts", stagingAttempts)
	}
	staged := true
	cleanup := func() {
		if fd >= 0 {
			_ = c.ops.close(fd)
			c.counters.closeOps.Add(1)
			fd = -1
		}
		if staged {
			_ = c.ops.unlinkat(c.dirfd, staging, 0)
			c.counters.removeOps.Add(1)
		}
	}
	state, err := statFD(fd, c.counters, c.ops)
	if err == nil {
		err = validateCacheFile(state)
	}
	if err != nil {
		cleanup()
		return err
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
	if err := c.ops.fsync(fd); err != nil {
		cleanup()
		return fmt.Errorf("sync staging file: %w", err)
	}
	c.counters.closeOps.Add(1)
	if err := c.ops.close(fd); err != nil {
		fd = -1
		cleanup()
		return fmt.Errorf("close staging file: %w", err)
	}
	fd = -1
	c.counters.renameOps.Add(1)
	if err := c.ops.renameat(c.dirfd, staging, c.dirfd, target); err != nil {
		cleanup()
		return fmt.Errorf("replace %s: %w", target, err)
	}
	staged = false
	c.counters.directorySyncOps.Add(1)
	if err := c.ops.fsync(c.dirfd); err != nil {
		return fmt.Errorf("sync cache directory: %w", err)
	}
	return nil
}

type randomReader struct{ ops unixIO }

func (r randomReader) Read(p []byte) (int, error) { return r.ops.random(p) }

func (c *unixCache) writeFull(fd int, p []byte) error {
	for len(p) > 0 {
		c.counters.writeOps.Add(1)
		n, err := c.ops.write(fd, p)
		if n > 0 {
			c.counters.writeBytes.Add(uint64(n))
			p = p[n:]
		}
		if errors.Is(err, unix.EINTR) {
			continue
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

func (c *unixCache) acquire(ctx context.Context, timeout time.Duration) (lockBackend, error) {
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
	fd, _, err := c.secureOpen(lockFileName, secureLockFlags, 0o600)
	c.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	closeFD := func() {
		_ = c.ops.close(fd)
		c.counters.closeOps.Add(1)
	}
	for {
		c.counters.lockAttempts.Add(1)
		err = c.ops.flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			releaseLocal = false
			return &unixLock{fd: fd, local: local, counters: c.counters, ops: c.ops}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
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

type unixLock struct {
	fd       int
	local    *localLock
	counters *Counters
	ops      unixIO
}

func (l *unixLock) release() error {
	unlockErr := l.ops.flock(l.fd, unix.LOCK_UN)
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
