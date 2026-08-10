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

package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Outcome 是一次 CLI 调用的结果类别（互斥，四值）。
// 门禁拦截（confirmation_required）是 failure 的子类，不占第五值
// （契约规范 §1 结果总图）。
type Outcome string

// 四个规范 Outcome 值。wire 值恒为这些小写字符串（契约规范 §2.5）。
const (
	OutcomeSuccess        Outcome = "success"
	OutcomePending        Outcome = "pending"
	OutcomePartialFailure Outcome = "partial_failure"
	OutcomeFailure        Outcome = "failure"
)

// String 返回 Outcome 的 wire 表示（恒为小写规范值）。
func (o Outcome) String() string { return string(o) }

// MarshalJSON 把 Outcome 序列化为小写规范字符串。仅接受四个规范值，
// 非法值（含零值）在序列化期即报错，杜绝静默产出无效 wire 值
// （契约规范 §1/§2.5「仅四值」）。
func (o Outcome) MarshalJSON() ([]byte, error) {
	switch o {
	case OutcomeSuccess, OutcomePending, OutcomePartialFailure, OutcomeFailure:
		return json.Marshal(string(o))
	default:
		return nil, fmt.Errorf("output: cannot marshal invalid outcome %q", string(o))
	}
}

// ParseOutcome 严格解析 s 为 Outcome：仅接受四个小写规范值，
// 拒绝大小写变体、前后空白与空串（契约规范 §1）。
func ParseOutcome(s string) (Outcome, error) {
	switch s {
	case string(OutcomeSuccess):
		return OutcomeSuccess, nil
	case string(OutcomePending):
		return OutcomePending, nil
	case string(OutcomePartialFailure):
		return OutcomePartialFailure, nil
	case string(OutcomeFailure):
		return OutcomeFailure, nil
	}
	return "", fmt.Errorf("output: invalid outcome %q: must be one of %q, %q, %q, %q",
		s, OutcomeSuccess, OutcomePending, OutcomePartialFailure, OutcomeFailure)
}

// Envelope 是统一输出信封（契约规范 §2 信封结构）。
// 字段声明顺序对齐 §2.5 字段总表；Go 序列化按声明顺序输出，
// 由此得到稳定的顶层键顺序（golden 断言的前提）。
type Envelope struct {
	// OK 为框架计算值：ok == (outcome ∈ {success, pending})（不变量 I1）。
	// 恒序列化（无 omitempty），类型为 bool，禁止字符串布尔。
	OK bool `json:"ok"`

	// Outcome 仅四值（见 Outcome 常量）。恒序列化。
	Outcome Outcome `json:"outcome"`

	// Identity 为执行身份（user/app），空则省略（§2.5）。
	Identity string `json:"identity,omitempty"`

	// DryRun 在 dry-run 预览时为 true，默认省略（§6 场景6）。
	DryRun bool `json:"dry_run,omitempty"`

	// Data 是业务载荷（L2）：强类型 DTO 序列化，禁止手拼 JSON。
	// 可为 object 或 array；空数组 [] 是合法载荷（原样序列化，
	// 见空结果表达 data:[] + count:0）。
	Data any `json:"data,omitempty"`

	// Meta 是元数据层（count/operation/pagination），整体 omitempty。
	Meta *Meta `json:"meta,omitempty"`

	// Error 是失败明细（L1）：非空 ⇔ outcome==failure（不变量 I3）。
	Error *ErrorInfo `json:"error,omitempty"`

	// Notice 是系统级通知注入位（_notice，如升级提示），空则省略。
	// 契约仅规定 object 注入位；具体通知 shape 由后续批次定型。
	Notice any `json:"_notice,omitempty"`
}

// Meta 是信封元数据层（契约规范 §2.5）。
// 分页/异步操作元数据挂 meta 层而非 data 层（§3）。
type Meta struct {
	// Count 是列表结果条数。指针类型区分「未设置」与「0 条」：
	// 空结果场景必须显式输出 count:0（§3 / AC-06），
	// nil 时字段缺席（omitempty）。
	Count *int `json:"count,omitempty"`

	// Operation 是异步操作信息（pending 信封，§2.2）。
	Operation *OperationInfo `json:"operation,omitempty"`

	// Pagination 是分页元数据（§3）。
	Pagination *Pagination `json:"pagination,omitempty"`
}

// NewCount 返回 n 的指针，供 Meta.Count 使用，
// 以区分「count:0」（空结果，必须输出）与「count 缺席」（非列表场景）。
func NewCount(n int) *int { return &n }

// OperationInfo 是异步操作元数据（契约规范 §2.2）。
// 轮询超时必须保持 State 真实值并置 TimedOut:true，禁止伪装 success；
// NextCommand 必须是可直接执行的完整命令（恢复方式）。
type OperationInfo struct {
	ID          string `json:"id,omitempty"`
	State       string `json:"state,omitempty"`
	TimedOut    bool   `json:"timed_out,omitempty"`
	NextCommand string `json:"next_command,omitempty"`
}

// Pagination 是分页元数据（契约规范 §3）。
// EndpointExhausted 仅表示「观察到服务端分页耗尽」，不承诺索引健康或
// 数据全覆盖；弃用 complete 键。EndpointExhausted:false 必须携带
// NextToken（可续跑）——两者合起来即全部状态，不设 stop_reason。
// EndpointExhausted 不加 omitempty：true/false 两态都必须在 wire 上显式出现。
type Pagination struct {
	EndpointExhausted bool   `json:"endpoint_exhausted"`
	Pages             int    `json:"pages,omitempty"`
	Items             int    `json:"items,omitempty"`
	NextToken         string `json:"next_token,omitempty"`
}

// ErrorInfo 是失败信封的错误明细（契约规范 §2.4）。
// Type/Subtype/Code/Retryable/RetryAfterSeconds 为 wire-stable（Agent 可分支）；
// Message/Hint/RequestID 为 informational（禁止分支）。
type ErrorInfo struct {
	Type         string `json:"type,omitempty"`
	Subtype      string `json:"subtype,omitempty"`
	ExitCode     int    `json:"exit_code,omitempty"`
	HTTPStatus   int    `json:"http_status,omitempty"`
	UpstreamCode any    `json:"upstream_code,omitempty"`
	RPCCode      int    `json:"rpc_code,omitempty"`
	Code         int    `json:"code,omitempty"`
	Message      string `json:"message,omitempty"`
	Hint         string `json:"hint,omitempty"`

	// Retryable 仅为 true 时出现在 wire（§2.4；重试设计 §2.1）：
	// bool 零值 false 遇 omitempty 即字段缺席。CLI 不自动重放。
	Retryable bool `json:"retryable,omitempty"`

	// RetryAfterSeconds 是服务端给出的重试等待秒数。指针类型保留
	// 零值语义：0 秒有意义（nil 缺席、&0 序列化为 0），负值应在
	// 构造侧拒绝。
	RetryAfterSeconds *int64 `json:"retry_after_seconds,omitempty"`

	RequestID        string         `json:"request_id,omitempty"`
	TraceID          string         `json:"trace_id,omitempty"`
	Operation        string         `json:"operation,omitempty"`
	ServerKey        string         `json:"server_key,omitempty"`
	Origin           string         `json:"origin,omitempty"`
	Stage            string         `json:"stage,omitempty"`
	ExecutionStarted *bool          `json:"execution_started,omitempty"`
	NextRetryAt      string         `json:"next_retry_at,omitempty"`
	AvailableFlags   []string       `json:"available_flags,omitempty"`
	SnapshotPath     string         `json:"snapshot_path,omitempty"`
	Details          map[string]any `json:"details,omitempty"`
	RPCData          any            `json:"rpc_data,omitempty"`
	TechnicalDetail  string         `json:"technical_detail,omitempty"`
	FriendlyHint     string         `json:"friendly_hint,omitempty"`
	ActionURL        string         `json:"action_url,omitempty"`
	Cause            string         `json:"cause,omitempty"`

	// Param/Params 在校验错误时指明违规参数（§2.4）。
	Param  string   `json:"param,omitempty"`
	Params []string `json:"params,omitempty"`

	// Actions 是可执行的补救命令（门禁拦截 subtype=confirmation_required
	// 时携带，含 --yes 版本；§2.4）。
	Actions []string `json:"actions,omitempty"`
}

// ValidateDataErrorExclusivity 校验 wire 层面的 data/error 互斥（契约规范
// §1 不变量 I3 的字段级推论；§2.3/§2.4 形态）：error 非空时 data 必须缺席——
// 失败信封只承载 error 明细，部分成功明细走 data 且不携带顶层 error。
// 无违反返回 nil；聚合校验入口（B18 Envelope.Validate）将组合本检查。
func (e *Envelope) ValidateDataErrorExclusivity() error {
	if e == nil {
		return nil
	}
	if e.Error != nil && e.Data != nil {
		return fmt.Errorf("output: envelope violates invariant I3: data and error are mutually exclusive, data must be absent when error is present")
	}
	return nil
}

// Validate 是信封的聚合校验入口（B18，契约规范 §1）：一次性收集全部违反项
// （errors.Join）而非命中即返回，便于装配侧一轮定位所有契约缺陷。校验项：
//
//   - outcome 必须是四个规范值之一（§1/§2.5，复用 ParseOutcome）；
//   - 不变量 I1：OK == (outcome ∈ {success, pending})——框架计算值一致性；
//   - 不变量 I3：Error 非空 ⇔ outcome == failure（双向）；
//   - I3 字段级推论：data/error 互斥（复用 ValidateDataErrorExclusivity）；
//   - Data 为类型化 *PartialData 时的三通道纪律（对账、非空 succeeded、
//     id 可标识与跨通道互斥，§2.3；裸 map 等业务载荷形态不做解释）。
//
// outcome 非法时其派生检查（I1/I3）失去判定基准，仅报告 outcome 本身。
// Validate 只做静态字段校验，不解释业务载荷内容（L3 真实性归产品命令，§9）。
// nil 信封返回 nil：wire 出口（WriteEnvelopeTo）对 nil 已有降级 failure
// 信封的兜底，类型层不重复拦截。
func (e *Envelope) Validate() error {
	if e == nil {
		return nil
	}
	outcome, err := ParseOutcome(string(e.Outcome))
	if err != nil {
		return err
	}
	var errs []error
	wantOK := outcome == OutcomeSuccess || outcome == OutcomePending
	if e.OK != wantOK {
		errs = append(errs, fmt.Errorf("output: envelope violates invariant I1: ok=%v but outcome=%q requires ok=%v", e.OK, outcome, wantOK))
	}
	errorPresent := e.Error != nil
	isFailure := outcome == OutcomeFailure
	if errorPresent != isFailure {
		errs = append(errs, fmt.Errorf("output: envelope violates invariant I3: error present=%v but outcome=%q (error must be present if and only if outcome is failure)", errorPresent, outcome))
	}
	if err := e.ValidateDataErrorExclusivity(); err != nil {
		errs = append(errs, err)
	}
	if pd, ok := e.Data.(*PartialData); ok && pd != nil {
		if err := pd.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	if e.Meta != nil && e.Meta.Operation != nil {
		if err := e.Meta.Operation.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	if e.Meta != nil && e.Meta.Pagination != nil {
		if err := e.Meta.Pagination.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	if e.Error != nil {
		if err := e.Error.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *Pagination) Validate() error {
	if p == nil {
		return nil
	}
	if p.EndpointExhausted && strings.TrimSpace(p.NextToken) != "" {
		return fmt.Errorf("output: exhausted pagination must not carry next_token")
	}
	if !p.EndpointExhausted && strings.TrimSpace(p.NextToken) == "" {
		return fmt.Errorf("output: non-exhausted pagination requires next_token")
	}
	return nil
}

func (e *ErrorInfo) Validate() error {
	if e == nil {
		return fmt.Errorf("output: failure error is nil")
	}
	errorType := strings.TrimSpace(e.Type)
	if errorType == "" {
		return fmt.Errorf("output: failure error.type is required")
	}
	// error.type is a wire-stable Agent branch key, not an open-ended label.
	// Keep this set aligned with exitCodeForErrorInfo. "permission" is the
	// compatibility projection for PAT failures (rc=4).
	switch errorType {
	case "api", "auth", "validation", "permission", "discovery", "internal":
	default:
		return fmt.Errorf("output: unsupported failure error.type %q", e.Type)
	}
	if e.ExitCode < 0 {
		return fmt.Errorf("output: failure error.exit_code must not be negative")
	}
	if e.RetryAfterSeconds != nil && *e.RetryAfterSeconds < 0 {
		return fmt.Errorf("output: failure retry_after_seconds must be non-negative")
	}
	return nil
}

// NewSuccessEnvelope 构造成功信封（B19，契约规范 §2.1）。
// OK 由框架按 outcome 计算（I1），调用层不得自行改写。data 必须是强类型
// DTO 序列化产物，禁止手拼 JSON（§2.5/§7）；空数组 [] 是合法载荷（AC-06）。
func NewSuccessEnvelope(data any) *Envelope {
	return &Envelope{OK: true, Outcome: OutcomeSuccess, Data: data}
}

// NewPendingEnvelope 构造异步受理信封（B19，契约规范 §2.2）。
// op 携带 id/state/timed_out/next_command；op 为 nil 时 meta 整体缺席。
// 轮询超时必须保持 State 真实值并置 TimedOut:true，禁止伪装 success；
// NextCommand 必须是可直接执行的完整命令（§2.2 规则）。
//
// 本构造器保持「构造宽松 + 校验严格」分层（轮6裁决⑨）：§2.2 两条硬规则
// 不在构造期拦截，由 OperationInfo.Validate 与严格装配助手
// NewPendingEnvelopeForPolling（B147/B148）把守。
func NewPendingEnvelope(op *OperationInfo) *Envelope {
	env := &Envelope{OK: true, Outcome: OutcomePending}
	if op != nil {
		env.Meta = &Meta{Operation: op}
	}
	return env
}

// NewFailureEnvelope 构造失败信封（B20，契约规范 §2.4）。
// error 非空 ⇔ outcome==failure（I3）：info 为 nil 的失败信封无法通过
// Validate()，调用侧不得提交；data 必须缺席（I3 字段级推论）。
func NewFailureEnvelope(info *ErrorInfo) *Envelope {
	return &Envelope{OK: false, Outcome: OutcomeFailure, Error: info}
}

// NewPartialEnvelope 构造部分成功信封（B20，契约规范 §2.3）。
// 三通道明细（total/succeeded/failed/unknown）进 data，不携带顶层 error。
// 通道规则（B140/B141/B144 已落盘）：推荐以 NewPartialData 装配明细——
// 构造期拒绝空 succeeded（全失败禁用 partial_failure）；Envelope.Validate
// 对 *PartialData 载荷追加通道校验（同 id 跨通道互斥、total 对账等）。
func NewPartialEnvelope(data any) *Envelope {
	return &Envelope{OK: false, Outcome: OutcomePartialFailure, Data: data}
}

// IsOK 计算不变量 I1（B21，契约规范 §1）：ok == (outcome ∈ {success, pending})。
// 这是框架侧的唯一计算函数，命令层不可写或改写该结果；Envelope.OK 字段值
// 必须与 IsOK 一致（Validate 交叉校验，违反即 I1 缺陷）。
// nil 信封视为不 ok：没有结果可言，不得汇报为成功/受理。
// 非法（非四值）outcome 必然返回 false：I1 只定义在四个规范值上，
// 非法值由 ParseOutcome/Validate/MarshalJSON 各自拒绝。
func (e *Envelope) IsOK() bool {
	if e == nil {
		return false
	}
	return e.Outcome == OutcomeSuccess || e.Outcome == OutcomePending
}

// ---------------------------------------------------------------------------
// Phase G：partial_failure 三通道明细（B139~B146；契约规范 §2.3）
// ---------------------------------------------------------------------------

// PartialFailedEntry 是 partial 失败通道的单条明细（B139，契约规范 §2.3）：
// id 标识条目，Error 携带该条目的失败明细（复用 §2.4 ErrorInfo 形态，
// wire-stable 字段组可供 Agent 分支）。
type PartialFailedEntry struct {
	ID    string     `json:"id"`
	Error *ErrorInfo `json:"error,omitempty"`
}

// PartialUnknownEntry 是 partial unknown 通道的单条明细（B144，契约规范
// §2.3/P0#9「报错却写入」类问题的诚实通道）：shape = id + reason——
// 已提交但终态无法确认的条目放这里；reason 必须非空，说明不确定的原因。
type PartialUnknownEntry struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// PartialData 是部分成功三通道明细的类型化形态（B139，契约规范 §2.3），
// 整体放入 Envelope.Data。声明顺序 = wire 顺序：total/succeeded/failed/
// unknown（对齐契约示例）。三通道纪律（B140/B141/B144/B146）由
// NewPartialData 构造期校验 + Validate 集中复验：
//
//   - succeeded 必须完整保留（构造器不接受丢弃明细，§2.3 规则1）；
//   - unknown 条目禁止归入 succeeded/failed（同一 id 跨通道互斥，§2.3 规则2）；
//   - 空 succeeded 禁用 partial_failure（全失败必须用普通 failure，§2.3 规则4）。
//
// 三个切片字段不带 omitempty：空通道以 [] 显式表达（不输出 null，
// §2.5 无 null 纪律），与 data:[] 空态合法载荷同口径。
type PartialData struct {
	Total     int                   `json:"total"`
	Succeeded []any                 `json:"succeeded"`
	Failed    []PartialFailedEntry  `json:"failed"`
	Unknown   []PartialUnknownEntry `json:"unknown"`
}

// NewPartialData 装配三通道明细（B139/B140/B141/B144/B146，契约规范 §2.3）。
// 装配期即校验通道纪律：
//
//  1. succeeded 非空且原样保留——空 succeeded 的 partial_failure 被拒绝
//     （全部失败必须用 outcome:failure，§2.3 规则4；B141）；构造器不丢弃、
//     不过滤任何 succeeded 条目（B146：Agent 需要知道哪些已生效，避免重复提交）；
//  2. failed/unknown 条目 id 必须非空（条目可标识是跨通道互斥的唯一依据）；
//     unknown 条目 reason 必须非空（诚实通道必须说明不确定原因，B144）；
//  3. 同一 id 不得跨通道或同通道重复出现——一个条目只有一个终态通道
//     （「unknown 禁止归入 succeeded/failed」，§2.3 规则2；B140）。succeeded
//     条目是异构业务 DTO，其 id 按 JSON 泛化形态的 "id" 键提取；无 id 或非
//     object 的条目不参与互斥判定（框架不解释业务载荷内容，§9）；
//  4. total 对账：total == len(succeeded)+len(failed)+len(unknown)——
//     三通道明细是 total 的完整划分，不允许出现无明细的条目。
//
// nil 的 failed/unknown 入参归一为空切片（空通道 wire 上是 [] 而非 null）。
func NewPartialData(total int, succeeded []any, failed []PartialFailedEntry, unknown []PartialUnknownEntry) (*PartialData, error) {
	if failed == nil {
		failed = []PartialFailedEntry{}
	}
	if unknown == nil {
		unknown = []PartialUnknownEntry{}
	}
	d := &PartialData{Total: total, Succeeded: succeeded, Failed: failed, Unknown: unknown}
	return d, d.Validate()
}

// Validate 校验三通道纪律（B140）。NewPartialData 构造期已校验；本方法供
// 手工装配形态（绕过构造器）集中复验，并由 Envelope.Validate 在
// outcome==partial_failure 且 Data 为 *PartialData 时聚合调用。
func (d *PartialData) Validate() error {
	if d == nil {
		return nil
	}
	var errs []error
	if d.Succeeded == nil || d.Failed == nil || d.Unknown == nil {
		errs = append(errs, fmt.Errorf("output: partial channels must serialize as [] not null (§2.5 no-null discipline)"))
	}
	// B141：全失败禁用 partial_failure（§2.3 规则4）。
	if len(d.Succeeded) == 0 {
		errs = append(errs, fmt.Errorf("output: partial_failure with empty succeeded is forbidden: all-failed batches must use outcome:failure (§2.3)"))
	}
	// B146：succeeded 完整保留——不接受 nil（null）条目混入明细。
	for i, item := range d.Succeeded {
		if item == nil {
			errs = append(errs, fmt.Errorf("output: partial succeeded[%d] is nil: succeeded details must be fully preserved entries (§2.3)", i))
		}
	}
	for i, entry := range d.Failed {
		if strings.TrimSpace(entry.ID) == "" {
			errs = append(errs, fmt.Errorf("output: partial failed[%d] has empty id: entries must be identifiable (§2.3)", i))
		}
		if entry.Error == nil {
			errs = append(errs, fmt.Errorf("output: partial failed[%d] requires a typed error (§2.3)", i))
		} else if err := entry.Error.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("output: partial failed[%d] has invalid typed error: %w", i, err))
		}
	}
	// B144：unknown shape = id + reason，两者都必须非空。
	for i, entry := range d.Unknown {
		if strings.TrimSpace(entry.ID) == "" {
			errs = append(errs, fmt.Errorf("output: partial unknown[%d] has empty id: entries must be identifiable (§2.3)", i))
		}
		if strings.TrimSpace(entry.Reason) == "" {
			errs = append(errs, fmt.Errorf("output: partial unknown[%d] has empty reason: the honest channel must state why the terminal state is unconfirmed (§2.3/P0#9)", i))
		}
	}
	// B141/§2.3：total 对账——三通道明细是 total 的完整划分。
	if sum := len(d.Succeeded) + len(d.Failed) + len(d.Unknown); d.Total != sum {
		errs = append(errs, fmt.Errorf("output: partial total=%d does not reconcile with channel details (succeeded %d + failed %d + unknown %d = %d) (§2.3)",
			d.Total, len(d.Succeeded), len(d.Failed), len(d.Unknown), sum))
	}
	// B140：同一 id 跨通道/同通道互斥（unknown 不得归入 succeeded/failed）。
	seen := make(map[string]string)
	register := func(id, channel string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if prev, dup := seen[id]; dup {
			errs = append(errs, fmt.Errorf("output: partial entry id %q appears in both %s and %s: one entry has exactly one terminal channel (§2.3)", id, prev, channel))
			return
		}
		seen[id] = channel
	}
	for _, item := range d.Succeeded {
		register(partialEntryID(item), "succeeded")
	}
	for _, entry := range d.Failed {
		register(entry.ID, "failed")
	}
	for _, entry := range d.Unknown {
		register(entry.ID, "unknown")
	}
	return errors.Join(errs...)
}

// partialEntryID 从异构 succeeded 条目中提取 "id"（JSON 泛化形态的字符串键）。
// 无 id、id 非字符串或非 object 的条目返回空串（不参与互斥判定）。
func partialEntryID(item any) string {
	generic, ok := toGeneric(item).(map[string]any)
	if !ok {
		return ""
	}
	id, _ := generic["id"].(string)
	return id
}

// ---------------------------------------------------------------------------
// Phase H：operation / pagination 装配助手（B147~B152；契约规范 §2.2/§3）
// ---------------------------------------------------------------------------

// operation.state 取值表（B150，契约规范 §2.2）：state 描述异步操作所处阶段，
// 规范值为以下集合（开放枚举——业务特化阶段容忍，但以下三值是 Agent 必须能
// 分支的最小集）。timed_out:true 禁止与完成态共存（§2.2 规则1，见
// OperationInfo.ValidateTimeoutState）。
const (
	OperationStateProcessing = "processing"
	OperationStateCompleted  = "completed"
	OperationStateFailed     = "failed"
)

// ValidateNextCommand 校验 next_command 的可执行形态（B148，契约规范 §2.2
// 规则2「next_command 必须是可直接执行的完整命令」）：非空时必须是无前后
// 空白、不含换行的单行命令串——这是框架侧对「可直接执行」的形态代理；
// 命令真实可执行性（命令存在、参数有效）归装配侧保证（§9）。
// 空串在此不拦截：omitempty wire 允许无 operation 明细的裸 pending 信封，
// 轮询形态装配 NewPendingEnvelopeForPolling 才要求非空。
func (op *OperationInfo) ValidateNextCommand() error {
	if op == nil {
		return nil
	}
	if op.NextCommand == "" {
		return nil
	}
	if strings.TrimSpace(op.NextCommand) != op.NextCommand {
		return fmt.Errorf("output: operation.next_command %q must carry no leading/trailing whitespace (§2.2: directly executable)", op.NextCommand)
	}
	if strings.ContainsAny(op.NextCommand, "\n\r") {
		return fmt.Errorf("output: operation.next_command must be a single executable command line (§2.2)")
	}
	return nil
}

// ValidateTimeoutState 执行 §2.2 规则1 的防伪装校验（B149）：轮询超时必须
// 保持 state 真实值并置 timed_out:true——TimedOut 时 State 必须非空（真实值
// 必报），且不得宣称任何完成态（completed/success）：超时操作宣称完成态
// 即伪装 success（对齐 lark wiki_node_delete 教训）。
func (op *OperationInfo) ValidateTimeoutState() error {
	if op == nil || !op.TimedOut {
		return nil
	}
	state := strings.TrimSpace(op.State)
	if state == "" {
		return fmt.Errorf("output: operation.timed_out=true requires the real state to be reported (empty state, §2.2 anti-spoof)")
	}
	if state == OperationStateCompleted || strings.EqualFold(state, string(OutcomeSuccess)) {
		return fmt.Errorf("output: operation.timed_out=true must keep the real state %q: a timed-out operation must not claim a completed state (§2.2 anti-spoof)", state)
	}
	return nil
}

// Validate 聚合 operation 的 §2.2 形态校验。必填字段由新框架的
// CommandResult 边界校验；这里保持旧 Envelope 构造 API 的兼容性。
func (op *OperationInfo) Validate() error {
	if op == nil {
		return nil
	}
	return errors.Join(op.ValidateNextCommand(), op.ValidateTimeoutState())
}

// NewPendingEnvelopeForPolling 装配轮询形态的 pending 信封（B147 增强，
// 契约规范 §2.2）：比 NewPendingEnvelope 更严格——轮询形态的恢复方式只有
// next_command 一条出口，故 op 与 next_command 必须非空且通过可执行形态
// 校验；state 必须遵守 timed_out 防伪装规则。返回 I1 一致（ok:true/
// outcome:pending）的信封；装配失败返回 error（信封为 nil）。
func NewPendingEnvelopeForPolling(op *OperationInfo) (*Envelope, error) {
	if op == nil {
		return nil, fmt.Errorf("output: polling-form pending envelope requires operation (§2.2: meta.operation is the recovery carrier)")
	}
	if strings.TrimSpace(op.NextCommand) == "" {
		return nil, fmt.Errorf("output: polling-form pending envelope requires a non-empty, directly executable next_command (§2.2 rule 2)")
	}
	if err := op.Validate(); err != nil {
		return nil, err
	}
	return NewPendingEnvelope(op), nil
}

// NewPagination 装配分页元数据（B152，契约规范 §3）：两态完备校验——
// endpoint_exhausted:false 必须携带 next_token（可续跑：无 token 则
// 「可续跑」不可表达，破坏 §3 两态完备性）；endpoint_exhausted:true
// 不得携带 next_token（耗尽即无续跑凭据，两者合起来即全部状态，
// 不设 stop_reason）。Pages/Items 为信息性计数（omitempty），由调用方
// 按观察到的页数/条数在返回结构上另行设置。
func NewPagination(endpointExhausted bool, nextToken string) (*Pagination, error) {
	token := strings.TrimSpace(nextToken)
	if !endpointExhausted && token == "" {
		return nil, fmt.Errorf("output: pagination endpoint_exhausted=false requires next_token (§3: resumable state must carry the resume handle)")
	}
	if endpointExhausted && token != "" {
		return nil, fmt.Errorf("output: pagination endpoint_exhausted=true must not carry next_token (§3: exhausted and resumable are the only two states)")
	}
	pg := &Pagination{EndpointExhausted: endpointExhausted, NextToken: token}
	return pg, nil
}
