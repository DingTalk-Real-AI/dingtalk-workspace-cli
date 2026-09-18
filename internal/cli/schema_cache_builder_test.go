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
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageSchemaCacheBuildResultRoundTrip(t *testing.T) {
	artifacts, err := buildSchemaCacheArtifactsFromLoaded(*packageCLIAssembledDelivery)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := IdentityFromArtifacts("open", artifacts)
	if err != nil {
		t.Fatal(err)
	}
	result := SchemaCacheBuildResult{Artifacts: artifacts, Identity: identity}
	var output bytes.Buffer
	if err := WriteSchemaCacheBuildResult(&output, result); err != nil {
		t.Fatal(err)
	}
	decoded, err := ReadSchemaCacheBuildResult(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Identity.BuildID != identity.BuildID {
		t.Fatalf("decoded identity = %x, want %x", decoded.Identity.BuildID, identity.BuildID)
	}
	if err := WriteSchemaCacheBuildResult(failingSchemaCacheWriter{}, result); err == nil {
		t.Fatal("failing writer unexpectedly succeeded")
	}
	oldMarshal := marshalSchemaCacheBuilder
	marshalSchemaCacheBuilder = func(any) ([]byte, error) { return nil, errors.New("marshal failed") }
	if err := WriteSchemaCacheBuildResult(&bytes.Buffer{}, result); err == nil {
		t.Fatal("marshal failure unexpectedly succeeded")
	}
	marshalSchemaCacheBuilder = oldMarshal
	oldLimit := maxSchemaCacheBuilderResponse
	maxSchemaCacheBuilderResponse = 1
	if err := WriteSchemaCacheBuildResult(&bytes.Buffer{}, result); err == nil {
		t.Fatal("response limit unexpectedly accepted")
	}
	maxSchemaCacheBuilderResponse = oldLimit
}

type failingSchemaCacheWriter struct{}

func (failingSchemaCacheWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestCrossPlatformCoverageSchemaCacheBuildResultBuilderRegistration(t *testing.T) {
	old := isolatedSchemaCacheBuilder
	t.Cleanup(func() { isolatedSchemaCacheBuilder = old })
	RegisterSchemaCacheIsolatedBuilder(nil)
	if _, err := buildSchemaCacheInIsolatedProcess(nil); err == nil {
		t.Fatal("unregistered isolated builder unexpectedly succeeded")
	}
}

func TestCrossPlatformCoverageSchemaCacheBuildResultValidationErrors(t *testing.T) {
	artifacts, err := buildSchemaCacheArtifactsFromLoaded(*packageCLIAssembledDelivery)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := IdentityFromArtifacts("open", artifacts)
	if err != nil {
		t.Fatal(err)
	}
	badIdentity := identity
	badIdentity.BuildID[0]++
	if err := validateDetachedSchemaCacheBuildResult(SchemaCacheBuildResult{Artifacts: artifacts, Identity: badIdentity}); err == nil {
		t.Fatal("identity mismatch unexpectedly accepted")
	}
	invalid := artifacts
	invalid.Payload = []byte{1}
	if err := validateDetachedSchemaCacheBuildResult(SchemaCacheBuildResult{Artifacts: invalid, Identity: identity}); err == nil {
		t.Fatal("invalid payload unexpectedly accepted")
	}
	invalid = artifacts
	invalid.SourceHash = "bad"
	if err := validateDetachedSchemaCacheBuildResult(SchemaCacheBuildResult{Artifacts: invalid, Identity: identity}); err == nil {
		t.Fatal("invalid source hash unexpectedly accepted")
	}
}

type failingSchemaCacheReader struct{}

func (failingSchemaCacheReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestCrossPlatformCoverageReadSchemaCacheBuildResultReadError(t *testing.T) {
	if _, err := ReadSchemaCacheBuildResult(failingSchemaCacheReader{}); err == nil {
		t.Fatal("read failure unexpectedly succeeded")
	}
}

func TestCrossPlatformCoverageReadSchemaCacheBuildResultRejectsEmptyAndTrailing(t *testing.T) {
	for _, payload := range []string{
		`{"ProtocolVersion":1}`,
		`{"ProtocolVersion":1} {}`,
		`{"ProtocolVersion":1,"Unknown":true}`,
	} {
		if _, err := ReadSchemaCacheBuildResult(strings.NewReader(payload)); err == nil {
			t.Fatalf("payload unexpectedly accepted: %s", payload)
		}
	}
}

func TestCrossPlatformCoverageReadSchemaCacheBuildResultRejectsUnsupportedProtocol(t *testing.T) {
	payload, err := json.Marshal(schemaCacheBuildWire{ProtocolVersion: schemaCacheBuilderProtocolVersion + 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSchemaCacheBuildResult(bytes.NewReader(payload)); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error = %v, want unsupported protocol", err)
	}
}

func TestCrossPlatformCoverageReadSchemaCacheBuildResultRejectsOversizedResponse(t *testing.T) {
	payload := bytes.Repeat([]byte{'x'}, maxSchemaCacheBuilderResponse+1)
	if _, err := ReadSchemaCacheBuildResult(bytes.NewReader(payload)); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want size limit", err)
	}
}
