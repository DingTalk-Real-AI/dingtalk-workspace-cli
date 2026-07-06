package helpers

import (
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws aiapp — AI 应用
// ──────────────────────────────────────────────────────────
//
// MCP 端点（无身份）: https://mcp-gw.dingtalk.com/server/d5ea9e57768bd9b8c44bca271e4109fd1bc45ef5449dc66c2b8682bc25ba75f8
//
// Tool 入参约定（与 tools/list 一致）:
//   - create_ai_app: 顶层 prompt(必填), attachments?(object[]), officialSkillUids?(string[])。无 threadId
//   - query_ai_app:  顶层 taskId(必填)
//   - modify_ai_app: 顶层 prompt(必填), threadId(必填), officialSkillUids?(string[])
//

func newAiappCommand() *cobra.Command {
	aiappCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建 AI 应用",
		Example: `  dws aiapp create --prompt "创建一个天气查询应用"
  dws aiapp create --prompt "翻译应用" --skills s1,s2
  dws aiapp create --prompt "根据附件创建应用" --attachments '[{"name":"data.xlsx","type":"excel","url":"https://...","size":102400}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// create_ai_app 入参: 顶层 prompt, attachments?, officialSkillUids?
			argsMap := map[string]any{"prompt": mustGetFlag(cmd, "prompt")}
			if v, _ := cmd.Flags().GetString("attachments"); v != "" {
				var att []any
				if err := json.Unmarshal([]byte(v), &att); err == nil && len(att) > 0 {
					argsMap["attachments"] = att
				}
			}
			if v, _ := cmd.Flags().GetString("skills"); v != "" {
				argsMap["officialSkillUids"] = parseSkillIds(v)
			}
			return callMCPTool("create_ai_app", argsMap)
		},
	}

	aiappQueryCmd := &cobra.Command{
		Use:     "query",
		Short:   "查询 AI 应用",
		Example: `  dws aiapp query --task-id <taskId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, err := mustFlagWithHint(cmd, "task-id", `dws aiapp query --task-id <taskId>`)
			if err != nil {
				return err
			}
			return callMCPTool("query_ai_app", map[string]any{
				"taskId": taskID,
			})
		},
	}

	aiappModifyCmd := &cobra.Command{
		Use:   "modify",
		Short: "修改 AI 应用",
		Example: `  dws aiapp modify --prompt "改为翻译应用" --thread-id <threadId>
  dws aiapp modify --prompt "新描述" --thread-id <threadId> --skills s1,s2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// modify_ai_app 入参: 顶层 prompt, threadId, officialSkillUids?
			argsMap := map[string]any{
				"prompt":   mustGetFlag(cmd, "prompt"),
				"threadId": mustGetFlag(cmd, "thread-id"),
			}
			if v, _ := cmd.Flags().GetString("skills"); v != "" {
				argsMap["officialSkillUids"] = parseSkillIds(v)
			}
			return callMCPTool("modify_ai_app", argsMap)
		},
	}

	aiappCreateCmd.Flags().String("prompt", "", "创建 AI 应用的 prompt（必填）")
	_ = aiappCreateCmd.MarkFlagRequired("prompt")
	aiappCreateCmd.Flags().String("attachments", "", "附件对象数组 JSON（可选）")
	aiappCreateCmd.Flags().String("skills", "", "技能 ID 列表，逗号分隔（可选）")

	aiappQueryCmd.Flags().String("task-id", "", "AI 应用任务 ID（必填）")

	aiappModifyCmd.Flags().String("prompt", "", "新的 prompt（必填）")
	_ = aiappModifyCmd.MarkFlagRequired("prompt")
	aiappModifyCmd.Flags().String("thread-id", "", "threadId（必填）")
	_ = aiappModifyCmd.MarkFlagRequired("thread-id")
	aiappModifyCmd.Flags().String("skills", "", "技能 ID 列表，逗号分隔（可选）")

	aiappCmd := &cobra.Command{
		Use:   "aiapp",
		Short: "AI 应用创建 / 查询 / 修改",
		Long:  `创建、查询、修改钉钉 AI 应用任务。`,
		RunE:  groupRunE,
	}
	aiappCmd.AddCommand(aiappCreateCmd, aiappQueryCmd, aiappModifyCmd)
	aiappCmd.AddCommand(hintSubCmd("search",
		"dws aiapp 不支持 search，可用子命令: create / query / modify"))
	return aiappCmd
}

func parseSkillIds(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	ids := make([]string, 0, len(parts))
	for _, p := range parts {
		if id := strings.TrimSpace(p); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
