// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemacache

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestCrossPlatformCoverageUnixAncestryAndNamedMkdirFaults(t *testing.T) {
	base := privateTestBase(t)
	missing := filepath.Join(base, "missing-cache-root")
	wrongOwner := &ancestryFlagIO{part: "missing-cache-root", wrongUID: true}
	oldUser, oldIO := userCacheDir, platformIO
	userCacheDir = func() (string, error) { return missing, nil }
	platformIO = wrongOwner
	t.Cleanup(func() { userCacheDir, platformIO = oldUser, oldIO })
	if _, err := Open("official"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("wrong-owner ancestry = %v", err)
	}

	statFail := &ancestryFlagIO{part: "missing-cache-root", failStat: true}
	userCacheDir = func() (string, error) { return filepath.Join(privateTestBase(t), "missing-cache-root"), nil }
	platformIO = statFail
	if _, err := Open("official"); err == nil {
		t.Fatal("ancestry parent fstat failure accepted")
	}

	if _, _, err := openTestCacheMaybe(t, failNamedMkdirIO{name: "dws"}); err == nil {
		t.Fatal("named dws mkdir failure accepted")
	}
	ancestryBase := privateTestBase(t)
	missingPart := filepath.Join(ancestryBase, "missing-part")
	userCacheDir = func() (string, error) { return missingPart, nil }
	platformIO = &mkdirThenOpenFailIO{target: "missing-part"}
	if _, err := Open("official"); err == nil {
		t.Fatal("ancestry mkdir then open failure accepted")
	}
	userCacheDir, platformIO = oldUser, oldIO
}

func TestCrossPlatformCoverageUnixReadWriteLockRemainingFaults(t *testing.T) {
	cache, _, identity := openTestCache(t, nil)
	meta := testArtifact(KindMeta, []byte("meta-remaining"))
	reg := testArtifact(KindRegistry, []byte("registry-remaining-bytes"))
	if err := cache.Publish(identity, reg, meta); err != nil {
		t.Fatal(err)
	}

	uc := cache.backend.(*unixCache)
	uc.ops = &countingFstatIO{failAt: 2, err: errors.New("final fstat")}
	if _, err := cache.ReadMeta(identity, meta.Expectation); err == nil {
		t.Fatal("ReadMeta final fstat failure accepted")
	}
	uc.ops = &countingFstatIO{failAt: 2, mutate: true}
	if _, err := cache.ReadMeta(identity, meta.Expectation); err == nil {
		t.Fatal("ReadMeta file-changed accepted")
	}

	uc.ops = realUnixIO{}
	if err := os.Remove(filepath.Join(cache.Directory(), registryFileName)); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.OpenRegistry(identity, reg.Expectation); err == nil {
		t.Fatal("missing registry open accepted")
	}

	cache2, _, identity2 := openTestCache(t, nil)
	meta2 := testArtifact(KindMeta, []byte("meta-remaining-2"))
	reg2 := testArtifact(KindRegistry, []byte("registry-remaining-2"))
	if err := cache2.Publish(identity2, reg2, meta2); err != nil {
		t.Fatal(err)
	}
	uc2 := cache2.backend.(*unixCache)
	uc2.ops = failPreadIO{err: unix.EIO}
	if _, err := cache2.OpenRegistry(identity2, reg2.Expectation); err == nil {
		t.Fatal("registry header pread failure accepted")
	}

	body, err := os.ReadFile(filepath.Join(cache2.Directory(), registryFileName))
	if err != nil {
		t.Fatal(err)
	}
	body[0] ^= 0xff
	if err := os.WriteFile(filepath.Join(cache2.Directory(), registryFileName), body, 0o600); err != nil {
		t.Fatal(err)
	}
	uc2.ops = realUnixIO{}
	if _, err := cache2.OpenRegistry(identity2, reg2.Expectation); err == nil {
		t.Fatal("corrupt registry envelope accepted")
	}

	cache3, _, identity3 := openTestCache(t, nil)
	meta3 := testArtifact(KindMeta, []byte("meta-remaining-3"))
	reg3 := testArtifact(KindRegistry, []byte("registry-remaining-3"))
	if err := cache3.Publish(identity3, reg3, meta3); err != nil {
		t.Fatal(err)
	}
	wrongSHA := reg3.Expectation
	wrongSHA.EncodedSHA256 = sha256.Sum256([]byte("not-the-registry"))
	if _, err := cache3.OpenRegistry(identity3, wrongSHA); err == nil {
		t.Fatal("registry authenticate mismatch accepted")
	}
	uc3 := cache3.backend.(*unixCache)
	uc3.ops = &countingFstatIO{failAt: 2, err: errors.New("open shard fstat")}
	if _, err := cache3.OpenRegistry(identity3, reg3.Expectation); err == nil {
		t.Fatal("openShardFile fstat failure accepted")
	}
	uc3.ops = &countingFstatIO{failAt: 2, mutate: true}
	if _, err := cache3.OpenRegistry(identity3, reg3.Expectation); err == nil {
		t.Fatal("openShardFile file-changed accepted")
	}

	uc3.ops = &eintrThenRealIO{remaining: 3}
	if _, err := cache3.ReadMeta(identity3, meta3.Expectation); err != nil {
		t.Fatalf("EINTR ReadMeta: %v", err)
	}

	opened, err := cache3.OpenRegistry(identity3, reg3.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	ur := opened.backend.(*unixRegistry)
	ur.ops = failPreadIO{err: unix.EIO}
	if _, err := opened.ReadRange(RangeDescriptor{Offset: 0, Length: 1, SHA256: sha256.Sum256([]byte("x"))}); err == nil {
		t.Fatal("ReadRange pread failure accepted")
	}
	digest := sha256.Sum256(reg3.Payload[:1])
	ur.ops = &countingFstatIO{failAt: 2, err: errors.New("range fstat")}
	if _, err := opened.ReadRange(RangeDescriptor{Offset: 0, Length: 1, SHA256: digest}); err == nil {
		t.Fatal("ReadRange final fstat failure accepted")
	}
	ur.ops = &countingFstatIO{failAt: 2, mutate: true}
	if _, err := opened.ReadRange(RangeDescriptor{Offset: 0, Length: 1, SHA256: digest}); err == nil {
		t.Fatal("ReadRange file-changed accepted")
	}
	wrongRange := digest
	wrongRange[0] ^= 0xff
	ur.ops = realUnixIO{}
	if _, err := opened.ReadRange(RangeDescriptor{Offset: 0, Length: 1, SHA256: wrongRange}); err == nil {
		t.Fatal("ReadRange digest mismatch accepted")
	}

	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	if err := opened.ValidateAggregate(); err == nil {
		t.Fatal("closed ValidateAggregate accepted")
	}

	opened2, err := cache3.OpenRegistry(identity3, reg3.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = opened2.Close() })
	ur2 := opened2.backend.(*unixRegistry)
	ur2.ops = &countingFstatIO{failAt: 1, err: errors.New("aggregate fstat")}
	if err := opened2.ValidateAggregate(); err == nil {
		t.Fatal("ValidateAggregate before fstat failure accepted")
	}
	ur2.ops = &countingFstatIO{failAt: 1, mutate: true}
	if err := opened2.ValidateAggregate(); err == nil {
		t.Fatal("ValidateAggregate before file-changed accepted")
	}
	ur2.ops = &countingFstatIO{failAt: 2, err: errors.New("aggregate after fstat")}
	if err := opened2.ValidateAggregate(); err == nil {
		t.Fatal("ValidateAggregate after fstat failure accepted")
	}
	ur2.ops = &countingFstatIO{failAt: 2, mutate: true}
	if err := opened2.ValidateAggregate(); err == nil {
		t.Fatal("ValidateAggregate after file-changed accepted")
	}

	wrongEdition := identity3
	wrongEdition.EditionSHA256 = sha256.Sum256([]byte("other-edition"))
	if err := uc3.writeArtifact(wrongEdition, meta3); err == nil {
		t.Fatal("edition mismatch writeArtifact accepted")
	}
	zeroVer := identity3
	zeroVer.CatalogSnapshotVersion = 0
	if err := uc3.writeArtifact(zeroVer, meta3); err == nil {
		t.Fatal("zero snapshot writeArtifact accepted")
	}

	uc3.ops = failRandomIO{}
	if err := cache3.WriteArtifact(identity3, testArtifact(KindMeta, []byte("rand"))); err == nil {
		t.Fatal("random staging name failure accepted")
	}
	uc3.ops = &countingFstatIO{failAt: 1, err: errors.New("staging fstat")}
	if err := cache3.WriteArtifact(identity3, testArtifact(KindMeta, []byte("stage-stat"))); err == nil {
		t.Fatal("staging fstat failure accepted")
	}
	uc3.ops = &failNthWriteIO{failAt: 2}
	if err := cache3.WriteArtifact(identity3, testArtifact(KindMeta, []byte("payload-write"))); err == nil {
		t.Fatal("payload write failure accepted")
	}
	uc3.ops = failLockOpenIO{}
	if _, err := cache3.AcquireLock(context.Background(), time.Millisecond); err == nil {
		t.Fatal("lock open failure accepted")
	}

	if err := validateArtifactPayload(identity3, Artifact{
		Payload:     []byte("x"),
		Expectation: ArtifactExpectation{Kind: KindMeta, Serializer: SerializerProtobuf, Codec: CodecRaw, FormatVersion: DTOFormatVersion, EncodedLength: 1, DecodedLength: 1, EncodedSHA256: sha256.Sum256([]byte("y"))},
	}); err == nil {
		t.Fatal("digest mismatch validateArtifactPayload accepted")
	}
}

type ancestryFlagIO struct {
	realUnixIO
	part     string
	wrongUID bool
	failStat bool
	flag     atomic.Bool
}

func (a *ancestryFlagIO) openat(dirfd int, path string, flags int, mode uint32) (int, error) {
	if path == a.part {
		a.flag.Store(true)
		return -1, unix.ENOENT
	}
	return a.realUnixIO.openat(dirfd, path, flags, mode)
}

func (a *ancestryFlagIO) fstat(fd int, stat *unix.Stat_t) error {
	if a.flag.Load() {
		if a.failStat {
			return errors.New("parent fstat")
		}
		if err := a.realUnixIO.fstat(fd, stat); err != nil {
			return err
		}
		if a.wrongUID {
			stat.Uid = uint32(unix.Geteuid()) + 1
		}
		return nil
	}
	return a.realUnixIO.fstat(fd, stat)
}

type failNamedMkdirIO struct {
	realUnixIO
	name string
}

func (f failNamedMkdirIO) mkdirat(dirfd int, path string, mode uint32) error {
	if path == f.name {
		return unix.EPERM
	}
	return f.realUnixIO.mkdirat(dirfd, path, mode)
}

type mkdirThenOpenFailIO struct {
	realUnixIO
	target string
	opens  atomic.Int32
}

func (m *mkdirThenOpenFailIO) openat(dirfd int, path string, flags int, mode uint32) (int, error) {
	if path == m.target {
		if m.opens.Add(1) == 1 {
			return -1, unix.ENOENT
		}
		return -1, unix.EPERM
	}
	return m.realUnixIO.openat(dirfd, path, flags, mode)
}

func (m *mkdirThenOpenFailIO) mkdirat(dirfd int, path string, mode uint32) error {
	if path == m.target {
		return nil
	}
	return m.realUnixIO.mkdirat(dirfd, path, mode)
}

type countingFstatIO struct {
	realUnixIO
	n      atomic.Int32
	failAt int32
	err    error
	mutate bool
}

func (c *countingFstatIO) fstat(fd int, stat *unix.Stat_t) error {
	n := c.n.Add(1)
	if err := c.realUnixIO.fstat(fd, stat); err != nil {
		return err
	}
	if c.failAt > 0 && n >= c.failAt {
		if c.mutate {
			stat.Ino++
			return nil
		}
		if c.err != nil {
			return c.err
		}
		return errors.New("fstat failed")
	}
	return nil
}

type failNthWriteIO struct {
	realUnixIO
	n      atomic.Int32
	failAt int32
}

func (f *failNthWriteIO) write(fd int, p []byte) (int, error) {
	if f.n.Add(1) >= f.failAt {
		return 0, io.ErrShortWrite
	}
	return f.realUnixIO.write(fd, p)
}

type failRandomIO struct{ realUnixIO }

func (failRandomIO) random([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type failLockOpenIO struct{ realUnixIO }

func (f failLockOpenIO) openat(dirfd int, path string, flags int, mode uint32) (int, error) {
	if path == lockFileName {
		return -1, unix.EPERM
	}
	return f.realUnixIO.openat(dirfd, path, flags, mode)
}
