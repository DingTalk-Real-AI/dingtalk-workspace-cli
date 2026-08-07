package helpers

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// newCommonLeafCmd 构造带本地 --format flag 的叶子命令（B44/B57/B58 共用）。
// --format 直接挂本地 flag：resolveFormatWithWarning 经 cmd.Flags() 命中。
func newCommonLeafCmd(format string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	cmd := &cobra.Command{Use: "leaf"}
	cmd.Flags().String("format", format, "")
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	return cmd, &out, &errOut
}

// newCommonRootWithFilters 构造带生产同款 usage 的全局 --jq/--fields persistent
// flag 的 root（output.ResolveJQ/ResolveFields 以 usage 匹配区分业务同名 flag，
// 测试必须复刻生产 flags.go 的 usage 文案才能命中全局过滤器）。
func newCommonRootWithFilters(leaf *cobra.Command) *cobra.Command {
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().String("fields", "", "筛选输出字段 (逗号分隔, 如: name,id,status)")
	root.PersistentFlags().String("jq", "", "jq 表达式过滤输出 (如: '.items[] | .name')")
	root.AddCommand(leaf)
	return root
}

// --- B44：format 解析桥接（cmd flags → ResolveFormatWithJSONShorthand）---

func TestResolveCommandFormatExplicitFormatWins(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want output.Format
	}{
		{"table", output.FormatTable},
		{"  PRETTY ", output.FormatPretty},
		{"csv", output.FormatCSV},
		{"ndjson", output.FormatNDJSON},
		{"raw", output.FormatRaw},
		{"json", output.FormatJSON},
		{"bogus", output.FormatJSON}, // 未知值降级 fallback
		{"", output.FormatJSON},      // 空值 = 未指定，静默 fallback
	} {
		cmd, _, _ := newCommonLeafCmd(tc.raw)
		if got := resolveCommandFormat(cmd); got != tc.want {
			t.Fatalf("--format=%q resolved = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestResolveCommandFormatJSONShorthandAndPriority(t *testing.T) {
	// --json 单独生效 = -f json 简写。
	plain := &cobra.Command{Use: "leaf"}
	plain.Flags().Bool("json", false, "")
	if err := plain.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	if got := resolveCommandFormat(plain); got != output.FormatJSON {
		t.Fatalf("--json shorthand resolved = %q, want json", got)
	}
	// 显式 --format 恒优先于 --json。
	explicit := &cobra.Command{Use: "leaf"}
	explicit.Flags().String("format", "table", "")
	explicit.Flags().Bool("json", false, "")
	if err := explicit.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	if got := resolveCommandFormat(explicit); got != output.FormatTable {
		t.Fatalf("--format table + --json resolved = %q, want table (explicit wins)", got)
	}
	// 两者皆无 → fallback json。
	bare := &cobra.Command{Use: "leaf"}
	if got := resolveCommandFormat(bare); got != output.FormatJSON {
		t.Fatalf("no flags resolved = %q, want json fallback", got)
	}
	// nil cmd 不 panic，返回 fallback。
	if got := resolveCommandFormat(nil); got != output.FormatJSON {
		t.Fatalf("nil cmd resolved = %q, want json fallback", got)
	}
}

// --- B57：writeCommandPayload 接 format 分发（替换固定 json 兜底）---

func TestWriteCommandPayloadDispatchesResolvedFormat(t *testing.T) {
	payload := map[string]any{"name": "alice", "role": "admin"}

	jsonCmd, jsonOut, _ := newCommonLeafCmd("json")
	if err := writeCommandPayload(jsonCmd, payload); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(jsonOut.Bytes(), &decoded); err != nil {
		t.Fatalf("json payload not parseable: %v\n%s", err, jsonOut.String())
	}
	if decoded["name"] != "alice" {
		t.Fatalf("json payload = %#v", decoded)
	}

	tableCmd, tableOut, _ := newCommonLeafCmd("table")
	if err := writeCommandPayload(tableCmd, payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tableOut.String(), "alice") || !strings.Contains(tableOut.String(), "admin") {
		t.Fatalf("table payload missing data: %q", tableOut.String())
	}
	if strings.Contains(tableOut.String(), "\"role\"") {
		t.Fatalf("table output looks like JSON document (dispatch bypass?): %q", tableOut.String())
	}
}

func TestWriteCommandPayloadHonorsFieldsAndJQ(t *testing.T) {
	payload := map[string]any{"name": "alice", "role": "admin"}

	fieldsLeaf, fieldsOut, _ := newCommonLeafCmd("json")
	fieldsRoot := newCommonRootWithFilters(fieldsLeaf)
	if err := fieldsRoot.PersistentFlags().Set("fields", "name"); err != nil {
		t.Fatal(err)
	}
	if err := writeCommandPayload(fieldsLeaf, payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fieldsOut.String(), "alice") || strings.Contains(fieldsOut.String(), "admin") {
		t.Fatalf("--fields=name projection = %q", fieldsOut.String())
	}

	jqLeaf, jqOut, _ := newCommonLeafCmd("json")
	jqRoot := newCommonRootWithFilters(jqLeaf)
	if err := jqRoot.PersistentFlags().Set("jq", ".name"); err != nil {
		t.Fatal(err)
	}
	if err := writeCommandPayload(jqLeaf, payload); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(jqOut.String()) != `"alice"` {
		t.Fatalf("--jq .name result = %q", jqOut.String())
	}
}

// B57 边界：nil cmd 不 panic（io.Discard 容错渲染）。
func TestWriteCommandPayloadNilCmdDoesNotPanic(t *testing.T) {
	if err := writeCommandPayload(nil, map[string]any{"a": 1}); err != nil {
		t.Fatalf("nil cmd error = %v", err)
	}
}

// --- B58~B63：writeEnvelope 统一信封装配出口 ---

func commonSuccessEnvelope() *output.Envelope {
	return output.NewSuccessEnvelope(map[string]any{"name": "alice"})
}

// B59：装配出口走 cmd.OutOrStdout()（重定向捕获，非硬编码 os.Stdout）。
func TestWriteEnvelopeSuccessGoesToCmdOutStream(t *testing.T) {
	cmd, out, errOut := newCommonLeafCmd("json")
	if err := writeEnvelope(cmd, commonSuccessEnvelope()); err != nil {
		t.Fatal(err)
	}
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\n%s", err, out.String())
	}
	if env["ok"] != true || env["outcome"] != "success" {
		t.Fatalf("envelope = %#v", env)
	}
	if errOut.Len() != 0 {
		t.Fatalf("success path must keep stderr silent, got %q", errOut.String())
	}

	// 换一个新 buffer 再写一次：输出跟随 cmd 的注入出口，证明无静态缓冲残留。
	cmd2, out2, _ := newCommonLeafCmd("json")
	if err := writeEnvelope(cmd2, commonSuccessEnvelope()); err != nil {
		t.Fatal(err)
	}
	if out2.String() != out.String() {
		t.Fatalf("second command output diverges:\n%s\nvs\n%s", out2.String(), out.String())
	}
}

// B60：失败路径 stdout 严格零字节，信封落 stderr。
func TestWriteEnvelopeFailureKeepsStdoutEmpty(t *testing.T) {
	cmd, out, errOut := newCommonLeafCmd("json")
	failEnv := output.NewFailureEnvelope(&output.ErrorInfo{Type: "api", Message: "boom"})
	if err := writeEnvelope(cmd, failEnv); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("failure envelope leaked to stdout: %q", out.String())
	}
	var env map[string]any
	if err := json.Unmarshal(errOut.Bytes(), &env); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v\n%s", err, errOut.String())
	}
	if env["ok"] != false || env["outcome"] != "failure" {
		t.Fatalf("failure envelope = %#v", env)
	}
}

// B60 边界：nil 信封经装配出口降级为 I3 合法 failure 兜底，同样 stdout 零字节。
func TestWriteEnvelopeNilEnvelopeDegradesToStderrFailure(t *testing.T) {
	cmd, out, errOut := newCommonLeafCmd("json")
	if err := writeEnvelope(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("nil envelope leaked to stdout: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "\"outcome\": \"failure\"") {
		t.Fatalf("stderr fallback envelope = %q", errOut.String())
	}
}

// B61：-f json 完整信封 / -f pretty 人读视图（无信封外壳）双格式断言。
func TestWriteEnvelopeJSONFullEnvelopeVsPrettyHumanView(t *testing.T) {
	jsonCmd, jsonOut, _ := newCommonLeafCmd("json")
	if err := writeEnvelope(jsonCmd, commonSuccessEnvelope()); err != nil {
		t.Fatal(err)
	}
	var env map[string]any
	if err := json.Unmarshal(jsonOut.Bytes(), &env); err != nil {
		t.Fatalf("json mode stdout not an envelope: %v\n%s", err, jsonOut.String())
	}
	for _, key := range []string{"ok", "outcome", "data"} {
		if _, present := env[key]; !present {
			t.Fatalf("json envelope missing %q key: %s", key, jsonOut.String())
		}
	}

	prettyCmd, prettyOut, _ := newCommonLeafCmd("pretty")
	if err := writeEnvelope(prettyCmd, commonSuccessEnvelope()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prettyOut.String(), "alice") {
		t.Fatalf("pretty view missing business data: %q", prettyOut.String())
	}
	for _, banned := range []string{"\"ok\"", "\"outcome\""} {
		if strings.Contains(prettyOut.String(), banned) {
			t.Fatalf("pretty view leaks envelope wrapper key %s: %q", banned, prettyOut.String())
		}
	}

	// table 同样只出业务数据视图（§5.2 矩阵行）。
	tableCmd, tableOut, _ := newCommonLeafCmd("table")
	if err := writeEnvelope(tableCmd, commonSuccessEnvelope()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tableOut.String(), "alice") || strings.Contains(tableOut.String(), "\"ok\"") {
		t.Fatalf("table view wrong shape: %q", tableOut.String())
	}
}

// B62：未知 format 经装配出口降级 json + stderr warning（AC-09）。
func TestWriteEnvelopeUnknownFormatDegradesWithWarning(t *testing.T) {
	cmd, out, errOut := newCommonLeafCmd("bogus")
	if err := writeEnvelope(cmd, commonSuccessEnvelope()); err != nil {
		t.Fatal(err)
	}
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unknown format must degrade to full JSON envelope: %v\n%s", err, out.String())
	}
	if env["ok"] != true {
		t.Fatalf("degraded envelope = %#v", env)
	}
	warning := errOut.String()
	if !strings.Contains(warning, "[WARN]") || !strings.Contains(warning, "bogus") || !strings.Contains(warning, "json") {
		t.Fatalf("stderr warning missing or malformed: %q", warning)
	}
	if strings.Count(warning, "\n") != 1 {
		t.Fatalf("warning must be exactly one line: %q", warning)
	}
}

// B63：--jq 表达式对完整信封求值（.data/.ok 可提取，优先于 format）。
func TestWriteEnvelopeJQQueriesFullEnvelope(t *testing.T) {
	dataLeaf, dataOut, _ := newCommonLeafCmd("table") // jq 优先于 format
	dataRoot := newCommonRootWithFilters(dataLeaf)
	if err := dataRoot.PersistentFlags().Set("jq", ".data.name"); err != nil {
		t.Fatal(err)
	}
	if err := writeEnvelope(dataLeaf, commonSuccessEnvelope()); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(dataOut.String()) != `"alice"` {
		t.Fatalf("--jq .data.name = %q", dataOut.String())
	}

	okLeaf, okOut, _ := newCommonLeafCmd("json")
	okRoot := newCommonRootWithFilters(okLeaf)
	if err := okRoot.PersistentFlags().Set("jq", ".ok"); err != nil {
		t.Fatal(err)
	}
	if err := writeEnvelope(okLeaf, commonSuccessEnvelope()); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(okOut.String()) != "true" {
		t.Fatalf("--jq .ok = %q", okOut.String())
	}
}

// B63 边界（轮8裁决⑪经装配出口成立）：--jq 对失败信封不生效——失败信封绕过
// jq 恒出完整 JSON 到 stderr，错误明细不会被过滤为空。
func TestWriteEnvelopeFailureBypassesJQAtAssemblyExit(t *testing.T) {
	leaf, out, errOut := newCommonLeafCmd("json")
	root := newCommonRootWithFilters(leaf)
	if err := root.PersistentFlags().Set("jq", ".data"); err != nil {
		t.Fatal(err)
	}
	failEnv := output.NewFailureEnvelope(&output.ErrorInfo{Type: "validation", Message: "bad input"})
	if err := writeEnvelope(leaf, failEnv); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("failure envelope leaked to stdout under --jq: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "bad input") {
		t.Fatalf("failure detail lost (jq applied to failure envelope?): %q", errOut.String())
	}
}

func TestUnknownFormatFullChainDegradesOverDevAppCommand(t *testing.T) {
	out, errBuf, err := runDevAppFamily(t,
		devAppFamilyContentRunner(map[string]any{
			"unifiedAppId": "u-1",
			"name":         "DemoApp",
			"appStatus":    "ENABLED",
		}),
		"dev", "app", "get", "--unified-app-id", "u-1", "--format", "bogus")
	if err != nil {
		t.Fatalf("Execute() error = %v, want JSON fallback\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
	}
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil || env["ok"] != true {
		t.Fatalf("fallback output is not a successful JSON envelope: %v\n%s", err, out.String())
	}
	if warning := errBuf.String(); !strings.Contains(warning, "[WARN]") || !strings.Contains(warning, "bogus") || !strings.Contains(warning, "json") {
		t.Fatalf("fallback warning missing or malformed: %q", warning)
	}
}
