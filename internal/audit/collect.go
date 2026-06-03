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

package audit

import (
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
)

// Environment variables that drive auditing. All default OFF: the CLI emits
// nothing unless the deploying organization opts in, and the forward target is
// always the organization's own endpoint — never a vendor default.
const (
	// EnvEnabled turns the local audit file on ("true"/"1").
	EnvEnabled = "DWS_AUDIT_ENABLED"
	// EnvForwardURL points at the ORGANIZATION's own audit sink. Empty = no
	// forwarding (local file only).
	EnvForwardURL = "DWS_AUDIT_FORWARD_URL"
	// EnvForwardToken is the bearer the org uses to authenticate to its sink.
	EnvForwardToken = "DWS_AUDIT_FORWARD_TOKEN"
	// EnvForwardRedact selects the off-box redaction tier: none|hashed|minimal.
	// Defaults to "none" because the org's own sink is inside its trust
	// boundary; set hashed/minimal to ship less.
	EnvForwardRedact = "DWS_AUDIT_FORWARD_REDACT"
	// EnvRedactSalt salts the hashed tier so correlation is possible without
	// raw content. Required when redact=hashed.
	EnvRedactSalt = "DWS_AUDIT_REDACT_SALT"
	// EnvDeviceFingerprint opts in to collecting device_id / sn_no (PIPL
	// personal information). Default off.
	EnvDeviceFingerprint = "DWS_AUDIT_DEVICE_FINGERPRINT"
	// EnvNLIntent carries the user's natural-language request, injected by the
	// orchestrating agent/skill. The CLI cannot verify it (provenance=agent).
	EnvNLIntent = "DWS_AUDIT_NL_INTENT"
)

func init() {
	for _, it := range []configmeta.ConfigItem{
		{Name: EnvEnabled, Category: configmeta.CategorySecurity, Description: "启用本地审计日志(JSONL)", Example: "true"},
		{Name: EnvForwardURL, Category: configmeta.CategorySecurity, Description: "审计转发目标(企业自有 sink，非厂商默认)", Example: "https://audit.internal.example.com/dws"},
		{Name: EnvForwardToken, Category: configmeta.CategorySecurity, Description: "企业审计 sink 的 Bearer 鉴权", Example: "xxxxx"},
		{Name: EnvForwardRedact, Category: configmeta.CategorySecurity, Description: "转发脱敏档: none|hashed|minimal", Example: "none"},
		{Name: EnvRedactSalt, Category: configmeta.CategorySecurity, Description: "hashed 档的加盐值", Example: "tenant-salt"},
		{Name: EnvDeviceFingerprint, Category: configmeta.CategorySecurity, Description: "采集 device_id/sn_no(PIPL 个人信息，默认关)", Example: "true"},
		{Name: EnvNLIntent, Category: configmeta.CategorySecurity, Description: "上层 agent 注入的自然语言原文(provenance=agent)", Example: "把上周听记导出"},
	} {
		configmeta.Register(it)
	}
}

// Enabled reports whether auditing should run at all.
func Enabled() bool {
	return truthy(os.Getenv(EnvEnabled))
}

// DeviceFingerprintEnabled reports the opt-in for device_id/sn_no collection.
func DeviceFingerprintEnabled() bool {
	return truthy(os.Getenv(EnvDeviceFingerprint))
}

// NLIntent returns the agent-injected natural-language request (may be empty).
func NLIntent() string {
	return os.Getenv(EnvNLIntent)
}

// redactLevelFromEnv maps the env string to a RedactLevel (default none).
func redactLevelFromEnv() RedactLevel {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvForwardRedact))) {
	case "hashed":
		return RedactHashed
	case "minimal":
		return RedactMinimal
	default:
		return RedactNone
	}
}

// BuildSink assembles the active sink from env: a local FileSink (writing to
// fileW, the operator-owned durable file) plus, when EnvForwardURL is set, a
// RedactingSink wrapping an HTTPForwarder to the organization's endpoint. When
// auditing is disabled or fileW is nil and no forward URL is set, returns
// NopSink so callers never need a nil check.
func BuildSink(fileW interface{ Write([]byte) (int, error) }) Sink {
	if !Enabled() {
		return NopSink{}
	}
	var sinks []Sink
	if fileW != nil {
		sinks = append(sinks, NewFileSink(fileW))
	}
	if url := strings.TrimSpace(os.Getenv(EnvForwardURL)); url != "" {
		fwd := &RedactingSink{
			Inner: NewHTTPForwarder(url, os.Getenv(EnvForwardToken)),
			Level: redactLevelFromEnv(),
			Salt:  os.Getenv(EnvRedactSalt),
		}
		sinks = append(sinks, fwd)
	}
	switch len(sinks) {
	case 0:
		return NopSink{}
	case 1:
		return sinks[0]
	default:
		return &MultiSink{Sinks: sinks}
	}
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
