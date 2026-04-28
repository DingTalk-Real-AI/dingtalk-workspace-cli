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
