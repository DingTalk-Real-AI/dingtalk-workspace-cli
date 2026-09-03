// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import "testing"

func TestPrimaryParamMigrationKeepsConceptDirectionCommandScoped(t *testing.T) {
	concepts, err := LoadParamConcepts()
	if err != nil {
		t.Fatalf("LoadParamConcepts() error = %v", err)
	}
	if got := concepts.ByConcept["content_text"].CanonicalHint; got != "text" {
		t.Fatalf("content_text canonical hint = %q, want unchanged global hint text", got)
	}
	if got := concepts.ByConcept["open_conversation_id"].CanonicalHint; got != "conversation-id" {
		t.Fatalf("open_conversation_id canonical hint = %q, want unchanged global hint conversation-id", got)
	}

	content := concepts.ByConcept["content_text"]
	docNode := concepts.ByConcept["doc_node_id"]
	docAppend, problems := reduceLeafParamAliases(
		"doc +doc-append",
		realMap(
			realFlag{name: "content"},
			realFlag{name: "text", hidden: true},
			realFlag{name: "doc"},
		),
		[]Concept{content, docNode},
		CommandOverride{},
	)
	if len(problems) != 0 {
		t.Fatalf("doc +doc-append reduction problems = %v", problems)
	}
	assertPrimaryParamAlias(t, docAppend, "body", "content")
	assertPrimaryParamAlias(t, docAppend, "node", "doc")

	insertOverride := primaryParamOverride(t, concepts, "doc block insert")
	docInsert, problems := reduceLeafParamAliases(
		"doc block insert",
		realMap(
			realFlag{name: "content"},
			realFlag{name: "text", hidden: true},
			realFlag{name: "parent-block"},
			realFlag{name: "ref-block"},
		),
		[]Concept{content},
		insertOverride,
	)
	if len(problems) != 0 {
		t.Fatalf("doc block insert reduction problems = %v", problems)
	}
	assertPrimaryParamAlias(t, docInsert, "body", "content")

	docUpdate, problems := reduceLeafParamAliases(
		"doc block update",
		realMap(realFlag{name: "content"}, realFlag{name: "text", hidden: true}),
		[]Concept{content},
		CommandOverride{},
	)
	if len(problems) != 0 {
		t.Fatalf("doc block update reduction problems = %v", problems)
	}
	assertPrimaryParamAlias(t, docUpdate, "body", "content")

	replyOverride := primaryParamOverride(t, concepts, "chat +messages-reply")
	chatReply, problems := reduceLeafParamAliases(
		"chat +messages-reply",
		realMap(
			realFlag{name: "group"},
			realFlag{name: "conversation-id", hidden: true},
			realFlag{name: "ref-msg-id"},
		),
		[]Concept{concepts.ByConcept["open_conversation_id"]},
		replyOverride,
	)
	if len(problems) != 0 {
		t.Fatalf("chat +messages-reply reduction problems = %v", problems)
	}
	for _, emitted := range []string{"chat", "chat-id", "open-conversation-id"} {
		assertPrimaryParamAlias(t, chatReply, emitted, "group")
	}
	assertPrimaryParamAlias(t, chatReply, "msg-id", "ref-msg-id")
	if chatReply.IsBlocked("group") {
		t.Fatal("chat +messages-reply canonical --group must not remain blocked")
	}
}

func TestPrimaryParamMigrationUpdatesReviewedFixturesAndMappingKeys(t *testing.T) {
	concepts, err := LoadParamConcepts()
	if err != nil {
		t.Fatalf("LoadParamConcepts() error = %v", err)
	}
	fixtures := []ParamFixtureCase{
		{Command: "doc +doc-append", Emitted: "text", Expect: "content", Via: "native:reviewed-compatibility-alias"},
		{Command: "doc +doc-append", Emitted: "node", Expect: "doc", Via: "concept:doc_node_id"},
		{Command: "doc block insert", Emitted: "text", Expect: "content", Via: "native:reviewed-compatibility-alias", Occ: 2},
		{Command: "doc block update", Emitted: "text", Expect: "content", Via: "native:reviewed-compatibility-alias", Occ: 6},
		{Command: "chat message send", Emitted: "file-path", Expect: "file", Via: "native:reviewed-compatibility-alias"},
		{Command: "chat +messages-reply", Emitted: "conversation-id", Expect: "group", Via: "native:reviewed-compatibility-alias"},
		{Command: "chat +messages-reply", Emitted: "chat", Expect: "group", Via: "concept:open_conversation_id"},
		{Command: "chat +messages-reply", Emitted: "open-conversation-id", Expect: "group", Via: "concept:open_conversation_id"},
	}
	for _, want := range fixtures {
		found := false
		for _, got := range concepts.Fixture {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("reviewed fixture missing %#v", want)
		}
	}

	tests := []struct {
		oldKey string
		newKey string
		reason string
	}{
		{"chat.reply_personal_message --text", "chat.reply_personal_message --content", "serialized into the aggregate content JSON string"},
		{"chat.send_personal_message --file-path", "chat.send_personal_message --file", "local upload/preprocessing input"},
		{"chat.send_personal_message --text", "chat.send_personal_message --content", "serialized into the aggregate content payload"},
		{"doc.insert_document_block --text", "doc.insert_document_block --content", "aggregate convenience input used to build element"},
		{"doc.update_document_block --text", "doc.update_document_block --content", "aggregate convenience input used to build element"},
		{"todo.add_todo_attachment --file-path", "todo.add_todo_attachment --file", "local upload input used to construct attachmentList"},
	}
	for _, test := range tests {
		if _, exists := reviewedSchemaParameterMappingExclusions[test.oldKey]; exists {
			t.Errorf("stale mapping exclusion remains at %q", test.oldKey)
		}
		if got := reviewedSchemaParameterMappingExclusions[test.newKey]; got != test.reason {
			t.Errorf("mapping exclusion %q reason = %q, want %q", test.newKey, got, test.reason)
		}
	}
}

func TestPrimaryParamMigrationPreservesDeliverySchemaSignatures(t *testing.T) {
	tests := []struct {
		path             string
		primary          string
		legacy           string
		required         bool
		cliRequired      bool
		property         string
		propertyFrom     string
		format           string
		requiredWhen     string
		requiredWhenFrom string
	}{
		{path: "aisearch person", primary: "query", legacy: "keyword", required: true, property: "keyword", propertyFrom: "native_annotation", requiredWhenFrom: "default"},
		{path: "chat message reply", primary: "content", legacy: "text", required: true, cliRequired: true, propertyFrom: "reviewed_mapping_exclusion", requiredWhenFrom: "default"},
		{path: "chat message send", primary: "content", legacy: "text", propertyFrom: "reviewed_mapping_exclusion", requiredWhenFrom: "default"},
		{path: "chat message send", primary: "file", legacy: "file-path", propertyFrom: "reviewed_mapping_exclusion", format: "file-path", requiredWhen: "msg-type is file or audio or video", requiredWhenFrom: "typed_parameter_metadata"},
		{path: "chat message send-by-webhook", primary: "content", legacy: "text", required: true, cliRequired: true, property: "text", propertyFrom: "native_annotation", requiredWhenFrom: "default"},
		{path: "chat +broadcast", primary: "content", legacy: "text", required: true, property: "text", propertyFrom: "native_annotation", requiredWhenFrom: "default"},
		{path: "chat +dm", primary: "content", legacy: "text", required: true, property: "text", propertyFrom: "native_annotation", requiredWhenFrom: "default"},
		{path: "chat +send-to-group", primary: "content", legacy: "text", required: true, property: "text", propertyFrom: "native_annotation", requiredWhenFrom: "default"},
		{path: "chat +messages-send-by-bot", primary: "content", legacy: "text", required: true, property: "text", propertyFrom: "native_annotation", requiredWhenFrom: "default"},
		{path: "chat +messages-batch-send-by-bot", primary: "content", legacy: "text", required: true, property: "text", propertyFrom: "native_annotation", requiredWhenFrom: "default"},
		{path: "chat +messages-send-by-webhook", primary: "content", legacy: "text", required: true, property: "text", propertyFrom: "native_annotation", requiredWhenFrom: "default"},
		{path: "chat +messages-reply", primary: "content", legacy: "text", required: true, property: "text", propertyFrom: "native_annotation", requiredWhenFrom: "default"},
		{path: "chat +messages-reply", primary: "group", legacy: "conversation-id", required: true, property: "conversationId", propertyFrom: "native_annotation", requiredWhenFrom: "default"},
		{path: "doc +doc-append", primary: "content", legacy: "text", required: true, property: "text", propertyFrom: "native_annotation", requiredWhenFrom: "default"},
		{path: "doc block insert", primary: "content", legacy: "text", propertyFrom: "reviewed_mapping_exclusion", requiredWhenFrom: "default"},
		{path: "doc block update", primary: "content", legacy: "text", propertyFrom: "reviewed_mapping_exclusion", requiredWhenFrom: "default"},
		{path: "todo task add-attachment", primary: "file", legacy: "file-path", required: true, propertyFrom: "reviewed_mapping_exclusion", requiredWhenFrom: "default"},
	}
	for _, test := range tests {
		t.Run(test.path+"/"+test.primary, func(t *testing.T) {
			leaf, err := queryDeliverySchemaPayload([]string{test.path})
			if err != nil {
				t.Fatal(err)
			}
			parameters := schemaMap(leaf["parameters"])
			parameter, exists := parameters[test.primary]
			if !exists {
				t.Fatalf("%s missing visible Primary --%s", test.path, test.primary)
			}
			if _, exists := parameters[test.legacy]; exists {
				t.Fatalf("%s unexpectedly publishes hidden legacy --%s", test.path, test.legacy)
			}
			if got := parameter["type"]; got != "string" {
				t.Errorf("%s --%s type = %#v, want string", test.path, test.primary, got)
			}
			if got := parameter["required"]; got != test.required {
				t.Errorf("%s --%s required = %#v, want %v", test.path, test.primary, got, test.required)
			}
			if got, _ := parameter["cli_required"].(bool); got != test.cliRequired {
				t.Errorf("%s --%s cli_required = %#v, want %v", test.path, test.primary, parameter["cli_required"], test.cliRequired)
			}
			if got, _ := parameter["property"].(string); got != test.property {
				t.Errorf("%s --%s property = %q, want %q", test.path, test.primary, got, test.property)
			}
			if got := parameter["interface_type"]; got != nil && got != "" {
				t.Errorf("%s --%s interface_type = %#v, want omitted/empty", test.path, test.primary, got)
			}
			if got, _ := parameter["format"].(string); got != test.format {
				t.Errorf("%s --%s format = %q, want %q", test.path, test.primary, got, test.format)
			}
			if got, _ := parameter["required_when"].(string); got != test.requiredWhen {
				t.Errorf("%s --%s required_when = %q, want %q", test.path, test.primary, got, test.requiredWhen)
			}

			provenance := schemaMap(parameter["field_provenance"])
			propertyProvenance := provenance["property"]
			if got := propertyProvenance["source"]; got != test.propertyFrom {
				t.Errorf("%s --%s property source = %#v, want %s", test.path, test.primary, got, test.propertyFrom)
			}
			typeProvenance := provenance["type"]
			if got := typeProvenance["source"]; got != "cobra_flag_type" {
				t.Errorf("%s --%s type source = %#v, want cobra_flag_type", test.path, test.primary, got)
			}
			requiredWhenProvenance := provenance["required_when"]
			if got := requiredWhenProvenance["source"]; got != test.requiredWhenFrom {
				t.Errorf("%s --%s required_when source = %#v, want %s", test.path, test.primary, got, test.requiredWhenFrom)
			}
		})
	}
}

func primaryParamOverride(t *testing.T, concepts ParamConcepts, path string) CommandOverride {
	t.Helper()
	for _, override := range concepts.Overrides {
		if override.CommandPath == path {
			return override
		}
	}
	t.Fatalf("command override %q not found", path)
	return CommandOverride{}
}

func assertPrimaryParamAlias(t *testing.T, entry *ParamAliasEntry, emitted, want string) {
	t.Helper()
	if entry == nil {
		t.Fatalf("alias entry is nil; want %s -> %s", emitted, want)
	}
	if got, ok := entry.ResolveAlias(emitted); !ok || got != want {
		t.Fatalf("ResolveAlias(%q) = %q (ok=%v), want %q", emitted, got, ok, want)
	}
}
