// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

const (
	schemaCacheBuilderProtocolVersion = 1
	maxSchemaCacheBuilderResponse     = 64 << 20
)

// SchemaCacheBuildResult is the detached result of a clean declaration-only
// Schema assembly. It contains no live Cobra or registry state.
type SchemaCacheBuildResult struct {
	Artifacts SchemaCacheArtifacts
	Identity  SchemaCacheIdentity
}

type schemaCacheBuildWire struct {
	ProtocolVersion int
	ArtifactVersion int
	SourceHash      string
	SurfaceHash     string
	Meta            []byte
	Registry        []byte
	Payload         []byte
	ProductCount    int
	Identity        SchemaCacheIdentity
}

// SchemaCacheIsolatedBuilder assembles a cache generation outside the caller's
// process state. Production installs this from internal/app.
type SchemaCacheIsolatedBuilder func(context.Context) (SchemaCacheBuildResult, error)

var isolatedSchemaCacheBuilder SchemaCacheIsolatedBuilder

// RegisterSchemaCacheIsolatedBuilder installs the production child-process
// builder. Passing nil is intended for tests only.
func RegisterSchemaCacheIsolatedBuilder(builder SchemaCacheIsolatedBuilder) {
	isolatedSchemaCacheBuilder = builder
}

func buildSchemaCacheInIsolatedProcess(ctx context.Context) (SchemaCacheBuildResult, error) {
	if isolatedSchemaCacheBuilder == nil {
		return SchemaCacheBuildResult{}, fmt.Errorf("isolated Schema cache builder is not registered")
	}
	return isolatedSchemaCacheBuilder(ctx)
}

// WriteSchemaCacheBuildResult writes a bounded, versioned private response.
func WriteSchemaCacheBuildResult(w io.Writer, result SchemaCacheBuildResult) error {
	wire := schemaCacheBuildWire{
		ProtocolVersion: schemaCacheBuilderProtocolVersion,
		ArtifactVersion: result.Artifacts.Version,
		SourceHash:      result.Artifacts.SourceHash,
		SurfaceHash:     result.Artifacts.SurfaceHash,
		Meta:            result.Artifacts.Meta,
		Registry:        result.Artifacts.Registry,
		Payload:         result.Artifacts.Payload,
		ProductCount:    result.Artifacts.ProductCount,
		Identity:        result.Identity,
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return err
	}
	if len(encoded) > maxSchemaCacheBuilderResponse {
		return fmt.Errorf("Schema cache builder response exceeds %d bytes", maxSchemaCacheBuilderResponse)
	}
	_, err = w.Write(encoded)
	return err
}

// ReadSchemaCacheBuildResult reads and validates a private builder response.
func ReadSchemaCacheBuildResult(r io.Reader) (SchemaCacheBuildResult, error) {
	encoded, err := io.ReadAll(io.LimitReader(r, maxSchemaCacheBuilderResponse+1))
	if err != nil {
		return SchemaCacheBuildResult{}, err
	}
	if len(encoded) > maxSchemaCacheBuilderResponse {
		return SchemaCacheBuildResult{}, fmt.Errorf("Schema cache builder response exceeds %d bytes", maxSchemaCacheBuilderResponse)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var wire schemaCacheBuildWire
	if err := decoder.Decode(&wire); err != nil {
		return SchemaCacheBuildResult{}, fmt.Errorf("decode Schema cache builder response: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return SchemaCacheBuildResult{}, fmt.Errorf("Schema cache builder response has trailing data")
	}
	if wire.ProtocolVersion != schemaCacheBuilderProtocolVersion {
		return SchemaCacheBuildResult{}, fmt.Errorf("unsupported Schema cache builder protocol version %d", wire.ProtocolVersion)
	}
	artifacts := SchemaCacheArtifacts{
		Version:        wire.ArtifactVersion,
		SourceHash:     wire.SourceHash,
		SurfaceHash:    wire.SurfaceHash,
		Meta:           append([]byte(nil), wire.Meta...),
		Registry:       append([]byte(nil), wire.Registry...),
		Payload:        append([]byte(nil), wire.Payload...),
		ProductCount:   wire.ProductCount,
		MetaSHA256:     sha256.Sum256(wire.Meta),
		RegistrySHA256: sha256.Sum256(wire.Registry),
		PayloadSHA256:  sha256.Sum256(wire.Payload),
	}
	result := SchemaCacheBuildResult{Artifacts: artifacts, Identity: wire.Identity}
	if err := validateDetachedSchemaCacheBuildResult(result); err != nil {
		return SchemaCacheBuildResult{}, err
	}
	return result, nil
}

func validateDetachedSchemaCacheBuildResult(result SchemaCacheBuildResult) error {
	artifacts := result.Artifacts
	if len(artifacts.Meta) == 0 || len(artifacts.Registry) == 0 || len(artifacts.Payload) == 0 {
		return fmt.Errorf("Schema cache builder response contains an empty artifact")
	}
	if _, _, err := artifacts.PayloadIndexPins(); err != nil {
		return fmt.Errorf("validate Schema cache payload: %w", err)
	}
	identity, err := IdentityFromArtifacts(result.Identity.Edition, artifacts)
	if err != nil {
		return fmt.Errorf("derive Schema cache identity from response: %w", err)
	}
	if !reflect.DeepEqual(identity, result.Identity) {
		return fmt.Errorf("Schema cache builder identity does not match artifacts")
	}
	return nil
}
