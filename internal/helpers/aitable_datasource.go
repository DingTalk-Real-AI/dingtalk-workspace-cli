// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func readAitableDatasourceSourceConfig(ctx context.Context, baseID, tableID string) (string, error) {
	raw, err := callAitableReadToolTextContext(ctx, "get_datasource_config", map[string]any{
		"baseId": baseID, "tableId": tableID,
	})
	if err != nil {
		return "", err
	}
	var payload map[string]any
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return "", aitableDatasourceConfigReadError("get_datasource_config 返回的不是合法 JSON 对象")
	}
	config, err := aitableprotocol.DatasourceSourceConfig(payload)
	if err != nil {
		return "", aitableDatasourceConfigReadError(err.Error())
	}
	return config, nil
}

func aitableDatasourceConfigReadError(message string) error {
	return apperrors.NewAPI(message+"；未执行数据源更新，请显式提供完整 --source-config",
		apperrors.WithOperation("aitable/get_datasource_config"),
		apperrors.WithReason("datasource_config_unavailable"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(false))
}
