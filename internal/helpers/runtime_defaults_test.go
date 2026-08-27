package helpers

import (
	"context"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// Relocated verbatim from content_splitter_test.go, which was deleted along with
// the block-rebuilding splitter. This test targets runtime_defaults.go and has
// nothing to do with markdown chunking.
func TestCrossPlatformCoverageRuntimeDefaultsRegistryValidationAndSnapshot(t *testing.T) {
	runtimeDefaultsMu.Lock()
	previous := runtimeDefaults
	runtimeDefaults = make(map[string]edition.RuntimeDefaultFn)
	runtimeDefaultsMu.Unlock()
	t.Cleanup(func() {
		runtimeDefaultsMu.Lock()
		runtimeDefaults = previous
		runtimeDefaultsMu.Unlock()
	})

	resolver := func(context.Context) (string, bool) { return "value", true }
	RegisterRuntimeDefault("$value", resolver)
	snapshot := RuntimeDefaultsSnapshot()
	if len(snapshot) != 1 || snapshot["$value"] == nil {
		t.Fatalf("RuntimeDefaultsSnapshot() = %#v", snapshot)
	}
	delete(snapshot, "$value")
	if len(RuntimeDefaultsSnapshot()) != 1 {
		t.Fatal("snapshot mutated the runtime registry")
	}

	for _, tc := range []struct {
		name string
		fn   edition.RuntimeDefaultFn
	}{
		{"", resolver}, {"$nil", nil}, {"$value", resolver},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("RegisterRuntimeDefault(%q) did not panic", tc.name)
				}
			}()
			RegisterRuntimeDefault(tc.name, tc.fn)
		}()
	}
}
