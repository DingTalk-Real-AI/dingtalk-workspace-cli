// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemacachepb"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
)

const (
	identityProofVersion = 1
	protocVersion        = "7.35.1"
	protocGenGoVersion   = "v1.33.0"
)

type identityProof struct {
	Version                int    `json:"version"`
	Edition                string `json:"edition"`
	EnvelopeVersion        uint16 `json:"envelope_version"`
	DTOFormatVersion       uint32 `json:"dto_format_version"`
	SchemaCacheDTOVersion  int    `json:"schema_cache_dto_version"`
	CatalogSnapshotVersion int    `json:"catalog_snapshot_version"`
	Serializer             uint8  `json:"serializer"`
	Codec                  uint8  `json:"codec"`
	SourceSHA256           string `json:"source_sha256"`
	SurfaceSHA256          string `json:"surface_sha256"`
	MetaLength             uint64 `json:"meta_length"`
	MetaSHA256             string `json:"meta_sha256"`
	RegistryLength         uint64 `json:"registry_length"`
	RegistrySHA256         string `json:"registry_sha256"`
	ProductCount           int    `json:"product_count"`
	GoRuntimeVersion       string `json:"go_runtime_version"`
	ProtoSHA256            string `json:"proto_sha256"`
	GeneratedPBGoSHA256    string `json:"generated_pb_go_sha256"`
	DescriptorSHA256       string `json:"descriptor_sha256"`
	ProtocVersion          string `json:"protoc_version"`
	ProtocGenGoVersion     string `json:"protoc_gen_go_version"`
	ProtobufRuntimeVersion string `json:"protobuf_runtime_version"`
	BuildID                string `json:"build_id"`
}

type buildIDInput struct {
	Edition                string
	EnvelopeVersion        uint16
	DTOFormatVersion       uint32
	SchemaCacheDTOVersion  uint32
	CatalogSnapshotVersion uint32
	Serializer             uint8
	Codec                  uint8
	SourceSHA256           [sha256.Size]byte
	SurfaceSHA256          [sha256.Size]byte
	MetaLength             uint64
	MetaSHA256             [sha256.Size]byte
	RegistryLength         uint64
	RegistrySHA256         [sha256.Size]byte
	ProductCount           uint64
	GoRuntimeVersion       string
	ProtoSHA256            [sha256.Size]byte
	GeneratedPBGoSHA256    [sha256.Size]byte
	DescriptorSHA256       [sha256.Size]byte
	ProtocVersion          string
	ProtocGenGoVersion     string
	ProtobufRuntimeVersion string
}

func main() {
	var rootPath, editionName, outputPath, format string
	flag.StringVar(&rootPath, "root", ".", "repository root containing the Schema cache protobuf sources")
	flag.StringVar(&editionName, "edition", "open", "release edition identity")
	flag.StringVar(&outputPath, "output", "", "atomic proof output path (default stdout)")
	flag.StringVar(&format, "format", "json", "proof format: json or shell")
	flag.Parse()

	proof, err := generateIdentityProof(rootPath, editionName)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	encoded, err := encodeIdentityProof(proof, format)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if outputPath == "" {
		_, err = os.Stdout.Write(encoded)
	} else {
		err = writeAtomic(outputPath, encoded)
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func generateIdentityProof(rootPath, editionName string) (identityProof, error) {
	editionName = strings.TrimSpace(editionName)
	if _, err := schemacache.EditionSHA256(editionName); err != nil {
		return identityProof{}, err
	}
	hooks := edition.Get()
	if hooks == nil || hooks.Name != editionName || hooks.RegisterExtraCommands != nil {
		return identityProof{}, fmt.Errorf("edition %q is not the compiled, overlay-free Schema authority", editionName)
	}
	// Resolve once. BuildSchemaCacheArtifacts consumes this exact hand-off and
	// runs the authoritative validation/snapshot path without rebuilding Cobra.
	resolved, err := cli.ResolveSchemaBuild(app.NewSchemaSourceRootCommand())
	if err != nil {
		return identityProof{}, fmt.Errorf("resolve authoritative Schema build: %w", err)
	}
	artifacts, err := cli.BuildSchemaCacheArtifacts(resolved)
	if err != nil {
		return identityProof{}, fmt.Errorf("build Schema cache artifacts: %w", err)
	}
	if err := artifacts.ValidateRoundTrip(); err != nil {
		return identityProof{}, fmt.Errorf("validate Schema cache release projections: %w", err)
	}
	second, err := cli.BuildSchemaCacheArtifacts(resolved)
	if err != nil {
		return identityProof{}, fmt.Errorf("repeat Schema cache encoding: %w", err)
	}
	if !bytes.Equal(artifacts.Meta, second.Meta) || !bytes.Equal(artifacts.Registry, second.Registry) {
		return identityProof{}, fmt.Errorf("Schema cache artifact encoding is not deterministic")
	}
	protoBytes, err := os.ReadFile(filepath.Join(rootPath, "internal", "cli", "schemacachepb", "schema_cache.proto"))
	if err != nil {
		return identityProof{}, fmt.Errorf("read schema_cache.proto: %w", err)
	}
	pbGoBytes, err := os.ReadFile(filepath.Join(rootPath, "internal", "cli", "schemacachepb", "schema_cache.pb.go"))
	if err != nil {
		return identityProof{}, fmt.Errorf("read schema_cache.pb.go: %w", err)
	}
	descriptorBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(protodesc.ToFileDescriptorProto(schemacachepb.File_schema_cache_proto))
	if err != nil {
		return identityProof{}, fmt.Errorf("marshal Schema cache descriptor: %w", err)
	}
	source, err := exactProofDigest(artifacts.SourceHash)
	if err != nil {
		return identityProof{}, err
	}
	surface, err := exactProofDigest(artifacts.SurfaceHash)
	if err != nil {
		return identityProof{}, err
	}
	protobufVersion := protobufRuntimeVersion()
	if protobufVersion == "unknown" {
		return identityProof{}, fmt.Errorf("resolve protobuf runtime version from Go build info")
	}
	input := buildIDInput{
		Edition: editionName, EnvelopeVersion: schemacache.EnvelopeVersion, DTOFormatVersion: schemacache.DTOFormatVersion,
		SchemaCacheDTOVersion: schemaruntime.SchemaCacheDTOVersion, CatalogSnapshotVersion: uint32(artifacts.Version),
		Serializer: schemacache.SerializerProtobuf, Codec: schemacache.CodecRaw,
		SourceSHA256: source, SurfaceSHA256: surface, MetaLength: uint64(len(artifacts.Meta)), MetaSHA256: artifacts.MetaSHA256,
		RegistryLength: uint64(len(artifacts.Registry)), RegistrySHA256: artifacts.RegistrySHA256, ProductCount: uint64(artifacts.ProductCount),
		GoRuntimeVersion: runtime.Version(), ProtoSHA256: sha256.Sum256(protoBytes), GeneratedPBGoSHA256: sha256.Sum256(pbGoBytes),
		DescriptorSHA256: sha256.Sum256(descriptorBytes), ProtocVersion: protocVersion, ProtocGenGoVersion: protocGenGoVersion,
		ProtobufRuntimeVersion: protobufVersion,
	}
	buildID := deterministicBuildID(input)
	return identityProof{
		Version: identityProofVersion, Edition: editionName, EnvelopeVersion: input.EnvelopeVersion,
		DTOFormatVersion: input.DTOFormatVersion, SchemaCacheDTOVersion: int(input.SchemaCacheDTOVersion),
		CatalogSnapshotVersion: int(input.CatalogSnapshotVersion), Serializer: input.Serializer, Codec: input.Codec,
		SourceSHA256: digestHex(source), SurfaceSHA256: digestHex(surface), MetaLength: input.MetaLength,
		MetaSHA256: digestHex(input.MetaSHA256), RegistryLength: input.RegistryLength, RegistrySHA256: digestHex(input.RegistrySHA256),
		ProductCount: int(input.ProductCount), GoRuntimeVersion: input.GoRuntimeVersion, ProtoSHA256: digestHex(input.ProtoSHA256),
		GeneratedPBGoSHA256: digestHex(input.GeneratedPBGoSHA256), DescriptorSHA256: digestHex(input.DescriptorSHA256),
		ProtocVersion: input.ProtocVersion, ProtocGenGoVersion: input.ProtocGenGoVersion,
		ProtobufRuntimeVersion: input.ProtobufRuntimeVersion, BuildID: digestHex(buildID),
	}, nil
}

func deterministicBuildID(input buildIDInput) [sha256.Size]byte {
	var canonical bytes.Buffer
	canonical.WriteString("dws-schema-cache-build-id-v1\x00")
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
	field(16, input.ProtoSHA256[:])
	field(17, input.GeneratedPBGoSHA256[:])
	field(18, input.DescriptorSHA256[:])
	field(19, []byte(input.ProtocVersion))
	field(20, []byte(input.ProtocGenGoVersion))
	field(21, []byte(input.ProtobufRuntimeVersion))
	return sha256.Sum256(canonical.Bytes())
}

func protobufRuntimeVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dependency := range info.Deps {
			if dependency.Path == "google.golang.org/protobuf" {
				return dependency.Version
			}
		}
	}
	return "unknown"
}

func exactProofDigest(value string) ([sha256.Size]byte, error) {
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

func digestHex(digest [sha256.Size]byte) string { return hex.EncodeToString(digest[:]) }

func encodeIdentityProof(proof identityProof, format string) ([]byte, error) {
	switch strings.TrimSpace(format) {
	case "json":
		encoded, err := json.Marshal(proof)
		if err != nil {
			return nil, err
		}
		return append(encoded, '\n'), nil
	case "shell":
		values := []struct{ key, value string }{
			{"SCHEMA_CACHE_EDITION", proof.Edition}, {"SCHEMA_CACHE_SOURCE_SHA256", proof.SourceSHA256},
			{"SCHEMA_CACHE_SURFACE_SHA256", proof.SurfaceSHA256}, {"SCHEMA_CACHE_BUILD_ID", proof.BuildID},
			{"SCHEMA_CACHE_META_LENGTH", strconv.FormatUint(proof.MetaLength, 10)}, {"SCHEMA_CACHE_META_SHA256", proof.MetaSHA256},
			{"SCHEMA_CACHE_REGISTRY_LENGTH", strconv.FormatUint(proof.RegistryLength, 10)}, {"SCHEMA_CACHE_REGISTRY_SHA256", proof.RegistrySHA256},
		}
		var output strings.Builder
		for _, value := range values {
			if value.value == "" || strings.ContainsAny(value.value, " \t\r\n'\"`$\\") {
				return nil, fmt.Errorf("%s is not shell-safe", value.key)
			}
			output.WriteString(value.key + "=" + value.value + "\n")
		}
		return []byte(output.String()), nil
	default:
		return nil, fmt.Errorf("unsupported proof format %q", format)
	}
}

func writeAtomic(path string, payload []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create proof directory: %w", err)
	}
	file, err := os.CreateTemp(directory, ".schema-cache-identity-*")
	if err != nil {
		return fmt.Errorf("create proof staging file: %w", err)
	}
	staging := file.Name()
	defer os.Remove(staging)
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(staging, path); err != nil {
		return fmt.Errorf("replace proof: %w", err)
	}
	return nil
}
