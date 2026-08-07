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
	"encoding/json"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// phaseCListFixture 是三格式（table/csv/ndjson）共用的列表载荷：
// 3 条同构对象记录，配 meta.count 与 meta.pagination 以便同时断言
// 「人读摘要」与「分页诊断行」两条分发行为。
func phaseCListFixture() *Envelope {
	env := NewSuccessEnvelope([]any{
		map[string]any{"id": "a", "name": "alpha"},
		map[string]any{"id": "b", "name": "beta"},
		map[string]any{"id": "c", "name": "gamma"},
	})
	env.Meta = &Meta{
		Count: NewCount(3),
		Pagination: &Pagination{
			EndpointExhausted: false,
			Pages:             2,
			Items:             60,
			NextToken:         "cursor_xyz",
		},
	}
	return env
}

// --- B35：format 值归一化（trim/小写）——normalizeFormat 既有实现的补差断言 ---

func TestNormalizeFormatTrimAndCaseExhaustive(t *testing.T) {
	// 六格式 × （原始小写 / 大写 / 混合大小写 + 前后空白）全部命中对应常量；
	// 归一化规则 trim + 小写，与 ResolveFormat/ParseFormat 单一事实源。
	formats := []Format{FormatJSON, FormatTable, FormatRaw, FormatPretty, FormatNDJSON, FormatCSV}
	for _, want := range formats {
		base := string(want)
		for _, raw := range []string{
			base,
			strings.ToUpper(base),
			"  " + strings.ToUpper(base[:1]) + base[1:] + "\t",
		} {
			if got := normalizeFormat(raw, FormatJSON); got != want {
				t.Fatalf("normalizeFormat(%q) = %q, want %q", raw, got, want)
			}
			// ParseFormat 薄包装与 normalizeFormat 结果恒一致（单一事实源）。
			if got := ParseFormat(raw, FormatJSON); got != want {
				t.Fatalf("ParseFormat(%q) = %q, want %q", raw, got, want)
			}
		}
	}
	// 空串 = 未指定 → fallback；未知值 → fallback（B36 降级的归一化前提）。
	if got := normalizeFormat("", FormatTable); got != FormatTable {
		t.Fatalf("normalizeFormat empty = %q, want fallback table", got)
	}
	if got := normalizeFormat("  \t ", FormatCSV); got != FormatCSV {
		t.Fatalf("normalizeFormat whitespace-only = %q, want fallback csv", got)
	}
	if got := normalizeFormat("yaml", FormatJSON); got != FormatJSON {
		t.Fatalf("normalizeFormat unknown = %q, want fallback json", got)
	}
}

// --- B36：未知 format 降级 + stderr warning（不崩不静默，AC-09）---

func TestResolveFormatWithWarningKnownValuesSilent(t *testing.T) {
	formats := []Format{FormatJSON, FormatTable, FormatRaw, FormatPretty, FormatNDJSON, FormatCSV}
	for _, want := range formats {
		cmd := &cobra.Command{Use: "t"}
		cmd.Flags().String("format", "", "")
		if err := cmd.Flags().Set("format", string(want)); err != nil {
			t.Fatalf("set format: %v", err)
		}
		got, warning := resolveFormatWithWarning(cmd, FormatJSON)
		if got != want {
			t.Fatalf("resolveFormatWithWarning(%q) = %q, want %q", want, got, want)
		}
		if warning != "" {
			t.Fatalf("known format %q must not warn, got %q", want, warning)
		}
	}
}

func TestResolveFormatWithWarningUnknownDegradesWithWarning(t *testing.T) {
	for _, fallback := range []Format{FormatJSON, FormatTable} {
		cmd := &cobra.Command{Use: "t"}
		cmd.Flags().String("format", "", "")
		if err := cmd.Flags().Set("format", "yaml"); err != nil {
			t.Fatalf("set format: %v", err)
		}
		got, warning := resolveFormatWithWarning(cmd, fallback)
		if got != fallback {
			t.Fatalf("unknown format must degrade to fallback %q, got %q", fallback, got)
		}
		if warning == "" {
			t.Fatal("unknown format must produce a warning (AC-09: no silent swallow)")
		}
		if !strings.Contains(warning, "yaml") || !strings.Contains(warning, string(fallback)) {
			t.Fatalf("warning must name the unknown value and the fallback target: %q", warning)
		}
	}
}

func TestResolveFormatWithWarningEmptyAndMissingSilent(t *testing.T) {
	// 空值 = 未指定（走 fallback 是正常默认路径），flag 缺席同：均不报警。
	cmd := &cobra.Command{Use: "t"}
	cmd.Flags().String("format", "", "")
	if got, warning := resolveFormatWithWarning(cmd, FormatRaw); got != FormatRaw || warning != "" {
		t.Fatalf("empty format = (%q, %q), want (raw, silent)", got, warning)
	}
	cmd2 := &cobra.Command{Use: "t2"}
	if got, warning := resolveFormatWithWarning(cmd2, FormatRaw); got != FormatRaw || warning != "" {
		t.Fatalf("missing flag = (%q, %q), want (raw, silent)", got, warning)
	}
	if got, warning := resolveFormatWithWarning(nil, FormatJSON); got != FormatJSON || warning != "" {
		t.Fatalf("nil cmd = (%q, %q), want (json, silent)", got, warning)
	}
}

func TestResolveFormatKeepsWarningFreeReturnContract(t *testing.T) {
	// ResolveFormat（既有导出 API）行为不变：只返回 format，未知值静默降级。
	cmd := &cobra.Command{Use: "t"}
	cmd.Flags().String("format", "", "")
	if err := cmd.Flags().Set("format", "bogus"); err != nil {
		t.Fatalf("set format: %v", err)
	}
	if got := ResolveFormat(cmd, FormatJSON); got != FormatJSON {
		t.Fatalf("ResolveFormat unknown = %q, want json fallback", got)
	}
}

func TestWriteEnvelopeUnknownFormatDegradesToJSONWithStderrWarning(t *testing.T) {
	// 端到端：未知 --format 不崩不静默——stdout 仍为完整 json 信封（降级
	// 生效，数据不丢），stderr 恰有一条 warning 点名未知值与降级目标。
	cmd := &cobra.Command{Use: "t"}
	cmd.Flags().String("format", "", "")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Flags().Set("format", "yaml"); err != nil {
		t.Fatalf("set format: %v", err)
	}

	env := NewSuccessEnvelope(map[string]any{"name": "DemoApp"})
	if err := WriteEnvelope(cmd, env, FormatJSON); err != nil {
		t.Fatalf("WriteEnvelope with unknown format must not crash: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout must degrade to a full json envelope: %v\n%s", err, stdout.String())
	}
	if decoded["outcome"] != "success" {
		t.Fatalf("degraded envelope outcome = %v, want success: %s", decoded["outcome"], stdout.String())
	}
	data, _ := decoded["data"].(map[string]any)
	if data == nil || data["name"] != "DemoApp" {
		t.Fatalf("degraded envelope lost data payload: %s", stdout.String())
	}
	if got := strings.Count(stderr.String(), "[WARN]"); got != 1 {
		t.Fatalf("stderr must carry exactly one warning line, got %d: %s", got, stderr.String())
	}
	if !strings.Contains(stderr.String(), "yaml") || !strings.Contains(stderr.String(), "json") {
		t.Fatalf("warning must name unknown value and fallback: %s", stderr.String())
	}
}

func TestWriteEnvelopeKnownFormatLeavesStderrSilent(t *testing.T) {
	// 已知 format 不产生 warning（stderr 零字节），防 B36 误伤正常路径。
	cmd := &cobra.Command{Use: "t"}
	cmd.Flags().String("format", "", "")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Flags().Set("format", "table"); err != nil {
		t.Fatalf("set format: %v", err)
	}
	if err := WriteEnvelope(cmd, NewSuccessEnvelope(map[string]any{"name": "x"}), FormatJSON); err != nil {
		t.Fatalf("WriteEnvelope: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("known format must not warn, stderr = %d bytes: %s", stderr.Len(), stderr.String())
	}
}

// --- B37：信封渲染接统一 format 分发点（WS1 改动点4）---

func TestRenderEnvelopeSingleDispatchPointEmitterAndWriterAgree(t *testing.T) {
	// 分发点唯一性：Emitter 路径与 WriteEnvelopeTo 路径对同一信封 + 同一
	// format 逐字节一致——两条出口共用 renderEnvelope，不存在第二套渲染。
	formats := []Format{FormatJSON, FormatTable, FormatPretty, FormatCSV, FormatNDJSON, FormatRaw}
	for _, format := range formats {
		env := phaseCListFixture()
		var direct bytes.Buffer
		if err := WriteEnvelopeTo(&direct, env, format, "", ""); err != nil {
			t.Fatalf("WriteEnvelopeTo(%s): %v", format, err)
		}
		var stdout, stderr bytes.Buffer
		if err := NewEmitter(&stdout, &stderr, format, "", "").Emit(env); err != nil {
			t.Fatalf("Emit(%s): %v", format, err)
		}
		if !bytes.Equal(direct.Bytes(), stdout.Bytes()) {
			t.Fatalf("format %s: Emitter and WriteEnvelopeTo disagree:\n writer: %s\nemitter: %s",
				format, direct.String(), stdout.String())
		}
	}
}

func TestRenderEnvelopeBufferFirstZeroLeakOnRenderFailure(t *testing.T) {
	// §5.2 末条对整个分发点成立：渲染失败（不可序列化载荷 chan）
	// 不向目标流泄漏任何字节——渲染段在写出前整体失败。
	unrenderable := NewSuccessEnvelope(map[string]any{"bad": make(chan int)})
	for _, format := range []Format{FormatJSON, FormatTable, FormatCSV, FormatNDJSON} {
		target := &emitterFailingWriter{acceptN: 64}
		err := WriteEnvelopeTo(target, unrenderable, format, "", "")
		if err == nil {
			t.Fatalf("format %s must surface render failure", format)
		}
		if len(target.accepted) != 0 {
			t.Fatalf("format %s leaked %d bytes on render failure: %s", format, len(target.accepted), target.accepted)
		}
	}
}

func TestRenderEnvelopeFailureEnvelopeBypassesFormatAtDispatchPoint(t *testing.T) {
	// 轮8裁决⑪在分发点内落地：failure 信封即使被调用方以 table/jq 配置
	// 送入 renderEnvelope（WriteEnvelopeTo 层无流路由），也恒输出完整 JSON
	// 信封——format/jq/fields 不得抹掉 §2.4 wire-stable 字段组。
	var buf bytes.Buffer
	env := NewFailureEnvelope(&ErrorInfo{Type: "api", Subtype: "rate_limit", Code: 90018, Retryable: true})
	if err := WriteEnvelopeTo(&buf, env, FormatTable, "id", ".data"); err != nil {
		t.Fatalf("WriteEnvelopeTo failure under table/jq: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("failure envelope must stay full JSON regardless of format: %v\n%s", err, buf.String())
	}
	if decoded["outcome"] != "failure" {
		t.Fatalf("outcome = %v, want failure: %s", decoded["outcome"], buf.String())
	}
	errObj, ok := decoded["error"].(map[string]any)
	if !ok {
		t.Fatalf("failure envelope lost error detail: %s", buf.String())
	}
	for _, key := range []string{"type", "subtype", "code", "retryable"} {
		if _, has := errObj[key]; !has {
			t.Fatalf("wire-stable key %q lost under format/jq bypass: %s", key, buf.String())
		}
	}
}

// --- B38：-f json 输出完整信封（唯一 JSON 契约）---

func TestWriteEnvelopeJSONIsTheOnlyJSONContract(t *testing.T) {
	// json 与空 format 等价：stdout 恰为一个完整信封 JSON 文档，
	// ok/outcome/data/meta 外壳键齐备（唯一 JSON 契约，§5.2 首行）。
	for _, format := range []Format{FormatJSON, ""} {
		var buf bytes.Buffer
		env := phaseCListFixture()
		if err := WriteEnvelopeTo(&buf, env, format, "", ""); err != nil {
			t.Fatalf("WriteEnvelopeTo(%q): %v", format, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
			t.Fatalf("format %q stdout is not valid JSON: %v\n%s", format, err, buf.String())
		}
		if okVal, isBool := decoded["ok"].(bool); !isBool || !okVal {
			t.Fatalf("ok = %v (%T), want true (bool)", decoded["ok"], decoded["ok"])
		}
		if decoded["outcome"] != "success" {
			t.Fatalf("outcome = %v, want success", decoded["outcome"])
		}
		for _, key := range []string{"data", "meta"} {
			if _, has := decoded[key]; !has {
				t.Fatalf("full envelope must carry %q: %s", key, buf.String())
			}
		}
	}
}

func TestWriteEnvelopeJSONFieldsProjectionOnEnvelope(t *testing.T) {
	// --fields 与 json 联动（成功通道）：投影作用于信封顶层键。
	var buf bytes.Buffer
	env := NewSuccessEnvelope(map[string]any{"name": "DemoApp"})
	if err := WriteEnvelopeTo(&buf, env, FormatJSON, "ok,outcome", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if _, has := decoded["ok"]; !has {
		t.Fatalf("fields=ok,outcome must keep ok: %s", buf.String())
	}
	if _, has := decoded["outcome"]; !has {
		t.Fatalf("fields=ok,outcome must keep outcome: %s", buf.String())
	}
	if _, has := decoded["data"]; has {
		t.Fatalf("fields=ok,outcome must drop data: %s", buf.String())
	}
}

func TestJQOverridesFormatAndEvaluatesFullEnvelope(t *testing.T) {
	// --jq 优先于 format（§5.2 矩阵行）：-f table 下 jq 仍对**完整信封**
	// 求值——.data 可取业务载荷、.ok/.outcome 可取框架字段。
	for _, tc := range []struct {
		jq   string
		want string
	}{
		{".ok", "true"},
		{".outcome", `"success"`},
		{".meta.pagination.next_token", `"cursor_xyz"`},
	} {
		var buf bytes.Buffer
		if err := WriteEnvelopeTo(&buf, phaseCListFixture(), FormatTable, "", tc.jq); err != nil {
			t.Fatalf("WriteEnvelopeTo jq=%q: %v", tc.jq, err)
		}
		if got := strings.TrimSpace(buf.String()); got != tc.want {
			t.Fatalf("jq %q = %q, want %q (jq must win over -f table on the full envelope)", tc.jq, got, tc.want)
		}
	}
	// .data 提取业务载荷：输出不再是信封（无 outcome 外壳键）。
	var buf bytes.Buffer
	if err := WriteEnvelopeTo(&buf, phaseCListFixture(), FormatTable, "", ".data"); err != nil {
		t.Fatalf("WriteEnvelopeTo jq=.data: %v", err)
	}
	if strings.Contains(buf.String(), `"outcome"`) {
		t.Fatalf("jq=.data output must not carry the envelope wrapper: %s", buf.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("jq=.data must emit the bare data payload: %v\n%s", err, buf.String())
	}
	if len(rows) != 3 {
		t.Fatalf("jq=.data rows = %d, want 3", len(rows))
	}
}

// --- B39：-f table/pretty 输出业务数据 + 人读摘要（不输出信封外壳）---

func TestTableRendersDataWithMetaSummaryWithoutEnvelopeWrapper(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteEnvelopeTo(&buf, phaseCListFixture(), FormatTable, "", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo(table): %v", err)
	}
	out := buf.String()
	// 数据行可见。
	for _, want := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table output missing data row %q:\n%s", want, out)
		}
	}
	// 人读摘要：count 与分页摘要（wire 字段名 snake_case，两态显式——
	// endpoint_exhausted:false 也输出，它是「可续跑」语义）。
	if !strings.Contains(out, "count: 3") {
		t.Fatalf("table output missing count summary:\n%s", out)
	}
	if !strings.Contains(out, "pagination: endpoint_exhausted: false, pages: 2, items: 60, next_token: cursor_xyz") {
		t.Fatalf("table output missing pagination summary:\n%s", out)
	}
	// 不输出信封外壳键。
	for _, forbidden := range []string{`"ok"`, `"outcome"`, `"endpoint_exhausted": `} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("table output must not emit envelope wrapper (%q):\n%s", forbidden, out)
		}
	}
	// stdout 不是 JSON 文档（人读视图）。
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("table output should not be a JSON document:\n%s", out)
	}
}

func TestTableWithoutMetaEmitsNoSummary(t *testing.T) {
	// 无 meta 时不追加摘要行，不留空壳。
	var buf bytes.Buffer
	env := NewSuccessEnvelope([]any{map[string]any{"id": "a"}})
	if err := WriteEnvelopeTo(&buf, env, FormatTable, "", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo(table): %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "count:") || strings.Contains(out, "pagination:") {
		t.Fatalf("no-meta table must not emit summary lines:\n%s", out)
	}
	if !strings.Contains(out, "a") {
		t.Fatalf("table output missing data:\n%s", out)
	}
}

func TestPrettyRendersDataWithMetaSummaryWithoutEnvelopeWrapper(t *testing.T) {
	forceNoColor(t)
	var buf bytes.Buffer
	if err := WriteEnvelopeTo(&buf, phaseCListFixture(), FormatPretty, "", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo(pretty): %v", err)
	}
	out := buf.String()
	for _, want := range []string{"alpha", "count: 3", "endpoint_exhausted: false"} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing %q:\n%s", want, out)
		}
	}
	for _, forbidden := range []string{`"ok"`, `"outcome"`} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("pretty output must not emit envelope wrapper (%q):\n%s", forbidden, out)
		}
	}
}

func TestTableWithFieldsProjectionNarrowsColumns(t *testing.T) {
	// --fields 与 table 联动（成功通道）：投影先作用于数据载荷再渲染。
	var buf bytes.Buffer
	if err := WriteEnvelopeTo(&buf, phaseCListFixture(), FormatTable, "id", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo(table, fields=id): %v", err)
	}
	out := buf.String()
	// id 列保留（值 a/b/c 可见），name 列与其值被投影掉。
	for _, want := range []string{"id", "│ a", "│ b", "│ c"} {
		if !strings.Contains(out, want) {
			t.Fatalf("fields=id must keep id column/values (%q):\n%s", want, out)
		}
	}
	if strings.Contains(out, "name") || strings.Contains(out, "alpha") {
		t.Fatalf("fields=id must drop name column and its values:\n%s", out)
	}
}

// --- B40：-f csv/ndjson 裸记录；分页元数据走 stderr 诊断行 ---

// newPhaseCCmd 构造带 --format flag 且 stdout/stderr 均可捕获的测试命令
// （诊断行断言必须走 cmd 出口路径：WriteEnvelopeTo 的 writer 级签名无
// stderr 出口，诊断行在该签名下按 io.Discard 处理）。
func newPhaseCCmd(t *testing.T, formatValue string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	cmd := &cobra.Command{Use: "t"}
	cmd.Flags().StringP("format", "f", "", "")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Flags().Set("format", formatValue); err != nil {
		t.Fatalf("set format: %v", err)
	}
	return cmd, &stdout, &stderr
}

func TestNDJSONEmitsBareRecordsWithoutEnvelope(t *testing.T) {
	cmd, stdout, stderr := newPhaseCCmd(t, "ndjson")
	env := NewSuccessEnvelope([]any{
		map[string]any{"id": "a"},
		map[string]any{"id": "b"},
		map[string]any{"id": "c"},
	})
	if err := WriteEnvelope(cmd, env, FormatJSON); err != nil {
		t.Fatalf("WriteEnvelope(ndjson): %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("ndjson must emit one bare record per line, got %d:\n%s", len(lines), stdout.String())
	}
	for i, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("ndjson line %d is not valid JSON: %v", i, err)
		}
		if _, has := rec["outcome"]; has {
			t.Fatalf("ndjson line %d carries envelope wrapper: %s", i, line)
		}
	}
	// 无 pagination 时 stderr 零诊断行。
	if stderr.Len() != 0 {
		t.Fatalf("no-pagination ndjson must keep diagnostics silent, got %d bytes: %s", stderr.Len(), stderr.String())
	}
}

func TestCSVEmitsBareRecordsWithoutEnvelope(t *testing.T) {
	cmd, stdout, stderr := newPhaseCCmd(t, "csv")
	env := NewSuccessEnvelope([]any{
		map[string]any{"id": "a", "name": "alpha"},
		map[string]any{"id": "b", "name": "beta"},
	})
	if err := WriteEnvelope(cmd, env, FormatJSON); err != nil {
		t.Fatalf("WriteEnvelope(csv): %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 3 { // header + 2 records
		t.Fatalf("csv must emit header + one row per record, got %d lines:\n%s", len(lines), stdout.String())
	}
	if !strings.Contains(lines[0], "id") || !strings.Contains(lines[0], "name") {
		t.Fatalf("csv header missing columns: %s", lines[0])
	}
	if strings.Contains(stdout.String(), `"outcome"`) || strings.Contains(stdout.String(), `"ok"`) {
		t.Fatalf("csv must not emit envelope wrapper keys:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("no-pagination csv must keep diagnostics silent, got %d bytes: %s", stderr.Len(), stderr.String())
	}
}

func TestCSVNDJSONPaginationMetadataGoesToStderrDiagnostic(t *testing.T) {
	// 分页元数据不进裸记录流，走 stderr 诊断行（§5.2 csv/ndjson 行）。
	for _, format := range []Format{FormatCSV, FormatNDJSON} {
		cmd, stdout, stderr := newPhaseCCmd(t, string(format))
		if err := WriteEnvelope(cmd, phaseCListFixture(), FormatJSON); err != nil {
			t.Fatalf("WriteEnvelope(%s): %v", format, err)
		}
		diag := stderr.String()
		if !strings.Contains(diag, "[pagination]") {
			t.Fatalf("%s pagination diagnostic missing from stderr: %q", format, diag)
		}
		for _, want := range []string{"endpoint_exhausted: false", "next_token: cursor_xyz"} {
			if !strings.Contains(diag, want) {
				t.Fatalf("%s diagnostic missing %q: %q", format, want, diag)
			}
		}
		if got := strings.Count(diag, "[pagination]"); got != 1 {
			t.Fatalf("%s must emit exactly one pagination diagnostic line, got %d: %q", format, got, diag)
		}
		// stdout 保持纯裸记录：无诊断行、无信封键、无分页键。
		out := stdout.String()
		if strings.Contains(out, "[pagination]") || strings.Contains(out, "next_token") || strings.Contains(out, `"outcome"`) {
			t.Fatalf("%s stdout polluted by diagnostics/envelope:\n%s", format, out)
		}
	}
}

func TestEmitterCSVNDJSONDiagnosticRoutedToEmitterStderr(t *testing.T) {
	// Emitter 路径：诊断行落 Emitter 的 stderr writer（可注入），stdout
	// 只承载裸记录；流路由纪律不变（success → stdout）。
	for _, format := range []Format{FormatCSV, FormatNDJSON} {
		var stdout, stderr bytes.Buffer
		em := NewEmitter(&stdout, &stderr, format, "", "")
		if err := em.Emit(phaseCListFixture()); err != nil {
			t.Fatalf("Emit(%s): %v", format, err)
		}
		if !strings.Contains(stderr.String(), "[pagination]") {
			t.Fatalf("%s: emitter stderr must carry pagination diagnostic: %q", format, stderr.String())
		}
		if strings.Contains(stdout.String(), "[pagination]") {
			t.Fatalf("%s: stdout must stay bare records:\n%s", format, stdout.String())
		}
	}
}

func TestPartialFailureCSVKeepsStderrZero(t *testing.T) {
	// §2.3 特例优先：partial_failure 的 stderr 零输出约束压过 csv/ndjson
	// 的诊断行通则——即便携带分页元数据也不写诊断行。
	env := newPartialEnvelopeFixture()
	env.Meta = &Meta{
		Count:      NewCount(3),
		Pagination: &Pagination{EndpointExhausted: true},
	}
	var stdout, stderr bytes.Buffer
	em := NewEmitter(&stdout, &stderr, FormatCSV, "", "")
	if err := em.Emit(env); err != nil {
		t.Fatalf("Emit partial under csv: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("partial_failure must keep stderr zero (§2.3), got %d bytes: %s", stderr.Len(), stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatal("partial_failure details must stay on stdout (§2.3)")
	}
}

// --- B43：--json 语义 = --format json 简写（显式 --format 优先）---

func TestResolveFormatWithJSONShorthandMatrix(t *testing.T) {
	// 优先级链穷举（契约规范 §5.2：「--json 自动注册为 --format json 简写
	// （显式 --format 优先）」）：显式 --format 非空值恒优先；--format 缺席
	// 或为空时 --json=true 等价 --format json；两者皆无 → fallback。
	newCmd := func(formatSet string, formatValue string, jsonSet bool, jsonValue bool) *cobra.Command {
		cmd := &cobra.Command{Use: "t"}
		cmd.Flags().String("format", "", "")
		cmd.Flags().Bool("json", false, "")
		if formatSet != "" {
			if err := cmd.Flags().Set("format", formatValue); err != nil {
				t.Fatalf("set format: %v", err)
			}
		}
		if jsonSet {
			if err := cmd.Flags().Set("json", jsonFlagValue(jsonValue)); err != nil {
				t.Fatalf("set json: %v", err)
			}
		}
		return cmd
	}
	cases := []struct {
		name     string
		cmd      *cobra.Command
		fallback Format
		want     Format
	}{
		{"json shorthand alone", newCmd("", "", true, true), FormatTable, FormatJSON},
		{"explicit format wins over json shorthand", newCmd("set", "table", true, true), FormatJSON, FormatTable},
		{"explicit raw wins over json shorthand", newCmd("set", "raw", true, true), FormatJSON, FormatRaw},
		{"empty format does not shadow shorthand", newCmd("set", "", true, true), FormatTable, FormatJSON},
		{"json=false is not shorthand", newCmd("", "", true, false), FormatTable, FormatTable},
		{"neither flag", newCmd("", "", false, false), FormatTable, FormatTable},
		{"unknown format not rescued by json shorthand", newCmd("set", "yaml", true, true), FormatJSON, FormatJSON},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResolveFormatWithJSONShorthand(c.cmd, c.fallback); got != c.want {
				t.Fatalf("ResolveFormatWithJSONShorthand = %q, want %q (fallback=%q)", got, c.want, c.fallback)
			}
		})
	}
	if got := ResolveFormatWithJSONShorthand(nil, FormatRaw); got != FormatRaw {
		t.Fatalf("nil cmd = %q, want fallback raw", got)
	}
}

// jsonFlagValue 把 bool 渲染成 pflag.Set 可接受的字符串。
func jsonFlagValue(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func TestResolveFormatWithJSONShorthandIgnoresStringTypedJSONFlag(t *testing.T) {
	// 业务层存在同名 string flag（如 table create --json 载荷参数）：
	// GetBool 失败即跳过，不得被误判为简写。
	cmd := &cobra.Command{Use: "t"}
	cmd.Flags().String("json", "", "base JSON payload")
	if err := cmd.Flags().Set("json", `{"x":1}`); err != nil {
		t.Fatalf("set json: %v", err)
	}
	if got := ResolveFormatWithJSONShorthand(cmd, FormatTable); got != FormatTable {
		t.Fatalf("string-typed --json must not act as shorthand, got %q", got)
	}
}

func TestResolveFormatWithJSONShorthandReadsInheritedAndPersistentFlags(t *testing.T) {
	// --format 继承自 root persistent（与 ResolveFormat 同查找面）；
	// --json 挂在子命令本地亦生效。
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().String("format", "", "")
	child := &cobra.Command{Use: "list"}
	child.Flags().Bool("json", false, "")
	root.AddCommand(child)
	if err := root.PersistentFlags().Set("format", "csv"); err != nil {
		t.Fatalf("set format: %v", err)
	}
	if err := child.Flags().Set("json", "true"); err != nil {
		t.Fatalf("set json: %v", err)
	}
	// 显式 format（继承）优先于 json 简写。
	if got := ResolveFormatWithJSONShorthand(child, FormatJSON); got != FormatCSV {
		t.Fatalf("inherited explicit format must win, got %q", got)
	}
	// 清空 format 后 json 简写生效。
	if err := root.PersistentFlags().Set("format", ""); err != nil {
		t.Fatalf("reset format: %v", err)
	}
	if got := ResolveFormatWithJSONShorthand(child, FormatTable); got != FormatJSON {
		t.Fatalf("json shorthand = %q, want json", got)
	}
}

// --- B209（轮10裁决⑭小批，随 Phase C 同轮收口）：ExitCodeForEnvelope 边界
// 组合精确值断言 + 与 internal/errors.ExitCode 表同源锁定 ---

func TestExitCodeForEnvelopeBoundaryCombinationsExactValues(t *testing.T) {
	// ① failure + Error 缺席（I3 非法的手工形态，构造器不产出）→ internal
	// 兜底 exit==5，精确值固化（无明细可分支的失败不得假装可恢复）。
	invalid := &Envelope{OK: false, Outcome: OutcomeFailure}
	if err := invalid.Validate(); err == nil {
		t.Fatal("failure without Error must be rejected by Validate (I3)")
	}
	if got := ExitCodeForEnvelope(invalid); got != 5 {
		t.Fatalf("failure without Error = exit %d, want 5 (internal fallback)", got)
	}
	// ② failure + Error.Type 空串（wire-stable 字段缺失形态）→ 同样 internal 5。
	emptyType := NewFailureEnvelope(&ErrorInfo{Message: "no type"})
	if got := ExitCodeForEnvelope(emptyType); got != 5 {
		t.Fatalf("failure with empty error.type = exit %d, want 5 (internal fallback)", got)
	}
	// ③ failure + Error.Type 未知值（不在码表类别内）→ internal 5。
	unknown := NewFailureEnvelope(&ErrorInfo{Type: "mystery"})
	if got := ExitCodeForEnvelope(unknown); got != 5 {
		t.Fatalf("failure with unknown error.type = exit %d, want 5", got)
	}
}

func TestExitCodeForEnvelopeSameSourceAsErrorsExitCode(t *testing.T) {
	// B33 常量块与 internal/errors/errors.go ExitCode 表同源锁定：逐类别值
	// 相等，防未来单边漂移（轮10裁决⑬/⑭）。PAT=4 边界注记：PAT 行为授权
	// 专属码经 ExitCoder 接口路径分发，不出现在信封码表（信封 failure 的
	// error.type 枚举不含 pat 类别），故不在本锁定表内。
	cases := []struct {
		category string
		err      error
	}{
		{"api", apperrors.NewAPI("x")},
		{"auth", apperrors.NewAuth("x")},
		{"validation", apperrors.NewValidation("x")},
		{"discovery", apperrors.NewDiscovery("x")},
		{"internal", apperrors.NewInternal("x")},
	}
	for _, c := range cases {
		want := apperrors.ExitCode(c.err)
		env := NewFailureEnvelope(&ErrorInfo{Type: c.category})
		if got := ExitCodeForEnvelope(env); got != want {
			t.Fatalf("category %q: ExitCodeForEnvelope=%d but errors.ExitCode=%d (code tables drifted)",
				c.category, got, want)
		}
	}
	// confirmation_required 子类与 validation 共享 rc=3（OQ-1 定案）：
	// runtime 形态 = NewValidation + WithReason(confirmation_required)。
	confirmErr := apperrors.NewValidation("confirm", apperrors.WithReason("confirmation_required"))
	confirmCode := apperrors.ExitCode(confirmErr)
	confirmEnv := NewFailureEnvelope(&ErrorInfo{Type: "validation", Subtype: "confirmation_required"})
	if got := ExitCodeForEnvelope(confirmEnv); got != confirmCode {
		t.Fatalf("confirmation subclass: ExitCodeForEnvelope=%d but runtime errors.ExitCode=%d", got, confirmCode)
	}
	if confirmCode != 3 {
		t.Fatalf("confirmation/validation shared rc = %d, want 3 (OQ-1)", confirmCode)
	}
}

// --- B35~B40 综合：六格式在 WriteEnvelopeTo/Emitter 路径的完整分发表 ---

func TestRawPassThroughEnvelopeDiffersFromJSONByteForByte(t *testing.T) {
	// -f raw 对信封的透传：只出数据载荷本身（writeRaw 语义），与 json 的
	// 完整信封逐字节不同——format flag 不是惰性的（对照附录 A.3：
	// 历史上 --format 六值 stdout 逐字节相同即判 flag 惰性缺陷）。
	env := NewSuccessEnvelope(map[string]any{"path": "/tmp/a.bin", "size": 42})
	var rawBuf, jsonBuf bytes.Buffer
	if err := WriteEnvelopeTo(&rawBuf, env, FormatRaw, "", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo(raw): %v", err)
	}
	if err := WriteEnvelopeTo(&jsonBuf, env, FormatJSON, "", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo(json): %v", err)
	}
	if bytes.Equal(rawBuf.Bytes(), jsonBuf.Bytes()) {
		t.Fatalf("-f raw must differ from -f json byte-for-byte (format flag must not be inert):\n%s", rawBuf.String())
	}
	// raw = 数据载荷透传：恰为 writeRaw(data) 的输出（紧凑单行 JSON +
	// 终端净化），不含信封外壳键。
	var direct bytes.Buffer
	if err := Write(&direct, FormatRaw, env.Data); err != nil {
		t.Fatalf("Write(raw, data): %v", err)
	}
	if !bytes.Equal(rawBuf.Bytes(), direct.Bytes()) {
		t.Fatalf("-f raw output must be a verbatim pass-through of the data payload:\n got: %s\nwant: %s", rawBuf.String(), direct.String())
	}
	if strings.Contains(rawBuf.String(), `"outcome"`) || strings.Contains(rawBuf.String(), `"ok"`) {
		t.Fatalf("-f raw must not emit the envelope wrapper: %s", rawBuf.String())
	}
	// 数据值可用：raw 输出可解码回业务载荷。
	var decoded map[string]any
	if err := json.Unmarshal(rawBuf.Bytes(), &decoded); err != nil {
		t.Fatalf("raw output is not decodable JSON for structured payload: %v\n%s", err, rawBuf.String())
	}
	if decoded["path"] != "/tmp/a.bin" {
		t.Fatalf("raw payload lost data: %s", rawBuf.String())
	}
}

func TestSixFormatDispatchMatrixOnEnvelopePath(t *testing.T) {
	// 六格式逐格式断言分发行为（§5.2 矩阵在信封出口的行为端）：
	// json = 完整信封；table/pretty = 数据 + 摘要；csv/ndjson = 裸记录；
	// raw = 数据透传。全部格式 stdout 非空、均不产出「半个信封」。
	cases := []struct {
		format      Format
		wantJSON    bool // stdout 是否恰为 JSON 文档（完整信封）
		wantWrapper bool // 是否含信封外壳键（仅 json 允许）
	}{
		{FormatJSON, true, true},
		{FormatTable, false, false},
		{FormatPretty, false, false},
		{FormatCSV, false, false},
		{FormatNDJSON, false, false},
		{FormatRaw, false, false},
	}
	for _, c := range cases {
		t.Run(string(c.format), func(t *testing.T) {
			forceNoColor(t)
			var buf bytes.Buffer
			if err := WriteEnvelopeTo(&buf, phaseCListFixture(), c.format, "", ""); err != nil {
				t.Fatalf("WriteEnvelopeTo(%s): %v", c.format, err)
			}
			out := buf.String()
			if len(out) == 0 {
				t.Fatalf("format %s produced empty stdout", c.format)
			}
			hasWrapper := strings.Contains(out, `"outcome"`)
			if hasWrapper != c.wantWrapper {
				t.Fatalf("format %s wrapper presence = %v, want %v:\n%s", c.format, hasWrapper, c.wantWrapper, out)
			}
			if c.wantJSON {
				var decoded map[string]any
				if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
					t.Fatalf("format %s must emit one complete JSON envelope: %v\n%s", c.format, err, out)
				}
				if decoded["ok"] != true {
					t.Fatalf("format %s envelope ok = %v, want true", c.format, decoded["ok"])
				}
			} else {
				// 数据可见性：三格式与 raw 都能见到业务值。
				if !strings.Contains(out, "alpha") {
					t.Fatalf("format %s lost data payload:\n%s", c.format, out)
				}
			}
		})
	}
}
