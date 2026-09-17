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
	"testing"

	"github.com/spf13/cobra"
)

// The schema source factory — including any environment its command
// constructors read — must execute under the registration-time environment
// snapshot, never the live one. This is the structural boundary that keeps a
// plugin-present process (whose loader may inject arbitrary non-blacklisted
// variables after registration) from assembling and publishing a schema
// surface that only holds in its own environment.
func TestDeliverySchemaCatalogAssemblesUnderPristineEnviron(t *testing.T) {
	const canary = "DWS_TEST_SCHEMA_ASSEMBLY_ENV_ISOLATION"
	t.Cleanup(func() {
		_ = os.Unsetenv(canary)
		RestorePackageCLISchemaDeliveryForTest()
	})

	// Register with the canary absent: the snapshot must not contain it.
	_ = os.Unsetenv(canary)
	var observed string
	RestorePackageCLISchemaDeliveryForTest()
	RegisterSchemaSourceRoot(func() *cobra.Command {
		observed = os.Getenv(canary)
		return nil // factory ran; assembly fails closed on nil, which is fine here
	})

	// Simulate a post-registration plugin-config injection.
	if err := os.Setenv(canary, "plugin-injected"); err != nil {
		t.Fatal(err)
	}
	_ = deliverySchemaCatalog()
	if observed != "" {
		t.Fatalf("assembly observed post-registration env %s=%q; the pristine-environment boundary leaked", canary, observed)
	}
	if got := os.Getenv(canary); got != "plugin-injected" {
		t.Fatalf("live environment not restored after assembly: %s=%q", canary, got)
	}

	// Control: a variable already present at registration stays visible to
	// assembly — the snapshot is the registration-time environment, not an
	// arbitrary blacklist.
	observed = ""
	RestorePackageCLISchemaDeliveryForTest()
	RegisterSchemaSourceRoot(func() *cobra.Command {
		observed = os.Getenv(canary)
		return nil
	})
	_ = deliverySchemaCatalog()
	if observed != "plugin-injected" {
		t.Fatalf("control: variable present at registration invisible to assembly: %q", observed)
	}
}
