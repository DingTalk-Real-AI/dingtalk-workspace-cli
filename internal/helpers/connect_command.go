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

package helpers

import "strings"

// connectControlAction is a built-in slash command a user can type in the chat
// to control the connector's per-conversation session, instead of a message
// that gets forwarded to the agent. Modelled on codex / OpenClaw connect's
// preset commands: the set is fixed (never dynamically extended) so the bot's
// behaviour stays predictable.
type connectControlAction struct {
	// name is the canonical action ("new" | "clear"). Both reset the current
	// conversation's session today (the next message starts fresh with no prior
	// context), but they stay distinct actions so the ack wording can differ and
	// a future channel can diverge their behaviour.
	name string
	// ack is the user-facing confirmation sent back into the chat (Chinese, for
	// DingTalk users). It is never forwarded to the agent.
	ack string
}

// resetsSession reports whether this action should drop the conversation's
// agent session. Both built-in commands do today; kept as a predicate so adding
// a non-resetting command later does not require touching the call site.
func (a connectControlAction) resetsSession() bool {
	return a.name == "new" || a.name == "clear"
}

// connectControlCommands maps the recognised slash tokens to their action. The
// map is the single source of truth for "which commands exist" — both the
// parser and any help text should read from it. "/new", "/start" and "/reset"
// are aliases for opening a fresh session; "/clear" wipes the current one.
var connectControlCommands = map[string]connectControlAction{
	"/new":   {name: "new", ack: "🆕 已为你开启新会话，之前的上下文不再带入。"},
	"/start": {name: "new", ack: "🆕 已为你开启新会话，之前的上下文不再带入。"},
	"/reset": {name: "new", ack: "🆕 已为你开启新会话，之前的上下文不再带入。"},
	"/clear": {name: "clear", ack: "🧹 已清空当前对话的上下文，我们从头开始。"},
}

// parseConnectControlCommand recognises a built-in slash command. It only
// matches when the WHOLE trimmed message is exactly one known token (case
// insensitive), so a normal question that merely starts with a slash — e.g.
// "/new 这个功能怎么实现?" — is forwarded to the agent untouched rather than
// silently swallowed as a command. Returns (action, true) on a match.
func parseConnectControlCommand(text string) (connectControlAction, bool) {
	token := strings.ToLower(strings.TrimSpace(text))
	if token == "" || !strings.HasPrefix(token, "/") {
		return connectControlAction{}, false
	}
	// A command is a bare token: reject anything carrying arguments/whitespace.
	if strings.ContainsAny(token, " \t\n") {
		return connectControlAction{}, false
	}
	action, ok := connectControlCommands[token]
	return action, ok
}
