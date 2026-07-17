package registry

import (
	"bytes"
	"testing"
)

func TestCrossPlatformCoverageEmbeddedRegistryDataReturnsDefensiveCopies(t *testing.T) {
	personas := PersonasYAML()
	recipes := RecipesYAML()
	if len(personas) == 0 || len(recipes) == 0 {
		t.Fatal("embedded registry data is empty")
	}
	want := append([]byte(nil), personas...)
	personas[0] ^= 0xff
	if !bytes.Equal(PersonasYAML(), want) {
		t.Fatal("PersonasYAML() returned shared mutable storage")
	}
	want = append([]byte(nil), recipes...)
	recipes[0] ^= 0xff
	if !bytes.Equal(RecipesYAML(), want) {
		t.Fatal("RecipesYAML() returned shared mutable storage")
	}
}
