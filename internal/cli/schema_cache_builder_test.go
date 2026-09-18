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
	"strings"
	"testing"
)

func TestReadSchemaCacheBuildResultRejectsUnsupportedProtocol(t *testing.T) {
	payload, err := json.Marshal(schemaCacheBuildWire{ProtocolVersion: schemaCacheBuilderProtocolVersion + 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSchemaCacheBuildResult(bytes.NewReader(payload)); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error = %v, want unsupported protocol", err)
	}
}

func TestReadSchemaCacheBuildResultRejectsOversizedResponse(t *testing.T) {
	payload := bytes.Repeat([]byte{'x'}, maxSchemaCacheBuilderResponse+1)
	if _, err := ReadSchemaCacheBuildResult(bytes.NewReader(payload)); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want size limit", err)
	}
}
