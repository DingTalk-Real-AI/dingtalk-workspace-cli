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

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
)

const (
	envDWSAgentVersion     = "DWS_AGENT_VER"
	envDWSAgentExt         = "DWS_AGENT_EXT"
	maxAgentVersionBytes   = 64
	maxAgentExtensionBytes = 8 * 1024
)

var agentVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

type agentMetadataSnapshot struct {
	version    string
	ext        string
	versionErr error
	extErr     error
}

type agentMetadataSnapshotContextKey struct{}

func (snapshot agentMetadataSnapshot) validationError() error {
	if snapshot.versionErr != nil {
		return snapshot.versionErr
	}
	return snapshot.extErr
}

func contextWithAgentMetadataSnapshot(ctx context.Context, snapshot agentMetadataSnapshot) context.Context {
	return context.WithValue(ctx, agentMetadataSnapshotContextKey{}, snapshot)
}

func agentMetadataSnapshotFromContext(ctx context.Context) (agentMetadataSnapshot, bool) {
	if ctx == nil {
		return agentMetadataSnapshot{}, false
	}
	snapshot, ok := ctx.Value(agentMetadataSnapshotContextKey{}).(agentMetadataSnapshot)
	return snapshot, ok
}

func init() {
	configmeta.Register(configmeta.ConfigItem{
		Name:        envDWSAgentVersion,
		Category:    configmeta.CategoryExternal,
		Description: "调用 DWS 的 Agent 版本；仅作为 x-dws-agent-ver 透传到非插件 MCP 请求",
		Example:     "1.2.3-beta.1+build.7",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        envDWSAgentExt,
		Category:    configmeta.CategoryExternal,
		Description: "调用 DWS 的 Agent 扩展上下文 JSON；仅作为 x-dws-agent-ext 透传到非插件 MCP 请求",
		Example:     `{"umt":"<token>","miniwua":"<token>","ua":"agent/1.0"}`,
		Sensitive:   true,
	})
}

// parseAgentVersion normalizes and validates the caller-declared Agent
// version. Only surrounding ASCII spaces and tabs are trimmed. An unset or
// ASCII-whitespace-only value means "do not emit".
func parseAgentVersion(raw string) (string, error) {
	value := strings.Trim(raw, " \t")
	if value == "" {
		return "", nil
	}
	if len(value) > maxAgentVersionBytes || !agentVersionPattern.MatchString(value) {
		return "", invalidAgentVersionError()
	}
	return value, nil
}

// parseAgentExt validates one generic JSON object and returns its compact
// one-line representation. Raw control characters other than horizontal tab
// are rejected before JSON parsing; escaped JSON control characters remain
// valid because they are safe on the HTTP header wire.
func parseAgentExt(raw string) (string, error) {
	if len(raw) > maxAgentExtensionBytes || !utf8.ValidString(raw) {
		return "", invalidAgentExtError()
	}
	for _, r := range raw {
		if unicode.IsControl(r) && r != '\t' {
			return "", invalidAgentExtError()
		}
	}

	value := strings.Trim(raw, " \t")
	if value == "" {
		return "", nil
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(value)); err != nil {
		return "", invalidAgentExtError()
	}
	compactBytes := compact.Bytes()
	if len(compactBytes) > maxAgentExtensionBytes || len(compactBytes) < 2 || compactBytes[0] != '{' {
		return "", invalidAgentExtError()
	}
	return compact.String(), nil
}

func invalidAgentVersionError() error {
	return apperrors.NewValidation(
		"DWS_AGENT_VER must be at most 64 bytes and match ^[A-Za-z0-9][A-Za-z0-9._+-]*$",
		apperrors.WithReason("invalid_agent_version"),
	)
}

func invalidAgentExtError() error {
	return apperrors.NewValidation(
		"DWS_AGENT_EXT must be a UTF-8 JSON object of at most 8192 bytes without raw control characters",
		apperrors.WithReason("invalid_agent_ext"),
	)
}

// readAgentMetadataSnapshot reads both environment variables from one
// os.Environ snapshot, then parses them once. Normal CLI execution retains the
// validated result through the invocation so hooks and transport observe the
// same pair even in an embedding process that mutates its environment.
func readAgentMetadataSnapshot() agentMetadataSnapshot {
	var rawVersion, rawExt string
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		switch key {
		case envDWSAgentVersion:
			rawVersion = value
		case envDWSAgentExt:
			rawExt = value
		}
	}
	version, versionErr := parseAgentVersion(rawVersion)
	ext, extErr := parseAgentExt(rawExt)
	return agentMetadataSnapshot{
		version:    version,
		ext:        ext,
		versionErr: versionErr,
		extErr:     extErr,
	}
}

// removeAgentMetadataHeaders removes every case variant so edition or
// credential hooks cannot smuggle MCP-only metadata into shared transports.
func removeAgentMetadataHeaders(headers map[string]string) {
	for key := range headers {
		if strings.EqualFold(key, transport.HeaderAgentVersion) ||
			strings.EqualFold(key, transport.HeaderAgentExt) {
			delete(headers, key)
		}
	}
}

// applyAgentMetadataHeaders applies validated environment values as the final
// authority for non-plugin MCP requests. Invalid values are omitted on
// library paths that bypass root validation; normal CLI execution rejects
// them before hooks or network access.
func applyAgentMetadataHeaders(headers map[string]string) map[string]string {
	return applyAgentMetadataSnapshot(headers, readAgentMetadataSnapshot())
}

func applyAgentMetadataSnapshot(headers map[string]string, snapshot agentMetadataSnapshot) map[string]string {
	removeAgentMetadataHeaders(headers)

	if (snapshot.versionErr != nil || snapshot.version == "") && (snapshot.extErr != nil || snapshot.ext == "") {
		return headers
	}
	if headers == nil {
		headers = make(map[string]string)
	}
	if snapshot.versionErr == nil && snapshot.version != "" {
		headers[transport.HeaderAgentVersion] = snapshot.version
	}
	if snapshot.extErr == nil && snapshot.ext != "" {
		headers[transport.HeaderAgentExt] = snapshot.ext
	}
	return headers
}

// resolveMCPRequestHeaders adds Agent version and extension metadata only to
// the built-in DingTalk MCP request path. Shared identity consumers (notably
// A2A) continue to use resolveIdentityHeaders and never receive these fields.
func resolveMCPRequestHeaders() map[string]string {
	return resolveMCPRequestHeadersWithSnapshot(readAgentMetadataSnapshot())
}

func resolveMCPRequestHeadersWithSnapshot(snapshot agentMetadataSnapshot) map[string]string {
	return applyAgentMetadataSnapshot(resolveIdentityHeaders(), snapshot)
}

// resolveMCPRequestHeadersForInvocation resolves one immutable Header snapshot
// for an invocation. The helper-only mcp-meta server performs endpoint
// discovery rather than an ordinary MCP product call, so caller-declared
// Agent metadata must not cross that boundary.
func resolveMCPRequestHeadersForInvocation(invocation executor.Invocation, snapshots ...agentMetadataSnapshot) map[string]string {
	headers := resolveIdentityHeaders()
	if strings.EqualFold(strings.TrimSpace(invocation.CanonicalProduct), mcpMetaServerID) {
		return headers
	}
	snapshot := readAgentMetadataSnapshot()
	if len(snapshots) > 0 {
		snapshot = snapshots[0]
	}
	return applyAgentMetadataSnapshot(headers, snapshot)
}

// pluginRequestHeaders returns a private, sanitized copy of plugin-owned
// Headers. Third-party plugins never receive DWS-owned Agent metadata, even if
// their manifest tries to declare the reserved Header names itself.
func pluginRequestHeaders(pluginAuth *PluginAuth) map[string]string {
	if pluginAuth == nil || len(pluginAuth.ExtraHeaders) == 0 {
		return nil
	}
	headers := make(map[string]string, len(pluginAuth.ExtraHeaders))
	for key, value := range pluginAuth.ExtraHeaders {
		headers[key] = value
	}
	removeAgentMetadataHeaders(headers)
	if len(headers) == 0 {
		return nil
	}
	return headers
}
