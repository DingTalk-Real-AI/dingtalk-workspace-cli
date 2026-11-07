package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageDocSuggestionHiddenInDeliveredSchema(t *testing.T) {
	canonicals := []string{"doc.get_document_content", "doc.insert_document_block", "doc.update_document_block", "doc.delete_document_block"}
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), canonicals...)
	for _, canonical := range canonicals {
		tool := payload.Tools[canonical]
		parameters, _ := tool["parameters"].(map[string]any)
		for _, name := range []string{"suggestion", "sg"} {
			if _, ok := parameters[name]; ok {
				t.Errorf("%s exposes rollout parameter %s", canonical, name)
			}
		}
		ref, _ := tool["interface_ref"].(map[string]any)
		if tool["interface_mode"] != "mcp" || ref["rpc_name"] != canonical[len("doc."):] {
			t.Errorf("%s interface = %#v %#v, want existing MCP RPC", canonical, tool["interface_mode"], ref)
		}
	}
	for _, path := range []string{"doc read", "doc block insert", "doc block update", "doc block delete", "doc +update"} {
		for _, compact := range []bool{false, true} {
			query := []string{"--cli-path", path}
			if compact {
				query = append(query, "--compact")
			}
			leaf := executeShortcutSchemaQuery(t, query...)
			encoded, err := json.Marshal(leaf)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "suggestion") || strings.Contains(string(encoded), "--sg") || strings.Contains(string(encoded), "建议模式") {
				t.Errorf("%s compact=%t publishes rollout selector", path, compact)
			}
		}
	}
}

func TestCrossPlatformCoverageDiffblockRollbackAndRangeEditFinalSchema(t *testing.T) {
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), "doc.update_document", "doc.update_document_block", "doc.insert_document_block")
	for canonical, want := range map[string]map[string]string{
		"doc.update_document":       {"force": "force"},
		"doc.update_document_block": {"start-block-id": "startBlockId", "end-block-id": "endBlockId", "jsonml-nodes": "jsonml"},
		"doc.insert_document_block": {"jsonml-nodes": "jsonml"},
	} {
		parameters, _ := payload.Tools[canonical]["parameters"].(map[string]any)
		for name, property := range want {
			parameter, _ := parameters[name].(map[string]any)
			if parameter["property"] != property {
				t.Fatalf("%s %s property=%#v", canonical, name, parameter)
			}
		}
	}
}

func TestCrossPlatformCoverageDocInsertParentHasNoPublicReferenceMapping(t *testing.T) {
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(), "doc.insert_document_block")
	parameters, _ := payload.Tools["doc.insert_document_block"]["parameters"].(map[string]any)
	parent, _ := parameters["parent-block"].(map[string]any)
	if property, _ := parent["property"].(string); property != "" {
		t.Fatalf("unsupported parent publishes interface property %q", property)
	}
	reference, _ := parameters["ref-block"].(map[string]any)
	if reference["property"] != "referenceBlockId" {
		t.Fatalf("supported reference mapping changed: %#v", reference)
	}
}
