package app

import (
	"strings"
	"testing"
)

func TestSheetCSVPutAutoConvertFinalSchema(t *testing.T) {
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), "sheet.set_range_from_csv")
	tool := payload.Tools["sheet.set_range_from_csv"]
	if tool == nil {
		t.Fatal("missing sheet.set_range_from_csv")
	}
	parameters, _ := tool["parameters"].(map[string]any)
	autoConvert, _ := parameters["auto-convert"].(map[string]any)
	if autoConvert == nil {
		t.Fatalf("missing auto-convert parameter: %#v", parameters)
	}
	if autoConvert["type"] != "boolean" {
		t.Fatalf("auto-convert type = %#v, want boolean", autoConvert["type"])
	}
	if _, exists := autoConvert["interface_type"]; exists {
		t.Fatalf("auto-convert should omit redundant interface_type matching its CLI type: %#v", autoConvert["interface_type"])
	}
	if autoConvert["property"] != "autoConvert" {
		t.Fatalf("auto-convert property = %#v, want autoConvert", autoConvert["property"])
	}
	if autoConvert["required"] != false || autoConvert["default"] != "true" {
		t.Fatalf("auto-convert required/default = %#v/%#v, want false/true", autoConvert["required"], autoConvert["default"])
	}
	description := schemaContractString(autoConvert["description"])
	for _, want := range []string{"非公式", "文本原样写入", "= 开头仍作为公式"} {
		if !strings.Contains(description, want) {
			t.Fatalf("auto-convert description does not contain %q: %s", want, description)
		}
	}

	command := exactCommandForTest(NewRootCommand(), "sheet csv-put")
	if command == nil {
		t.Fatal("sheet csv-put is not runnable")
	}
	flag := command.Flags().Lookup("auto-convert")
	if flag == nil || flag.DefValue != "true" {
		t.Fatalf("executable --auto-convert = %#v, want default true", flag)
	}
}
