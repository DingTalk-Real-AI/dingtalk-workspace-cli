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
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
)

// AI-card reply support for the stream-bridge channels: instead of a plain
// one-shot text message, the bot answers with a DingTalk AI card that shows a
// "Thinking" state while the local agent runs and flips to done with the
// final content — the same UX the hermes official pipeline provides.
//
// The experience combines the two verified official implementations:
//   - the "🤔Thinking/🥳Done" chip on the user's message is a text emotion
//     (message reaction), ported from the hermes gateway adapter;
//   - the card itself uses the openclaw connector's public template and
//     payload contract (msgContent + flowStatus state machine), because card
//     templates are app-scoped: hermes' own template (c629162a-...) renders
//     "内容加载失败" for any other app — confirmed by A/B on the same robot.
//
// Call order: create instance → deliver → flowStatus INPUTING → streaming
// frames (throttled, isFull) → finalize frame → flowStatus FINISHED.
const (
	// defaultAICardTemplateID is the public AI-card streaming template the
	// openclaw connector ships with (messaging/card.ts) — a best-effort
	// default. Card templates are APP-SCOPED: the reliable production setup is
	// registering an AI-card template under YOUR app in the DingTalk developer
	// console (the hermes docs prescribe exactly this) and passing its ID via
	// --card-template / DWS_CARD_TEMPLATE. Using another app's template (e.g.
	// hermes' c629162a-...) renders "内容加载失败" — confirmed by live A/B on
	// the same robot.
	defaultAICardTemplateID = "02fcf2f4-5e02-4a85-b672-46d1f715543e.schema"

	// aiCardMaxContent mirrors hermes' MAX_MESSAGE_LENGTH truncation.
	aiCardMaxContent = 20000

	// flowStatus states of the AI-card template (openclaw AICardStatus).
	aiCardFlowInputing = "2"
	aiCardFlowFinished = "3"
	aiCardFlowFailed   = "5"
)

// dingtalkCardAPIBase is a var so tests can point it at a httptest server.
var dingtalkCardAPIBase = "https://api.dingtalk.com"

// aiCardClient creates and finalizes AI cards using the robot's own
// credentials (clientId/clientSecret → app access token, cached ~2h).
type aiCardClient struct {
	clientID     string
	clientSecret string
	templateID   string
	httpClient   *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// newAICardClient builds the reply-UX client. With an empty templateID the
// client does emotions only (Thinking/Done chips) and no cards — mirroring
// hermes, where cards are an opt-in enabled by configuring a template ID and
// replies stay plain text otherwise. This avoids the silent-failure trap:
// card APIs all succeed even when the client cannot render the template.
func newAICardClient(clientID, clientSecret, templateID string) *aiCardClient {
	templateID = strings.TrimSpace(templateID)
	return &aiCardClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		templateID:   templateID,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
	}
}

// hasTemplate reports whether card replies are enabled (a template is
// configured); without one the client is used for emotions only.
func (c *aiCardClient) hasTemplate() bool { return c.templateID != "" }

// aiCardInstance is one delivered card, identified by its outTrackId.
type aiCardInstance struct {
	outTrackID string
	// inputing marks that the INPUTING flow state was already set (once per
	// card, before the first streaming frame).
	inputing bool
	// lastFrame timestamps the last non-final streaming frame for the
	// per-card throttle (the streaming endpoint 403s on <~500ms updates;
	// hermes uses 800ms).
	lastFrame time.Time
}

// accessToken returns a cached app access token, refreshing 5 minutes early.
func (c *aiCardClient) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp.Add(-5*time.Minute)) {
		return c.token, nil
	}
	body, _ := json.Marshal(map[string]string{
		"appKey":    c.clientID,
		"appSecret": c.clientSecret,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		dingtalkCardAPIBase+"/v1.0/oauth2/accessToken", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("accessToken HTTP %d: %s", resp.StatusCode, truncateRunes(string(raw), 200))
	}
	var parsed struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int64  `json:"expireIn"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.AccessToken == "" {
		return "", fmt.Errorf("accessToken parse failed: %s", truncateRunes(string(raw), 200))
	}
	c.token = parsed.AccessToken
	expire := parsed.ExpireIn
	if expire <= 0 {
		expire = 7200
	}
	c.tokenExp = time.Now().Add(time.Duration(expire) * time.Second)
	return c.token, nil
}

// call performs one authenticated card-API request and fails on non-2xx.
func (c *aiCardClient) call(ctx context.Context, method, path string, payload map[string]any) error {
	_, err := c.callRaw(ctx, method, path, payload)
	return err
}

func (c *aiCardClient) callRaw(ctx context.Context, method, path string, payload map[string]any) (string, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return "", err
	}
	var bodyReader io.Reader
	if payload != nil {
		body, merr := json.Marshal(payload)
		if merr != nil {
			return "", merr
		}
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, dingtalkCardAPIBase+path, bodyReader)
	if err != nil {
		return "", err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("x-acs-dingtalk-access-token", token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%s %s HTTP %d: %s", method, path, resp.StatusCode, truncateRunes(string(raw), 300))
	}
	// Surface every response body in the connector log: the card APIs report
	// business-level failures inside a 200, and a silently dropped frame is
	// otherwise indistinguishable from the client-side "内容加载失败".
	fmt.Fprintf(os.Stderr, "[connect][card][api] %s %s -> %d %s\n", method, path, resp.StatusCode, truncateRunes(string(raw), 200))
	return string(raw), nil
}

// sendOTOText sends a plain-text 1:1 (robot → person) message to each userId,
// using the robot batch one-to-one send API. It is how the connector reaches a
// specific person (e.g. the digital-twin owner) proactively, outside the
// ephemeral inbound sessionWebhook — the webhook can only reply into the
// conversation a message arrived on, and may be stale by the time an owner
// decides. Auth reuses the app access token (the bot's own clientId/secret), so
// the message is sent as the bot in its own org.
func (c *aiCardClient) sendOTOText(ctx context.Context, userIDs []string, text string) error {
	if len(userIDs) == 0 {
		return nil
	}
	param, _ := json.Marshal(map[string]string{"content": text})
	return c.call(ctx, http.MethodPost, "/v1.0/robot/oToMessages/batchSend", map[string]any{
		"robotCode": c.clientID,
		"userIds":   userIDs,
		"msgKey":    "sampleText",
		"msgParam":  string(param),
	})
}

// createAndDeliver creates an AI-card instance and delivers it into the
// conversation the message came from. With empty content the template
// renders the "Thinking" state while the agent runs. Group messages deliver
// to IM_GROUP (with robotCode), 1:1 to IM_ROBOT (spaceType only) — exactly
// the hermes deliver shapes.
func (c *aiCardClient) createAndDeliver(ctx context.Context, data *chatbot.BotCallbackDataModel) (*aiCardInstance, error) {
	outTrackID := "dws_" + uuid.NewString()
	create := map[string]any{
		"cardTemplateId": c.templateID,
		"outTrackId":     outTrackID,
		"cardData": map[string]any{
			"cardParamMap": map[string]any{"config": `{"autoLayout":true}`},
		},
		"callbackType":          "STREAM",
		"imGroupOpenSpaceModel": map[string]any{"supportForward": true},
		"imRobotOpenSpaceModel": map[string]any{"supportForward": true},
	}
	if err := c.call(ctx, http.MethodPost, "/v1.0/card/instances", create); err != nil {
		return nil, err
	}

	deliver := map[string]any{
		"outTrackId": outTrackID,
		"userIdType": 1,
	}
	if data.ConversationType == "2" { // group chat
		deliver["openSpaceId"] = "dtv1.card//IM_GROUP." + data.ConversationId
		deliver["imGroupOpenDeliverModel"] = map[string]any{"robotCode": c.clientID}
	} else { // 1:1 chat with the robot
		if data.SenderStaffId == "" {
			return nil, fmt.Errorf("missing senderStaffId for 1:1 card delivery")
		}
		deliver["openSpaceId"] = "dtv1.card//IM_ROBOT." + data.SenderStaffId
		deliver["imRobotOpenDeliverModel"] = map[string]any{
			"spaceType": "IM_ROBOT",
			"robotCode": c.clientID,
			"extension": map[string]any{"dynamicSummary": "true"},
		}
	}
	if err := c.callChecked(ctx, http.MethodPost, "/v1.0/card/instances/deliver", deliver); err != nil {
		return nil, err
	}
	return &aiCardInstance{outTrackID: outTrackID}, nil
}

// callChecked is call() plus a business-level result check: the deliver API
// reports per-target failures INSIDE a HTTP 200 (e.g. {"result":[{"success":
// false,"errorMsg":"spaceId is illegal"}],"success":true}) — observed live.
func (c *aiCardClient) callChecked(ctx context.Context, method, path string, payload map[string]any) error {
	raw, err := c.callRaw(ctx, method, path, payload)
	if err != nil {
		return err
	}
	if strings.Contains(raw, `"success":false`) {
		return fmt.Errorf("%s %s business failure: %s", method, path, truncateRunes(raw, 300))
	}
	return nil
}

// streamingUpdate sends one full content frame to the card.
func (c *aiCardClient) streamingUpdate(ctx context.Context, card *aiCardInstance, content string, finalize, isError bool) error {
	if r := []rune(content); len(r) > aiCardMaxContent {
		content = string(r[:aiCardMaxContent])
	}
	return c.call(ctx, http.MethodPut, "/v1.0/card/streaming", map[string]any{
		"outTrackId": card.outTrackID,
		"guid":       uuid.NewString(),
		"key":        "msgContent",
		"content":    content,
		"isFull":     true,
		"isFinalize": finalize,
		"isError":    isError,
	})
}

// aiCardFrameGap spaces the deliver → content → finalize frames. Delivering
// and finalizing back-to-back races the client's card fetch and intermittently
// renders "内容加载失败" (the very failure that killed the #407 card attempt);
// a short gap lets the client subscribe before the closing frame lands.
var aiCardFrameGap = 500 * time.Millisecond

var aiCardSleepCtx = sleepCtx

// cardContentParams is the cardParamMap contract of the openclaw template.
func cardContentParams(flowStatus, content string) map[string]any {
	return map[string]any{
		"flowStatus":        flowStatus,
		"msgContent":        content,
		"staticMsgContent":  "",
		"sys_full_json_obj": `{"order":["msgContent"]}`,
		"config":            `{"autoLayout":true}`,
	}
}

// setFlowStatus updates the card instance's flow state (openclaw contract:
// INPUTING before streaming, FINISHED after the finalize frame).
func (c *aiCardClient) setFlowStatus(ctx context.Context, card *aiCardInstance, flowStatus, content string, byKey bool) error {
	payload := map[string]any{
		"outTrackId": card.outTrackID,
		"cardData":   map[string]any{"cardParamMap": cardContentParams(flowStatus, content)},
	}
	if byKey {
		payload["cardUpdateOptions"] = map[string]any{"updateCardDataByKey": true}
	}
	return c.call(ctx, http.MethodPut, "/v1.0/card/instances", payload)
}

// aiCardFrameThrottle is the minimum spacing between non-final streaming
// frames per card (hermes uses 800ms; the endpoint 403s on rapid updates).
var aiCardFrameThrottle = 800 * time.Millisecond

// streamFrame pushes one full-content frame into the card, switching the card
// to INPUTING before the first frame. Non-final frames are throttled per
// card — a skipped frame is fine because every frame carries the full text so
// far (isFull). The final frame always goes out.
func (c *aiCardClient) streamFrame(ctx context.Context, card *aiCardInstance, content string, final bool) error {
	normalized := normalizeForCard(content)
	if !card.inputing {
		if err := c.setFlowStatus(ctx, card, aiCardFlowInputing, normalized, false); err != nil {
			return err
		}
		card.inputing = true
		card.lastFrame = time.Now()
	}
	if !final {
		if time.Since(card.lastFrame) < aiCardFrameThrottle {
			return nil // skip; a later frame re-sends the full text
		}
	}
	if err := c.streamingUpdate(ctx, card, normalized, final, false); err != nil {
		return err
	}
	card.lastFrame = time.Now()
	return nil
}

// finish closes the card: the finalized streaming frame plus the FINISHED
// flow state. A failure leaves the card spinning, so callers must treat an
// error as "fall back to a plain webhook reply" (after markFailed).
func (c *aiCardClient) finish(ctx context.Context, card *aiCardInstance, content string) error {
	normalized := normalizeForCard(content)
	if err := c.streamFrame(ctx, card, content, true); err != nil {
		return err
	}
	if err := aiCardSleepCtx(ctx, aiCardFrameGap); err != nil {
		return err
	}
	return c.setFlowStatus(ctx, card, aiCardFlowFinished, normalized, true)
}

// finalize is the one-shot path for channels without incremental output:
// INPUTING → final frame → FINISHED in one call.
func (c *aiCardClient) finalize(ctx context.Context, card *aiCardInstance, content string) error {
	if err := aiCardSleepCtx(ctx, aiCardFrameGap); err != nil {
		return err
	}
	return c.finish(ctx, card, content)
}

// repair re-pushes the finalize frame once, a few seconds after the reply.
// hermes streams many frames so a client that misses one recovers on the
// next; our burst of two frames has no such retry, and a missed fetch shows
// "内容加载失败" until a new frame arrives. Best-effort by design.
func (c *aiCardClient) repair(ctx context.Context, card *aiCardInstance, content string) {
	if err := aiCardSleepCtx(ctx, aiCardRepairDelay); err != nil {
		return
	}
	_ = c.setFlowStatus(ctx, card, aiCardFlowFinished, normalizeForCard(content), true)
}

// aiCardRepairDelay is how long after finalize the repair frame goes out.
var aiCardRepairDelay = 3 * time.Second

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// markFailed best-effort closes a stuck card in the error state so the
// Thinking chip does not spin forever when finalize failed and the reply
// went out as a plain message instead. Errors are ignored by design.
func (c *aiCardClient) markFailed(ctx context.Context, card *aiCardInstance) {
	_ = c.setFlowStatus(ctx, card, aiCardFlowFailed, "", true)
}

// ---- Message reactions (the "🤔Thinking" chip) ----
//
// The status chip attached to the user's message is NOT a card: it is a text
// emotion (message reaction) — hermes fires "🤔Thinking" on receive and swaps
// it for "🥳Done" when the reply lands (gateway/platforms/dingtalk.py,
// _send_emotion / _fire_done_reaction). During thinking there is no card at
// all; the card only appears with the final content.

const (
	emotionThinking = "🤔Thinking"
	emotionDone     = "🥳Done"
)

// sendEmotion adds (or recalls) a text reaction on the user's message.
// Failures are logged by callers at most — the reaction is decoration, never
// worth failing the reply over. Payload contract per the robot_1_0 SDK:
// POST /v1.0/robot/emotion/reply | /v1.0/robot/emotion/recall.
func (c *aiCardClient) sendEmotion(ctx context.Context, conversationID, msgID, name string, recall bool) error {
	if conversationID == "" || msgID == "" {
		return fmt.Errorf("emotion needs openConversationId and openMsgId")
	}
	path := "/v1.0/robot/emotion/reply"
	if recall {
		path = "/v1.0/robot/emotion/recall"
	}
	return c.call(ctx, http.MethodPost, path, map[string]any{
		"robotCode":          c.clientID,
		"openConversationId": conversationID,
		"openMsgId":          msgID,
		"emotionType":        2,
		"emotionName":        name,
		"textEmotion": map[string]any{
			"emotionId":    "2659900",
			"emotionName":  name,
			"text":         name,
			"backgroundId": "im_bg_1",
		},
	})
}

// markThinking fires the "🤔Thinking" chip on the user's message.
func (c *aiCardClient) markThinking(ctx context.Context, conversationID, msgID string) error {
	return c.sendEmotion(ctx, conversationID, msgID, emotionThinking, false)
}

// swapThinkingToDone replaces the chip with "🥳Done" after the reply landed.
// Best-effort by design (mirrors hermes' fire-and-forget swap).
func (c *aiCardClient) swapThinkingToDone(ctx context.Context, conversationID, msgID string) {
	_ = c.sendEmotion(ctx, conversationID, msgID, emotionThinking, true)
	_ = c.sendEmotion(ctx, conversationID, msgID, emotionDone, false)
}

// ---- Markdown normalization for the AI-card renderer ----
//
// Ported from openclaw's normalizeForCard (messaging/card.ts): the 02fcf2f4
// template renders <br> as visual line breaks in plain text, needs real \n
// inside code fences and before Markdown block syntax, and a blank line
// before tables.

var (
	cardTableDividerRe = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|?\s*:?-+:?\s*)+\|?\s*$`)
	cardTableRowRe     = regexp.MustCompile(`^\s*\|?.*\|.*\|?\s*$`)
	cardBlockStartRe   = regexp.MustCompile(`^(\s{0,3}(?:[-*+]|\d+[.)])[ ])|(\s{0,3}\|)|(\s{0,3}#{1,6}\s)|(\s{0,3}(?:[-*_])\s*(?:[-*_])\s*(?:[-*_]))`)
	cardFenceRe        = regexp.MustCompile("^\\s{0,3}```")
	cardQuoteRe        = regexp.MustCompile(`^\s{0,3}>\s?`)
)

func normalizeCardLineEndings(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

// ensureCardTableBlankLines inserts a blank line before a table header that
// directly follows non-table text, which the card renderer requires.
func ensureCardTableBlankLines(text string) string {
	lines := strings.Split(normalizeCardLineEndings(text), "\n")
	out := make([]string, 0, len(lines))
	isDivider := func(line string) bool {
		return strings.Contains(line, "|") && cardTableDividerRe.MatchString(line)
	}
	for i, line := range lines {
		next := ""
		if i+1 < len(lines) {
			next = lines[i+1]
		}
		if cardTableRowRe.MatchString(line) && isDivider(next) && i > 0 &&
			strings.TrimSpace(lines[i-1]) != "" && !cardTableRowRe.MatchString(lines[i-1]) {
			out = append(out, "")
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// fixCardNewlines converts single \n to <br> for plain text while keeping
// real newlines inside code fences and before Markdown block syntax, and
// merges consecutive quote lines with <br> (lazy continuation).
func fixCardNewlines(text string) string {
	normalized := normalizeCardLineEndings(text)

	var merged []string
	var pendingQuote []string
	inCode := false
	flushQuote := func() {
		if len(pendingQuote) > 0 {
			merged = append(merged, strings.Join(pendingQuote, "<br>"))
			pendingQuote = nil
		}
	}
	for _, line := range strings.Split(normalized, "\n") {
		isFence := cardFenceRe.MatchString(line)
		if inCode {
			flushQuote()
			merged = append(merged, line)
			if isFence {
				inCode = false
			}
			continue
		}
		if isFence {
			flushQuote()
			merged = append(merged, line)
			inCode = true
			continue
		}
		if cardQuoteRe.MatchString(line) {
			if len(pendingQuote) == 0 {
				pendingQuote = append(pendingQuote, line)
			} else {
				pendingQuote = append(pendingQuote, cardQuoteRe.ReplaceAllString(line, ""))
			}
		} else {
			flushQuote()
			merged = append(merged, line)
		}
	}
	flushQuote()

	inCode = false
	var b strings.Builder
	for i, line := range merged {
		nextInCode := inCode
		if cardFenceRe.MatchString(line) {
			nextInCode = !inCode
		}
		if i < len(merged)-1 {
			next := merged[i+1]
			keepNewline := nextInCode || line == "" || next == "" ||
				cardFenceRe.MatchString(next) || cardBlockStartRe.MatchString(next)
			b.WriteString(line)
			if keepNewline {
				b.WriteString("\n")
			} else {
				b.WriteString("<br>")
			}
		} else {
			b.WriteString(line)
		}
		inCode = nextInCode
	}
	return b.String()
}

// normalizeForCard prepares agent output for the AI-card renderer.
func normalizeForCard(content string) string {
	return fixCardNewlines(ensureCardTableBlankLines(content))
}
