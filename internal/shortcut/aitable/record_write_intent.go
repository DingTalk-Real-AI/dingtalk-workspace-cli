// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/google/uuid"
)

// recordWriteIntent is immutable recovery evidence, not proof of dispatch or
// application. Keep only a payload digest: cell contents and credentials must
// not be copied into the recovery directory. The token covers creates only.
type recordWriteIntent struct {
	Version       int    `json:"version"`
	Operation     string `json:"operation"`
	Identity      string `json:"identity"`
	BaseID        string `json:"baseId"`
	TableID       string `json:"tableId"`
	ClientToken   string `json:"clientToken"`
	RequestDigest string `json:"requestDigest"`
	BatchOffset   int    `json:"batchOffset"`
	RecordCount   int    `json:"recordCount"`
	CreateCount   int    `json:"createCount"`
	State         string `json:"state"`
}

// persistRecordWriteIntent publishes the original selectors and token before
// any mutation. Failure is a dispatch barrier. Each invocation has a unique
// file, so concurrent callers never overwrite one another's recovery evidence;
// cross-client deduplication remains the server's responsibility.
func persistRecordWriteIntent(rt *shortcut.RuntimeContext, batch []map[string]any, offset int, token, identity string) (string, error) {
	if identity == "" {
		return "", fmt.Errorf("cannot persist record write without a verified identity")
	}
	raw, err := json.Marshal(batch)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	creates := 0
	for _, record := range batch {
		if recordID(record) == "" {
			creates++
		}
	}
	intent := recordWriteIntent{Version: 2, Operation: "record_upsert", Identity: identity,
		BaseID: rt.Str("base-id"), TableID: rt.Str("table-id"), ClientToken: token,
		RequestDigest: hex.EncodeToString(digest[:]), BatchOffset: offset,
		RecordCount: len(batch), CreateCount: creates, State: "prepared"}
	raw, _ = json.MarshalIndent(intent, "", "  ")
	path := filepath.Join(config.DefaultConfigDir(), "aitable", "write-intents", uuid.NewString()+".json")
	if err := aitableTaskWriteJSON(path, append(raw, '\n')); err != nil {
		return "", fmt.Errorf("cannot persist original write token before dispatch: %w", err)
	}
	return path, nil
}

// validateRecordWriteIntent binds a later read to the prepared write's account,
// gateway configuration and selectors. It never authorizes replaying the payload.
func validateRecordWriteIntent(rt *shortcut.RuntimeContext, path, token string) error {
	var intent recordWriteIntent
	if err := readAitableTaskFile(path, &intent); err != nil {
		return err
	}
	if (intent.Version != 1 && intent.Version != 2) || intent.Operation != "record_upsert" ||
		intent.State != "prepared" || intent.BaseID != rt.Str("base-id") ||
		intent.TableID != rt.Str("table-id") || intent.ClientToken != token {
		return fmt.Errorf("write intent does not match the original identity, endpoint or request")
	}
	// 旧文件未保存稳定身份，不能用当前身份重新解释其凭据摘要。
	resolveIdentity := recordWriteIdentity
	if intent.Version == 1 {
		resolveIdentity = legacyRecordWriteIdentity
	}
	identity, err := resolveIdentity(rt)
	if err != nil {
		return err
	}
	if intent.Identity != identity {
		return fmt.Errorf("write intent does not match the original identity, endpoint or request")
	}
	return nil
}

// 仅外部凭据缺少稳定身份时查询一次；调用者在本次批次循环内复用结果。
// 显式 Token 优先于 profile，不能借用本地其他账号的身份或信任环境变量。
func recordWriteIdentity(rt *shortcut.RuntimeContext) (string, error) {
	if rt.Str("token") == "" {
		return legacyRecordWriteIdentity(rt)
	}
	body, err := rt.CallMCPData("contact", "get_current_user_profile", nil)
	if err != nil {
		return "", err
	}
	// CallMCPData 的结果来自 JSON 解码；保留原响应交给认证模块严格解析。
	raw, _ := json.Marshal(body)
	var response struct {
		Success bool `json:"success"`
		Result  []struct {
			Employee struct {
				CorpID string `json:"corpId"`
			} `json:"orgEmployeeModel"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &response); err != nil || !response.Success || len(response.Result) != 1 {
		return "", fmt.Errorf("cannot bind record recovery: current-token identity is missing or ambiguous")
	}
	identity, err := auth.ManagedIdentityFromToolResult(&edition.ToolResult{
		Content: []edition.ContentBlock{{Type: "text", Text: string(raw)}},
	}, response.Result[0].Employee.CorpID)
	if err != nil {
		return "", fmt.Errorf("cannot bind record recovery: %w", err)
	}
	principal, _ := json.Marshal([]string{identity.CorpID, identity.UserID})
	return recordWriteScopeIdentity("principal:" + string(principal)), nil
}

// 保留 v1 恢复文件的原校验；profile 路径仍复用本地身份，不新增网络调用。
func legacyRecordWriteIdentity(rt *shortcut.RuntimeContext) (string, error) {
	token, _ := rt.Command().Flags().GetString("token")
	credential := "explicit-token:" + strings.TrimSpace(token)
	if token = strings.TrimSpace(token); token == "" {
		identity, err := buildPlanIdentity(rt)
		if err != nil {
			return "", err
		}
		credential = "profile:" + identity
	}
	return recordWriteScopeIdentity(credential), nil
}

func recordWriteScopeIdentity(credential string) string {
	// Both logical products can carry an explicit endpoint. The configured MCP
	// base also rewrites their gateway host, even when no process override exists.
	// Keep this journal's binding separate from older build-plan checkpoints.
	scope, _ := json.Marshal([]string{credential, config.GetMCPBaseURL(), auth.MCPBaseURLOverride(),
		os.Getenv("DINGTALK_AITABLE_MCP_URL"), os.Getenv("DINGTALK_AITABLE_HELPER_MCP_URL")})
	digest := sha256.Sum256(scope)
	return hex.EncodeToString(digest[:])
}

// recordWriteRecoveryCommand retains the optional immutable intent file so a
// later process checks the same identity before making even a read-only call.
func recordWriteRecoveryCommand(base, table, token, path string) string {
	args := []string{"dws", "aitable", "+record-write-result", "--base-id", base, "--table-id", table, "--client-token", token}
	if path != "" {
		args = append(args, "--write-intent", path)
	}
	return aitableRecoveryCommand(args...)
}
