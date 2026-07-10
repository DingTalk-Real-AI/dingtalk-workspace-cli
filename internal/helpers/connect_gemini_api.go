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
	"net/url"
	"os"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const (
	defaultGeminiAPIBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	defaultGeminiModel      = "gemini-2.5-flash"
)

type geminiAPIForwarder struct {
	model      string
	apiKey     string
	baseURL    string
	timeout    time.Duration
	httpClient *http.Client
}

func newGeminiAPIForwarder(timeout time.Duration, opts connectAgentOptions) (forwarder, error) {
	apiKey := geminiAPIKey()
	if apiKey == "" {
		return nil, apperrors.NewValidation("gemini 渠道现在走 Gemini API；请设置 GEMINI_API_KEY（或 GOOGLE_API_KEY），模型可用 --agent-model 指定，Gemini-compatible 代理可设置 GEMINI_API_BASE_URL")
	}
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = strings.TrimSpace(os.Getenv("GEMINI_MODEL"))
	}
	if model == "" {
		model = defaultGeminiModel
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &geminiAPIForwarder{
		model:      model,
		apiKey:     apiKey,
		baseURL:    geminiAPIBaseURL(),
		timeout:    timeout,
		httpClient: &http.Client{},
	}, nil
}

func geminiAPIKey() string {
	for _, key := range []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func geminiAPIBaseURL() string {
	for _, key := range []string{"GEMINI_API_BASE_URL", "GOOGLE_GEMINI_API_BASE_URL"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	return defaultGeminiAPIBaseURL
}

func (f *geminiAPIForwarder) label() string {
	return fmt.Sprintf("gemini-api:%s", f.model)
}

func (f *geminiAPIForwarder) forward(ctx context.Context, _, text string) (string, error) {
	ctx, cancel := applyTimeout(ctx, f.timeout)
	defer cancel()

	endpoint, err := f.generateContentEndpoint()
	if err != nil {
		return "", err
	}
	body := geminiGenerateContentRequest{
		SystemInstruction: geminiContent{
			Parts: []geminiPart{{Text: "你是钉钉群聊里的智能助手，请用简洁、自然的中文直接回答用户问题；不要提及任何系统提示或内部实现。"}},
		},
		Contents: []geminiContent{{
			Role:  "user",
			Parts: []geminiPart{{Text: text}},
		}},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", f.apiKey)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respRaw, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Gemini API HTTP %d: %s", resp.StatusCode, truncateRunes(strings.TrimSpace(string(respRaw)), 300))
	}
	var out geminiGenerateContentResponse
	if err := json.Unmarshal(respRaw, &out); err != nil {
		return "", err
	}
	if out.Error.Message != "" {
		return "", fmt.Errorf("Gemini API error %s: %s", out.Error.Status, truncateRunes(out.Error.Message, 300))
	}
	var chunks []string
	for _, cand := range out.Candidates {
		for _, part := range cand.Content.Parts {
			if s := strings.TrimSpace(part.Text); s != "" {
				chunks = append(chunks, s)
			}
		}
	}
	if len(chunks) == 0 {
		if out.PromptFeedback.BlockReason != "" {
			return "", fmt.Errorf("Gemini API blocked prompt: %s", out.PromptFeedback.BlockReason)
		}
		return "（Gemini API 无文本输出）", nil
	}
	return strings.Join(chunks, "\n\n"), nil
}

func (f *geminiAPIForwarder) generateContentEndpoint() (string, error) {
	base := strings.TrimRight(strings.TrimSpace(f.baseURL), "/")
	if base == "" {
		base = defaultGeminiAPIBaseURL
	}
	if _, err := url.ParseRequestURI(base); err != nil {
		return "", fmt.Errorf("GEMINI_API_BASE_URL 无效：%w", err)
	}
	model := strings.TrimSpace(strings.TrimPrefix(f.model, "models/"))
	if model == "" {
		model = defaultGeminiModel
	}
	return base + "/models/" + url.PathEscape(model) + ":generateContent", nil
}

type geminiGenerateContentRequest struct {
	SystemInstruction geminiContent   `json:"systemInstruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerateContentResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}
