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
	"bytes"
	"fmt"
	"io"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// WriteEnvelope 是统一信封的出口函数（B0 dev 试点桥接，WS1 改动点2/4）。
// 它复用 envelope.go 的权威 Envelope 类型，把信封按当前 --format 分发写出。
// Phase B 的 Emitter 类型将在此之上泛化（渲染先进 buffer、exit code 映射等）；
// 本函数仅提供 Phase F dev 域迁移所需的「信封 → cmd 流」出口，不重复定义类型。
//
// format 分发（契约规范 §5.2，经 renderEnvelope 实现）：
//   - json（默认）：输出完整信封——唯一 JSON 契约（§5.2 首行）。
//   - table/pretty：业务数据人读视图 + meta 摘要（不输出信封外壳）。
//   - csv/ndjson：裸记录（无信封）；meta.pagination 走 stderr 诊断行。
//   - raw：数据载荷透传（与 Write 的 raw 语义一致）。
//
// 数据出口一律写 cmd.OutOrStdout()、诊断出口写 cmd.ErrOrStderr()（均可被
// 测试重定向），不硬编码 os.Stdout/os.Stderr。未知 --format 值按 §5.2/AC-09
// 降级 fallback 并在此向 stderr 写一条 warning（不崩不静默）。
func WriteEnvelope(cmd *cobra.Command, env *Envelope, fallback Format) error {
	w, errW := io.Writer(io.Discard), io.Writer(io.Discard)
	format := fallback
	fields, jq, warning := "", "", ""
	if cmd != nil {
		w = cmd.OutOrStdout()
		errW = cmd.ErrOrStderr()
		format, warning = resolveFormatWithWarning(cmd, fallback)
		fields = ResolveFields(cmd)
		jq = ResolveJQ(cmd)
	}
	if env == nil {
		env = nilFallbackEnvelope()
	}
	if err := renderEnvelope(w, errW, env, format, fields, jq); err != nil {
		return err
	}
	// AC-09「不静默」：warning 仅在信封成功写出后补写 stderr——
	// 写出失败时错误优先，不留半截通道。
	if warning != "" {
		if _, werr := fmt.Fprintln(errW, "[WARN] "+warning); werr != nil {
			return werr
		}
	}
	return nil
}

// WriteEnvelopeTo 是 WriteEnvelope 的 writer 级实现（签名自 B205 保持不变），
// 供不持有 *cobra.Command 的调用方与单测直接使用。B37 起收编到 renderEnvelope
// 统一分发：json/空 format 输出完整信封（唯一 JSON 契约）；table/pretty 输出
// 业务数据人读视图 + meta 摘要；csv/ndjson 输出裸记录（分页诊断行在本签名下
// 无 stderr 出口可走，按 io.Discard 处理——需要诊断行的调用方走 WriteEnvelope
// 的 cmd 路径或 Emitter）。nil 信封降级为 I3 合法的 failure 兜底信封（B208）：
// 携带 internal 类 ErrorInfo——框架最后的兜底不得产出被自身 Validate() 拒绝的形态。
func WriteEnvelopeTo(w io.Writer, env *Envelope, format Format, fields, jq string) error {
	if env == nil {
		env = nilFallbackEnvelope()
	}
	return renderEnvelope(w, io.Discard, env, format, fields, jq)
}

// EmitResult is the framework 2.0 CLI adapter. Unlike the legacy Emitter,
// machine-readable JSON/NDJSON always writes the primary result to stdout for
// every outcome; stderr is diagnostics-only. Human failure formats retain the
// conventional stderr path. The result is validated before any byte is
// rendered, and the returned exit code comes from the same immutable result.
func EmitResult(cmd *cobra.Command, result CommandResult) (int, error) {
	code, _, err := emitResult(cmd, result)
	return code, err
}

func emitResult(cmd *cobra.Command, result CommandResult) (int, bool, error) {
	if err := ValidateResult(result); err != nil {
		return exitCodeInternal, false, err
	}
	env := result.envelope()
	env = redactEnvelope(env)
	format, warning := resolveFormatWithWarning(cmd, FormatJSON)
	fields, jq := ResolveFields(cmd), ResolveJQ(cmd)
	stdout, stderr := io.Writer(io.Discard), io.Writer(io.Discard)
	if cmd != nil {
		stdout, stderr = cmd.OutOrStdout(), cmd.ErrOrStderr()
	}

	if env.Outcome == OutcomeFailure && format != FormatJSON && format != FormatNDJSON {
		message := "command failed"
		if env.Error != nil && strings.TrimSpace(env.Error.Message) != "" {
			message = env.Error.Message
		}
		var buf bytes.Buffer
		_, _ = fmt.Fprintln(&buf, "Error: "+message)
		if env.Error != nil {
			if hint := strings.TrimSpace(env.Error.Hint); hint != "" {
				_, _ = fmt.Fprintln(&buf, "Hint: "+hint)
			}
			if hint := strings.TrimSpace(env.Error.FriendlyHint); hint != "" {
				_, _ = fmt.Fprintln(&buf, "Hint: "+hint)
			}
			if actionURL := strings.TrimSpace(env.Error.ActionURL); actionURL != "" {
				_, _ = fmt.Fprintln(&buf, "Action: 处理入口: "+actionURL)
			}
			for _, action := range env.Error.Actions {
				action = strings.TrimSpace(action)
				if action == "" {
					continue
				}
				if action == apperrors.DoctorCommand {
					action = apperrors.DoctorHumanCommand
				}
				_, _ = fmt.Fprintln(&buf, "Action: "+action)
			}
		}
		n, err := writeAllCount(stderr, buf.Bytes())
		if err != nil {
			return exitCodeInternal, n > 0, err
		}
		return result.ExitCode(), n > 0, nil
	}

	// Failure bypasses jq/fields so a filter cannot erase the typed error.
	if env.Outcome == OutcomeFailure {
		fields, jq = "", ""
	}
	var buf bytes.Buffer
	if err := renderEnvelope(&buf, stderr, env, format, fields, jq); err != nil {
		return exitCodeInternal, false, err
	}
	n, err := writeAllCount(stdout, buf.Bytes())
	if err != nil {
		return exitCodeInternal, n > 0, err
	}
	if warning != "" {
		// The primary result is already complete. Diagnostics are best-effort
		// and must not reverse its outcome when stderr is unavailable.
		_, _ = fmt.Fprintln(stderr, "[WARN] "+warning)
	}
	return result.ExitCode(), n > 0, nil
}

// nilFallbackEnvelope 构造 nil 信封的兜底信封（B208，轮6裁决⑨）。保留
// 「不 panic、必产合法 failure」的既有语义，且自身满足不变量 I3
// （error 非空 ⇔ outcome==failure）、通过 Envelope.Validate()——框架出口
// 兜底不得被自己的校验器判非法。Type 取 "internal"：这是框架内部缺陷而非
// 业务错误（§4 internal=5），Agent 可据 wire-stable 字段分支处置。
func nilFallbackEnvelope() *Envelope {
	return &Envelope{
		OK:      false,
		Outcome: OutcomeFailure,
		Error: &ErrorInfo{
			Type:    "internal",
			Message: "nil envelope: framework fallback",
		},
	}
}

// ---------------------------------------------------------------------------
// format 分发（B35~B40）：信封 → format 矩阵的统一分发点
// （契约规范 §5.2；适用域仅 stdout 数据通道——轮8裁决⑪）
// ---------------------------------------------------------------------------

// renderEnvelope 是信封渲染的统一分发点（B37，WS1 改动点4；契约规范 §5.2
// format 矩阵）。渲染先进 buffer 再一次性写出：数据通道任何一步失败都不向
// w 泄漏半截输出（§5.2 末条；与 B25 buffer-first 同口径）。
//
// 矩阵各行（适用域仅 stdout 数据通道——失败信封在 Emitter 流路由层恒以
// 完整 JSON 写 stderr，从不进入本矩阵，轮8裁决⑪）：
//
//   - json / 空：完整信封——唯一 JSON 契约（§5.2 首行，B38）；
//   - table / pretty：业务数据人读视图 + meta 层人读摘要（count/pagination），
//     不输出信封外壳（§5.2 矩阵行，B39）；
//   - csv / ndjson：裸记录（无信封外壳）；meta.pagination 走 errW 的 stderr
//     诊断行，不污染裸记录机器可读流（§5.2 矩阵行，B40）；
//   - raw：数据载荷透传（writeRaw 语义）。
//
// --jq 优先于 format：对**完整信封**求值后直接输出结果（§5.2 矩阵行，B42）。
// --fields 先投影数据载荷再按 format 分发（成功通道联动）。errW 为 nil 时按
// io.Discard 处理（csv/ndjson 诊断行无出口即丢弃）。
//
// buffer-first 纪律：全部分发路径（含 json/jq）先渲染进内存 buffer，成功后
// 才一次性写出到 w（§5.2 末条对分发点整体成立）——渲染失败零泄漏、写出
// 失败原子中止、短写报 io.ErrShortWrite。csv/ndjson 的 stderr 诊断行不经
// buffer，且在数据渲染成功后才写（渲染失败不留诊断噪声）。
func renderEnvelope(w io.Writer, errW io.Writer, env *Envelope, format Format, fields, jq string) error {
	if errW == nil {
		errW = io.Discard
	}
	if env == nil {
		env = nilFallbackEnvelope()
	}
	env = redactEnvelope(env)
	var buf bytes.Buffer
	if err := renderEnvelopeInto(&buf, errW, env, format, fields, jq); err != nil {
		return err
	}
	return writeAll(w, buf.Bytes())
}

// renderEnvelopeInto 按 format 矩阵把信封渲染进 buf（renderEnvelope 的渲染段）。
func renderEnvelopeInto(buf *bytes.Buffer, errW io.Writer, env *Envelope, format Format, fields, jq string) error {
	// 失败信封不参与 format/fields/jq 分发（轮8裁决⑪；契约规范 §5.1 v1.1
	// 回写条款）：恒输出完整 JSON 信封。§5.2 矩阵适用域仅 stdout 数据通道，
	// 失败信封无 data 载荷（I3），若按矩阵分发或经 jq/fields 过滤，§2.4
	// wire-stable 字段组将从错误通道消失。
	if env.Outcome == OutcomeFailure {
		if format == FormatNDJSON {
			return Write(buf, FormatNDJSON, env)
		}
		return WriteFiltered(buf, FormatJSON, env, "", "")
	}
	if strings.TrimSpace(jq) != "" {
		// jq 优先于 format：对完整信封求值后直接输出（§5.2 矩阵行）。
		return ApplyJQ(buf, env, strings.TrimSpace(jq))
	}

	switch format {
	case FormatJSON, "":
		// json（默认）始终保留完整信封。--fields 只投影业务载荷，
		// ok/outcome/meta 等稳定契约字段仍必须可供 Agent 分支和续页。
		if trimmed := strings.TrimSpace(fields); trimmed != "" {
			projected := *env
			projected.Data = SelectFields(env.Data, strings.Split(trimmed, ","))
			return WriteFiltered(buf, FormatJSON, &projected, "", "")
		}
		return WriteFiltered(buf, FormatJSON, env, "", "")
	default:
	}

	payload := env.Data
	if trimmed := strings.TrimSpace(fields); trimmed != "" {
		payload = SelectFields(payload, strings.Split(trimmed, ","))
	}

	switch format {
	case FormatTable, FormatPretty:
		if err := Write(buf, format, payload); err != nil {
			return err
		}
		if summary := renderMetaSummary(env.Meta); summary != "" {
			_, _ = fmt.Fprintf(buf, "\n%s\n", summary)
		}
		return nil
	case FormatCSV, FormatNDJSON:
		if err := Write(buf, format, payload); err != nil {
			return err
		}
		// 分页元数据走 stderr 诊断行（§5.2 csv/ndjson 行）。partial_failure
		// 例外：§2.3 明确「stderr 零输出」，特例优先于 §5.2 诊断行通则。
		if env.Outcome != OutcomePartialFailure {
			return writePaginationDiagnostic(errW, env.Meta)
		}
		return nil
	case FormatRaw:
		return Write(buf, FormatRaw, payload)
	default:
		// 未知 Format 常量（归一化/ParseFormat 后理论不可达）：按 json
		// 数据通道语义兜底，绝不静默丢弃载荷。--fields 同样投影业务载荷。
		if trimmed := strings.TrimSpace(fields); trimmed != "" {
			return WriteFiltered(buf, FormatJSON, env.Data, trimmed, "")
		}
		return WriteFiltered(buf, FormatJSON, env, "", "")
	}
}

// writeAll 把 p 一次性写入 w；短写（接受了部分字节却未写全）报
// io.ErrShortWrite——与 Emitter.writeOnce 同一短写纪律，绝不把截断输出当成功。
func writeAll(w io.Writer, p []byte) error {
	_, err := writeAllCount(w, p)
	return err
}

func writeAllCount(w io.Writer, p []byte) (int, error) {
	n, err := w.Write(p)
	if err != nil {
		return n, err
	}
	if n < len(p) {
		return n, io.ErrShortWrite
	}
	return n, nil
}

// renderMetaSummary 把信封 meta 层渲染为人读摘要行（§5.2：table/pretty 输出
// 「业务数据 + 人读分页摘要」）。无 meta 内容返回空串。count 单独成行；
// pagination 摘要复用 renderPaginationSummary。
func renderMetaSummary(meta *Meta) string {
	if meta == nil {
		return ""
	}
	var lines []string
	if meta.Count != nil {
		lines = append(lines, fmt.Sprintf("count: %d", *meta.Count))
	}
	if meta.Pagination != nil {
		lines = append(lines, "pagination: "+renderPaginationSummary(meta.Pagination))
	}
	return strings.Join(lines, "\n")
}

// renderPaginationSummary 按 wire 字段名输出分页摘要：
// endpoint_exhausted/pages/items/next_token（snake_case 与 wire 一致，Agent
// 可交叉核对；§3 两态显式语义保留——false 也输出，因为它是「可续跑」的
// 有意义状态）。
func renderPaginationSummary(p *Pagination) string {
	parts := []string{fmt.Sprintf("endpoint_exhausted: %t", p.EndpointExhausted)}
	if p.Pages > 0 {
		parts = append(parts, fmt.Sprintf("pages: %d", p.Pages))
	}
	if p.Items > 0 {
		parts = append(parts, fmt.Sprintf("items: %d", p.Items))
	}
	if p.NextToken != "" {
		parts = append(parts, fmt.Sprintf("next_token: %s", p.NextToken))
	}
	return strings.Join(parts, ", ")
}

// writePaginationDiagnostic 把分页诊断行写到 errW（§5.2：csv/ndjson 裸记录
// 格式下分页元数据走 stderr 诊断行）。无 meta/pagination 时零输出——
// 不给无分页场景制造噪声。
func writePaginationDiagnostic(errW io.Writer, meta *Meta) error {
	if meta == nil || meta.Pagination == nil {
		return nil
	}
	_, err := fmt.Fprintln(errW, "[pagination] "+renderPaginationSummary(meta.Pagination))
	return err
}

// ---------------------------------------------------------------------------
// Phase B 泛化（B24~B29）：Emitter 类型——success/pending/partial 走 stdout、
// failure 走 stderr 的双向出口；渲染先进 buffer 再一次性写出。
// ---------------------------------------------------------------------------

// Emitter 是统一信封的双向出口（B24，WS1 改动点2）。它持有 stdout/stderr 两个
// 写入目标与当前 format/fields/jq 配置，按契约规范 §5.1 流向纪律把信封分发到
// 正确的流：
//
//   - success / pending / partial_failure → stdout（数据通道）；
//   - failure（含 nil 信封降级）→ stderr，stdout 严格零字节（AC-11）。
//
// 每次写出都经过「先渲染进内存 buffer、再一次性写出」（B25，契约规范 §5.2
// 末条），写一半失败不产生残缺 JSON。渲染复用 WriteEnvelopeTo（B205），
// Emitter 不重新定义信封类型或 wire 键。
type Emitter struct {
	stdout io.Writer
	stderr io.Writer
	format Format
	fields string
	jq     string
}

// NewEmitter 构造 Emitter（B24）。nil writer 按 io.Discard 处理（等价丢弃，
// 防 panic）。format 是解析后的当前 format（空值由 WriteEnvelopeTo 按 json
// 处理）；fields/jq 为命令解析出的 --fields/--jq 过滤条件。
func NewEmitter(stdout, stderr io.Writer, format Format, fields, jq string) *Emitter {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return &Emitter{stdout: stdout, stderr: stderr, format: format, fields: fields, jq: jq}
}

// Emit 按 env 的 outcome 把信封写到对应流（B26/B27/B28，契约规范 §5.1）：
//
//   - success / pending：完整信封落 stdout（B26，ok:true，§2.1/§2.2）；
//   - partial_failure：三通道明细落 stdout、stderr 零输出（B28，§2.3）；
//   - failure：信封落 stderr、stdout 严格零字节（B27，AC-11）；
//   - nil 信封由 WriteEnvelopeTo 降级为 failure，同样落 stderr。
//
// exit 0 语义标注（B32 收口，契约规范 §4/I2）：Emit 的流路由与
// ExitCodeForEnvelope 的退出码语义一一对应——落 stdout 的信封
// （success/pending/partial_failure 中 ok:true 的前两者）对应 exit 0，
// 落 stderr 的 failure（含 nil 降级）对应非零退出码。Emitter 只负责写出，
// 不自行 os.Exit；进程退出码由 runner 出口按同一映射统一决定（§4 规则）。
//
// 失败信封恒以完整 JSON 信封渲染并绕过 format/fields/jq：失败信封没有数据
// 载荷（I3），§5.1 要求 stderr 上的失败信封恒为结构化信封；「非 json 只渲染
// data」的分发规则只适用于携带数据的信封。若 --jq 作用于失败信封会把错误
// 通道过滤为空，Agent 将丢失错误明细，故同样绕过。
func (em *Emitter) Emit(env *Envelope) error {
	if env == nil || env.Outcome == OutcomeFailure {
		return em.writeOnce(em.stderr, env, FormatJSON, "", "")
	}
	return em.writeOnce(em.stdout, env, em.format, em.fields, em.jq)
}

// writeOnce 先把信封完整渲染进内存 buffer，再一次性写出到 target（B25，
// 契约规范 §5.2 末条「渲染先进 buffer 再一次性写出」）：渲染失败不向 target
// 泄漏任何字节；写出失败原子中止——writer 报错即原样返回，短写（接受了部分
// 字节却未写全）报 io.ErrShortWrite，绝不把截断输出当成功。渲染统一走
// renderEnvelope 分发点（B37），csv/ndjson 的分页诊断行（B40）以 em.stderr
// 为诊断出口。
func (em *Emitter) writeOnce(target io.Writer, env *Envelope, format Format, fields, jq string) error {
	if env == nil {
		env = nilFallbackEnvelope()
	}
	var buf bytes.Buffer
	if err := renderEnvelope(&buf, em.stderr, env, format, fields, jq); err != nil {
		return err
	}
	p := buf.Bytes()
	n, err := target.Write(p)
	if err != nil {
		return err
	}
	if n < len(p) {
		return io.ErrShortWrite
	}
	return nil
}

// ParseFormat 把字符串形式的 format 值（来自 --format flag 或 caller 接口）
// 归一化为 Format 常量；大小写不敏感，未知/空值返回 fallback。它是不持有
// *cobra.Command 的调用方（如主漏斗 callMCPToolInternalOpts）的 ResolveFormat
// 等价物，二者共用同一归一化规则。
func ParseFormat(raw string, fallback Format) Format {
	return normalizeFormat(raw, fallback)
}

// ---------------------------------------------------------------------------
// 退出码映射（B32/B33/B34）：outcome / error 类别 → exit code
// （契约规范 §4；不变量 I2 —— exit code == 0 ⇔ ok == true）
// ---------------------------------------------------------------------------

// 退出码表取值对齐：契约规范 §4 × 规划 v1.2 决策（OQ-1 已关闭——保留现行
// 码表 api=1 / auth=2 / validation=3 / PAT=4 / internal=5 / discovery=6，
// 仅新增 partial_failure 专用码 7；草案表中 validation→2 / confirmation→3
// 的拆分属 wire 破坏性重排，放弃）。confirmation_required 是 failure 的子类，
// 与 validation 共享 rc=3、以 subtype 区分（AC-13；runtime 同源于
// internal/errors 的 ExitCode 表）。
const (
	exitCodeOK         = 0
	exitCodeAPI        = 1
	exitCodeAuth       = 2
	exitCodeValidation = 3 // confirmation_required 子类共享此码（OQ-1 定案）
	exitCodePermission = 4 // PAT / permission failures
	exitCodeInternal   = 5
	exitCodeDiscovery  = 6
	exitCodePartial    = 7 // partial_failure 专用（契约 §4；规划 WS2 第4项；B142 将在 errors 侧补同源常量）
)

// subtypeConfirmationRequired 是门禁拦截的 failure 子类标记（契约规范 §2.4），
// 与 runtime apperrors.WithReason("confirmation_required") 形态同源。
const subtypeConfirmationRequired = "confirmation_required"

// ExitCodeForEnvelope 计算信封对应的进程退出码（B33，AC-10/I2；契约规范 §4）。
// 纯函数：只读信封字段、不做任何 I/O。映射规则：
//
//   - success / pending → 0（I2：exit 0 ⇔ ok；异步受理不是失败，§1/§4）；
//   - partial_failure → 7（部分成功专用码，区别于普通失败，§4）；
//   - failure → 按 Error.Type 类别映射，confirmation_required 子类恒 3；
//   - nil 信封 → 5（与 WriteEnvelopeTo 的 internal 类兜底信封同源）；
//   - 非法 outcome → 5（非法信封属框架缺陷，归 internal）。
//
// 判定只看 outcome / error 类别，不看可能被命令层篡改的 OK 字段（与 IsOK
// 同口径，I1）。退出码由 runner 出口统一决定、命令层禁止自行 os.Exit
// （§4 规则）；本函数为 Emitter 语义标注、runner 映射与测试提供同一依据。
func ExitCodeForEnvelope(env *Envelope) int {
	if env == nil {
		return exitCodeInternal
	}
	switch env.Outcome {
	case OutcomeSuccess, OutcomePending:
		return exitCodeOK
	case OutcomePartialFailure:
		return exitCodePartial
	case OutcomeFailure:
		return exitCodeForErrorInfo(env.Error)
	default:
		return exitCodeInternal
	}
}

// exitCodeForErrorInfo 把失败信封的 error 类别映射为退出码（契约规范 §4）。
// subtype=confirmation_required 优先于 type：门禁拦截无论挂在哪个 type 下
// 都恒为 3（AC-13）。error 缺席（违反 I3 的形态）按 internal 处理——
// 没有明细可分支的失败不得假装可恢复。未知 type 归 internal（不应发生）。
func exitCodeForErrorInfo(info *ErrorInfo) int {
	if info == nil {
		return exitCodeInternal
	}
	if info.Subtype == subtypeConfirmationRequired {
		return exitCodeValidation
	}
	switch info.Type {
	case "api":
		return exitCodeAPI
	case "auth":
		return exitCodeAuth
	case "validation":
		return exitCodeValidation
	case "permission":
		return exitCodePermission
	case "discovery":
		return exitCodeDiscovery
	default:
		return exitCodeInternal
	}
}
