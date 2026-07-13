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

package executor

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestInvocationReadOnlyDryRunCapabilityIsNotSerialized(t *testing.T) {
	invocation := NewHelperInvocation("overlay.pat.pat.batch_plan", "pat", "pat.batch_plan", map[string]any{"dryRun": true})
	invocation.DryRun = true
	invocation.AllowReadOnlyDuringDryRun = true

	encoded, err := json.Marshal(invocation)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), "AllowReadOnlyDuringDryRun") || strings.Contains(string(encoded), "allow_read_only") {
		t.Fatalf("read-only dry-run capability leaked into JSON: %s", encoded)
	}
	if invocation.SkipExecutionDuringDryRun() {
		t.Fatal("allowed read-only invocation should not be skipped")
	}

	invocation.AllowReadOnlyDuringDryRun = false
	if !invocation.SkipExecutionDuringDryRun() {
		t.Fatal("ordinary dry-run invocation should be skipped")
	}
	result, err := (EchoRunner{}).Run(context.Background(), invocation)
	if err != nil {
		t.Fatalf("EchoRunner.Run() error = %v", err)
	}
	if result.Response["dry_run"] != true {
		t.Fatalf("EchoRunner dry_run = %#v, want true", result.Response["dry_run"])
	}
}
