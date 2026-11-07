// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"encoding/json"
	"fmt"
)

// DatasourceSourceConfig reads the reviewed get_datasource_config envelope.
// Keep the original JSON string: decoding and re-encoding an opaque stored
// config could lose number precision or fields unknown to this CLI version.
func DatasourceSourceConfig(payload map[string]any) (string, error) {
	if payload["status"] != "success" || payload["error"] != nil {
		return "", fmt.Errorf("get_datasource_config 未返回明确成功的配置响应")
	}
	data, ok := payload["data"].(map[string]any)
	if !ok || data["datasourceType"] != "OA" {
		return "", fmt.Errorf("get_datasource_config 未返回可确认的 OA 数据源配置")
	}
	raw, ok := data["sourceConfig"].(string)
	var config map[string]json.RawMessage
	if !ok || json.Unmarshal([]byte(raw), &config) != nil || len(config) == 0 {
		return "", fmt.Errorf("get_datasource_config 的 sourceConfig 必须是非空 JSON 对象字符串")
	}
	return raw, nil
}
