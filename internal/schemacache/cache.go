package schemacache

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"sync/atomic"
	"time"
)

const (
	metaFileName     = "meta.cache"
	registryFileName = "registry.shards.cache"
	payloadFileName  = "payloads.shards.cache"
	lockFileName     = "rebuild.lock"
)

var (
	ErrDisabled         = errors.New("schema cache disabled on this platform")
	ErrNotFound         = errors.New("schema cache artifact not found")
	ErrInvalidArtifact  = errors.New("invalid schema cache artifact")
	ErrIdentityMismatch = errors.New("schema cache identity mismatch")
	ErrUnsafePath       = errors.New("unsafe schema cache path")
	ErrLockTimeout      = errors.New("schema cache lock timeout")
	ErrClosed           = errors.New("schema cache handle closed")
)

// PersistentBackendEnabled is the reviewed v1 disk-cache surface: darwin,
// linux, and windows on amd64/arm64. Each machine generates identity locally,
// so the backend is not limited to the old compile-time proof pair. Other
// targets stay live-only.
func PersistentBackendEnabled(goos, goarch string) bool {
	return (goos == "darwin" || goos == "linux" || goos == "windows") && (goarch == "amd64" || goarch == "arm64")
}

var editionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

type backend interface {
	close() error
	directory() string
	readMeta(ExpectedIdentity, ArtifactExpectation) ([]byte, error)
	openRegistry(ExpectedIdentity, ArtifactExpectation) (registryBackend, error)
	openPayloads(ExpectedIdentity, ArtifactExpectation) (registryBackend, error)
	writeArtifact(ExpectedIdentity, Artifact) error
	acquire(context.Context, time.Duration) (lockBackend, error)
}

type registryBackend interface {
	close() error
	readRange(RangeDescriptor) ([]byte, error)
	validateAggregate() error
}

type lockBackend interface{ release() error }

type openOptions struct {
	counters *Counters
	noCreate bool
}

type Option func(*openOptions)

func WithCounters(c *Counters) Option {
	return func(o *openOptions) { o.counters = c }
}

// WithNoCreate turns a missing cache ancestry into ErrNotFound instead of
// creating it. Speculative readers must never mutate the filesystem.
func WithNoCreate() Option {
	return func(o *openOptions) { o.noCreate = true }
}

// Cache is a securely opened edition-specific cache directory.
type Cache struct{ backend backend }

// Artifact pairs a trusted expectation with the exact payload to publish.
type Artifact struct {
	Expectation ArtifactExpectation
	Payload     []byte
}

func Open(edition string, options ...Option) (*Cache, error) {
	if !editionPattern.MatchString(edition) {
		return nil, fmt.Errorf("%w: invalid edition", ErrUnsafePath)
	}
	var opts openOptions
	for _, option := range options {
		if option != nil {
			option(&opts)
		}
	}
	if opts.counters == nil {
		opts.counters = &Counters{}
	}
	b, err := openPlatformImpl(edition, opts.counters, opts.noCreate)
	if err != nil {
		return nil, err
	}
	return &Cache{backend: b}, nil
}

// openPlatformImpl is the platform Open seam. Production points at openPlatform.
// Tests may swap it (via UseMemoryOpenForTest) so Windows coverage can exercise
// Publish/Read without compiling the unix backend.
var openPlatformImpl = openPlatform

func EditionSHA256(edition string) ([32]byte, error) {
	if !editionPattern.MatchString(edition) {
		return [32]byte{}, fmt.Errorf("%w: invalid edition", ErrUnsafePath)
	}
	return sha256.Sum256([]byte(edition)), nil
}

func (c *Cache) Directory() string {
	if c == nil || c.backend == nil {
		return ""
	}
	return c.backend.directory()
}

func (c *Cache) Close() error {
	if c == nil || c.backend == nil {
		return nil
	}
	return c.backend.close()
}

// ReadMeta authenticates the exact complete Meta payload before returning it.
func (c *Cache) ReadMeta(identity ExpectedIdentity, expected ArtifactExpectation) ([]byte, error) {
	if c == nil || c.backend == nil {
		return nil, ErrClosed
	}
	if err := identity.validate(); err != nil {
		return nil, err
	}
	if err := expected.validate(KindMeta); err != nil {
		return nil, err
	}
	return c.backend.readMeta(identity, expected)
}

// Registry keeps one authenticated regular file open for bounded ReadRange calls.
type Registry struct{ backend registryBackend }

// OpenRegistry authenticates the header and binary-pinned total identity. It
// deliberately does not hash Registry payload bytes on this leaf path.
func (c *Cache) OpenRegistry(identity ExpectedIdentity, expected ArtifactExpectation) (*Registry, error) {
	if c == nil || c.backend == nil {
		return nil, ErrClosed
	}
	if err := identity.validate(); err != nil {
		return nil, err
	}
	if err := expected.validate(KindRegistry); err != nil {
		return nil, err
	}
	b, err := c.backend.openRegistry(identity, expected)
	if err != nil {
		return nil, err
	}
	return &Registry{backend: b}, nil
}

// OpenPayloads opens the command payload file for bounded ReadRange calls. It
// is deliberately independent of the registry so a corrupted registry cannot
// affect payload reads.
func (c *Cache) OpenPayloads(identity ExpectedIdentity, expected ArtifactExpectation) (*Registry, error) {
	if c == nil || c.backend == nil {
		return nil, ErrClosed
	}
	if err := identity.validate(); err != nil {
		return nil, err
	}
	if err := expected.validate(KindPayloads); err != nil {
		return nil, err
	}
	b, err := c.backend.openPayloads(identity, expected)
	if err != nil {
		return nil, err
	}
	return &Registry{backend: b}, nil
}

func (r *Registry) Close() error {
	if r == nil || r.backend == nil {
		return nil
	}
	return r.backend.close()
}

func (r *Registry) ReadRange(descriptor RangeDescriptor) ([]byte, error) {
	if r == nil || r.backend == nil {
		return nil, ErrClosed
	}
	if descriptor.Length == 0 || descriptor.Length > MaxProductRangeSize || isZeroDigest(descriptor.SHA256) {
		return nil, fmt.Errorf("%w: invalid product range descriptor", ErrInvalidArtifact)
	}
	return r.backend.readRange(descriptor)
}

// ValidateAggregate hashes the complete Registry payload for repair or full
// audit. ReadRange does not call it.
func (r *Registry) ValidateAggregate() error {
	if r == nil || r.backend == nil {
		return ErrClosed
	}
	return r.backend.validateAggregate()
}

// WriteArtifact atomically replaces one artifact. Publish should be used when
// committing a Registry and Meta pair.
func (c *Cache) WriteArtifact(identity ExpectedIdentity, artifact Artifact) error {
	if c == nil || c.backend == nil {
		return ErrClosed
	}
	if err := validateArtifactPayload(identity, artifact); err != nil {
		return err
	}
	return c.backend.writeArtifact(identity, artifact)
}

// Publish commits Registry first and Meta last. Meta is the generation commit marker.
func (c *Cache) Publish(identity ExpectedIdentity, registry, meta Artifact, extra ...Artifact) error {
	if c == nil || c.backend == nil {
		return ErrClosed
	}
	if registry.Expectation.Kind != KindRegistry || meta.Expectation.Kind != KindMeta {
		return fmt.Errorf("%w: publish requires Registry then Meta", ErrInvalidArtifact)
	}
	for _, artifact := range extra {
		if artifact.Expectation.Kind != KindPayloads {
			return fmt.Errorf("%w: publish extras must be payloads", ErrInvalidArtifact)
		}
	}
	artifacts := append([]Artifact{registry}, append(extra, meta)...)
	// Validate all before replacing any old artifact.
	for _, artifact := range artifacts {
		if err := validateArtifactPayload(identity, artifact); err != nil {
			return err
		}
	}
	for _, artifact := range artifacts {
		if err := c.backend.writeArtifact(identity, artifact); err != nil {
			return err
		}
	}
	return nil
}

func validateArtifactPayload(identity ExpectedIdentity, artifact Artifact) error {
	if err := identity.validate(); err != nil {
		return err
	}
	if err := artifact.Expectation.validate(artifact.Expectation.Kind); err != nil {
		return err
	}
	if uint64(len(artifact.Payload)) != artifact.Expectation.EncodedLength {
		return fmt.Errorf("%w: payload length differs from trusted expectation", ErrIdentityMismatch)
	}
	digest := sha256.Sum256(artifact.Payload)
	if !digestEqual(digest, artifact.Expectation.EncodedSHA256) {
		return fmt.Errorf("%w: payload digest differs from trusted expectation", ErrIdentityMismatch)
	}
	return nil
}

// Lock is a process-local and cross-process rebuild lock.
type Lock struct {
	backend  lockBackend
	released atomic.Bool
}

func (c *Cache) AcquireLock(ctx context.Context, timeout time.Duration) (*Lock, error) {
	if c == nil || c.backend == nil {
		return nil, ErrClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	b, err := c.backend.acquire(ctx, timeout)
	if err != nil {
		return nil, err
	}
	return &Lock{backend: b}, nil
}

func (l *Lock) Release() error {
	if l == nil || l.backend == nil || !l.released.CompareAndSwap(false, true) {
		return nil
	}
	return l.backend.release()
}
