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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
)

// AI Card streaming reply for DingTalk "智能体" (AI-assistant) bots.
//
// `dws connect bot create` provisions AI-assistant robots. DingTalk renders
// their messages as a streaming AI card (a "数据加载中" placeholder) that can
// ONLY be filled through the card streaming API — a plain sessionWebhook reply
// leaves the card stuck forever. This mirrors the official
// dingtalk-openclaw-connector (src/services/messaging/card.ts): create a card
// instance from the shared streaming template, deliver it to the conversation,
// then stream the content into the "msgContent" key and finalize.
const (
	// aiCardTemplateID is DingTalk's shared streaming-card template, usable by
	// any app without registering one (same constant the official connector
	// hardcodes). Its streaming field key is "msgContent".
	aiCardTemplateID = "02fcf2f4-5e02-4a85-b672-46d1f715543e.schema"
	dingtalkAPIBase  = "https://api.dingtalk.com"
)

// aiCardReplier fills a DingTalk AI card by streaming the agent reply into it.
// One instance per connector; the app access token is cached and refreshed
// lazily.
type aiCardReplier struct {
	clientID     string
	clientSecret string
	httpc        *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

func newAICardReplier(clientID, clientSecret string) *aiCardReplier {
	return &aiCardReplier{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpc:        &http.Client{Timeout: 15 * time.Second},
	}
}

// accessToken returns a cached app access token, fetching a fresh one when the
// cache is empty or within 5 minutes of expiry.
func (r *aiCardReplier) accessToken(ctx context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.token != "" && time.Until(r.tokenExp) > 5*time.Minute {
		return r.token, nil
	}
	body, _ := json.Marshal(map[string]string{"appKey": r.clientID, "appSecret": r.clientSecret})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dingtalkAPIBase+"/v1.0/oauth2/accessToken", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int    `json:"expireIn"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("empty accessToken")
	}
	r.token = out.AccessToken
	exp := out.ExpireIn
	if exp <= 0 {
		exp = 7200
	}
	r.tokenExp = time.Now().Add(time.Duration(exp) * time.Second)
	return r.token, nil
}

// reply creates, delivers and streams an AI card carrying content. robotCode is
// the bot's robotCode (== clientID for dws-provisioned bots). It returns an
// error if any step fails so the caller can fall back to a plain webhook reply
// (e.g. for a non-AI-assistant robot or when card scopes are missing).
func (r *aiCardReplier) reply(ctx context.Context, data *chatbot.BotCallbackDataModel, robotCode, content string) error {
	token, err := r.accessToken(ctx)
	if err != nil {
		return fmt.Errorf("access token: %w", err)
	}
	outTrackID := "card_" + randHex(12)

	// 1. Create a streaming card instance from the shared template.
	createBody := map[string]any{
		"cardTemplateId":        aiCardTemplateID,
		"outTrackId":            outTrackID,
		"cardData":              map[string]any{"cardParamMap": map[string]any{"config": `{"autoLayout": true}`}},
		"callbackType":          "STREAM",
		"imGroupOpenSpaceModel": map[string]any{"supportForward": true},
		"imRobotOpenSpaceModel": map[string]any{"supportForward": true},
	}
	if err := r.do(ctx, http.MethodPost, "/v1.0/card/instances", token, createBody); err != nil {
		return fmt.Errorf("create card: %w", err)
	}

	// 2. Deliver the card to the conversation (group vs single-chat).
	deliverBody := map[string]any{"outTrackId": outTrackID, "userIdType": 1}
	if data.ConversationType == "2" {
		deliverBody["openSpaceId"] = "dtv1.card//IM_GROUP." + data.ConversationId
		deliverBody["imGroupOpenDeliverModel"] = map[string]any{"robotCode": robotCode}
	} else {
		deliverBody["openSpaceId"] = "dtv1.card//IM_ROBOT." + data.SenderStaffId
		deliverBody["imRobotOpenDeliverModel"] = map[string]any{
			"spaceType": "IM_ROBOT",
			"robotCode": robotCode,
			"extension": map[string]any{"dynamicSummary": "true"},
		}
	}
	if err := r.do(ctx, http.MethodPost, "/v1.0/card/instances/deliver", token, deliverBody); err != nil {
		return fmt.Errorf("deliver card: %w", err)
	}

	// 3. Stream the content into the card's msgContent field and finalize.
	streamBody := map[string]any{
		"outTrackId": outTrackID,
		"guid":       randHex(8),
		"key":        "msgContent",
		"content":    content,
		"isFull":     true,
		"isFinalize": true,
		"isError":    false,
	}
	if err := r.do(ctx, http.MethodPut, "/v1.0/card/streaming", token, streamBody); err != nil {
		return fmt.Errorf("stream card: %w", err)
	}

	// 4. Switch the card to its FINISHED state with the full parameter map. The
	// shared template only renders once flowStatus + msgContent + the field
	// order in sys_full_json_obj are set via a card-instances update — without
	// it the card stays on "内容加载失败". (openclaw finishAICard, card.ts:601.)
	finishBody := map[string]any{
		"outTrackId": outTrackID,
		"cardData": map[string]any{
			"cardParamMap": map[string]any{
				"flowStatus":       "3", // AICardStatus.FINISHED
				"msgContent":       content,
				"staticMsgContent": "",
				"sys_full_json_obj": `{"order":["msgContent"]}`,
				"config":           `{"autoLayout": true}`,
			},
		},
		"cardUpdateOptions": map[string]any{"updateCardDataByKey": true},
	}
	if err := r.do(ctx, http.MethodPut, "/v1.0/card/instances", token, finishBody); err != nil {
		return fmt.Errorf("finalize card: %w", err)
	}
	return nil
}

// do issues a JSON request to the DingTalk card API and treats any non-2xx
// response as an error, surfacing the body so card-scope / template problems are
// visible in the connector log. A 5xx is retried once after a short pause: the
// card endpoints occasionally return a transient 503 and every call is
// idempotent by outTrackId, so a retry avoids a needless webhook fallback (which
// would leave an AI-assistant bot on the "数据加载中" placeholder).
func (r *aiCardReplier) do(ctx context.Context, method, path, token string, body any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, dingtalkAPIBase+path, bytes.NewReader(buf))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-acs-dingtalk-access-token", token)
		resp, err := r.httpc.Do(req)
		if err != nil {
			lastErr = err
			continue // network error: retry once
		}
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if resp.StatusCode < 300 {
			return nil
		}
		lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		if resp.StatusCode < 500 {
			return lastErr // 4xx is not transient: do not retry
		}
	}
	return lastErr
}

// randHex returns n random bytes hex-encoded (2n chars), used for card and guid
// identifiers. On a (practically impossible) RNG failure it falls back to a
// timestamp so the identifier stays non-empty.
func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
