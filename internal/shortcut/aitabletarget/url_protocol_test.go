// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitabletarget

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageNavigationRejectsRetiredTargetParameters(t *testing.T) {
	for _, key := range []string{"targetSheetId", "targetViewId", "targetFieldId", "targetAppId", "targetPageId", "targetApplicationId", "targetFlowId", "targetRecordId", "targetFutureId", "TARGETSHEETID"} {
		for _, current := range []string{"", "sheetId=s&"} {
			for _, nested := range []bool{false, true} {
				query := current + key + "=old"
				if nested {
					query = url.Values{"iframeQuery": {query}}.Encode()
				}
				if got, err := ParseURL("https://alidocs.dingtalk.com/i/nodes/b?" + query); err == nil || !strings.Contains(err.Error(), "无法解析") {
					t.Fatalf("retired locator accepted: %s: %#v, %v", query, got, err)
				}
			}
		}
	}
}

func TestCrossPlatformCoverageNavigationURLObjects(t *testing.T) {
	cases := []struct {
		query string
		want  Target
	}{
		{"entrance=data&sheetId=s", Target{Kind: "table", TableID: "s", Entrance: "data"}},
		{"sheetId=s&viewId=v", Target{Kind: "view", TableID: "s", ViewID: "v"}},
		{"entrance=form&sheetId=s&viewId=v", Target{Kind: "form", TableID: "s", ViewID: "v", Entrance: "form"}},
		{"entrance=data&sheetId=s&fieldId=f&viewId=v", Target{Kind: "field", TableID: "s", FieldID: "f", ViewID: "v", Entrance: "data"}},
		{"entrance=apps&appId=a", Target{Kind: "app", AppID: "a", Entrance: "apps"}},
		{"entrance=apps&appId=a&pageId=p", Target{Kind: "app_page", AppID: "a", PageID: "p", Entrance: "apps"}},
		{"entrance=automation&flowId=9007199254740993&autoOpenAutomation=true", Target{Kind: "workflow", FlowID: "9007199254740993", Entrance: "automation"}},
		{"applicationId=d", Target{Kind: "application", ApplicationID: "d"}},
		{"sheetId=s&rowId=r", Target{Kind: "record", TableID: "s", RecordID: "r"}},
		{"record=s_v_r_1", Target{Kind: "record", TableID: "s", ViewID: "v", RecordID: "r"}},
		{"record=s__r", Target{Kind: "record", TableID: "s", RecordID: "r"}},
		{"record=s_v_r&recordId=r&sheetId=s&viewId=v", Target{Kind: "record", TableID: "s", ViewID: "v", RecordID: "r"}},
		{"entrance=automation", Target{Kind: "base", Entrance: "automation"}},
		{"entrance=automation&sheetId=s", Target{Kind: "base", Entrance: "automation"}},
		{"entrance=automation&sheetId=s&viewId=v", Target{Kind: "base", Entrance: "automation"}},
		{"entrance=automation&applicationId=background", Target{Kind: "base", Entrance: "automation"}},
		{"entrance=automation&sheetId=s&viewId=v&flowId=f", Target{Kind: "workflow", FlowID: "f", Entrance: "automation"}},
		{"entrance=automation&applicationId=background&flowId=f", Target{Kind: "workflow", FlowID: "f", Entrance: "automation"}},
		{"preview=compact", Target{Kind: "base"}},
	}
	for _, tc := range cases {
		for _, nested := range []bool{false, true} {
			query := tc.query
			if nested {
				query = url.Values{"iframeQuery": {query}}.Encode()
			}
			t.Run(query, func(t *testing.T) {
				want := tc.want
				want.BaseID = "b"
				want.Source = "url"
				want.SchemaVersion = "aitable.target.v1"
				got, err := ParseURL("https://alidocs.dingtalk.com/i/nodes/b?" + query)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("got %#v, %v; want %#v", got, err, want)
				}
			})
		}
	}
	got, err := ParseURL("https://docs.dingtalk.com/notable/record?dentryUuid=b&sheetId=s&viewId=v&rowId=r")
	if err != nil || got.Kind != "record" || got.BaseID != "b" || got.ViewID != "v" {
		t.Fatal(got, err)
	}
	got, err = ParseURL("https://alidocs.dingtalk.com/i/nodes/b?sheetId=s&iframeQuery=sheetId%3Ds%26fieldId%3Df")
	if err != nil || got.FieldID != "f" {
		t.Fatal(got, err)
	}
}

func TestCrossPlatformCoverageNavigationURLRejectsIncompleteOrConflictingObjects(t *testing.T) {
	queries := []string{
		"fieldId=f", "pageId=p", "entrance=other", "entrance=form&sheetId=s",
		"sheetId=s&fieldId=f&recordId=r", "sheetId=s&appId=a", "sheetId=s&applicationId=d", "appId=a&flowId=f",
		"entrance=data&appId=a", "entrance=apps&sheetId=s", "entrance=form&sheetId=s&fieldId=f",
		"entrance=data&flowId=f",
		"entrance=automation&sheetId=s&fieldId=f", "entrance=automation&sheetId=s&rowId=r",
		"entrance=automation&appId=a&pageId=p", "entrance=automation&viewId=v",
		"entrance=automation&sheetId=s&iframeQuery=sheetId%3Dother",
		"dentryUuid=other", "sheetId=", "fieldId=%20&sheetId=s", "pageId=&appId=a", "flowId=f&flowId=other",
		"record=", "record=one&record=two", "record=bad", "record=_v_r", "record=s_v_", "record=s_v_r_bad", "record=s_v_r_-1", "record=s_v_r_1_extra",
		"record=s_v_r&recordId=other", "record=s_v_r&sheetId=other", "sheetId=bad%2Fid", "sheetId=s&viewId=%7F",
		"iframeQuery=sheetId%3Ds&iframeQuery=sheetId%3Ds", "iframeQuery=iframeQuery%3DsheetId%253Ds", "sheetId=%zz", "sheetId=s;viewId=v",
	}
	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			if got, err := ParseURL("https://alidocs.dingtalk.com/i/nodes/b?" + query); err == nil {
				t.Fatalf("accepted %#v", got)
			}
		})
	}
	for _, raw := range []string{
		"https://alidocs.dingtalk.com/notable/record?sheetId=s&rowId=r",
		"https://alidocs.dingtalk.com/notable/record?dentryUuid=b&rowId=r",
		"https://alidocs.dingtalk.com/notable/record?dentryUuid=b&sheetId=s",
		"https://alidocs.dingtalk.com/notable/share/form/uuid",
		"https://alidocs.dingtalk.com/i/nodes/b?sheetId=s&iframeQuery=sheetId%3Dother",
	} {
		if got, err := ParseURL(raw); err == nil {
			t.Fatalf("accepted %s: %#v", raw, got)
		}
	}
}
