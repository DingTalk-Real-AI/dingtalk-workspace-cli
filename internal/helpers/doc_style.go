package helpers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

// doc_style.go — `dws doc style`（文档封面/背景样式配置）。
// 写入收口到 MCP 工具 update_document_style；读取用 get_document_style。

const docStyleToolName = "update_document_style"

const docStyleGetToolName = "get_document_style"

// newDocStyleCommand 构建 `dws doc style` 命令组：cover set|clear、background set|clear、get。
func newDocStyleCommand() *cobra.Command {
	styleCmd := &cobra.Command{
		Use:   "style",
		Short: "文档样式配置 (封面/背景)",
		Long: `配置钉钉文档的封面与背景（单接口收口 update_document_style）。

命令结构:
  dws doc style cover set          设置文档封面
  dws doc style cover clear        移除文档封面
  dws doc style background set      设置文档背景纯色
  dws doc style background clear    清除文档背景
  dws doc style get                读取文档封面/背景 (只读)

封面图片支持 --image 外链 (自动转存) 或 --file 本地文件上传，均会转存为公开读地址；背景仅支持 --color 纯色。`,
		RunE: groupRunE,
	}

	coverCmd := &cobra.Command{
		Use:   "cover",
		Short: "文档封面设置/移除",
		RunE:  groupRunE,
	}
	coverSetCmd := &cobra.Command{
		Use:   "set",
		Short: "设置文档封面",
		Long:  `为钉钉文档设置顶部封面图，可指定竖直显示位置。图片来源二选一：--image 外链或 --file 本地文件。`,
		Example: `  dws doc style cover set --node DOC_ID --image https://img.example.com/cover.png
  dws doc style cover set --node DOC_ID --file ./cover.png --position 0.3`,
		RunE: runDocStyleCoverSet,
	}
	DeclareLeafMetadata(coverSetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "style_cover_set",
				CanonicalPath:  "doc.style_cover_set",
				CLIPath:        "doc style cover set",
				PrimaryCLIPath: "doc style cover set",
			},
			Description: "设置钉钉文档顶部封面图（外链或本地图片上传）",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "设置钉钉文档顶部封面图（外链或本地图片上传）",
				UseWhen:      []string{"用户说 给文档设置封面/换个封面图"},
				AvoidWhen:    []string{"读取当前封面用 doc style get；移除封面用 cover clear"},
				Examples:     []string{"dws doc style cover set --node <DOC_ID> --image https://img.example.com/cover.png --format json"},
			},
		},
	})
	coverClearCmd := &cobra.Command{
		Use:     "clear",
		Short:   "移除文档封面",
		Example: `  dws doc style cover clear --node DOC_ID`,
		RunE:    runDocStyleCoverClear,
	}
	DeclareLeafMetadata(coverClearCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "style_cover_clear",
				CanonicalPath:  "doc.style_cover_clear",
				CLIPath:        "doc style cover clear",
				PrimaryCLIPath: "doc style cover clear",
			},
			Description: "移除钉钉文档封面",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "移除钉钉文档封面",
				UseWhen:      []string{"用户说 去掉文档封面"},
				AvoidWhen:    []string{"设置封面用 cover set"},
				Examples:     []string{"dws doc style cover clear --node <DOC_ID> --format json"},
			},
		},
	})

	backgroundCmd := &cobra.Command{
		Use:   "background",
		Short: "文档背景设置/清除",
		RunE:  groupRunE,
	}
	backgroundSetCmd := &cobra.Command{
		Use:     "set",
		Short:   "设置文档背景纯色",
		Long:    `为钉钉文档设置背景纯色（--color）。背景仅支持纯色，不支持背景图片上传（对齐前端能力）。`,
		Example: `  dws doc style background set --node DOC_ID --color "#E8F2FE"`,
		RunE:    runDocStyleBackgroundSet,
	}
	DeclareLeafMetadata(backgroundSetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "style_background_set",
				CanonicalPath:  "doc.style_background_set",
				CLIPath:        "doc style background set",
				PrimaryCLIPath: "doc style background set",
			},
			Description: "设置钉钉文档背景纯色（#RRGGBB）",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "设置钉钉文档背景纯色（#RRGGBB）",
				UseWhen:      []string{"用户说 给文档设置背景色"},
				AvoidWhen:    []string{"背景不支持图片；封面用 cover set"},
				Examples:     []string{"dws doc style background set --node <DOC_ID> --color \"#E8F2FE\" --format json"},
			},
		},
	})
	backgroundClearCmd := &cobra.Command{
		Use:     "clear",
		Short:   "清除文档背景",
		Example: `  dws doc style background clear --node DOC_ID`,
		RunE:    runDocStyleBackgroundClear,
	}
	DeclareLeafMetadata(backgroundClearCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "style_background_clear",
				CanonicalPath:  "doc.style_background_clear",
				CLIPath:        "doc style background clear",
				PrimaryCLIPath: "doc style background clear",
			},
			Description: "清除钉钉文档背景",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "清除钉钉文档背景",
				UseWhen:      []string{"用户说 去掉文档背景色"},
				AvoidWhen:    []string{"设置背景用 background set"},
				Examples:     []string{"dws doc style background clear --node <DOC_ID> --format json"},
			},
		},
	})
	getCmd := &cobra.Command{
		Use:     "get",
		Short:   "读取文档封面/背景 (只读)",
		Long:    `读取钉钉文档当前的封面与背景配置（只读，单接口收口 get_document_style）。`,
		Example: `  dws doc style get --node DOC_ID`,
		RunE:    runDocStyleGet,
	}
	DeclareLeafMetadata(getCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "doc",
				Name:           "get_document_style",
				CanonicalPath:  "doc.get_document_style",
				CLIPath:        "doc style get",
				PrimaryCLIPath: "doc style get",
			},
			Description: "读取文档当前封面与背景配置（只读）",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "读取文档当前封面与背景配置（只读）",
				UseWhen:      []string{"用户说 看下文档现在的封面/背景"},
				AvoidWhen:    []string{"修改用 cover set / background set"},
				Examples:     []string{"dws doc style get --node <DOC_ID> --format json"},
			},
		},
	})

	// cover set flags
	coverSetCmd.Flags().String("node", "", "目标文档标识，支持 URL 或 ID (必填)")
	coverSetCmd.Flags().String("image", "", "封面图片 URL (外链会自动转存为内部地址)")
	coverSetCmd.Flags().String("file", "", "本地图片文件路径 (与 --image 互斥)")
	coverSetCmd.Flags().Float64("position", 0.5, "封面竖直位置 [0,1]，默认 0.5")
	coverSetCmd.MarkFlagsMutuallyExclusive("image", "file")

	// cover clear flags
	coverClearCmd.Flags().String("node", "", "目标文档标识，支持 URL 或 ID (必填)")

	// background set flags（仅纯色，对齐前端；不支持背景图片上传）
	backgroundSetCmd.Flags().String("node", "", "目标文档标识，支持 URL 或 ID (必填)")
	backgroundSetCmd.Flags().String("color", "", "背景纯色，如 #E8F2FE (必填)")

	// background clear flags
	backgroundClearCmd.Flags().String("node", "", "目标文档标识，支持 URL 或 ID (必填)")

	// get flags
	getCmd.Flags().String("node", "", "目标文档标识，支持 URL 或 ID (必填)")

	// --node 隐藏别名 (--url/--id/--node-id/--doc-id/--file-id)
	for _, c := range []*cobra.Command{coverSetCmd, coverClearCmd, backgroundSetCmd, backgroundClearCmd, getCmd} {
		c.Flags().String("url", "", "--node 的别名")
		c.Flags().String("id", "", "--node 的别名")
		c.Flags().String("node-id", "", "--node 的别名")
		c.Flags().String("doc-id", "", "--node 的别名")
		c.Flags().String("file-id", "", "--node 的别名 (跨产品兼容 drive)")
		_ = c.Flags().MarkHidden("url")
		_ = c.Flags().MarkHidden("id")
		_ = c.Flags().MarkHidden("node-id")
		_ = c.Flags().MarkHidden("doc-id")
		_ = c.Flags().MarkHidden("file-id")
	}

	coverCmd.AddCommand(coverSetCmd, coverClearCmd)
	backgroundCmd.AddCommand(backgroundSetCmd, backgroundClearCmd)
	styleCmd.AddCommand(coverCmd, backgroundCmd, getCmd)

	// 递归注册跨产品别名，让 cover set 的 --file 获得统一别名 --file-path。
	registerCrossProductAliasesRecursive(styleCmd)
	return styleCmd
}

// registerCrossProductAliasesRecursive 递归为 cmd 及其所有子命令注册跨产品别名。
// 可正确处理多层分组（如 style 的 cover/background 组下还有 set/clear 叶子）。
func registerCrossProductAliasesRecursive(cmd *cobra.Command) {
	RegisterCrossProductAliases(cmd)
	for _, sub := range cmd.Commands() {
		registerCrossProductAliasesRecursive(sub)
	}
}

// docStyleNodeID 解析目标文档标识，支持 --node 及隐藏别名。
func docStyleNodeID(cmd *cobra.Command) (string, error) {
	return mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
}

func runDocStyleCoverSet(cmd *cobra.Command, _ []string) error {
	nodeID, err := docStyleNodeID(cmd)
	if err != nil {
		return err
	}
	// 封面图片来源二选一：--image 外链（服务端转存公开）或 --file 本地上传（拿 resourceId 转存公开）。
	image, _ := cmd.Flags().GetString("image")
	filePath := flagOrFallback(cmd, "file", "file-path")
	// --image 与 --file/--file-path 互斥。cobra 的 MarkFlagsMutuallyExclusive 只认主名
	// image/file，拦不住别名 --file-path，故在此显式兜底。
	if image != "" && filePath != "" {
		return fmt.Errorf("--image and --file/--file-path are mutually exclusive; specify only one")
	}
	if image == "" && filePath == "" {
		return fmt.Errorf("flag --image or --file is required for `cover set`")
	}

	cover := map[string]any{"action": "set"}
	// position 客户端校验：帮助文档承诺 [0,1]，越界直接报错，避免触发上传/请求。
	if cmd.Flags().Changed("position") {
		pos, _ := cmd.Flags().GetFloat64("position")
		if pos < 0 || pos > 1 {
			return fmt.Errorf("--position must be within [0,1], got %v", pos)
		}
		cover["position"] = pos
	}

	if image != "" {
		cover["imageUrl"] = image
	} else {
		// --file：先做本地校验（存在 + 必须为图片）；dry-run 时在上传前短路，
		// 绝不读取/上传本地文件。
		mimeType, fileSize, err := validateCoverImageFile(filePath)
		if err != nil {
			return err
		}
		if deps.Caller.DryRun() {
			cover["resourceId"] = fmt.Sprintf("(pending upload from --file %s)", filePath)
		} else {
			// 使用命令上下文，让 Ctrl-C 能够取消上传凭证获取与 OSS 上传两次网络请求。
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			resourceID, err := uploadDocStyleImage(ctx, nodeID, filePath, filepath.Base(filePath), mimeType, fileSize)
			if err != nil {
				return err
			}
			cover["resourceId"] = resourceID
		}
	}

	return callMCPToolOnServer("doc", docStyleToolName, map[string]any{
		"nodeId": nodeID,
		"cover":  cover,
	})
}

func runDocStyleCoverClear(cmd *cobra.Command, _ []string) error {
	nodeID, err := docStyleNodeID(cmd)
	if err != nil {
		return err
	}
	return callMCPToolOnServer("doc", docStyleToolName, map[string]any{
		"nodeId": nodeID,
		"cover":  map[string]any{"action": "clear"},
	})
}

func runDocStyleBackgroundSet(cmd *cobra.Command, _ []string) error {
	nodeID, err := docStyleNodeID(cmd)
	if err != nil {
		return err
	}
	// 背景仅支持纯色（对齐前端，不支持背景图片上传）。
	color, _ := cmd.Flags().GetString("color")
	if color == "" {
		return fmt.Errorf("flag --color is required for `background set`")
	}
	// 帮助文档承诺 #RRGGBB 十六进制纯色，非法值直接报错，不下发请求。
	if !isHexColor(color) {
		return fmt.Errorf("--color must be a hex color like #E8F2FE, got %q", color)
	}
	return callMCPToolOnServer("doc", docStyleToolName, map[string]any{
		"nodeId": nodeID,
		"background": map[string]any{
			"action":          "set",
			"backgroundColor": color,
		},
	})
}

func runDocStyleBackgroundClear(cmd *cobra.Command, _ []string) error {
	nodeID, err := docStyleNodeID(cmd)
	if err != nil {
		return err
	}
	return callMCPToolOnServer("doc", docStyleToolName, map[string]any{
		"nodeId":     nodeID,
		"background": map[string]any{"action": "clear"},
	})
}

// runDocStyleGet 读取文档当前封面/背景（只读）。
func runDocStyleGet(cmd *cobra.Command, _ []string) error {
	nodeID, err := docStyleNodeID(cmd)
	if err != nil {
		return err
	}
	return callMCPToolOnServer("doc", docStyleGetToolName, map[string]any{
		"nodeId": nodeID,
	})
}

// docStyleCoverMaxBytes 是 cover set --file 本地图片的大小上限（20 MiB）。
// 悟空参考实现未设上限，此处补充客户端 fail-fast，避免超大文件被先行上传。
const docStyleCoverMaxBytes = 20 << 20

// validateCoverImageFile 对 cover set --file 做本地校验（不上传/不读取文件内容）：
// 文件须存在、非目录、不超过大小上限，且按扩展名推断的 MIME 必须为 image/*。
// 返回 mimeType 与文件大小，供 dry-run 预览与真实上传前 fail-fast，避免非法文件被先行上传。
func validateCoverImageFile(filePath string) (mimeType string, fileSize int64, err error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("cannot read file %s: %w", filePath, err)
	}
	if fileInfo.IsDir() {
		return "", 0, fmt.Errorf("%s is a directory, not a file", filePath)
	}
	if fileInfo.Size() > docStyleCoverMaxBytes {
		return "", 0, fmt.Errorf("封面图片文件过大：上限 %d 字节 (20 MiB)，实际 %d 字节 (%s)", int64(docStyleCoverMaxBytes), fileInfo.Size(), filepath.Base(filePath))
	}
	mimeType = inferMimeType(filepath.Base(filePath))
	if !strings.HasPrefix(mimeType, "image/") {
		return "", 0, fmt.Errorf("cover --file must be an image (png/jpg/jpeg/gif/svg/webp), got %s", filepath.Base(filePath))
	}
	return mimeType, fileInfo.Size(), nil
}

// isHexColor 校验 #RRGGBB 形式的十六进制颜色（大小写均可）。
func isHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// uploadDocStyleImage 复用附件上传子流程（get_doc_attachment_upload_info + httpPutFile），
// 上传本地文件为鉴权资源，返回其 resourceId（供封面在服务端转存为公开读地址）。
// 入参 mimeType/fileSize 由调用方经 validateCoverImageFile 预校验得到；进度写 stderr，
// 保证 --format json 时 stdout 只含合法 JSON。本函数一定发起网络请求，调用方须自行做 dry-run 短路。
func uploadDocStyleImage(ctx context.Context, nodeID, filePath, fileName, mimeType string, fileSize int64) (string, error) {
	deps.Out.PrintProgress(fmt.Sprintf("[1/2] 获取图片上传凭证 (%s, %d bytes)...", fileName, fileSize))
	credText, err := callMCPToolReturnTextOnServer(ctx, "doc", "get_doc_attachment_upload_info", map[string]any{
		"nodeId":   nodeID,
		"fileName": fileName,
		"fileSize": float64(fileSize),
		"mimeType": mimeType,
	})
	if err != nil {
		return "", err
	}
	uploadURL, resourceID, _, err := parseAttachmentUploadInfo(credText)
	if err != nil {
		return "", err
	}

	deps.Out.PrintProgress("[2/2] 上传图片到 OSS...")
	ossHeaders := map[string]string{"Content-Type": mimeType}
	if err := httpPutFile(ctx, uploadURL, ossHeaders, filePath, fileSize); err != nil {
		return "", err
	}
	return resourceID, nil
}
