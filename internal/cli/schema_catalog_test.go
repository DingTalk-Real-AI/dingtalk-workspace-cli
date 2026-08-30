// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0

package cli

import (
	"sort"
	"strings"
	"testing"
)

func TestBuildSchemaCatalogSnapshotRejectsUnresolvedSource(t *testing.T) {
	_, err := BuildSchemaCatalogSnapshot(ResolvedSchemaBuild{}, SchemaCatalogBuildOptions{})
	if err == nil || !strings.Contains(err.Error(), "ResolveSchemaBuild") {
		t.Fatalf("BuildSchemaCatalogSnapshot() error = %v, want resolved-source requirement", err)
	}
}

func TestDeliverySchemaCatalogIntegrity(t *testing.T) {
	loaded := deliverySchemaCatalog()
	if !deliverySchemaCatalogAvailable() {
		t.Fatal("delivery schema catalog is unavailable or failed integrity validation")
	}
	if got := loaded.Registry.Source; got != SchemaSourceRuntimeAssembled {
		t.Fatalf("catalog source = %q, want %q", got, SchemaSourceRuntimeAssembled)
	}
}

func TestDeliverySchemaCatalogProgressiveQueries(t *testing.T) {
	overview, err := deliverySchemaOverviewPayload()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := schemaProductToolCount(map[string]any{"tools": overview["products"]}), len(deliverySchemaCatalog().Registry.Products); got != want {
		t.Fatalf("compact product count = %d, want %d", got, want)
	}

	leaf, err := queryDeliverySchemaPayload([]string{"calendar event create"})
	if err != nil {
		t.Fatal(err)
	}
	if got := schemaString(leaf["canonical_path"]); got != "calendar.create_calendar_event" {
		t.Fatalf("canonical path = %q", got)
	}
	if len(schemaMapSlice(leaf["parameters"])) != 0 {
		t.Fatal("parameters unexpectedly decoded as a list")
	}
	if parameters, ok := leaf["parameters"].(map[string]any); !ok || len(parameters) == 0 {
		t.Fatal("calendar.create_event parameters are empty")
	}

	group, err := queryDeliverySchemaPayload([]string{"calendar.event"})
	if err != nil {
		t.Fatal(err)
	}
	if schemaProductToolCount(map[string]any{"tools": group["tools"]}) == 0 {
		t.Fatal("calendar.event group is empty")
	}

	alias, err := queryDeliverySchemaPayload([]string{"aitable record list"})
	if err != nil {
		t.Fatal(err)
	}
	if alias["is_alias"] != true || schemaString(alias["cli_path"]) != "aitable record list" {
		t.Fatalf("alias query did not preserve compatibility path: %#v", alias)
	}
	if schemaString(alias["canonical_path"]) != "aitable.query_records" {
		t.Fatalf("alias canonical path = %q", schemaString(alias["canonical_path"]))
	}
}

func TestDeliverySchemaAllPayloadContainsEveryFullLeaf(t *testing.T) {
	loaded := mustDeliverySchemaCatalogMaps(t)
	payload, err := deliverySchemaAllPayload()
	if err != nil {
		t.Fatal(err)
	}

	expanded := 0
	parameterized := 0
	for _, product := range schemaMapSlice(payload["products"]) {
		for _, tool := range schemaMapSlice(product["tools"]) {
			canonical := schemaString(tool["canonical_path"])
			expected, ok := loaded.Snapshot.Tools[canonical]
			if !ok {
				t.Fatalf("full export contains unknown tool %q", canonical)
			}
			parameters, ok := tool["parameters"].(map[string]any)
			if !ok {
				t.Fatalf("full export tool %s has no parameters object", canonical)
			}
			if len(parameters) > 0 {
				parameterized++
			}
			if !schemaJSONEqual(tool, expected) {
				t.Fatalf("full export tool %s differs from stored leaf Schema", canonical)
			}
			expanded++
		}
	}
	if got, want := expanded, len(loaded.Snapshot.Tools); got != want {
		t.Fatalf("full export tools = %d, want %d", got, want)
	}
	if parameterized == 0 {
		t.Fatal("full export contains no parameterized tools")
	}
}

func TestDeliveryCatalogPreservesRegistryIdentityAndManualParameterContract(t *testing.T) {
	leaf, err := queryDeliverySchemaPayload([]string{"chat category create-smart"})
	if err != nil {
		t.Fatal(err)
	}
	if got := schemaString(leaf["source"]); got != "contract_identity" {
		t.Fatalf("source = %q, want contract_identity", got)
	}
	identity := schemaMap(leaf["field_provenance"])["canonical_path"]
	// Migrated ContractFinal tools stamp identity provenance as contract_final
	// pass-through while the collector remains the path index; accept either.
	switch identity["precedence"] {
	case "command_registry", "contract_final":
	default:
		t.Fatalf("canonical identity provenance = %#v", identity)
	}
	if identity["precedence"] == "command_registry" && identity["source"] != "contract_identity" {
		t.Fatalf("canonical identity provenance = %#v", identity)
	}
	parameters := schemaMap(leaf["parameters"])
	assertNativeAnnotation := func(flagName, field string) {
		t.Helper()
		provenance := schemaMap(parameters[flagName]["field_provenance"])
		winner := provenance[field]
		src, _ := winner["source"].(string)
		prec, _ := winner["precedence"].(string)
		if src != "native_annotation" || prec != "native_annotation" {
			t.Fatalf("%s.%s provenance = %#v", flagName, field, winner)
		}
	}
	name := parameters["name"]
	if name["property"] != "categoryName" || name["required"] != true {
		t.Fatalf("name parameter = %#v", name)
	}
	assertNativeAnnotation("name", "property")
	assertNativeAnnotation("name", "required")
	for flagName, property := range map[string]string{
		"keywords": "groupNameKeywords",
		"members":  "memberOpenDingTalkIds",
	} {
		parameter := parameters[flagName]
		if parameter["property"] != property || parameter["interface_type"] != "array" || parameter["required"] != false {
			t.Fatalf("%s parameter = %#v", flagName, parameter)
		}
		for _, field := range []string{"property", "interface_type", "required"} {
			assertNativeAnnotation(flagName, field)
		}
	}
}

func TestDeliveryCatalogMailMessageListLimitMapsToSize(t *testing.T) {
	// mail message list is a composite wrapper over search_emails and shares
	// that RPC with mail message search. list has no versioned binding entry
	// (composite identity), so clearing hints without a leaf-local contract.ParamDecl
	// previously fell back to flag_name_inference → "limit".
	listLeaf, err := queryDeliverySchemaPayload([]string{"mail message list"})
	if err != nil {
		t.Fatal(err)
	}
	listParams := schemaMap(listLeaf["parameters"])
	listLimit := listParams["limit"]
	if listLimit["property"] != "size" {
		t.Fatalf("mail message list --limit property = %#v, want size", listLimit["property"])
	}
	listProv := schemaMap(listLimit["field_provenance"])["property"]
	if src, _ := listProv["source"].(string); src != "native_annotation" {
		t.Fatalf("mail message list --limit property source = %#v, want native_annotation", listProv)
	}
	if _, ok := listParams["folder-id"]; !ok {
		t.Fatal("mail message list must keep --folder-id on its composite surface")
	}
	if _, ok := listParams["query"]; ok {
		t.Fatal("mail message list must not publish search-only --query")
	}

	// Sibling search leaf maps the same --limit → size via binding + contract.ParamDecl
	// and must keep the free-form KQL surface (not fold into list).
	searchLeaf, err := queryDeliverySchemaPayload([]string{"mail message search"})
	if err != nil {
		t.Fatal(err)
	}
	searchParams := schemaMap(searchLeaf["parameters"])
	searchLimit := searchParams["limit"]
	if searchLimit["property"] != "size" {
		t.Fatalf("mail message search --limit property = %#v, want size", searchLimit["property"])
	}
	searchProv := schemaMap(searchLimit["field_provenance"])["property"]
	src, _ := searchProv["source"].(string)
	if src != "native_annotation" && src != "versioned_parameter_binding" {
		t.Fatalf("mail message search --limit property source = %#v, want native_annotation or versioned_parameter_binding", searchProv)
	}
	if _, ok := searchParams["query"]; !ok {
		t.Fatal("mail message search must keep --query")
	}
	if _, ok := searchParams["folder-id"]; ok {
		t.Fatal("mail message search must not publish list-only --folder-id")
	}
}

func TestDeliveryCatalogSharedRenameDocumentParamDeclsStayOnDriveLeaf(t *testing.T) {
	// drive rename and doc rename share RPC rename_document. The reviewed
	// extension-stripping description must stay on drive rename only.
	driveLeaf, err := queryDeliverySchemaPayload([]string{"drive rename"})
	if err != nil {
		t.Fatal(err)
	}
	driveName := schemaMap(driveLeaf["parameters"])["name"]
	driveDesc, _ := driveName["description"].(string)
	if driveDesc == "" || !strings.Contains(driveDesc, "扩展名") {
		t.Fatalf("drive rename --name description = %q, want reviewed extension-stripping prose", driveDesc)
	}
	driveDescProv := schemaMap(driveName["field_provenance"])["description"]
	if src, _ := driveDescProv["source"].(string); src != "native_annotation" {
		t.Fatalf("drive rename --name description source = %#v, want native_annotation", driveDescProv)
	}

	docLeaf, err := queryDeliverySchemaPayload([]string{"doc rename"})
	if err != nil {
		t.Fatal(err)
	}
	docName := schemaMap(docLeaf["parameters"])["name"]
	docDesc, _ := docName["description"].(string)
	// Cobra usage may mention 扩展名 when pointing users at drive rename; the
	// failure mode is inheriting the drive-only reviewed overlay prose.
	if strings.Contains(docDesc, "避免双扩展名") || strings.Contains(docDesc, "仅对非文件夹") {
		t.Fatalf("doc rename --name description incorrectly inherited drive rename prose: %q", docDesc)
	}
	docDescProv := schemaMap(docName["field_provenance"])["description"]
	if src, _ := docDescProv["source"].(string); src == "native_annotation" && strings.Contains(docDesc, "读取节点类型") {
		t.Fatalf("doc rename --name description source/value look like a misplaced drive contract.ParamDecl: %#v", docDescProv)
	}
}

func TestDeliveryCatalogDriveUploadCompositeNodeParamDecl(t *testing.T) {
	// drive.upload is a composite multi-step leaf (no single RPCName /
	// interface_ref). The reviewed --node → nodeId overlay must stay on the
	// upload leaf via contract.ParamDecl, not on the manual upload-info / commit steps.
	leaf, err := queryDeliverySchemaPayload([]string{"drive upload"})
	if err != nil {
		t.Fatal(err)
	}
	if got := schemaString(leaf["interface_mode"]); got != "composite" {
		t.Fatalf("drive upload interface_mode = %q, want composite", got)
	}
	if leaf["interface_ref"] != nil {
		t.Fatalf("drive upload interface_ref = %#v, want nil for multi-step composite", leaf["interface_ref"])
	}
	parameters := schemaMap(leaf["parameters"])
	node := parameters["node"]
	if node["property"] != "nodeId" || node["required"] != false {
		t.Fatalf("drive upload --node = %#v, want property=nodeId required=false", node)
	}
	for _, field := range []string{"property", "required"} {
		provenance := schemaMap(node["field_provenance"])[field]
		if src, _ := provenance["source"].(string); src != "native_annotation" {
			t.Fatalf("drive upload --node %s source = %#v, want native_annotation", field, provenance)
		}
	}

	for _, path := range []string{"drive upload-info", "drive commit"} {
		sibling, err := queryDeliverySchemaPayload([]string{path})
		if err != nil {
			t.Fatal(err)
		}
		siblingParams := schemaMap(sibling["parameters"])
		if _, ok := siblingParams["node"]; ok {
			t.Fatalf("%s unexpectedly publishes --node (upload overwrite contract.ParamDecl leaked)", path)
		}
		if got := schemaString(sibling["interface_mode"]); got != "mcp" {
			t.Fatalf("%s interface_mode = %q, want mcp", path, got)
		}
	}
}

func TestDeliveryCatalogDocReadParamDeclsMatchMergeBaseContract(t *testing.T) {
	// Former schema_hints overlays for get_document_content must stay on the
	// doc read leaf after contract.ParamDecl migration (87910880 value parity).
	leaf, err := queryDeliverySchemaPayload([]string{"doc read"})
	if err != nil {
		t.Fatal(err)
	}
	parameters := schemaMap(leaf["parameters"])

	contentFormat := parameters["content-format"]
	if contentFormat["property"] != "format" {
		t.Fatalf("doc read --content-format property = %#v, want format", contentFormat["property"])
	}
	if contentFormat["required"] != false {
		t.Fatalf("doc read --content-format required = %#v, want false", contentFormat["required"])
	}

	for _, flagName := range []string{"scope", "tags", "max-depth", "start-block-id", "end-block-id", "version", "password"} {
		if parameters[flagName]["required"] != false {
			t.Fatalf("doc read --%s required = %#v, want false", flagName, parameters[flagName]["required"])
		}
	}
	if got := parameters["tags"]["required_when"]; got != "--scope=tags" {
		t.Fatalf("doc read --tags required_when = %#v", got)
	}
	if got := parameters["start-block-id"]["required_when"]; got != "--scope=range or --scope=section" {
		t.Fatalf("doc read --start-block-id required_when = %#v", got)
	}
	if parameters["max-depth"]["type"] != "integer" {
		t.Fatalf("doc read --max-depth type = %#v, want integer", parameters["max-depth"]["type"])
	}
	if got := parameters["version"]["property"]; got != "historyVersion" {
		t.Fatalf("doc read --version property = %#v, want historyVersion", got)
	}
	if parameters["version"]["type"] != "integer" {
		t.Fatalf("doc read --version type = %#v, want integer", parameters["version"]["type"])
	}
	if got := parameters["password"]["property"]; got != "password" {
		t.Fatalf("doc read --password property = %#v, want password", got)
	}
}

func TestDeliveryCatalogDocCommentParamDeclsMatchMergeBaseContract(t *testing.T) {
	// create/reply/update mentioned-open-conversation-id overlays + update
	// mention interface_type must survive hint clearance.
	for _, path := range []string{"doc comment create", "doc comment reply"} {
		leaf, err := queryDeliverySchemaPayload([]string{path})
		if err != nil {
			t.Fatal(err)
		}
		param := schemaMap(leaf["parameters"])["mentioned-open-conversation-id"]
		if param["required"] != false {
			t.Fatalf("%s --mentioned-open-conversation-id required = %#v, want false", path, param["required"])
		}
		if param["type"] != "array" {
			t.Fatalf("%s --mentioned-open-conversation-id type = %#v, want array", path, param["type"])
		}
		prov := schemaMap(param["field_provenance"])["required"]
		if src, _ := prov["source"].(string); src != "native_annotation" {
			t.Fatalf("%s --mentioned-open-conversation-id required source = %#v, want native_annotation", path, prov)
		}
	}

	updateLeaf, err := queryDeliverySchemaPayload([]string{"doc comment update"})
	if err != nil {
		t.Fatal(err)
	}
	updateParams := schemaMap(updateLeaf["parameters"])
	mention := updateParams["mention"]
	if mention["interface_type"] != "array" {
		t.Fatalf("doc comment update --mention interface_type = %#v, want array", mention["interface_type"])
	}
	mentionTypeProv := schemaMap(mention["field_provenance"])["interface_type"]
	if src, _ := mentionTypeProv["source"].(string); src != "native_annotation" {
		t.Fatalf("doc comment update --mention interface_type source = %#v, want native_annotation", mentionTypeProv)
	}
	groupMention := updateParams["mentioned-open-conversation-id"]
	if groupMention["property"] != "mentionedOpenConversationIds" || groupMention["required"] != false {
		t.Fatalf("doc comment update --mentioned-open-conversation-id = %#v", groupMention)
	}
	groupPropProv := schemaMap(groupMention["field_provenance"])["property"]
	if src, _ := groupPropProv["source"].(string); src != "native_annotation" {
		t.Fatalf("doc comment update --mentioned-open-conversation-id property source = %#v, want native_annotation", groupPropProv)
	}
}

func TestDeliveryCatalogDocRenameKeepsPassthroughNameDescription(t *testing.T) {
	// Shared-RPC sibling of drive rename: doc must keep the merge-base
	// passthrough usage, not the drive extension-stripping contract.ParamDecl.
	leaf, err := queryDeliverySchemaPayload([]string{"doc rename"})
	if err != nil {
		t.Fatal(err)
	}
	name := schemaMap(leaf["parameters"])["name"]
	desc, _ := name["description"].(string)
	if !strings.Contains(desc, "原样传给服务端") {
		t.Fatalf("doc rename --name description = %q, want passthrough Cobra usage", desc)
	}
	if strings.Contains(desc, "避免双扩展名") || strings.Contains(desc, "仅对非文件夹") {
		t.Fatalf("doc rename --name description incorrectly inherited drive contract.ParamDecl: %q", desc)
	}
	descProv := schemaMap(name["field_provenance"])["description"]
	if src, _ := descProv["source"].(string); src != "cobra_usage" {
		t.Fatalf("doc rename --name description source = %#v, want cobra_usage", descProv)
	}
}

func TestDeliveryCatalogDevdocQueryRemainsOptionalInRequireOneOf(t *testing.T) {
	// Former hint required=false keeps query optional; positional keyword is
	// the other require_one_of member (constraint may outrank the annotation).
	leaf, err := queryDeliverySchemaPayload([]string{"devdoc article search"})
	if err != nil {
		t.Fatal(err)
	}
	query := schemaMap(leaf["parameters"])["query"]
	if query["required"] != false {
		t.Fatalf("devdoc article search --query required = %#v, want false", query["required"])
	}
	prov := schemaMap(query["field_provenance"])["required"]
	src, _ := prov["source"].(string)
	if src != "native_annotation" && src != "require_one_of_constraint" {
		t.Fatalf("devdoc article search --query required source = %#v, want native_annotation or require_one_of_constraint", prov)
	}
	constraints, ok := leaf["constraints"].(map[string]any)
	if !ok {
		t.Fatalf("constraints = %#v", leaf["constraints"])
	}
	groups, _ := constraints["require_one_of"].([]any)
	found := false
	for _, raw := range groups {
		group, _ := raw.([]any)
		names := make([]string, 0, len(group))
		for _, item := range group {
			if s, ok := item.(string); ok {
				names = append(names, s)
			}
		}
		if len(names) == 2 && ((names[0] == "query" && names[1] == "keyword") || (names[0] == "keyword" && names[1] == "query")) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("devdoc article search require_one_of missing [query,keyword]: %#v", groups)
	}
}

func TestDeliveryCatalogModelsAitableExportBranches(t *testing.T) {
	leaf, err := queryDeliverySchemaPayload([]string{"aitable export data"})
	if err != nil {
		t.Fatal(err)
	}
	parameters := schemaMap(leaf["parameters"])
	if _, exists := parameters["format"]; exists {
		t.Fatal("business export format still shadows the global --format output flag")
	}
	exportFormat := parameters["export-format"]
	if exportFormat["property"] != "format" || exportFormat["required"] != false {
		t.Fatalf("export-format parameter = %#v", exportFormat)
	}
	if got, want := schemaStringSlice(exportFormat["enum"]), []string{"excel", "attachment", "excel_and_attachment", "excel_with_inline_images"}; !equalStringSlices(got, want) {
		t.Fatalf("export-format enum = %v, want %v", got, want)
	}
	if parameters["scope"]["required"] != false {
		t.Fatalf("scope must be conditional, got %#v", parameters["scope"])
	}
	if got := parameters["table-id"]["required_when"]; got != "scope is table or view" {
		t.Fatalf("table-id required_when = %#v", got)
	}
	if got := parameters["view-id"]["required_when"]; got != "scope is view" {
		t.Fatalf("view-id required_when = %#v", got)
	}

	constraints, ok := leaf["constraints"].(map[string]any)
	if !ok {
		t.Fatalf("constraints = %#v", leaf["constraints"])
	}
	hasGroup := func(field string, want ...string) bool {
		groups, _ := constraints[field].([]any)
		for _, raw := range groups {
			if equalStringSlices(schemaStringSlice(raw), want) {
				return true
			}
		}
		return false
	}
	if !hasGroup("require_one_of", "scope", "task-id") ||
		!hasGroup("require_one_of", "export-format", "task-id") ||
		!hasGroup("require_together", "scope", "export-format") {
		t.Fatalf("branch constraints = %#v", constraints)
	}
}

func TestDeliveryCatalogKeepsSharedFlagSemanticsCommandScoped(t *testing.T) {
	queryLeaf, err := queryDeliverySchemaPayload([]string{"aitable record query"})
	if err != nil {
		t.Fatal(err)
	}
	getLeaf, err := queryDeliverySchemaPayload([]string{"aitable record get"})
	if err != nil {
		t.Fatal(err)
	}
	queryRecordIDs := schemaMap(queryLeaf["parameters"])["record-ids"]
	getRecordIDs := schemaMap(getLeaf["parameters"])["record-ids"]
	if queryRecordIDs["required"] != false {
		t.Fatalf("record query --record-ids = %#v, want optional", queryRecordIDs)
	}
	if getRecordIDs["required"] != true {
		t.Fatalf("record get --record-ids = %#v, want required", getRecordIDs)
	}
	getProvenance := schemaMap(getRecordIDs["field_provenance"])["required"]
	if getProvenance["source"] != "typed_parameter_metadata" {
		t.Fatalf("record get required provenance = %#v", getProvenance)
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestStripSchemaPayloadCompactLeaf(t *testing.T) {
	leaf, err := queryDeliverySchemaPayload([]string{"calendar event create"})
	if err != nil {
		t.Fatal(err)
	}
	leaf["future_audit_field"] = "must not leak into Agent view"
	for _, raw := range schemaMap(leaf["parameters"]) {
		raw["future_mapping_field"] = "must not leak into Agent view"
	}
	stripped := stripSchemaPayloadCompact(leaf)

	// Must keep agent-essential fields.
	for _, key := range []string{"cli_path", "canonical_path", "description", "effect", "risk", "confirmation", "parameters", "constraints"} {
		if _, ok := stripped[key]; !ok {
			t.Fatalf("compact leaf missing essential key %q", key)
		}
	}

	// Must strip provenance / redundant fields.
	for _, key := range []string{"agent_metadata_source", "agent_source_refs", "agent_summary_source", "effect_source", "metadata_source", "primary_cli_path", "parameter_count", "has_parameters", "interface_ref", "source", "title", "display", "future_audit_field"} {
		if _, ok := stripped[key]; ok {
			t.Fatalf("compact leaf still contains stripped key %q", key)
		}
	}

	// Parameters must not contain interface_description / property.
	if params, ok := stripped["parameters"].(map[string]any); ok {
		for name, p := range params {
			if pm, ok := p.(map[string]any); ok {
				for _, stripped := range []string{"interface_description", "interface_type", "property", "future_mapping_field"} {
					if _, present := pm[stripped]; present {
						t.Fatalf("compact param %q still contains %q", name, stripped)
					}
				}
				// Must keep type and required.
				if _, present := pm["type"]; !present {
					t.Fatalf("compact param %q missing type", name)
				}
			}
		}
	}
}

func TestStripSchemaPayloadCompactPreservesParameterIdentity(t *testing.T) {
	leaf, err := queryDeliverySchemaPayload([]string{"chat category create-smart"})
	if err != nil {
		t.Fatal(err)
	}
	full := schemaMap(leaf["parameters"])
	compact := schemaMap(stripSchemaPayloadCompact(leaf)["parameters"])
	if len(compact) != len(full) {
		t.Fatalf("compact parameter count = %d, want %d: full=%v compact=%v", len(compact), len(full), sortedSchemaKeys(full), sortedSchemaKeys(compact))
	}
	name := compact["name"]
	if name["required"] != true || name["type"] != "string" {
		t.Fatalf("compact --name parameter = %#v", name)
	}

	synthetic := map[string]any{"parameters": map[string]any{}}
	parameters := synthetic["parameters"].(map[string]any)
	for _, parameterName := range []string{"name", "path", "source", "title", "group", "aliases"} {
		parameters[parameterName] = map[string]any{"type": "string", "required": false, "field_provenance": map[string]any{"source": "test"}}
	}
	stripped := schemaMap(stripSchemaPayloadCompact(synthetic)["parameters"])
	for parameterName := range parameters {
		if _, ok := stripped[parameterName]; !ok {
			t.Errorf("compact projection dropped parameter identity %q", parameterName)
		}
	}
}

func sortedSchemaKeys(values map[string]map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestStripSchemaPayloadCompactOverview(t *testing.T) {
	overview, err := deliverySchemaOverviewPayload()
	if err != nil {
		t.Fatal(err)
	}
	stripped := stripSchemaPayloadCompact(overview)

	// Overview must keep kind/level/count/products.
	for _, key := range []string{"kind", "level", "count", "products"} {
		if _, ok := stripped[key]; !ok {
			t.Fatalf("compact overview missing key %q", key)
		}
	}
	// Must strip agent_metadata at top level.
	for _, key := range []string{"agent_metadata", "source"} {
		if _, ok := stripped[key]; ok {
			t.Fatalf("compact overview still contains stripped key %q", key)
		}
	}
}

func TestStripSchemaPayloadCompactProduct(t *testing.T) {
	product, err := queryDeliverySchemaPayload([]string{"calendar"})
	if err != nil {
		t.Fatal(err)
	}
	stripped := stripSchemaPayloadCompact(product)

	if _, ok := stripped["product"]; !ok {
		t.Fatal("compact product missing 'product' key")
	}
	prod := stripped["product"].(map[string]any)
	for _, key := range []string{"agent_metadata_source", "agent_source_refs", "source"} {
		if _, ok := prod[key]; ok {
			t.Fatalf("compact product still contains stripped key %q", key)
		}
	}
}

func TestDeliveryCatalogHrbrainUnpinnedParamDeclsKeepExplicitMappings(t *testing.T) {
	// hrbrain leaves are Reviewed unpinned remote adapters. Clearing hint
	// overlays must keep non-inferable properties on native_annotation
	// (page→currentPage, order-by→orderByClauses), not flag_name_inference.
	listLeaf, err := queryDeliverySchemaPayload([]string{"hrbrain talent-pool list"})
	if err != nil {
		t.Fatal(err)
	}
	listParams := schemaMap(listLeaf["parameters"])
	page := listParams["page"]
	if page["property"] != "currentPage" || page["required"] != false {
		t.Fatalf("hrbrain talent-pool list --page = %#v, want property=currentPage required=false", page)
	}
	pageProv := schemaMap(page["field_provenance"])["property"]
	if src, _ := pageProv["source"].(string); src != "native_annotation" {
		t.Fatalf("hrbrain talent-pool list --page property source = %#v, want native_annotation", pageProv)
	}

	structuredLeaf, err := queryDeliverySchemaPayload([]string{"hrbrain search employees-structured"})
	if err != nil {
		t.Fatal(err)
	}
	structuredParams := schemaMap(structuredLeaf["parameters"])
	orderBy := structuredParams["order-by"]
	if orderBy["property"] != "orderByClauses" || orderBy["required"] != false {
		t.Fatalf("hrbrain search employees-structured --order-by = %#v, want property=orderByClauses required=false", orderBy)
	}
	orderByProv := schemaMap(orderBy["field_provenance"])["property"]
	if src, _ := orderByProv["source"].(string); src != "native_annotation" {
		t.Fatalf("hrbrain search employees-structured --order-by property source = %#v, want native_annotation", orderByProv)
	}
}

func TestDeliveryCatalogAttendanceSearchRequiredStaysNativeWithoutProperty(t *testing.T) {
	// attendance * search overlays only reviewed required=false (no property).
	// After clearing hints, Required must stay native_annotation and property
	// must remain the reviewed mapping exclusion (empty), not inference.
	for _, path := range []string{
		"attendance adjustment search",
		"attendance group search",
		"attendance overtime search",
	} {
		leaf, err := queryDeliverySchemaPayload([]string{path})
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		parameters := schemaMap(leaf["parameters"])
		for _, flagName := range []string{"page", "limit"} {
			param := parameters[flagName]
			if param["required"] != false {
				t.Fatalf("%s --%s required = %#v, want false", path, flagName, param["required"])
			}
			if param["property"] != nil && param["property"] != "" {
				t.Fatalf("%s --%s property = %#v, want omitted/empty mapping exclusion", path, flagName, param["property"])
			}
			reqProv := schemaMap(param["field_provenance"])["required"]
			if src, _ := reqProv["source"].(string); src != "native_annotation" {
				t.Fatalf("%s --%s required source = %#v, want native_annotation", path, flagName, reqProv)
			}
			propProv := schemaMap(param["field_provenance"])["property"]
			if src, _ := propProv["source"].(string); src != "reviewed_mapping_exclusion" {
				t.Fatalf("%s --%s property source = %#v, want reviewed_mapping_exclusion", path, flagName, propProv)
			}
		}
	}
}

func TestDeliveryCatalogMarkdownFetchOmitsHiddenIDAlias(t *testing.T) {
	// markdown fetch keeps a hidden --id Cobra compat alias, but Schema must
	// publish only --node (merge-base / 87910880 catalog parity).
	leaf, err := queryDeliverySchemaPayload([]string{"markdown fetch"})
	if err != nil {
		t.Fatal(err)
	}
	parameters := schemaMap(leaf["parameters"])
	if _, ok := parameters["id"]; ok {
		t.Fatal("markdown fetch must not publish hidden --id in Schema parameters")
	}
	node := parameters["node"]
	if node["property"] != "nodeId" || node["required"] != true {
		t.Fatalf("markdown fetch --node = %#v, want property=nodeId required=true", node)
	}
	prov := schemaMap(node["field_provenance"])["property"]
	if src, _ := prov["source"].(string); src != "native_annotation" {
		t.Fatalf("markdown fetch --node property source = %#v, want native_annotation", prov)
	}
}

func TestDeliveryCatalogMarkdownPatchContentMapsToReplacement(t *testing.T) {
	// --content is the CLI flag; MCP/workflow property is replacement.
	leaf, err := queryDeliverySchemaPayload([]string{"markdown patch"})
	if err != nil {
		t.Fatal(err)
	}
	content := schemaMap(leaf["parameters"])["content"]
	if content["property"] != "replacement" || content["required"] != true {
		t.Fatalf("markdown patch --content = %#v, want property=replacement required=true", content)
	}
	prov := schemaMap(content["field_provenance"])["property"]
	if src, _ := prov["source"].(string); src != "native_annotation" {
		t.Fatalf("markdown patch --content property source = %#v, want native_annotation", prov)
	}
}

func TestDeliveryCatalogMinutesReplaceBatchPairDescriptionIsNative(t *testing.T) {
	// Former metadata overlay on minutes +replace-batch --pair description.
	leaf, err := queryDeliverySchemaPayload([]string{"minutes +replace-batch"})
	if err != nil {
		t.Fatal(err)
	}
	pair := schemaMap(leaf["parameters"])["pair"]
	desc, _ := pair["description"].(string)
	if desc == "" || !strings.Contains(desc, "原文=>替换") {
		t.Fatalf("minutes +replace-batch --pair description = %q, want reviewed pair-format prose", desc)
	}
	prov := schemaMap(pair["field_provenance"])["description"]
	if src, _ := prov["source"].(string); src != "native_annotation" {
		t.Fatalf("minutes +replace-batch --pair description source = %#v, want native_annotation", prov)
	}
}

func TestDeliveryCatalogTodoParamDeclsFrom87910880Reviewed(t *testing.T) {
	// Spot-check former metadata/todo.json overlays now declared as contract.ParamDecl (87910880).
	// --query-all is a Cobra Bool: merge-base catalog publishes type=boolean from
	// cobra_flag_type (no separate interface_type field), while required/description
	// stay native_annotation after contract.ParamDecl migration.
	listLeaf, err := queryDeliverySchemaPayload([]string{"todo task list"})
	if err != nil {
		t.Fatal(err)
	}
	listParams := schemaMap(listLeaf["parameters"])
	queryAll := listParams["query-all"]
	if queryAll["required"] != false || queryAll["type"] != "boolean" {
		t.Fatalf("todo task list --query-all = %#v, want required=false type=boolean", queryAll)
	}
	if desc, _ := queryAll["description"].(string); !strings.Contains(desc, "跨组织") {
		t.Fatalf("todo task list --query-all description = %q, want cross-org prose", desc)
	}
	for _, field := range []string{"required", "description"} {
		prov := schemaMap(queryAll["field_provenance"])[field]
		if src, _ := prov["source"].(string); src != "native_annotation" {
			t.Fatalf("todo task list --query-all %s source = %#v, want native_annotation", field, prov)
		}
	}
	typeProv := schemaMap(queryAll["field_provenance"])["type"]
	if src, _ := typeProv["source"].(string); src != "cobra_flag_type" {
		t.Fatalf("todo task list --query-all type source = %#v, want cobra_flag_type", typeProv)
	}
	roleTypes := listParams["role-types"]
	if roleTypes["property"] != "roleTypes" || roleTypes["required"] != false {
		t.Fatalf("todo task list --role-types = %#v, want property=roleTypes required=false", roleTypes)
	}
	rolePropProv := schemaMap(roleTypes["field_provenance"])["property"]
	if src, _ := rolePropProv["source"].(string); src != "native_annotation" {
		t.Fatalf("todo task list --role-types property source = %#v, want native_annotation", rolePropProv)
	}
	if desc, _ := roleTypes["description"].(string); !strings.Contains(desc, "executor") {
		t.Fatalf("todo task list --role-types description = %q, want executor default prose", desc)
	}
	roleDescProv := schemaMap(roleTypes["field_provenance"])["description"]
	if src, _ := roleDescProv["source"].(string); src != "native_annotation" {
		t.Fatalf("todo task list --role-types description source = %#v, want native_annotation", roleDescProv)
	}

	resetLeaf, err := queryDeliverySchemaPayload([]string{"todo task reset-reminder"})
	if err != nil {
		t.Fatal(err)
	}
	rules := schemaMap(resetLeaf["parameters"])["reminder-rules"]
	desc, _ := rules["description"].(string)
	if !strings.Contains(desc, "不传表示清除") {
		t.Fatalf("todo task reset-reminder --reminder-rules description = %q", desc)
	}
	rulesDescProv := schemaMap(rules["field_provenance"])["description"]
	if src, _ := rulesDescProv["source"].(string); src != "native_annotation" {
		t.Fatalf("todo task reset-reminder --reminder-rules description source = %#v, want native_annotation", rulesDescProv)
	}

	createTag, err := queryDeliverySchemaPayload([]string{"todo tag create"})
	if err != nil {
		t.Fatal(err)
	}
	name := schemaMap(createTag["parameters"])["name"]
	if name["property"] != "name" || name["required"] != true {
		t.Fatalf("todo tag create --name = %#v, want property=name required=true", name)
	}

	deleteTag, err := queryDeliverySchemaPayload([]string{"todo tag delete"})
	if err != nil {
		t.Fatal(err)
	}
	deleteCodes := schemaMap(deleteTag["parameters"])["tag-codes"]
	if deleteCodes["property"] != "tagCodes" || deleteCodes["interface_type"] != "array" || deleteCodes["required"] != true {
		t.Fatalf("todo tag delete --tag-codes = %#v", deleteCodes)
	}

	addTag, err := queryDeliverySchemaPayload([]string{"todo tag add"})
	if err != nil {
		t.Fatal(err)
	}
	addParams := schemaMap(addTag["parameters"])
	if addParams["task-id"]["property"] != "taskId" || addParams["task-id"]["required"] != true {
		t.Fatalf("todo tag add --task-id = %#v", addParams["task-id"])
	}
	if addParams["tag-codes"]["property"] != "tagCodes" || addParams["tag-codes"]["interface_type"] != "array" || addParams["tag-codes"]["required"] != true {
		t.Fatalf("todo tag add --tag-codes = %#v", addParams["tag-codes"])
	}

	updateTag, err := queryDeliverySchemaPayload([]string{"todo tag update"})
	if err != nil {
		t.Fatal(err)
	}
	userTags := schemaMap(updateTag["parameters"])["user-tags"]
	if userTags["property"] != "userTags" || userTags["interface_type"] != "array" || userTags["required"] != true {
		t.Fatalf("todo tag update --user-tags = %#v", userTags)
	}
	for _, tc := range []struct {
		path  string
		flag  string
		field string
	}{
		{"todo tag create", "name", "property"},
		{"todo tag delete", "tag-codes", "interface_type"},
		{"todo tag add", "task-id", "property"},
		{"todo tag update", "user-tags", "property"},
	} {
		leaf, err := queryDeliverySchemaPayload([]string{tc.path})
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		prov := schemaMap(schemaMap(leaf["parameters"])[tc.flag]["field_provenance"])[tc.field]
		if src, _ := prov["source"].(string); src != "native_annotation" {
			t.Fatalf("%s --%s %s source = %#v, want native_annotation", tc.path, tc.flag, tc.field, prov)
		}
	}
}

func TestDeliveryCatalogSheetParamDeclsFrom87910880Reviewed(t *testing.T) {
	// Former metadata/sheet.json overlays now declared as contract.ParamDecl (87910880).
	for _, tc := range []struct {
		path string
		flag string
		want string
	}{
		{"sheet pivot-table create", "properties", "object"},
		{"sheet pivot-table update", "properties", "object"},
		{"sheet table-put", "sheets", "array"},
	} {
		leaf, err := queryDeliverySchemaPayload([]string{tc.path})
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		param := schemaMap(leaf["parameters"])[tc.flag]
		if param["interface_type"] != tc.want {
			t.Fatalf("%s --%s interface_type = %#v, want %q", tc.path, tc.flag, param["interface_type"], tc.want)
		}
		prov := schemaMap(param["field_provenance"])["interface_type"]
		if src, _ := prov["source"].(string); src != "native_annotation" {
			t.Fatalf("%s --%s interface_type source = %#v, want native_annotation", tc.path, tc.flag, prov)
		}
	}
}

func TestDeliveryCatalogPatSheetWikiParamDecls(t *testing.T) {
	// pat/sheet/wiki required and required_when pins publish via ParamDecl.
	for _, tc := range []struct {
		path         string
		flag         string
		wantRequired bool
		requiredWhen string
	}{
		{"pat chmod", "session-id", false, "grant-type is session"},
		{"sheet create", "name", true, ""},
		{"sheet list", "node", true, ""},
		{"sheet new", "node", true, ""},
		{"sheet new", "name", true, ""},
		{"wiki space create", "name", true, ""},
		{"wiki space get", "workspace", true, ""},
	} {
		leaf, err := queryDeliverySchemaPayload([]string{tc.path})
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		param := schemaMap(leaf["parameters"])[tc.flag]
		if param["required"] != tc.wantRequired {
			t.Fatalf("%s --%s required = %#v, want %v", tc.path, tc.flag, param["required"], tc.wantRequired)
		}
		reqProv := schemaMap(param["field_provenance"])["required"]
		if got, _ := reqProv["source"].(string); got != "native_annotation" {
			t.Fatalf("%s --%s required source = %#v, want native_annotation", tc.path, tc.flag, reqProv)
		}
		if tc.requiredWhen == "" {
			continue
		}
		if param["required_when"] != tc.requiredWhen {
			t.Fatalf("%s --%s required_when = %#v, want %q", tc.path, tc.flag, param["required_when"], tc.requiredWhen)
		}
		whenProv := schemaMap(param["field_provenance"])["required_when"]
		if got, _ := whenProv["source"].(string); got != "native_annotation" {
			t.Fatalf("%s --%s required_when source = %#v, want native_annotation", tc.path, tc.flag, whenProv)
		}
	}
}

func TestDeliveryCatalogReportCreateToChatRequiredAndAliasView(t *testing.T) {
	// Former report.create_report overlay: --to-chat required=false (MCP marks
	// it required). Alias "report create" must keep the same contract.
	primary, err := queryDeliverySchemaPayload([]string{"report entry submit"})
	if err != nil {
		t.Fatal(err)
	}
	if schemaString(primary["canonical_path"]) != "report.create_report" {
		t.Fatalf("report entry submit canonical = %q", schemaString(primary["canonical_path"]))
	}
	toChat := schemaMap(primary["parameters"])["to-chat"]
	if toChat["required"] != false {
		t.Fatalf("report entry submit --to-chat required = %#v, want false", toChat["required"])
	}
	reqProv := schemaMap(toChat["field_provenance"])["required"]
	if src, _ := reqProv["source"].(string); src != "native_annotation" {
		t.Fatalf("report entry submit --to-chat required source = %#v, want native_annotation", reqProv)
	}

	alias, err := queryDeliverySchemaPayload([]string{"report create"})
	if err != nil {
		t.Fatal(err)
	}
	if alias["is_alias"] != true || schemaString(alias["cli_path"]) != "report create" {
		t.Fatalf("report create alias view = %#v", alias)
	}
	if schemaString(alias["canonical_path"]) != "report.create_report" {
		t.Fatalf("report create canonical = %q", schemaString(alias["canonical_path"]))
	}
	aliasToChat := schemaMap(alias["parameters"])["to-chat"]
	if aliasToChat["required"] != false {
		t.Fatalf("report create --to-chat required = %#v, want false (same as primary)", aliasToChat["required"])
	}
	aliasReqProv := schemaMap(aliasToChat["field_provenance"])["required"]
	if src, _ := aliasReqProv["source"].(string); src != "native_annotation" {
		t.Fatalf("report create --to-chat required source = %#v, want native_annotation", aliasReqProv)
	}
}

func TestDeliveryCatalogAitableParamDeclsMatchMergeBaseContract(t *testing.T) {
	// Former schema_hints overlays (87910880) must stay on helpers leaves via
	// contract.ParamDecl after hint clearance — not on aitable +shortcuts.
	queryLeaf, err := queryDeliverySchemaPayload([]string{"aitable record query"})
	if err != nil {
		t.Fatal(err)
	}
	queryRecordIDs := schemaMap(queryLeaf["parameters"])["record-ids"]
	if queryRecordIDs["required"] != false {
		t.Fatalf("aitable record query --record-ids required = %#v, want false", queryRecordIDs["required"])
	}
	queryReqProv := schemaMap(queryRecordIDs["field_provenance"])["required"]
	if src, _ := queryReqProv["source"].(string); src != "native_annotation" {
		t.Fatalf("aitable record query --record-ids required source = %#v, want native_annotation", queryReqProv)
	}

	// Sibling atomic get keeps Cobra-required --record-ids; shortcut has its
	// own optional StringSlice surface and must not inherit helpers contract.ParamDecl.
	getLeaf, err := queryDeliverySchemaPayload([]string{"aitable record get"})
	if err != nil {
		t.Fatal(err)
	}
	if schemaMap(getLeaf["parameters"])["record-ids"]["required"] != true {
		t.Fatalf("aitable record get --record-ids must stay required")
	}
	shortcutLeaf, err := queryDeliverySchemaPayload([]string{"aitable +record-query"})
	if err != nil {
		t.Fatal(err)
	}
	shortcutRecordIDs := schemaMap(shortcutLeaf["parameters"])["record-ids"]
	if shortcutRecordIDs["required"] != false {
		t.Fatalf("aitable +record-query --record-ids required = %#v, want false", shortcutRecordIDs["required"])
	}
	if shortcutRecordIDs["type"] != "array" {
		t.Fatalf("aitable +record-query --record-ids type = %#v, want array (shortcut StringSlice)", shortcutRecordIDs["type"])
	}

	for _, path := range []string{
		"aitable view update aggregate",
		"aitable view update card",
		"aitable view update field-widths",
		"aitable view update timebar",
	} {
		leaf, err := queryDeliverySchemaPayload([]string{path})
		if err != nil {
			t.Fatal(err)
		}
		jsonParam := schemaMap(leaf["parameters"])["json"]
		if jsonParam["required"] != false {
			t.Fatalf("%s --json required = %#v, want false", path, jsonParam["required"])
		}
		reqProv := schemaMap(jsonParam["field_provenance"])["required"]
		src, _ := reqProv["source"].(string)
		if src != "native_annotation" && src != "require_one_of_constraint" {
			t.Fatalf("%s --json required source = %#v, want native_annotation or require_one_of_constraint", path, reqProv)
		}
	}

	for _, tc := range []struct {
		path string
		flag string
		prop string
	}{
		{"aitable workflow create", "base-id", "baseId"},
		{"aitable workflow create", "dsl", "dsl"},
		{"aitable workflow create", "locale", "locale"},
		{"aitable workflow update", "base-id", "baseId"},
		{"aitable workflow update", "workflow-id", "workflowId"},
		{"aitable workflow update", "dsl", "dsl"},
		{"aitable workflow update", "locale", "locale"},
	} {
		leaf, err := queryDeliverySchemaPayload([]string{tc.path})
		if err != nil {
			t.Fatal(err)
		}
		param := schemaMap(leaf["parameters"])[tc.flag]
		if param["property"] != tc.prop {
			t.Fatalf("%s --%s property = %#v, want %s", tc.path, tc.flag, param["property"], tc.prop)
		}
		propProv := schemaMap(param["field_provenance"])["property"]
		if src, _ := propProv["source"].(string); src != "native_annotation" {
			t.Fatalf("%s --%s property source = %#v, want native_annotation", tc.path, tc.flag, propProv)
		}
	}

	createLeaf, err := queryDeliverySchemaPayload([]string{"aitable workflow create"})
	if err != nil {
		t.Fatal(err)
	}
	createParams := schemaMap(createLeaf["parameters"])
	for _, flagName := range []string{"base-id", "dsl"} {
		if createParams[flagName]["required"] != true {
			t.Fatalf("aitable workflow create --%s required = %#v, want true", flagName, createParams[flagName]["required"])
		}
	}
	if createParams["locale"]["required"] != false {
		t.Fatalf("aitable workflow create --locale required = %#v, want false", createParams["locale"]["required"])
	}
	dslTypeProv := schemaMap(createParams["dsl"]["field_provenance"])["interface_type"]
	if src, _ := dslTypeProv["source"].(string); src != "native_annotation" {
		t.Fatalf("aitable workflow create --dsl interface_type source = %#v, want native_annotation", dslTypeProv)
	}
}

func TestDeliveryCatalogAitableQueryKeywordAndParamDecls(t *testing.T) {
	// helpers record query: --query → keyword must not fall back to
	// flag_name_inference; JSON list flags keep wire interface_type via ParamDecl.
	queryLeaf, err := queryDeliverySchemaPayload([]string{"aitable record query"})
	if err != nil {
		t.Fatal(err)
	}
	queryParams := schemaMap(queryLeaf["parameters"])
	queryFlag := queryParams["query"]
	if queryFlag["property"] != "keyword" {
		t.Fatalf("aitable record query --query property = %#v, want keyword", queryFlag["property"])
	}
	queryPropProv := schemaMap(queryFlag["field_provenance"])["property"]
	src, _ := queryPropProv["source"].(string)
	if src != "native_annotation" && src != "versioned_parameter_binding" {
		t.Fatalf("aitable record query --query property source = %#v, want native_annotation or versioned_parameter_binding", queryPropProv)
	}
	for flagName, wantType := range map[string]string{
		"filters":    "object",
		"sort":       "array",
		"record-ids": "array",
		"field-ids":  "array",
	} {
		param := queryParams[flagName]
		if param["interface_type"] != wantType {
			t.Fatalf("aitable record query --%s interface_type = %#v, want %s", flagName, param["interface_type"], wantType)
		}
		typeProv := schemaMap(param["field_provenance"])["interface_type"]
		if got, _ := typeProv["source"].(string); got != "native_annotation" {
			t.Fatalf("aitable record query --%s interface_type source = %#v, want native_annotation", flagName, typeProv)
		}
	}

	// shortcut +record-query Execute maps --query → MCP keyword at runtime, but
	// merge-base Schema published property=query (flag_name_inference). Keep that
	// delivery for schema-compatibility; do not reintroduce contract.ParamDecl keyword.
	shortcutLeaf, err := queryDeliverySchemaPayload([]string{"aitable +record-query"})
	if err != nil {
		t.Fatal(err)
	}
	shortcutQuery := schemaMap(shortcutLeaf["parameters"])["query"]
	if shortcutQuery["property"] != "query" {
		t.Fatalf("aitable +record-query --query property = %#v, want query (merge-base)", shortcutQuery["property"])
	}
	if _, has := shortcutQuery["interface_type"]; has {
		t.Fatalf("aitable +record-query --query unexpectedly publishes interface_type: %#v", shortcutQuery["interface_type"])
	}
	for _, flagName := range []string{"filters", "sort"} {
		param := schemaMap(shortcutLeaf["parameters"])[flagName]
		if _, has := param["interface_type"]; has {
			t.Fatalf("aitable +record-query --%s unexpectedly publishes interface_type: %#v", flagName, param["interface_type"])
		}
	}

	// Required pins that match runtime validation.
	for _, tc := range []struct {
		path string
		flag string
	}{
		{"aitable attachment upload", "size"},
		{"aitable chart update", "config"},
		{"aitable record upsert", "records"},
		{"aitable field create", "fields"},
		{"aitable dashboard create", "name"},
		{"aitable dashboard update", "name"},
		{"aitable view update visible-fields", "field-ids"},
	} {
		leaf, err := queryDeliverySchemaPayload([]string{tc.path})
		if err != nil {
			t.Fatal(err)
		}
		param := schemaMap(leaf["parameters"])[tc.flag]
		if param["required"] != true {
			t.Fatalf("%s --%s required = %#v, want true", tc.path, tc.flag, param["required"])
		}
		reqProv := schemaMap(param["field_provenance"])["required"]
		if got, _ := reqProv["source"].(string); got != "native_annotation" {
			t.Fatalf("%s --%s required source = %#v, want native_annotation", tc.path, tc.flag, reqProv)
		}
	}
}

func TestDeliveryCatalogShortcutQueryKeepsMergeBaseProperty(t *testing.T) {
	// Composite shortcuts whose Execute maps --query → MCP keyword still publish
	// property=query at merge-base (flag_name_inference). Publishing keyword via
	// contract.ParamDecl breaks schema-compatibility; keep Schema delivery aligned.
	for _, path := range []string{
		"aitable +find-record",
		"doc +find-doc",
		"drive +find-file",
		"mail +find-mail-user",
		"chat +search-msg",
		"doc +search",
		"drive +search",
		"drive +search-docs",
		"minutes +list-mine",
		"minutes +list-shared",
		"minutes +list-all",
		"wiki +space-search",
		"contact +search-user",
		"chat +chat-search",
		"chat +bot-find",
	} {
		leaf, err := queryDeliverySchemaPayload([]string{path})
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		query := schemaMap(leaf["parameters"])["query"]
		if query["property"] != "query" {
			t.Fatalf("%s --query property = %#v, want query (merge-base)", path, query["property"])
		}
	}
}

func TestDeliveryCatalogContactParamDeclsMatchMergeBaseContract(t *testing.T) {
	// Former metadata/contact.json overlays at 87910880 must stay on the
	// published primary flags after contract.ParamDecl migration. Hidden Cobra
	// aliases (--id/--userid/--dept-name/--super-dept*) must not appear.
	type wantParam struct {
		property      string
		required      bool
		interfaceType string // empty → do not assert
	}
	cases := []struct {
		path   string
		params map[string]wantParam
		absent []string
	}{
		{
			path: "contact user invite",
			params: map[string]wantParam{
				"org-user-name":   {property: "orgUserName", required: true},
				"org-user-mobile": {property: "orgUserMobile", required: true},
				"depts":           {property: "depts", required: false, interfaceType: "array"},
			},
		},
		{
			path: "contact user update",
			params: map[string]wantParam{
				"user-id":        {property: "userId", required: true},
				"org-user-name":  {property: "orgUserName", required: false},
				"depts":          {property: "depts", required: false, interfaceType: "array"},
				"master-user-id": {property: "masterUserId", required: false},
			},
			absent: []string{"id", "userid"},
		},
		{
			path: "contact user update-self",
			params: map[string]wantParam{
				"nick":           {property: "nick", required: false},
				"avatar-file-id": {property: "avatarFileId", required: false},
			},
		},
		{
			path: "contact user profile get",
			params: map[string]wantParam{
				"fields":   {property: "fieldCodeList", required: false},
				"staff-id": {property: "staffId", required: false},
			},
		},
		{
			path: "contact dept create",
			params: map[string]wantParam{
				"name":              {property: "deptName", required: true},
				"parent":            {property: "superDeptId", required: false, interfaceType: "integer"},
				"create-dept-group": {property: "createDeptGroup", required: true},
			},
			absent: []string{"dept-name", "super-dept", "super-dept-id"},
		},
		{
			path: "contact label create",
			params: map[string]wantParam{
				"name":      {property: "labelModel.name", required: true},
				"type":      {property: "type", required: true},
				"parent-id": {property: "parentId", required: false, interfaceType: "integer"},
			},
			absent: []string{"label-name", "create-type", "label-type", "parentId", "parent", "label-parent-id", "labelParentId"},
		},
		{
			path: "contact label update",
			params: map[string]wantParam{
				"id":   {property: "labelId", required: true, interfaceType: "integer"},
				"name": {property: "label.name", required: true},
			},
			absent: []string{"label-id", "role-id", "labelName"},
		},
		{
			path: "contact label delete",
			params: map[string]wantParam{
				"id": {property: "id", required: true, interfaceType: "integer"},
			},
			absent: []string{"label-id", "role-id"},
		},
		{
			path: "contact label add-members",
			params: map[string]wantParam{
				"id":    {property: "labelIds", required: true, interfaceType: "array"},
				"users": {property: "staffIds", required: true, interfaceType: "array"},
			},
			absent: []string{"label-id", "role-id", "user-ids", "userIds", "staff-ids", "staffIds"},
		},
		{
			path: "contact label remove-members",
			params: map[string]wantParam{
				"id":    {property: "labelIds", required: true, interfaceType: "array"},
				"users": {property: "staffIds", required: true, interfaceType: "array"},
			},
			absent: []string{"label-id", "role-id", "user-ids", "userIds", "staff-ids", "staffIds"},
		},
		{
			path: "contact label update-member-scope",
			params: map[string]wantParam{
				"user":  {property: "staffId", required: true},
				"id":    {property: "labelId", required: true, interfaceType: "integer"},
				"depts": {property: "deptIds", required: true, interfaceType: "array"},
			},
			absent: []string{"staff-id", "staffId", "user-id", "userId", "label-id", "role-id", "dept-ids", "deptIds"},
		},
		{
			path: "contact ext-field create",
			params: map[string]wantParam{
				"name": {property: "orgEmpAttrModels[0].name", required: true},
			},
			absent: []string{"field-name", "fieldName"},
		},
		{
			path: "contact ext-field update",
			params: map[string]wantParam{
				"code":           {property: "orgEmpAttrModels[0].code", required: true},
				"org-self-tag":   {property: "orgEmpAttrModels[0].orgSelfTag", required: false, interfaceType: "integer"},
				"client-display": {property: "orgEmpAttrModels[0].clientDisplay", required: true},
				"is-search":      {property: "orgEmpAttrModels[0].isSearch", required: true},
			},
			absent: []string{"field-code", "fieldCode", "field-type", "fieldType", "clientDisplay", "isSearch"},
		},
		{
			path: "contact ext-field delete",
			params: map[string]wantParam{
				"code":         {property: "orgEmpAttrModels[0].code", required: true},
				"org-self-tag": {property: "orgEmpAttrModels[0].orgSelfTag", required: false, interfaceType: "integer"},
			},
			absent: []string{"field-code", "fieldCode", "field-type", "fieldType"},
		},
		{
			path: "contact dept update",
			params: map[string]wantParam{
				"dept":   {property: "deptId", required: true, interfaceType: "integer"},
				"name":   {property: "deptName", required: true},
				"parent": {property: "superDeptId", required: false, interfaceType: "integer"},
			},
			absent: []string{"id", "ids", "dept-id", "dept-ids", "dept-name", "super-dept", "super-dept-id"},
		},
		{
			path: "contact org create",
			params: map[string]wantParam{
				"org-name":         {property: "orgName", required: true},
				"creator-username": {property: "creatorUsername", required: true},
			},
		},
		{
			path: "contact account create",
			params: map[string]wantParam{
				"org-user-name":    {property: "orgUserName", required: true},
				"login-id":         {property: "loginId", required: true},
				"org-user-mobile":  {property: "orgUserMobile", required: false},
				"email":            {property: "email", required: false},
				"dept-ids":         {property: "deptIds", required: false, interfaceType: "array"},
				"send-pwd-via-sms": {property: "sendPwdViaSms", required: false},
			},
		},
		{
			path: "contact account update",
			params: map[string]wantParam{
				"user-id":        {property: "userId", required: true},
				"org-user-name":  {property: "orgUserName", required: false},
				"depts":          {property: "depts", required: false, interfaceType: "array"},
				"master-user-id": {property: "masterUserId", required: false},
				"nick":           {property: "nick", required: false},
				"avatar-file-id": {property: "avatarFileId", required: false},
			},
			absent: []string{"id", "userid"},
		},
	}

	for _, tc := range cases {
		leaf, err := queryDeliverySchemaPayload([]string{tc.path})
		if err != nil {
			t.Fatal(err)
		}
		parameters := schemaMap(leaf["parameters"])
		for flagName, want := range tc.params {
			param := parameters[flagName]
			if param["property"] != want.property || param["required"] != want.required {
				t.Fatalf("%s --%s = %#v, want property=%s required=%v",
					tc.path, flagName, param, want.property, want.required)
			}
			if want.interfaceType != "" && param["interface_type"] != want.interfaceType {
				t.Fatalf("%s --%s interface_type = %#v, want %s",
					tc.path, flagName, param["interface_type"], want.interfaceType)
			}
			propProv := schemaMap(param["field_provenance"])["property"]
			if src, _ := propProv["source"].(string); src != "native_annotation" &&
				src != "versioned_parameter_binding" && src != "flag_name_inference" {
				t.Fatalf("%s --%s property source = %#v, want native_annotation/binding/inference",
					tc.path, flagName, propProv)
			}
			reqProv := schemaMap(param["field_provenance"])["required"]
			if src, _ := reqProv["source"].(string); src != "native_annotation" && src != "require_one_of_constraint" {
				t.Fatalf("%s --%s required source = %#v, want native_annotation or require_one_of_constraint",
					tc.path, flagName, reqProv)
			}
			if want.interfaceType != "" {
				prov := schemaMap(param["field_provenance"])["interface_type"]
				if src, _ := prov["source"].(string); src != "native_annotation" {
					t.Fatalf("%s --%s interface_type source = %#v", tc.path, flagName, prov)
				}
			}
		}
		for _, flagName := range tc.absent {
			if _, ok := parameters[flagName]; ok {
				t.Fatalf("%s unexpectedly publishes hidden alias --%s", tc.path, flagName)
			}
		}
	}
}

func TestDeliveryCatalogChatParamDeclsFrom87910880Reviewed(t *testing.T) {
	// CI P1: +messages-send must not publish RequiredWhen for bot/webhook
	// credentials. Runtime validateMessagesSend already enforces them;
	// re-adding Flag.RequiredWhen breaks merge-base schema-compatibility.
	sendLeaf, err := queryDeliverySchemaPayload([]string{"chat +messages-send"})
	if err != nil {
		t.Fatal(err)
	}
	sendParams := schemaMap(sendLeaf["parameters"])
	for _, flagName := range []string{"robot-code", "webhook-token"} {
		param := sendParams[flagName]
		if param == nil {
			t.Fatalf("chat +messages-send missing --%s", flagName)
		}
		if rw := param["required_when"]; rw != nil && rw != "" {
			t.Fatalf("chat +messages-send --%s required_when = %#v, want absent/empty (no Schema gate)", flagName, rw)
		}
		rwProv := schemaMap(param["field_provenance"])["required_when"]
		if src, _ := rwProv["source"].(string); src != "default" && src != "" {
			t.Fatalf("chat +messages-send --%s required_when source = %#v, want default", flagName, rwProv)
		}
	}

	// Former metadata/chat.json overlays (87910880) must stay on
	// helpers/shortcut leaves via contract.ParamDecl after hint clearance.
	for _, tc := range []struct {
		path          string
		flag          string
		property      string
		required      any
		interfaceType string
	}{
		{"chat message edit", "conversation-id", "openConversationId", true, ""},
		{"chat message edit", "message-id", "openMessageId", true, ""},
		{"chat message edit", "at-open-dingtalk-ids", "atOpenDingTalkIds", false, "array"},
		{"chat message update-text-emotion", "message-id", "openMsgId", true, ""},
		{"chat message send", "idempotency-key", "uuid", false, ""},
		{"chat message send-card", "at-all", "atAll", false, ""},
		{"chat message send-card", "at-open-dingtalk-ids", "atOpenDingTalkIds", false, "array"},
		{"chat message update-text-emotion", "old-emotion-id", "oldEmotionId", true, ""},
		{"chat category batch-info", "category-ids", "categoryIds", true, "array"},
		{"chat category list-by-conv", "conversation-id", "openConversationId", true, ""},
		{"chat group update-nick", "conversation-id", "", true, ""},
		{"chat group upgrade-to-external", "extension", "extension", false, "object"},
		{"chat +messages-send-card", "receiver-open-dingtalk-id", "receiverOpenDingTalkId", false, ""},
		{"chat message list-favorites", "size", "", false, "string"},
	} {
		leaf, err := queryDeliverySchemaPayload([]string{tc.path})
		if err != nil {
			t.Fatal(err)
		}
		param := schemaMap(leaf["parameters"])[tc.flag]
		if tc.property != "" && param["property"] != tc.property {
			t.Fatalf("%s --%s property = %#v, want %s", tc.path, tc.flag, param["property"], tc.property)
		}
		if tc.required != nil && param["required"] != tc.required {
			t.Fatalf("%s --%s required = %#v, want %#v", tc.path, tc.flag, param["required"], tc.required)
		}
		if tc.interfaceType != "" && param["interface_type"] != tc.interfaceType {
			t.Fatalf("%s --%s interface_type = %#v, want %s", tc.path, tc.flag, param["interface_type"], tc.interfaceType)
		}
		if tc.property != "" {
			propProv := schemaMap(param["field_provenance"])["property"]
			src, _ := propProv["source"].(string)
			if src != "native_annotation" && src != "versioned_parameter_binding" && src != "reviewed_mapping_exclusion" {
				t.Fatalf("%s --%s property source = %#v, want native_annotation/binding/exclusion", tc.path, tc.flag, propProv)
			}
		}
		if tc.interfaceType != "" {
			typeProv := schemaMap(param["field_provenance"])["interface_type"]
			if src, _ := typeProv["source"].(string); src != "native_annotation" {
				t.Fatalf("%s --%s interface_type source = %#v, want native_annotation", tc.path, tc.flag, typeProv)
			}
		}
	}

	// Manifest-covered migrations hide legacy aliases; manifest-external
	// commands keep their existing visible flags for compatibility.
	editLeaf, err := queryDeliverySchemaPayload([]string{"chat message edit"})
	if err != nil {
		t.Fatal(err)
	}
	editParams := schemaMap(editLeaf["parameters"])
	for _, hidden := range []string{"group", "id", "chat"} {
		if _, ok := editParams[hidden]; ok {
			t.Fatalf("chat message edit unexpectedly publishes hidden alias --%s", hidden)
		}
	}
	sendLeaf, err = queryDeliverySchemaPayload([]string{"chat message send"})
	if err != nil {
		t.Fatal(err)
	}
	sendParams = schemaMap(sendLeaf["parameters"])
	if _, ok := sendParams["uuid"]; ok {
		t.Fatal("chat message send unexpectedly publishes hidden alias --uuid")
	}
	listByConv, err := queryDeliverySchemaPayload([]string{"chat category list-by-conv"})
	if err != nil {
		t.Fatal(err)
	}
	listParams := schemaMap(listByConv["parameters"])
	if _, ok := listParams["conversation-id"]; !ok {
		t.Fatalf("chat category list-by-conv missing public canonical --conversation-id")
	}
	if _, ok := listParams["group"]; !ok {
		t.Fatalf("chat category list-by-conv unexpectedly hides manifest-external --group")
	}
	if _, ok := listParams["id"]; ok {
		t.Fatalf("chat category list-by-conv unexpectedly publishes hidden alias --id")
	}

	for _, path := range []string{
		"chat message add-emoji",
		"chat message remove-emoji",
		"chat message add-text-emotion",
		"chat message remove-text-emotion",
	} {
		leaf, err := queryDeliverySchemaPayload([]string{path})
		if err != nil {
			t.Fatal(err)
		}
		params := schemaMap(leaf["parameters"])
		if _, ok := params["conversation-id"]; !ok {
			t.Fatalf("%s missing public canonical --conversation-id", path)
		}
		for _, visible := range []string{"group", "id", "chat"} {
			if _, ok := params[visible]; !ok {
				t.Fatalf("%s unexpectedly hides manifest-external --%s", path, visible)
			}
		}
	}

	groupBots, err := queryDeliverySchemaPayload([]string{"chat group bots"})
	if err != nil {
		t.Fatal(err)
	}
	groupBotsParams := schemaMap(groupBots["parameters"])
	group := groupBotsParams["group"]
	if group == nil {
		t.Fatal("chat group bots missing public legacy --group")
	}
	if group["property"] != "openConversationId" {
		t.Fatalf("chat group bots --group property = %#v, want openConversationId", group["property"])
	}
	for _, migrated := range []string{"conversation-id", "group-name"} {
		if _, ok := groupBotsParams[migrated]; ok {
			t.Fatalf("chat group bots unexpectedly publishes migrated --%s", migrated)
		}
	}
}
