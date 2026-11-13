package helpers

import (
	"encoding/json"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

func mustGetBool(cmd *cobra.Command, name string) bool {
	value, _ := cmd.Flags().GetBool(name)
	return value
}

func validateDocBlockTarget(cmd *cobra.Command) error {
	start, end, block := mustGetFlag(cmd, "start-block-id"), mustGetFlag(cmd, "end-block-id"), mustGetFlag(cmd, "block-id")
	if (start == "") != (end == "") || start != "" && block != "" || start == "" && block == "" || (cmd.Flags().Changed("start-block-id") || cmd.Flags().Changed("end-block-id")) && (start == "" || end == "") {
		return apperrors.NewValidation("提供 --block-id 或成对的 --start-block-id/--end-block-id；两种目标不可混用")
	}
	return nil
}

func docBlockSequenceContent(cmd *cobra.Command, params map[string]any) error {
	format := mustGetFlag(cmd, "content-format")
	if format == "markdown" {
		if mustGetFlag(cmd, "jsonml-nodes") != "" || mustGetFlag(cmd, "element") != "" {
			return apperrors.NewValidation("Markdown 与 JSONML 输入不能混用")
		}
		text := mustGetFlag(cmd, "content")
		if strings.TrimSpace(text) == "" {
			return apperrors.NewValidation("Markdown 内容不能为空")
		}
		params["format"], params["markdown"] = "markdown", text
		return nil
	}
	if format != "jsonml" {
		return apperrors.NewValidation("范围或序列写入要求 --content-format markdown/jsonml")
	}
	raw := mustGetFlag(cmd, "jsonml-nodes")
	if raw == "" {
		node, err := prepareJsonMLNode(cmd, mustGetFlag(cmd, "element"))
		if err != nil {
			return apperrors.NormalizeValidation(err)
		}
		params["format"], params["jsonml"] = "jsonml", node
		return nil
	}
	if mustGetFlag(cmd, "element") != "" {
		return apperrors.NewValidation("--jsonml-nodes 与 --element 互斥")
	}
	nodes, err := PrepareDocJSONMLNodes(cmd, raw)
	if err != nil {
		return err
	}
	params["format"], params["jsonml"] = "jsonml", nodes
	return nil
}

// PrepareDocJSONMLNodes validates each supplied node using the existing node pipeline.
// Identity repair is limited to duplicate UUIDs inside this explicit sequence.
func PrepareDocJSONMLNodes(cmd *cobra.Command, raw string) (string, error) {
	var values []any
	if err := json.Unmarshal([]byte(raw), &values); err != nil || len(values) == 0 {
		return "", apperrors.NewValidation("--jsonml-nodes 必须是非空 JSONML 节点序列")
	}
	return prepareDecodedDocJSONMLNodes(cmd, values)
}

func prepareDecodedDocJSONMLNodes(cmd *cobra.Command, values []any) (string, error) {
	nodes := make([]any, 0, len(values))
	nodeCmd := &cobra.Command{Use: "sequence-node"}
	nodeCmd.Flags().Bool("fix-jsonml", mustGetBool(cmd, "fix-jsonml"), "")
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		normalized, err := prepareJsonMLNode(nodeCmd, string(encoded))
		if err != nil {
			return "", apperrors.NormalizeValidation(err)
		}
		var node []any
		// prepareJsonMLNode returns a validated JSON array. Decode it back
		// to mutable nodes for sequence UUID normalization.
		_ = json.Unmarshal([]byte(normalized), &node)
		nodes = append(nodes, node)
	}
	if docSuggestionEnabled(cmd) {
		wrapper := append([]any{"sequence", map[string]any{}}, nodes...)
		if err := prepareInputNodeUUIDs(wrapper, mustGetBool(cmd, "fix-jsonml"), ""); err != nil {
			return "", err
		}
	}
	encoded, err := json.Marshal(nodes)
	return string(encoded), err
}

func classifyDocOverwriteError(body map[string]any) error {
	code := businessErrorCode(body)
	if code != "PENDING_SUGGESTIONS_EXIST" && code != "PENDING_SUGGESTIONS_UNAVAILABLE" {
		return nil
	}
	action := "摘要不可用，覆盖尚未开始；稍后检查服务端状态"
	if code == "PENDING_SUGGESTIONS_EXIST" {
		action = "先审阅本人待处理建议；明确同意后追加 --force 发起覆盖，--yes 不代表 --force"
	}
	return apperrors.NewAPI(businessErrorDisplayMessage(body, code), apperrors.WithReason(code), apperrors.WithExecutionStarted(false), apperrors.WithRetryable(false), apperrors.WithActions(action))
}

func isDocBlockSequenceWrite(tool string, args map[string]any) bool {
	if tool != "insert_document_block" && tool != "update_document_block" {
		return false
	}
	if args["format"] == "markdown" || args["startBlockId"] != nil {
		return true
	}
	// The public jsonml string carries either one node or a nonempty sequence.
	// A length-one sequence still needs actual sequence identities in the receipt.
	raw, _ := args["jsonml"].(string)
	var nodes []any
	if args["format"] != "jsonml" || json.Unmarshal([]byte(raw), &nodes) != nil || len(nodes) == 0 {
		return false
	}
	_, sequence := nodes[0].([]any)
	return sequence
}

// validateDocBlockSequenceReceipt requires actual server identities before a new
// sequence route reports completion; absent evidence cannot prove no mutation.
func validateDocBlockSequenceReceipt(tool string, args map[string]any, text string) error {
	if !isDocBlockSequenceWrite(tool, args) {
		return nil
	}
	var result map[string]any
	_ = json.Unmarshal([]byte(text), &result)
	var ids func(map[string]any, int) ([]any, bool)
	ids = func(value map[string]any, depth int) ([]any, bool) {
		if value == nil || depth > 4 {
			return nil, false
		}
		if raw, exists := value["newBlockIds"]; exists {
			parsed, ok := raw.([]any)
			return parsed, ok
		}
		for _, key := range []string{"result", "data"} {
			if nested, ok := value[key].(map[string]any); ok {
				if parsed, found := ids(nested, depth+1); found {
					return parsed, true
				}
			}
		}
		return nil, false
	}
	values, valid := ids(result, 0)
	valid = valid && len(values) > 0
	seen := map[string]bool{}
	for _, raw := range values {
		id, ok := raw.(string)
		if !ok || strings.TrimSpace(id) == "" || seen[id] {
			valid = false
			break
		}
		seen[id] = true
	}
	if valid {
		return nil
	}
	return apperrors.NewAPI("远端可能已执行块序列写入，但实际新块 ID 回执未确认；请先读回，不要自动重试",
		apperrors.WithReason("doc_block_sequence_receipt_unconfirmed"), apperrors.WithExecutionStarted(true), apperrors.WithRetryable(false),
		apperrors.WithFailureStage("response_validation"), apperrors.WithDetails(map[string]any{"nodeId": args["nodeId"], "remoteEffect": "unknown"}),
		apperrors.WithActions("先读取目标文档或本人建议视图确认实际内容及块 ID；核对 MCP 输出映射后再决定后续操作"))
}
