// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestCrossPlatformCoverageChatGroupJoinDataSchemaAcceptsRealBackendShape(t *testing.T) {
	joinResponses := []string{
		`{"success":true,"result":{"openConversationId":"cidGroup1","status":"ALREADY_MEMBER"},"errorCode":null,"errorMsg":null,"arguments":[]}`,
		`{"success":true,"result":{"openConversationId":"cidGroup1","status":"JOINED","isMember":true,"alreadyMember":false},"errorCode":null,"errorMsg":null,"arguments":[]}`,
		`{"result":{"openConversationId":"cidGroup1","status":"APPLIED"}}`,
		`{"success":true,"result":{"openConversationId":"cidGroup1","status":"ALREADY_MEMBER","isMember":null}}`,
		`{"success":true}`,
	}
	previewResponses := []string{
		`{"success":true,"result":{"groupInfo":{"openConversationId":"cidGroup1","title":"群名","memberCount":3},"isMember":true},"errorCode":null,"errorMsg":null,"arguments":[]}`,
		`{"success":true,"result":{"groupInfo":{"openConversationId":"cidGroup1","title":"群名","memberCount":2},"isMember":false},"errorCode":null,"errorMsg":null,"arguments":[]}`,
		`{"result":{"groupInfo":{"title":"群名"},"isMember":null}}`,
		`{"success":true,"result":{"groupInfo":{"title":"群名"}}}`,
		`{"success":true}`,
	}
	for _, tc := range []struct {
		path    string
		valid   []string
		invalid string
	}{
		{"chat group join-by-group-id", joinResponses, `{"result":{"status":7}}`},
		{"chat group get-info-by-invite-url", previewResponses, `{"result":{"groupInfo":{"memberCount":"3"}}}`},
		{"chat group join-by-invite-url", joinResponses, `{"result":{"isMember":"unknown"}}`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			for _, compact := range []bool{false, true} {
				args := []string{"--cli-path", tc.path}
				if compact {
					args = append(args, "--compact")
				}
				leaf := executeShortcutSchemaQuery(t, args...)
				result := leaf["result"].(map[string]any)
				compiler := jsonschema.NewCompiler()
				compiler.DefaultDraft(jsonschema.Draft2020)
				const resource = "https://dws.test/group-join-result.json"
				if err := compiler.AddResource(resource, result["data_schema"]); err != nil {
					t.Fatal(err)
				}
				schema, err := compiler.Compile(resource)
				if err != nil {
					t.Fatal(err)
				}
				for _, raw := range tc.valid {
					var data any
					if err := json.Unmarshal([]byte(raw), &data); err != nil {
						t.Fatal(err)
					}
					if err := schema.Validate(data); err != nil {
						t.Fatalf("compact=%t actual backend shape rejected: %s: %v", compact, raw, err)
					}
				}
				for _, raw := range []string{
					`{"success":"true"}`,
					`{"result":[]}`,
					`{"errorMsg":7}`,
					`{"arguments":{}}`,
					tc.invalid,
				} {
					var data any
					if err := json.Unmarshal([]byte(raw), &data); err != nil {
						t.Fatal(err)
					}
					if err := schema.Validate(data); err == nil {
						t.Fatalf("compact=%t invalid nested backend shape accepted: %s", compact, raw)
					}
				}
			}
		})
	}
}
