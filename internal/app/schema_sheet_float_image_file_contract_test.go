package app

import (
	"testing"
)

func TestSheetFloatImageLocalFileFinalSchema(t *testing.T) {
	payload := schemaContractPayloadForBoundCanonicals(t, NewRootCommand(),
		"sheet.create_float_image",
		"sheet.update_float_image",
	)

	create := payload.Tools["sheet.create_float_image"]
	if create == nil {
		t.Fatal("missing sheet.create_float_image")
	}
	if create["interface_mode"] != "mcp" {
		t.Fatalf("create interface mode = %#v", create["interface_mode"])
	}
	createRef, _ := create["interface_ref"].(map[string]any)
	if createRef["product_id"] != "sheet" || createRef["rpc_name"] != "create_float_image" {
		t.Fatalf("create interface ref = %#v", createRef)
	}
	createDryRun, _ := create["dry_run"].(map[string]any)
	if createDryRun["preview_kind"] != "request" {
		t.Fatalf("create dry-run = %#v", createDryRun)
	}
	if remoteReads, exists := createDryRun["remote_reads"]; exists && remoteReads != false {
		t.Fatalf("create dry-run remote_reads = %#v", remoteReads)
	}
	createParameters, _ := create["parameters"].(map[string]any)
	file, _ := createParameters["file"].(map[string]any)
	src, _ := createParameters["src"].(map[string]any)
	if file["required"] != false || file["required_when"] != "exactly one of --file or --src must be provided" {
		t.Fatalf("create --file metadata = %#v", file)
	}
	if schemaContractString(file["property"]) != "" {
		t.Fatalf("create --file leaked an RPC property: %#v", file["property"])
	}
	if src["required"] != false || schemaContractString(src["required_when"]) != "" || src["property"] != "src" {
		t.Fatalf("create --src compatibility metadata = %#v", src)
	}
	assertSchemaContractConstraintGroup(t, create, "mutually_exclusive", []string{"file", "src"})
	assertSchemaContractConstraintGroup(t, create, "require_one_of", []string{"file", "src"})

	update := payload.Tools["sheet.update_float_image"]
	if update == nil {
		t.Fatal("missing sheet.update_float_image")
	}
	updateDryRun, _ := update["dry_run"].(map[string]any)
	if update["interface_mode"] != "mcp" || updateDryRun["preview_kind"] != "request" {
		t.Fatalf("update interface/dry-run = %#v/%#v", update["interface_mode"], updateDryRun)
	}
	updateParameters, _ := update["parameters"].(map[string]any)
	updateFile, _ := updateParameters["file"].(map[string]any)
	if schemaContractString(updateFile["property"]) != "" {
		t.Fatalf("update --file leaked an RPC property: %#v", updateParameters["file"])
	}
	assertSchemaContractConstraintGroup(t, update, "mutually_exclusive", []string{"file", "src"})
	assertSchemaContractConstraintGroup(t, update, "require_one_of", []string{"file", "src", "range", "width", "height", "offset-x", "offset-y"})

	root := NewRootCommand()
	for _, cliPath := range []string{"sheet create-float-image", "sheet update-float-image"} {
		command := exactCommandForTest(root, cliPath)
		if command == nil || command.Flags().Lookup("file") == nil {
			t.Fatalf("%s has no executable --file flag", cliPath)
		}
	}

}
