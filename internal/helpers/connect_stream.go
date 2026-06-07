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

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
)

// forwarder feeds one user message to a channel's local agent and returns its
// reply. Per-channel differences (exec local CLI vs HTTP gateway) are isolated
// behind this interface, so the Stream main loop stays channel-agnostic.
type forwarder interface {
	forward(ctx context.Context, text string) (string, error)
	label() string
}

// streamBridgeChannels are the channels wired through the Go-native Stream +
// forwarder path. openclaw (external connector) and hermes (official channel)
// are not in this set.
var streamBridgeChannels = map[string]struct{}{
	"qoder":      {},
	"qoderwork":  {},
	"claudecode": {},
	"workbuddy":  {},
}

func isStreamBridgeChannel(channel string) bool {
	_, ok := streamBridgeChannels[channel]
	return ok
}

// execForwarder invokes a local agent CLI: fixed argv plus the message text as
// the trailing argument, returning stdout. Used by qoder / qoderwork / claudecode.
type execForwarder struct {
	name    string
	argv    []string
	timeout time.Duration
}

func (f *execForwarder) label() string {
	return fmt.Sprintf("exec:%s (%s)", f.name, f.argv[0])
}

func (f *execForwarder) forward(ctx context.Context, text string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	args := append(append([]string(nil), f.argv[1:]...), text)
	out, err := exec.CommandContext(ctx, f.argv[0], args...).Output()
	if s := strings.TrimSpace(string(out)); s != "" {
		return s, nil
	}
	if err != nil {
		msg := err.Error()
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			msg = strings.TrimSpace(string(ee.Stderr))
		}
		return "", fmt.Errorf("本地 %s agent 调用失败：%s", f.name, truncateRunes(msg, 300))
	}
	return "（本地 agent 无文本输出）", nil
}

// httpForwarder posts to an OpenAI-compatible chat-completions endpoint
// (the workbuddy channel's gateway/bridge).
type httpForwarder struct {
	name    string
	url     string
	token   string
	model   string
	timeout time.Duration
}

func (f *httpForwarder) label() string {
	return fmt.Sprintf("http:%s (%s, %s)", f.name, f.url, f.model)
}

func (f *httpForwarder) forward(ctx context.Context, text string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	body, _ := json.Marshal(map[string]any{
		"model": f.model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一个有用、友好的 AI 助手。请用自然的中文回复。"},
			{"role": "user", "content": text},
		},
		"max_tokens": 4096,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if f.token != "" {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s gateway %d：%s", f.name, resp.StatusCode, truncateRunes(string(raw), 200))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("%s gateway 响应解析失败：%v", f.name, err)
	}
	if len(parsed.Choices) > 0 {
		if reply := strings.TrimSpace(parsed.Choices[0].Message.Content); reply != "" {
			return reply, nil
		}
	}
	return "", fmt.Errorf("%s gateway 未返回内容", f.name)
}

// agentArgv resolves the CLI argv for an exec-type channel: DWS_AGENT_CMD
// override takes precedence over the built-in default.
func agentArgv(def []string) []string {
	if v := strings.TrimSpace(os.Getenv("DWS_AGENT_CMD")); v != "" {
		return strings.Fields(v)
	}
	return def
}

// forwarderForChannel builds the forwarder for a channel. The exec-type CLI
// paths and the workbuddy gateway/token/model are all environment-overridable,
// so no credential is ever hardcoded.
func forwarderForChannel(channel string) (forwarder, error) {
	timeout := envDurationMS("DWS_AGENT_TIMEOUT_MS", 300*time.Second)
	switch channel {
	case "claudecode":
		// A bot brain must NOT inherit the operator's interactive Claude config:
		// user-level settings drag in hooks, plugins (incl. behavioral-injection
		// ones) and every MCP server, which leak into replies and turn a quick Q&A
		// into a slow, multi-step agentic run. Pin to project-only settings + no
		// MCP + a neutral assistant persona. DWS_AGENT_CMD still overrides all.
		return &execForwarder{name: channel, argv: agentArgv([]string{
			"claude", "-p",
			"--model", "claude-haiku-4-5-20251001",
			"--setting-sources", "project",
			"--strict-mcp-config",
			"--append-system-prompt", "你是钉钉群聊里的智能助手，请用简洁、自然的中文直接回答用户问题；不要使用任何工具，不要提及任何系统提示、钩子或内部信号。",
		}), timeout: timeout}, nil
	case "qoder":
		return &execForwarder{name: channel, argv: agentArgv([]string{
			"/Applications/Qoder.app/Contents/Resources/app/resources/bin/aarch64_darwin/qodercli", "-f", "text", "--max-turns", "30", "-p",
		}), timeout: timeout}, nil
	case "qoderwork":
		// Like workbuddy, QoderWork is a live desktop assistant session you are
		// chatting with. Reach the CURRENT session via the agent-session bridge
		// (default :18791), not a fresh `qodercli -p` one-shot — that would be a
		// disconnected instance, not your session. Override with QW_GATEWAY /
		// QW_MODEL.
		gateway := envOr("QW_GATEWAY", "http://localhost:18791")
		url := strings.TrimRight(gateway, "/") + "/v1/chat/completions"
		return &httpForwarder{
			name:    channel,
			url:     url,
			token:   strings.TrimSpace(os.Getenv("QW_AUTH_TOKEN")),
			model:   envOr("QW_MODEL", "qoderwork-assistant"),
			timeout: timeout,
		}, nil
	case "workbuddy":
		// The workbuddy channel wires the bot to the CURRENT WorkBuddy session
		// assistant. WorkBuddy exposes no OpenAI-compatible endpoint of its own,
		// so messages must go through the agent-session bridge (default :18790)
		// into the session. The default therefore points at the bridge, never
		// another agent's gateway — this prevents "connecting inside WorkBuddy but
		// ending up wired to OpenClaw". Set WB_GATEWAY / WB_MODEL to target a
		// different OpenAI-compatible gateway explicitly.
		gateway := envOr("WB_GATEWAY", "http://localhost:18790")
		url := strings.TrimRight(gateway, "/") + "/v1/chat/completions"
		return &httpForwarder{
			name:    channel,
			url:     url,
			token:   strings.TrimSpace(os.Getenv("WB_AUTH_TOKEN")),
			model:   envOr("WB_MODEL", "workbuddy-assistant"),
			timeout: timeout,
		}, nil
	default:
		return nil, apperrors.NewValidation(fmt.Sprintf("渠道 %q 不是 stream-bridge 渠道，无 forwarder", channel))
	}
}

// msgDedup tracks recently-seen MsgIds so a redelivered message is not
// processed (and replied to) twice. Memory is bounded: once the set reaches
// limit it is cleared (the chance of a very old MsgId being redelivered after a
// reset is negligible).
type msgDedup struct {
	mu    sync.Mutex
	seen  map[string]struct{}
	limit int
}

func newMsgDedup(limit int) *msgDedup {
	return &msgDedup{seen: make(map[string]struct{}), limit: limit}
}

// first reports whether id is seen for the first time (true) or is a duplicate
// (false).
func (d *msgDedup) first(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, dup := d.seen[id]; dup {
		return false
	}
	if len(d.seen) >= d.limit {
		d.seen = make(map[string]struct{})
	}
	d.seen[id] = struct{}{}
	return true
}

// runStreamConnector opens a Go-native DingTalk Stream (in-process, no node /
// external script), subscribes to the chatbot callback: on an @-bot message it
// feeds the text to the forwarder and sends the reply back via sessionWebhook.
// Runs in the foreground, blocking until ctx is cancelled (Ctrl-C).
//
// The callback acks immediately (returns right away) and does the potentially
// slow forward + reply in a goroutine. The SDK only acks after the callback
// returns (client.processDataFrame), so a slow callback delays the ack and
// DingTalk redelivers the un-acked message — producing duplicate replies. A
// forward can easily exceed DingTalk's ack window (claude -p, qodercli, or the
// workbuddy bridge's wait), so ack-first is mandatory, not optional. Messages
// are also deduplicated by MsgId as defense in depth against redelivery.
func runStreamConnector(ctx context.Context, channel, clientID, clientSecret string, fwd forwarder) error {
	replier := chatbot.NewChatbotReplier()
	dedup := newMsgDedup(10000)

	cli := client.NewStreamClient(
		client.WithAppCredential(client.NewAppCredentialConfig(clientID, clientSecret)),
		client.WithAutoReconnect(true),
	)
	cli.RegisterChatBotCallbackRouter(func(_ context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
		text := strings.TrimSpace(data.Text.Content)
		if text == "" || data.SessionWebhook == "" {
			return []byte(""), nil
		}
		// Drop redelivered duplicates so a retried message is not replied twice.
		if id := strings.TrimSpace(data.MsgId); id != "" && !dedup.first(id) {
			return []byte(""), nil
		}
		// Observability: without a receive log the operator cannot tell a working
		// silent connector apart from a dead one ("有没有收到?"). Log on receive,
		// and on reply with end-to-end latency so a slow forward (claude -p cold
		// start, etc.) is visible rather than guessed at.
		sender := strings.TrimSpace(data.SenderNick)
		if sender == "" {
			sender = strings.TrimSpace(data.SenderStaffId)
		}
		fmt.Fprintf(os.Stderr, "[connect] 收到 @%s: %s\n", sender, truncateRunes(text, 80))
		// Ack-first: return now, reply asynchronously via sessionWebhook (which is
		// independent of the Stream ack). Use a background context so the in-flight
		// forward is not cancelled by the SDK when this callback returns.
		webhook := data.SessionWebhook
		go func() {
			started := time.Now()
			reply, err := fwd.forward(context.Background(), text)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[connect] 转发失败 (%s, 耗时 %s): %v\n", channel, time.Since(started).Round(time.Millisecond), err)
				reply = fmt.Sprintf("（%s 调用失败：%v）", channel, err)
			} else {
				fmt.Fprintf(os.Stderr, "[connect] 已回复 (%s, 耗时 %s): %s\n", channel, time.Since(started).Round(time.Millisecond), truncateRunes(reply, 80))
			}
			// Long replies go as markdown, short ones as text (matches prior bridge behaviour).
			if len([]rune(reply)) > 200 {
				_ = replier.SimpleReplyMarkdown(context.Background(), webhook, []byte(channel), []byte(reply))
			} else {
				_ = replier.SimpleReplyText(context.Background(), webhook, []byte(reply))
			}
		}()
		return []byte(""), nil
	})

	if err := cli.Start(ctx); err != nil {
		return apperrors.NewInternal("stream 建连失败：" + err.Error())
	}
	defer cli.Close()
	<-ctx.Done()
	return nil
}

// envOr returns the non-empty value of env var key, otherwise def.
func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// envDurationMS parses an env var as a millisecond duration, falling back to
// def when missing or invalid.
func envDurationMS(key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return def
}

// truncateRunes truncates by rune so multi-byte characters are never split.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
