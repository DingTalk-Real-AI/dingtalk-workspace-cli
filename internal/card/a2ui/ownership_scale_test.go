// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"fmt"
	"testing"
)

// Every template starts at a different point of one shared static chain. A
// traversal per template would repeatedly scan the same suffixes quadratically.
func ownershipScaleGraph(size int) (referenceSnapshot, map[string][]referenceEdge) {
	s := referenceSnapshot{nodes: map[string]referenceNode{"root": {location: "/components/root"}}}
	edges := map[string][]referenceEdge{}
	for i := 0; i < size; i++ {
		owner, source := fmt.Sprintf("container_%d", i), fmt.Sprintf("source_%d", i)
		s.nodes[owner] = referenceNode{location: "/components/" + owner}
		s.nodes[source] = referenceNode{location: "/components/" + source}
		edges["root"] = append(edges["root"], referenceEdge{owner, fmt.Sprintf("/root/children/%d", i), false})
		edges[owner] = []referenceEdge{{source, "/" + owner + "/children/componentId", true}}
		if i+1 < size {
			edges[source] = []referenceEdge{{fmt.Sprintf("source_%d", i+1), "/" + source + "/children/0", false}}
		}
	}
	return s, edges
}

func TestAicardOwnershipLargeOverlappingTemplateSources(t *testing.T) {
	for _, size := range []int{1, 3000} {
		t.Run(fmt.Sprintf("%d sources", size), func(t *testing.T) {
			s, edges := ownershipScaleGraph(size)
			diagnostics := checkStaticOwnership(s, "scale", edges)
			if len(diagnostics) != size-1 {
				t.Fatalf("want %d additional source owners, got %d", size-1, len(diagnostics))
			}
			for _, diagnostic := range diagnostics {
				if diagnostic.Code != "reference.multiple_parents" || diagnostic.Severity != "error" {
					t.Fatal(diagnostic)
				}
			}
		})
	}
}

func BenchmarkAicardOwnershipOverlappingTemplateSources(b *testing.B) {
	for _, size := range []int{300, 3000} {
		b.Run(fmt.Sprintf("%d sources", size), func(b *testing.B) {
			s, edges := ownershipScaleGraph(size)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				checkStaticOwnership(s, "scale", edges)
			}
		})
	}
}
