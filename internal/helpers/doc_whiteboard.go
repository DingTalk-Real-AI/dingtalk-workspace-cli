// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

const (
	whiteboardDrawPluginType = "application/x-alidocs-plugin-draw"
	whiteboardDefaultHeight  = 600
)

// errWhiteboardBlockPending 标记「块查询成功但目标块尚不可见」这一最终一致性场景。
// 只有它允许插入后回查退化成 soft success；鉴权失败、MCP 错误、响应/JSONML 解析失败
// 都是硬失败，必须 fail-closed，否则 Agent 会把它误判成最终一致性并带着空 partId
// 继续调用 whiteboard query/update。
var errWhiteboardBlockPending = errors.New("whiteboard card block is not visible yet")

var (
	whiteboardRetryDelays = []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second}
	whiteboardSleep       = time.Sleep
	whiteboardJSONMarshal = json.Marshal
	prepareWhiteboardCard = prepareJsonMLNode
)

func buildWhiteboardCardJSONML(blockUUID, whiteboardID string) string {
	node := []any{
		"card",
		map[string]any{
			"uuid":     blockUUID,
			"cardType": "hetu",
			"height":   whiteboardDefaultHeight,
			"metadata": map[string]any{"type": whiteboardDrawPluginType, "id": whiteboardID},
		},
		[]any{"span", map[string]any{"data-type": "text"},
			[]any{"span", map[string]any{"data-type": "leaf"}, ""}},
	}
	out, err := whiteboardJSONMarshal(node)
	if err != nil {
		return ""
	}
	return string(out)
}

func extractWhiteboardID(attrs map[string]any) string {
	meta, _ := attrs["metadata"].(map[string]any)
	if meta == nil {
		return ""
	}
	id, _ := meta["id"].(string)
	return id
}

func queryWhiteboardCardNode(ctx context.Context, nodeID, blockID string) ([]any, error) {
	text, err := callMCPToolReturnTextOnServer(ctx, "doc", "list_document_blocks", map[string]any{
		"nodeId":  nodeID,
		"blockId": blockID,
		"format":  "jsonml",
	})
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return nil, fmt.Errorf("parse list_document_blocks response: %w", err)
	}
	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}
	blocksField, ok := data["blocks"]
	if !ok {
		return nil, fmt.Errorf("list_document_blocks 响应缺少 blocks 字段")
	}
	blocks, ok := blocksField.([]any)
	if !ok {
		return nil, fmt.Errorf("list_document_blocks 响应的 blocks 字段不是数组")
	}
	var raw string
	for _, block := range blocks {
		entry, _ := block.(map[string]any)
		if entry == nil || entry["blockId"] != blockID {
			continue
		}
		raw, _ = entry["jsonml"].(string)
		break
	}
	if raw == "" {
		return nil, fmt.Errorf("块 %s 不存在或查询无结果: %w", blockID, errWhiteboardBlockPending)
	}
	var node []any
	if err := json.Unmarshal([]byte(raw), &node); err != nil {
		return nil, fmt.Errorf("parse block jsonml: %w", err)
	}
	return node, nil
}

func queryWhiteboardCardAttrs(ctx context.Context, nodeID, blockID string) (map[string]any, error) {
	node, err := queryWhiteboardCardNode(ctx, nodeID, blockID)
	if err != nil {
		return nil, err
	}
	if len(node) < 2 {
		return nil, fmt.Errorf("块 %s 的 jsonml 节点缺少 attrs", blockID)
	}
	attrs, _ := node[1].(map[string]any)
	if attrs == nil {
		return nil, fmt.Errorf("块 %s 的 jsonml attrs 不是对象", blockID)
	}
	return attrs, nil
}

func runWhiteboardInsert(cmd *cobra.Command, _ []string) error {
	nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
	if err != nil {
		return err
	}

	blockUUID := uuid.New().String()
	whiteboardID := uuid.New().String()
	element := buildWhiteboardCardJSONML(blockUUID, whiteboardID)
	normalized, err := prepareWhiteboardCard(cmd, element)
	if err != nil {
		return fmt.Errorf("内部错误: 白板卡片模板未通过 JSONML 校验: %w", err)
	}

	toolArgs := map[string]any{
		"nodeId": nodeID,
		"jsonml": normalized,
		"format": "jsonml",
	}
	// --ref-block 与 --parent-block 已由 MarkFlagsMutuallyExclusive 保证互斥，
	// 这里用 else if 让「只有一条定位分支会写 referenceBlockId/where」在代码上自证。
	if v, _ := cmd.Flags().GetString("ref-block"); v != "" {
		toolArgs["referenceBlockId"] = v
		where, _ := cmd.Flags().GetString("where")
		if where == "" {
			where = "after"
		}
		toolArgs["where"] = where
	} else if v, _ := cmd.Flags().GetString("parent-block"); v != "" {
		toolArgs["referenceBlockId"] = v
	}
	if cmd.Flags().Changed("index") {
		index, _ := cmd.Flags().GetInt("index")
		toolArgs["index"] = index
	}

	if deps.Caller.DryRun() {
		return callMCPToolOnServer("doc", "insert_document_block", toolArgs)
	}

	// 用户确认由 DeclareLeafMetadata(user_required) 的 ConfirmSafety 门控接管：
	// 推迟到首次 deps.Caller.CallTool（下方 insert_document_block），避免与
	// 门控双读 stdin。--yes / --dry-run 经 confirmationBypass 跳过。
	ctx := cmd.Context()
	deps.Out.PrintProgress("[1/2] 插入白板卡片...")
	if _, err := callMCPToolReturnTextOnServer(ctx, "doc", "insert_document_block", toolArgs); err != nil {
		return err
	}

	deps.Out.PrintProgress("[2/2] 验证白板资源 ID 落库...")
	persistedID := ""
	for attempt := 0; attempt <= len(whiteboardRetryDelays); attempt++ {
		attrs, queryErr := queryWhiteboardCardAttrs(ctx, nodeID, blockUUID)
		switch {
		case queryErr == nil:
			// 块已可见；metadata.id 仍可能未落库，交给下方 soft success 分支重试。
			persistedID = extractWhiteboardID(attrs)
		case errors.Is(queryErr, errWhiteboardBlockPending):
			// 块暂不可见，属于最终一致性，继续重试。
		default:
			// 查询本身失败（鉴权 / MCP / 响应解析），不是最终一致性：
			// 必须 fail-closed，同时带出已插入的 blockId 供人工或后续回查复原。
			return fmt.Errorf(
				"白板卡片已插入 (blockId=%s)，但回查验证失败，无法确认 whiteboardId: %w",
				blockUUID, queryErr)
		}
		if persistedID != "" {
			break
		}
		if attempt < len(whiteboardRetryDelays) {
			whiteboardSleep(whiteboardRetryDelays[attempt])
		}
	}

	result := map[string]any{"blockId": blockUUID}
	if persistedID == "" {
		result["whiteboardId"] = nil
		deps.Out.PrintWarning(fmt.Sprintf(
			"白板已插入但未验证到 whiteboardId 落库，可稍后回查: dws doc block list --node %s --content-format jsonml --block-id %s",
			nodeID, blockUUID))
	} else {
		result["whiteboardId"] = persistedID
	}
	return deps.Out.PrintJSON(map[string]any{"success": true, "result": result})
}

func newDocWhiteboardCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "whiteboard",
		Short: "白板卡片管理",
		Long:  `管理钉钉文档中的白板卡片：插入空白板并获取白板资源 ID。删除白板卡片请使用 dws doc block delete。`,
		RunE:  groupRunE,
	}

	insertCmd := &cobra.Command{
		Use:   "insert",
		Short: "插入白板卡片",
		Long: `向文档插入一个空白板卡片（hetu draw card），并返回 blockId 与 whiteboardId。

CLI 生成卡片块 UUID 与白板资源 ID，插入后按块 UUID 回查并验证 metadata.id 落库。
如果块暂不可见或 metadata.id 尚未落库，插入仍成功并返回 blockId，whiteboardId 为 null。
如果回查本身失败（鉴权 / MCP 错误 / 响应解析失败），命令报错并在错误中带出已插入的 blockId。

定位方式互斥: --ref-block（配合 --where 同级插入）与 --parent-block（配合 --index 容器内插入）
不能同时使用。`,
		Example: `  dws doc whiteboard insert --node DOC_ID
  dws doc whiteboard insert --node DOC_ID --ref-block BLOCK_ID --where before
  dws doc whiteboard insert --node DOC_ID --parent-block PARENT_ID --index 2`,
		RunE: runWhiteboardInsert,
	}
	insertCmd.Flags().String("node", "", "文档 ID 或 URL (必填)")
	insertCmd.Flags().String("ref-block", "", "参照块 UUID（同级插入，配合 --where）")
	insertCmd.Flags().String("where", "", "插入方向: before / after (默认 after，配合 --ref-block)")
	insertCmd.Flags().String("parent-block", "", "父容器 UUID（容器内插入，与 --index 配合）")
	insertCmd.Flags().Int("index", 0, "位置索引 (从 0 开始)")
	insertCmd.Flags().Bool("yes", false, "确认插入白板卡片")

	// 同级插入与容器内插入共用 MCP 的 referenceBlockId：同时传两者会让 parent 静默
	// 覆盖 ref-block、而 --where 仍留在请求里污染容器插入语义。显式互斥而非静默取舍。
	insertCmd.MarkFlagsMutuallyExclusive("ref-block", "parent-block")
	insertCmd.MarkFlagsMutuallyExclusive("where", "parent-block")

	for _, name := range []string{"url", "id", "node-id", "doc-id", "file-id"} {
		insertCmd.Flags().String(name, "", "--node 的兼容别名")
		_ = insertCmd.Flags().MarkHidden(name)
	}

	DeclareLeafMetadata(insertCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "whiteboard_insert",
				CanonicalPath:  "doc.whiteboard_insert",
				CLIPath:        "doc whiteboard insert",
				PrimaryCLIPath: "doc whiteboard insert",
			},
			Description: "向文档插入空白板卡片并返回块 ID 与白板 part ID",
			DryRun:      &contract.DryRunSpec{PreviewKind: "request", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "命令生成卡片与白板 UUID、插入规范 JSONML，再回读块验证 metadata.id，不能绑定为单一 interface_ref",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "经用户确认后向钉钉文档插入空白板卡片并返回块 ID 与白板 part ID",
				UseWhen:      []string{"目标文档还没有可操作白板，需要创建空白板卡片并取得后续 query/update 使用的 partId 时"},
				AvoidWhen:    []string{"已有白板只需读取或编辑时使用 whiteboard query/update；删除卡片使用 doc block delete"},
				Examples:     []string{"dws doc whiteboard insert --node <DOC_ID> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId", Required: boolPtr(true)},
			},
		},
	})

	root.AddCommand(insertCmd)
	return root
}
