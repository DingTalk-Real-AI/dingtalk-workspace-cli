// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitabletarget

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// parseNavigationURL keeps resource IDs separate from navigation options.
// Inner/outer IDs must agree; object families never win by parameter order.
func parseNavigationURL(parsed *url.URL, values url.Values) (Target, error) {
	target := Target{SchemaVersion: "aitable.target.v1", Source: "url", Kind: "base"}
	for key := range values {
		if strings.HasPrefix(strings.ToLower(key), "target") {
			return Target{}, fmt.Errorf("不支持已移除的 target* 定位参数；请使用当前对象链接")
		}
	}
	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	standalone := parsed.Path == "/notable/record"
	if len(segments) == 3 && segments[0] == "i" && segments[1] == "nodes" {
		baseID, err := url.PathUnescape(segments[2])
		if err != nil || !validID(baseID) {
			return Target{}, fmt.Errorf("URL 路径中的 baseId 无效")
		}
		values.Add("dentryUuid", baseID)
	} else if !standalone {
		return Target{}, fmt.Errorf("路径必须是 /i/nodes/{baseId} 或 /notable/record")
	}
	if err := addRecordNavigation(values); err != nil {
		return Target{}, err
	}
	bindings := []struct {
		out  *string
		keys []string
	}{
		{&target.BaseID, []string{"dentryUuid"}},
		{&target.Entrance, []string{"entrance"}},
		{&target.TableID, []string{"sheetId", "tableId"}},
		{&target.ViewID, []string{"viewId"}},
		{&target.RecordID, []string{"recordId", "rowId"}},
		{&target.FieldID, []string{"fieldId"}},
		{&target.AppID, []string{"appId"}},
		{&target.PageID, []string{"pageId"}},
		{&target.ApplicationID, []string{"applicationId"}},
		{&target.FlowID, []string{"flowId"}},
	}
	for _, binding := range bindings {
		for _, key := range binding.keys {
			for _, value := range values[key] {
				if strings.TrimSpace(value) == "" {
					return Target{}, fmt.Errorf("%s 不可为空", key)
				}
			}
		}
		id, err := uniqueQueryID(values, binding.keys...)
		if err != nil {
			return Target{}, err
		}
		*binding.out = id
	}
	if target.BaseID == "" {
		return Target{}, fmt.Errorf("独立记录链接缺少 dentryUuid")
	}
	if standalone && (target.TableID == "" || target.RecordID == "") {
		return Target{}, fmt.Errorf("独立记录链接缺少 sheetId 或 rowId")
	}
	if (target.ViewID != "" || target.FieldID != "" || target.RecordID != "") && target.TableID == "" {
		return Target{}, fmt.Errorf("视图、字段或记录缺少所属 sheetId/tableId")
	}
	if target.PageID != "" && target.AppID == "" {
		return Target{}, fmt.Errorf("pageId 缺少所属 appId")
	}
	if target.RecordID != "" && target.FieldID != "" {
		return Target{}, fmt.Errorf("记录和字段定位不可混用")
	}
	// Web keeps the last data/application selection when sharing the automation
	// Tab. These IDs are background context, not additional navigation targets.
	// Validate them above before discarding them; record/field actions stay explicit.
	if !standalone && target.Entrance == "automation" && target.RecordID == "" && target.FieldID == "" {
		target.TableID, target.ViewID, target.ApplicationID = "", "", ""
	}
	families := 0
	for _, present := range []bool{target.TableID != "", target.AppID != "", target.ApplicationID != "", target.FlowID != ""} {
		if present {
			families++
		}
	}
	if families > 1 {
		return Target{}, fmt.Errorf("数据对象、应用模式、Application 和流程定位不可混用")
	}
	switch {
	case target.RecordID != "":
		target.Kind = "record"
	case target.FieldID != "":
		target.Kind = "field"
	case target.ViewID != "":
		target.Kind = "view"
	case target.TableID != "":
		target.Kind = "table"
	case target.PageID != "":
		target.Kind = "app_page"
	case target.AppID != "":
		target.Kind = "app"
	case target.ApplicationID != "":
		target.Kind = "application"
	case target.FlowID != "":
		target.Kind = "workflow"
	}
	expectedEntrance := "data"
	switch target.Kind {
	case "app", "app_page":
		expectedEntrance = "apps"
	case "workflow":
		expectedEntrance = "automation"
	}
	if target.Entrance == "form" && target.Kind == "view" {
		target.Kind, expectedEntrance = "form", "form"
	}
	if target.Entrance != "" {
		switch target.Entrance {
		case "data", "form", "apps", "automation":
		default:
			return Target{}, fmt.Errorf("entrance 必须是 data/form/apps/automation")
		}
		if target.Kind != "base" && target.Entrance != expectedEntrance {
			return Target{}, fmt.Errorf("entrance 与目标对象类型不一致")
		}
	}
	return target, nil
}

// addRecordNavigation follows Web's sheet_view_row[_tab] convention. Explicit
// IDs are checked against these IDs later; ambiguous separators are rejected.
func addRecordNavigation(values url.Values) error {
	for _, value := range values["record"] {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("record 不可为空")
		}
	}
	record, err := uniqueQueryID(values, "record")
	if err != nil || record == "" {
		return err
	}
	parts := strings.Split(record, "_")
	if (len(parts) != 3 && len(parts) != 4) || parts[0] == "" || parts[2] == "" {
		return fmt.Errorf("record 必须是 sheet_view_row 或 sheet_view_row_tab")
	}
	if len(parts) == 4 {
		tab, err := strconv.Atoi(parts[3])
		if err != nil || tab < 0 {
			return fmt.Errorf("record tab 必须是非负整数")
		}
	}
	values.Add("sheetId", parts[0])
	if parts[1] != "" {
		values.Add("viewId", parts[1])
	}
	values.Add("recordId", parts[2])
	return nil
}
