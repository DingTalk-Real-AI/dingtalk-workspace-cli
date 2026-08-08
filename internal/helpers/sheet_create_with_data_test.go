package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"io"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// installSheetProductArgs 让 callMCPToolReturnText 能把工具解析到 sheet server
// （产品解析读 os.Args，与其他 sheet 测试一致）。
func installSheetProductArgs(t *testing.T) {
	t.Helper()
	testseam.Swap(t, &os.Args, []string{"dws", "sheet"})
}

// newCreateCmdForTest 取出 sheet create 命令本体，便于直接驱动 RunE。
func newCreateCmdForTest(t *testing.T) *cobra.Command {
	t.Helper()
	for _, cmd := range newWorkbookCmds() {
		if cmd.Name() == "create" {
			return cmd
		}
	}
	t.Fatal("create command not found in newWorkbookCmds()")
	return nil
}

// runCreate 用脚本化的 MCP 响应驱动一次 sheet create。
func runCreate(t *testing.T, caller *scriptedToolCaller, flags map[string]string) error {
	t.Helper()
	installScriptedCaller(t, caller)
	installImmediateTiming(t)
	installSheetProductArgs(t)
	cmd := newCreateCmdForTest(t)
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	return cmd.RunE(cmd, nil)
}

// createOKSteps 是「建表 → 探活/定位默认表 → 写数据 → 回读校验」的成功响应序列。
// resolveFirstSheetID 在 waitSheetWritable 和随后的定位里各调一次 get_all_sheets。
func createOKSteps(csvReadback string) []scriptedToolStep {
	return []scriptedToolStep{
		{text: `{"nodeId":"NODE_1","docUrl":"https://alidocs.test/i/nodes/NODE_1"}`}, // create_workspace_sheet
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},                 // waitSheetWritable 探活
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},                 // resolveFirstSheetID
		{text: `{"success":true}`},                                                   // set_range_from_csv
		{text: `{"csv":"` + csvReadback + `"}`},                                      // get_range_as_csv 回读
	}
}

func TestSheetCreateEmptyKeepsSingleCallBehaviour(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"nodeId":"N"}`}}}
	if err := runCreate(t, caller, map[string]string{"name": "空表"}); err != nil {
		t.Fatalf("create without data: %v", err)
	}
	if caller.calls != 1 {
		t.Fatalf("calls = %d, want 1 (不带初始数据时只调 create_workspace_sheet)", caller.calls)
	}
	if caller.tool != "create_workspace_sheet" {
		t.Fatalf("tool = %q", caller.tool)
	}
}

func TestSheetCreateRejectsConflictingAndOrphanFlags(t *testing.T) {
	cases := []struct {
		name  string
		flags map[string]string
		want  string
	}{
		{
			name:  "styles-without-data",
			flags: map[string]string{"name": "X", "styles": `{"styles":[{"name":"S","cell_merges":[{"range":"A1:B1"}]}]}`},
			want:  "需与 --values 或 --sheets 搭配",
		},
		{
			name:  "values-and-sheets",
			flags: map[string]string{"name": "X", "values": `[[1]]`, "sheets": `[{"name":"S"}]`},
			want:  "二选一",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			err := runCreate(t, caller, tc.flags)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contains %q", err, tc.want)
			}
			if caller.calls != 0 {
				t.Fatalf("calls = %d, want 0 (参数冲突必须在发请求前失败)", caller.calls)
			}
		})
	}
}

// 关键不变量：所有 JSON/枚举校验都必须在 create_workspace_sheet 之前完成，
// 否则会留下一份白建的空文档。
func TestSheetCreateValidatesBeforeCreatingDocument(t *testing.T) {
	cases := []struct {
		name  string
		flags map[string]string
		want  string
	}{
		{"values-bad-json", map[string]string{"name": "X", "values": `[[`}, "--values JSON 解析失败"},
		// Decode 只读一个值就返回：粘贴多了一段/shell 拼接残留会静默丢掉后半截，
		// 却照常建出文档。必须要求读到 EOF。
		{"values-trailing-data", map[string]string{"name": "X", "values": `[[1]] trailing-data`}, "--values 只接受单个 JSON 值，末尾有多余内容"},
		{"values-two-json-values", map[string]string{"name": "X", "values": `[["a"]][["b"]]`}, "--values 只接受单个 JSON 值，末尾有多余内容"},
		// `null` 是合法 JSON 但解析出 nil slice：判据若用 values != nil 会误入
		// --sheets 分支并对空 sheetSpecs 取 [0] panic。
		{"values-null", map[string]string{"name": "X", "values": `null`}, "--values 不能为空"},
		// `[]` 一个字节都写不出，若放过去会白建空文档再报成"写入未生效"。
		{"values-empty-array", map[string]string{"name": "X", "values": `[]`}, "--values 不能为空"},
		// 全空矩阵写不出任何数据，同样必须在建文档之前拒掉
		{"values-all-cells-empty", map[string]string{"name": "X", "values": `[["",""],["",""]]`}, "--values 全部单元格为空"},
		{"sheets-bad-json", map[string]string{"name": "X", "sheets": `{`}, "--sheets JSON 解析失败"},
		{"sheets-trailing-data", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"]}] trailing-data`}, "--sheets 只接受单个 JSON 值，末尾有多余内容"},
		{"sheets-two-json-values", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"]}][{"name":"二月","columns":["a"]}]`}, "--sheets 只接受单个 JSON 值，末尾有多余内容"},
		{"sheets-wrong-type", map[string]string{"name": "X", "sheets": `"str"`}, "必须是 JSON 数组"},
		{"sheets-empty", map[string]string{"name": "X", "sheets": `[]`}, "不能为空数组"},
		{"sheets-item-not-object", map[string]string{"name": "X", "sheets": `[1]`}, "不是对象"},
		{"sheets-missing-name", map[string]string{"name": "X", "sheets": `[{"columns":["a"]}]`}, "缺少必填的 name"},
		// 重名工作表：table_put 会建出两张同名表，之后 --styles 按名称定位会落到
		// 不确定的那一张。--styles 是建文档后的非原子步骤，必须前置拒绝。
		{"sheets-duplicate-name", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"]},{"name":"一月","columns":["a"]}]`}, `--sheets[1].name="一月" 与 --sheets[0] 重复`},
		{"sheets-name-wrong-type", map[string]string{"name": "X", "sheets": `[{"name":1,"columns":["a"]}]`}, "--sheets[0].name 必须是字符串"},
		{"sheets-name-blank", map[string]string{"name": "X", "sheets": `[{"name":"  ","columns":["a"]}]`}, "--sheets[0] 缺少必填的 name"},
		// 畸形包装：出现 sheets 键就必须按包装对象处理，不能退化成"整个对象是单个 spec"，
		// 否则 {"sheets":"bad","name":"一月"} 会白建一份只有名字的文档。
		{"sheets-wrapper-not-array", map[string]string{"name": "X", "sheets": `{"sheets":"bad","name":"一月"}`}, "--sheets.sheets 必须是数组，实际是 string"},
		// 以下都是 table_put 的输入契约校验：类型/枚举错误若放过去，会先建文档、
		// 重命名，再在 table_put 阶段失败，留下用户并未请求到的半成品文档。
		{"spec-data-not-array", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"data":{"bad":1}}]`}, "--sheets[0]: data 必须是二维数组，实际是 map[string]interface {}"},
		{"spec-data-row-not-array", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"data":[5]}]`}, "--sheets[0]: data[0] 必须是数组，实际是 json.Number"},
		{"spec-data-row-width-mismatch", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a","b"],"data":[["x"]]}]`}, "--sheets[0]: data[0] 有 1 列，与 columns 的 2 列不一致"},
		{"spec-data-cell-not-scalar", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"data":[[{"x":1}]]}]`}, "--sheets[0]: data[0][0] 必须是字符串/数字/布尔/null"},
		{"spec-missing-columns", map[string]string{"name": "X", "sheets": `[{"name":"一月","data":[["x"]]}]`}, "--sheets[0]: 缺少必填的 columns"},
		{"spec-columns-wrong-type", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":"a"}]`}, "--sheets[0]: columns 必须是字符串数组，实际是 string"},
		{"spec-columns-empty", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":[]}]`}, "--sheets[0]: columns 不能为空数组"},
		{"spec-columns-item-not-string", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":[1]}]`}, "--sheets[0]: columns[0] 必须是字符串，实际是 json.Number"},
		{"spec-columns-item-blank", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"," "]}]`}, "--sheets[0]: columns[1] 不能为空字符串"},
		// 服务端按 trim 后的列名判重，本地必须用同一套归一化，否则 ["量","量 "] 只会
		// 在 table_put 阶段被拒。
		{"spec-columns-duplicate", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["量","量 "]}]`}, `--sheets[0]: columns[1]="量" 与 columns[0] 重复`},
		{"spec-dtypes-not-object", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"dtypes":["int64"]}]`}, "--sheets[0]: dtypes 必须是对象"},
		{"spec-dtypes-value-not-string", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"dtypes":{"a":1}}]`}, `--sheets[0]: dtypes["a"] 必须是字符串`},
		// dtypes/formats 的键按列名查表，写错既不报错也不生效——静默失效比报错更难发现。
		{"spec-dtypes-unknown-column", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"dtypes":{"b":"int64"}}]`}, `--sheets[0]: dtypes 的键 "b" 不是 columns 中的列名`},
		{"spec-formats-not-object", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"formats":"0.00"}]`}, "--sheets[0]: formats 必须是对象"},
		{"spec-cell-styles-wrong-type", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"cellStyles":"bold"}]`}, "--sheets[0]: cellStyles 必须是对象或二维数组"},
		{"spec-mode-invalid", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"mode":"upsert"}]`}, `--sheets[0]: mode="upsert" 非法（合法值: overwrite / append）`},
		{"spec-mode-wrong-type", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"mode":true}]`}, "--sheets[0]: mode 必须是字符串，实际是 bool"},
		{"spec-header-wrong-type", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"header":"yes"}]`}, "--sheets[0]: header 必须是布尔值，实际是 string"},
		{"spec-allow-overwrite-wrong-type", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"allowOverwrite":"yes"}]`}, "--sheets[0]: allowOverwrite 必须是布尔值"},
		{"spec-start-cell-wrong-type", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"startCell":3}]`}, "--sheets[0]: startCell 必须是字符串，实际是 json.Number"},
		// 非法 startCell 以前被静默忽略：写入按 A1 落盘，回读探针却按错位地址找，
		// 成功的写入会被报成"数据未落盘"。
		{"spec-start-cell-invalid", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"startCell":"A1:B2"}]`}, `--sheets[0]: startCell="A1:B2" 不是合法的单元格地址`},
		// 小写与 $ 绝对引用是服务端接受的写法，不能误拒（见 normalizeSpecStartCell）
		{"spec-start-cell-absolute-accepted", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"startCell":"$b$2","data":{"bad":1}}]`}, "data 必须是二维数组"},
		// 拼错的字段会被服务端 DTO 静默丢弃：datas 会写出"只有表头"的表，而探针刚好
		// 落在表头上，整条命令报成功——静默丢数据必须前置拒绝。
		{"spec-unknown-field", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"datas":[["x"]]}]`}, `--sheets[0]: 未知字段 "datas"`},
		{"spec-snake-case-field", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"start_cell":"B2"}]`}, `--sheets[0]: 字段 "start_cell" 拼写不符合契约，应为 "startCell"`},
		// 文档此刻还不存在，任何 sheetId 都不可能属于它；而重命名默认表和回读校验
		// 都按 name 定位，服务端却优先用 sheetId。
		{"spec-sheet-id-rejected", map[string]string{"name": "X", "sheets": `[{"name":"一月","columns":["a"],"sheetId":"S1"}]`}, "--sheets[0]: 不支持 sheetId"},
		{"styles-bad-json", map[string]string{"name": "X", "values": `[[1]]`, "styles": `{`}, "--styles JSON 解析失败"},
		{"styles-object-without-styles-key", map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"a":1}`}, "必须含 styles 数组"},
		{"styles-wrong-type", map[string]string{"name": "X", "values": `[[1]]`, "styles": `"s"`}, `必须是 {"styles":[...]}`},
		{"styles-empty", map[string]string{"name": "X", "values": `[[1]]`, "styles": `[]`}, "不能为空数组"},
		{"styles-item-empty", map[string]string{"name": "X", "values": `[[1]]`, "styles": `[{"name":"S"}]`}, "至少需要 cell_styles"},
		{
			"styles-count-mismatch-with-sheets",
			map[string]string{"name": "X", "sheets": `[{"name":"A","columns":["a"]},{"name":"B","columns":["b"]}]`, "styles": `[{"name":"A","cell_merges":[{"range":"A1:B1"}]}]`},
			"必须与 --sheets 子表数",
		},
		{
			"styles-name-mismatch-with-sheets",
			map[string]string{"name": "X", "sheets": `[{"name":"A","columns":["a"]}]`, "styles": `[{"name":"Z","cell_merges":[{"range":"A1:B1"}]}]`},
			"不一致（需顺序对应）",
		},
		{
			"styles-multiple-with-values",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `[{"name":"A","cell_merges":[{"range":"A1"}]},{"name":"B","cell_merges":[{"range":"A1"}]}]`},
			"只能有 1 项",
		},
		{
			"cell-styles-missing-range",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","cell_styles":[{"font_weight":"bold"}]}]}`},
			"cell_styles 项缺少必填的 range",
		},
		{
			"cell-styles-bad-range",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","cell_styles":[{"range":"zz","font_weight":"bold"}]}]}`},
			"无效单元格地址",
		},
		{
			"cell-styles-no-style-field",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","cell_styles":[{"range":"A1:B2"}]}]}`},
			"至少需要指定一个样式参数",
		},
		{
			"row-sizes-missing-range",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","row_sizes":[{"type":"pixel","size":20}]}]}`},
			"row_sizes[0] 缺少必填的 range",
		},
		{
			"row-sizes-pixel-without-size",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","row_sizes":[{"range":"1:2","type":"pixel"}]}]}`},
			"type=pixel 时必须提供正整数 size",
		},
		{
			// JSON 数字都是 float64，直接 int(n) 会把 12.9 悄悄写成 12。
			// --styles 不可原子回滚，静默取整会留下与配置不符的表格。
			"cell-styles-fractional-font-size",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","cell_styles":[{"range":"A1","font_size":12.9}]}]}`},
			"font_size=12.9 必须是整数",
		},
		{
			"row-sizes-fractional-size",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","row_sizes":[{"range":"1:1","type":"pixel","size":28.5}]}]}`},
			"size=28.5 必须是整数",
		},
		{
			// 1e20 以前被 int() 截成 math.MaxInt64 后照样下发
			"row-sizes-overflow-size",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","row_sizes":[{"range":"1:1","type":"pixel","size":1e20}]}]}`},
			"超出取值范围",
		},
		{
			"col-sizes-fractional-size",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","col_sizes":[{"range":"A:A","type":"pixel","size":120.5}]}]}`},
			"size=120.5 必须是整数",
		},
		{
			"row-sizes-standard-with-size",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","row_sizes":[{"range":"1:2","type":"standard","size":28}]}]}`},
			"type=standard 表示恢复默认尺寸，不能同时给 size",
		},
		{
			// standard / auto 分支也要先报出数值本身的问题，而不是笼统的
			// "不能同时给 size"：小数是配置写错，提示必须指向 size 才能让调用方
			// 知道该改哪里。
			"row-sizes-standard-with-fractional-size",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","row_sizes":[{"range":"1:2","type":"standard","size":28.5}]}]}`},
			"size=28.5 必须是整数",
		},
		{
			"row-sizes-auto-with-fractional-size",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","row_sizes":[{"range":"1:2","type":"auto","size":28.5}]}]}`},
			"size=28.5 必须是整数",
		},
		{
			"row-sizes-auto-with-size",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","row_sizes":[{"range":"1:2","type":"auto","size":28}]}]}`},
			"type=auto 表示按内容自适应，不能同时给 size",
		},
		{
			"row-sizes-bad-range",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","row_sizes":[{"range":"a:b","type":"standard"}]}]}`},
			"行范围非法",
		},
		{
			// 列宽不提供 auto（与飞书 col_sizes 的 type 枚举一致）
			"col-sizes-auto-rejected",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","col_sizes":[{"range":"A:B","type":"auto"}]}]}`},
			"col_sizes[0].type=\"auto\" 非法（合法值: pixel / standard）",
		},
		{
			"col-sizes-bad-range",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","col_sizes":[{"range":"1:2","type":"standard"}]}]}`},
			"列范围非法",
		},
		{
			"cell-merges-missing-range",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","cell_merges":[{"merge_type":"all"}]}]}`},
			"cell_merges[0] 缺少必填的 range",
		},
		{
			// merge_type 未知值以前会原样透传，直到 merge_cells 才被服务端拒绝，
			// 留下一个已建且已部分修改的文档。必须在建文档之前拦住。
			"cell-merges-invalid-merge-type",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","cell_merges":[{"range":"A1:B1","merge_type":"invalid"}]}]}`},
			`cell_merges[0]: merge_type 非法: "invalid"`,
		},
		{
			// 非法合并区域地址同样只在最后 merge_cells 才被拒，会留下部分完成文档。
			// 必须在建文档之前严格解析拦下（与 cell_styles 的 range 校验一致）。
			"cell-merges-invalid-range",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","cell_merges":[{"range":"not-a-range"}]}]}`},
			"cell_merges[0].range",
		},
		{
			// json.Unmarshal 只认 sheetStyleOps 的 tag：cellStyles 那份样式会被静默
			// 丢弃、同项的 row_sizes 却照常生效，留下一份"命令成功但样式不全"的表格。
			"styles-top-level-camel-case-key",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `[{"name":"S","cellStyles":[{"range":"A1","font_weight":"bold"}],"row_sizes":[{"range":"1:1","type":"pixel","size":28}]}]`},
			`--styles[0]: 字段 "cellStyles" 拼写不符合契约，应为 "cell_styles"`,
		},
		{
			// 键名对但值类型不对：结构化解码必须报错而不是被前一步的 map 解码放过
			"styles-list-value-wrong-type",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `[{"name":"S","cell_styles":"A1"}]`},
			"--styles 解析失败",
		},
		{
			"styles-top-level-unknown-key",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `[{"name":"S","cell_merges":[{"range":"A1:B1"}],"borders":[]}]`},
			`--styles[0]: 未知字段 "borders"（--styles 单项只接受 name / cell_styles / row_sizes / col_sizes / cell_merges）`,
		},
		{
			// 内层键同理：pickStr 认不出的拼写会被丢掉，加粗生效而背景色悄悄丢失。
			// background_color / backgroundColor / bgColor 三种拼写才是被接受的。
			"cell-styles-near-miss-key",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","cell_styles":[{"range":"A1","font_weight":"bold","backgroundcolor":"#FFF"}]}]}`},
			`cell_styles[0]: 字段 "backgroundcolor" 拼写不符合契约，应为 "background_color"`,
		},
		{
			"cell-styles-unknown-key",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","cell_styles":[{"range":"A1","font_weight":"bold","italic":true}]}]}`},
			`cell_styles[0]: 未知字段 "italic"（cell_styles 项只接受 range / background_color /`,
		},
		{
			// border_styles 的边对象只认 style / color：写错的键此前被静默忽略，
			// 会画出一条没有颜色的边框而命令报成功。create 路径同样必须在建文档前拒掉。
			"cell-styles-border-unknown-edge-field",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","cell_styles":[{"range":"A1","border_styles":{"bottom":{"style":"medium","colour":"#f00"}}}]}]}`},
			`--border-styles-json.bottom: 未知字段 "colour"`,
		},
		{
			"cell-styles-border-color-not-string",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","cell_styles":[{"range":"A1","borderStyles":{"top":{"style":"solid","color":123}}}]}]}`},
			"--border-styles-json.top.color 必须是字符串，实际是 float64",
		},
		{
			// camelCase 别名必须照常放行（不能把兼容写法当成拼错）
			"cell-styles-camel-case-alias-accepted",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","cell_styles":[{"range":"A1","backgroundColor":"#FFF","hAlign":"center","borderStyles":{"bottom":{"style":"medium"}},"fontSize":"x"}]}]}`},
			"cell_styles.fontSize 必须是数字", // 只有 fontSize 的类型错误应被报出，其余别名均已接受
		},
		{
			"row-sizes-unknown-key",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","row_sizes":[{"range":"1:1","type":"pixel","size":28,"height":9}]}]}`},
			`row_sizes[0]: 未知字段 "height"（row_sizes 项只接受 range / type / size）`,
		},
		{
			"cell-merges-unknown-key",
			map[string]string{"name": "X", "values": `[[1]]`, "styles": `{"styles":[{"name":"S","cell_merges":[{"range":"A1:B1","merge_style":"all"}]}]}`},
			`cell_merges[0]: 未知字段 "merge_style"`,
		},
		{
			// 嵌套对象/数组会被 cellToString 写成 "map[a:1]" 这类无意义文本，
			// 而回读只校验非空，于是垃圾数据被报成写入成功。
			"values-cell-not-scalar",
			map[string]string{"name": "X", "values": `[[1,{"a":1}]]`},
			"--values[0][1] 必须是字符串/数字/布尔/null",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			err := runCreate(t, caller, tc.flags)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contains %q", err, tc.want)
			}
			if caller.calls != 0 {
				t.Fatalf("calls = %d, want 0 (校验必须早于建文档，否则会留下白建的空表)", caller.calls)
			}
		})
	}
}

// 写入规模上限是服务端硬限（table_put 30000 单元格；set_range_from_csv 30000
// 单元格 / 2M 字符）。超限若不前置拦下，同样是先建好文档再在写入阶段失败。
func TestSheetCreateRejectsOversizedWriteBeforeCreatingDocument(t *testing.T) {
	repeatRow := func(row string, n int) string {
		rows := make([]string, n)
		for i := range rows {
			rows[i] = row
		}
		return "[" + strings.Join(rows, ",") + "]"
	}
	cases := []struct {
		name  string
		flags map[string]string
		want  string
	}{
		{
			// 30000 数据行 + 1 表头行 = 30001 > 30000
			"sheets-cells-over-cap",
			map[string]string{"name": "X", "sheets": `[{"name":"S","columns":["a"],"data":` + repeatRow(`[1]`, 30000) + `}]`},
			"单个工作表写入单元格总数上限为 30000（当前 30001 行 × 1 列 = 30001）",
		},
		{
			// header=false 少写一行，刚好 30000 应放行 —— 见下方 accepted 断言
			"sheets-cells-over-cap-multi-column",
			map[string]string{"name": "X", "sheets": `[{"name":"S","columns":["a","b"],"data":` + repeatRow(`[1,2]`, 15000) + `}]`},
			"当前 15001 行 × 2 列 = 30002",
		},
		{
			"values-cells-over-cap",
			map[string]string{"name": "X", "values": repeatRow(`[1]`, 30001)},
			"--values 单元格总数上限为 30000（当前 30001 行 × 1 列 = 30001）",
		},
		{
			// 字符上限按 rune 计：中文按字符数而不是字节数，避免误拒 2/3 长度的中文表
			"values-chars-over-cap",
			map[string]string{"name": "X", "values": `[["` + strings.Repeat("中", maxCSVWriteChars) + `"]]`},
			"编码为 CSV 后长度为 2000001 字符，超过上限 2000000",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			err := runCreate(t, caller, tc.flags)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contains %q", err, tc.want)
			}
			if caller.calls != 0 {
				t.Fatalf("calls = %d, want 0（超限必须早于建文档）", caller.calls)
			}
		})
	}

	// 恰好等于上限不该被拒（header=false 时不占行）
	atCap := `[{"name":"S","columns":["a"],"header":false,"data":` + repeatRow(`[1]`, maxTableWriteCells) + `}]`
	if _, err := parseCreateSheetSpecs(atCap); err != nil {
		t.Fatalf("恰好 %d 个单元格被误拒: %v", maxTableWriteCells, err)
	}
}

func TestSheetCreateWithValuesWritesAndVerifies(t *testing.T) {
	caller := &scriptedToolCaller{steps: createOKSteps("[row=1]姓名,分数")}
	if err := runCreate(t, caller, map[string]string{"name": "名单", "values": `[["姓名","分数"],["张三",90]]`}); err != nil {
		t.Fatalf("create with values: %v", err)
	}
	// create → 探活 → 定位默认表 → 写 csv → 回读
	if caller.calls != 5 {
		t.Fatalf("calls = %d, want 5", caller.calls)
	}
	if caller.tool != "get_range_as_csv" {
		t.Fatalf("last tool = %q, want get_range_as_csv (回读校验)", caller.tool)
	}
}

func TestSheetCreateWithValuesDryRunNeverCallsRemote(t *testing.T) {
	caller := &scriptedToolCaller{dry: true}
	if err := runCreate(t, caller, map[string]string{"name": "X", "values": `[[1]]`}); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("calls = %d, want 0", caller.calls)
	}
}

func TestSheetCreateWithSheetsRenamesDefaultThenTablePut(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodeId":"NODE_1"}`},
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},
		{text: `{"success":true}`}, // update_sheet 重命名默认表
		{text: `{"success":true}`}, // table_put
		// 回读校验阶段：先取 name→sheetId，再逐表回读
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"一月"},{"sheetId":"SHEET_2","name":"二月"}]}`},
		{text: `{"csv":"[row=1] 项目\n"}`}, // 一月 回读非空
		{text: `{"csv":"[row=1] 项目\n"}`}, // 二月 回读非空
	}}
	if err := runCreate(t, caller, map[string]string{
		"name":   "报表",
		"sheets": `[{"name":"一月","columns":["项目"],"data":[["房租"]]},{"name":"二月","columns":["项目"],"data":[["房租"]]}]`,
	}); err != nil {
		t.Fatalf("create with sheets: %v", err)
	}
	// create → 探活 → 定位默认表 → 重命名 → table_put → 取 name 映射 → 两表各回读一次
	if caller.calls != 8 {
		t.Fatalf("calls = %d, want 8", caller.calls)
	}
}

// table_put 返回成功但某工作表回读为空（新建文档初始化竞态），必须报数据未落盘，
// 而不是静默成功。错误须带 nodeId 与补写指引。
func TestSheetCreateWithSheetsVerifiesEachSheetLanded(t *testing.T) {
	// 二月回读为空 → 报错
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodeId":"NODE_1"}`},
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},
		{text: `{"success":true}`},
		{text: `{"success":true}`},
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"一月"},{"sheetId":"SHEET_2","name":"二月"}]}`},
		{text: `{"csv":"[row=1] 项目\n"}`}, // 一月 非空
		{text: `{"csv":""}`},             // 二月 回读为空
	}}
	err := runCreate(t, caller, map[string]string{
		"name":   "报表",
		"sheets": `[{"name":"一月","columns":["项目"],"data":[["房租"]]},{"name":"二月","columns":["项目"],"data":[["房租"]]}]`,
	})
	if err == nil {
		t.Fatal("二月回读为空应报错，而非静默成功")
	}
	for _, want := range []string{"NODE_1", "二月", "写入未生效", "table-put"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误信息缺少 %q: %v", want, err)
		}
	}

	// table_put 后工作表列表里找不到某个 spec 名 → 同样报错
	missing := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodeId":"NODE_1"}`},
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},
		{text: `{"success":true}`},
		{text: `{"success":true}`},
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"一月"}]}`}, // 缺二月
		{text: `{"csv":"[row=1] 项目\n"}`},                         // 一月 回读非空，随后二月查找失败
	}}
	err = runCreate(t, missing, map[string]string{
		"name":   "报表",
		"sheets": `[{"name":"一月","columns":["项目"],"data":[["房租"]]},{"name":"二月","columns":["项目"],"data":[["房租"]]}]`,
	})
	if err == nil || !strings.Contains(err.Error(), "二月") || !strings.Contains(err.Error(), "NODE_1") {
		t.Fatalf("缺失工作表应带 nodeId 报错: %v", err)
	}

	// header=false 且没给 data 的工作表本就应为空，不回读、不报错
	empty := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodeId":"NODE_1"}`},
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},
		{text: `{"success":true}`},
		{text: `{"success":true}`},
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"有数据"},{"sheetId":"SHEET_2","name":"空表"}]}`},
		{text: `{"csv":"[row=1] a\n"}`}, // 有数据 回读非空；空表不回读
	}}
	if err := runCreate(t, empty, map[string]string{
		"name":   "报表",
		"sheets": `[{"name":"有数据","columns":["a"],"data":[["v"]]},{"name":"空表","columns":["a"],"header":false}]`,
	}); err != nil {
		t.Fatalf("无数据工作表不应触发回读失败: %v", err)
	}
	if empty.calls != 7 {
		t.Fatalf("calls = %d, want 7（空表不回读，只 6 步编排 + 有数据表 1 次回读）", empty.calls)
	}
}

func TestFirstNonEmptySheetSpecCell(t *testing.T) {
	mustDecode := func(s string) map[string]any {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	for _, tc := range []struct {
		spec        string
		wantCell    string
		wantContent bool
	}{
		{`{"name":"S","columns":["项目"],"data":[["房租"]]}`, "A1", true},
		{`{"name":"S","columns":["",""],"data":[["","x"]]}`, "B2", true},            // 首列表头空
		{`{"name":"S","columns":["id"],"data":[[1]],"startCell":"C3"}`, "C3", true}, // startCell（table_put 线上 camelCase）
		// 服务端解析前会 toUpperCase 并允许 $ 绝对引用，探针须同样解释
		{`{"name":"S","columns":["id"],"data":[[1]],"startCell":"$c$3"}`, "C3", true},
		// header=false 时服务端不写表头行，探针必须从 data 首行算起，
		// 否则会盯着一个永远不会被写入的表头位置报"数据未落盘"。
		{`{"name":"S","columns":["id"],"data":[[1]]}`, "A1", true},
		{`{"name":"S","columns":["id"],"data":[[1]],"header":false}`, "A1", true},
		{`{"name":"S","columns":["id"],"data":[["",""],["x"]],"header":false}`, "A2", true},
		{`{"name":"S","columns":["id"],"header":false}`, "", false}, // 不写表头又没有 data
		// mode=append 落在新建的空表上，服务端从第 1 行开始写：startCell 的行号被忽略，
		// 列号仍生效。探针若照搬 startCell 会指到空白处。
		{`{"name":"S","columns":["id"],"data":[[1]],"mode":"append","startCell":"C7"}`, "C1", true},
		{`{"name":"S","columns":[],"data":[]}`, "", false}, // 空
	} {
		gotCell, gotContent := firstNonEmptySheetSpecCell(mustDecode(tc.spec))
		if gotCell != tc.wantCell || gotContent != tc.wantContent {
			t.Errorf("firstNonEmptySheetSpecCell(%s) = (%q,%v), want (%q,%v)", tc.spec, gotCell, gotContent, tc.wantCell, tc.wantContent)
		}
	}
}

// resolveSheetIDsByName 的失败路径：RPC 出错、以及响应无法解析为对象。
func TestResolveSheetIDsByNameFailures(t *testing.T) {
	installSheetProductArgs(t)
	rpcErr := &scriptedToolCaller{steps: []scriptedToolStep{{err: fmt.Errorf("boom")}}}
	installScriptedCaller(t, rpcErr)
	if _, err := resolveSheetIDsByName(context.Background(), "N"); err == nil {
		t.Fatal("RPC 出错时应返回 error")
	}

	bad := &scriptedToolCaller{steps: []scriptedToolStep{{text: `not-json`}}}
	installScriptedCaller(t, bad)
	if _, err := resolveSheetIDsByName(context.Background(), "N"); err == nil ||
		!strings.Contains(err.Error(), "解析工作表列表失败") {
		t.Fatalf("响应不可解析时应报解析失败: %v", err)
	}
}

// create --sheets 时 table_put 后取工作表列表失败，须带 nodeId 报错而非静默成功。
func TestSheetCreateWithSheetsReadbackListFailureSurfacesNodeID(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodeId":"NODE_1"}`},
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},
		{text: `{"success":true}`},
		{text: `{"success":true}`},
		{err: fmt.Errorf("list boom")}, // resolveSheetIDsByName 失败
	}}
	err := runCreate(t, caller, map[string]string{
		"name":   "报表",
		"sheets": `[{"name":"一月","columns":["项目"],"data":[["房租"]]}]`,
	})
	if err == nil || !strings.Contains(err.Error(), "NODE_1") || !strings.Contains(err.Error(), "无法回读校验") {
		t.Fatalf("取列表失败应带 nodeId 报错: %v", err)
	}
}

// 每个远端步骤失败时，错误必须带上已创建的 nodeId，便于在部分成功后继续操作同一文档。
func TestSheetCreateFailuresSurfaceNodeIDForRecovery(t *testing.T) {
	valuesFlags := map[string]string{"name": "X", "values": `[["a"]]`}
	sheetsFlags := map[string]string{"name": "X", "sheets": `[{"name":"S","columns":["a"],"data":[["v"]]}]`}
	okSheets := `{"sheets":[{"sheetId":"SHEET_1"}]}`

	cases := []struct {
		name      string
		flags     map[string]string
		steps     []scriptedToolStep
		want      string
		wantNode  bool
		wantNoErr bool
	}{
		{
			name:  "create-failed",
			flags: valuesFlags,
			steps: []scriptedToolStep{{err: errors.New("boom")}},
			want:  "创建表格失败",
		},
		{
			name:  "create-response-unparseable",
			flags: valuesFlags,
			steps: []scriptedToolStep{{text: `{`}},
			want:  "解析创建结果失败",
		},
		{
			name:  "create-response-without-node-id",
			flags: valuesFlags,
			steps: []scriptedToolStep{{text: `{"ok":true}`}},
			want:  "创建结果未返回 nodeId",
		},
		{
			name:     "probe-never-ready",
			flags:    valuesFlags,
			steps:    []scriptedToolStep{{text: `{"nodeId":"NODE_1"}`}, {err: errors.New("not ready")}},
			want:     "等待文档就绪失败",
			wantNode: true,
		},
		{
			name:     "probe-returns-no-sheets",
			flags:    valuesFlags,
			steps:    []scriptedToolStep{{text: `{"nodeId":"NODE_1"}`}, {text: `{"sheets":[]}`}},
			want:     "未找到任何工作表",
			wantNode: true,
		},
		{
			name:  "write-values-failed",
			flags: valuesFlags,
			steps: []scriptedToolStep{
				{text: `{"nodeId":"NODE_1"}`}, {text: okSheets}, {text: okSheets},
				{err: errors.New("write failed")},
			},
			want:     "但写入数据失败",
			wantNode: true,
		},
		{
			name:  "readback-empty-means-not-persisted",
			flags: valuesFlags,
			steps: []scriptedToolStep{
				{text: `{"nodeId":"NODE_1"}`}, {text: okSheets}, {text: okSheets},
				{text: `{"success":true}`},
				{text: `{"csv":"[row=1] , ,"}`}, // 去掉行号前缀与分隔符后为空
			},
			want:     "初始数据写入未生效",
			wantNode: true,
		},
		{
			name:  "readback-call-failed",
			flags: valuesFlags,
			steps: []scriptedToolStep{
				{text: `{"nodeId":"NODE_1"}`}, {text: okSheets}, {text: okSheets},
				{text: `{"success":true}`},
				{err: errors.New("readback boom")},
			},
			want:     "回读校验失败",
			wantNode: true,
		},
		{
			name:  "rename-default-sheet-failed",
			flags: sheetsFlags,
			steps: []scriptedToolStep{
				{text: `{"nodeId":"NODE_1"}`}, {text: okSheets}, {text: okSheets},
				{err: errors.New("rename failed")},
			},
			want:     "重命名默认工作表失败",
			wantNode: true,
		},
		{
			name:  "table-put-failed",
			flags: sheetsFlags,
			steps: []scriptedToolStep{
				{text: `{"nodeId":"NODE_1"}`}, {text: okSheets}, {text: okSheets},
				{text: `{"success":true}`},
				{err: errors.New("table_put failed")},
			},
			want:     "但写入初始数据失败",
			wantNode: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: tc.steps}
			err := runCreate(t, caller, tc.flags)
			if err == nil {
				t.Fatalf("err = nil, want contains %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contains %q", err, tc.want)
			}
			if tc.wantNode && !strings.Contains(err.Error(), "NODE_1") {
				t.Fatalf("err = %v, want to carry nodeId for recovery", err)
			}
		})
	}
}

func TestSheetCreateStyleApplicationFailureReportsIndex(t *testing.T) {
	okSheets := `{"sheets":[{"sheetId":"SHEET_1"}]}`
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodeId":"NODE_1"}`}, {text: okSheets}, {text: okSheets},
		{text: `{"success":true}`},          // set_range_from_csv
		{text: `{"csv":"[row=1]a"}`},        // 回读通过
		{err: errors.New("style rejected")}, // set_cell_range 失败
	}}
	err := runCreate(t, caller, map[string]string{
		"name":   "X",
		"values": `[["a"]]`,
		"styles": `{"styles":[{"name":"S","cell_styles":[{"range":"A1","font_weight":"bold"}]}]}`,
	})
	if err == nil || !strings.Contains(err.Error(), "--styles[0] 应用失败") {
		t.Fatalf("err = %v, want --styles[0] 应用失败", err)
	}
	// 数据已写入成功，错误信息必须同时说明这一点
	if !strings.Contains(err.Error(), "数据已写入") || !strings.Contains(err.Error(), "NODE_1") {
		t.Fatalf("err = %v, want to state data was written and carry nodeId", err)
	}
}

func TestSheetCreateAppliesStylesInReviewedOrder(t *testing.T) {
	okSheets := `{"sheets":[{"sheetId":"SHEET_1"}]}`
	// 建表(1) 探活(1) 定位(1) 写值(1) 回读(1) + 样式 4 次 = 9
	steps := []scriptedToolStep{
		{text: `{"nodeId":"NODE_1"}`}, {text: okSheets}, {text: okSheets},
		{text: `{"success":true}`}, {text: `{"csv":"[row=1]a"}`},
	}
	for i := 0; i < 4; i++ {
		steps = append(steps, scriptedToolStep{text: `{"success":true}`})
	}
	caller := &scriptedToolCaller{steps: steps}
	if err := runCreate(t, caller, map[string]string{
		"name":   "X",
		"values": `[["a"]]`,
		"styles": `{"styles":[{"name":"S",` +
			`"cell_styles":[{"range":"A1","font_weight":"bold"}],` +
			`"row_sizes":[{"range":"1:1","type":"pixel","size":28}],` +
			`"col_sizes":[{"range":"A:B","type":"standard"}],` +
			`"cell_merges":[{"range":"A1:B1","merge_type":"rows"}]}]}`,
	}); err != nil {
		t.Fatalf("create with full styles: %v", err)
	}
	if caller.calls != 9 {
		t.Fatalf("calls = %d, want 9", caller.calls)
	}
	// 顺序约定：cell_styles → row_sizes → col_sizes → cell_merges，合并最后
	if caller.tool != "merge_cells" {
		t.Fatalf("last tool = %q, want merge_cells", caller.tool)
	}
	if got, _ := caller.args["mergeType"].(string); got != "mergeRows" {
		t.Fatalf("mergeType = %q, want mergeRows (飞书 rows 映射)", got)
	}
}

// ── 纯函数 ───────────────────────────────────────────────────────────────────

func TestPlanStyleOpsTranslatesFeishuFieldsAndOrder(t *testing.T) {
	ops := sheetStyleOps{
		Name: "S",
		CellStyles: []map[string]any{{
			"range":                "A1:B2",
			"background_color":     "#FFF2CC",
			"font_color":           "#000000",
			"font_family":          "微软雅黑",
			"font_style":           "italic",
			"font_weight":          "bold",
			"font_line":            "underline",
			"horizontal_alignment": "center",
			"vertical_alignment":   "middle",
			"word_wrap":            "auto-wrap",
			"number_format":        "@",
			"font_size":            float64(12),
			"border_styles":        map[string]any{"top": map[string]any{"style": "solid"}},
		}},
		RowSizes:   []map[string]any{{"range": "1:3", "type": "auto"}},
		ColSizes:   []map[string]any{{"range": "A:C", "type": "pixel", "size": float64(120)}},
		CellMerges: []map[string]any{{"range": "A1:B1"}},
	}
	calls, err := planStyleOps("N", "S", ops)
	if err != nil {
		t.Fatalf("planStyleOps: %v", err)
	}
	wantTools := []string{"set_cell_range", "update_dimension", "update_dimension", "merge_cells"}
	if len(calls) != len(wantTools) {
		t.Fatalf("calls = %d, want %d", len(calls), len(wantTools))
	}
	for i, want := range wantTools {
		if calls[i].tool != want {
			t.Fatalf("calls[%d].tool = %q, want %q", i, calls[i].tool, want)
		}
	}

	cells, _ := calls[0].args["cells"].([][]any)
	if len(cells) != 2 || len(cells[0]) != 2 {
		t.Fatalf("cells shape = %v, want 2x2", cells)
	}
	cell, _ := cells[0][0].(map[string]any)
	cs, _ := cell["cellStyles"].(map[string]any)
	// word_wrap 的飞书写法 auto-wrap 必须映射为引擎枚举 autoWrap
	if cs["wordWrap"] != "autoWrap" {
		t.Fatalf("wordWrap = %v, want autoWrap", cs["wordWrap"])
	}
	if cs["fontFamily"] != "微软雅黑" || cs["fontSize"] != 12 {
		t.Fatalf("cellStyles = %v", cs)
	}
	// font_line=underline 展开为两个布尔
	if cs["textUnderline"] != true || cs["textLineThrough"] != false {
		t.Fatalf("font-line mapping = %v", cs)
	}
	if _, ok := cell["borderStyles"]; !ok {
		t.Fatal("borderStyles missing")
	}

	// ROWS 支持 auto，无需 pixelSize
	if calls[1].args["dimension"] != "ROWS" || calls[1].args["sizeType"] != "auto" {
		t.Fatalf("row size args = %v", calls[1].args)
	}
	if _, ok := calls[1].args["pixelSize"]; ok {
		t.Fatal("auto 行高不应带 pixelSize")
	}
	if calls[1].args["startIndex"] != "1" || calls[1].args["length"] != 3 {
		t.Fatalf("row range args = %v", calls[1].args)
	}
	if calls[2].args["dimension"] != "COLUMNS" || calls[2].args["pixelSize"] != 120 {
		t.Fatalf("col size args = %v", calls[2].args)
	}
	if calls[2].args["startIndex"] != "A" || calls[2].args["length"] != 3 {
		t.Fatalf("col range args = %v", calls[2].args)
	}
	// merge_type 缺省时按飞书语义落到 mergeAll
	if calls[3].args["mergeType"] != "mergeAll" {
		t.Fatalf("mergeType = %v, want mergeAll", calls[3].args["mergeType"])
	}
}

func TestPlanStyleOpsAcceptsCamelCaseAliases(t *testing.T) {
	calls, err := planStyleOps("N", "S", sheetStyleOps{
		CellStyles: []map[string]any{{
			"range":               "A1",
			"backgroundColor":     "#FFF",
			"fontSize":            12,
			"horizontalAlignment": "left",
			"borderStyles":        map[string]any{"bottom": map[string]any{"style": "medium"}},
		}},
	})
	if err != nil {
		t.Fatalf("camelCase aliases rejected: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
}

func TestSheetStyleOpsIsEmpty(t *testing.T) {
	if !(sheetStyleOps{Name: "S"}).isEmpty() {
		t.Fatal("只有 name 应视为空")
	}
	for _, ops := range []sheetStyleOps{
		{CellStyles: []map[string]any{{}}},
		{RowSizes: []map[string]any{{}}},
		{ColSizes: []map[string]any{{}}},
		{CellMerges: []map[string]any{{}}},
	} {
		if ops.isEmpty() {
			t.Fatalf("%#v 不应视为空", ops)
		}
	}
}

func TestPickStrAndPickNum(t *testing.T) {
	m := map[string]any{"a": "", "b": "hit", "n": float64(3), "i": 4, "nil": nil, "bad": true}
	if got := pickStr(m, "missing", "a", "b"); got != "hit" {
		t.Fatalf("pickStr = %q, want hit（空串应继续找下一个候选键）", got)
	}
	if got := pickStr(m, "nil", "bad"); got != "" {
		t.Fatalf("pickStr = %q, want empty", got)
	}
	for _, tc := range []struct {
		keys    []string
		want    int
		ok      bool
		wantErr string
	}{
		{keys: []string{"n"}, want: 3, ok: true},
		{keys: []string{"i"}, want: 4, ok: true},
		{keys: []string{"missing"}},
		// nil 视为未提供，继续找下一个候选键；bool 命中后按类型错误报出，
		// 而不是当成"没给"静默放过。
		{keys: []string{"nil", "bad"}, ok: true, wantErr: "必须是数字"},
	} {
		got, ok, err := pickNum(m, tc.keys...)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("pickNum(%v) = (%d,%v), want (%d,%v)", tc.keys, got, ok, tc.want, tc.ok)
		}
		if tc.wantErr == "" {
			if err != nil {
				t.Fatalf("pickNum(%v) err = %v, want nil", tc.keys, err)
			}
		} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Fatalf("pickNum(%v) err = %v, want contains %q", tc.keys, err, tc.wantErr)
		}
	}
}

func TestFeishuEnumMappings(t *testing.T) {
	for raw, want := range map[string]string{
		"auto-wrap": "autoWrap", "word-clip": "clip",
		"overflow": "overflow", "autoWrap": "autoWrap", "": "",
	} {
		if got := feishuWordWrap(raw); got != want {
			t.Fatalf("feishuWordWrap(%q) = %q, want %q", raw, got, want)
		}
	}
	// merge_type 的映射与枚举拒绝由 TestSheetCreateMergeTypeAcceptsEveryLegalForm 覆盖。
	// 原先这里断言 "custom" → "custom"，那是把「未知值原样透传」当成期望锁死了，
	// 而透传正是导致「文档已建但 merge_cells 被服务端拒绝」的原因。
}

func TestParseRowColRange(t *testing.T) {
	cases := []struct {
		addr      string
		isRow     bool
		wantStart string
		wantLen   int
		wantErr   bool
	}{
		{"1:3", true, "1", 3, false},
		{"3:1", true, "1", 3, false}, // 反序自动纠正
		{"2", true, "2", 1, false},
		{"Sheet1!1:2", true, "1", 2, false}, // 前缀被剥离
		{" 1 : 2 ", true, "1", 2, false},
		{"A:C", false, "A", 3, false},
		// 反序列范围与行分支一致地自动纠正：起始列取较小的那个，
		// 否则 "C:A" 会静默改 C/D/E 而不是 A/B/C。
		{"C:A", false, "A", 3, false},
		{"AB:Z", false, "Z", 3, false}, // 多字母列也要按列号而非字典序比较
		{"b", false, "B", 1, false},
		{"", true, "", 0, true},
		{"a:b", true, "", 0, true}, // 行范围收到字母
		{"0:2", true, "", 0, true}, // 行号必须 >= 1
		{"1:0", true, "", 0, true},
		{"1:2", false, "", 0, true}, // 列范围收到数字
		// 带尾随字符的行范围必须整体拒绝：Sscanf("%d") 只消费前缀数字，会把
		// "1x"/"2foo" 静默当成第 1/2 行，放过后 update_dimension 改到错误行。
		{"1x:3", true, "", 0, true},
		{"2foo", true, "", 0, true},
		{"1 2:3", true, "", 0, true}, // 内部空格也非法
		// 列范围拒绝带数字/尾随字符：否则 "A5" 会被 parseA1Cell（补 "1" 后）
		// 当成 A 列静默放过。纯多字母列名（如 "AX"）是合法的，不在此列。
		{"A5:C", false, "", 0, true},
		{"A1", false, "", 0, true},
		{":C", false, "", 0, true},      // 空列 token 也要拒绝
		{"AX:C", false, "C", 48, false}, // 多字母列合法：C..AX 共 48 列
	}
	for _, tc := range cases {
		start, length, err := parseRowColRange(tc.addr, tc.isRow)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("parseRowColRange(%q, isRow=%v) 应报错", tc.addr, tc.isRow)
			}
			continue
		}
		if err != nil || start != tc.wantStart || length != tc.wantLen {
			t.Fatalf("parseRowColRange(%q, isRow=%v) = (%q,%d,%v), want (%q,%d,nil)",
				tc.addr, tc.isRow, start, length, err, tc.wantStart, tc.wantLen)
		}
	}
}

func TestValuesToCSVAndCellToString(t *testing.T) {
	got := valuesToCSV([][]any{
		{"名称", "含,逗号", `含"引号`},
		{nil, true, false},
		{float64(90), float64(1.5), int64(7)},
	})
	want := "名称,\"含,逗号\",\"含\"\"引号\"\n,true,false\n90,1.5,7\n"
	if got != want {
		t.Fatalf("valuesToCSV =\n%q\nwant\n%q", got, want)
	}
	if valuesToCSV(nil) != "" {
		t.Fatal("空输入应得空串")
	}
}

func TestWriteValuesToSheetSkipsEmptyPayload(t *testing.T) {
	installSheetProductArgs(t)
	caller := &scriptedToolCaller{}
	installScriptedCaller(t, caller)
	if err := writeValuesToSheet(context.Background(), "N", "S", nil); err != nil {
		t.Fatalf("writeValuesToSheet(nil): %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("calls = %d, want 0（空数据不应发请求）", caller.calls)
	}
	if err := writeValuesToSheet(context.Background(), "N", "S", [][]any{{"a"}}); err != nil {
		t.Fatalf("writeValuesToSheet: %v", err)
	}
	if caller.tool != "set_range_from_csv" {
		t.Fatalf("tool = %q", caller.tool)
	}
	if caller.args["allowOverwrite"] != true || caller.args["startCell"] != "A1" {
		t.Fatalf("args = %v", caller.args)
	}
}

func TestWaitSheetWritableRetriesThenSucceeds(t *testing.T) {
	installSheetProductArgs(t)
	installImmediateTiming(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{err: errors.New("initializing")},
		{err: errors.New("initializing")},
		{text: `{"sheets":[{"sheetId":"S1"}]}`},
	}}
	installScriptedCaller(t, caller)
	if err := waitSheetWritable(context.Background(), "N"); err != nil {
		t.Fatalf("waitSheetWritable: %v", err)
	}
	if caller.calls != 3 {
		t.Fatalf("calls = %d, want 3（前两次失败后重试成功）", caller.calls)
	}
}

func TestVerifyRangeNotEmpty(t *testing.T) {
	installSheetProductArgs(t)
	cases := []struct {
		name    string
		step    scriptedToolStep
		wantErr string
	}{
		{"has-content", scriptedToolStep{text: `{"csv":"[row=1]a,b"}`}, ""},
		{"wrapped-result", scriptedToolStep{text: `{"result":{"csv":"[row=1]a"}}`}, ""},
		{"only-separators", scriptedToolStep{text: `{"csv":"[row=1] , ,\n[row=2],"}`}, "数据未落盘"},
		{"empty-csv", scriptedToolStep{text: `{"csv":""}`}, "数据未落盘"},
		{"unparseable", scriptedToolStep{text: `{`}, "无法解析返回"},
		{"call-error", scriptedToolStep{err: errors.New("boom")}, "回读校验失败"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{tc.step}})
			err := verifyRangeNotEmpty(context.Background(), "N", "S", "A1")
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want contains %q", err, tc.wantErr)
			}
		})
	}
}

func TestResolveFirstSheetIDAndParseCreatedNodeID(t *testing.T) {
	installSheetProductArgs(t)
	for _, tc := range []struct {
		name    string
		step    scriptedToolStep
		wantID  string
		wantErr string
	}{
		{"ok", scriptedToolStep{text: `{"sheets":[{"sheetId":"S1"},{"sheetId":"S2"}]}`}, "S1", ""},
		{"wrapped", scriptedToolStep{text: `{"result":{"sheets":[{"sheetId":"S9"}]}}`}, "S9", ""},
		{"unparseable", scriptedToolStep{text: `{`}, "", "解析工作表列表失败"},
		{"no-sheets", scriptedToolStep{text: `{"sheets":[]}`}, "", "未找到任何工作表"},
		{"missing-id", scriptedToolStep{text: `{"sheets":[{"name":"x"}]}`}, "", "未返回 sheetId"},
		{"call-error", scriptedToolStep{err: errors.New("boom")}, "", "boom"},
	} {
		t.Run("resolveFirstSheetID/"+tc.name, func(t *testing.T) {
			installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{tc.step}})
			got, err := resolveFirstSheetID(context.Background(), "N")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want contains %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.wantID {
				t.Fatalf("= (%q,%v), want (%q,nil)", got, err, tc.wantID)
			}
		})
	}

	for _, tc := range []struct {
		text    string
		want    string
		wantErr bool
	}{
		{`{"nodeId":"N1"}`, "N1", false},
		{`{"result":{"nodeId":"N2"}}`, "N2", false},
		{`{`, "", true},
		{`{"ok":true}`, "", true},
	} {
		got, err := parseCreatedNodeID(tc.text)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("parseCreatedNodeID(%q) 应报错", tc.text)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("parseCreatedNodeID(%q) = (%q,%v)", tc.text, got, err)
		}
	}
}

func TestCallMCPToolSilentSuppressesOutput(t *testing.T) {
	installSheetProductArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"noisy":true}`}}}
	installScriptedCaller(t, caller)
	if err := callMCPToolSilent(context.Background(), "update_sheet", map[string]any{"a": 1}); err != nil {
		t.Fatalf("callMCPToolSilent: %v", err)
	}
	if caller.tool != "update_sheet" {
		t.Fatalf("tool = %q", caller.tool)
	}
	installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("boom")}}})
	if err := callMCPToolSilent(context.Background(), "update_sheet", nil); err == nil {
		t.Fatal("调用失败应返回错误")
	}
}

func TestParseCreateSheetSpecsAcceptedShapes(t *testing.T) {
	for _, raw := range []string{
		`[{"name":"A","columns":["a"]},{"name":"B","columns":["b"]}]`,
		`{"sheets":[{"name":"A","columns":["a"]}]}`,
		`{"name":"A","columns":["a"]}`, // 单个 spec 对象
		// 契约里的每个可选字段都必须被接受（别把校验写成误拒）
		// dtypes/formats 的键按 trim 后的列名查表，因此 " b " 列写 "b" 是对的
		`{"name":"A","columns":["a"," b "],"data":[["x",1],[true,null]],"dtypes":{"a":"object"},` +
			`"formats":{"b":"0.00"},"cellStyles":{"a":{"bold":true}},"mode":"append",` +
			`"header":false,"allowOverwrite":true,"startCell":"b2"}`,
	} {
		specs, err := parseCreateSheetSpecs(raw)
		if err != nil || len(specs) == 0 {
			t.Fatalf("parseCreateSheetSpecs(%s) = (%v,%v)", raw, specs, err)
		}
	}
}

func TestApplyStyleOpsPropagatesPlanError(t *testing.T) {
	installSheetProductArgs(t)
	installScriptedCaller(t, &scriptedToolCaller{})
	err := applyStyleOps(context.Background(), "N", "S", sheetStyleOps{
		CellMerges: []map[string]any{{"merge_type": "all"}}, // 缺 range
	})
	if err == nil || !strings.Contains(err.Error(), "缺少必填的 range") {
		t.Fatalf("err = %v, want plan error", err)
	}
}

// --styles 的 styles 值不是数组时，内层 Unmarshal 必须报错而不是 panic。
func TestSheetCreateStylesInnerValueMustBeArray(t *testing.T) {
	caller := &scriptedToolCaller{}
	err := runCreate(t, caller, map[string]string{
		"name": "X", "values": `[[1]]`, "styles": `{"styles":"notanarray"}`,
	})
	if err == nil || !strings.Contains(err.Error(), "--styles 解析失败") {
		t.Fatalf("err = %v, want inner unmarshal failure", err)
	}
	if caller.calls != 0 {
		t.Fatalf("calls = %d, want 0（校验必须早于建文档）", caller.calls)
	}
}

// row_sizes/col_sizes 省略 type 时按 pixel 处理（仍需 size）。
func TestSheetCreateResizeTypeDefaultsToPixel(t *testing.T) {
	caller := &scriptedToolCaller{}
	err := runCreate(t, caller, map[string]string{
		"name": "X", "values": `[[1]]`,
		"styles": `{"styles":[{"name":"S","row_sizes":[{"range":"1:1"}]}]}`,
	})
	if err == nil || !strings.Contains(err.Error(), "type=pixel 时必须提供正整数 size") {
		t.Fatalf("err = %v, want pixel default requiring size", err)
	}

	ok := &scriptedToolCaller{steps: append(createOKSteps("A"), scriptedToolStep{text: `{"success":true}`})}
	if err := runCreate(t, ok, map[string]string{
		"name": "X", "values": `[[1]]`,
		"styles": `{"styles":[{"name":"S","row_sizes":[{"range":"1:1","size":28}]}]}`,
	}); err != nil {
		t.Fatalf("省略 type + 提供 size 应成功: %v", err)
	}
}

// 定位默认工作表失败（探活已过、随后 get_all_sheets 失败）必须带上 nodeId。
func TestSheetCreateReportsDefaultSheetLookupFailureWithNodeID(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodeId":"NODE_1"}`},                                // create_workspace_sheet
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`}, // waitSheetWritable 探活
		{err: errors.New("boom")},                                    // resolveFirstSheetID 失败
	}}
	err := runCreate(t, caller, map[string]string{"name": "X", "values": `[["a"]]`})
	if err == nil || !strings.Contains(err.Error(), "定位默认工作表失败") ||
		!strings.Contains(err.Error(), "NODE_1") {
		t.Fatalf("err = %v, want lookup failure carrying nodeId", err)
	}
}

// --sheets 搭配 --styles：样式按子表 name 定位（而非默认 sheetId）。
func TestSheetCreateSheetsAppliesStylesBySheetName(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodeId":"NODE_1"}`},                                // create_workspace_sheet
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`}, // 探活
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`}, // resolveFirstSheetID
		{text: `{"success":true}`},                                   // update_sheet 重命名
		{text: `{"success":true}`},                                   // table_put
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"一月"}]}`},     // resolveSheetIDsByName
		{text: `{"csv":"[row=1] 项目\n"}`},                             // 一月 回读非空
		{text: `{"success":true}`},                                   // set_cell_range（样式）
	}}
	if err := runCreate(t, caller, map[string]string{
		"name":   "X",
		"sheets": `[{"name":"一月","columns":["项目"],"data":[["房租"]]}]`,
		"styles": `{"styles":[{"name":"一月","cell_styles":[{"range":"A1","font_weight":"bold"}]}]}`,
	}); err != nil {
		t.Fatalf("--sheets + --styles: %v", err)
	}
	// 样式调用的 sheetId 必须是子表名，不是默认 SHEET_1
	if got := caller.args["sheetId"]; got != "一月" {
		t.Fatalf("样式作用于 sheetId=%v, want 一月", got)
	}
}

// merge_type 的全部合法写法（飞书别名 + 引擎原生值 + 省略）都必须放行，
// 且映射到正确的原生枚举。
func TestSheetCreateMergeTypeAcceptsEveryLegalForm(t *testing.T) {
	for input, want := range map[string]string{
		"":             "mergeAll",
		"all":          "mergeAll",
		"rows":         "mergeRows",
		"columns":      "mergeColumns",
		"mergeAll":     "mergeAll",
		"mergeRows":    "mergeRows",
		"mergeColumns": "mergeColumns",
	} {
		got, err := feishuMergeType(input)
		if err != nil || got != want {
			t.Errorf("feishuMergeType(%q) = (%q,%v), want (%q,nil)", input, got, err, want)
		}
	}
	for _, bad := range []string{"invalid", "MERGEALL", "merge_all", "none"} {
		if _, err := feishuMergeType(bad); err == nil {
			t.Errorf("feishuMergeType(%q) 应报错", bad)
		}
	}
}

// 回读校验必须瞄准输入里第一个非空单元格。此前固定读 A1，导致
// [["","姓名"],[1,"张三"]] 这类首格为空的合法数据被误报成"写入未生效"，
// 还引导调用方去重复补写。
func TestSheetCreateVerifiesFirstNonEmptyCellNotAlwaysA1(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodeId":"NODE_1"}`},
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},
		{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},
		{text: `{"success":true}`},
		{text: `{"csv":"[row=1] 姓名\n"}`}, // 回读 B1 命中"姓名"
	}}
	if err := runCreate(t, caller, map[string]string{
		"name": "X", "values": `[["","姓名"],[1,"张三"]]`,
	}); err != nil {
		t.Fatalf("首格为空的合法数据不应报写入失败: %v", err)
	}
	if got := caller.args["range"]; got != "B1" {
		t.Fatalf("回读用的 range = %v, want B1（输入里第一个非空格）", got)
	}
}

func TestFirstNonEmptyValuesCell(t *testing.T) {
	for _, tc := range []struct {
		values [][]any
		want   string
	}{
		{[][]any{{"姓名"}}, "A1"},
		{[][]any{{"", "姓名"}}, "B1"},
		{[][]any{{"", ""}, {"", "张三"}}, "B2"},
		{[][]any{{nil, nil, 0.0}}, "C1"},
		// 第 27 列应是 AA，验证多字母列号
		{[][]any{append(make([]any, 26), "x")}, "AA1"},
	} {
		if got := firstNonEmptyValuesCell(tc.values); got != tc.want {
			t.Errorf("firstNonEmptyValuesCell(%v) = %q, want %q", tc.values, got, tc.want)
		}
	}
	// 全空时兜底返回 A1（调用方已在更早处拒掉这种输入）
	if got := firstNonEmptyValuesCell([][]any{{"", nil}}); got != "A1" {
		t.Errorf("全空矩阵兜底 = %q, want A1", got)
	}
}

func TestValuesHaveContent(t *testing.T) {
	if valuesHaveContent([][]any{{"", nil}, {""}}) {
		t.Error("全空矩阵应判为无内容")
	}
	for _, v := range [][][]any{
		{{"a"}},
		{{""}, {"", "b"}},
		{{0.0}},
		{{false}},
	} {
		if !valuesHaveContent(v) {
			t.Errorf("%v 应判为有内容", v)
		}
	}
}

func TestPickNumRejectsNonIntegralAndOutOfRange(t *testing.T) {
	for _, tc := range []struct {
		name string
		val  any
		want string
	}{
		{"fractional", 12.9, "必须是整数"},
		{"negative-fractional", -3.5, "必须是整数"},
		{"overflow", 1e20, "超出取值范围"},
		{"underflow", -1e20, "超出取值范围"},
		{"nan", math.NaN(), "不是有限数值"},
		{"inf", math.Inf(1), "不是有限数值"},
		{"string", "12", "必须是数字"},
		{"bool", true, "必须是数字"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := pickNum(map[string]any{"size": tc.val}, "size")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contains %q", err, tc.want)
			}
			// 键存在过就要报 ok=true，调用方才知道"给了但非法"而非"没给"
			if !ok {
				t.Error("ok = false, want true（键存在）")
			}
			if got != 0 {
				t.Errorf("值 = %d, want 0", got)
			}
		})
	}

	// 合法输入照常返回
	for _, tc := range []struct {
		val  any
		want int
	}{
		{float64(12), 12},
		{float64(0), 0},
		{float64(-5), -5},
		{int(28), 28},
		{float64(math.MaxInt32), math.MaxInt32},
	} {
		got, ok, err := pickNum(map[string]any{"size": tc.val}, "size")
		if err != nil || !ok || got != tc.want {
			t.Errorf("pickNum(%v) = (%d,%v,%v), want (%d,true,nil)", tc.val, got, ok, err, tc.want)
		}
	}

	// 键缺失：ok=false 且无错误
	if got, ok, err := pickNum(map[string]any{}, "size"); got != 0 || ok || err != nil {
		t.Errorf("缺键 = (%d,%v,%v), want (0,false,nil)", got, ok, err)
	}
	// 显式 null 视为未提供
	if _, ok, err := pickNum(map[string]any{"size": nil}, "size"); ok || err != nil {
		t.Errorf("null = (ok=%v,err=%v), want (false,nil)", ok, err)
	}
	// 多候选键：取第一个命中的
	if got, _, _ := pickNum(map[string]any{"fontSize": float64(9)}, "font_size", "fontSize"); got != 9 {
		t.Errorf("别名键取值 = %d, want 9", got)
	}
}

// 工作表名唯一性由 parseCreateSheetSpecs 保证；--styles 又强制逐项 name 等于
// --sheets 对应项，所以 --styles 内部不可能出现重名，无需重复校验。
func TestParseCreateSheetSpecsRejectsDuplicateNames(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sheets  string
		wantErr string
	}{
		{
			"two-identical",
			`[{"name":"一月","columns":["a"]},{"name":"一月","columns":["a"]}]`,
			`--sheets[1].name="一月" 与 --sheets[0] 重复`,
		},
		{
			// 非相邻重复也要报出首次出现的下标
			"duplicate-not-adjacent",
			`[{"name":"a","columns":["c"]},{"name":"b","columns":["c"]},{"name":"a","columns":["c"]}]`,
			`--sheets[2].name="a" 与 --sheets[0] 重复`,
		},
		{
			"wrapped-form-also-checked",
			`{"sheets":[{"name":"x","columns":["a"]},{"name":"x","columns":["a"]}]}`,
			`--sheets[1].name="x" 与 --sheets[0] 重复`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseCreateSheetSpecs(tc.sheets); err == nil ||
				!strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want contains %q", err, tc.wantErr)
			}
		})
	}

	// 名称不同照常放行。仅大小写不同不算重复：服务端区分大小写，CLI 不该收紧。
	for _, sheets := range []string{
		`[{"name":"一月","columns":["a"]},{"name":"二月","columns":["a"]}]`,
		`[{"name":"Sheet","columns":["a"]},{"name":"sheet","columns":["a"]}]`,
		`[{"name":"only","columns":["a"]}]`,
	} {
		if _, err := parseCreateSheetSpecs(sheets); err != nil {
			t.Errorf("parseCreateSheetSpecs(%s) 误拒: %v", sheets, err)
		}
	}
}

// 普通 json.Unmarshal 会把 JSON 数字解成 float64，超过 2^53 的整数在写出之前
// 就被舍入（雪花 ID 1234567890123456789 变成 ...768，20 位订单号更糟），而回读
// 只校验单元格非空，这种篡改会被报告成写入成功。两条数据通道都必须保留字面量。
func TestSheetCreatePreservesLargeIntegerLiterals(t *testing.T) {
	const snowflake = "1234567890123456789"
	const order = "12345678901234567890"
	const beyond2p53 = "9007199254740993"

	t.Run("values-to-csv", func(t *testing.T) {
		rec, err := runCreateRecording(t, &scriptedToolCaller{steps: createOKSteps(snowflake)}, map[string]string{
			"name": "X", "values": `[["id"],[` + snowflake + `],[` + order + `],[` + beyond2p53 + `]]`,
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		// 断言 set_range_from_csv 实际收到的载荷，而不是重算一遍解码
		args := rec.argsFor("set_range_from_csv")
		if args == nil {
			t.Fatal("未调用 set_range_from_csv")
		}
		csv, _ := args["csv"].(string)
		for _, want := range []string{snowflake, order, beyond2p53} {
			if !strings.Contains(csv, want) {
				t.Errorf("CSV 未原样保留 %s；实际 = %q", want, csv)
			}
		}
		// 反向守卫：解码若退回 float64，会出现这些被舍入后的值
		for _, bad := range []string{"1234567890123456768", "12345678901234567000", "9007199254740992"} {
			if strings.Contains(csv, bad) {
				t.Errorf("CSV 含被舍入的值 %s，说明解码仍经过 float64；实际 = %q", bad, csv)
			}
		}
	})

	t.Run("sheets-to-table-put", func(t *testing.T) {
		rec, err := runCreateRecording(t, &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"nodeId":"NODE_1"}`},
			{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},
			{text: `{"sheets":[{"sheetId":"SHEET_1","name":"Sheet1"}]}`},
			{text: `{"success":true}`},
			{text: `{"success":true}`},
			{text: `{"sheets":[{"sheetId":"SHEET_1","name":"a"}]}`}, // resolveSheetIDsByName
			{text: `{"csv":"[row=1] id\n"}`},                        // a 回读非空
		}}, map[string]string{
			"name":   "X",
			"sheets": `[{"name":"a","columns":["id"],"data":[[` + snowflake + `],[` + order + `]]}]`,
		})
		if err != nil {
			t.Fatalf("create with sheets: %v", err)
		}
		args := rec.argsFor("table_put")
		if args == nil {
			t.Fatal("未调用 table_put")
		}
		raw, marshalErr := json.Marshal(args)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		for _, want := range []string{snowflake, order} {
			if !strings.Contains(string(raw), want) {
				t.Errorf("table_put 未原样保留 %s；实际 = %s", want, raw)
			}
		}
		for _, bad := range []string{"1234567890123456800", "12345678901234567000"} {
			if strings.Contains(string(raw), bad) {
				t.Errorf("table_put 含被舍入的值 %s；实际 = %s", bad, raw)
			}
		}
	})
}

// json.Number 走 cellToString 时必须原样输出，不能再经浮点中转。
func TestCellToStringKeepsJSONNumberVerbatim(t *testing.T) {
	for _, raw := range []string{
		"1234567890123456789",
		"12345678901234567890",
		"9007199254740993",
		"-9007199254740993",
		"1.5",
		"1e3",
		"0",
	} {
		if got := cellToString(json.Number(raw)); got != raw {
			t.Errorf("cellToString(json.Number(%q)) = %q, want 原样", raw, got)
		}
	}
	// float64 分支保持原行为（其他调用方仍可能传 float64）
	if got := cellToString(float64(12)); got != "12" {
		t.Errorf("float64(12) = %q, want 12", got)
	}
	if got := cellToString(1.5); got != "1.5" {
		t.Errorf("float64(1.5) = %q, want 1.5", got)
	}
}

// callRecorder 包装 scriptedToolCaller 并记下每一次调用。共享的
// scriptedToolCaller 只保留最后一次的 tool/args，无法断言多步编排里中间某一步
// 实际发出的载荷（写入是第 4 次调用、回读是第 5 次）。InitDeps 接受
// edition.ToolCaller 接口，所以这个包装只存在于本文件，不必改动共享 helper：
// 嵌入 *scriptedToolCaller 继承其余接口方法，只覆写 CallTool。
type callRecorder struct {
	*scriptedToolCaller
	calls []recordedCall
}

type recordedCall struct {
	tool string
	args map[string]any
}

func (r *callRecorder) CallTool(ctx context.Context, serverID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	r.calls = append(r.calls, recordedCall{tool: toolName, args: args})
	return r.scriptedToolCaller.CallTool(ctx, serverID, toolName, args)
}

// argsFor 返回指定工具第一次被调用时的参数；未调用过则返回 nil。
func (r *callRecorder) argsFor(tool string) map[string]any {
	for _, c := range r.calls {
		if c.tool == tool {
			return c.args
		}
	}
	return nil
}

// runCreateRecording 与 runCreate 等价，但额外记录全部调用以便断言中间步骤。
func runCreateRecording(t *testing.T, inner *scriptedToolCaller, flags map[string]string) (*callRecorder, error) {
	t.Helper()
	rec := &callRecorder{scriptedToolCaller: inner}
	testseam.Protect(t, &deps)
	InitDeps(rec)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	installImmediateTiming(t)
	installSheetProductArgs(t)
	cmd := newCreateCmdForTest(t)
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	return rec, cmd.RunE(cmd, nil)
}
