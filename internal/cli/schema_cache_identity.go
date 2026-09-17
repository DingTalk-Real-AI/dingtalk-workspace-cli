// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/buildversion"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemacachepb"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemareader"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
)

var (
	marshalSchemaCacheFileDescriptor = marshalSchemaCacheFileDescriptorDefault
	readSchemaCacheBuildInfo         = debug.ReadBuildInfo
	schemaCacheBinaryDigest          = buildversion.Digest
)

func marshalSchemaCacheFileDescriptorDefault() ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(protodesc.ToFileDescriptorProto(schemacachepb.File_schema_cache_proto))
}

// IdentityFromArtifacts derives the authenticated Schema cache identity of one
// live declaration assembly. Production never embeds this at compile time;
// each machine generates it from the running binary's declarations.
func IdentityFromArtifacts(edition string, artifacts SchemaCacheArtifacts) (SchemaCacheIdentity, error) {
	edition = strings.TrimSpace(edition)
	if _, err := schemacache.EditionSHA256(edition); err != nil {
		return SchemaCacheIdentity{}, err
	}
	source, err := exactSchemaHash(artifacts.SourceHash)
	if err != nil {
		return SchemaCacheIdentity{}, fmt.Errorf("source hash: %w", err)
	}
	surface, err := exactSchemaHash(artifacts.SurfaceHash)
	if err != nil {
		return SchemaCacheIdentity{}, fmt.Errorf("surface hash: %w", err)
	}
	indexLength, indexDigest, err := artifacts.PayloadIndexPins()
	if err != nil {
		return SchemaCacheIdentity{}, fmt.Errorf("payload index pins: %w", err)
	}
	descriptorBytes, err := marshalSchemaCacheFileDescriptor()
	if err != nil {
		return SchemaCacheIdentity{}, fmt.Errorf("marshal Schema cache descriptor: %w", err)
	}
	buildID := localSchemaCacheBuildID(localSchemaCacheBuildIDInput{
		Edition:                edition,
		EnvelopeVersion:        schemacache.EnvelopeVersion,
		DTOFormatVersion:       schemacache.DTOFormatVersion,
		SchemaCacheDTOVersion:  uint32(schemaruntime.SchemaCacheDTOVersion),
		CatalogSnapshotVersion: uint32(artifacts.Version),
		Serializer:             schemacache.SerializerProtobuf,
		Codec:                  schemacache.CodecRaw,
		SourceSHA256:           source,
		SurfaceSHA256:          surface,
		MetaLength:             uint64(len(artifacts.Meta)),
		MetaSHA256:             artifacts.MetaSHA256,
		RegistryLength:         uint64(len(artifacts.Registry)),
		RegistrySHA256:         artifacts.RegistrySHA256,
		PayloadLength:          uint64(len(artifacts.Payload)),
		PayloadSHA256:          artifacts.PayloadSHA256,
		PayloadIndexLength:     indexLength,
		PayloadIndexSHA256:     indexDigest,
		ProductCount:           uint64(artifacts.ProductCount),
		GoRuntimeVersion:       runtime.Version(),
		DescriptorSHA256:       sha256.Sum256(descriptorBytes),
		ProtobufRuntimeVersion: protobufModuleVersion(),
		BinaryDigest:           schemaCacheBinaryDigest(),
	})
	identity := SchemaCacheIdentity{
		Edition:                edition,
		CatalogSnapshotVersion: uint32(artifacts.Version),
		SourceSHA256:           source,
		SurfaceSHA256:          surface,
		BuildID:                buildID,
		Meta:                   artifacts.MetaArtifact().Expectation,
		Registry:               artifacts.RegistryArtifact().Expectation,
		Payload:                artifacts.PayloadArtifact().Expectation,
		PayloadIndexLength:     indexLength,
		PayloadIndexSHA256:     indexDigest,
	}
	return identity, identity.Validate()
}

type localSchemaCacheBuildIDInput struct {
	Edition                                 string
	EnvelopeVersion                         uint16
	DTOFormatVersion, SchemaCacheDTOVersion uint32
	CatalogSnapshotVersion                  uint32
	Serializer, Codec                       uint8
	SourceSHA256, SurfaceSHA256             [sha256.Size]byte
	MetaLength                              uint64
	MetaSHA256                              [sha256.Size]byte
	RegistryLength                          uint64
	RegistrySHA256                          [sha256.Size]byte
	PayloadLength                           uint64
	PayloadSHA256                           [sha256.Size]byte
	PayloadIndexLength                      uint64
	PayloadIndexSHA256                      [sha256.Size]byte
	ProductCount                            uint64
	GoRuntimeVersion                        string
	DescriptorSHA256                        [sha256.Size]byte
	ProtobufRuntimeVersion                  string
	BinaryDigest                            [sha256.Size]byte
}

func localSchemaCacheBuildID(input localSchemaCacheBuildIDInput) [sha256.Size]byte {
	var canonical bytes.Buffer
	canonical.WriteString("dws-schema-cache-local-build-id-v1\x00")
	field := func(tag uint16, value []byte) {
		_ = binary.Write(&canonical, binary.BigEndian, tag)
		_ = binary.Write(&canonical, binary.BigEndian, uint64(len(value)))
		canonical.Write(value)
	}
	uintField := func(tag uint16, value uint64) {
		var encoded [8]byte
		binary.BigEndian.PutUint64(encoded[:], value)
		field(tag, encoded[:])
	}
	field(1, []byte(input.Edition))
	uintField(2, uint64(input.EnvelopeVersion))
	uintField(3, uint64(input.DTOFormatVersion))
	uintField(4, uint64(input.SchemaCacheDTOVersion))
	uintField(5, uint64(input.CatalogSnapshotVersion))
	uintField(6, uint64(input.Serializer))
	uintField(7, uint64(input.Codec))
	field(8, input.SourceSHA256[:])
	field(9, input.SurfaceSHA256[:])
	uintField(10, input.MetaLength)
	field(11, input.MetaSHA256[:])
	uintField(12, input.RegistryLength)
	field(13, input.RegistrySHA256[:])
	uintField(14, input.ProductCount)
	field(15, []byte(input.GoRuntimeVersion))
	field(16, input.DescriptorSHA256[:])
	field(17, []byte(input.ProtobufRuntimeVersion))
	uintField(18, input.PayloadLength)
	field(19, input.PayloadSHA256[:])
	uintField(20, input.PayloadIndexLength)
	field(21, input.PayloadIndexSHA256[:])
	field(22, input.BinaryDigest[:])
	return sha256.Sum256(canonical.Bytes())
}

func protobufModuleVersion() string {
	if info, ok := readSchemaCacheBuildInfo(); ok {
		for _, dependency := range info.Deps {
			if dependency.Path == "google.golang.org/protobuf" {
				return dependency.Version
			}
		}
	}
	return "unknown"
}

func exactSchemaHash(value string) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return digest, fmt.Errorf("invalid Schema hash %q", value)
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	if err != nil {
		return digest, err
	}
	copy(digest[:], decoded)
	return digest, nil
}

func schemaCacheIdentityReady(identity SchemaCacheIdentity) bool {
	return identity.Validate() == nil
}

func schemaCacheIdentityAbsent(identity SchemaCacheIdentity) bool {
	if schemaCacheIdentityReady(identity) {
		return false
	}
	var zero [sha256.Size]byte
	return identity.SourceSHA256 == zero && identity.SurfaceSHA256 == zero && identity.BuildID == zero
}

func identityToRaw(identity SchemaCacheIdentity) schemareader.RawIdentity {
	return schemareader.RawIdentity{
		Edition:            identity.Edition,
		SourceSHA256:       hex.EncodeToString(identity.SourceSHA256[:]),
		SurfaceSHA256:      hex.EncodeToString(identity.SurfaceSHA256[:]),
		BuildID:            hex.EncodeToString(identity.BuildID[:]),
		MetaLength:         fmt.Sprintf("%d", identity.Meta.EncodedLength),
		MetaSHA256:         hex.EncodeToString(identity.Meta.EncodedSHA256[:]),
		RegistryLength:     fmt.Sprintf("%d", identity.Registry.EncodedLength),
		RegistrySHA256:     hex.EncodeToString(identity.Registry.EncodedSHA256[:]),
		PayloadLength:      fmt.Sprintf("%d", identity.Payload.EncodedLength),
		PayloadSHA256:      hex.EncodeToString(identity.Payload.EncodedSHA256[:]),
		PayloadIndexLength: fmt.Sprintf("%d", identity.PayloadIndexLength),
		PayloadIndexSHA256: hex.EncodeToString(identity.PayloadIndexSHA256[:]),
	}
}
