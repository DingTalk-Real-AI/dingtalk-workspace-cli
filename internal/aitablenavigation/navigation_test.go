// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitablenavigation

import (
	"net/url"
	"reflect"
	"testing"
)

func TestCrossPlatformCoveragePreserveNavigation(t *testing.T) {
	receipt := map[string]any{"url": "receipt", "links": []any{"link"}, "appId": "encoded", "status": "pending"}
	target := map[string]any{"url": "readback", "status": "verified"}
	Preserve(target, receipt)
	if target["url"] != "readback" || target["appId"] != "encoded" || target["status"] != "verified" {
		t.Fatal(target)
	}
	if !reflect.DeepEqual(target["links"], receipt["links"]) {
		t.Fatal(target)
	}
	old := map[string]any{}
	Preserve(old, nil)
	if len(old) != 0 {
		t.Fatal(old)
	}
}

func TestCrossPlatformCoverageVerifiedFieldsNavigation(t *testing.T) {
	definitions := []any{map[string]any{"fieldName": "标题"}, "invalid", map[string]any{"fieldName": 1}}
	result := Fields("b", "s &%中文", []string{"f=+", "f2", "f3", "f4"}, definitions)
	fields := result["fields"].([]any)
	if len(fields) != 4 || len(result["links"].([]any)) != 6 {
		t.Fatal(result)
	}
	first := fields[0].(map[string]any)
	if first["fieldName"] != "标题" {
		t.Fatal(first)
	}
	u, err := url.Parse(first["url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	q, err := url.ParseQuery(u.Query().Get("iframeQuery"))
	if err != nil {
		t.Fatal(err)
	}
	if len(q) != 3 || q.Get("entrance") != "data" || q.Get("sheetId") != "s &%中文" || q.Get("fieldId") != "f=+" {
		t.Fatal(q)
	}
	links := result["links"].([]any)
	u, err = url.Parse(links[4].(map[string]any)["url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	q, err = url.ParseQuery(u.Query().Get("iframeQuery"))
	if err != nil || len(q) != 2 || q.Get("entrance") != "data" || q.Get("sheetId") != "s &%中文" {
		t.Fatal(q, err)
	}
	for _, tc := range [][3]string{{"", "s", "f"}, {"b", " ", "f"}, {"b", "s", ""}} {
		if len(Fields(tc[0], tc[1], []string{tc[2]}, nil)["fields"].([]any)) != 0 {
			t.Fatal(tc)
		}
	}
}
