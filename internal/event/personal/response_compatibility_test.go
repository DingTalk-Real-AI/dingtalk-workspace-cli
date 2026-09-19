// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package personal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 使用兼容响应夹具覆盖列表字段和时间戳类型。
func TestCrossPlatformCoverageSubscriptionResponseCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		created     any
	}{
		{"items_string_control", "items", "2026-09-16T10:50:38Z"},
		{"list_string", "list", "2026-09-16T10:50:38Z"},
		{"items_numeric", "items", int64(1789555838000)},
		{"list_numeric", "list", int64(1789555838000)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/event/sublist" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]any{"total": 1, "pageNo": 1, "pageSize": 50, tc.field: []any{map[string]any{"subId": "sub-fixture", "eventKey": EventAllSingleChat, "sourceId": "digital_employee", "gmtCreate": tc.created}}}})
			}))
			defer srv.Close()
			c := NewClient(srv.URL, Identity{AccessToken: "local-fixture-only", ClientID: "repro-client", SourceID: "digital_employee"})
			subs, err := c.ListSubscriptions(t.Context(), ListOptions{})
			t.Logf("服务端夹具 total=1，列表字段=%s，gmtCreate=%T；客户端数量=%d，err=%v", tc.field, tc.created, len(subs), err)
			if err != nil {
				t.Fatalf("订阅响应解码失败: %v", err)
			}
			if len(subs) != 1 || subs[0].SubscribeID != "sub-fixture" {
				t.Fatalf("服务端返回 1 个订阅，客户端读取到 %d 个", len(subs))
			}
			if subs[0].CreatedAt != fmt.Sprint(tc.created) {
				t.Fatalf("时间戳未保留: %q", subs[0].CreatedAt)
			}
		})
	}
}

func TestCrossPlatformCoverageSubscriptionTimestampValidation(t *testing.T) {
	for _, raw := range []string{`null`, `""`, `1234567890123`, `"2026-01-01T00:00:00Z"`} {
		var value dwsSubscription
		if err := json.Unmarshal([]byte(`{"gmtCreate":`+raw+`}`), &value); err != nil {
			t.Fatalf("合法时间戳被拒绝: %s: %v", raw, err)
		}
	}
	for _, raw := range []string{`{}`, `[]`, `true`, `1.25`, `9223372036854775808`} {
		var value dwsSubscription
		if err := json.Unmarshal([]byte(`{"gmtCreate":`+raw+`}`), &value); err == nil {
			t.Fatalf("非法时间戳被接受: %s", raw)
		}
	}
}

func TestCrossPlatformCoverageSubscriptionListPaginationAndMissingItems(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page := r.URL.Query().Get("pageNo")
				result := map[string]any{"total": 2, "pageSize": 1}
				if !missing {
					result["list"] = []any{map[string]any{"subId": "fixture-" + page}}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": result})
			}))
			defer server.Close()
			client := NewClient(server.URL, Identity{AccessToken: "fixture-only"})
			subs, err := client.ListSubscriptions(t.Context(), ListOptions{})
			if missing {
				if err == nil {
					t.Fatal("total>0 且列表缺失不能伪装空列表")
				}
				return
			}
			if err != nil || len(subs) != 2 || subs[1].SubscribeID != "fixture-2" {
				t.Fatalf("list 分页失败: %+v %v", subs, err)
			}
		})
	}
}
