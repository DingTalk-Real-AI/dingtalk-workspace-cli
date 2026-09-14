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

package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

const (
	sheetRevisionGetRemoteTool  = "get_sheet_revision"
	sheetChangesetGetRemoteTool = "get_sheet_changeset"
	sheetChangesetMaxSpan       = int64(20)
	sheetChangesetJSONMaxBytes  = 2 * 1024 * 1024
)

var sheetRevisionResult = &contract.ResultSpec{
	Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
	DataSchema: json.RawMessage(`{
		"type":"object",
		"description":"工作簿当前持久化 Delta revision",
		"properties":{
			"success":{"type":"boolean","description":"服务端业务调用是否成功"},
			"logId":{"type":"string","description":"服务端请求追踪 ID，可用于问题排查和反馈"},
			"revision":{"type":"number","description":"当前工作簿 revision；空工作簿为 0"}
		},
		"required":["success","logId","revision"],
		"additionalProperties":true
	}`),
}

var sheetChangesetResult = &contract.ResultSpec{
	Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
	DataSchema: json.RawMessage(`{
		"type":"object",
		"description":"工作簿 revision 区间内连续、按 revision 升序排列的 V2 前向语义 changeset",
		"properties":{
			"success":{"type":"boolean","description":"服务端业务调用是否成功"},
			"logId":{"type":"string","description":"服务端请求追踪 ID，可用于问题排查和反馈"},
			"schemaVersion":{"type":"number","description":"changeset 业务响应的语义版本；当前固定为 2","enum":[2]},
			"changeSemantics":{"type":"string","description":"变更详情只描述当时提交的前向效果，不提供统一 old/current 值；当前固定为 FORWARD_ONLY","enum":["FORWARD_ONLY"]},
			"latestRevision":{"type":"number","description":"请求开始时观测并固定的工作簿最新 revision"},
			"startRevision":{"type":"number","description":"查询基线 revision；结果不包含该 revision"},
			"endRevision":{"type":"number","description":"本次实际查询的结束 revision；结果包含该 revision"},
			"summary":{
				"type":"object",
				"description":"本次区间内语义 change 的完整性和影响范围汇总",
				"properties":{
					"changeCount":{"type":"number","description":"全部 changesets 中 change 对象的总数；STATE_RESET 不计作普通 change"},
					"completeChangeCount":{"type":"number","description":"detailsStatus 为 COMPLETE 的 change 数量"},
					"partialChangeCount":{"type":"number","description":"detailsStatus 为 PARTIAL 的 change 数量"},
					"unsupportedChangeCount":{"type":"number","description":"type 为 UNSUPPORTED_CHANGE 的 change 数量"},
					"containsStateReset":{"type":"boolean","description":"区间内是否包含 STATE_RESET 事件"},
					"containsIncompleteChanges":{"type":"boolean","description":"区间内是否包含 PARTIAL、UNAVAILABLE 或 UNSUPPORTED_CHANGE"},
					"affectedSheets":{
						"type":"array",
						"description":"由 AFFECTED 或 DESTINATION target 去重排序得到的工作表与 A1 范围摘要；SOURCE 不计入",
						"items":{
							"type":"object",
							"properties":{
								"sheetId":{"type":"string","description":"受影响工作表的稳定 ID"},
								"sheetName":{"type":"string","description":"可可靠解析时的工作表显示名称；缺失时不得由 sheetId 猜测"},
								"ranges":{"type":"array","description":"去重排序后的 1-based A1 范围；工作表级变更为空数组","items":{"type":"string","description":"1-based A1 范围；整行或整列可写为 1:3 或 A:C"}}
							},
							"required":["sheetId","ranges"],
							"additionalProperties":true
						}
					}
				},
				"required":["changeCount","completeChangeCount","partialChangeCount","unsupportedChangeCount","containsStateReset","containsIncompleteChanges","affectedSheets"],
				"additionalProperties":true
			},
			"changesets":{
				"type":"array",
				"description":"区间 (startRevision, endRevision] 内按 revision 升序排列的完整事件",
				"items":{
					"type":"object",
					"properties":{
						"revision":{"type":"number","description":"该 changeset 对应的 Delta revision"},
						"createTime":{"type":"string","description":"该 revision 的创建时间"},
						"isSelfEdit":{"type":"boolean","description":"该 Delta 是否由当前请求用户提交；false 不能用于识别具体编辑者"},
						"eventType":{"type":"string","description":"事件类型；EDIT 为普通前向编辑，UNDO 为撤销提交产生的前向效果，STATE_RESET 为全量状态替换点","enum":["EDIT","UNDO","STATE_RESET"]},
						"detailsStatus":{"type":"string","description":"该事件内所有 change 的最差详情完整度；COMPLETE、PARTIAL 或 UNAVAILABLE","enum":["COMPLETE","PARTIAL","UNAVAILABLE"]},
						"reset":{
							"type":"object",
							"description":"STATE_RESET 的状态替换信息；EDIT 和 UNDO 不返回该对象",
							"properties":{
								"type":{"type":"string","description":"状态替换原因类型","enum":["ROLLBACK","OVERWRITE","UPGRADE","TEMPLATE","PRETTIFY","UNKNOWN_RESET"]},
								"targetRevision":{"type":"number","description":"targetStatus 为 KNOWN 时的目标 revision；0 是合法空基线"},
								"targetStatus":{"type":"string","description":"目标 revision 状态；KNOWN 表示已返回，NOT_APPLICABLE 表示该 reset 没有目标，UNAVAILABLE 表示应有目标但无法确认","enum":["KNOWN","NOT_APPLICABLE","UNAVAILABLE"]}
							},
							"required":["type","targetStatus"],
							"additionalProperties":true
						},
						"changes":{
							"type":"array",
							"description":"该 revision 内按提交顺序排列的前向语义 change；STATE_RESET 固定为空数组",
							"items":{
								"type":"object",
								"properties":{
									"type":{"type":"string","description":"Agent 可解释的前向变更类型","enum":["ROWS_INSERTED","ROWS_DELETED","ROWS_UPDATED","COLUMNS_INSERTED","COLUMNS_DELETED","COLUMNS_UPDATED","SHEET_CREATED","SHEET_DELETED","SHEET_UPDATED","CUSTOM_TAB_ADDED","CUSTOM_TAB_DELETED","CUSTOM_TAB_UPDATED","CELLS_INSERTED","CELLS_DELETED","RANGE_PASTED","RANGE_AUTOFILLED","RANGE_CLEARED","RANGE_CONTENT_SET","RANGE_BORDER_SET","RANGE_STYLE_SET","RANGE_TAG_SET","RANGE_SORTED","CELLS_CONTENT_SET","CELLS_STYLE_SET","CELLS_TAG_SET","DIMENSION_GROUP_ADDED","DIMENSION_GROUP_REMOVED","DIMENSION_GROUP_UPDATED","DATA_VALIDATION_SET","DATA_VALIDATION_CLEARED","CELLS_MERGED","CELLS_UNMERGED","NAMED_RANGE_SET","NAMED_RANGE_CLEARED","FEATURE_ADDED","FEATURE_DELETED","FEATURE_UPDATED","WORKBOOK_SETTING_UPDATED","EXTERNAL_REFERENCES_REPLACED","UNSUPPORTED_CHANGE"]},
									"targets":{
										"type":"array",
										"description":"该 change 涉及的工作簿、工作表或范围；源和目的范围用 role 区分",
										"items":{
											"type":"object",
											"properties":{
												"scope":{"type":"string","description":"定位层级","enum":["WORKBOOK","SHEET","RANGE"]},
												"sheetId":{"type":"string","description":"SHEET 或 RANGE target 的稳定工作表 ID"},
												"sheetName":{"type":"string","description":"可可靠解析时的工作表显示名称"},
												"sheetNameSource":{"type":"string","description":"工作表名称来源；AT_CHANGE 为变更时名称，CURRENT_STATE 为当前状态映射，UNKNOWN 为无法确认","enum":["AT_CHANGE","CURRENT_STATE","UNKNOWN"]},
												"a1Range":{"type":"string","description":"RANGE target 的 1-based A1 范围"},
												"role":{"type":"string","description":"target 在变更中的角色；省略时等价于 AFFECTED","enum":["AFFECTED","SOURCE","DESTINATION"]}
											},
											"required":["scope"],
											"additionalProperties":true
										}
									},
									"details":{
										"type":"object",
										"description":"按 change.type 归一后的前向详情；字段集合见 Sheet Skill，未返回的旧值或当前值不得推断",
										"properties":{
											"cell":{
												"type":"object",
												"description":"前向单元格内容；整体清除时为 {cleared:true}",
												"properties":{
													"cleared":{"type":"boolean","description":"true 表示整个单元格内容对象被清除","enum":[true]},
													"value":{"type":"object","description":"具名类型化前向值","properties":{"kind":{"type":"string","description":"值类型","enum":["STRING","NUMBER","BOOLEAN","NULL","MULTI"]},"stringValue":{"type":"string","description":"kind=STRING 时的字符串值"},"numberValue":{"type":"number","description":"kind=NUMBER 时的数值"},"booleanValue":{"type":"boolean","description":"kind=BOOLEAN 时的布尔值"},"values":{"type":"array","description":"kind=MULTI 时的字符串值列表","items":{"type":"string","description":"多选值"}}},"required":["kind"],"additionalProperties":true},
													"formula":{"type":"string","description":"本次修改写入的可读公式文本"},
													"formulaCleared":{"type":"boolean","description":"true 表示本次修改清除了公式","enum":[true]},
													"cellType":{"type":"object","description":"单元格类型；整体清除时为 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个单元格类型对象被清除","enum":[true]},"type":{"type":"string","description":"单元格类型","enum":["general","checkbox","select"]},"options":{"type":"array","description":"select 类型的选项","items":{"type":"object","properties":{"value":{"type":"string","description":"选项值"},"color":{"type":"string","description":"可用时的选项颜色"}},"required":["value"],"additionalProperties":true}}},"additionalProperties":true},
															"link":{"type":"object","description":"工作簿内范围链接；整体清除时为 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个链接对象被清除","enum":[true]},"type":{"type":"string","description":"正常链接类型；当前为 range","enum":["range"]},"sheetId":{"type":"string","description":"正常链接的目标工作表 ID"},"a1Range":{"type":"string","description":"正常链接目标的 1-based A1 范围"},"absolute":{"type":"boolean","description":"正常链接是否使用绝对引用"}},"additionalProperties":true}
												},
												"additionalProperties":true
											},
											"cellType":{"type":"object","description":"相对内容变更的单元格类型；整体清除时为 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个单元格类型对象被清除","enum":[true]},"type":{"type":"string","description":"单元格类型","enum":["general","checkbox","select"]},"options":{"type":"array","description":"select 类型的选项","items":{"type":"object","properties":{"value":{"type":"string","description":"选项值"},"color":{"type":"string","description":"可用时的选项颜色"}},"required":["value"],"additionalProperties":true}}},"additionalProperties":true},
											"tag":{"type":"object","description":"tag 清除标记；非清除的动态 tag 内容不会返回","properties":{"cleared":{"type":"boolean","description":"true 表示 tag 被清除","enum":[true]}},"required":["cleared"],"additionalProperties":false},
											"border":{"type":"object","description":"前向边框；整体清除时为 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个边框对象被清除","enum":[true]},"color":{"description":"边框颜色；已知属性为 null 表示清除该属性"},"style":{"description":"边框线型；已知属性为 null 表示清除该属性"}},"additionalProperties":true},
											"style":{"type":"object","description":"前向样式白名单；整体清除时为 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个样式对象被清除","enum":[true]}},"additionalProperties":true},
											"properties":{"type":"object","description":"行列、工作表或自定义 Tab 的前向属性；整体清除时为 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个属性对象被清除","enum":[true]}},"additionalProperties":true},
											"changes":{"type":"object","description":"工作簿设置的前向字段；整体清除时为 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个设置对象被清除","enum":[true]},"mode":{"type":"string","description":"CALCULATION 的重算模式；default 表示恢复默认模式","enum":["auto","autoNoTable","manual","default"]},"iterate":{"type":["boolean","null"],"description":"CALCULATION 是否启用迭代计算；null 表示清除该设置"},"iterateCount":{"type":["number","null"],"description":"CALCULATION 的最大迭代次数；null 表示清除该设置"},"iterateDelta":{"type":["number","null"],"description":"CALCULATION 的迭代收敛阈值；null 表示清除该设置"},"enableDynamicArray":{"type":["boolean","null"],"description":"CALCULATION 是否启用动态数组；null 表示清除该设置"},"date1904":{"type":["boolean","null"],"description":"CALCULATION 是否使用 1904 日期系统；null 表示清除该设置"},"image":{"type":"object","description":"BACKGROUND 图片设置；整体清除时为 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示背景图片设置被清除","enum":[true]},"opacity":{"type":["number","null"],"description":"背景图片不透明度；null 表示清除该属性"}},"additionalProperties":true}},"additionalProperties":true},
											"fillMode":{"type":"string","description":"RANGE_AUTOFILLED 的填充模式","enum":["copy","series","trend","predict","none"]},
											"copyStyle":{"type":"boolean","description":"RANGE_AUTOFILLED 是否复制样式；当前协议的 COMPLETE change 必有"},
											"styleMode":{"type":"string","description":"CELLS_STYLE_SET 的样式作用模式；coverStyle 未传 mode 时服务端显式返回 cell","enum":["sheet","row","col","cell"]},
											"step":{"type":"object","description":"粘贴或自动填充模式块大小","properties":{"rows":{"type":"number","description":"模式块行数"},"columns":{"type":"number","description":"模式块列数"}},"required":["rows","columns"],"additionalProperties":true},
											"pasteMode":{"type":"string","description":"RANGE_PASTED 的粘贴作用模式","enum":["cell","row","col","sheet"]},
											"isCut":{"type":"boolean","description":"RANGE_PASTED 是否来自剪切"},
											"iterateMode":{"type":"string","description":"RANGE_PASTED 如何重复应用内容模式","enum":["step","flex"]},
											"includedParts":{"type":"array","description":"RANGE_PASTED 实际包含非空数据的变更类别；空切片不计入","items":{"type":"string","description":"实际包含的变更类别","enum":["CONTENT","STYLE","MERGES","CELL_TYPES","CONDITIONAL_FORMATTING","TABLES","PIVOT_TABLES","COMMENTS","REMINDERS","MENTIONS","DATA_VALIDATION","FILTERS","LOCKS","FOLLOWERS","PROTECTION_RANGES","DIMENSION_METADATA","REACTIONS","RANGE_TAGS"]}},
											"contentPattern":{"type":"object","description":"粘贴内容模式的安全化前向表示","properties":{"rows":{"type":"number","description":"模式块行数"},"columns":{"type":"number","description":"模式块列数"},"cells":{"type":"array","description":"相对模式块的单元格内容","items":{"type":"object","properties":{"rowOffset":{"type":"number","description":"相对模式块左上角的 0-based 行偏移"},"columnOffset":{"type":"number","description":"相对模式块左上角的 0-based 列偏移"},"cleared":{"type":"boolean","description":"true 表示整个模式单元格被清除","enum":[true]},"value":{"type":"object","description":"具名类型化前向值","properties":{"kind":{"type":"string","description":"值类型","enum":["STRING","NUMBER","BOOLEAN","NULL","MULTI"]},"stringValue":{"type":"string","description":"kind=STRING 时的字符串值"},"numberValue":{"type":"number","description":"kind=NUMBER 时的数值"},"booleanValue":{"type":"boolean","description":"kind=BOOLEAN 时的布尔值"},"values":{"type":"array","description":"kind=MULTI 时的字符串值列表","items":{"type":"string","description":"多选值"}}},"required":["kind"],"additionalProperties":true},"formula":{"type":"string","description":"本次修改写入的可读公式文本"},"formulaCleared":{"type":"boolean","description":"true 表示本次修改清除了公式","enum":[true]},"cellType":{"type":"object","description":"前向单元格类型或 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个单元格类型对象被清除","enum":[true]},"type":{"type":"string","description":"单元格类型","enum":["general","checkbox","select"]}},"additionalProperties":true},"link":{"type":"object","description":"工作簿内范围链接或 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个链接对象被清除","enum":[true]},"type":{"type":"string","description":"正常链接类型；当前为 range","enum":["range"]},"sheetId":{"type":"string","description":"正常链接的目标工作表 ID"},"a1Range":{"type":"string","description":"正常链接目标的 1-based A1 范围"},"absolute":{"type":"boolean","description":"正常链接是否使用绝对引用"}},"additionalProperties":true}},"required":["rowOffset","columnOffset"],"additionalProperties":true}}},"required":["cells"],"additionalProperties":true},
											"clearParts":{"type":"array","description":"RANGE_CLEARED 实际清除的内容类别","items":{"type":"string","description":"被清除的内容类别","enum":["VALUES","FORMULAS","MERGES","STYLES","CELL_TYPES","CONDITIONAL_FORMATTING","DATA_VALIDATION","DATA_VALIDATION_LIST","COLUMN_TYPES","LINKS","TABLES","COMMENTS","REMINDERS","REACTIONS"]}},
											"preservedCellTypes":{"type":"array","description":"清除 CELL_TYPES 时明确保留的类型；空数组表示不保留 select 或 checkbox","items":{"type":"string","description":"保留的单元格类型","enum":["SELECT","CHECKBOX"]}},
											"relativeChanges":{"type":"array","description":"相对 target 定位的前向行列属性、内容、样式或 tag 变更；sheet 级项可没有坐标","items":{"type":"object","properties":{"offset":{"type":"number","description":"ROWS_UPDATED 或 COLUMNS_UPDATED 中相对行列区间起点的 0-based 偏移"},"properties":{"type":"object","description":"offset 对应的前向行列属性或 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个行列属性对象被清除","enum":[true]},"size":{"type":["number","null"],"description":"行高或列宽；null 表示清除该属性"},"customSize":{"type":["boolean","null"],"description":"是否使用自定义尺寸；null 表示清除该属性"},"hidden":{"type":["boolean","null"],"description":"是否隐藏；null 表示清除该属性"},"sticky":{"type":["boolean","null"],"description":"是否冻结；null 表示清除该属性"},"cellType":{"type":"object","description":"行列默认单元格类型或 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个单元格类型对象被清除","enum":[true]},"type":{"type":"string","description":"单元格类型","enum":["general","checkbox","select"]},"options":{"type":"array","description":"select 类型的选项","items":{"type":"object","properties":{"value":{"type":"string","description":"选项值"},"color":{"type":"string","description":"可用时的选项颜色"}},"required":["value"],"additionalProperties":true}}},"additionalProperties":true}},"additionalProperties":true},"rowOffset":{"type":"number","description":"0-based 行偏移；行级或单元格级项使用"},"columnOffset":{"type":"number","description":"0-based 列偏移；列级或单元格级项使用"},"cell":{"type":"object","description":"前向单元格内容或 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个单元格内容对象被清除","enum":[true]},"value":{"type":"object","description":"具名类型化前向值","properties":{"kind":{"type":"string","description":"值类型","enum":["STRING","NUMBER","BOOLEAN","NULL","MULTI"]},"stringValue":{"type":"string","description":"kind=STRING 时的字符串值"},"numberValue":{"type":"number","description":"kind=NUMBER 时的数值"},"booleanValue":{"type":"boolean","description":"kind=BOOLEAN 时的布尔值"},"values":{"type":"array","description":"kind=MULTI 时的字符串值列表","items":{"type":"string","description":"多选值"}}},"required":["kind"],"additionalProperties":true},"formula":{"type":"string","description":"本次修改写入的可读公式文本"},"formulaCleared":{"type":"boolean","description":"true 表示本次修改清除了公式","enum":[true]},"cellType":{"type":"object","description":"前向单元格类型或 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个单元格类型对象被清除","enum":[true]},"type":{"type":"string","description":"单元格类型","enum":["general","checkbox","select"]}},"additionalProperties":true},"link":{"type":"object","description":"工作簿内范围链接或 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个链接对象被清除","enum":[true]},"type":{"type":"string","description":"正常链接类型；当前为 range","enum":["range"]},"sheetId":{"type":"string","description":"正常链接的目标工作表 ID"},"a1Range":{"type":"string","description":"正常链接目标的 1-based A1 范围"},"absolute":{"type":"boolean","description":"正常链接是否使用绝对引用"}},"additionalProperties":true}},"additionalProperties":true},"cellType":{"type":"object","description":"前向单元格类型或 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个单元格类型对象被清除","enum":[true]}},"additionalProperties":true},"styleId":{"type":"string","description":"引用同一 change 的 styles[] 条目的 ID"},"styleCleared":{"type":"boolean","description":"true 表示该相对位置的样式被清除","enum":[true]},"tag":{"type":"object","description":"tag 清除标记；非清除的动态 tag 内容不会返回","properties":{"cleared":{"type":"boolean","description":"true 表示 tag 被清除","enum":[true]}},"required":["cleared"],"additionalProperties":false}},"additionalProperties":true}},
											"styles":{"type":"array","description":"同一 change 内可由 relativeChanges.styleId 引用的具名样式条目","items":{"type":"object","properties":{"styleId":{"type":"string","description":"样式引用 ID"},"style":{"type":"object","description":"样式白名单对象；整体清除时为 {cleared:true}","properties":{"cleared":{"type":"boolean","description":"true 表示整个样式对象被清除","enum":[true]}},"additionalProperties":true}},"required":["styleId","style"],"additionalProperties":true}},
											"dataValidation":{
												"type":"object",
												"description":"DATA_VALIDATION_SET 的归一化验证规则；敏感或无法安全解释的内部字段不会返回",
												"properties":{
													"type":{"type":"string","description":"数据验证规则类型"},
													"templateId":{"type":"number","description":"列表验证使用的模板 ID；存在时不代表已返回内联选项或来源区域"},
													"sourceType":{"type":"string","description":"下拉选项来源；inline 为内联选项，sourceRange 为区域引用","enum":["inline","sourceRange"]},
													"options":{"type":"array","description":"安全化后的下拉选项","items":{"type":"object","properties":{"value":{"type":"string","description":"选项值"},"color":{"type":"string","description":"可用时的选项颜色"}},"required":["value"],"additionalProperties":true}},
													"sourceRange":{"type":"object","description":"解析成功时的下拉来源区域","properties":{"sheetId":{"type":"string","description":"来源工作表 ID"},"sheetName":{"type":"string","description":"可用时的来源工作表名称"},"a1Notation":{"type":"string","description":"来源区域的 1-based A1 表示"}},"additionalProperties":true},
													"sourceRangeStatus":{"type":"string","description":"来源区域解析状态","enum":["RESOLVED","UNRESOLVED","INVALID"]},
													"sourceRangeExpression":{"type":"string","description":"安全来源区域表达式；解析为 RESOLVED 时也可能保留原表达式"},
													"enableMultiSelect":{"type":"boolean","description":"下拉是否允许多选"},
																	"criteria":{"type":"object","description":"非下拉规则可安全公开的具名条件参数","properties":{"operator":{"type":"string","description":"条件运算符"},"value1":{"type":"object","description":"第一个具名类型化条件值","properties":{"kind":{"type":"string","description":"值类型","enum":["STRING","NUMBER","BOOLEAN","NULL","FORMULA"]},"stringValue":{"type":"string","description":"kind=STRING 时的字符串值"},"numberValue":{"type":"number","description":"kind=NUMBER 时的数值"},"booleanValue":{"type":"boolean","description":"kind=BOOLEAN 时的布尔值"},"formula":{"type":"string","description":"kind=FORMULA 时的公式文本"}},"required":["kind"],"additionalProperties":true},"value2":{"type":"object","description":"第二个具名类型化条件值","properties":{"kind":{"type":"string","description":"值类型","enum":["STRING","NUMBER","BOOLEAN","NULL","FORMULA"]},"stringValue":{"type":"string","description":"kind=STRING 时的字符串值"},"numberValue":{"type":"number","description":"kind=NUMBER 时的数值"},"booleanValue":{"type":"boolean","description":"kind=BOOLEAN 时的布尔值"},"formula":{"type":"string","description":"kind=FORMULA 时的公式文本"}},"required":["kind"],"additionalProperties":true},"formula":{"type":"string","description":"条件公式"}},"additionalProperties":true},
													"settings":{"type":"object","description":"可安全公开的数据验证通用行为设置；已知属性为 null 表示清除该属性","properties":{"allowBlank":{"type":["boolean","null"],"description":"是否允许空值；null 表示清除该设置"},"errorStyle":{"type":["string","null"],"description":"验证失败时的错误样式；null 表示清除该设置"},"showInputMessage":{"type":["boolean","null"],"description":"是否显示输入提示；null 表示清除该设置"},"showErrorMessage":{"type":["boolean","null"],"description":"是否显示错误提示；null 表示清除该设置"},"prompt":{"type":["string","null"],"description":"输入提示文本；null 表示清除该设置"},"error":{"type":["string","null"],"description":"验证失败提示文本；null 表示清除该设置"},"requiredInRow":{"type":["boolean","null"],"description":"是否要求行内必填；null 表示清除该设置"},"showDropDown":{"type":["boolean","null"],"description":"是否显示下拉控件；null 表示清除该设置"},"columnType":{"type":["boolean","null"],"description":"是否启用列类型行为；null 表示清除该设置"},"promptTitle":{"type":["string","null"],"description":"输入提示标题；null 表示清除该设置"},"errorTitle":{"type":["string","null"],"description":"验证失败提示标题；null 表示清除该设置"},"imeMode":{"type":["string","null"],"description":"输入法模式；null 表示清除该设置"}},"additionalProperties":true}
												},
												"required":["type"],
												"additionalProperties":true
											}
										},
										"additionalProperties":true
									},
									"detailsStatus":{"type":"string","description":"details 的完整度；COMPLETE 仍只代表前向提交详情，不代表 old/current 状态完整","enum":["COMPLETE","PARTIAL","UNAVAILABLE"]},
									"omissions":{
										"type":"array",
										"description":"PARTIAL 或 UNAVAILABLE 时的稳定省略原因；COMPLETE 时固定为空数组",
										"items":{"type":"object","properties":{"code":{"type":"string","description":"稳定的省略原因码","enum":["MISSING_REQUIRED_FIELD","PLUGIN_OPERATION","UNSUPPORTED_WRAPPER","UNSUPPORTED_PROTOCOL_VERSION","UNKNOWN_ACTION","UNKNOWN_VARIANT","DETAILS_NOT_FULLY_INTERPRETED","INVALID_TARGET","VALUE_NOT_AGENT_READABLE","FORMULA_TEXT_UNAVAILABLE","SOURCE_RANGE_UNRESOLVED","SOURCE_RANGE_INVALID","SENSITIVE_DETAILS_OMITTED"]},"fields":{"type":"array","description":"受该原因影响的 details 字段路径","items":{"type":"string","description":"被省略或不完整的字段路径"}}},"required":["code"],"additionalProperties":true}
									}
								},
								"required":["type","targets","details","detailsStatus","omissions"],
								"additionalProperties":true
							}
						}
					},
					"required":["revision","createTime","isSelfEdit","eventType","detailsStatus","changes"],
					"additionalProperties":true
				}
			}
		},
		"required":["success","logId","schemaVersion","changeSemantics","latestRevision","startRevision","endRevision","summary","changesets"],
		"additionalProperties":true
	}`),
}

type sheetPublishedResultSchemaCache struct {
	once   sync.Once
	schema map[string]any
	err    error
}

var (
	sheetRevisionPublishedSchemaCache  sheetPublishedResultSchemaCache
	sheetChangesetPublishedSchemaCache sheetPublishedResultSchemaCache
)

func newSheetRevisionCmds() []*cobra.Command {
	return []*cobra.Command{newSheetRevisionGetCmd(), newSheetChangesetGetCmd()}
}

func newSheetRevisionGetCmd() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "revision-get",
		Short:         "获取表格工作簿当前 revision",
		Long:          "获取在线电子表格工作簿当前持久化 revision。该能力是工作簿级的，不接收 --sheet-id；--node 可传文档 ID 或完整 URL。服务端业务 JSON 原样放入统一输出的 data。",
		Example:       "  dws sheet revision-get --node <NODE_ID_OR_URL> --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        "sheet",
		Tool:          sheetRevisionGetRemoteTool,
		Flags: []LeafFlag{
			{Name: "node", Usage: "表格文档 ID 或 URL (必填)", Bind: "nodeId", Required: true, Trim: true, Example: "https://alidocs.dingtalk.com/i/nodes/xxx"},
		},
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           sheetRevisionGetRemoteTool,
				CanonicalPath:  "sheet." + sheetRevisionGetRemoteTool,
				CLIPath:        "sheet revision-get",
				PrimaryCLIPath: "sheet revision-get",
			},
			Description: "获取在线电子表格工作簿当前持久化 revision；结果为后续 changeset 区间查询的 revision 锚点。",
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Result:      sheetRevisionResult,
			Interface: &contract.InterfaceSpec{
				Mode:         contract.InterfaceModeMCP,
				Availability: contract.InterfaceAvailable,
				Ref:          &contract.InterfaceRefSpec{ProductID: "sheet", RPCName: sheetRevisionGetRemoteTool},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取在线电子表格工作簿当前 Delta revision，作为 changeset 查询锚点。",
				UseWhen:      []string{"需要知道当前 revision，或准备按 revision 区间复核工作簿编辑时"},
				AvoidWhen:    []string{"要查看可命名/可回滚的历史快照时用 sheet version list；要读当前单元格值时用 csv-get、table-get 或 range read"},
				Examples:     []string{"dws sheet revision-get --node <NODE_ID_OR_URL> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId", Description: "在线电子表格文档 ID 或完整 URL"},
			},
		},
		ResultCall: callSheetRevisionResult,
	})
}

func newSheetChangesetGetCmd() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:   "changeset-get",
		Short: "获取表格工作簿 revision 区间内的 changeset",
		Long: `获取在线电子表格工作簿在 (startRevision, endRevision] 区间内的连续、Agent 可解释的前向 changeset。

该能力是工作簿级的，不接收 --sheet-id。--end-revision 省略时由服务端在请求开始时固定为 latestRevision；单次区间最多 20 个 revision。统一输出 data.changesets 始终是可直接遍历的 JSON 数组；CLI 只解码服务端传输格式，不改写语义化 change。changeset 只描述当时提交的前向变更，不提供统一 old/current 值；确认最终状态必须另行回读。`,
		Example: `  dws sheet changeset-get --node <NODE_ID_OR_URL> --start-revision 120 --end-revision 121 --format json
  dws sheet changeset-get --node <NODE_ID_OR_URL> --start-revision 120 --format json`,
		OutputRollout: output.RolloutUnifiedActive,
		Server:        "sheet",
		Tool:          sheetChangesetGetRemoteTool,
		Flags: []LeafFlag{
			{Name: "node", Usage: "表格文档 ID 或 URL (必填)", Bind: "nodeId", Required: true, Trim: true, Example: "https://alidocs.dingtalk.com/i/nodes/xxx"},
			{
				Name: "start-revision", Usage: "查询基线 revision (必填，非负；结果不包含该 revision)", Bind: "startRevision",
				Required: true, Trim: true, Example: "120", Transform: sheetRevisionNumberArg,
			},
			{
				Name: "end-revision", Usage: "查询结束 revision (可选，非负；结果包含该 revision)", Bind: "endRevision",
				Trim: true, OmitEmpty: true, Example: "121", Transform: sheetRevisionNumberArg,
			},
		},
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           sheetChangesetGetRemoteTool,
				CanonicalPath:  "sheet." + sheetChangesetGetRemoteTool,
				CLIPath:        "sheet changeset-get",
				PrimaryCLIPath: "sheet changeset-get",
			},
			Description: "获取在线电子表格工作簿 revision 区间内连续、语义化的前向 changeset；区间语义为 (startRevision, endRevision]。",
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Result:      sheetChangesetResult,
			Interface: &contract.InterfaceSpec{
				Mode:         contract.InterfaceModeMCP,
				Availability: contract.InterfaceAvailable,
				Ref:          &contract.InterfaceRefSpec{ProductID: "sheet", RPCName: sheetChangesetGetRemoteTool},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "读取工作簿两个 revision 之间的语义化前向 changeset，区分 EDIT、UNDO 与 STATE_RESET。",
				UseWhen:      []string{"已知起始 revision，需要复核之后发生了哪些工作簿级编辑或回滚时"},
				AvoidWhen:    []string{"要读取单元格当前最终值时用 csv-get、table-get 或 range read；要查看或回滚命名历史快照时用 sheet version list/revert"},
				Examples:     []string{"dws sheet changeset-get --node <NODE_ID_OR_URL> --start-revision 120 --end-revision 121 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId", Description: "在线电子表格文档 ID 或完整 URL"},
				{Name: "start-revision", Property: "startRevision", InterfaceType: "number", Description: "非负查询基线；返回区间不包含该 revision"},
				{Name: "end-revision", Property: "endRevision", InterfaceType: "number", Description: "可选非负结束 revision；返回区间包含该 revision"},
			},
		},
		Validate:   validateSheetChangesetRange,
		ResultCall: callSheetRevisionResult,
	})
}

func callSheetRevisionResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	if deps.Caller.DryRun() {
		return output.Success(map[string]any{
			"executed":  false,
			"tool":      tool,
			"arguments": args,
		}, output.WithDryRun()), nil
	}
	raw, err := callMCPToolReturnTextOnServer(cmd.Context(), "sheet", tool, args)
	if err != nil {
		return nil, err
	}
	data, err := decodeSheetRevisionResult(tool, args, raw)
	if err != nil {
		return nil, err
	}
	return output.Success(data), nil
}

func decodeSheetRevisionResult(tool string, request map[string]any, raw string) (any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, invalidSheetRevisionResponse(tool,
			"MCP sheet read tool returned no non-empty text content",
			"empty_tool_response",
			true,
		)
	}
	data, err := decodeSheetSingleJSON(raw)
	if err != nil {
		return nil, invalidSheetRevisionResponse(tool,
			fmt.Sprintf("MCP sheet read tool returned invalid JSON: %v", err),
			"invalid_tool_response",
			false,
		)
	}
	object, ok := data.(map[string]any)
	if !ok {
		return nil, invalidSheetRevisionResponse(tool,
			"MCP sheet read tool returned a non-object business result",
			"invalid_tool_response",
			false,
		)
	}
	if isBusinessError(object) {
		return nil, &CLIError{
			Code:       CodeMCPToolError,
			Message:    raw,
			Suggestion: suggestForBusinessError(object),
			Operation:  "sheet/" + tool,
		}
	}
	success, ok := object["success"].(bool)
	if !ok || !success {
		return nil, invalidSheetRevisionResponse(tool,
			"MCP sheet read tool returned a business result without success=true",
			"invalid_tool_response",
			false,
		)
	}
	if tool == sheetChangesetGetRemoteTool {
		if err := normalizeSheetChangesetTransport(object); err != nil {
			return nil, invalidSheetRevisionResponse(tool,
				fmt.Sprintf("MCP sheet read tool returned invalid changeset data: %v%s",
					err, sheetResultLogIDSuffix(object)),
				"invalid_tool_response",
				false,
			)
		}
	}
	if err := validateSheetPublishedResult(tool, object); err != nil {
		return nil, invalidSheetRevisionResponse(tool,
			fmt.Sprintf("MCP sheet read tool returned data that does not match its published result contract: %v%s",
				err, sheetResultLogIDSuffix(object)),
			"invalid_tool_response",
			false,
		)
	}
	if tool == sheetChangesetGetRemoteTool {
		if err := validateSheetChangesetAudit(request, object); err != nil {
			return nil, invalidSheetRevisionResponse(tool,
				fmt.Sprintf("MCP sheet read tool returned changeset data that does not match the requested interval or its summary: %v%s",
					err, sheetResultLogIDSuffix(object)),
				"invalid_tool_response",
				false,
			)
		}
	}
	return object, nil
}

func invalidSheetRevisionResponse(tool, message, reason string, retryable bool) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation("sheet/"+tool),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(retryable),
		apperrors.WithReason(reason),
	)
}

func sheetResultLogIDSuffix(object map[string]any) string {
	logID, ok := object["logId"].(string)
	if !ok || strings.TrimSpace(logID) == "" {
		return ""
	}
	return fmt.Sprintf(" (logId=%s)", strings.TrimSpace(logID))
}

func decodeSheetSingleJSON(raw string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var data any
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("包含多个 JSON 值")
		}
		return nil, err
	}
	return data, nil
}

func normalizeSheetChangesetTransport(object map[string]any) error {
	encoded, hasEncoded := object["changesetsJson"]
	if hasEncoded {
		changesetsJSON, ok := encoded.(string)
		if !ok {
			return fmt.Errorf("changesetsJson 不是字符串")
		}
		if strings.TrimSpace(changesetsJSON) == "" {
			return fmt.Errorf("changesetsJson 为空")
		}
		if len(changesetsJSON) > sheetChangesetJSONMaxBytes {
			return fmt.Errorf("changesetsJson 超过 %d 字节", sheetChangesetJSONMaxBytes)
		}
		decoded, err := decodeSheetSingleJSON(changesetsJSON)
		if err != nil {
			return fmt.Errorf("changesetsJson 不是完整 JSON: %v", err)
		}
		changesets, ok := decoded.([]any)
		if !ok {
			return fmt.Errorf("changesetsJson 根节点不是数组")
		}
		object["changesets"] = changesets
		delete(object, "changesetsJson")
		return nil
	}

	if legacy, exists := object["changesets"]; exists {
		if _, ok := legacy.([]any); !ok {
			return fmt.Errorf("changesets 不是数组")
		}
		return nil
	}

	return fmt.Errorf("成功响应缺少 changesetsJson")
}

func validateSheetPublishedResult(tool string, object map[string]any) error {
	var rawSchema json.RawMessage
	var cache *sheetPublishedResultSchemaCache
	switch tool {
	case sheetRevisionGetRemoteTool:
		rawSchema = sheetRevisionResult.DataSchema
		cache = &sheetRevisionPublishedSchemaCache
	case sheetChangesetGetRemoteTool:
		rawSchema = sheetChangesetResult.DataSchema
		cache = &sheetChangesetPublishedSchemaCache
	default:
		return fmt.Errorf("未知工具 %q", tool)
	}
	return validateSheetPublishedResultWithSchema(tool, object, rawSchema, cache)
}

func validateSheetPublishedResultWithSchema(tool string, object map[string]any,
	rawSchema json.RawMessage, cache *sheetPublishedResultSchemaCache,
) error {
	cache.once.Do(func() { cache.err = json.Unmarshal(rawSchema, &cache.schema) })
	if cache.err != nil {
		return fmt.Errorf("读取已发布 Result Schema 失败: %v", cache.err)
	}
	schema := cache.schema
	if err := cli.ValidateJSONSchemaValue(object, schema); err != nil {
		return err
	}
	if strings.TrimSpace(object["logId"].(string)) == "" {
		return fmt.Errorf("$.logId 不能为空")
	}

	switch tool {
	case sheetRevisionGetRemoteTool:
		if _, err := sheetNonNegativeInteger(object["revision"]); err != nil {
			return fmt.Errorf("$.revision %v", err)
		}
	case sheetChangesetGetRemoteTool:
		latest, err := sheetNonNegativeInteger(object["latestRevision"])
		if err != nil {
			return fmt.Errorf("$.latestRevision %v", err)
		}
		start, err := sheetNonNegativeInteger(object["startRevision"])
		if err != nil {
			return fmt.Errorf("$.startRevision %v", err)
		}
		end, err := sheetNonNegativeInteger(object["endRevision"])
		if err != nil {
			return fmt.Errorf("$.endRevision %v", err)
		}
		if start > end {
			return fmt.Errorf("$.startRevision 不能大于 $.endRevision")
		}
		if end > latest {
			return fmt.Errorf("$.endRevision 不能大于 $.latestRevision")
		}
	}
	return nil
}

type sheetChangesetSummaryAudit struct {
	changeCount               int64
	completeChangeCount       int64
	partialChangeCount        int64
	unsupportedChangeCount    int64
	containsStateReset        bool
	containsIncompleteChanges bool
	affectedSheets            []sheetChangesetAffectedSheetAudit
}

type sheetChangesetAffectedSheetAudit struct {
	sheetID   string
	sheetName string
	ranges    []string
}

type sheetChangesetAffectedSheetAccumulator struct {
	sheetName string
	ranges    map[string]struct{}
}

func validateSheetChangesetAudit(request, object map[string]any) error {
	requestStart := request["startRevision"].(int64)
	responseStart, _ := sheetNonNegativeInteger(object["startRevision"])
	responseEnd, _ := sheetNonNegativeInteger(object["endRevision"])
	latest, _ := sheetNonNegativeInteger(object["latestRevision"])
	if responseStart != requestStart {
		return fmt.Errorf("$.startRevision=%d，与请求值 %d 不一致", responseStart, requestStart)
	}
	if requestEnd, exists := request["endRevision"]; exists {
		if responseEnd != requestEnd.(int64) {
			return fmt.Errorf("$.endRevision=%d，与请求值 %d 不一致", responseEnd, requestEnd)
		}
	} else if responseEnd != latest {
		return fmt.Errorf("省略 endRevision 时 $.endRevision=%d，必须等于 $.latestRevision=%d", responseEnd, latest)
	}
	if responseEnd-responseStart > sheetChangesetMaxSpan {
		return fmt.Errorf("响应区间超过 %d 个 revision", sheetChangesetMaxSpan)
	}

	changesets := object["changesets"].([]any)
	wantCount := responseEnd - responseStart
	if int64(len(changesets)) != wantCount {
		return fmt.Errorf("$.changesets 数量为 %d，无法完整覆盖 (%d,%d]", len(changesets), responseStart, responseEnd)
	}
	for index, rawChangeset := range changesets {
		changeset := rawChangeset.(map[string]any)
		revision, err := sheetNonNegativeInteger(changeset["revision"])
		if err != nil {
			return fmt.Errorf("$.changesets[%d].revision %v", index, err)
		}
		expected := responseStart + int64(index) + 1
		if revision != expected {
			return fmt.Errorf("$.changesets[%d].revision=%d，期望连续 revision %d", index, revision, expected)
		}
	}

	actualSummary, err := parseSheetChangesetSummaryAudit(object["summary"].(map[string]any))
	if err != nil {
		return err
	}
	expectedSummary := summarizeSheetChangesetsForAudit(changesets)
	if !reflect.DeepEqual(actualSummary, expectedSummary) {
		return fmt.Errorf("$.summary 与 $.changesets 复算结果不一致")
	}
	return nil
}

func parseSheetChangesetSummaryAudit(raw map[string]any) (sheetChangesetSummaryAudit, error) {
	fields := []string{"changeCount", "completeChangeCount", "partialChangeCount", "unsupportedChangeCount"}
	counts := make([]int64, len(fields))
	for index, field := range fields {
		value, err := sheetNonNegativeInteger(raw[field])
		if err != nil {
			return sheetChangesetSummaryAudit{}, fmt.Errorf("$.summary.%s %v", field, err)
		}
		counts[index] = value
	}

	result := sheetChangesetSummaryAudit{
		changeCount:               counts[0],
		completeChangeCount:       counts[1],
		partialChangeCount:        counts[2],
		unsupportedChangeCount:    counts[3],
		containsStateReset:        raw["containsStateReset"].(bool),
		containsIncompleteChanges: raw["containsIncompleteChanges"].(bool),
	}
	for _, rawSheet := range raw["affectedSheets"].([]any) {
		sheet := rawSheet.(map[string]any)
		rawRanges := sheet["ranges"].([]any)
		affected := sheetChangesetAffectedSheetAudit{
			sheetID: sheet["sheetId"].(string),
			ranges:  make([]string, 0, len(rawRanges)),
		}
		affected.sheetName, _ = sheet["sheetName"].(string)
		for _, rawRange := range rawRanges {
			affected.ranges = append(affected.ranges, rawRange.(string))
		}
		result.affectedSheets = append(result.affectedSheets, affected)
	}
	return result, nil
}

func summarizeSheetChangesetsForAudit(changesets []any) sheetChangesetSummaryAudit {
	result := sheetChangesetSummaryAudit{}
	affected := map[string]*sheetChangesetAffectedSheetAccumulator{}
	for _, rawChangeset := range changesets {
		changeset := rawChangeset.(map[string]any)
		result.containsStateReset = result.containsStateReset || changeset["eventType"] == "STATE_RESET"
		result.containsIncompleteChanges = result.containsIncompleteChanges || changeset["detailsStatus"] != "COMPLETE"
		for _, rawChange := range changeset["changes"].([]any) {
			change := rawChange.(map[string]any)
			result.changeCount++
			switch change["detailsStatus"] {
			case "COMPLETE":
				result.completeChangeCount++
			case "PARTIAL":
				result.partialChangeCount++
			}
			if change["type"] == "UNSUPPORTED_CHANGE" {
				result.unsupportedChangeCount++
			}
			for _, rawTarget := range change["targets"].([]any) {
				target := rawTarget.(map[string]any)
				if target["role"] == "SOURCE" {
					continue
				}
				sheetID, _ := target["sheetId"].(string)
				if strings.TrimSpace(sheetID) == "" {
					continue
				}
				accumulator := affected[sheetID]
				if accumulator == nil {
					accumulator = &sheetChangesetAffectedSheetAccumulator{ranges: map[string]struct{}{}}
					affected[sheetID] = accumulator
				}
				if sheetName, ok := target["sheetName"].(string); ok && strings.TrimSpace(sheetName) != "" {
					accumulator.sheetName = sheetName
				}
				if a1Range, ok := target["a1Range"].(string); ok && strings.TrimSpace(a1Range) != "" {
					accumulator.ranges[a1Range] = struct{}{}
				}
			}
		}
	}

	sheetIDs := make([]string, 0, len(affected))
	for sheetID := range affected {
		sheetIDs = append(sheetIDs, sheetID)
	}
	sort.Strings(sheetIDs)
	for _, sheetID := range sheetIDs {
		accumulator := affected[sheetID]
		ranges := make([]string, 0, len(accumulator.ranges))
		for a1Range := range accumulator.ranges {
			ranges = append(ranges, a1Range)
		}
		sort.Strings(ranges)
		result.affectedSheets = append(result.affectedSheets, sheetChangesetAffectedSheetAudit{
			sheetID: sheetID, sheetName: accumulator.sheetName, ranges: ranges,
		})
	}
	return result
}

func sheetNonNegativeInteger(value any) (int64, error) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, fmt.Errorf("必须是非负整数")
	}
	parsed, err := strconv.ParseInt(number.String(), 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("必须是非负整数")
	}
	return parsed, nil
}

func sheetRevisionNumberArg(raw string) (any, error) {
	return strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
}

func validateSheetChangesetRange(cmd *cobra.Command, _ []string) error {
	start, err := parseSheetRevisionFlag(cmd, "start-revision")
	if err != nil {
		return err
	}
	if start < 0 {
		return apperrors.NewValidation("--start-revision 必须是非负整数")
	}

	endRaw, _ := cmd.Flags().GetString("end-revision")
	if strings.TrimSpace(endRaw) == "" {
		return nil
	}
	end, err := parseSheetRevisionFlag(cmd, "end-revision")
	if err != nil {
		return err
	}
	if end < 0 {
		return apperrors.NewValidation("--end-revision 必须是非负整数")
	}
	if end < start {
		return apperrors.NewValidation("--end-revision 必须大于或等于 --start-revision")
	}
	if end-start > sheetChangesetMaxSpan {
		return apperrors.NewValidation(fmt.Sprintf(
			"单次最多查询 %d 个 revision；请把 --end-revision 调整为不大于 %d",
			sheetChangesetMaxSpan, start+sheetChangesetMaxSpan,
		))
	}
	return nil
}

func parseSheetRevisionFlag(cmd *cobra.Command, name string) (int64, error) {
	raw, _ := cmd.Flags().GetString(name)
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, apperrors.NewValidation(fmt.Sprintf("--%s 必须是 64 位整数", name))
	}
	return value, nil
}
