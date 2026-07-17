package ir

import "testing"

func TestCrossPlatformCoverageCatalogLookupCoverage(t *testing.T) {
	tool := ToolDescriptor{RPCName: "create"}
	product := CanonicalProduct{ID: "doc", Tools: []ToolDescriptor{tool}}
	catalog := Catalog{Products: []CanonicalProduct{product}}

	if got, ok := catalog.FindProduct("doc"); !ok || got.ID != "doc" {
		t.Fatalf("product lookup = %#v, %v", got, ok)
	}
	if _, ok := catalog.FindProduct("missing"); ok {
		t.Fatal("missing product found")
	}
	if gotProduct, gotTool, ok := catalog.FindTool("doc.create"); !ok || gotProduct.ID != "doc" || gotTool.RPCName != "create" {
		t.Fatalf("tool lookup = %#v %#v %v", gotProduct, gotTool, ok)
	}
	for _, path := range []string{"", ".create", "doc.", "missing.create", "doc.missing"} {
		if _, _, ok := catalog.FindTool(path); ok {
			t.Fatalf("invalid tool path %q found", path)
		}
	}
	if _, ok := product.FindTool("missing"); ok {
		t.Fatal("missing product tool found")
	}
}
