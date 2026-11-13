// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/aitabletarget"
)

// verifyURLTarget uses only existing read-only endpoints. In particular get_app
// and list_app_pages are excluded because they can initialize an App.
func verifyURLTarget(rt *shortcut.RuntimeContext, target aitabletarget.Target) (map[string]any, error) {
	tool, server, collection, id := "get_base", serverMain, "", target.BaseID
	keys := []string{"baseId", "base_id", "id"}
	params := map[string]any{"baseId": target.BaseID}
	switch target.Kind {
	case "app":
		return nil, apperrors.NewValidation("仅 App 链接暂无只读验证入口；请提供含 appId/pageId 的页面链接，或不传 --verify 仅解析", apperrors.WithReason("target_verification_unsupported"))
	case "record":
		tool, collection, id, keys = "query_records", "records", target.RecordID, []string{"recordId", "record_id", "id"}
		params["tableId"], params["recordIds"] = target.TableID, []string{target.RecordID}
	case "field":
		tool, collection, id, keys = "get_fields", "fields", target.FieldID, []string{"fieldId", "field_id", "id"}
		params["tableId"], params["fieldIds"] = target.TableID, []string{target.FieldID}
	case "view", "form":
		tool, collection, id, keys = "get_views", "views", target.ViewID, []string{"viewId", "view_id", "id"}
		params["tableId"], params["viewIds"] = target.TableID, []string{target.ViewID}
	case "table":
		tool, collection, id, keys = "get_tables", "tables", target.TableID, []string{"tableId", "table_id", "id"}
		params["tableIds"] = []string{target.TableID}
	case "app_page":
		tool, id, keys = "get_app_page", target.PageID, []string{"pageId"}
		params["pageId"] = target.PageID
	case "workflow":
		tool, server, id, keys = "get_workflow", serverHelper, target.FlowID, []string{"flowId", "workflowId"}
		params["workflowId"] = target.FlowID
	case "application":
		// get_base.documents[].documentId identifies the referenced document,
		// not the Base-local Application node consumed by Web navigation.
		tool, server, collection, id, keys = "list_nsheet_nodes", serverHelper, "items", target.ApplicationID, []string{"nodeId"}
	case "base":
	default:
		return nil, apperrors.NewValidation("不支持验证该目标类型", apperrors.WithReason("target_verification_unsupported"))
	}
	data, err := rt.CallMCPReadData(server, tool, params)
	if err != nil {
		return nil, err
	}
	body := urlResponseBody(data, target)
	object := urlFindObject(body, collection, id, keys...)
	entityType := target.Kind
	matched := object != nil && urlParentMatches(body, target) && urlParentMatches(object, target)
	switch target.Kind {
	case "application":
		entityType, _ = object["nodeType"].(string)
		_, directory := body["items"].([]any)
		matched = matched && directory && (entityType == "dashboard" || entityType == "document")
	case "form":
		matched = matched && urlObjectIDMatches(object, "FormDesigner", "viewType", "type")
	case "app_page":
		matched = matched && urlObjectIDMatches(object, target.AppID, "appId")
	}
	if !matched {
		return nil, urlVerificationFailed(tool, target)
	}
	// Optional view context must also exist in the same table. A valid record or
	// field alone cannot certify a stale or unrelated view carried in the URL.
	if (target.Kind == "field" || target.Kind == "record") && target.ViewID != "" {
		view := target
		view.Kind, view.FieldID, view.RecordID = "view", "", ""
		if _, err := verifyURLTarget(rt, view); err != nil {
			return nil, err
		}
		tool += "+get_views"
	}
	return map[string]any{"tool": tool, "entityType": entityType, "id": id, "status": "verified"}, nil
}

// urlResponseBody checks explicit parents before unwrapping each documented
// envelope. A nested object cannot override a conflicting outer identity.
func urlResponseBody(data map[string]any, target aitabletarget.Target) map[string]any {
	for depth := 0; depth < 4; depth++ {
		if !urlParentMatches(data, target) {
			return nil
		}
		if nested, ok := data["data"].(map[string]any); ok {
			data = nested
			continue
		}
		if nested, ok := data["result"].(map[string]any); ok {
			data = nested
			continue
		}
		return data
	}
	return nil
}

// urlFindObject requires one exact identity at the expected response location.
// Matching an ID inside config/request or another object's child is not evidence.
func urlFindObject(body map[string]any, collection, id string, keys ...string) map[string]any {
	if value, exists := body[collection]; collection != "" && exists {
		items, ok := value.([]any)
		if !ok {
			return nil
		}
		var found map[string]any
		for _, item := range items {
			object, ok := item.(map[string]any)
			if !ok {
				return nil
			}
			if urlObjectIDMatches(object, id, keys...) {
				if found != nil {
					return nil
				}
				found = object
			}
		}
		return found
	}
	if urlObjectIDMatches(body, id, keys...) {
		return body
	}
	return nil
}

// urlObjectIDMatches accepts text and exact JSON integer IDs; conflicting aliases
// or floating-point integers beyond the exact range must never be treated as proof.
func urlObjectIDMatches(object map[string]any, expected string, keys ...string) bool {
	found := false
	for _, key := range keys {
		value, exists := object[key]
		if !exists {
			continue
		}
		var id string
		switch value := value.(type) {
		case string:
			id = strings.TrimSpace(value)
		case json.Number:
			number, err := strconv.ParseUint(value.String(), 10, 64)
			if err != nil {
				return false
			}
			id = strconv.FormatUint(number, 10)
			if id != value.String() {
				return false
			}
		case float64:
			if value < 0 || value > 9007199254740991 || math.Trunc(value) != value {
				return false
			}
			id = strconv.FormatUint(uint64(value), 10)
		default:
			return false
		}
		if id != expected {
			return false
		}
		found = true
	}
	return found
}

// urlParentMatches enforces parent identities whenever an endpoint returns them;
// omitted parents remain scoped by the exact read-only request arguments.
func urlParentMatches(object map[string]any, target aitabletarget.Target) bool {
	for _, pair := range []struct{ key, id string }{{"baseId", target.BaseID}, {"base_id", target.BaseID}, {"tableId", target.TableID}, {"table_id", target.TableID}, {"sheetId", target.TableID}, {"appId", target.AppID}} {
		if _, exists := object[pair.key]; exists && pair.id != "" && !urlObjectIDMatches(object, pair.id, pair.key) {
			return false
		}
	}
	return true
}

// urlVerificationFailed preserves the resolved IDs without claiming remote existence.
func urlVerificationFailed(tool string, target aitabletarget.Target) error {
	return apperrors.NewAPI(fmt.Sprintf("%s 未证明目标 %s 及所属关系存在", tool, target.Kind),
		apperrors.WithOperation("aitable/"+tool), apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("target_verification"), apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(false), apperrors.WithReason("target_verification_failed"),
		apperrors.WithHint("请核对原链接及读取权限；本次未修改对象，也未验证页面定位或写后状态"),
		apperrors.WithDetails(map[string]any{"target": target}))
}
