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

// Package usage records the SHAPE of MCP tool calls (which product/tool, which
// argument keys, and a redacted sample of ID/enum-like values) so that the
// shortcut layer can later mine high-frequency patterns and suggest distilling
// them into custom shortcuts (see docs/shortcut-p2-design.md).
//
// Privacy first: free-text / content / credential values are NEVER recorded —
// only argument KEYS plus short, whitespace-free ID/enum-like values. Tracking
// is OFF by default (opt-in): enable it with DWS_USAGE_TRACKING=1 (or
// true/on/yes). When enabled it announces itself once on first use.
package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

const (
	logFileName    = "usage.jsonl"
	noticeFileName = ".usage_notified"
	// maxSampleValueLen bounds a value we're willing to record verbatim; longer
	// strings are treated as free text and dropped.
	maxSampleValueLen = 64
)

// Record is one line in usage.jsonl. It captures the call shape, not its data.
type Record struct {
	TS         string            `json:"ts"`
	Product    string            `json:"product"`
	Tool       string            `json:"tool"`
	ArgKeys    []string          `json:"arg_keys,omitempty"`
	SampleArgs map[string]string `json:"sample_args,omitempty"`
	OK         bool              `json:"ok"`
}

// sensitiveKeyParts match argument keys whose VALUES must never be recorded even if
// they look short — they routinely carry user content or secrets.
var sensitiveKeyParts = []string{
	"text", "content", "body", "message", "subject", "title", "name",
	"desc", "keyword", "query", "comment", "remark", "summary", "token",
	"secret", "password", "credential", "clientid", "mobile", "email",
	"phone", "address", "code", "url", "path",
}

// safeScalarKeys is the small set of non-ID scalars whose values are known
// enums or pagination controls. Sampling is deliberately an
// allowlist: a denylist cannot uphold the promise that free text is never
// recorded as new MCP schemas introduce new field names.
var safeScalarKeys = map[string]bool{
	"type": true, "status": true, "state": true, "role": true,
	"action": true, "format": true, "sort": true, "order": true,
	"cursor": true, "page": true, "pagenum": true, "pagesize": true,
	"limit": true, "offset": true,
}

var noticeOnce sync.Once

// Enabled reports whether usage tracking is active. Default OFF (opt-in):
// local telemetry — even shape-only — should not be on without the user's
// explicit consent. Enable by setting DWS_USAGE_TRACKING to 1/true/on/yes
// (case-insensitive).
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DWS_USAGE_TRACKING"))) {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}

// LogPath returns the absolute path of the usage log.
func LogPath() string { return filepath.Join(config.DefaultConfigDir(), logFileName) }

// Append records one tool call. It never blocks or fails the caller: any error
// (disabled, unwritable dir, marshal issue) is swallowed. dryRun calls are
// skipped — they are not real usage.
func Append(product, tool string, args map[string]any, ok, dryRun bool) {
	if dryRun || !Enabled() || tool == "" {
		return
	}
	defer func() { _ = recover() }()

	rec := Record{
		TS:         time.Now().Format(time.RFC3339),
		Product:    product,
		Tool:       tool,
		ArgKeys:    argKeys(args),
		SampleArgs: sampleArgs(args),
		OK:         ok,
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}

	dir := config.DefaultConfigDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	maybeNotice(dir)

	f, err := os.OpenFile(filepath.Join(dir, logFileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

// argKeys returns the sorted set of argument keys.
func argKeys(args map[string]any) []string {
	if len(args) == 0 {
		return nil
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sampleArgs returns the subset of args whose values are safe to record: short,
// whitespace-free ID/enum-like scalars on non-sensitive keys. Everything else
// (free text, slices, objects, long strings) is dropped.
func sampleArgs(args map[string]any) map[string]string {
	out := map[string]string{}
	for k, v := range args {
		normalized := normalizeKey(k)
		if isSensitiveKey(normalized) || !safeSampleKey(normalized) {
			continue
		}
		s, ok := safeScalar(v)
		if !ok {
			continue
		}
		out[k] = s
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeKey(k string) string {
	k = strings.ToLower(strings.TrimSpace(k))
	k = strings.NewReplacer("_", "", "-", "").Replace(k)
	return k
}

func isSensitiveKey(normalized string) bool {
	for _, part := range sensitiveKeyParts {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
}

func safeSampleKey(normalized string) bool {
	return safeScalarKeys[normalized] || strings.HasSuffix(normalized, "id") ||
		strings.HasSuffix(normalized, "uuid") || strings.HasPrefix(normalized, "is") ||
		strings.HasPrefix(normalized, "has") || strings.HasPrefix(normalized, "enable") ||
		strings.HasPrefix(normalized, "include")
}

// safeScalar renders v as a recordable string, or returns ok=false if it is a
// composite, empty, too long, or contains whitespace (i.e. likely free text).
func safeScalar(v any) (string, bool) {
	var s string
	switch t := v.(type) {
	case string:
		s = t
	case bool:
		return fmt.Sprintf("%t", t), true
	case int, int64, int32:
		return fmt.Sprintf("%d", t), true
	case float64:
		return fmt.Sprintf("%v", t), true
	default:
		return "", false // slices, maps, nil
	}
	if s == "" || len(s) > maxSampleValueLen || strings.ContainsAny(s, " \t\n\r") {
		return "", false
	}
	return s, true
}

// maybeNotice prints a one-time notice (to stderr) the first time tracking
// records anything, then drops a marker file so it never repeats.
func maybeNotice(dir string) {
	noticeOnce.Do(func() {
		marker := filepath.Join(dir, noticeFileName)
		if _, err := os.Stat(marker); err == nil {
			return
		}
		fmt.Fprintln(os.Stderr,
			"提示: 你已通过 DWS_USAGE_TRACKING 开启本地使用统计（仅记录命令形状，不记录内容/凭证），用于将高频操作沉淀为自定义 shortcut。"+
				"关闭: 取消该环境变量（默认关闭）；查看/清除: dws shortcut stats [--purge]。")
		_ = os.WriteFile(marker, []byte(time.Now().Format(time.RFC3339)), 0o600)
	})
}
