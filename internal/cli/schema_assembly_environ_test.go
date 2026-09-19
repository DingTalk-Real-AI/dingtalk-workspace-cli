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
	"os"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageSchemaAssemblyEnvironmentSnapshotDoesNotMutateParent(t *testing.T) {
	const canary = "DWS_TEST_SCHEMA_ASSEMBLY_ENV_ISOLATION"
	t.Cleanup(func() {
		_ = os.Unsetenv(canary)
		clearSchemaAssemblyEnviron()
	})

	_ = os.Unsetenv(canary)
	CaptureSchemaAssemblyEnviron()
	if err := os.Setenv(canary, "plugin-injected"); err != nil {
		t.Fatal(err)
	}
	snapshot := SchemaAssemblyEnvironmentSnapshot()
	for _, entry := range snapshot {
		if strings.HasPrefix(entry, canary+"=") {
			t.Fatalf("snapshot captured post-registration environment: %q", entry)
		}
	}
	if got := os.Getenv(canary); got != "plugin-injected" {
		t.Fatalf("parent environment changed: %q", got)
	}

	CaptureSchemaAssemblyEnviron()
	if got := SchemaAssemblyEnvironmentSnapshot(); !containsEnvironment(got, canary+"=plugin-injected") {
		t.Fatalf("snapshot omitted registration-time variable: %v", got)
	}
	if dir := SchemaAssemblyWorkingDirectory(); dir == "" {
		t.Fatal("expected non-empty working dir")
	}
}

func containsEnvironment(environment []string, want string) bool {
	for _, entry := range environment {
		if entry == want {
			return true
		}
	}
	return false
}
