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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Incremental forwarding: channels whose CLI can emit stream-json get their
// reply streamed into the AI card as it is produced (the hermes/openclaw
// experience), instead of one shot at the end.
//
// Verified stdout protocols (run live against each CLI):
//   - "cc" (claude / codebuddy): JSONL; visible text arrives as
//     {"type":"stream_event","event":{"type":"content_block_delta",
//     "delta":{"type":"text_delta","text":"…"}}} (thinking_delta frames are
//     internal reasoning and are NOT shown); the final authoritative text is
//     {"type":"result","result":"…"}.
//   - "qoder" (qodercli): JSONL; per-turn snapshots arrive as
//     {"type":"assistant","message":{"content":[{"type":"text","text":"…"}]}}
//     and the final answer as {"type":"result","subtype":"success",
//     "message":{"content":[…]}}.

// canStream reports whether this forwarder has an incremental-output mode.
func (f *execForwarder) canStream() bool {
	return len(f.streamArgv) > 0 && f.parser != ""
}

// forwardStream runs the agent in incremental-output mode, calling onDelta
// with the full visible text so far whenever it grows, and returns the final
// reply. Falls back to the one-shot forward when streaming is unsupported.
func (f *execForwarder) forwardStream(ctx context.Context, convID, text string, onDelta func(string)) (string, error) {
	if !f.canStream() || onDelta == nil {
		return f.forward(ctx, convID, text)
	}
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	var args []string
	if f.sessions != nil && strings.TrimSpace(convID) != "" {
		args = append(args, f.sessions.args(convID)...)
	}
	args = append(args, f.streamArgv[1:]...)
	args = append(args, text)
	cmd := exec.CommandContext(ctx, f.streamArgv[0], args...)
	if f.workDir != "" {
		cmd.Dir = f.workDir
	} else {
		cmd.Dir = connectWorkDir()
	}
	if len(f.env) > 0 {
		cmd.Env = append(os.Environ(), f.env...)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var acc strings.Builder // accumulated visible text ("cc" deltas / "qoder" turns)
	finalText := ""
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		delta, final := parseStreamLine(f.parser, line)
		if delta != "" {
			acc.WriteString(delta)
			onDelta(brandReply(f.name, acc.String()))
		}
		if final != "" {
			finalText = final
		}
	}
	waitErr := cmd.Wait()

	if finalText == "" {
		finalText = strings.TrimSpace(acc.String())
	}
	// A bare backend error (e.g. claude's "API Error: 4xx ...") must not be
	// forwarded as the answer (issue #14): return an actionable hint instead.
	if agentReplyIsError(finalText) {
		if f.sessions != nil && strings.TrimSpace(convID) != "" {
			f.sessions.reset(convID)
		}
		return agentBackendErrorReply(finalText), nil
	}
	if finalText != "" {
		return brandReply(f.name, finalText), nil
	}
	if f.sessions != nil && strings.TrimSpace(convID) != "" {
		f.sessions.reset(convID)
	}
	if waitErr != nil {
		msg := waitErr.Error()
		if s := strings.TrimSpace(stderrBuf.String()); s != "" {
			msg = s
		}
		return "", fmt.Errorf("本地 %s agent 调用失败：%s", f.name, truncateRunes(msg, 300))
	}
	return "（本地 agent 无文本输出）", nil
}

// parseStreamLine extracts (visible-text delta, final text) from one JSONL
// line, per parser protocol. Either return value may be empty.
func parseStreamLine(parser, line string) (delta, final string) {
	switch parser {
	case "cc":
		var ev struct {
			Type  string `json:"type"`
			Event struct {
				Type  string `json:"type"`
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			} `json:"event"`
			Result string `json:"result"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil {
			return "", ""
		}
		switch ev.Type {
		case "stream_event":
			if ev.Event.Type == "content_block_delta" && ev.Event.Delta.Type == "text_delta" {
				return ev.Event.Delta.Text, ""
			}
		case "result":
			return "", strings.TrimSpace(ev.Result)
		}
	case "qoder":
		var ev struct {
			Type    string `json:"type"`
			Subtype string `json:"subtype"`
			Result  string `json:"result"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil {
			return "", ""
		}
		text := func() string {
			var b strings.Builder
			for _, c := range ev.Message.Content {
				if c.Type == "text" {
					b.WriteString(c.Text)
				}
			}
			return strings.TrimSpace(b.String())
		}
		switch {
		case ev.Type == "result" && ev.Subtype == "success":
			if t := text(); t != "" {
				return "", t
			}
			return "", strings.TrimSpace(ev.Result)
		case ev.Type == "assistant":
			if t := text(); t != "" {
				// Turn-level snapshot: append as a paragraph.
				return t + "\n\n", ""
			}
		}
	}
	return "", ""
}
