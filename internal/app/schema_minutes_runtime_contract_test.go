// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageMinutesDisplayFinalSchema(t *testing.T) {
	for _, path := range []string{"minutes +list-mine", "minutes +list-shared", "minutes +list-all", "minutes +search"} {
		full := executeShortcutSchemaQuery(t, "--cli-path", path)
		compact := executeShortcutSchemaQuery(t, "--cli-path", path, "--compact")
		if !reflect.DeepEqual(full["result"], compact["result"]) {
			t.Fatalf("%s compact result drift", path)
		}
		props := schemaContractMap(schemaContractMap(schemaContractMap(full["result"])["data_schema"])["properties"])
		item := schemaContractMap(schemaContractMap(props["minutes"])["items"])
		display := schemaContractMap(item["properties"])
		for _, field := range []string{"orgName", "flashUserInfo"} {
			if schemaContractString(schemaContractMap(display[field])["description"]) == "" {
				t.Fatalf("%s missing %s", path, field)
			}
		}
		if full["confirmation"] != "not_required" {
			t.Fatalf("%s confirmation drift", path)
		}
	}
}

func TestCrossPlatformCoverageMinutesExportSanitizationFinalSchema(t *testing.T) {
	full := executeShortcutSchemaQuery(t, "--cli-path", "minutes +export-pack")
	compact := executeShortcutSchemaQuery(t, "--cli-path", "minutes +export-pack", "--compact")
	if !reflect.DeepEqual(full["result"], compact["result"]) {
		t.Fatal("compact result differs from full result")
	}
	result := schemaContractMap(full["result"])
	properties := schemaContractMap(schemaContractMap(result["data_schema"])["properties"])
	for _, name := range []string{"published", "path", "manifest", "files", "sanitized", "redactionCount", "redactionKinds", "sanitizationScope", "offlineImagesComplete"} {
		if schemaContractString(schemaContractMap(properties[name])["description"]) == "" {
			t.Errorf("missing field %s", name)
		}
	}
	for _, name := range []string{"sha256", "hashVerified", "readbackVerified"} {
		if _, ok := properties[name]; ok {
			t.Errorf("P1 field unexpectedly declared: %s", name)
		}
	}
}

func TestCrossPlatformCoverageMinutesPreviewPlanFinalDelivery(t *testing.T) {
	for _, tc := range []struct {
		path   string
		fields []string
		cue    string
	}{
		{"minutes +share", []string{"permission", "options", "failurePolicy"}, "预览显示目标和失败策略"},
		{"minutes +unshare", []string{"failurePolicy"}, "预览显示目标和失败策略"},
		{"minutes +upload", []string{"options", "completeTimeoutSeconds", "pollIntervalSeconds"}, "预览包含显式语言"},
		{"minutes +upload-and-notify", []string{"options", "completeTimeoutSeconds", "pollIntervalSeconds"}, "预览包含显式语言"},
	} {
		full := executeShortcutSchemaQuery(t, "--cli-path", tc.path)
		compact := executeShortcutSchemaQuery(t, "--cli-path", tc.path, "--compact")
		if !reflect.DeepEqual(full["result"], compact["result"]) {
			t.Fatalf("%s result drift", tc.path)
		}
		properties := schemaContractMap(schemaContractMap(schemaContractMap(full["result"])["data_schema"])["properties"])
		for _, field := range tc.fields {
			if schemaContractString(schemaContractMap(properties[field])["description"]) == "" {
				t.Fatalf("%s missing %s", tc.path, field)
			}
		}
		if full["confirmation"] != "user_required" {
			t.Fatalf("%s confirmation drift", tc.path)
		}
		if !strings.Contains(schemaContractString(full["description"]), tc.cue) {
			t.Fatalf("%s Schema lost intent", tc.path)
		}
		root := NewRootCommand()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetErr(&output)
		root.SetArgs(append(strings.Fields(tc.path), "--help"))
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), tc.cue) {
			t.Fatalf("%s Help lost intent", tc.path)
		}
	}
}

func TestCrossPlatformCoverageMinutesStaffIDPermissionFinalSchema(t *testing.T) {
	full := executeShortcutSchemaQuery(t, "--cli-path", "minutes +share")
	compact := executeShortcutSchemaQuery(t, "--cli-path", "minutes +share", "--compact")
	if !reflect.DeepEqual(full["result"], compact["result"]) {
		t.Fatal("compact result drift")
	}
	properties := schemaContractMap(schemaContractMap(schemaContractMap(full["result"])["data_schema"])["properties"])
	for _, name := range []string{"results", "failures"} {
		item := schemaContractMap(schemaContractMap(properties[name])["items"])
		fields := schemaContractMap(item["properties"])
		if schemaContractString(schemaContractMap(fields["memberStaffId"])["description"]) == "" {
			t.Fatalf("%s lost staffId receipt declaration", name)
		}
	}
}

func TestCrossPlatformCoverageMinutesSpeakerFinalSchema(t *testing.T) {
	full := executeShortcutSchemaQuery(t, "--cli-path", "minutes +speaker-insights")
	compact := executeShortcutSchemaQuery(t, "--cli-path", "minutes +speaker-insights", "--compact")
	if !reflect.DeepEqual(full["result"], compact["result"]) {
		t.Fatal("compact result differs")
	}
	result := schemaContractMap(full["result"])
	properties := schemaContractMap(schemaContractMap(result["data_schema"])["properties"])
	for _, name := range []string{"state", "complete", "taskUuid", "taskId", "createStatus", "attempts", "retryable", "recovery", "result"} {
		if schemaContractString(schemaContractMap(properties[name])["description"]) == "" {
			t.Errorf("missing %s", name)
		}
	}
	if full["confirmation"] != "user_required" {
		t.Fatal("confirmation changed")
	}
}
