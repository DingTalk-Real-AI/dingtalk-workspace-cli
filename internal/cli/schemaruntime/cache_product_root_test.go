package schemaruntime

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func TestCrossPlatformCoverageCacheLocatorsKeepDeclaredProductAndDifferentCLIRoot(t *testing.T) {
	tool := ToolSpec{Identity: contract.ToolIdentitySpec{
		ProductID: "samplecatalog", Name: "query_record", CanonicalPath: "samplecatalog.query_record",
		Path: "samplecatalog.query_record", CLIPath: "sample record query", PrimaryCLIPath: "sample record query",
		Aliases: []string{"samplecatalog record query"},
	}}
	registry, err := SchemaRegistryFromRuntime("test", []ProductSpec{{ID: "samplecatalog", Tools: []ToolSpec{tool}}})
	if err != nil {
		t.Fatal(err)
	}
	locators, err := BuildSchemaProductLocators(registry)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"sample", "sample record", "sample.record"} {
		if _, ok := locators[path]; ok {
			t.Errorf("不可查询的 CLI 前缀进入缓存导航: %s", path)
		}
	}
	for _, path := range []string{"samplecatalog", "samplecatalog.query_record", "sample record query", "samplecatalog record"} {
		if locators[path] != "samplecatalog" {
			t.Errorf("已声明产品/叶子/组丢失: %s", path)
		}
	}
	index, err := registry.Index()
	if err != nil {
		t.Fatal(err)
	}
	for path := range locators {
		if _, err := RenderQuery(registry, index, path); err != nil {
			t.Errorf("缓存 locator %s 无法通过真实查询: %v", path, err)
		}
	}
}
