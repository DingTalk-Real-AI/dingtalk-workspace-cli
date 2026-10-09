// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

// contactFriendTools records the friend-domain MCP tool names served by the
// contact MCP gateway (mcpId 2400). The `+friend-*` shortcuts in
// internal/shortcut/contact/friend.go wrap these tools exclusively — the
// friend workflows have no separate atomic CLI surface — and this registry
// keeps the helper-side tool-name ground truth in sync so the tool-literal
// coverage tests in internal/shortcut/builtin can verify the shortcut wiring
// against the real tool names instead of treating them as hallucinated.
var contactFriendTools = []string{
	"get_friend_list",
	"get_friend_request_list",
	"send_friend_request",
	"accept_friend_request",
	"remove_friend_request",
	"remove_friend",
}
