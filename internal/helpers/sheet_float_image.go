package helpers

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newFloatImageCmds() []*cobra.Command {
	createFloatImageCmd := &cobra.Command{
		Use:   "create-float-image",
		Short: "创建浮动图片",
		Long: `在钉钉表格的指定工作表上创建一个浮动图片。

浮动图片悬浮于单元格之上，不占用单元格内容，可自由定位和调整大小。

使用流程：
  1. 先通过 media-upload 上传本地图片获取 resourceUrl
  2. 再通过 create-float-image 将图片以浮动方式放置到工作表上

--range 指定浮动图片锚定的单元格位置，使用 A1 表示法（如 A1、B3）。
--width / --height 为必填，单位像素，必须为正整数。
--offset-x / --offset-y 可选，表示相对锚点单元格左上角的偏移量（像素），默认 0。`,
		Example: `  # 先上传图片获取 resourceUrl
  dws sheet media-upload --node NODE_ID --file ./chart.png
  # 输出: resourceUrl: /core/api/resources/img/xxxx...

  # 再创建浮动图片（--src 传入 media-upload 返回的 resourceUrl）
  dws sheet create-float-image --node NODE_ID --sheet-id SHEET_ID \
    --src "/core/api/resources/img/xxxx..." --range A1 --width 400 --height 300

  # 带偏移量
  dws sheet create-float-image --node NODE_ID --sheet-id SHEET_ID \
    --src "/core/api/resources/img/xxxx..." --range B2 --width 200 --height 150 --offset-x 10 --offset-y 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "src", "range"); err != nil {
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
				"src":     mustGetFlag(cmd, "src"),
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
			return callMCPTool("create_float_image", toolArgs)
		},
	}
	createFloatImageCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	createFloatImageCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	createFloatImageCmd.Flags().String("src", "", "图片资源路径，通过 media-upload 获取的 resourceUrl (必填)")
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
	listFloatImagesCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	listFloatImagesCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")

	updateFloatImageCmd := &cobra.Command{
		Use:   "update-float-image",
		Short: "更新浮动图片属性",
		Long: `更新钉钉表格指定工作表中浮动图片的属性。

可更新的属性包括：图片资源路径（src）、锚点位置、尺寸、偏移量。
至少需要传入一个更新字段（--src / --range / --width / --height / --offset-x / --offset-y）。
floatImageId 可通过 list-float-images 获取。`,
		Example: `  # 移动浮动图片到新位置
  dws sheet update-float-image --node NODE_ID --sheet-id SHEET_ID --float-image-id FI_ID --range C5

  # 调整尺寸
  dws sheet update-float-image --node NODE_ID --sheet-id SHEET_ID --float-image-id FI_ID --width 600 --height 400

  # 替换图片（需先 media-upload 新图片获取 resourceUrl）
  dws sheet update-float-image --node NODE_ID --sheet-id SHEET_ID --float-image-id FI_ID \
    --src "/core/api/resources/img/xxxx..."`,
		RunE: func(cmd *cobra.Command, args []string) error {
			srcChanged := cmd.Flags().Changed("src")
			rangeChanged := cmd.Flags().Changed("range")
			widthChanged := cmd.Flags().Changed("width")
			heightChanged := cmd.Flags().Changed("height")
			oxChanged := cmd.Flags().Changed("offset-x")
			oyChanged := cmd.Flags().Changed("offset-y")

			if !srcChanged && !rangeChanged && !widthChanged && !heightChanged && !oxChanged && !oyChanged {
				return fmt.Errorf("--src、--range、--width、--height、--offset-x、--offset-y 至少必须提供一个")
			}

			toolArgs := map[string]any{
				"nodeId":       mustGetFlag(cmd, "node"),
				"sheetId":      mustGetFlag(cmd, "sheet-id"),
				"floatImageId": mustGetFlag(cmd, "float-image-id"),
			}
			if srcChanged {
				toolArgs["src"], _ = cmd.Flags().GetString("src")
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
			return callMCPTool("update_float_image", toolArgs)
		},
	}
	updateFloatImageCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	updateFloatImageCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	updateFloatImageCmd.Flags().String("float-image-id", "", "浮动图片 ID (必填)")
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
	deleteFloatImageCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	deleteFloatImageCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	deleteFloatImageCmd.Flags().String("float-image-id", "", "浮动图片 ID (必填)")

	return []*cobra.Command{createFloatImageCmd, getFloatImageCmd, listFloatImagesCmd, updateFloatImageCmd, deleteFloatImageCmd}
}
