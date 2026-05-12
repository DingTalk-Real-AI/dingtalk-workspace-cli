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
	"context"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

const (
	docProductID           = "doc"
	docDownloadFileTool    = "download_file"
	docGetDocumentInfoTool = "get_document_info"
	docAXLSExtension       = "axls"
)

func (r *runtimeRunner) preflightDocDownload(ctx context.Context, tc *transport.Client, endpoint string, invocation executor.Invocation) error {
	if !isDocDownloadInvocation(invocation) {
		return nil
	}
	nodeID := docDownloadNodeID(invocation.Params)
	if nodeID == "" {
		return nil
	}

	preflightStart := time.Now()
	info, err := tc.CallTool(ctx, endpoint, docGetDocumentInfoTool, map[string]any{"nodeId": nodeID})
	RecordTiming(ctx, "doc_download_preflight", time.Since(preflightStart))
	if err != nil {
		return err
	}

	if classify := edition.Get().ClassifyToolResult; classify != nil {
		if err := classify(info.Content); err != nil {
			return err
		}
	}
	if patCheck := apperrors.ClassifyPatAuthCheck(info.Content); patCheck != nil {
		return patCheck
	}
	if info.IsError {
		return apperrors.NewAPI(
			extractMCPErrorMessage(info),
			apperrors.WithOperation("doc.get_document_info"),
			apperrors.WithReason("doc_download_preflight_failed"),
			apperrors.WithServerKey(docProductID),
			apperrors.WithHint("doc download 必须先确认节点类型，避免对不支持下载的在线表格触发 drive:download 授权。"),
			apperrors.WithActions("dws doc info --node <nodeId>"),
		)
	}
	if bizErr := detectBusinessError(info.Content); bizErr != "" {
		return apperrors.NewAPI(
			bizErr,
			apperrors.WithOperation("doc.get_document_info"),
			apperrors.WithReason("doc_download_preflight_failed"),
			apperrors.WithServerKey(docProductID),
			apperrors.WithHint("doc download 必须先确认节点类型，避免对不支持下载的在线表格触发 drive:download 授权。"),
			apperrors.WithActions("dws doc info --node <nodeId>"),
		)
	}

	if strings.EqualFold(documentInfoExtension(info.Content), docAXLSExtension) {
		return unsupportedAXLSDownloadError()
	}
	return nil
}

func isDocDownloadInvocation(invocation executor.Invocation) bool {
	return strings.EqualFold(strings.TrimSpace(invocation.CanonicalProduct), docProductID) &&
		strings.TrimSpace(invocation.Tool) == docDownloadFileTool
}

func docDownloadNodeID(params map[string]any) string {
	for _, key := range []string{"nodeId", "node", "dentryUuid"} {
		if value, ok := params[key].(string); ok {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func unsupportedAXLSDownloadError() error {
	return apperrors.NewValidation(
		"nodeId 指向的节点是钉钉表格（extension=axls），在线表格不支持直接下载。请使用 getRange 工具获取表格数据。",
		apperrors.WithOperation("doc.download_file.preflight"),
		apperrors.WithReason("unsupported_alidoc_extension"),
		apperrors.WithServerKey(docProductID),
		apperrors.WithHint("在线表格应先用 doc info 确认 extension，再改用表格 MCP 的 get_all_sheets / get_range 读取数据。"),
		apperrors.WithActions("dws doc info --node <nodeId>", "使用表格 MCP get_all_sheets / get_range"),
	)
}

func documentInfoExtension(content map[string]any) string {
	for _, path := range [][]string{
		{"result", "extension"},
		{"data", "extension"},
		{"extension"},
	} {
		if value := stringAtPath(content, path...); value != "" {
			return value
		}
	}
	return ""
}

func stringAtPath(value any, path ...string) string {
	current := value
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = object[key]
	}
	if text, ok := current.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}
