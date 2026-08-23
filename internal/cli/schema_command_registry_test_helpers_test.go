package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func commandRegistryTestRoot(paths ...string) *cobra.Command {
	root := &cobra.Command{Use: "dws"}
	groups := map[string]*cobra.Command{}
	for _, path := range paths {
		parts := strings.Fields(path)
		current, prefix := root, ""
		for index, part := range parts {
			if prefix == "" {
				prefix = part
			} else {
				prefix += " " + part
			}
			if index == len(parts)-1 {
				current.AddCommand(&cobra.Command{Use: part, Run: func(*cobra.Command, []string) {}})
				continue
			}
			next := groups[prefix]
			if next == nil {
				next = &cobra.Command{Use: part}
				groups[prefix] = next
				current.AddCommand(next)
			}
			current = next
		}
	}
	return root
}

func mustEffectiveCommandRegistry(t *testing.T, commands []CommandSpec) EffectiveCommandRegistry {
	t.Helper()
	registry, err := newEffectiveCommandRegistry(commands)
	if err != nil {
		t.Fatalf("newEffectiveCommandRegistry() error = %v", err)
	}
	return registry
}

func annotateTestCompatibilityPair(primary, alias *cobra.Command) {
	AnnotateRuntimeCompatibilityEquivalence(primary, alias, RuntimeCompatibilityEquivalence{
		ID:       "test-compatibility-pair-v1",
		Reason:   "Focused test review confirms equivalent compatibility leaves.",
		Reviewed: true,
	})
}
