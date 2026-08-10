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

// Phase I：负面样例扫描与 complete:true 残留扫描（B155/B158/B159/B160；
// AC-07/G1）。落盘策略：轮8裁决⑩新文件——不编辑 envelope_test.go 既有断言。
//
// 扫描能力落在测试侧（非生产面）：队列要求是「扫描命中断言」，扫描器本身
// 是检测手段而非出口渲染——golden/负面样例 fixture 的命中与干净两侧对照
// 构成 AC-07 契约卫生的可复现门禁。

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// --- 扫描器（测试侧辅助）---

// boolContractKeys 是契约中必须为 JSON bool 的键（AC-02）。
var boolContractKeys = map[string]struct{}{
	"ok":                 {},
	"dry_run":            {},
	"retryable":          {},
	"timed_out":          {},
	"endpoint_exhausted": {},
}

// nonStandardErrorKeys 是被契约弃用的错误码别名键（G1）：失败明细唯一权威
// 是 error.type/error.code 等（§2.4）。
var nonStandardErrorKeys = map[string]struct{}{
	"errcode":    {},
	"errorCode":  {},
	"error_code": {},
}

// logPrefixes 是 stdout 数据通道禁止混入的日志行前缀。
var logPrefixes = []string{"[INFO]", "[DEBUG]", "[WARN]", "[WARNING]", "[ERROR]", "[TRACE]"}

// scanEnvelopeBytes 对一段原始输出字节做契约卫生扫描，返回违反项描述
// （空切片 = 干净）。宽松解析：非单一 JSON 文档不 panic，继续行级扫描。
func scanEnvelopeBytes(raw []byte) []string {
	var violations []string
	violations = append(violations, scanLogPollution(raw)...)

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		if len(scanLogPollution(raw)) == 0 {
			violations = append(violations, fmt.Sprintf("output is not a single JSON document: %v", err))
		}
		return violations
	}
	violations = append(violations, scanStringBooleans("", decoded)...)
	violations = append(violations, scanNonStandardKeys("", decoded)...)
	return violations
}

func scanStringBooleans(path string, node any) []string {
	var out []string
	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			childPath := joinScanPath(path, key)
			if _, isBoolKey := boolContractKeys[key]; isBoolKey {
				if str, ok := value.(string); ok && isBoolLiteral(str) {
					out = append(out, fmt.Sprintf("string boolean at %s: value %q must be a JSON bool (AC-02)", childPath, str))
				}
			}
			out = append(out, scanStringBooleans(childPath, value)...)
		}
	case []any:
		for i, item := range typed {
			out = append(out, scanStringBooleans(fmt.Sprintf("%s[%d]", path, i), item)...)
		}
	}
	return out
}

func isBoolLiteral(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "false":
		return true
	}
	return false
}

func scanNonStandardKeys(path string, node any) []string {
	var out []string
	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			childPath := joinScanPath(path, key)
			if _, banned := nonStandardErrorKeys[key]; banned {
				out = append(out, fmt.Sprintf("non-standard error key at %s: use error.type/error.code instead (G1)", childPath))
			}
			out = append(out, scanNonStandardKeys(childPath, value)...)
		}
	case []any:
		for i, item := range typed {
			out = append(out, scanNonStandardKeys(fmt.Sprintf("%s[%d]", path, i), item)...)
		}
	}
	return out
}

func scanLogPollution(raw []byte) []string {
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		for _, prefix := range logPrefixes {
			if strings.HasPrefix(trimmed, prefix) {
				out = append(out, fmt.Sprintf("stdout log pollution: line %q mixes a log line into the data channel", trimmed))
				break
			}
		}
	}
	return out
}

func joinScanPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func mustReadTestdata(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("negative fixture missing: %v", err)
	}
	return raw
}

// --- B158：字符串布尔负面样例扫描命中 ---

func TestNegativeFixtureStringBooleanDetected(t *testing.T) {
	raw := mustReadTestdata(t, "envelope_negative_string_boolean.json")
	violations := scanEnvelopeBytes(raw)
	if len(violations) == 0 {
		t.Fatal("string-boolean fixture must be detected, got clean scan")
	}
	var stringBoolHits []string
	for _, v := range violations {
		if strings.Contains(v, "string boolean") {
			stringBoolHits = append(stringBoolHits, v)
		}
	}
	if len(stringBoolHits) != 3 {
		t.Fatalf("string-boolean fixture must hit exactly 3 sites (ok/dry_run/meta.pagination.endpoint_exhausted), got %d: %v", len(stringBoolHits), violations)
	}
	for _, wantPath := range []string{"ok", "dry_run", "meta.pagination.endpoint_exhausted"} {
		found := false
		for _, hit := range stringBoolHits {
			if strings.Contains(hit, "at "+wantPath+":") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected string-boolean hit at %q: %v", wantPath, stringBoolHits)
		}
	}
}

// --- B159：非标准键名负面样例扫描命中 ---

func TestNegativeFixtureNonStandardKeysDetected(t *testing.T) {
	raw := mustReadTestdata(t, "envelope_negative_nonstandard_keys.json")
	violations := scanEnvelopeBytes(raw)
	if len(violations) == 0 {
		t.Fatal("non-standard-key fixture must be detected, got clean scan")
	}
	var keyHits []string
	for _, v := range violations {
		if strings.Contains(v, "non-standard error key") {
			keyHits = append(keyHits, v)
		}
	}
	if len(keyHits) != 3 {
		t.Fatalf("non-standard fixture must hit exactly 3 aliases (errcode/errorCode/error_code), got %d: %v", len(keyHits), violations)
	}
	for _, want := range []string{"errcode", "errorCode", "error_code"} {
		found := false
		for _, hit := range keyHits {
			if strings.Contains(hit, "at "+want+":") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected non-standard-key hit at %q: %v", want, keyHits)
		}
	}
}

// --- B160：stdout 日志污染负面样例扫描命中 ---

func TestNegativeFixtureStdoutLogPollutionDetected(t *testing.T) {
	raw := mustReadTestdata(t, "envelope_negative_stdout_log_pollution.txt")
	violations := scanEnvelopeBytes(raw)
	if len(violations) == 0 {
		t.Fatal("log-pollution fixture must be detected, got clean scan")
	}
	var pollutionHits []string
	for _, v := range violations {
		if strings.Contains(v, "stdout log pollution") {
			pollutionHits = append(pollutionHits, v)
		}
	}
	if len(pollutionHits) != 2 {
		t.Fatalf("log-pollution fixture must hit exactly 2 [INFO] lines, got %d: %v", len(pollutionHits), violations)
	}
}

// --- 干净侧对照：golden 四类信封与生产序列化产物零命中 ---

func TestLegalEnvelopesPassNegativeScan(t *testing.T) {
	// golden 四类信封序列化产物必须全部扫描干净（命中/干净两侧对照才是
	// 完整的扫描门禁：只测命中不测干净会掩盖误报）。
	for name, env := range goldenEnvelopes(t) {
		raw := marshalGolden(t, env)
		if violations := scanEnvelopeBytes(raw); len(violations) != 0 {
			t.Fatalf("golden %s envelope must scan clean, got: %v", name, violations)
		}
	}
	// 生产出口路径同判：WriteEnvelopeTo 的 json 输出零命中。
	for _, format := range []Format{FormatJSON} {
		var buf strings.Builder
		if err := WriteEnvelopeTo(&buf, phaseCListFixture(), format, "", ""); err != nil {
			t.Fatalf("WriteEnvelopeTo: %v", err)
		}
		if violations := scanEnvelopeBytes([]byte(buf.String())); len(violations) != 0 {
			t.Fatalf("production json output must scan clean, got: %v", violations)
		}
	}
}

// --- B155：complete:true 残留扫描 ---

// completeKeyResiduePatterns 匹配 JSON 键形态的 complete（不含 completed 等
// 合法后缀）：struct tag 形态 json:"complete[,…]" 与字面量形态 "complete":。
// 不匹配注释、命令名（如 cobra Use: "complete"）与非键位置。
var completeKeyResiduePatterns = []*regexp.Regexp{
	regexp.MustCompile(`json:"complete(,|")`),
	regexp.MustCompile(`"complete"\s*:`),
}

// TestCompleteKeyResidueScan 扫描 internal/output 与 internal/helpers 的输出
// 统一输出实现：弃用的 complete 键（AC-06）不得以 JSON 键形态残留。
// Legacy 产品命令尚未全部迁移，不能用框架单测跨包禁止其历史 wire；每条
// 命令晋级时由产品 contract test 检查 endpoint_exhausted 投影。
func TestCompleteKeyResidueScan(t *testing.T) {
	dirs := []string{"."}
	scanned := 0
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					// testdata 负面样例是故意非标准形态，不在残留扫描域内。
					return fs.SkipDir
				}
				return nil
			}
			ext := filepath.Ext(path)
			if ext != ".go" && ext != ".json" {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			scanned++
			// 扫描器自身测试文件内含自检 snippet（故意出现 complete 键形态），自排除。
			if filepath.Base(path) == "envelope_negative_test.go" {
				return nil
			}
			for _, pattern := range completeKeyResiduePatterns {
				if loc := pattern.FindIndex(raw); loc != nil {
					line := strings.Count(string(raw[:loc[0]]), "\n") + 1
					t.Fatalf("retired complete key residue at %s:%d (AC-06: pagination uses endpoint_exhausted)", path, line)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", dir, err)
		}
	}
	if scanned == 0 {
		t.Fatal("complete-residue scan covered no files; walk is broken")
	}
	// 负面对照：扫描器确实能命中 complete 键形态（防止正则静默失效）。
	for _, snippet := range []string{
		`json:"complete"`,
		`json:"complete,omitempty"`,
		`"complete": true`,
		`"complete" : true`,
	} {
		matched := false
		for _, pattern := range completeKeyResiduePatterns {
			if pattern.MatchString(snippet) {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("complete-residue pattern must detect %q (scanner self-check)", snippet)
		}
	}
	// 非残留形态不得误报（completed 后缀、命令名、注释文本）。
	for _, snippet := range []string{
		`json:"completed"`,
		`Use: "complete",`,
		`// 弃用 complete 键`,
	} {
		for _, pattern := range completeKeyResiduePatterns {
			if pattern.MatchString(snippet) {
				t.Fatalf("complete-residue pattern false-positive on %q", snippet)
			}
		}
	}
}
