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
	"strings"
	"time"
)

// atPollInterval is how often we check for new @-mentions (bot-at-bot).
const atPollInterval = 5 * time.Second

// atPollWindow is how far back we look on each poll tick.
const atPollWindow = 30 * time.Second

var (
	atPollExecutable     = os.Executable
	atPollCommandContext = exec.CommandContext
	atPollTicker         = func(interval time.Duration) (<-chan time.Time, func()) {
		ticker := time.NewTicker(interval)
		return ticker.C, ticker.Stop
	}
)

// atMentionPoller periodically queries the "search_at_me_message" API
// using the logged-in user's token and feeds new messages into the
// connect forwarder — supplementing the Stream callback which DingTalk
// does NOT deliver for bot-to-bot @-mentions (anti-loop policy).
type atMentionPoller struct {
	clientID  string
	botClient *aiCardClient
	fwd       forwarder
	queue     *convQueue
	dedup     *msgDedup
	health    *connectHealth
	extras    *connectExtras
	channel   string
}

type atMentionMessage struct {
	MsgID              string `json:"msgId"`
	OpenConversationID string `json:"openConversationId"`
	SenderStaffID      string `json:"senderStaffId"`
	SenderNick         string `json:"senderNick"`
	Content            string `json:"content"`
	ContentType        string `json:"contentType"`
	CreateAt           int64  `json:"createAt"`
	ConversationType   string `json:"conversationType"`
}

func (p *atMentionPoller) start(ctx context.Context) <-chan struct{} {
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-helperAfter(3 * time.Second):
		case <-ctx.Done():
			return
		}
		fmt.Fprintf(os.Stderr, "[connect][at-poll] 已启动 @消息轮询（间隔 %s）\n", atPollInterval)
		ticks, stopTicker := atPollTicker(atPollInterval)
		defer stopTicker()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticks:
				p.poll(ctx)
			}
		}
	}()
	return stopped
}

func (p *atMentionPoller) poll(ctx context.Context) {
	exe, err := atPollExecutable()
	if err != nil {
		return
	}
	now := time.Now()
	start := now.Add(-atPollWindow).Format("2006-01-02T15:04:05+08:00")
	end := now.Format("2006-01-02T15:04:05+08:00")

	cmd := atPollCommandContext(ctx, exe, "chat", "message", "list-mentions",
		"--start", start,
		"--end", end,
		"--limit", "50",
		"--cursor", "0",
		"--format", "json",
	)
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return
	}

	var result struct {
		Result struct {
			Items   []atMentionMessage `json:"items"`
			HasMore bool               `json:"hasMore"`
		} `json:"result"`
		Success bool `json:"success"`
	}
	if json.Unmarshal(out, &result) != nil || !result.Success {
		return
	}

	for _, msg := range result.Result.Items {
		if msg.MsgID == "" {
			continue
		}
		if !p.dedup.first(msg.MsgID) {
			continue
		}
		if msg.ConversationType != "2" && msg.ConversationType != "" {
			continue
		}
		p.handleMessage(ctx, msg)
	}
}

func (p *atMentionPoller) handleMessage(ctx context.Context, msg atMentionMessage) {
	text := extractAtPollText(msg.Content, msg.ContentType)
	if text == "" {
		return
	}
	sender := msg.SenderNick
	if sender == "" {
		sender = msg.SenderStaffID
	}
	fmt.Fprintf(os.Stderr, "[connect][at-poll] 收到 @消息 from=%s convId=%s msgId=%s: %s\n",
		sender, msg.OpenConversationID, msg.MsgID, truncateRunes(text, 80))
	p.health.onPush()

	convID := msg.OpenConversationID
	if convID == "" {
		convID = msg.SenderStaffID
	}

	turn := connectQueuedTurn{
		convID:           convID,
		text:             text,
		msgID:            msg.MsgID,
		senderStaffID:    strings.TrimSpace(msg.SenderStaffID),
		conversationID:   strings.TrimSpace(msg.OpenConversationID),
		conversationType: strings.TrimSpace(msg.ConversationType),
	}
	p.queue.submit(turn, func(turns []connectQueuedTurn) {
		turn := mergeConnectQueuedTurns(turns)
		if len(turns) > 1 {
			fmt.Fprintf(os.Stderr, "[connect][at-poll] 合并 %d 条待处理 @消息 (convId=%s, latestMsgId=%s)\n", len(turns), turn.convID, turn.msgID)
		}
		text := turn.text
		convID := turn.convID
		senderStaffID := turn.senderStaffID
		openConversationID := turn.conversationID
		if p.extras.gate.enabled() {
			if ok, reason := p.extras.gate.allow(senderStaffID, "2", openConversationID); !ok {
				fmt.Fprintf(os.Stderr, "[connect][at-poll] 已拦截消息（%s）: staffId=%s convId=%s\n",
					reason, senderStaffID, openConversationID)
				return
			}
		}

		prompt := text
		if p.extras.kb != nil {
			prompt = p.extras.kb.augment(prompt)
		}
		if persona := strings.TrimSpace(p.extras.persona); persona != "" {
			prompt = persona + "\n\n" + prompt
		}

		started := time.Now()
		var reply string
		var err error
		if sf, ok := p.fwd.(streamingForwarder); ok && sf.canStream() {
			reply, err = sf.forwardStream(context.Background(), convID, prompt, nil)
		} else {
			reply, err = p.fwd.forward(context.Background(), convID, prompt)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "[connect][at-poll] 转发失败 (%s, 耗时 %s): %v\n",
				p.channel, time.Since(started).Round(time.Millisecond), err)
			reply = fmt.Sprintf("（%s 调用失败：%v）", p.channel, err)
			p.health.onError(err)
		} else {
			fmt.Fprintf(os.Stderr, "[connect][at-poll] agent 已回复 (%s, 耗时 %s): %s\n",
				p.channel, time.Since(started).Round(time.Millisecond), truncateRunes(reply, 80))
			p.health.onReply()
		}

		if reply != "" && openConversationID != "" {
			if serr := p.sendGroupReply(ctx, openConversationID, reply); serr != nil {
				fmt.Fprintf(os.Stderr, "[connect][at-poll] 群回复发送失败: %v\n", serr)
			}
		}
	})
}

func (p *atMentionPoller) sendGroupReply(ctx context.Context, openConversationID, text string) error {
	msgParam, _ := json.Marshal(map[string]string{
		"title": p.channel,
		"text":  text,
	})
	payload := map[string]any{
		"robotCode":          p.clientID,
		"openConversationId": openConversationID,
		"msgKey":             "sampleMarkdown",
		"msgParam":           string(msgParam),
	}
	token, err := p.botClient.accessToken(ctx)
	if err != nil {
		return fmt.Errorf("get app token: %w", err)
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		dingtalkCardAPIBase+"/v1.0/robot/groupMessages/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateRunes(string(raw), 200))
	}
	return nil
}

func extractAtPollText(content, contentType string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	var parsed map[string]any
	if json.Unmarshal([]byte(content), &parsed) == nil {
		if t, ok := parsed["text"].(string); ok {
			return strings.TrimSpace(t)
		}
		if t, ok := parsed["content"].(string); ok {
			return strings.TrimSpace(t)
		}
	}
	return content
}
