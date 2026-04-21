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

// Package edition provides an extension-point mechanism that allows private
// overlay modules (e.g. an internal distribution) to customise CLI behaviour
// without modifying the open-source core. The open-source build uses the
// zero-value defaults; an overlay calls Override before Execute.
package edition

import (
	"context"
	"sync"

	"github.com/spf13/cobra"
)

// ServerInfo describes a static MCP server endpoint.
type ServerInfo struct {
	ID       string
	Name     string
	Endpoint string
	Prefixes []string
}

// ContentBlock is a single content item in a ToolResult.
type ContentBlock struct {
	Type string
	Text string
}

// ToolResult holds the response from an MCP tool call.
type ToolResult struct {
	Content []ContentBlock
}

// ToolCaller abstracts MCP tool invocation so private overlays can call MCP
// tools without importing internal packages. The open-source core provides a
// concrete adapter wrapping executor.Runner.
type ToolCaller interface {
	// CallTool invokes an MCP tool by product ID and tool name.
	CallTool(ctx context.Context, productID, toolName string, args map[string]any) (*ToolResult, error)
	// Format returns the current output format ("json", "table", "raw").
	Format() string
	// DryRun returns true when --dry-run is active.
	DryRun() bool
}

// Hooks groups all edition-specific behavioural overrides. Zero values
// fall back to open-source defaults so the struct is safe to use as-is.
type Hooks struct {
	// --- identity ---
	Name           string // "open" (default) / overlay identifier
	ScenarioCode   string // injected into x-dingtalk-scenario-code header
	DingTalkSource string // when non-empty, overrides x-dingtalk-source header; defaults to "github"

	// --- runtime mode ---
	IsEmbedded     bool // true when running inside a host application
	HideAuthLogin  bool // true suppresses the "dws auth login" command
	AutoPurgeToken bool // true deletes local token data on expiry

	// --- paths ---
	ConfigDir func() string // custom config directory; nil → ~/.dws

	// --- HTTP headers ---
	MergeHeaders func(base map[string]string) map[string]string

	// --- auth ---
	AuthClientID      string // OAuth client ID for device-flow authorisation
	AuthClientFromMCP bool   // true → fetch client ID from MCP at runtime
	OnAuthError       func(configDir string, err error) error
	TokenProvider     func(ctx context.Context, fallback func() (string, error)) (string, error)

	// --- token persistence (overlay-managed keychain / encrypted storage) ---
	SaveToken   func(configDir string, data []byte) error // persist token blob
	LoadToken   func(configDir string) ([]byte, error)    // retrieve token blob
	DeleteToken func(configDir string) error              // remove persisted token

	// --- MCP result classification ---
	// ClassifyToolResult inspects raw MCP tool-call content and returns a typed
	// error (e.g. PATError, CLIError) when the response contains a known
	// gateway-auth or PAT-permission failure. nil → no special handling.
	ClassifyToolResult func(content map[string]any) error

	// --- product & endpoint ---
	StaticServers         func() []ServerInfo                          // non-nil → skip Market discovery
	VisibleProducts       func() []string                              // non-nil → override help visibility
	RegisterExtraCommands func(root *cobra.Command, caller ToolCaller) // register overlay-only commands

	// AfterPersistentPreRun runs at the end of the root PersistentPreRunE after
	// global setup (OAuth flag overrides, log level, output sink). Overlays use
	// this for clients that bypass the MCP runner (e.g. A2A gateway).
	AfterPersistentPreRun func(cmd *cobra.Command, args []string) error
}

var (
	mu      sync.RWMutex
	current = defaultHooks()
)

// Get returns the active edition hooks (never nil).
func Get() *Hooks {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Override replaces the active edition hooks. Must be called before Execute.
func Override(h *Hooks) {
	if h == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	current = h
}
