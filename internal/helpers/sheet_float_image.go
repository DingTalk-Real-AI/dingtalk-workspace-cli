package helpers

import (
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

func newFloatImageCmds() []*cobra.Command {
	createFloatImageCmd := &cobra.Command{
		Use:   "create-float-image",
		Short: "创建浮动图片",
		Long: `在钉钉表格的指定工作表上创建一个浮动图片。

浮动图片悬浮于单元格之上，不占用单元格内容，可自由定位和调整大小。

直接传 --file 可在一个命令中上传本地图片并创建浮动图片。
已经通过 media-upload 获得 resourceUrl 时，也可以改传 --src；--file 与 --src 二选一。

--range 指定浮动图片锚定的单元格位置，使用 A1 表示法（如 A1、B3）。
--width / --height 为必填，单位像素，必须为正整数。
--offset-x / --offset-y 可选，表示相对锚点单元格左上角的偏移量（像素），默认 0。`,
		Example: `  # 直接从本地文件创建
  dws sheet create-float-image --node NODE_ID --sheet-id SHEET_ID \
    --file ./chart.png --range A1 --width 400 --height 300

  # 高级用法：先上传图片获取 resourceUrl
  dws sheet media-upload --node NODE_ID --file ./chart.png
  # 输出: resourceUrl: /core/api/resources/img/xxxx...

  # 再通过 --src 创建浮动图片
  dws sheet create-float-image --node NODE_ID --sheet-id SHEET_ID \
    --src "/core/api/resources/img/xxxx..." --range A1 --width 400 --height 300

  # 带偏移量
  dws sheet create-float-image --node NODE_ID --sheet-id SHEET_ID \
    --src "/core/api/resources/img/xxxx..." --range B2 --width 200 --height 150 --offset-x 10 --offset-y 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "range"); err != nil {
				return err
			}
			filePath, src, err := validateFloatImageCreateInput(cmd)
			if err != nil {
				return err
			}
			width, err := cmd.Flags().GetInt("width")
			if err != nil {
				return fmt.Errorf("--width 解析失败: %w", err)
			}
			if width <= 0 {
				return fmt.Errorf("--width 必须为正整数，当前值: %d", width)
			}
			height, err := cmd.Flags().GetInt("height")
			if err != nil {
				return fmt.Errorf("--height 解析失败: %w", err)
			}
			if height <= 0 {
				return fmt.Errorf("--height 必须为正整数，当前值: %d", height)
			}
			toolArgs := map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"range":   mustGetFlag(cmd, "range"),
				"width":   width,
				"height":  height,
			}
			if cmd.Flags().Changed("offset-x") {
				ox, _ := cmd.Flags().GetInt("offset-x")
				if ox < 0 {
					return fmt.Errorf("--offset-x 不能为负数，当前值: %d", ox)
				}
				toolArgs["offsetX"] = ox
			}
			if cmd.Flags().Changed("offset-y") {
				oy, _ := cmd.Flags().GetInt("offset-y")
				if oy < 0 {
					return fmt.Errorf("--offset-y 不能为负数，当前值: %d", oy)
				}
				toolArgs["offsetY"] = oy
			}
			if filePath != "" {
				return runFloatImageFileMode(cmd, "create_float_image", filePath, toolArgs)
			}
			input := map[string]any{
				"sheet-id": mustGetFlag(cmd, "sheet-id"), "src": src,
				"range": mustGetFlag(cmd, "range"), "width": width, "height": height,
			}
			if cmd.Flags().Changed("offset-x") {
				input["offset-x"] = toolArgs["offsetX"]
			}
			if cmd.Flags().Changed("offset-y") {
				input["offset-y"] = toolArgs["offsetY"]
			}
			sharedArgs, err := BuildBatchCreateFloatImageArgs(input)
			if err != nil {
				return err
			}
			sharedArgs["nodeId"] = mustGetFlag(cmd, "node")
			return callMCPToolContext(cmd.Context(), "create_float_image", sharedArgs)
		},
	}
	DeclareLeafMetadata(createFloatImageCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "create_float_image",
				CanonicalPath:  "sheet.create_float_image",
				CLIPath:        "sheet create-float-image",
				PrimaryCLIPath: "sheet create-float-image",
			},
			Description: "从本地文件或已上传的 resourceUrl 创建浮动图片。",
			DryRun:      &contract.DryRunSpec{PreviewKind: "request", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "sheet", RPCName: "create_float_image"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "从本地文件或已上传的 resourceUrl 创建浮动图片。",
				UseWhen:      []string{"需要在单元格上方悬浮图片、不占用单元格内容时"},
				AvoidWhen:    []string{"单元格内嵌图片用 write-image；更新/删除浮动图用 update/delete-float-image"},
				Examples:     []string{"dws sheet create-float-image --node <NODE_ID> --sheet-id <SHEET_ID> --src \"/core/api/resources/img/...\" --range A1 --width 400 --height 300"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
				{Name: "file", Required: boolPtr(false), RequiredWhen: "exactly one of --file or --src must be provided"},
				{Name: "src", Property: "src", Required: boolPtr(false)},
			},
		},
	})
	createFloatImageCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	createFloatImageCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	createFloatImageCmd.Flags().String("file", "", "本地图片文件路径，与 --src 二选一")
	createFloatImageCmd.Flags().String("src", "", "通过 media-upload 获取的 resourceUrl，与 --file 二选一")
	createFloatImageCmd.Flags().String("range", "", "锚点单元格，A1 表示法，如 A1、B3 (必填)")
	createFloatImageCmd.Flags().Int("width", 0, "图片宽度，像素 (必填)")
	createFloatImageCmd.Flags().Int("height", 0, "图片高度，像素 (必填)")
	createFloatImageCmd.Flags().Int("offset-x", 0, "水平偏移量，像素 (默认 0)")
	createFloatImageCmd.Flags().Int("offset-y", 0, "垂直偏移量，像素 (默认 0)")

	getFloatImageCmd := &cobra.Command{
		Use:   "get-float-image",
		Short: "获取浮动图片详情",
		Long: `获取钉钉表格指定工作表中某个浮动图片的详细信息。

返回浮动图片的 ID、图片 URL、锚点位置、尺寸和偏移量等信息。
floatImageId 可通过 list-float-images 获取。`,
		Example: `  dws sheet get-float-image --node NODE_ID --sheet-id SHEET_ID --float-image-id FI_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("get_float_image", map[string]any{
				"nodeId":       mustGetFlag(cmd, "node"),
				"sheetId":      mustGetFlag(cmd, "sheet-id"),
				"floatImageId": mustGetFlag(cmd, "float-image-id"),
			})
		},
	}
	DeclareLeafMetadata(getFloatImageCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "get_float_image",
				CanonicalPath:  "sheet.get_float_image",
				CLIPath:        "sheet get-float-image",
				PrimaryCLIPath: "sheet get-float-image",
			},
			Description: "获取单张浮动图片详情。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "sheet", RPCName: "get_float_image"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取单张浮动图片详情。",
				UseWhen:      []string{"已知 float-image-id，需要查看锚点/尺寸/src 时"},
				AvoidWhen:    []string{"列出全部浮动图用 list-float-images"},
				Examples:     []string{"dws sheet get-float-image --node <NODE_ID> --sheet-id <SHEET_ID> --float-image-id <FI_ID>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
			},
		},
	})
	getFloatImageCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	getFloatImageCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	getFloatImageCmd.Flags().String("float-image-id", "", "浮动图片 ID (必填)")

	listFloatImagesCmd := &cobra.Command{
		Use:   "list-float-images",
		Short: "列出工作表所有浮动图片",
		Long: `列出钉钉表格指定工作表中所有浮动图片。

返回每个浮动图片的 ID、图片 URL、锚点位置、尺寸和偏移量等信息，以及总数。`,
		Example: `  dws sheet list-float-images --node NODE_ID --sheet-id SHEET_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("list_float_images", map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
			})
		},
	}
	DeclareLeafMetadata(listFloatImagesCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "list_float_images",
				CanonicalPath:  "sheet.list_float_images",
				CLIPath:        "sheet list-float-images",
				PrimaryCLIPath: "sheet list-float-images",
			},
			Description: "列出工作表全部浮动图片。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "sheet", RPCName: "list_float_images"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "列出工作表全部浮动图片。",
				UseWhen:      []string{"需要枚举浮动图 ID 以便后续 get/update/delete 时"},
				AvoidWhen:    []string{"查单张详情用 get-float-image"},
				Examples:     []string{"dws sheet list-float-images --node <NODE_ID> --sheet-id <SHEET_ID>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
			},
		},
	})
	listFloatImagesCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	listFloatImagesCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")

	updateFloatImageCmd := &cobra.Command{
		Use:   "update-float-image",
		Short: "更新浮动图片属性",
		Long: `更新钉钉表格指定工作表中浮动图片的属性。

可直接传 --file 上传本地图片并替换浮动图片，也可传已上传图片的 --src。
至少需要传入一个更新字段（--file / --src / --range / --width / --height / --offset-x / --offset-y），且 --file 与 --src 不能同时使用。
floatImageId 可通过 list-float-images 获取。`,
		Example: `  # 移动浮动图片到新位置
  dws sheet update-float-image --node NODE_ID --sheet-id SHEET_ID --float-image-id FI_ID --range C5

  # 调整尺寸
  dws sheet update-float-image --node NODE_ID --sheet-id SHEET_ID --float-image-id FI_ID --width 600 --height 400

  # 直接用本地文件替换图片
  dws sheet update-float-image --node NODE_ID --sheet-id SHEET_ID --float-image-id FI_ID \
    --file ./replacement.png

  # 高级用法：用 media-upload 返回的 resourceUrl 替换图片
  dws sheet update-float-image --node NODE_ID --sheet-id SHEET_ID --float-image-id FI_ID \
    --src "/core/api/resources/img/xxxx..."`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileChanged := cmd.Flags().Changed("file")
			srcChanged := cmd.Flags().Changed("src")
			rangeChanged := cmd.Flags().Changed("range")
			widthChanged := cmd.Flags().Changed("width")
			heightChanged := cmd.Flags().Changed("height")
			oxChanged := cmd.Flags().Changed("offset-x")
			oyChanged := cmd.Flags().Changed("offset-y")

			if !fileChanged && !srcChanged && !rangeChanged && !widthChanged && !heightChanged && !oxChanged && !oyChanged {
				return fmt.Errorf("%s", floatImageUpdateFieldsError)
			}
			filePath, src, err := validateFloatImageUpdateInput(cmd)
			if err != nil {
				return err
			}
			if fileChanged {
				if err := validateRequiredFlags(cmd, "node", "sheet-id", "float-image-id"); err != nil {
					return err
				}
			}

			toolArgs := map[string]any{
				"nodeId":       mustGetFlag(cmd, "node"),
				"sheetId":      mustGetFlag(cmd, "sheet-id"),
				"floatImageId": mustGetFlag(cmd, "float-image-id"),
			}
			if srcChanged {
				toolArgs["src"] = src
			}
			if rangeChanged {
				toolArgs["range"], _ = cmd.Flags().GetString("range")
			}
			if widthChanged {
				w, _ := cmd.Flags().GetInt("width")
				if w <= 0 {
					return fmt.Errorf("--width 必须为正整数，当前值: %d", w)
				}
				toolArgs["width"] = w
			}
			if heightChanged {
				h, _ := cmd.Flags().GetInt("height")
				if h <= 0 {
					return fmt.Errorf("--height 必须为正整数，当前值: %d", h)
				}
				toolArgs["height"] = h
			}
			if oxChanged {
				ox, _ := cmd.Flags().GetInt("offset-x")
				if ox < 0 {
					return fmt.Errorf("--offset-x 不能为负数，当前值: %d", ox)
				}
				toolArgs["offsetX"] = ox
			}
			if oyChanged {
				oy, _ := cmd.Flags().GetInt("offset-y")
				if oy < 0 {
					return fmt.Errorf("--offset-y 不能为负数，当前值: %d", oy)
				}
				toolArgs["offsetY"] = oy
			}
			if fileChanged {
				return runFloatImageFileMode(cmd, "update_float_image", filePath, toolArgs)
			}
			input := map[string]any{
				"sheet-id": mustGetFlag(cmd, "sheet-id"), "float-image-id": mustGetFlag(cmd, "float-image-id"),
			}
			if srcChanged {
				input["src"] = src
			}
			if rangeChanged {
				input["range"] = toolArgs["range"]
			}
			for cliKey, mcpKey := range map[string]string{
				"width": "width", "height": "height", "offset-x": "offsetX", "offset-y": "offsetY",
			} {
				if cmd.Flags().Changed(cliKey) {
					input[cliKey] = toolArgs[mcpKey]
				}
			}
			sharedArgs, err := BuildBatchUpdateFloatImageArgs(input)
			if err != nil {
				return err
			}
			sharedArgs["nodeId"] = mustGetFlag(cmd, "node")
			return callMCPToolContext(cmd.Context(), "update_float_image", sharedArgs)
		},
	}
	DeclareLeafMetadata(updateFloatImageCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "update_float_image",
				CanonicalPath:  "sheet.update_float_image",
				CLIPath:        "sheet update-float-image",
				PrimaryCLIPath: "sheet update-float-image",
			},
			Description: "更新浮动图片锚点、尺寸、偏移或资源路径。",
			DryRun:      &contract.DryRunSpec{PreviewKind: "request", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "sheet", RPCName: "update_float_image"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新浮动图片锚点、尺寸、偏移或资源路径。",
				UseWhen:      []string{"需要移动/缩放/替换已有浮动图片时"},
				AvoidWhen:    []string{"创建用 create-float-image；删除用 delete-float-image"},
				Examples:     []string{"dws sheet update-float-image --node <NODE_ID> --sheet-id <SHEET_ID> --float-image-id <FI_ID> --range C5"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
				{Name: "file", Required: boolPtr(false)},
				{Name: "src", Property: "src", Required: boolPtr(false)},
			},
		},
	})
	updateFloatImageCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	updateFloatImageCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	updateFloatImageCmd.Flags().String("float-image-id", "", "浮动图片 ID (必填)")
	updateFloatImageCmd.Flags().String("file", "", "用于替换浮动图片的本地图片路径，与 --src 不能同时使用")
	updateFloatImageCmd.Flags().String("src", "", "新的图片资源路径，通过 media-upload 获取的 resourceUrl")
	updateFloatImageCmd.Flags().String("range", "", "新的锚点单元格，A1 表示法")
	updateFloatImageCmd.Flags().Int("width", 0, "新的图片宽度，像素")
	updateFloatImageCmd.Flags().Int("height", 0, "新的图片高度，像素")
	updateFloatImageCmd.Flags().Int("offset-x", 0, "新的水平偏移量，像素")
	updateFloatImageCmd.Flags().Int("offset-y", 0, "新的垂直偏移量，像素")

	deleteFloatImageCmd := &cobra.Command{
		Use:   "delete-float-image",
		Short: "删除浮动图片",
		Long: `删除钉钉表格指定工作表中的浮动图片。

操作不可恢复，删除后图片将从工作表中移除。
floatImageId 可通过 list-float-images 获取。`,
		Example: `  dws sheet delete-float-image --node NODE_ID --sheet-id SHEET_ID --float-image-id FI_ID --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("delete_float_image", map[string]any{
				"nodeId":       mustGetFlag(cmd, "node"),
				"sheetId":      mustGetFlag(cmd, "sheet-id"),
				"floatImageId": mustGetFlag(cmd, "float-image-id"),
			})
		},
	}
	DeclareLeafMetadata(deleteFloatImageCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "delete_float_image",
				CanonicalPath:  "sheet.delete_float_image",
				CLIPath:        "sheet delete-float-image",
				PrimaryCLIPath: "sheet delete-float-image",
			},
			Description: "删除浮动图片（需确认后加 --yes）。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "sheet", RPCName: "delete_float_image"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "删除浮动图片（需确认后加 --yes）。",
				UseWhen:      []string{"用户明确要求删除某张浮动图片时"},
				AvoidWhen:    []string{"删除单元格内嵌图需改写单元格；列目录用 list-float-images"},
				Examples:     []string{"dws sheet delete-float-image --node <NODE_ID> --sheet-id <SHEET_ID> --float-image-id <FI_ID>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
			},
		},
	})
	deleteFloatImageCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	deleteFloatImageCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	deleteFloatImageCmd.Flags().String("float-image-id", "", "浮动图片 ID (必填)")

	return []*cobra.Command{createFloatImageCmd, getFloatImageCmd, listFloatImagesCmd, updateFloatImageCmd, deleteFloatImageCmd}
}
