// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	DashboardGridColumnsV1     = 12
	DashboardGridColumnsV2     = 48
	RootResponsiveLayoutParent = "root-responsive-layout"
)

var layoutIntegerBounds = func() (int64, int64) {
	maxInt := int64(^uint(0) >> 1)
	return -maxInt - 1, maxInt
}

// ValidateDashboardPersistentMetadata rejects read-only protocol evidence from
// Dashboard/Chart write objects. isAppMode belongs to caller context and
// schemaVersion belongs to storage metadata; neither may be persisted by CLI
// adapters or shortcuts.
func ValidateDashboardPersistentMetadata(flag string, value map[string]any) error {
	for _, key := range []string{"schemaVersion", "isAppMode"} {
		if _, exists := value[key]; exists {
			return fmt.Errorf("--%s.%s 是只读运行时信息，不能写入 Dashboard/Chart 配置", flag, key)
		}
	}
	meta, ok := value["meta"].(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range []string{"schemaVersion", "isAppMode"} {
		if _, exists := meta[key]; exists {
			return fmt.Errorf("--%s.meta.%s 是只读运行时信息，不能写入 Dashboard/Chart 配置", flag, key)
		}
	}
	return nil
}

// ResolveDashboardRootColumns selects the root Dashboard grid using the public
// MCP protocol. The read response must identify the requested Base/Dashboard
// before either application-mode context or stored metadata can select a grid.
// Ordinary Dashboards require the original schemaVersion JSON type to be
// verified so a numeric 2 cannot be confused with the legacy string "2".
func ResolveDashboardRootColumns(
	payload map[string]any,
	expectedBaseID, expectedDashboardID string,
	isAppMode bool,
) (int, error) {
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return 0, fmt.Errorf("get_dashboard 返回缺少 data 对象，无法验证根布局协议")
	}
	baseID, ok := data["baseId"].(string)
	if !ok || baseID != expectedBaseID {
		return 0, fmt.Errorf("get_dashboard 返回的 data.baseId 与请求不一致")
	}
	dashboardID, ok := data["dashboardId"].(string)
	if !ok || dashboardID != expectedDashboardID {
		return 0, fmt.Errorf("get_dashboard 返回的 data.dashboardId 与请求不一致")
	}
	if isAppMode {
		return DashboardGridColumnsV2, nil
	}
	meta, ok := data["meta"].(map[string]any)
	if !ok {
		return 0, fmt.Errorf("get_dashboard 返回缺少 data.meta 对象，无法验证根布局协议")
	}
	verified, ok := meta["schemaVersionTypeVerified"].(bool)
	if !ok || !verified {
		return 0, fmt.Errorf("data.meta.schemaVersionTypeVerified 必须为 true，无法安全判断 12/48 列")
	}
	version := meta["schemaVersion"]
	if isJSONNumberTwo(version) {
		return DashboardGridColumnsV2, nil
	}
	return DashboardGridColumnsV1, nil
}

// ValidateRootChartLayout validates the writable x/y/w/h geometry against a
// trusted root grid width. Child containers and Tabs have independent column
// systems that the public Dashboard response does not expose, so they fail
// closed instead of inheriting the root 12/48-column rule.
func ValidateRootChartLayout(layout map[string]any, totalColumns int) error {
	if totalColumns != DashboardGridColumnsV1 && totalColumns != DashboardGridColumnsV2 {
		return fmt.Errorf("不支持的 Dashboard 根布局列数 %d", totalColumns)
	}
	if parent, exists := layout["parentId"]; exists && parent != nil {
		parentID, ok := parent.(string)
		if !ok {
			return fmt.Errorf("layout.parentId 必须是字符串")
		}
		if parentID != strings.TrimSpace(parentID) {
			return fmt.Errorf("layout.parentId 不能包含首尾空白")
		}
		if parentID != "" && parentID != RootResponsiveLayoutParent {
			return fmt.Errorf("layout.parentId=%q 使用独立容器坐标系，DWS 无法用根布局 12/48 列安全校验", parentID)
		}
	}

	x, err := requiredLayoutInteger(layout, "x")
	if err != nil {
		return err
	}
	y, err := requiredLayoutInteger(layout, "y")
	if err != nil {
		return err
	}
	w, err := requiredLayoutInteger(layout, "w")
	if err != nil {
		return err
	}
	h, err := requiredLayoutInteger(layout, "h")
	if err != nil {
		return err
	}
	if x < 0 || y < 0 {
		return fmt.Errorf("layout.x 和 layout.y 必须是非负整数")
	}
	if w <= 0 || h <= 0 {
		return fmt.Errorf("layout.w 和 layout.h 必须是正整数")
	}
	if x > totalColumns-w {
		return fmt.Errorf("layout 超出 Dashboard 根网格：totalColumns=%d, x=%d, w=%d, x+w=%d",
			totalColumns, x, w, x+w)
	}
	return nil
}

func isJSONNumberTwo(value any) bool {
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Float64()
		return err == nil && parsed == 2
	case float64:
		return number == 2
	case float32:
		return number == 2
	case int:
		return number == 2
	case int8:
		return number == 2
	case int16:
		return number == 2
	case int32:
		return number == 2
	case int64:
		return number == 2
	case uint:
		return number == 2
	case uint8:
		return number == 2
	case uint16:
		return number == 2
	case uint32:
		return number == 2
	case uint64:
		return number == 2
	default:
		return false
	}
}

func requiredLayoutInteger(layout map[string]any, key string) (int, error) {
	value, exists := layout[key]
	if !exists {
		return 0, fmt.Errorf("layout.%s 是必填字段", key)
	}
	switch number := value.(type) {
	case float64:
		minInt, maxInt := layoutIntegerBounds()
		if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number ||
			number > float64(maxInt) || number < float64(minInt) {
			return 0, fmt.Errorf("layout.%s 必须是整数", key)
		}
		return int(number), nil
	case json.Number:
		parsed, err := strconv.ParseInt(number.String(), 10, 64)
		minInt, maxInt := layoutIntegerBounds()
		if err != nil || parsed < minInt || parsed > maxInt {
			return 0, fmt.Errorf("layout.%s 必须是整数", key)
		}
		return int(parsed), nil
	case int:
		return number, nil
	case int32:
		return int(number), nil
	case int64:
		minInt, maxInt := layoutIntegerBounds()
		if number < minInt || number > maxInt {
			return 0, fmt.Errorf("layout.%s 超出整数范围", key)
		}
		return int(number), nil
	default:
		return 0, fmt.Errorf("layout.%s 必须是整数，got %T", key, value)
	}
}
