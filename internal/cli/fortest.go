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
	"sync/atomic"

	"github.com/spf13/cobra"
)

// Test helpers for this package's lazy delivery state. Production code must
// not call these; the ForTest suffix is the boundary. Cross-package tests use
// the exported ones; in-package tests may use the unexported resetters.

func resetDeliverySchemaCatalogStateForTest() {
	resetDeliverySchemaCatalogState()
}

func resetMetaByCLIPathStateForTest() {
	resetSchemaDeliveryState()
}

func resetSchemaSourceRootAtomicForTest() {
	schemaSourceRoot = atomic.Value{}
}

// clearDeclaredDryRunCapabilitiesForTest resets the declared index (tests only).
func clearDeclaredDryRunCapabilitiesForTest() {
	declaredDryRunCapabilities.Range(func(key, _ any) bool {
		declaredDryRunCapabilities.Delete(key)
		return true
	})
}

func resetReviewedDryRunCapabilitiesLazyForTest() {
	resetReviewedDryRunCapabilitiesLazy()
}

func setReviewedDryRunCapabilityGroupsForTest(groups []dryRunCapabilityGroup) func() {
	previous := reviewedDryRunCapabilityGroups
	reviewedDryRunCapabilityGroups = groups
	resetReviewedDryRunCapabilitiesLazy()
	return func() {
		reviewedDryRunCapabilityGroups = previous
		resetReviewedDryRunCapabilitiesLazy()
	}
}

// InstallProductionSchemaAssemblyForTest installs the production
// assembleSchemaCatalogFromRoot delivery path with factory and clears lazy
// caches. Tests must Cleanup via RegisterSchemaSourceRoot(nil) or a restore
// helper that reinstalls their prior delivery stub.
func InstallProductionSchemaAssemblyForTest(factory func() *cobra.Command) {
	RegisterSchemaSourceRoot(factory)
	assembleDeliverySchemaCatalogFn = assembleSchemaCatalogFromRoot
	resetDeliverySchemaCatalogStateForTest()
}

// DeliverySchemaAllPayloadForTest returns schema --all through the installed
// delivery loader (catalog_hash comes from Snapshot.SourceHash).
func DeliverySchemaAllPayloadForTest() (map[string]any, error) {
	return deliverySchemaAllPayload()
}

// RestorePackageCLISchemaDeliveryForTest reinstalls the package-cli TestMain
// assembled-delivery stub after a production-assembly exercise. Outside package
// cli TestMain it clears the factory and resets lazy delivery state.
func RestorePackageCLISchemaDeliveryForTest() {
	if restorePackageCLISchemaDeliveryHook != nil {
		restorePackageCLISchemaDeliveryHook()
		return
	}
	storeSchemaSourceRootFn(nil)
	assembleDeliverySchemaCatalogFn = assembleSchemaCatalogFromRoot
	resetSchemaDeliveryState()
}

// restorePackageCLISchemaDeliveryHook is installed by package-cli TestMain so
// external cli_test helpers can restore the assembled-delivery stub.
var restorePackageCLISchemaDeliveryHook func()
