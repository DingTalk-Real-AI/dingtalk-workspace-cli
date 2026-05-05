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
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
)

// Regression for the chat/bot tool routing bug: when the `chat` envelope
// declares toolOverrides with `serverOverride: "bot"` (e.g. `search_my_robots`,
// `send_message_by_custom_robot`), those tool names must NOT be registered
// into `dynamicToolEndpoints` pointing at chat's endpoint. Otherwise the
// tool-level Priority 1 lookup in `directRuntimeEndpoint` returns chat's URL
// even when the invocation's CanonicalProduct is "bot", causing the Portal to
// respond with `PARAM_ERROR - 未找到指定工具` because chat's mcpId has no such
// tool.
//
// Owner (bot envelope) still registers the tool (no serverOverride on the bot
// side), so product-level and tool-level lookups both resolve correctly.

const (
	testBotEndpoint  = "https://pre-mcp-gw.dingtalk.com/server/4717d5cbb92ecdebd89c174e4331dc17207208a97622e2004cac49c0fbedc9d1"
	testChatEndpoint = "https://pre-mcp-gw.dingtalk.com/server/0a1609437385696b77fc4771c3ddaf5656b487f809966c0cc8d4755e7b1d3b74"
)

// botDescriptor returns a minimal `bot` server descriptor that owns the
// `search_my_robots` + `send_message_by_custom_robot` tools (no
// serverOverride — bot is the real owner).
func botDescriptor() market.ServerDescriptor {
	return market.ServerDescriptor{
		Endpoint: testBotEndpoint,
		CLI: market.CLIOverlay{
			ID: "bot",
			ToolOverrides: map[string]market.CLIToolOverride{
				"search_my_robots":             {CLIName: "search"},
				"send_message_by_custom_robot": {CLIName: "send-by-webhook"},
				"add_robot_to_group":           {CLIName: "add-bot"},
			},
		},
	}
}

// chatDescriptor returns a minimal `chat` server descriptor whose
// toolOverrides include bot-owned tools via `serverOverride: "bot"`, plus a
// chat-native tool (`search_groups_by_keyword`) that must remain routed to
// chat's endpoint.
func chatDescriptor() market.ServerDescriptor {
	return market.ServerDescriptor{
		Endpoint: testChatEndpoint,
		CLI: market.CLIOverlay{
			ID:      "chat",
			Command: "chat",
			ToolOverrides: map[string]market.CLIToolOverride{
				"search_groups_by_keyword": {CLIName: "search"},
				"search_my_robots": {
					CLIName:        "search",
					ServerOverride: "bot",
				},
				"send_message_by_custom_robot": {
					CLIName:        "send-by-webhook",
					ServerOverride: "bot",
				},
				"add_robot_to_group": {
					CLIName:        "add-bot",
					ServerOverride: "bot",
				},
			},
		},
	}
}

// withCleanDynamicRegistry snapshots and restores the package-level dynamic
// registries so parallel/other tests aren't affected by this case's mutations.
func withCleanDynamicRegistry(t *testing.T) {
	t.Helper()
	dynamicMu.Lock()
	prev := struct {
		endpoints     map[string]string
		products      map[string]bool
		aliases       map[string]string
		toolEndpoints map[string]string
	}{dynamicEndpoints, dynamicProducts, dynamicAliases, dynamicToolEndpoints}
	dynamicEndpoints = nil
	dynamicProducts = nil
	dynamicAliases = nil
	dynamicToolEndpoints = nil
	dynamicMu.Unlock()
	t.Cleanup(func() {
		dynamicMu.Lock()
		dynamicEndpoints = prev.endpoints
		dynamicProducts = prev.products
		dynamicAliases = prev.aliases
		dynamicToolEndpoints = prev.toolEndpoints
		dynamicMu.Unlock()
	})
}

func assertEndpoint(t *testing.T, productID, toolName, want string) {
	t.Helper()
	got, ok := directRuntimeEndpoint(productID, toolName)
	if !ok {
		t.Fatalf("directRuntimeEndpoint(%q, %q) returned ok=false", productID, toolName)
	}
	if got != want {
		t.Fatalf("directRuntimeEndpoint(%q, %q) = %q, want %q", productID, toolName, got, want)
	}
}

// TestSetDynamicServers_ServerOverrideDoesNotHijackToolEndpoint verifies that
// chat's serverOverride entries cannot steal bot-owned tool routes, regardless
// of registration order.
func TestSetDynamicServers_ServerOverrideDoesNotHijackToolEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		servers []market.ServerDescriptor
	}{
		{
			name:    "bot first, chat second",
			servers: []market.ServerDescriptor{botDescriptor(), chatDescriptor()},
		},
		{
			name:    "chat first, bot second",
			servers: []market.ServerDescriptor{chatDescriptor(), botDescriptor()},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withCleanDynamicRegistry(t)
			SetDynamicServers(tc.servers)

			// Bot-owned tools must route to bot's endpoint even though chat
			// declares toolOverrides for them (with serverOverride="bot").
			assertEndpoint(t, "bot", "search_my_robots", testBotEndpoint)
			assertEndpoint(t, "bot", "send_message_by_custom_robot", testBotEndpoint)
			assertEndpoint(t, "bot", "add_robot_to_group", testBotEndpoint)

			// Chat-native tools must still route to chat.
			assertEndpoint(t, "chat", "search_groups_by_keyword", testChatEndpoint)

			// Product-level fallback for bot (no tool name) must also return
			// bot's endpoint.
			assertEndpoint(t, "bot", "", testBotEndpoint)
		})
	}
}

// TestAppendDynamicServer_ServerOverrideDoesNotHijackToolEndpoint exercises
// the plugin-injection path (`AppendDynamicServer`) which has the same
// `toolOverrides` registration loop as `SetDynamicServers`. Chat's
// serverOverride entries must not overwrite bot's tool → endpoint mapping.
func TestAppendDynamicServer_ServerOverrideDoesNotHijackToolEndpoint(t *testing.T) {
	orders := [][]market.ServerDescriptor{
		{botDescriptor(), chatDescriptor()},
		{chatDescriptor(), botDescriptor()},
	}

	for _, servers := range orders {
		t.Run("", func(t *testing.T) {
			withCleanDynamicRegistry(t)
			for _, s := range servers {
				AppendDynamicServer(s)
			}

			assertEndpoint(t, "bot", "search_my_robots", testBotEndpoint)
			assertEndpoint(t, "bot", "send_message_by_custom_robot", testBotEndpoint)
			assertEndpoint(t, "chat", "search_groups_by_keyword", testChatEndpoint)
		})
	}
}

// --- Issue #219 regression tests: cross-product tool name collision ---
//
// When two different products register tools with the same name (e.g. drive
// and doc both have "create_folder"), the product-level endpoint must win
// when the caller already knows the productID. Otherwise the tool-level map
// (last-writer-wins) routes the invocation to the wrong MCP server.

const (
	testDriveEndpoint = "https://mcp-gw.dingtalk.com/server/drive-hash"
	testDocEndpoint   = "https://mcp-gw.dingtalk.com/server/doc-hash"
)

func driveDescriptor() market.ServerDescriptor {
	return market.ServerDescriptor{
		Endpoint: testDriveEndpoint,
		CLI: market.CLIOverlay{
			ID:      "drive",
			Command: "drive",
			ToolOverrides: map[string]market.CLIToolOverride{
				"create_folder":   {CLIName: "mkdir"},
				"list_files":      {CLIName: "list"},
				"download_file":   {CLIName: "download"},
				"get_upload_info": {CLIName: "upload-info"},
			},
		},
	}
}

func docDescriptor() market.ServerDescriptor {
	return market.ServerDescriptor{
		Endpoint: testDocEndpoint,
		CLI: market.CLIOverlay{
			ID:      "doc",
			Command: "doc",
			ToolOverrides: map[string]market.CLIToolOverride{
				"create_folder":    {CLIName: "create", Group: "folder"},
				"download_file":    {CLIName: "download"},
				"search_documents": {CLIName: "search"},
				"list_nodes":       {CLIName: "list"},
			},
		},
	}
}

// TestDirectRuntimeEndpoint_ProductLevelWinsOverConflictingToolLevel verifies
// that when productID is known and has a registered endpoint, the product-level
// endpoint is used even if the tool-level map points to a different server
// (due to same-name tool collision). This is the core fix for issue #219.
func TestDirectRuntimeEndpoint_ProductLevelWinsOverConflictingToolLevel(t *testing.T) {
	tests := []struct {
		name    string
		servers []market.ServerDescriptor
	}{
		{
			name:    "drive first, doc second",
			servers: []market.ServerDescriptor{driveDescriptor(), docDescriptor()},
		},
		{
			name:    "doc first, drive second",
			servers: []market.ServerDescriptor{docDescriptor(), driveDescriptor()},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withCleanDynamicRegistry(t)
			SetDynamicServers(tc.servers)

			// Drive tools must always route to drive's endpoint regardless of
			// registration order — productID "drive" is known.
			assertEndpoint(t, "drive", "create_folder", testDriveEndpoint)
			assertEndpoint(t, "drive", "download_file", testDriveEndpoint)
			assertEndpoint(t, "drive", "list_files", testDriveEndpoint)
			assertEndpoint(t, "drive", "get_upload_info", testDriveEndpoint)

			// Doc tools must always route to doc's endpoint.
			assertEndpoint(t, "doc", "create_folder", testDocEndpoint)
			assertEndpoint(t, "doc", "download_file", testDocEndpoint)
			assertEndpoint(t, "doc", "search_documents", testDocEndpoint)
			assertEndpoint(t, "doc", "list_nodes", testDocEndpoint)

			// Product-level fallback (no tool name) still works.
			assertEndpoint(t, "drive", "", testDriveEndpoint)
			assertEndpoint(t, "doc", "", testDocEndpoint)
		})
	}
}

// TestDirectRuntimeEndpoint_ToolLevelFallbackWhenProductUnknown verifies that
// tool-level routing still works as a fallback when productID is empty or has
// no registered endpoint (the original design intent for tool-level Priority 1).
func TestDirectRuntimeEndpoint_ToolLevelFallbackWhenProductUnknown(t *testing.T) {
	withCleanDynamicRegistry(t)
	SetDynamicServers([]market.ServerDescriptor{driveDescriptor(), docDescriptor()})

	// When productID is empty, tool-level endpoint is the only option.
	// The actual endpoint depends on registration order (last-writer-wins),
	// but the lookup must succeed.
	endpoint, ok := directRuntimeEndpoint("", "create_folder")
	if !ok {
		t.Fatal("directRuntimeEndpoint(\"\", \"create_folder\") returned ok=false, want ok=true")
	}
	if endpoint != testDriveEndpoint && endpoint != testDocEndpoint {
		t.Fatalf("directRuntimeEndpoint(\"\", \"create_folder\") = %q, want one of drive/doc endpoints", endpoint)
	}

	// Unique tools (no collision) still resolve via tool-level.
	assertEndpoint(t, "", "search_documents", testDocEndpoint)
	assertEndpoint(t, "", "get_upload_info", testDriveEndpoint)
}
