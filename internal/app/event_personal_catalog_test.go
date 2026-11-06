// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func startPersonalCatalogSeams(t *testing.T, configDir string) {
	t.Helper()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	testseam.Swap(t, &personalResolveEventIdentity, func(context.Context, string, string, ...personalIdentityOptions) (personal.Identity, error) {
		return personal.Identity{ClientID: "client", SourceID: "open"}, nil
	})
	testseam.Swap(t, &personalResolveAuxiliaryAccessToken, func(context.Context, string, string) (string, error) {
		return "runtime-token", nil
	})
}

func TestCrossPlatformCoveragePersonalEventCatalogServesOfficialBuiltinAndAppEventsRemote(t *testing.T) {
	configDir := t.TempDir()
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/dws/event/catalog" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("x-user-access-token"); got != "runtime-token" {
			t.Fatalf("x-user-access-token = %q", got)
		}
		if got := r.URL.Query().Get("sourceType"); got != personal.CatalogSourceTypeAppEvent {
			t.Fatalf("sourceType query = %q, want appEvent", got)
		}
		// mock 镜像服务端语义：keyword 为 eventKey 全值等值（忽略大小写）过滤。
		events := []map[string]any{
			{"eventKey": "user_custom_remote_event", "name": "自定义事件", "eventSource": "appEvent", "producerUnifiedAppId": "producer-app-1", "description": "自定义事件描述", "scope": "8a9d-scope-demo"},
			// 与官方内置键同名：官方定义优先，远端同名条目必须被跳过。
			{"eventKey": personal.EventMention, "name": "同名远端事件", "eventSource": "appEvent"},
		}
		if kw := r.URL.Query().Get("keyword"); kw != "" {
			var filtered []map[string]any
			for _, ev := range events {
				if key, _ := ev["eventKey"].(string); strings.EqualFold(key, kw) {
					filtered = append(filtered, ev)
				}
			}
			events = filtered
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"total": len(events),
				"categories": []map[string]any{
					{
						"category": "appEvent",
						"events":   events,
					},
				},
			},
		})
	}))
	defer srv.Close()
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(srv.URL+"\n"), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}
	startPersonalCatalogSeams(t, configDir)

	var warn bytes.Buffer
	items, err := loadPersonalEventCatalog(context.Background(), personalListOptions{}, &warn)
	if err != nil {
		t.Fatalf("loadPersonalEventCatalog() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if warn.Len() != 0 {
		t.Fatalf("unexpected warning: %s", warn.String())
	}
	// 官方事件来自内置目录：元数据不受远端同名条目影响。
	mention, ok := findPersonalCatalogItem(items, personal.EventMention)
	if !ok || mention.DisplayName != "@我的消息" || mention.EventSource != "" {
		t.Fatalf("official definition = %#v, found=%v", mention, ok)
	}
	if _, ok := findPersonalCatalogItem(items, personal.EventCardAction); !ok {
		t.Fatal("built-in official catalog missing card events")
	}
	appEvent, ok := findPersonalCatalogItem(items, "user_custom_remote_event")
	if !ok || appEvent.Category != "appEvent" || appEvent.Status != personal.StatusRemote ||
		appEvent.EventSource != "appEvent" || appEvent.ProducerUnifiedAppID != "producer-app-1" || appEvent.Description != "自定义事件描述" ||
		appEvent.Scope != "8a9d-scope-demo" {
		t.Fatalf("app event definition = %#v, found=%v", appEvent, ok)
	}

	// 官方分类：直接读内置目录，不触发远端请求。
	items, err = loadPersonalEventCatalog(context.Background(), personalListOptions{Category: " card "}, &warn)
	if err != nil {
		t.Fatalf("official-category loadPersonalEventCatalog() error = %v", err)
	}
	if requests != 1 || len(items) == 0 {
		t.Fatalf("official-category requests/items = %d/%d, want no extra remote call", requests, len(items))
	}
	for _, item := range items {
		if item.Category != "card" {
			t.Fatalf("official-category leaked event: %#v", item)
		}
	}

	// appEvent 分类：只有远端应用事件。
	items, err = loadPersonalEventCatalog(context.Background(), personalListOptions{Category: "appEvent"}, &warn)
	if err != nil {
		t.Fatalf("appEvent-category loadPersonalEventCatalog() error = %v", err)
	}
	if requests != 2 || len(items) != 1 || items[0].EventKey != "user_custom_remote_event" {
		t.Fatalf("appEvent-category requests/items = %d/%#v", requests, items)
	}

	// keyword 在官方侧按 eventKey 全值等值（忽略大小写）过滤。
	items, err = loadPersonalEventCatalog(context.Background(), personalListOptions{Keyword: personal.EventCardAction}, &warn)
	if err != nil {
		t.Fatalf("keyword loadPersonalEventCatalog() error = %v", err)
	}
	if len(items) != 1 || items[0].EventKey != personal.EventCardAction {
		t.Fatalf("keyword items = %#v", items)
	}

	// producer 筛选只可能命中应用事件：官方目录整体让位。
	items, err = loadPersonalEventCatalog(context.Background(), personalListOptions{ProducerAppID: "producer-app-1"}, &warn)
	if err != nil {
		t.Fatalf("producer loadPersonalEventCatalog() error = %v", err)
	}
	if len(items) != 1 || items[0].ProducerUnifiedAppID != "producer-app-1" {
		t.Fatalf("producer items = %#v", items)
	}
}

func TestCrossPlatformCoveragePersonalEventCatalogErrorsPropagate(t *testing.T) {
	for _, tc := range []struct {
		code string
		msg  string
	}{
		{code: "REMOTE_SERVICE_UNAVAILABLE", msg: "事件服务暂不可用"},
		{code: "SYSTEM_ERROR", msg: "系统内部错误"},
	} {
		configDir := t.TempDir()
		var paths []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.Path)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success":   false,
				"errorCode": tc.code,
				"errorMsg":  tc.msg,
				"arguments": []string{"trace-1"},
			})
		}))
		t.Cleanup(srv.Close)
		if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(srv.URL+"\n"), 0o600); err != nil {
			t.Fatalf("write mcp_url: %v", err)
		}
		startPersonalCatalogSeams(t, configDir)

		// 未加筛选的浏览：官方事件完整可用，应用事件源故障只降级告警。
		var warn bytes.Buffer
		items, err := loadPersonalEventCatalog(context.Background(), personalListOptions{}, &warn)
		if err != nil {
			t.Fatalf("%s: unfiltered loadPersonalEventCatalog() error = %v", tc.code, err)
		}
		if _, ok := findPersonalCatalogItem(items, personal.EventCardAction); !ok {
			t.Fatalf("%s: unfiltered listing lost built-in official events", tc.code)
		}
		if !strings.Contains(warn.String(), "WARN") {
			t.Fatalf("%s: degraded listing was silent: %q", tc.code, warn.String())
		}

		// 圈定应用事件的查询必须 fail-closed：不返回可能缺失命中项的假完整结果。
		warn.Reset()
		_, err = loadPersonalEventCatalog(context.Background(), personalListOptions{Category: "appEvent"}, &warn)
		if err == nil {
			t.Fatalf("%s: app-scoped loadPersonalEventCatalog() unexpectedly returned nil error", tc.code)
		}
		var apiErr *personal.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("%s: app-scoped error = %T(%v), want personal.APIError", tc.code, err, err)
		}
		if apiErr.Code != tc.code {
			t.Fatalf("%s: APIError.Code = %q", tc.code, apiErr.Code)
		}
		if warn.Len() != 0 {
			t.Fatalf("%s: app-scoped failure also warned: %s", tc.code, warn.String())
		}
		if len(paths) != 2 || paths[0] != "/dws/event/catalog" || paths[1] != "/dws/event/catalog" {
			t.Fatalf("%s: request paths = %#v, want two /dws/event/catalog", tc.code, paths)
		}
	}
}

func TestCrossPlatformCoveragePersonalEventCatalogRemoteFailureFailsClosedWhenFiltered(t *testing.T) {
	configDir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "errorCode": "INTERNAL", "errorMsg": "boom"})
	}))
	defer srv.Close()
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(srv.URL+"\n"), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}
	startPersonalCatalogSeams(t, configDir)

	_, err := loadPersonalEventCatalog(context.Background(), personalListOptions{Keyword: "order"}, io.Discard)
	if err == nil {
		t.Fatal("filtered remote failure unexpectedly returned nil error")
	}
	var apiErr *personal.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("filtered remote failure error = %T(%v), want personal.APIError", err, err)
	}
}

func TestCrossPlatformCoveragePersonalEventCatalogEmptyAppSourceIsLegitimate(t *testing.T) {
	configDir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result":  map[string]any{"total": 0, "categories": []map[string]any{}},
		})
	}))
	defer srv.Close()
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(srv.URL+"\n"), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}
	startPersonalCatalogSeams(t, configDir)

	// 组织没有已发布应用事件是正常空态：官方事件照常展示，且不告警。
	var warn bytes.Buffer
	items, err := loadPersonalEventCatalog(context.Background(), personalListOptions{}, &warn)
	if err != nil {
		t.Fatalf("empty app source error = %v", err)
	}
	if _, ok := findPersonalCatalogItem(items, personal.EventCardAction); !ok {
		t.Fatal("empty app source wiped the built-in card events")
	}
	if warn.Len() != 0 {
		t.Fatalf("empty app source warned: %q", warn.String())
	}

	// keyword 无命中是合法空答案：保持为空且安静。
	items, err = loadPersonalEventCatalog(context.Background(), personalListOptions{Keyword: "order"}, &warn)
	if err != nil {
		t.Fatalf("filtered empty catalog error = %v", err)
	}
	if len(items) != 0 || warn.Len() != 0 {
		t.Fatalf("filtered empty result = %d items, warn %q", len(items), warn.String())
	}
}

func TestCrossPlatformCoveragePersonalEventCatalogCredentialsOnlyGateAppEvents(t *testing.T) {
	testseam.Swap(t, &personalResolveEventIdentity, func(context.Context, string, string, ...personalIdentityOptions) (personal.Identity, error) {
		return personal.Identity{}, authpkg.ErrTokenDataNotFound
	})
	var warn bytes.Buffer
	items, err := loadPersonalEventCatalog(context.Background(), personalListOptions{}, &warn)
	if err != nil {
		t.Fatalf("missing-credentials error = %v", err)
	}
	if _, ok := findPersonalCatalogItem(items, personal.EventCardAction); !ok {
		t.Fatal("missing credentials wiped built-in official events")
	}
	if !strings.Contains(warn.String(), "WARN") {
		t.Fatalf("missing-credentials listing was silent: %q", warn.String())
	}

	// 纯官方分类不触碰远端：无凭证也无需告警。
	warn.Reset()
	items, err = loadPersonalEventCatalog(context.Background(), personalListOptions{Category: "card"}, &warn)
	if err != nil {
		t.Fatalf("official-category missing-credentials error = %v", err)
	}
	if len(items) == 0 {
		t.Fatal("official-category missing-credentials returned no static card events")
	}
	if warn.Len() != 0 {
		t.Fatalf("official-category missing-credentials warned: %q", warn.String())
	}

	wantErr := errors.New("keychain unavailable")
	testseam.Swap(t, &personalResolveEventIdentity, func(context.Context, string, string, ...personalIdentityOptions) (personal.Identity, error) {
		return personal.Identity{}, wantErr
	})
	if _, err := loadPersonalEventCatalog(context.Background(), personalListOptions{}, io.Discard); !errors.Is(err, wantErr) {
		t.Fatalf("non-credential error = %v, want %v", err, wantErr)
	}
}

func TestCrossPlatformCoveragePersonalEventSchemaResolvesRemoteAppEvent(t *testing.T) {
	configDir := t.TempDir()
	var gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/dws/event/schema" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("eventKey"); got != "order created" {
			t.Fatalf("eventKey query = %q", got)
		}
		gotVersion = r.URL.Query().Get("payloadSchemaVersion")
		if got := r.Header.Get("x-user-access-token"); got != "runtime-token" {
			t.Fatalf("x-user-access-token = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"eventKey":             "order created",
				"displayName":          "订单创建",
				"description":          "订单创建后通知订阅人",
				"category":             "appEvent",
				"producerUnifiedAppId": "producer-app",
				"payloadSchemaVersion": "4",
				"payloadSchema": map[string]any{
					"type":       "object",
					"properties": map[string]any{"orderId": map[string]any{"type": "string"}},
				},
				"scope": "example.order.read",
			},
		})
	}))
	defer srv.Close()
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(srv.URL+"\n"), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}
	startPersonalCatalogSeams(t, configDir)

	var out bytes.Buffer
	cmd := newEventSchemaCommand()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"order created"})
	if err := corecmd.ExecuteForTest(cmd); err != nil {
		t.Fatalf("event schema remote error = %v", err)
	}
	if gotVersion != "" {
		t.Fatalf("payloadSchemaVersion query = %q, want omitted", gotVersion)
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("decode remote schema: %v\n%s", err, out.String())
	}
	if doc["event_key"] != "order created" || doc["display_name"] != "订单创建" ||
		doc["producer_unified_app_id"] != "producer-app" || doc["scope"] != "example.order.read" ||
		doc["jq_root_path"] != ".payload" || doc["payload_schema_version"] != "4" {
		t.Fatalf("remote schema document = %#v", doc)
	}
	schema, _ := doc["schema"].(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	if schema["type"] != "object" || props["orderId"] == nil {
		t.Fatalf("remote payload schema = %#v", doc["schema"])
	}

	versionOut := bytes.Buffer{}
	versionCmd := newEventSchemaCommand()
	versionCmd.SetOut(&versionOut)
	versionCmd.SetArgs([]string{"order created", "--payload-schema-version", "3"})
	if err := corecmd.ExecuteForTest(versionCmd); err != nil {
		t.Fatalf("event schema --payload-schema-version error = %v", err)
	}
	if gotVersion != "3" {
		t.Fatalf("payloadSchemaVersion query = %q, want 3", gotVersion)
	}

	formatCmd := newEventSchemaCommand()
	formatCmd.SetOut(&bytes.Buffer{})
	formatCmd.SetArgs([]string{"order created", "--format", "table"})
	if err := corecmd.ExecuteForTest(formatCmd); err == nil || !strings.Contains(err.Error(), "only supports json output") {
		t.Fatalf("app-event schema format error = %v", err)
	}
	flattenCmd := newEventSchemaCommand()
	var flattenOut bytes.Buffer
	flattenCmd.SetOut(&flattenOut)
	flattenCmd.SetArgs([]string{"order created", "--flatten"})
	if err := corecmd.ExecuteForTest(flattenCmd); err != nil {
		t.Fatalf("app-event schema --flatten error = %v", err)
	}
	if !strings.Contains(flattenOut.String(), `"jq_root_path": ".payload"`) {
		t.Fatalf("app-event schema --flatten output = %s", flattenOut.String())
	}
}

func TestCrossPlatformCoveragePersonalEventSchemaVersionErrorsAndOfficialReject(t *testing.T) {
	configDir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := "SCHEMA_VERSION_NOT_FOUND"
		if r.URL.Query().Get("payloadSchemaVersion") == "DRAFT" {
			code = "ILLEGAL_PARAMETER"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":   false,
			"errorCode": code,
			"errorMsg":  "invalid schema query",
		})
	}))
	defer srv.Close()
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(srv.URL+"\n"), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}
	startPersonalCatalogSeams(t, configDir)

	cmd := newEventSchemaCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"order created", "--payload-schema-version", "9"})
	err := corecmd.ExecuteForTest(cmd)
	if err == nil || !strings.Contains(err.Error(), "not published") {
		t.Fatalf("schema version not found error = %v", err)
	}

	cmd = newEventSchemaCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"order created", "--payload-schema-version", "DRAFT"})
	err = corecmd.ExecuteForTest(cmd)
	if err == nil || !strings.Contains(err.Error(), "invalid payload schema version") {
		t.Fatalf("illegal schema version error = %v", err)
	}

	official := newEventSchemaCommand()
	official.SetOut(&bytes.Buffer{})
	official.SetArgs([]string{"user_im_message_receive_at", "--payload-schema-version", "1"})
	err = corecmd.ExecuteForTest(official)
	if err == nil || !strings.Contains(err.Error(), "only applies to app events") {
		t.Fatalf("official event version flag error = %v", err)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || typed.ExitCode() != 3 {
		t.Fatalf("official event version flag error = %v (%T), want validation with exit code 3", err, err)
	}
}

// TestCrossPlatformCoveragePersonalEventSchemaOfficialVersionFlagProcessExitCode
// covers the full process exit path: a built-in official event carrying
// --payload-schema-version must classify as validation and exit 3, not internal/5.
func TestCrossPlatformCoveragePersonalEventSchemaOfficialVersionFlagProcessExitCode(t *testing.T) {
	configDir := t.TempDir()
	startPersonalCatalogSeams(t, configDir)
	testseam.Protect(t, &os.Args)
	os.Args = []string{"dws", "event", "schema", "user_im_message_receive_at", "--payload-schema-version", "1"}
	testseam.Swap(t, &rootNormalizeProcessProfileArgs, func() func() { return func() {} })
	testseam.Swap(t, &rootRunPreParse, func(*cobra.Command, *pipeline.Engine) error { return nil })
	testseam.Swap(t, &rootStopAllStdioClients, func() {})

	if code := Execute(); code != 3 {
		t.Fatalf("Execute exit code = %d, want validation code 3", code)
	}
}

func TestCrossPlatformCoveragePersonalEventSchemaRemoteUnknownAndLoggedOut(t *testing.T) {
	configDir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":   false,
			"errorCode": "EVENT_NOT_AVAILABLE",
			"errorMsg":  "event unavailable",
			"arguments": []string{"trace-1"},
		})
	}))
	defer srv.Close()
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(srv.URL+"\n"), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}
	startPersonalCatalogSeams(t, configDir)

	cmd := newEventSchemaCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"missing_event"})
	err := corecmd.ExecuteForTest(cmd)
	if err == nil || !strings.Contains(err.Error(), "unknown personal event key") {
		t.Fatalf("unknown remote event error = %v", err)
	}

	schemaUnavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":   false,
			"errorCode": "SCHEMA_NOT_AVAILABLE",
			"errorMsg":  "official event schema unavailable",
			"arguments": []string{"trace-2"},
		})
	}))
	t.Cleanup(schemaUnavailable.Close)
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(schemaUnavailable.URL+"\n"), 0o600); err != nil {
		t.Fatalf("rewrite mcp_url: %v", err)
	}
	cmd = newEventSchemaCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"official_new_event"})
	err = corecmd.ExecuteForTest(cmd)
	if err == nil || !strings.Contains(err.Error(), "upgrade dws") {
		t.Fatalf("schema-not-available error = %v", err)
	}

	testseam.Swap(t, &personalResolveEventIdentity, func(context.Context, string, string, ...personalIdentityOptions) (personal.Identity, error) {
		return personal.Identity{}, authpkg.ErrTokenDataNotFound
	})
	cmd = newEventSchemaCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"missing_event"})
	err = corecmd.ExecuteForTest(cmd)
	if err == nil || !strings.Contains(err.Error(), "requires login") {
		t.Fatalf("logged-out remote schema error = %v", err)
	}
}

func findPersonalCatalogItem(items []personal.Definition, eventKey string) (personal.Definition, bool) {
	for _, item := range items {
		if item.EventKey == eventKey {
			return item, true
		}
	}
	return personal.Definition{}, false
}

func TestCrossPlatformCoveragePersonalEventListWarnNilAndRunErrorWrap(t *testing.T) {
	healthyDir := t.TempDir()
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]any{"total": 0, "items": []map[string]any{}}})
	}))
	defer healthy.Close()
	if err := os.WriteFile(filepath.Join(healthyDir, "mcp_url"), []byte(healthy.URL+"\n"), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}
	startPersonalCatalogSeams(t, healthyDir)
	// warn == nil falls back to io.Discard without panic on a healthy catalog.
	if _, err := loadPersonalEventCatalog(context.Background(), personalListOptions{}, nil); err != nil {
		t.Fatalf("nil-warn catalog error = %v", err)
	}

	configDir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(srv.URL+"\n"), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}
	startPersonalCatalogSeams(t, configDir)

	// Filtered catalog failure propagates through runPersonalEventList with the
	// "event list --as user" wrap.
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := runPersonalEventList(cmd, personalListOptions{Keyword: "app_user_never_match"})
	if err == nil || !strings.Contains(err.Error(), "event list --as user") {
		t.Fatalf("runPersonalEventList error = %v", err)
	}
}

func TestCrossPlatformCoveragePersonalEventCatalogCategorySkipAndDescriptionFallback(t *testing.T) {
	configDir := t.TempDir()
	var categoryQueries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("sourceType"); got != personal.CatalogSourceTypeAppEvent {
			t.Fatalf("sourceType query = %q, want appEvent", got)
		}
		categoryQueries = append(categoryQueries, r.URL.Query().Get("category"))
		events := []map[string]any{
			{"eventKey": "app_user_other_created_event", "name": "Other", "category": "other", "event_source": "appEvent"},
			{"eventKey": "app_user_demo_created_event", "name": "Demo", "category": "appEvent", "event_source": "appEvent"},
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result":  map[string]any{"total": len(events), "items": events},
		})
	}))
	defer srv.Close()
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(srv.URL+"\n"), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}
	startPersonalCatalogSeams(t, configDir)

	// appEvent 是来源选择器，不是业务分类过滤：sourceType=appEvent 返回的
	// other/appEvent 分类都必须保留。
	items, err := loadPersonalEventCatalog(context.Background(), personalListOptions{Category: "appEvent"}, nil)
	if err != nil {
		t.Fatalf("catalog error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %#v, want both app-event categories", items)
	}
	demo, ok := findPersonalCatalogItem(items, "app_user_demo_created_event")
	if !ok {
		t.Fatalf("items = %#v, want the appEvent-category entry", items)
	}
	if demo.Description != "Demo" {
		t.Fatalf("empty description should fall back to name, got %q", demo.Description)
	}

	// 远端独有分类仍要访问应用事件目录，并按业务分类 other 过滤。
	items, err = loadPersonalEventCatalog(context.Background(), personalListOptions{Category: "other"}, nil)
	if err != nil {
		t.Fatalf("other-category catalog error = %v", err)
	}
	if len(items) != 1 || items[0].EventKey != "app_user_other_created_event" {
		t.Fatalf("other-category items = %#v, want only the other-category app event", items)
	}
	if len(categoryQueries) != 2 || categoryQueries[0] != "" || categoryQueries[1] != "other" {
		t.Fatalf("category queries = %#v, want [\"\", other] (appEvent is a source selector)", categoryQueries)
	}
}

func TestCrossPlatformCoveragePersonalEventCatalogProducerOverridesOfficialCategoryShortCircuit(t *testing.T) {
	configDir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("sourceType"); got != personal.CatalogSourceTypeAppEvent {
			t.Fatalf("sourceType query = %q, want appEvent", got)
		}
		if got := r.URL.Query().Get("category"); got != "card" {
			t.Fatalf("category query = %q, want card", got)
		}
		if got := r.URL.Query().Get("producerUnifiedAppId"); got != "producer-card" {
			t.Fatalf("producer query = %q, want producer-card", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"total": 1,
				"categories": []map[string]any{{
					"category": "card",
					"events": []map[string]any{{
						"eventKey": "app_user_card_created_event", "name": "Card app event",
						"category": "card", "event_source": "appEvent", "producerUnifiedAppId": "producer-card",
					}},
				}},
			},
		})
	}))
	defer srv.Close()
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(srv.URL+"\n"), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}
	startPersonalCatalogSeams(t, configDir)

	items, err := loadPersonalEventCatalog(context.Background(), personalListOptions{Category: "card", ProducerAppID: "producer-card"}, nil)
	if err != nil {
		t.Fatalf("producer official-category catalog error = %v", err)
	}
	if len(items) != 1 || items[0].EventKey != "app_user_card_created_event" {
		t.Fatalf("items = %#v, want the remote card-category app event", items)
	}
}

func TestCrossPlatformCoverageRemoteEventSchemaDefaultFormatAndErrorWrap(t *testing.T) {
	configDir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(srv.URL+"\n"), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}
	startPersonalCatalogSeams(t, configDir)

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetContext(context.Background())
	// Empty format defaults to json; a generic transport failure is wrapped by
	// runRemoteEventSchema instead of a specialized not-found mapping.
	err := runRemoteEventSchema(cmd, "app_user_demo_created_event", "", false, "", "", "")
	if err == nil || !strings.Contains(err.Error(), "event schema") {
		t.Fatalf("runRemoteEventSchema error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected output on failure: %q", out.String())
	}
}

func TestCrossPlatformCoverageEnsurePersonalSubscriptionAppEventRuleFallback(t *testing.T) {
	configDir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": map[string]any{
				"items": []map[string]any{{"subscribe_id": "sub-1", "source_id": "open"}},
			},
		})
	}))
	defer srv.Close()
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(srv.URL+"\n"), 0o600); err != nil {
		t.Fatalf("write mcp_url: %v", err)
	}
	startPersonalCatalogSeams(t, configDir)

	client := personal.NewClient(personalEventControlBaseURL("", configDir), personal.Identity{AccessToken: "token", ClientID: "client", SourceID: "open"})
	sub, eventKey, ruleType, err := ensurePersonalSubscription(context.Background(), client, personal.Identity{}, personalConsumeOptions{
		SubscribeID: "sub-1",
		EventKey:    "app_user_demo_created_event",
	})
	if err != nil {
		t.Fatalf("ensurePersonalSubscription error = %v", err)
	}
	if sub.SubscribeID != "sub-1" || eventKey != "app_user_demo_created_event" || ruleType != "all" {
		t.Fatalf("ensure = (%#v, %q, %q)", sub, eventKey, ruleType)
	}
}

func TestCrossPlatformCoverageRemoteEventSchemaWrapsNonCredentialIdentityError(t *testing.T) {
	configDir := t.TempDir()
	startPersonalCatalogSeams(t, configDir)
	testseam.Swap(t, &personalResolveEventIdentity, func(context.Context, string, string, ...personalIdentityOptions) (personal.Identity, error) {
		return personal.Identity{}, errors.New("identity backend unavailable")
	})
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := runRemoteEventSchema(cmd, "app_user_demo_created_event", "json", false, "", "", "")
	if err == nil || !strings.Contains(err.Error(), "event schema: identity backend unavailable") {
		t.Fatalf("identity error wrap = %v", err)
	}
}
