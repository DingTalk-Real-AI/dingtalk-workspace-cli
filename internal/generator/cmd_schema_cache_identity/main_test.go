// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageDeterministicBuildIDFixedVector(t *testing.T) {
	input := buildIDInput{
		Edition: "open", EnvelopeVersion: 1, DTOFormatVersion: 1, SchemaCacheDTOVersion: 1,
		CatalogSnapshotVersion: 1, Serializer: 2, Codec: 0,
		SourceSHA256: sha256.Sum256([]byte("source")), SurfaceSHA256: sha256.Sum256([]byte("surface")),
		MetaLength: 123, MetaSHA256: sha256.Sum256([]byte("meta")), RegistryLength: 456,
		RegistrySHA256: sha256.Sum256([]byte("registry")), ProductCount: 42, GoRuntimeVersion: "go1.test",
		ProtoSHA256: sha256.Sum256([]byte("proto")), GeneratedPBGoSHA256: sha256.Sum256([]byte("pb.go")),
		DescriptorSHA256: sha256.Sum256([]byte("descriptor")), ProtocVersion: "protoc-test",
		ProtocGenGoVersion: "protoc-gen-go-test", ProtobufRuntimeVersion: "protobuf-test",
		PayloadLength: 789, PayloadSHA256: sha256.Sum256([]byte("payload")),
		PayloadIndexLength: 12, PayloadIndexSHA256: sha256.Sum256([]byte("payload-index")),
	}
	const want = "151de3385e20b04ea2502ab329cdd9b74d3b03fdb5d50b71cc0df6ac11cc9dd2"
	if got := digestHex(deterministicBuildID(input)); got != want {
		t.Fatalf("BuildID = %s, want %s", got, want)
	}
	if second := deterministicBuildID(input); second != deterministicBuildID(input) {
		t.Fatal("BuildID is not deterministic")
	}
}

func TestCrossPlatformCoverageEncodeIdentityProofDeterministic(t *testing.T) {
	proof := identityProof{Version: 1, Edition: "open", SourceSHA256: "aa", SurfaceSHA256: "bb", BuildID: "cc", MetaLength: 1, MetaSHA256: "dd", RegistryLength: 2, RegistrySHA256: "ee",
		PayloadLength: 3, PayloadSHA256: "ff", PayloadIndexLength: 4, PayloadIndexSHA256: "00"}
	first, err := encodeIdentityProof(proof, "json")
	if err != nil {
		t.Fatal(err)
	}
	second, err := encodeIdentityProof(proof, "json")
	if err != nil || string(first) != string(second) {
		t.Fatalf("canonical JSON differs: %v", err)
	}
	if _, err := encodeIdentityProof(proof, "shell"); err != nil {
		t.Fatal(err)
	}
	if _, err := encodeIdentityProof(proof, "yaml"); err == nil {
		t.Fatal("unsupported format succeeded")
	}
}

func TestCrossPlatformCoverageIdentityProofRejectsUnprovenEditionBeforeAssembly(t *testing.T) {
	previous := edition.Get()
	t.Cleanup(func() { edition.Override(previous) })
	for _, hooks := range []*edition.Hooks{
		{Name: "different"},
		{Name: "open", RegisterExtraCommands: func(*cobra.Command, edition.ToolCaller) { t.Fatal("overlay must not run") }},
	} {
		edition.Override(hooks)
		if _, err := generateIdentityProof("missing-source-root", "open"); err == nil {
			t.Fatal("unproven edition was allowed to mint a cache identity")
		}
	}
}

func TestCrossPlatformCoverageWriteAtomicReplacesDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subdir", "proof.json")
	if err := writeAtomic(path, []byte("first\n")); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(path, []byte("second\n")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "second\n" {
		t.Fatalf("writeAtomic contents = %q, err=%v", got, err)
	}
}

func TestCrossPlatformCoverageExactProofDigestAndShellSafety(t *testing.T) {
	sum := sha256.Sum256([]byte("payload"))
	encoded := hex.EncodeToString(sum[:])
	got, err := exactProofDigest("sha256:" + encoded)
	if err != nil || got != sum {
		t.Fatalf("exactProofDigest = %x, %v", got, err)
	}
	if _, err := exactProofDigest("sha256:zz"); err == nil {
		t.Fatal("invalid hex succeeded")
	}
	if _, err := exactProofDigest("not-a-digest"); err == nil {
		t.Fatal("malformed hash succeeded")
	}
	proof := identityProof{Version: 1, Edition: "open space", SourceSHA256: "aa", SurfaceSHA256: "bb", BuildID: "cc", MetaLength: 1, MetaSHA256: "dd", RegistryLength: 2, RegistrySHA256: "ee",
		PayloadLength: 3, PayloadSHA256: "ff", PayloadIndexLength: 4, PayloadIndexSHA256: "00"}
	if _, err := encodeIdentityProof(proof, "shell"); err == nil || !strings.Contains(err.Error(), "SCHEMA_CACHE_EDITION") {
		t.Fatalf("unsafe shell edition error = %v", err)
	}
}

func TestCrossPlatformCoverageIdentityProofComparesPayloadBytes(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "!bytes.Equal(artifacts.Payload, second.Payload)") {
		t.Fatal("identity proof must compare Payload across the two encodes")
	}
}
