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
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

// devevent.go 是 `dws dev event` 配置态命令模块：企业自建应用产生的个人事件
// 定义的草稿创建与编辑、静态校验、端到端调试、直接启用与下线。
//
// 三条命令面的边界（每条叶子的 AvoidWhen 都重复声明，避免 Agent 混淆）：
//
//   - dev event ...       配置「应用产生什么事件」（本文件）
//   - dev app event ...   管理「应用接收哪些钉钉回调事件」的订阅（devapp.go）
//   - dws event ...       个人 IM/OA 实时事件的监听与消费
//
// 派发复用 devapp 的 op-app 网关（devEventProduct = "devapp"），不新增产品端点、
// 不新增 ProductDecl、不改 direct_runtime.go：九条叶子的 Identity.ProductID 仍是
// "dev"，与既有 `dev app *` 同产品分组；其中八条调用远端工具，一条 lint 在本地执行。
//
// 与 devapp.go 的关系是**同包只读复用**：本文件直接调用 devapp 的通用 helper
// （信封出口、分页元数据、错误诊断抽取、游标 flag 注册、叶子收尾），不复制副本、
// 不 import、不修改 devapp.go。事件定义域特有的语义（九条命令、--file 预检、
// 409 修订冲突、debug 阶段判定、分页 fail-closed 名单）全部在本文件内实现。

const (
	// devEventProduct 复用 devapp 的 /server/op-app 端点，不新增产品端点。
	devEventProduct = "devapp"

	// 工具名对齐 devapp 服务端注册名（tools/list 实测），改名只动这里。
	// 历史教训：`*_dev_app_event_definition(s)` 这组名字在服务端从未注册，
	// 会退化成 TOOL_NOT_FOUND → business error: success=false。
	devEventListTool     = "list_event_definitions"
	devEventGetTool      = "get_event_definition"
	devEventCreateTool   = "create_event_definition"
	devEventUpdateTool   = "update_event_definition"
	devEventValidateTool = "validate_event_definition"
	devEventDebugTool    = "debug_event_definition"
	devEventPublishTool  = "publish_event_definition"
	devEventOfflineTool  = "remove_event_definition"

	// devEventRevisionConflictCode 是写操作携带的 expectedConfigRevision 与
	// 服务端当前 config_revision 不一致时的稳定业务码；配套 HTTP 409。
	devEventRevisionConflictCode   = "CONFIG_REVISION_CONFLICT"
	devEventRevisionConflictStatus = 409

	// devEventDefinitionFileMaxBytes 是 --file / --payload-file 的读取上限。
	// 事件定义与其调试样例都是小 JSON，1 MiB 足够，同时避免误传大文件时
	// 把整个内容读进内存再报解析错误。
	devEventDefinitionFileMaxBytes = 1 << 20

	// devEventKeyMaxLength 对齐元数据表 event_key VARCHAR(128)。
	devEventKeyMaxLength = 128

	// devEventDebugTimeoutMin / Max 是 --timeout 的有界等待范围。上限存在的
	// 理由：debug 会阻塞当前 Agent 轮次等待真实事件到达，无界等待会让调用方
	// 无法区分「还在等」和「已经挂了」。
	devEventDebugTimeoutMin = time.Second
	devEventDebugTimeoutMax = 5 * time.Minute
)

// devEventReadFile 是文件读取的测试缝：Stat 成功之后再读取失败（权限被
// 并发改动、文件被换掉）只能靠替换这个变量确定性复现。
var devEventReadFile = func(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	limited := io.LimitReader(file, devEventDefinitionFileMaxBytes+1)
	return io.ReadAll(limited)
}

// devEventKeyPattern 是应用事件专用的 eventKey 命名表达式：app_user_ 前缀，
// 后续依次包含业务系统、对象和动作三段。本地只做快速失败，格式、保留名与
// 全局唯一性的权威校验仍在平台侧。
var devEventKeyPattern = regexp.MustCompile(`^app_user_[a-z][a-z0-9]*(?:_[a-z0-9]+){2,}$`)

// devEventControlledKeys 是 --file 内不接受开发者自行指定的键：Scope、启停、
// 版本与修订号都由平台按规则赋值或以独立参数携带（设计方案 §7.4）。命中即
// 本地 validation 失败，不下发到服务端。
var devEventControlledKeys = []string{
	"scope",
	"scopes",
	"defaultScope",
	"status",
	"version",
	"configRevision",
	"expectedConfigRevision",
	"debugPassedRevision",
	"unifiedAppId",
	"published",
}

// devEventDefinitionFields 是预检通过后从 --file 转发给服务端的定义字段。
// 原始文件路径不作为业务参数下发。
var devEventDefinitionFields = []string{
	"eventKey",
	"name",
	"description",
	"payloadSchema",
	"samplePayload",
}

// devEventDebugStages 是 debug 的四个阶段事实。只有全部为 true 且 debugPassed
// 为 true 才算调试通过（设计方案 §7.6：超时、断连或取消必须返回明确未通过）。
var devEventDebugStages = []string{
	"appAuthenticated",
	"eventAccepted",
	"subscriptionMatched",
	"consumerReceived",
}

// devEventRoutingAvoidWhen 是三条路由边界，每条叶子都必须携带。
var devEventRoutingAvoidWhen = []string{
	"管理应用接收钉钉回调事件的订阅用 dev app event list/subscribe/unsubscribe",
	"监听或消费个人 IM/OA 实时事件用 dws event listen-im/consume/schema/stop",
	"查询开放平台应用基本信息用 dev app get",
}

// devEventCodeHints 是稳定业务码到可恢复动作的映射。上游已给出 Hint 时不覆盖；
// 只补齐上游沉默的情况，不按错误文案猜类别。
var devEventCodeHints = map[string]string{
	"INVALID_EVENT_KEY":        "按 app_user_{业务系统}_{对象}_{动作} 规则改写 eventKey：使用 app_user_ 前缀，后接业务系统、对象和动作三段，全小写、只含字母数字与下划线、不超过 128 字符。",
	"EVENT_KEY_CONFLICT":       "eventKey 全平台唯一且不因下线释放；换一个 eventKey 重新创建，不要覆盖已有定义。",
	"INVALID_EVENT_STATE":      "先用 dws dev event get 读取当前 status 与 configRevision，再决定可执行的动作。",
	"SCOPE_NOT_READY":          "默认权限点尚未在权限服务登记完成；稍后重新执行 dws dev event get 查看 scopeReady，不要自行指定 Scope。",
	"DEBUG_REQUIRED":           "启用前必须先通过调试：执行 dws dev event debug 并等待四个阶段全部为 true。旧调试结果不能用于新草稿启用。",
	"DEBUG_CONFIG_CHANGED":     "草稿在调试通过之后又被修改，旧调试结果不能用于新草稿启用；用 dws dev event get 取最新 configRevision 后重新 debug。",
	"DEBUG_TIMEOUT":            "在 --timeout 内未收到真实事件；确认业务系统已按样例上报，再用相同 revision 重新 debug，不要把超时当作通过。",
	"SCHEMA_INCOMPATIBLE":      "生效后的 Schema 只允许兼容变更；破坏性变化请使用新的 eventKey，不要改写已启用的定义。",
	"APP_MANAGEMENT_FORBIDDEN": "当前身份没有该应用的管理权；用 dws dev app member list 确认成员角色，或改用有管理权的 --profile。",
	"INVALID_ARGUMENT":         "对照 dws schema --cli-path 输出核对本命令的参数名与取值；CLI 不会猜测或补全缺失的业务参数。",
}

// devEventRevisionConflictHint 是 409 的固定处置说明：CLI 不自动换修订号重放，
// 也不改写开发者的本地文件。
const devEventRevisionConflictHint = "重新执行 dws dev event get 获取最新草稿，对比并合并你的修改后，用返回的真实 configRevision 再提交；CLI 不会自动更新修订号重放，也不会改写你的本地文件。"

// ---------------------------------------------------------------------------
// 组构造
// ---------------------------------------------------------------------------

func newDevEventCommand(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:   "event",
		Short: "应用事件定义配置",
		Long: "配置企业自建应用产生的个人事件定义：草稿创建与编辑、静态校验、端到端调试、直接启用与下线。" +
			"管理应用接收钉钉回调事件用 dev app event；个人 IM/OA 实时事件监听与消费用 dws event。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	newGroupCommand(root)
	root.AddCommand(
		newDevEventListCommand(runner),
		newDevEventGetCommand(runner),
		newDevEventCreateCommand(runner),
		newDevEventUpdateCommand(runner),
		newDevEventValidateCommand(runner),
		newDevEventLintCommand(runner),
		newDevEventDebugCommand(runner),
		newDevEventPublishCommand(runner),
		newDevEventOfflineCommand(runner),
	)
	return root
}

// ---------------------------------------------------------------------------
// 共用声明构件
// ---------------------------------------------------------------------------

// devEventSafetyRead 是只读叶子的安全声明。四个字段彼此独立，不做机械推导。
func devEventSafetyRead() contract.SafetySpec {
	return contract.SafetySpec{
		Effect:       "read",
		Risk:         "low",
		Confirmation: "not_required",
		Idempotency:  "idempotent",
	}
}

// devEventSafetyWriteIdempotent 用于 create：同应用、同 eventKey、同输入的重试
// 返回原草稿而不覆盖不同内容，因此幂等性是已知的。
func devEventSafetyWriteIdempotent() contract.SafetySpec {
	return contract.SafetySpec{
		Effect:       "write",
		Risk:         "high",
		Confirmation: "user_required",
		Idempotency:  "idempotent",
	}
}

// devEventSafetyWrite 用于 update/debug/publish/offline：这些动作会改变草稿、
// 调试标记或线上生效状态，重复执行的结果不由 CLI 保证。
func devEventSafetyWrite() contract.SafetySpec {
	return contract.SafetySpec{
		Effect:       "write",
		Risk:         "high",
		Confirmation: "user_required",
		Idempotency:  "unknown",
	}
}

// devEventDryRun 是全树共用的 dry-run 声明：只出本地 invocation 预览，
// 不为预览发起远端读取。
func devEventDryRun() *contract.DryRunSpec {
	return &contract.DryRunSpec{PreviewKind: "invocation", RemoteReads: false}
}

// devEventInterface 声明这批叶子背后是未 pin 的远端适配器：op-app 上的事件
// 定义管理工具尚未进入 pinned MCP 元数据快照，没有单一可 pin 的等价 interface_ref。
func devEventInterface() *contract.InterfaceSpec {
	return &contract.InterfaceSpec{
		Mode:         contract.InterfaceModeComposite,
		Availability: contract.InterfaceAvailable,
		Reason:       "Reviewed unpinned remote adapter: the eight remote app-event-definition tools are dispatched through the devapp op-app gateway and are absent from the pinned MCP metadata snapshot; the local lint command does not use a remote adapter.",
	}
}

// devEventIdentity 按统一规则拼装 Identity：ProductID 固定为 "dev"，
// Name/CanonicalPath 由工具名派生，CLIPath 与 PrimaryCLIPath 逐字一致。
func devEventIdentity(name, cliPath string) contract.ToolIdentitySpec {
	return contract.ToolIdentitySpec{
		ProductID:      "dev",
		Name:           name,
		CanonicalPath:  "dev." + name,
		CLIPath:        cliPath,
		PrimaryCLIPath: cliPath,
	}
}

func devEventStringField(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func devEventIntField(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func devEventBoolField(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func devEventObjectField(description string) map[string]any {
	return map[string]any{"type": "object", "description": description, "additionalProperties": true}
}

func devEventObjectArrayField(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       map[string]any{"type": "object", "additionalProperties": true},
	}
}

// devEventResultSpec 组装叶子的 Result 契约。只描述信封 data，不重复
// ok/outcome/error/meta；每个命名 properties 子项都带非空 description。
// outcomes 固定为 success/failure —— 这组命令没有异步未终态：debug 的进行中
// 不是 pending，超时按明确未通过返回。
func devEventResultSpec(description string, properties map[string]any, required ...string) *contract.ResultSpec {
	schema := map[string]any{
		"type":                 "object",
		"description":          description,
		"additionalProperties": true,
		"properties":           properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	raw, _ := json.Marshal(schema)
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: raw,
	}
}

// devEventCursorPagination 只有 list 声明：游标翻页事实只在 meta.pagination。
func devEventCursorPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{
		Kind:            contract.PaginationKindCursor,
		CursorParameter: "cursor",
	}
}

func devEventAvoidWhen(leafSpecific string) []string {
	return append([]string{leafSpecific}, devEventRoutingAvoidWhen...)
}

// ---------------------------------------------------------------------------
// 共用 flag 声明
// ---------------------------------------------------------------------------

func devEventAppIDFlag() LeafFlag {
	return LeafFlag{
		Name:         "unified-app-id",
		Usage:        "开放平台统一应用 ID（必填），不接受 ClientId 或 AppKey 替代",
		Bind:         "unifiedAppId",
		Trim:         true,
		Required:     true,
		RequiredHint: "--unified-app-id 为必填",
	}
}

func devEventKeyFlag() LeafFlag {
	return LeafFlag{
		Name:         "event-key",
		Usage:        "完整事件键，app_user_ 开头且全平台唯一（必填）",
		Bind:         "eventKey",
		Trim:         true,
		Required:     true,
		RequiredHint: "--event-key 为必填",
	}
}

// devEventExpectedRevisionFlag 声明写操作的乐观锁参数。刻意不设 ArgDefault：
// 缺失或为 0 时由框架 required 校验拦下（exit 3），负值由 Validate 钩子拦下，
// 避免「未传被当作 0/1 提交」的静默写入。
func devEventExpectedRevisionFlag() LeafFlag {
	return LeafFlag{
		Name:         "expected-revision",
		Kind:         LeafInt,
		Usage:        "你正在修改的配置修订号，取自 dws dev event get 的 configRevision（必填，≥1）",
		Bind:         "expectedConfigRevision",
		Required:     true,
		RequiredHint: "--expected-revision 为必填：先用 dws dev event get 读取当前 configRevision，冲突时不自动换修订号重试",
	}
}

// devEventRevisionConstraint 把 ≥1 规则投影到 --help 与 Runtime Schema。
// 框架的 Custom 约束只做声明与渲染，实际规则由 devEventValidateRevision 执行。
func devEventRevisionConstraint() LeafConstraint {
	return LeafConstraint{
		Kind:        "custom",
		Flags:       []string{"expected-revision"},
		Description: "--expected-revision 必须 ≥ 1，取值为 dws dev event get 返回的真实 configRevision",
	}
}

func devEventDefinitionFileFlag() LeafFlag {
	return LeafFlag{
		Name:         "file",
		Usage:        "事件定义 JSON 文件路径（必填），一份文件定义一个事件；不接受自行指定 Scope、启停或版本",
		Bind:         "file",
		Trim:         true,
		Required:     true,
		RequiredHint: "--file 为必填：指向一份事件定义 JSON 文件",
	}
}

func devEventValidateRevision(cmd *cobra.Command, _ []string) error {
	if revision := devAppIntFlag(cmd, "expected-revision"); revision < 1 {
		return apperrors.NewValidation(
			"--expected-revision 必须 ≥ 1",
			apperrors.WithHint("先用 dws dev event get 读取当前 configRevision，再原样传入；CLI 不会用默认修订号代替。"))
	}
	return nil
}

// transformDevEventTimeout 校验 --timeout 是有界等待时长。规范化后仍回传用户
// 原始写法，避免把 120s 改写成 2m0s 让服务端与日志对不上。
func transformDevEventTimeout(raw string) (any, error) {
	value := strings.TrimSpace(raw)
	duration, err := time.ParseDuration(value)
	if err != nil {
		return nil, apperrors.NewValidation(
			fmt.Sprintf("--timeout 不是合法的时长：%v", err),
			apperrors.WithHint("使用带单位的 Go 时长写法，例如 --timeout 120s 或 --timeout 2m"))
	}
	if duration < devEventDebugTimeoutMin || duration > devEventDebugTimeoutMax {
		return nil, apperrors.NewValidation(
			fmt.Sprintf("--timeout 必须在 %s 到 %s 之间（当前 %s）", devEventDebugTimeoutMin, devEventDebugTimeoutMax, duration),
			apperrors.WithHint("调试等待过短会漏收真实事件，过长会阻塞当前轮次；超时不会被视为调试通过。"))
	}
	return value, nil
}

// ---------------------------------------------------------------------------
// 九条叶子
// ---------------------------------------------------------------------------

func newDevEventListCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "list",
		Short:         "查询应用的事件定义列表",
		Example:       "  dws dev event list --unified-app-id UNIFIED_APP_ID --status DRAFT --page-size 20 --format json",
		Tool:          devEventListTool,
		Safety:        devEventSafetyRead(),
		OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			devEventAppIDFlag(),
			{Name: "keyword", Usage: "按 eventKey 或事件名称模糊匹配", Bind: "keyword", Trim: true, OmitEmpty: true},
			{Name: "status", Usage: "按事件状态过滤", Bind: "status", Trim: true, OmitEmpty: true, Enum: []string{"DRAFT", "ONLINE", "OFFLINE"}},
		},
		Contract: LeafContract{
			Identity:    devEventIdentity(devEventListTool, "dev event list"),
			Description: "查询应用的事件定义列表",
			DryRun:      devEventDryRun(),
			Result: devEventResultSpec("当前页应用事件定义查询结果", map[string]any{
				"items": devEventObjectArrayField("当前页事件定义摘要，含 eventKey、name、status 与 configRevision；分页控制信息只读取 meta.pagination"),
			}, "items"),
			Pagination: devEventCursorPagination(),
			Parameters: []contract.ParamDecl{
				{Name: "cursor", Property: "cursor", InterfaceType: "string"},
				{Name: "page-size", Property: "pageSize", InterfaceType: "number"},
			},
			Interface: devEventInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "列出或搜索一个应用已配置的事件定义",
				UseWhen:      []string{"需要看某应用有哪些事件定义、各自处于草稿还是生效状态时"},
				AvoidWhen:    devEventAvoidWhen("已经知道 eventKey 时直接用 dev event get 读取完整定义"),
				Examples:     []string{`dws dev event list --unified-app-id uapp_self_hr_A --status DRAFT --page-size 20`},
			},
		},
		Call:      devEventCallCursor(runner),
		PostMount: devAppMetaCursor(devEventListTool),
	})
}

func newDevEventGetCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "get",
		Short:         "查询单个事件定义的草稿与生效内容",
		Example:       "  dws dev event get --unified-app-id UNIFIED_APP_ID --event-key app_user_hr_employee_joined --view draft --format json",
		Tool:          devEventGetTool,
		Safety:        devEventSafetyRead(),
		OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			devEventAppIDFlag(),
			devEventKeyFlag(),
			{Name: "view", Usage: "读取单个视图；缺省同时返回草稿与生效内容", Bind: "view", Trim: true, OmitEmpty: true, Enum: []string{"draft", "published"}},
		},
		Contract: LeafContract{
			Identity:    devEventIdentity(devEventGetTool, "dev event get"),
			Description: "查询单个事件定义的草稿与生效内容",
			DryRun:      devEventDryRun(),
			Result: devEventResultSpec("事件定义的草稿、生效内容、修订与权限点状态", map[string]any{
				"unifiedAppId":    devEventStringField("生产者应用的统一应用 ID"),
				"eventKey":        devEventStringField("事件键，创建后固定，下线不释放"),
				"status":          devEventStringField("事件状态：DRAFT、ONLINE 或 OFFLINE"),
				"configRevision":  devEventIntField("当前配置修订号；所有写操作必须原样回传为 --expected-revision"),
				"hasDraftChanges": devEventBoolField("草稿是否与其最近发布快照不同"),
				"defaultScope":    devEventStringField("按应用规则计算的默认权限点编码，开发者不可自行指定"),
				"scopeReady":      devEventBoolField("默认权限点是否已在权限服务登记完成，取自权威查询"),
				"reportUrl":       devEventStringField("业务系统上报事件使用的逻辑资源路径"),
				"updatedBy":       devEventStringField("最近修改草稿的身份"),
				"updatedAt":       devEventStringField("最近修改草稿的时间"),
				"draft":           devEventObjectField("当前草稿的完整定义"),
				"published":       devEventObjectField("最近发布的完整定义；DRAFT 状态下为空"),
			}, "unifiedAppId", "eventKey", "status", "configRevision"),
			Interface: devEventInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "读取一个事件定义的草稿、生效内容与当前修订号",
				UseWhen: []string{
					"执行 update、debug、publish 或 offline 前需要取得真实 configRevision 时",
					"需要确认权限点是否就绪（scopeReady）或上报地址时",
				},
				AvoidWhen: devEventAvoidWhen("只需要知道有哪些事件时用 dev event list；校验定义是否合法用 dev event validate"),
				Examples: []string{
					"dws dev event get --unified-app-id uapp_self_hr_A --event-key app_user_hr_employee_joined",
					"dws dev event get --unified-app-id uapp_self_hr_A --event-key app_user_hr_employee_joined --view published",
				},
			},
		},
		Call:      devEventCall(runner),
		PostMount: devAppMeta(devEventGetTool),
	})
}

func newDevEventCreateCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "create",
		Short:         "创建事件定义草稿并登记默认权限点",
		Example:       "  dws dev event create --unified-app-id UNIFIED_APP_ID --file ./hr-employee-joined.json --dry-run --format json",
		Tool:          devEventCreateTool,
		Safety:        devEventSafetyWriteIdempotent(),
		OutputRollout: output.RolloutUnifiedActive,
		// 与同树 devapp 写命令一致：确认门先于参数校验（guard-first）。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			devEventAppIDFlag(),
			devEventDefinitionFileFlag(),
		},
		Contract: LeafContract{
			Identity:    devEventIdentity(devEventCreateTool, "dev event create"),
			Description: "创建事件定义草稿并登记默认权限点",
			DryRun:      devEventDryRun(),
			Result: devEventResultSpec("新建事件定义草稿的结果", map[string]any{
				"unifiedAppId":   devEventStringField("生产者应用的统一应用 ID"),
				"eventKey":       devEventStringField("已创建的事件键，创建后固定"),
				"status":         devEventStringField("新建后的事件状态，首期为 DRAFT"),
				"configRevision": devEventIntField("新建草稿的配置修订号，后续写操作以此为准"),
				"defaultScope":   devEventStringField("按应用规则计算并登记的默认权限点编码"),
				"scopeReady":     devEventBoolField("默认权限点是否已登记完成"),
				"reportUrl":      devEventStringField("业务系统上报事件使用的逻辑资源路径"),
			}, "unifiedAppId", "eventKey", "status", "configRevision"),
			Interface: devEventInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "从一份 JSON 定义创建事件草稿，并由平台登记默认权限点",
				UseWhen:      []string{"应用需要新增一类由自己产生的个人事件时"},
				AvoidWhen: devEventAvoidWhen(
					"事件定义已存在时用 dev event update 修改草稿；create 不接受自行指定 Scope、启停或版本"),
				Examples: []string{
					"dws dev event create --unified-app-id uapp_self_hr_A --file ./hr-employee-joined.json --dry-run",
				},
			},
		},
		Call:      devEventCallWithDefinition(runner, false),
		PostMount: devAppMeta(devEventCreateTool),
	})
}

func newDevEventUpdateCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "update",
		Short:         "修改事件定义草稿，线上生效内容不变",
		Example:       "  dws dev event update --unified-app-id UNIFIED_APP_ID --event-key app_user_hr_employee_joined --file ./hr-employee-joined.json --expected-revision 2 --dry-run --format json",
		Tool:          devEventUpdateTool,
		Safety:        devEventSafetyWrite(),
		OutputRollout: output.RolloutUnifiedActive,
		// 与同树 devapp 写命令一致：确认门先于参数校验（guard-first）。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			devEventAppIDFlag(),
			devEventKeyFlag(),
			devEventDefinitionFileFlag(),
			devEventExpectedRevisionFlag(),
		},
		Constraints: []LeafConstraint{devEventRevisionConstraint()},
		Validate:    devEventValidateRevision,
		Contract: LeafContract{
			Identity:    devEventIdentity(devEventUpdateTool, "dev event update"),
			Description: "修改事件定义草稿，线上生效内容不变",
			DryRun:      devEventDryRun(),
			Result: devEventResultSpec("事件定义草稿的修改结果", map[string]any{
				"unifiedAppId":    devEventStringField("生产者应用的统一应用 ID"),
				"eventKey":        devEventStringField("被修改的事件键，不可通过 update 改名"),
				"status":          devEventStringField("修改后的事件状态；update 不改变 ONLINE 状态"),
				"configRevision":  devEventIntField("修改后的新配置修订号"),
				"hasDraftChanges": devEventBoolField("草稿是否已不同于最近发布快照"),
			}, "unifiedAppId", "eventKey", "status", "configRevision"),
			Interface: devEventInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "按修订号乐观锁修改事件定义草稿，不影响线上生效内容",
				UseWhen:      []string{"需要调整事件名称、描述、Payload Schema 或调试样例时"},
				AvoidWhen: devEventAvoidWhen(
					"修改后要让线上生效必须再执行 dev event publish；update 只写草稿，且会清除旧的调试通过标记"),
				Examples: []string{
					"dws dev event update --unified-app-id uapp_self_hr_A --event-key app_user_hr_employee_joined --file ./hr-employee-joined.json --expected-revision 2 --dry-run",
				},
			},
		},
		Call:      devEventCallWithDefinition(runner, true),
		PostMount: devAppMeta(devEventUpdateTool),
	})
}

func newDevEventValidateCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "validate",
		Short:         "只读校验事件定义与兼容性",
		Example:       "  dws dev event validate --unified-app-id UNIFIED_APP_ID --event-key app_user_hr_employee_joined --format json",
		Tool:          devEventValidateTool,
		Safety:        devEventSafetyRead(),
		OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			devEventAppIDFlag(),
			devEventKeyFlag(),
		},
		Contract: LeafContract{
			Identity:    devEventIdentity(devEventValidateTool, "dev event validate"),
			Description: "只读校验事件定义与兼容性",
			DryRun:      devEventDryRun(),
			Result: devEventResultSpec("事件定义的静态校验与兼容检查结论", map[string]any{
				"configRevision": devEventIntField("本次校验针对的配置修订号"),
				"schemaValid":    devEventBoolField("Payload Schema 本身是否合法"),
				"sampleValid":    devEventBoolField("调试样例是否符合 Payload Schema"),
				"compatible":     devEventBoolField("相对已生效定义是否为兼容变更"),
				"issues":         devEventObjectArrayField("校验问题清单，含字段路径与原因；为空表示无问题"),
			}, "configRevision", "schemaValid", "sampleValid", "compatible"),
			Interface: devEventInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "在不建立订阅、不上报事件的前提下静态校验事件定义",
				UseWhen:      []string{"publish 之前想先确认 Schema、样例与兼容性是否通过时"},
				AvoidWhen: devEventAvoidWhen(
					"validate 不验证真实投递链路，端到端确认用 dev event debug；它也不写调试通过标记"),
				Examples: []string{
					"dws dev event validate --unified-app-id uapp_self_hr_A --event-key app_user_hr_employee_joined",
				},
			},
		},
		Call:      devEventCall(runner),
		PostMount: devAppMeta(devEventValidateTool),
	})
}

func newDevEventDebugCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:   "debug",
		Short: "受控端到端调试事件定义",
		Long: "在受控会话内完成一次真实上报到真实接收的端到端验证，并只在四个阶段全部为 true 时" +
			"标记本修订调试通过。调试不递增配置修订号，因此草稿未变时可用同一 revision 直接 publish。",
		Example:       "  dws dev event debug --unified-app-id UNIFIED_APP_ID --event-key app_user_hr_employee_joined --expected-revision 1 --timeout 120s --dry-run --format json",
		Tool:          devEventDebugTool,
		Safety:        devEventSafetyWrite(),
		OutputRollout: output.RolloutUnifiedActive,
		// 与同树 devapp 写命令一致：确认门先于参数校验（guard-first）。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			devEventAppIDFlag(),
			devEventKeyFlag(),
			devEventExpectedRevisionFlag(),
			{Name: "payload-file", Usage: "调试用的纯业务 Payload JSON 文件路径；缺省使用已保存的 samplePayload", Bind: "payloadFile", Trim: true, OmitEmpty: true},
			{Name: "timeout", Usage: "有界等待时长，1s 到 5m；超时按明确未通过返回，不假报调试通过", Bind: "timeout", Trim: true, OmitEmpty: true, Transform: transformDevEventTimeout, Example: "120s"},
		},
		Constraints: []LeafConstraint{devEventRevisionConstraint()},
		Validate:    devEventValidateRevision,
		Contract: LeafContract{
			Identity:    devEventIdentity(devEventDebugTool, "dev event debug"),
			Description: "受控端到端调试事件定义",
			DryRun:      devEventDryRun(),
			Result: devEventResultSpec("受控调试的阶段事实与结论", map[string]any{
				"eventKey":       devEventStringField("被调试的事件键"),
				"testedRevision": devEventIntField("本次调试针对的配置修订号；草稿再变更后该结论失效"),
				"debugPassed":    devEventBoolField("调试是否通过；只有四个阶段全部为 true 时才可能为 true"),
				"stages": map[string]any{
					"type":        "object",
					"description": "四个阶段事实，任一为 false 表示调试未通过",
					"properties": map[string]any{
						"appAuthenticated":    devEventBoolField("业务系统用应用凭据鉴权上报是否成功"),
						"eventAccepted":       devEventBoolField("上报入口是否按会话冻结的草稿校验并接纳该事件"),
						"subscriptionMatched": devEventBoolField("事件是否匹配到本次调试的临时订阅，而非正式订阅"),
						"consumerReceived":    devEventBoolField("消费端是否真实收到该事件"),
					},
				},
				"eventId":     devEventStringField("本次调试事件的平台稳定 ID"),
				"nextCommand": devEventStringField("调试通过后可直接执行的 publish 命令；未通过时为空"),
			}, "eventKey", "testedRevision", "debugPassed", "stages"),
			Interface: devEventInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "对指定修订的事件定义执行一次真实上报到真实接收的受控调试",
				UseWhen:      []string{"publish 前需要证明事件真的能被业务系统上报并被消费端收到时"},
				AvoidWhen: devEventAvoidWhen(
					"只想静态检查 Schema 与兼容性用 dev event validate；debug 会建立临时订阅并产生一次真实事件"),
				Examples: []string{
					"dws dev event debug --unified-app-id uapp_self_hr_A --event-key app_user_hr_employee_joined --expected-revision 1 --timeout 120s --dry-run",
				},
			},
		},
		Call:      devEventCallWithPayloadFile(runner),
		PostMount: devAppMeta(devEventDebugTool),
	})
}

func newDevEventPublishCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "publish",
		Short:         "校验通过后直接启用事件定义",
		Example:       "  dws dev event publish --unified-app-id UNIFIED_APP_ID --event-key app_user_hr_employee_joined --expected-revision 1 --dry-run --format json",
		Tool:          devEventPublishTool,
		Safety:        devEventSafetyWrite(),
		OutputRollout: output.RolloutUnifiedActive,
		// 与同树 devapp 写命令一致：确认门先于参数校验（guard-first）。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			devEventAppIDFlag(),
			devEventKeyFlag(),
			devEventExpectedRevisionFlag(),
		},
		Constraints: []LeafConstraint{devEventRevisionConstraint()},
		Validate:    devEventValidateRevision,
		Contract: LeafContract{
			Identity:    devEventIdentity(devEventPublishTool, "dev event publish"),
			Description: "校验通过后直接启用事件定义",
			DryRun:      devEventDryRun(),
			Result: devEventResultSpec("事件定义的启用结果", map[string]any{
				"unifiedAppId":     devEventStringField("生产者应用的统一应用 ID"),
				"eventKey":         devEventStringField("被启用的事件键"),
				"status":           devEventStringField("启用后的事件状态，成功时为 ONLINE"),
				"configRevision":   devEventIntField("启用后递增的新配置修订号，后续 update 以此为准"),
				"published":        devEventObjectField("本次生效的完整发布定义快照"),
				"alreadyPublished": devEventBoolField("该修订是否此前已启用，用于区分重复执行"),
				"hasDraftChanges":  devEventBoolField("启用后草稿是否仍与快照不同"),
			}, "unifiedAppId", "eventKey", "status", "configRevision"),
			Interface: devEventInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "把通过校验与调试的事件定义草稿复制为线上生效配置",
				UseWhen:      []string{"dev event debug 已通过且草稿未再变更，需要让事件正式生效时"},
				AvoidWhen: devEventAvoidWhen(
					"未通过调试或草稿在调试后又被改过时不能启用；publish 无人工审核，生效后只允许兼容变更"),
				Examples: []string{
					"dws dev event publish --unified-app-id uapp_self_hr_A --event-key app_user_hr_employee_joined --expected-revision 1 --dry-run",
				},
			},
		},
		Call:      devEventCall(runner),
		PostMount: devAppMeta(devEventPublishTool),
	})
}

func newDevEventOfflineCommand(runner executor.Runner) *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:           "offline",
		Short:         "下线事件定义并停止正常投递",
		Example:       "  dws dev event offline --unified-app-id UNIFIED_APP_ID --event-key app_user_hr_employee_joined --expected-revision 3 --dry-run --format json",
		Tool:          devEventOfflineTool,
		Safety:        devEventSafetyWrite(),
		OutputRollout: output.RolloutUnifiedActive,
		// 与同树 devapp 写命令一致：确认门先于参数校验（guard-first）。
		ConfirmFirst: true,
		Flags: []LeafFlag{
			devEventAppIDFlag(),
			devEventKeyFlag(),
			devEventExpectedRevisionFlag(),
		},
		Constraints: []LeafConstraint{devEventRevisionConstraint()},
		Validate:    devEventValidateRevision,
		Contract: LeafContract{
			Identity:    devEventIdentity(devEventOfflineTool, "dev event offline"),
			Description: "下线事件定义并停止正常投递",
			DryRun:      devEventDryRun(),
			Result: devEventResultSpec("事件定义的下线结果", map[string]any{
				"unifiedAppId":   devEventStringField("生产者应用的统一应用 ID"),
				"eventKey":       devEventStringField("被下线的事件键；下线不释放该名称"),
				"status":         devEventStringField("下线后的事件状态，成功时为 OFFLINE"),
				"configRevision": devEventIntField("下线后的新配置修订号"),
			}, "unifiedAppId", "eventKey", "status", "configRevision"),
			Interface: devEventInterface(),
			Selection: contract.SelectionSpec{
				AgentSummary: "停止一个事件定义的正式投递，保留草稿与最后发布快照",
				UseWhen:      []string{"事件不再需要投递，但要保留定义以便后续按相同校验重新启用时"},
				AvoidWhen: devEventAvoidWhen(
					"offline 只改状态与修订号，不删除草稿、快照或订阅关系；重新启用要再走 validate、debug 与 publish"),
				Examples: []string{
					"dws dev event offline --unified-app-id uapp_self_hr_A --event-key app_user_hr_employee_joined --expected-revision 3 --dry-run",
				},
			},
		},
		Call:      devEventCall(runner),
		PostMount: devAppMeta(devEventOfflineTool),
	})
}

// ---------------------------------------------------------------------------
// 派发
// ---------------------------------------------------------------------------

func devEventCall(runner executor.Runner) func(*cobra.Command, string, map[string]any) error {
	return func(cmd *cobra.Command, tool string, params map[string]any) error {
		return runDevEventTool(runner, cmd, tool, params)
	}
}

// devEventCallCursor 复用 devapp 的游标参数装配（PASS-THROUGH：原样转发
// cursor/pageSize，不做 offset/page 翻译），但收尾走本模块的信封出口，
// 以便 list 的分页 fail-closed 判断生效。
func devEventCallCursor(runner executor.Runner) func(*cobra.Command, string, map[string]any) error {
	return func(cmd *cobra.Command, tool string, params map[string]any) error {
		devAppApplyCursorParams(cmd, params)
		return runDevEventTool(runner, cmd, tool, params)
	}
}

// devEventCallWithDefinition 是 create/update 的执行体：先本地预检 --file，
// 再把解析出的定义字段并入 params。matchEventKey 为 true 时（update）要求文件
// 内的 eventKey 与 --event-key 完全一致。
func devEventCallWithDefinition(runner executor.Runner, matchEventKey bool) func(*cobra.Command, string, map[string]any) error {
	return func(cmd *cobra.Command, tool string, params map[string]any) error {
		if err := devEventApplyDefinitionFile(params, matchEventKey); err != nil {
			return err
		}
		return runDevEventTool(runner, cmd, tool, params)
	}
}

// devEventCallWithPayloadFile 是 debug 的执行体：--payload-file 可选，提供时
// 必须是一份纯业务 Payload JSON 对象，作为 samplePayload 覆盖已保存的样例。
func devEventCallWithPayloadFile(runner executor.Runner) func(*cobra.Command, string, map[string]any) error {
	return func(cmd *cobra.Command, tool string, params map[string]any) error {
		path, _ := params["payloadFile"].(string)
		delete(params, "payloadFile")
		if strings.TrimSpace(path) != "" {
			payload, err := devEventReadJSONObject(path, "--payload-file")
			if err != nil {
				return err
			}
			params["samplePayload"] = payload
		}
		return runDevEventTool(runner, cmd, tool, params)
	}
}

func runDevEventTool(runner executor.Runner, cmd *cobra.Command, tool string, params map[string]any) error {
	invocation := executor.NewHelperInvocation(cobracmd.LegacyCommandPath(cmd), devEventProduct, tool, params)
	invocation.DryRun = commandDryRun(cmd)
	result, err := runner.Run(cmd.Context(), invocation)
	if err != nil {
		return devEventRunnerError(err)
	}
	result.Invocation.Tool = tool
	result = normalizeDevAppServiceResult(result)
	return writeDevEventEnvelope(cmd, result)
}

// writeDevEventEnvelope 是本模块的统一信封出口：成功 → ok:true/outcome:success
// + data；--dry-run → 追加 dry_run:true（dry-run 是已完成的本地预演，不是异步
// 未终态）。
//
// 九条叶子都声明 OutputRollout: RolloutUnifiedActive，因此实际生效的是
// devEventCommandResult：失败投影、debug 阶段判定与分页 fail-closed 只在那条
// 路径上成立。legacy 信封恒为 ok:true，会把 409 修订冲突写成成功，等于假通过。
func writeDevEventEnvelope(cmd *cobra.Command, result executor.Result) error {
	env := &output.Envelope{
		OK:      true,
		Outcome: output.OutcomeSuccess,
		Data:    devAppEnvelopeData(result),
	}
	if result.Invocation.DryRun {
		env.DryRun = true
	} else {
		env.Meta, _ = devAppPaginationMeta(env.Data)
	}
	return writeDevRolloutResult(cmd, devEventCommandResult(result), env, output.FormatJSON)
}

// ---------------------------------------------------------------------------
// 结果归一
// ---------------------------------------------------------------------------

func devEventCommandResult(result executor.Result) output.CommandResult {
	tool := strings.TrimSpace(result.Invocation.Tool)
	dryRun := result.Invocation.DryRun
	data := devAppEnvelopeData(result)
	content, isObject := data.(map[string]any)
	if isObject {
		if failure := devEventFailureResult(content); failure != nil {
			return failure
		}
		// debug 的阶段判定只在真实响应上做：dry-run 是本地 invocation 预览，
		// 没有阶段事实，也不得出现 debugPassed:true。
		if tool == devEventDebugTool && !dryRun {
			if failure := devEventDebugNotPassed(content); failure != nil {
				return failure
			}
			devEventAppendDebugNextCommand(content, result.Invocation.Params)
		}
	}
	successOptions := make([]output.ResultOption, 0, 2)
	if dryRun {
		successOptions = append(successOptions, output.WithDryRun())
	}
	meta, err := devAppPaginationMeta(data)
	if err != nil {
		return output.Failure(&output.ErrorInfo{
			Type: "api", Subtype: "pagination_inconsistent", Message: err.Error(),
			Hint: "保留原始响应并停止翻页；不要把当前页当作完整结果。",
		})
	}
	if meta != nil {
		successOptions = append(successOptions, output.WithMeta(meta))
		data = devAppDataWithoutPagination(data)
	} else if devEventToolRequiresPagination(tool) && !dryRun {
		// 声明了分页的读命令必须回 hasMore/nextCursor；缺失时 fail-closed，
		// 避免把不完整的一页当作全量结果。
		return output.Failure(&output.ErrorInfo{
			Type:    "api",
			Subtype: "pagination_inconsistent",
			Message: "declared paginated response is missing hasMore and nextCursor",
			Hint:    "保留原始响应并停止翻页；不要把当前页当作完整结果。",
		})
	}
	return output.Success(data, successOptions...)
}

// devEventToolRequiresPagination 是本模块的分页 fail-closed 名单：只有 list
// 声明了游标分页。
func devEventToolRequiresPagination(tool string) bool {
	return strings.TrimSpace(tool) == devEventListTool
}

// devEventFailureResult 复用 devapp 的失败判定，只在其之上叠加事件定义域的
// 稳定业务码语义。非失败载荷返回 nil，交由成功路径处理。
func devEventFailureResult(content map[string]any) output.CommandResult {
	if devAppFailureResult(content) == nil {
		return nil
	}
	return output.Failure(devEventErrorInfo(content))
}

// devEventErrorInfo 在 devapp 的诊断抽取之上补齐事件定义域的可恢复动作，并把
// 409 修订冲突映射为不可透明重试的显式失败。outcome=unknown 时保留 devapp
// 失败路径的专门化投影（remote_outcome_unknown、ExecutionStarted、不可盲目
// 重试的动作），而不是把它们降级成普通业务失败。
func devEventErrorInfo(content map[string]any) *output.ErrorInfo {
	info := devAppErrorInfo(content, "dev event operation failed")
	outcome := strings.ToLower(strings.TrimSpace(devAppCallStateString(content, "outcome")))
	if outcome == "unknown" {
		if info.Subtype == "" {
			info.Subtype = "remote_outcome_unknown"
		}
		if info.ExecutionStarted == nil {
			started := true
			info.ExecutionStarted = &started
		}
		info.Retryable = false
		if len(info.Actions) == 0 {
			info.Actions = []string{"先查询当前操作状态，确认远端是否已生效；不要直接重试写操作"}
		}
	}
	code := devEventUpstreamCode(info)
	if code == "" {
		return info
	}
	if hint, ok := devEventCodeHints[code]; ok && strings.TrimSpace(info.Hint) == "" {
		info.Hint = hint
	}
	if code != devEventRevisionConflictCode {
		return info
	}
	// 修订冲突不是网络抖动：Retryable 保持缺席（false），Details 同时给出双方
	// 修订号，让调用方能够对比而不是盲目重放。
	info.Type = "api"
	info.HTTPStatus = devEventRevisionConflictStatus
	info.Retryable = false
	info.Hint = devEventRevisionConflictHint
	details := info.Details
	if details == nil {
		details = map[string]any{}
	}
	if value, ok := devAppFirstContentValue(content, "expectedConfigRevision", "expected_config_revision"); ok {
		details["expectedConfigRevision"] = value
	}
	if value, ok := devAppFirstContentValue(content, "currentConfigRevision", "current_config_revision", "configRevision", "config_revision"); ok {
		details["currentConfigRevision"] = value
	}
	info.Details = details
	return info
}

func devEventUpstreamCode(info *output.ErrorInfo) string {
	if info == nil {
		return ""
	}
	code, _ := info.UpstreamCode.(string)
	return strings.TrimSpace(code)
}

// devEventDebugNotPassed 实现方案 A 的判定，三类结论互斥且都不假报通过：
//
//   - 上游完全没有回报阶段事实 → debug_not_ready：调试运行能力（临时订阅、
//     调试回执）与真实鉴权上报入口尚未冻结，命令与契约已声明但依赖未就绪；
//   - 回报了阶段事实但未全部为 true，或 debugPassed 不为 true → debug_not_passed，
//     并在 Params 中列出未完成阶段；
//   - 四个阶段全部为 true 且 debugPassed 为 true → 通过，返回 nil。
func devEventDebugNotPassed(content map[string]any) output.CommandResult {
	_, hasStages := content["stages"]
	_, hasPassed := content["debugPassed"]
	if !hasStages && !hasPassed {
		return output.Failure(devEventDebugErrorInfo(content, "debug_not_ready",
			"event definition debug is not ready: upstream reported no stage facts",
			"调试运行能力与真实鉴权上报入口尚未就绪，本次没有产生任何阶段事实；不要把它当作调试通过，也不要据此 publish。先用 dws dev event validate 做静态校验。"))
	}
	passed, _ := content["debugPassed"].(bool)
	stages, _ := content["stages"].(map[string]any)
	incomplete := make([]string, 0, len(devEventDebugStages))
	for _, stage := range devEventDebugStages {
		if ok, _ := stages[stage].(bool); !ok {
			incomplete = append(incomplete, stage)
		}
	}
	if passed && len(incomplete) == 0 {
		return nil
	}
	if len(incomplete) > 0 {
		info := devEventDebugErrorInfo(content, "debug_not_passed",
			"event definition debug did not pass: incomplete stages "+strings.Join(incomplete, ", "),
			"补齐未完成阶段后重新调试：确认业务系统已用应用凭据按样例真实上报，且调试会话未超时；旧调试结果不能用于新草稿启用。")
		info.Params = append([]string(nil), incomplete...)
		return output.Failure(info)
	}
	return output.Failure(devEventDebugErrorInfo(content, "debug_not_passed",
		"event definition debug did not pass",
		"配置服务回报调试未通过；用 dws dev event get 核对当前 configRevision 后重新 debug，不要把未通过当作可以 publish。"))
}

// devEventDebugErrorInfo 组装 debug 失败的稳定骨架：wire-stable 字段
// （type/subtype/message/hint）由本模块决定，上游的稳定业务码与修订号事实
// 原样透传，不按错误文案猜类别。
func devEventDebugErrorInfo(content map[string]any, subtype, message, hint string) *output.ErrorInfo {
	info := &output.ErrorInfo{
		Type:    "api",
		Subtype: subtype,
		Message: message,
		Hint:    hint,
	}
	if testedRevision, ok := devAppFirstContentValue(content, "testedRevision", "tested_revision"); ok {
		info.Details = map[string]any{"testedRevision": testedRevision}
	}
	if code := devEventUpstreamCode(devAppErrorInfo(content, "")); code != "" {
		info.UpstreamCode = code
		if codeHint, ok := devEventCodeHints[code]; ok {
			info.Hint = codeHint
		}
	}
	return info
}

// devEventAppendDebugNextCommand 在调试通过且上游未给出 nextCommand 时，按用户
// 本次传入的真实参数补一条可直接执行的 publish 命令。调试不递增修订号，因此
// publish 使用同一个 expectedConfigRevision。命令字符串会被 Agent/用户原样
// 复制执行，所以仅在两个标识符都落在安全字符集内时才生成，避免把含空格或
// shell 元字符的输入拼进可执行命令。
func devEventAppendDebugNextCommand(content map[string]any, params map[string]any) {
	if existing, _ := content["nextCommand"].(string); strings.TrimSpace(existing) != "" {
		return
	}
	appID, _ := params["unifiedAppId"].(string)
	eventKey, _ := params["eventKey"].(string)
	revision, hasRevision := params["expectedConfigRevision"].(int)
	appID = strings.TrimSpace(appID)
	eventKey = strings.TrimSpace(eventKey)
	if appID == "" || eventKey == "" || !hasRevision {
		return
	}
	if !devEventSafeCommandToken(appID) || !devEventKeyPattern.MatchString(eventKey) {
		return
	}
	content["nextCommand"] = fmt.Sprintf(
		"dws dev event publish --unified-app-id %s --event-key %s --expected-revision %d --yes",
		appID, eventKey, revision)
}

// devEventSafeCommandToken 约束可拼入 nextCommand 的自由文本输入：仅接受
// 字母、数字、下划线、点、连字符，杜绝空格与 shell 元字符。
func devEventSafeCommandToken(value string) bool {
	matched, err := regexp.MatchString(`^[A-Za-z0-9_.-]+$`, value)
	return err == nil && matched
}

// ---------------------------------------------------------------------------
// --file 本地预检
// ---------------------------------------------------------------------------

// devEventApplyDefinitionFile 读取并预检 --file，然后把定义字段并入 params。
// 原始文件路径不作为业务参数下发；CLI 只读该文件，任何情况下都不回写。
func devEventApplyDefinitionFile(params map[string]any, matchEventKey bool) error {
	path, _ := params["file"].(string)
	path = strings.TrimSpace(path)
	delete(params, "file")
	if path == "" {
		return apperrors.NewValidation("--file 为必填", apperrors.WithHint("指向一份事件定义 JSON 文件，一份文件定义一个事件。"))
	}
	definition, err := devEventReadDefinitionFile(path)
	if err != nil {
		return err
	}
	if matchEventKey {
		flagKey := strings.TrimSpace(devEventParamString(params, "eventKey"))
		fileKey := strings.TrimSpace(devEventParamString(definition, "eventKey"))
		if flagKey != fileKey {
			return apperrors.NewValidation(
				fmt.Sprintf("--file 中的 eventKey（%q）与 --event-key（%q）不一致", fileKey, flagKey),
				apperrors.WithHint("eventKey 创建后固定且不可通过 update 改名；请修正文件，或对另一个事件使用其自身的 eventKey。"))
		}
	}
	for _, field := range devEventDefinitionFields {
		if value, ok := definition[field]; ok {
			if field == "payloadSchema" || field == "samplePayload" {
				encoded, err := json.Marshal(value)
				if err != nil {
					return apperrors.NewValidation(
						fmt.Sprintf("%s 的 %s 无法序列化：%v", path, field, err),
						apperrors.WithHint("保持 JSON 对象结构传入；CLI 会以 JSON 文本透传，避免布尔值、数字等类型被网关适配层转换。"))
				}
				params[field] = string(encoded)
				continue
			}
			params[field] = value
		}
	}
	return nil
}

// devEventReadDefinitionFile 执行 --file 的本地预检。预检只做快速失败：格式、
// 保留名与全局唯一性的权威校验仍在平台侧，Schema 符合性判断是服务端 validate
// 的职责，本地不做。
func devEventReadDefinitionFile(path string) (map[string]any, error) {
	definition, err := devEventReadJSONObject(path, "--file")
	if err != nil {
		return nil, err
	}
	if rejected := devEventRejectedControlledKeys(definition); len(rejected) > 0 {
		return nil, apperrors.NewValidation(
			fmt.Sprintf("%s 含不接受开发者自行指定的键：%s", path, strings.Join(rejected, ", ")),
			apperrors.WithHint("Scope、启停状态、版本与修订号都由平台按规则赋值；请从文件中删除这些键后重试。"))
	}
	for _, field := range []string{"eventKey", "name"} {
		if strings.TrimSpace(devEventParamString(definition, field)) == "" {
			return nil, apperrors.NewValidation(
				fmt.Sprintf("%s 缺少必填键 %q", path, field),
				apperrors.WithHint("eventKey 是 app_user_ 开头的全平台唯一事件键，name 是事件名称，两者都必须是文件顶层的非空字符串。"))
		}
	}
	if err := devEventCheckObjectField(path, definition, "payloadSchema", true); err != nil {
		return nil, err
	}
	schema, _ := definition["payloadSchema"].(map[string]any)
	if kind := strings.TrimSpace(devEventParamString(schema, "type")); kind != "object" {
		return nil, apperrors.NewValidation(
			fmt.Sprintf("%s 的 payloadSchema.type 必须是 \"object\"（当前 %q）", path, kind),
			apperrors.WithHint("首期 Payload 根类型固定为 object；业务字段写在 payloadSchema.properties 下。"))
	}
	if err := devEventCheckObjectField(path, definition, "samplePayload", false); err != nil {
		return nil, err
	}
	eventKey := strings.TrimSpace(devEventParamString(definition, "eventKey"))
	if len(eventKey) > devEventKeyMaxLength {
		return nil, apperrors.NewValidation(
			fmt.Sprintf("%s 的 eventKey 超过 %d 字符（当前 %d）", path, devEventKeyMaxLength, len(eventKey)),
			apperrors.WithHint("缩短 eventKey；它同时用作 MQ Tag，长度受全链路最小可用容量限制。"))
	}
	if !devEventKeyPattern.MatchString(eventKey) {
		return nil, apperrors.NewValidation(
			fmt.Sprintf("%s 的 eventKey %q 不符合命名规则", path, eventKey),
			apperrors.WithHint("命名表达式为 ^app_user_[a-z][a-z0-9]*(?:_[a-z0-9]+){2,}$，格式为 app_user_{业务系统}_{对象}_{动作}，例如 app_user_hr_employee_joined。"))
	}
	return definition, nil
}

// devEventReadJSONObject 读取一份 JSON 对象文件，带上限与 BOM 容错。
func devEventReadJSONObject(path string, flagName string) (map[string]any, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, apperrors.NewValidation(
			fmt.Sprintf("%s 指向的文件不可读：%v", flagName, err),
			apperrors.WithHint("确认路径正确，且当前身份有读取权限；CLI 不会创建或改写该文件。"))
	}
	if info.IsDir() {
		return nil, apperrors.NewValidation(
			fmt.Sprintf("%s 指向的是目录而不是文件：%s", flagName, path),
			apperrors.WithHint("传入一份 JSON 文件路径，一份文件定义一个事件。"))
	}
	if info.Size() > devEventDefinitionFileMaxBytes {
		return nil, apperrors.NewValidation(
			fmt.Sprintf("%s 指向的文件超过 %d 字节上限（当前 %d 字节）：%s", flagName, devEventDefinitionFileMaxBytes, info.Size(), path),
			apperrors.WithHint("事件定义与调试样例都是小 JSON；请拆分内容或移除无关字段。"))
	}
	raw, err := devEventReadFile(path)
	if err != nil {
		return nil, apperrors.NewValidation(
			fmt.Sprintf("%s 读取文件 %q 失败：%v", flagName, path, err),
			apperrors.WithHint("确认文件存在且可读；CLI 只读该文件，不会改写它。"))
	}
	if len(raw) > devEventDefinitionFileMaxBytes {
		return nil, apperrors.NewValidation(
			fmt.Sprintf("%s 指向的文件超过 %d 字节上限：%s", flagName, devEventDefinitionFileMaxBytes, path),
			apperrors.WithHint("事件定义与调试样例都是小 JSON；请拆分内容或移除无关字段。"))
	}
	text := strings.TrimSpace(strings.TrimPrefix(string(raw), "\ufeff"))
	if text == "" {
		return nil, apperrors.NewValidation(
			fmt.Sprintf("%s 指向的文件为空：%s", flagName, path),
			apperrors.WithHint("写入一份 JSON 对象，至少包含 eventKey、name 与 payloadSchema。"))
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, apperrors.NewValidation(
			fmt.Sprintf("%s 指向的文件 %q 不是合法 JSON 对象：%v", flagName, path, err),
			apperrors.WithHint("文件顶层必须是 JSON 对象；数组、字符串或 null 都不接受。"))
	}
	if parsed == nil {
		return nil, apperrors.NewValidation(
			fmt.Sprintf("%s 指向的文件 %q 顶层是 null，必须是 JSON 对象", flagName, path),
			apperrors.WithHint("写入一份 JSON 对象，至少包含 eventKey、name 与 payloadSchema。"))
	}
	return parsed, nil
}

// devEventRejectedControlledKeys 返回文件中出现的受控键，保持声明顺序。
func devEventRejectedControlledKeys(definition map[string]any) []string {
	rejected := make([]string, 0, len(devEventControlledKeys))
	for _, key := range devEventControlledKeys {
		if _, ok := definition[key]; ok {
			rejected = append(rejected, key)
		}
	}
	return rejected
}

// devEventCheckObjectField 校验一个可选（required=false）或必填（required=true）
// 的字段必须是 JSON 对象。不做 Schema 符合性判断——那是服务端 validate 的职责。
//
// 显式 null 与「键缺席」刻意区分：缺席才是不修改，null 会被原样下发，可能把
// 服务端已保存的样例清空。定义文件里写 null 没有正当用途，一律快速失败。
func devEventCheckObjectField(path string, definition map[string]any, field string, required bool) error {
	value, present := definition[field]
	if !present {
		if required {
			return apperrors.NewValidation(
				fmt.Sprintf("%s 缺少必填键 %q", path, field),
				apperrors.WithHint("payloadSchema 必须是 JSON 对象，且 payloadSchema.type 固定为 \"object\"。"))
		}
		return nil
	}
	if value == nil {
		return apperrors.NewValidation(
			fmt.Sprintf("%s 的 %s 是 null", path, field),
			apperrors.WithHint(fmt.Sprintf("省略 %s 键表示不修改；写 null 会被原样下发并可能清空服务端已保存的值。请删除该键，或写成 JSON 对象。", field)))
	}
	if _, ok := value.(map[string]any); !ok {
		return apperrors.NewValidation(
			fmt.Sprintf("%s 的 %s 必须是 JSON 对象", path, field),
			apperrors.WithHint(fmt.Sprintf("%s 承载业务字段定义或脱敏调试样例，数组、字符串与 null 都不接受。", field)))
	}
	return nil
}

func devEventParamString(source map[string]any, key string) string {
	if source == nil {
		return ""
	}
	value, _ := source[key].(string)
	return value
}
