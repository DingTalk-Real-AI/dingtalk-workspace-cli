package helpers

import (
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws aidesign — AI 设计
// ──────────────────────────────────────────────────────────
//
// MCP 端点: TODO 从 mcp.dingtalk.com 获取后填入 endpoints.go
//
// Tool 入参约定（与 tools/list 一致）:
//   - generate:               prompt(必填), width?, height?
//   - generate_with_image:    prompt(必填), imageUrl(必填), width?, height?
//   - generate_with_template: imageUrl(必填), template(必填), name?
//   - edit:                   prompt(必填), imageUrl(必填)
//   - upscale:                imageUrl(必填)
//   - isolate:                imageUrl(必填)

func newAidesignCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "aidesign",
		Short: "AI 设计（文生图/图生图/编辑/超分/抠图）",
		Long:  `通过钉钉 AI 设计服务生成、编辑、优化图片。`,
		RunE:  groupRunE,
	}

	// ── generate ─────────────────────────────────────────────

	aidesignGenerateCmd := &cobra.Command{
		Use:   "generate",
		Short: "文生图 — 根据 prompt 生成图片",
		Example: `  dws aidesign generate --prompt "现代简约Logo，科技蓝#0066CC，白色背景"
  dws aidesign generate --prompt "扁平风海报" --width 1024 --height 1024`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsMap := map[string]any{"prompt": mustGetFlag(cmd, "prompt")}
			if v, _ := cmd.Flags().GetInt("width"); v > 0 {
				argsMap["width"] = v
			}
			if v, _ := cmd.Flags().GetInt("height"); v > 0 {
				argsMap["height"] = v
			}
			return callMCPTool("generate", argsMap)
		},
	}

	// ── generate-with-image ──────────────────────────────────

	aidesignGenerateWithImageCmd := &cobra.Command{
		Use:   "generate-with-image",
		Short: "参考图生图 — 根据参考图 + prompt 生成图片",
		Example: `  dws aidesign generate-with-image --prompt "参考风格设计新Logo" --image-url "https://..."
  dws aidesign generate-with-image --prompt "同风格名片" --image-url "https://..." --width 1024 --height 768`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsMap := map[string]any{
				"prompt":   mustGetFlag(cmd, "prompt"),
				"imageUrl": mustGetFlag(cmd, "image-url"),
			}
			if v, _ := cmd.Flags().GetInt("width"); v > 0 {
				argsMap["width"] = v
			}
			if v, _ := cmd.Flags().GetInt("height"); v > 0 {
				argsMap["height"] = v
			}
			return callMCPTool("generate_with_image", argsMap)
		},
	}

	// ── generate-with-template ───────────────────────────────

	aidesignGenerateWithTemplateCmd := &cobra.Command{
		Use:   "generate-with-template",
		Short: "模板生图 — 根据参考图 + 模板生成图片",
		Example: `  dws aidesign generate-with-template --image-url "https://..." --template "business_card_modern_01"
  dws aidesign generate-with-template --image-url "https://..." --template "social_media_header" --name "张三"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsMap := map[string]any{
				"imageUrl": mustGetFlag(cmd, "image-url"),
				"template": mustGetFlag(cmd, "template"),
			}
			if v := mustGetFlag(cmd, "name"); v != "" {
				argsMap["name"] = v
			}
			return callMCPTool("generate_with_template", argsMap)
		},
	}

	// ── edit ─────────────────────────────────────────────────

	aidesignEditCmd := &cobra.Command{
		Use:   "edit",
		Short: "编辑图片 — 通过文本指令修改已有图片",
		Example: `  dws aidesign edit --prompt "将背景颜色改为深蓝色" --image-url "https://..."
  dws aidesign edit --prompt "把文字改成BRAND，字体放大" --image-url "https://..."`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("edit", map[string]any{
				"prompt":   mustGetFlag(cmd, "prompt"),
				"imageUrl": mustGetFlag(cmd, "image-url"),
			})
		},
	}

	// ── upscale ──────────────────────────────────────────────

	aidesignUpscaleCmd := &cobra.Command{
		Use:     "upscale",
		Short:   "超分辨率 — 2倍放大提升图片清晰度",
		Example: `  dws aidesign upscale --image-url "https://..."`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("upscale", map[string]any{
				"imageUrl": mustGetFlag(cmd, "image-url"),
			})
		},
	}

	// ── isolate ──────────────────────────────────────────────

	aidesignIsolateCmd := &cobra.Command{
		Use:     "isolate",
		Short:   "抠图 — 去除背景提取主体",
		Example: `  dws aidesign isolate --image-url "https://..."`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("isolate", map[string]any{
				"imageUrl": mustGetFlag(cmd, "image-url"),
			})
		},
	}

	// generate
	aidesignGenerateCmd.Flags().String("prompt", "", "文生图 prompt（必填）")
	_ = aidesignGenerateCmd.MarkFlagRequired("prompt")
	aidesignGenerateCmd.Flags().Int("width", 0, "图片宽度（可选，默认1024）")
	aidesignGenerateCmd.Flags().Int("height", 0, "图片高度（可选，默认1024）")

	// generate-with-image
	aidesignGenerateWithImageCmd.Flags().String("prompt", "", "文生图 prompt（必填）")
	_ = aidesignGenerateWithImageCmd.MarkFlagRequired("prompt")
	aidesignGenerateWithImageCmd.Flags().String("image-url", "", "参考图片 URL（必填）")
	_ = aidesignGenerateWithImageCmd.MarkFlagRequired("image-url")
	aidesignGenerateWithImageCmd.Flags().Int("width", 0, "图片宽度（可选）")
	aidesignGenerateWithImageCmd.Flags().Int("height", 0, "图片高度（可选）")

	// generate-with-template
	aidesignGenerateWithTemplateCmd.Flags().String("image-url", "", "参考图片 URL（必填）")
	_ = aidesignGenerateWithTemplateCmd.MarkFlagRequired("image-url")
	aidesignGenerateWithTemplateCmd.Flags().String("template", "", "模板 ID（必填）")
	_ = aidesignGenerateWithTemplateCmd.MarkFlagRequired("template")
	aidesignGenerateWithTemplateCmd.Flags().String("name", "", "名字（部分模板需要）")

	// edit
	aidesignEditCmd.Flags().String("prompt", "", "编辑指令（必填）")
	_ = aidesignEditCmd.MarkFlagRequired("prompt")
	aidesignEditCmd.Flags().String("image-url", "", "待编辑图片 URL（必填）")
	_ = aidesignEditCmd.MarkFlagRequired("image-url")

	// upscale
	aidesignUpscaleCmd.Flags().String("image-url", "", "待超分图片 URL（必填）")
	_ = aidesignUpscaleCmd.MarkFlagRequired("image-url")

	// isolate
	aidesignIsolateCmd.Flags().String("image-url", "", "待抠图图片 URL（必填）")
	_ = aidesignIsolateCmd.MarkFlagRequired("image-url")

	root.AddCommand(
		aidesignGenerateCmd,
		aidesignGenerateWithImageCmd,
		aidesignGenerateWithTemplateCmd,
		aidesignEditCmd,
		aidesignUpscaleCmd,
		aidesignIsolateCmd,
	)

	return root
}
