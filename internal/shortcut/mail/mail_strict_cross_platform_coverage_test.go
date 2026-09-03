// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageMailContractsAreUnifiedAndTyped(t *testing.T) {
	declarations := []*struct {
		rollout output.RolloutState
		result  bool
	}{
		{FolderList.OutputRollout, FolderList.Contract.Result != nil},
		{UserSearch.OutputRollout, UserSearch.Contract.Result != nil},
		{Message.OutputRollout, Message.Contract.Result != nil},
		{Messages.OutputRollout, Messages.Contract.Result != nil},
		{Thread.OutputRollout, Thread.Contract.Result != nil},
	}
	for index, declaration := range declarations {
		if declaration.rollout != output.RolloutUnifiedActive || !declaration.result {
			t.Fatalf("mail declaration %d is not unified with Result", index)
		}
	}
}

func TestCrossPlatformCoverageMailUnavailableWritesMatchRuntimeBoundary(t *testing.T) {
	for _, declaration := range []*shortcut.Shortcut{&DraftCreate, &DraftEdit, &TemplateCreate, &TemplateUpdate} {
		if declaration.OutputRollout != output.RolloutLegacyOnly {
			t.Errorf("%s rollout=%q, want legacy_only", declaration.Command, declaration.OutputRollout)
		}
		if declaration.Contract.Result != nil || declaration.Contract.Pagination != nil {
			t.Errorf("%s unavailable contract still publishes result/pagination", declaration.Command)
		}
		if declaration.Contract.Interface == nil || declaration.Contract.Interface.Availability != "unavailable" || declaration.Contract.Interface.Reason == "" {
			t.Errorf("%s missing precise unavailable interface", declaration.Command)
		}
	}
}

func TestCrossPlatformCoverageMailCompatibilityReadsMatchRuntimeBoundary(t *testing.T) {
	for _, declaration := range []*shortcut.Shortcut{&ThreadList, &TagList, &TemplateList, &ContactList} {
		if declaration.OutputRollout != output.RolloutLegacyOnly {
			t.Errorf("%s rollout=%q, want legacy_only", declaration.Command, declaration.OutputRollout)
		}
		if declaration.Contract.Result != nil || declaration.Contract.Pagination != nil {
			t.Errorf("%s unavailable contract still publishes result/pagination", declaration.Command)
		}
		if declaration.Contract.Interface == nil || declaration.Contract.Interface.Availability != contract.InterfaceAvailable || declaration.Contract.Interface.Reason != mailCompatibilityInterfaceReason {
			t.Errorf("%s missing Schema-compatible non-public interface", declaration.Command)
		}
	}
}

func TestCrossPlatformCoverageMailMutationConstraintsAreDeclared(t *testing.T) {
	want := map[string]bool{"to": false, "cc": false, "subject": false, "body": false}
	for _, constraint := range DraftEdit.Constraints {
		if constraint.Kind != shortcut.ConstraintAtLeastOne {
			continue
		}
		for _, flag := range constraint.Flags {
			if _, ok := want[flag]; ok {
				want[flag] = true
			}
		}
	}
	for flag, found := range want {
		if !found {
			t.Fatalf("draft edit missing at-least-one declaration for %s", flag)
		}
	}
	if len(Messages.Constraints) != 1 || Messages.Constraints[0].Kind != shortcut.ConstraintCustom || Messages.Validate == nil {
		t.Fatal("messages must declare and execute the 1-100 nonempty ID contract")
	}
	userSearchNonempty := false
	for _, constraint := range UserSearch.Constraints {
		if constraint.Kind == shortcut.ConstraintCustom && len(constraint.Flags) == 2 && constraint.Flags[0] == "keyword" && constraint.Flags[1] == "employee-no" && constraint.Description != "" {
			userSearchNonempty = true
		}
	}
	if !userSearchNonempty {
		t.Fatal("user search must declare its explicit nonempty string contract")
	}
}

func TestCrossPlatformCoverageMailPaginationDeclarationsMatchLeaves(t *testing.T) {
	for _, declaration := range []*shortcut.Shortcut{&UserSearch} {
		if declaration.Contract.Pagination == nil || declaration.Contract.Pagination.CursorParameter != "cursor" {
			t.Fatalf("%s missing cursor pagination declaration", declaration.Command)
		}
	}
	for _, declaration := range []*shortcut.Shortcut{&ThreadList, &FolderList, &TagList, &TemplateList, &ContactList, &Messages} {
		if declaration.Contract.Pagination != nil {
			t.Fatalf("%s must not declare pagination", declaration.Command)
		}
	}
}

func TestCrossPlatformCoverageMailUnifiedPaginationLivesOnlyInMeta(t *testing.T) {
	for _, tc := range []struct {
		name      string
		complete  bool
		next      string
		exhausted bool
	}{
		{name: "continuing", complete: false, next: "cursor-2", exhausted: false},
		{name: "terminal", complete: true, next: "", exhausted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			declaration := UserSearch
			cmd := corecmd.New(shortcut.FromShortcut(declaration))
			ctx, _ := output.WithResultStore(context.Background())
			cmd.SetContext(ctx)
			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			rt := shortcut.RuntimeContextForTest(cmd, declaration)
			if err := mailOutputPage(rt, "users", []map[string]any{{"email": "fixture@example.invalid"}}, tc.complete, tc.next); err != nil {
				t.Fatalf("store pagination: %v", err)
			}
			if code, emitted, err := output.EmitStoredResult(cmd); err != nil || !emitted || code != 0 {
				t.Fatalf("emit code=%d emitted=%v err=%v", code, emitted, err)
			}
			var envelope struct {
				Data map[string]any `json:"data"`
				Meta struct {
					Pagination *struct {
						EndpointExhausted bool   `json:"endpoint_exhausted"`
						NextToken         string `json:"next_token"`
					} `json:"pagination"`
				} `json:"meta"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("decode output: %v", err)
			}
			for _, field := range []string{"complete", "hasMore", "nextCursor"} {
				if _, exists := envelope.Data[field]; exists {
					t.Fatalf("pagination field %s leaked into data", field)
				}
			}
			if envelope.Meta.Pagination == nil || envelope.Meta.Pagination.EndpointExhausted != tc.exhausted || envelope.Meta.Pagination.NextToken != tc.next {
				t.Fatalf("pagination=%+v", envelope.Meta.Pagination)
			}
		})
	}
}

func TestCrossPlatformCoverageMailResultSchemasRequireStableIdentity(t *testing.T) {
	collections := []struct {
		declaration *shortcut.Shortcut
		collection  string
		identity    string
	}{
		{&FolderList, "folders", "id"},
		{&UserSearch, "users", "email"},
		{&Messages, "messages", "id"},
	}
	for _, tc := range collections {
		if len(tc.declaration.Contract.Result.SensitivePaths) == 0 {
			t.Fatalf("%s missing sensitive_paths", tc.declaration.Command)
		}
		var schema map[string]any
		if err := json.Unmarshal(tc.declaration.Contract.Result.DataSchema, &schema); err != nil {
			t.Fatalf("%s schema: %v", tc.declaration.Command, err)
		}
		properties := schema["properties"].(map[string]any)
		count := properties["count"].(map[string]any)
		if count["minimum"] != float64(0) {
			t.Fatalf("%s count minimum=%v, want 0", tc.declaration.Command, count["minimum"])
		}
		items := properties[tc.collection].(map[string]any)["items"].(map[string]any)
		identity, ok := items["properties"].(map[string]any)[tc.identity].(map[string]any)
		if !ok || identity["description"] == "" {
			t.Fatalf("%s missing identity %s", tc.declaration.Command, tc.identity)
		}
	}
	for _, declaration := range []*shortcut.Shortcut{&Message, &Thread} {
		if len(declaration.Contract.Result.SensitivePaths) == 0 {
			t.Fatalf("%s missing sensitive_paths", declaration.Command)
		}
		var schema map[string]any
		if err := json.Unmarshal(declaration.Contract.Result.DataSchema, &schema); err != nil {
			t.Fatalf("%s schema: %v", declaration.Command, err)
		}
		value := schema["properties"].(map[string]any)["value"].(map[string]any)
		required := value["required"].([]any)
		if len(required) != 1 || required[0] != "id" {
			t.Fatalf("%s value does not require id", declaration.Command)
		}
	}
}

func TestCrossPlatformCoverageMailWritesRequireConfirmation(t *testing.T) {
	for _, declaration := range []struct {
		name         string
		confirmation string
	}{
		{DraftCreate.Command, DraftCreate.Safety.Confirmation},
		{DraftEdit.Command, DraftEdit.Safety.Confirmation},
		{TemplateCreate.Command, TemplateCreate.Safety.Confirmation},
		{TemplateUpdate.Command, TemplateUpdate.Safety.Confirmation},
	} {
		if declaration.confirmation != "user_required" {
			t.Errorf("%s confirmation=%q, want user_required", declaration.name, declaration.confirmation)
		}
	}
}

func TestCrossPlatformCoverageMailStrictResponseMatrix(t *testing.T) {
	validEmpty := map[string]any{"success": "true", "folders": []any{}}
	items, err := mailRequireCollection(validEmpty, "mail/list_folders", "folders")
	if err != nil || len(items) != 0 {
		t.Fatalf("explicit empty collection must succeed: items=%v err=%v", items, err)
	}
	for name, fixture := range map[string]map[string]any{
		"missing success":    {"folders": []any{}},
		"missing collection": {"success": "true"},
		"wrong collection":   {"success": true, "folders": map[string]any{}},
		"bad item":           {"success": "true", "folders": []any{"bad"}},
	} {
		if _, err := mailRequireCollection(fixture, "mail/list_folders", "folders"); err == nil {
			t.Fatalf("%s must fail closed", name)
		}
	}
	if err := mailRequireSuccess(map[string]any{"success": "false"}, "mail/test"); err == nil {
		t.Fatal("string false must not be success")
	}
	if err := mailRequireSuccess(map[string]any{"success": true}, "mail/test"); err != nil {
		t.Fatalf("boolean true must be accepted: %v", err)
	}
}

func TestCrossPlatformCoverageMailPaginationAndIdentity(t *testing.T) {
	complete, next, err := mailPage(map[string]any{"nextCursor": "$"}, "mail/search", "", "")
	if err != nil || !complete || next != "" {
		t.Fatalf("terminal dollar cursor mismatch: complete=%v next=%q err=%v", complete, next, err)
	}
	complete, next, err = mailPage(map[string]any{"hasMore": "false", "nextCursor": ""}, "mail/list", "", "")
	if err != nil || !complete || next != "" {
		t.Fatalf("string false pagination mismatch: complete=%v next=%q err=%v", complete, next, err)
	}
	if _, _, err := mailPage(map[string]any{"hasMore": true, "nextCursor": ""}, "mail/list", "", ""); err == nil {
		t.Fatal("hasMore without a progressing cursor must fail")
	}
	if _, _, err := mailPage(map[string]any{"hasMore": true, "nextCursor": "cursor-1"}, "mail/list", "", "cursor-1"); err == nil {
		t.Fatal("repeated cursor must fail closed")
	}
	if err := mailRequireIdentity(map[string]any{"id": "actual"}, "mail/get", "expected", "id"); err == nil {
		t.Fatal("identity mismatch must fail")
	}
}

func TestCrossPlatformCoverageMailUserProjectionOmitsEmptyOptionalIdentity(t *testing.T) {
	for _, tc := range []struct {
		name      string
		user      map[string]any
		wantKey   string
		absentKey string
	}{
		{name: "user id only", user: map[string]any{"id": "user-1", "email": ""}, wantKey: "userId", absentKey: "email"},
		{name: "email only", user: map[string]any{"id": "", "email": "user@example.invalid"}, wantKey: "email", absentKey: "userId"},
		{name: "numeric user id only", user: map[string]any{"id": float64(7), "email": "  "}, wantKey: "userId", absentKey: "email"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := mailProjectCollection(
				map[string]any{"success": true, "users": []any{tc.user}},
				"mail/search_mail_users", "users", []string{"id", "email"},
				map[string][]string{"userId": {"id"}, "email": {"email"}},
			)
			if err != nil || len(rows) != 1 {
				t.Fatalf("rows=%v err=%v", rows, err)
			}
			if _, ok := rows[0][tc.wantKey]; !ok {
				t.Fatalf("projection missing %s: %#v", tc.wantKey, rows[0])
			}
			if _, ok := rows[0][tc.absentKey]; ok {
				t.Fatalf("projection retained empty %s: %#v", tc.absentKey, rows[0])
			}
		})
	}
	if _, err := mailProjectCollection(
		map[string]any{"success": true, "users": []any{map[string]any{"id": "", "email": ""}}},
		"mail/search_mail_users", "users", []string{"id", "email"},
		map[string][]string{"userId": {"id"}, "email": {"email"}},
	); err == nil {
		t.Fatal("user without a stable identity unexpectedly projected")
	}
	for name, user := range map[string]map[string]any{
		"malformed email with valid id": {"id": "user-1", "email": 7},
		"malformed id with valid email": {"id": map[string]any{"value": "user-1"}, "email": "user@example.invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := mailProjectCollection(
				map[string]any{"success": true, "users": []any{user}},
				"mail/search_mail_users", "users", []string{"id", "email"},
				map[string][]string{"userId": {"id"}, "email": {"email"}},
			); err == nil {
				t.Fatal("malformed optional identity unexpectedly passed projection")
			}
		})
	}
}
