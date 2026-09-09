package schemacache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// UseMemoryOpenForTest swaps Open onto a portable file backend under t.TempDir
// so targets without the unix backend (Windows coverage) can Publish and Read
// the same artifacts. Production must not call this; the ForTest suffix is the
// boundary. The swap is inlined (not testseam) so schemacache stays out of the
// thin Schema dependency closure.
func UseMemoryOpenForTest(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	previous := openPlatformImpl
	openPlatformImpl = func(edition string, counters *Counters, noCreate bool) (backend, error) {
		return openPortableBackend(root, edition, counters, noCreate)
	}
	t.Cleanup(func() { openPlatformImpl = previous })
}

func openPortableBackend(root, edition string, counters *Counters, noCreate bool) (backend, error) {
	digest := sha256.Sum256([]byte(edition))
	dir := filepath.Join(root, hex.EncodeToString(digest[:]), "v1")
	if _, err := os.Stat(dir); err != nil {
		if noCreate {
			return nil, fmt.Errorf("%w: cache directory", ErrNotFound)
		}
		_ = os.MkdirAll(dir, 0o700)
		counters.mkdirOps.Add(1)
	}
	return &portableCache{dir: dir, edition: digest, counters: counters}, nil
}

type portableCache struct {
	mu       sync.RWMutex
	dir      string
	edition  [32]byte
	counters *Counters
	closed   bool
}

func (c *portableCache) directory() string { return c.dir }

func (c *portableCache) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.counters.closeOps.Add(1)
	return nil
}

func (c *portableCache) guard(identity ExpectedIdentity) error {
	if c.closed {
		return ErrClosed
	}
	if !digestEqual(c.edition, identity.EditionSHA256) {
		return ErrIdentityMismatch
	}
	return nil
}

func (c *portableCache) artifactPath(name string) string {
	return filepath.Join(c.dir, name)
}

func (c *portableCache) readFile(name string) ([]byte, error) {
	c.counters.fileOpenOps.Add(1)
	body, err := os.ReadFile(c.artifactPath(name))
	c.counters.closeOps.Add(1)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return body, nil
}

func (c *portableCache) decodeArtifact(identity ExpectedIdentity, expected ArtifactExpectation, name string) ([]byte, error) {
	if err := c.guard(identity); err != nil {
		return nil, err
	}
	body, err := c.readFile(name)
	if err != nil {
		return nil, err
	}
	if len(body) < HeaderSize {
		return nil, fmt.Errorf("%w: short artifact", ErrInvalidArtifact)
	}
	c.counters.headerReadOps.Add(1)
	envelope, err := ParseEnvelope(body[:HeaderSize])
	if err != nil {
		return nil, err
	}
	if err := envelope.authenticate(identity, expected); err != nil {
		return nil, err
	}
	payload := body[HeaderSize:]
	digest := sha256.Sum256(payload)
	if !digestEqual(digest, expected.EncodedSHA256) {
		return nil, fmt.Errorf("%w: payload digest mismatch", ErrIdentityMismatch)
	}
	return payload, nil
}

func (c *portableCache) readMeta(identity ExpectedIdentity, expected ArtifactExpectation) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	payload, err := c.decodeArtifact(identity, expected, metaFileName)
	if err != nil {
		return nil, err
	}
	c.counters.metaPayloadReadOps.Add(1)
	c.counters.metaPayloadReadBytes.Add(uint64(len(payload)))
	return payload, nil
}

func (c *portableCache) openRegistry(identity ExpectedIdentity, expected ArtifactExpectation) (registryBackend, error) {
	return c.openShard(identity, expected, registryFileName, false)
}

func (c *portableCache) openPayloads(identity ExpectedIdentity, expected ArtifactExpectation) (registryBackend, error) {
	return c.openShard(identity, expected, payloadFileName, true)
}

func (c *portableCache) openShard(identity ExpectedIdentity, expected ArtifactExpectation, name string, payloads bool) (registryBackend, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, err := c.decodeArtifact(identity, expected, name); err != nil {
		return nil, err
	}
	return &portableRegistry{cache: c, name: name, expected: expected, payloads: payloads}, nil
}

func (c *portableCache) writeArtifact(identity ExpectedIdentity, artifact Artifact) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if err := c.guard(identity); err != nil {
		return err
	}
	envelope, _ := envelopeFrom(identity, artifact.Expectation)
	header, _ := envelope.MarshalBinary()
	name := metaFileName
	if artifact.Expectation.Kind == KindRegistry {
		name = registryFileName
	} else if artifact.Expectation.Kind == KindPayloads {
		name = payloadFileName
	}
	body := append(append([]byte{}, header...), artifact.Payload...)
	_ = os.WriteFile(c.artifactPath(name), body, 0o600)
	c.counters.writeOps.Add(1)
	c.counters.writeBytes.Add(uint64(len(body)))
	c.counters.fileSyncOps.Add(1)
	c.counters.renameOps.Add(1)
	return nil
}

var portableLocks sync.Map

type portableLockSlot struct{ token chan struct{} }

func portableLockFor(path string) *portableLockSlot {
	created := &portableLockSlot{token: make(chan struct{}, 1)}
	created.token <- struct{}{}
	actual, _ := portableLocks.LoadOrStore(path, created)
	return actual.(*portableLockSlot)
}

func (c *portableCache) acquire(ctx context.Context, timeout time.Duration) (lockBackend, error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, ErrClosed
	}
	c.mu.RUnlock()
	c.counters.lockAttempts.Add(1)
	slot := portableLockFor(c.dir)
	if timeout <= 0 {
		select {
		case <-slot.token:
			return &portableHeldLock{slot: slot}, nil
		default:
			return nil, ErrLockTimeout
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-slot.token:
		return &portableHeldLock{slot: slot}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, ErrLockTimeout
	}
}

type portableHeldLock struct{ slot *portableLockSlot }

func (l *portableHeldLock) release() error {
	l.slot.token <- struct{}{}
	return nil
}

type portableRegistry struct {
	mu       sync.RWMutex
	cache    *portableCache
	name     string
	expected ArtifactExpectation
	payloads bool
	closed   bool
}

func (r *portableRegistry) close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.cache.counters.closeOps.Add(1)
	return nil
}

func (r *portableRegistry) loadPayload() ([]byte, error) {
	if r.closed {
		return nil, ErrClosed
	}
	body, err := r.cache.readFile(r.name)
	if err != nil {
		return nil, err
	}
	if uint64(len(body)) < HeaderSize+r.expected.EncodedLength {
		return nil, fmt.Errorf("%w: short shard", ErrInvalidArtifact)
	}
	return body[HeaderSize : HeaderSize+r.expected.EncodedLength], nil
}

func (r *portableRegistry) readRange(descriptor RangeDescriptor) ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	payload, err := r.loadPayload()
	if err != nil {
		return nil, err
	}
	if descriptor.Offset > uint64(len(payload)) || descriptor.Length > uint64(len(payload))-descriptor.Offset {
		return nil, fmt.Errorf("%w: product range is outside payload", ErrInvalidArtifact)
	}
	chunk := append([]byte{}, payload[descriptor.Offset:descriptor.Offset+descriptor.Length]...)
	if digest := sha256.Sum256(chunk); !digestEqual(digest, descriptor.SHA256) {
		return nil, fmt.Errorf("%w: product range digest mismatch", ErrIdentityMismatch)
	}
	if r.payloads {
		r.cache.counters.payloadReadOps.Add(1)
		r.cache.counters.payloadReadBytes.Add(uint64(len(chunk)))
	} else {
		r.cache.counters.registryReadOps.Add(1)
		r.cache.counters.registryReadBytes.Add(uint64(len(chunk)))
	}
	return chunk, nil
}

func (r *portableRegistry) validateAggregate() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	payload, err := r.loadPayload()
	if err != nil {
		return err
	}
	digest := sha256.Sum256(payload)
	if !digestEqual(digest, r.expected.EncodedSHA256) {
		return fmt.Errorf("%w: Registry aggregate digest mismatch", ErrIdentityMismatch)
	}
	return nil
}
