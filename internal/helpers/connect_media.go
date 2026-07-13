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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// mediaMaxDownloadBytes caps a single inbound image download (screenshots are
// well under this; the cap is a hostile-input guard).
const mediaMaxDownloadBytes = 20 << 20

// pictureDownloadCode digs the downloadCode out of a picture callback's
// loosely-typed content payload (the stream SDK models Content as
// interface{}).
func pictureDownloadCode(content interface{}) string {
	m, ok := content.(map[string]interface{})
	if !ok {
		return ""
	}
	for _, key := range []string{"downloadCode", "pictureDownloadCode"} {
		if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// richTextPictureDownloadCodes returns the media download codes embedded in a
// msgtype="richText" callback, preserving the node order. DingTalk represents
// an inline picture as a richText node instead of a top-level picture message:
//
//	{"type":"picture","downloadCode":"...","pictureDownloadCode":"..."}
//
// The two code fields identify the same picture; pictureDownloadCode handles
// their precedence and returns only one code per node.
func richTextPictureDownloadCodes(content interface{}) []string {
	m, ok := content.(map[string]interface{})
	if !ok {
		return nil
	}
	items, ok := m["richText"].([]interface{})
	if !ok {
		return nil
	}
	codes := make([]string, 0)
	for _, item := range items {
		node, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		code := pictureDownloadCode(node)
		if code == "" {
			continue
		}
		// A richText node with a picture download code is actionable even if an
		// older callback omitted type. When type is present, reject unrelated
		// node kinds so future payload fields are not mistaken for pictures.
		if typ, exists := node["type"].(string); exists && strings.TrimSpace(typ) != "" && !strings.EqualFold(strings.TrimSpace(typ), "picture") {
			continue
		}
		codes = append(codes, code)
	}
	return codes
}

// fileInboundInfo carries everything a msgtype="file" callback might expose.
// Client-sent files (a user attaching a file in the DingTalk client) surface
// DownloadCode + FileName; API-sent files (`dws chat message send --msg-type
// file --dentry-id --space-id`) surface DentryID + SpaceID + FileName +
// FileType + FileSize + FilePath and NO DownloadCode. Both shapes have to be
// recognisable or the connector silently drops legitimate file messages.
type fileInboundInfo struct {
	DownloadCode string
	FileName     string
	FileType     string
	FilePath     string
	DentryID     int64
	SpaceID      int64
	FileSize     int64
}

func (f fileInboundInfo) hasActionable() bool {
	return strings.TrimSpace(f.DownloadCode) != "" || (f.DentryID != 0 && f.SpaceID != 0)
}

// parseFileInbound reads every relevant field out of a file callback's
// content payload (msgtype="file"). The content is a loosely-typed
// map[string]interface{}; numeric fields (dentryId/spaceId/fileSize) can be
// JSON strings or numbers depending on which endpoint sent the message, so
// both branches are handled.
func parseFileInbound(content interface{}) fileInboundInfo {
	info := fileInboundInfo{}
	m, ok := content.(map[string]interface{})
	if !ok {
		return info
	}
	for _, key := range []string{"downloadCode", "fileDownloadCode"} {
		if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
			info.DownloadCode = strings.TrimSpace(v)
			break
		}
	}
	if v, ok := m["fileName"].(string); ok {
		info.FileName = strings.TrimSpace(v)
	}
	if v, ok := m["fileType"].(string); ok {
		info.FileType = strings.TrimSpace(v)
	}
	if v, ok := m["filePath"].(string); ok {
		info.FilePath = strings.TrimSpace(v)
	}
	info.DentryID = readInt64Field(m, "dentryId", "dentryID")
	info.SpaceID = readInt64Field(m, "spaceId", "spaceID")
	info.FileSize = readInt64Field(m, "fileSize", "size")
	if info.FileName == "" {
		info.FileName = "未知文件"
	}
	return info
}

// readInt64Field pulls an int64 out of the loose content map under any of the
// provided keys, tolerating JSON string / float64 / int64 / json.Number.
func readInt64Field(m map[string]interface{}, keys ...string) int64 {
	for _, key := range keys {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case string:
			if s := strings.TrimSpace(t); s != "" {
				if n, err := strconv.ParseInt(s, 10, 64); err == nil {
					return n
				}
			}
		case float64:
			return int64(t)
		case int64:
			return t
		case int:
			return int64(t)
		case json.Number:
			if n, err := t.Int64(); err == nil {
				return n
			}
		}
	}
	return 0
}

// fileDownloadInfo preserves the two-value shape used by legacy callers that
// only care about the downloadCode / fileName pair.
func fileDownloadInfo(content interface{}) (downloadCode, fileName string) {
	info := parseFileInbound(content)
	return info.DownloadCode, info.FileName
}

// summarizeContent renders the callback content into a short one-line string
// for stderr diagnostics (used when a file/other callback is being dropped
// so the operator can tell why after the fact).
func summarizeContent(content interface{}) string {
	if content == nil {
		return "<nil>"
	}
	b, err := json.Marshal(content)
	if err != nil {
		return fmt.Sprintf("<unmarshalable:%v>", err)
	}
	s := string(b)
	if len(s) > 400 {
		s = s[:400] + "…"
	}
	return s
}

// extractCallbackText pulls the visible text out of a structured-text callback
// payload (msgtype=richText / markdown / etc.) for the case where the SDK's
// data.Text.Content is empty. This matters because `dws chat message send
// --group ... --text ...` sends msgType="markdown" by default, and DingTalk's
// stream callback for markdown messages routinely leaves data.Text.Content
// blank while stashing the real body in data.Content (loosely-typed).
// Returns "" if no text-shaped field is found.
func extractCallbackText(content interface{}) string {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]interface{}:
		// Common shapes: {"text":"..."}, {"title":"...","text":"..."},
		// {"content":"..."}, richText {"richText":[{"text":"..."}]}.
		for _, key := range []string{"text", "content", "markdown", "title"} {
			if s, ok := v[key].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
		if arr, ok := v["richText"].([]interface{}); ok {
			var b strings.Builder
			for _, item := range arr {
				if m, ok := item.(map[string]interface{}); ok {
					if s, ok := m["text"].(string); ok {
						b.WriteString(s)
					}
				}
			}
			if s := strings.TrimSpace(b.String()); s != "" {
				return s
			}
		}
		// interactiveCard: another bot @-mentioning this bot arrives as
		// msgtype="interactiveCard" with the body nested in
		// content.cardContent[].children[].value (elementType TEXT). The SDK
		// leaves Text.Content blank, so without this the connector drops a
		// legitimate bot-to-bot @ message.
		if s := extractCardContentText(v["cardContent"]); s != "" {
			return s
		}
	}
	return ""
}

// cardContentLeaves flattens an interactiveCard cardContent tree into its
// ordered TEXT leaf values. Shape (verified live): cardContent is an array of
// blocks, each with a "children" array of {elementType:"TEXT", value:"..."}
// leaves; nested blocks recurse via their own "children".
func cardContentLeaves(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var leaves []string
	var walk func(items []interface{})
	walk = func(items []interface{}) {
		for _, item := range items {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if s, ok := m["value"].(string); ok && s != "" {
				leaves = append(leaves, s)
			}
			if kids, ok := m["children"].([]interface{}); ok {
				walk(kids)
			}
		}
	}
	walk(arr)
	return leaves
}

// extractCardContentText joins all interactiveCard leaves — the raw body,
// mention included. Used by the generic extractCallbackText fallback.
func extractCardContentText(v interface{}) string {
	return strings.TrimSpace(strings.Join(cardContentLeaves(v), ""))
}

// extractInteractiveCardText returns the interactiveCard body with the leading
// @-mention removed. A bot @-ing another bot renders the mention as its own
// leading "@name" TEXT leaf (the display name may itself contain spaces, so we
// drop by leaf boundary, not by whitespace), followed by the real message in
// the next leaf. Returns "" when there is nothing beyond the mention.
func extractInteractiveCardText(content interface{}) string {
	m, ok := content.(map[string]interface{})
	if !ok {
		return ""
	}
	leaves := cardContentLeaves(m["cardContent"])
	for len(leaves) > 0 && strings.HasPrefix(strings.TrimSpace(leaves[0]), "@") {
		leaves = leaves[1:]
	}
	return strings.TrimSpace(strings.Join(leaves, ""))
}

// downloadMessageFile resolves a chatbot media callback (picture etc.) to a
// local temp file via the robot messageFiles/download API. Error-screenshot
// questions are the top Q&A inbound; without this the connector silently
// drops every picture message.
func (c *aiCardClient) downloadMessageFile(ctx context.Context, robotCode, downloadCode string) (string, error) {
	raw, err := c.callRaw(ctx, http.MethodPost, "/v1.0/robot/messageFiles/download", map[string]any{
		"robotCode":    robotCode,
		"downloadCode": downloadCode,
	})
	if err != nil {
		return "", err
	}
	var parsed struct {
		DownloadUrl string `json:"downloadUrl"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil || strings.TrimSpace(parsed.DownloadUrl) == "" {
		return "", fmt.Errorf("messageFiles/download 未返回 downloadUrl: %s", truncateRunes(raw, 200))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.DownloadUrl, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("媒体下载 HTTP %d", resp.StatusCode)
	}
	dir := filepath.Join(os.TempDir(), "dws-connect-media")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, uuid.NewString()+mediaExt(parsed.DownloadUrl, resp.Header.Get("Content-Type")))
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, io.LimitReader(resp.Body, mediaMaxDownloadBytes)); err != nil {
		_ = os.Remove(dest)
		return "", err
	}
	return dest, nil
}

// getUserUnionID resolves a staffId (userId) to unionId via the contact API.
// The connector needs unionId to call the storage/drive download APIs.
func (c *aiCardClient) getUserUnionID(ctx context.Context, userID string) (string, error) {
	raw, err := c.callRaw(ctx, http.MethodGet, "/v1.0/contact/users/"+userID, nil)
	if err != nil {
		return "", fmt.Errorf("contact/users/%s: %w", userID, err)
	}
	var parsed struct {
		UnionID string `json:"unionId"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil || strings.TrimSpace(parsed.UnionID) == "" {
		return "", fmt.Errorf("contact/users/%s 未返回 unionId: %s", userID, truncateRunes(raw, 200))
	}
	return parsed.UnionID, nil
}

// downloadDentryFile downloads a file from DingTalk storage by numeric
// dentryId + spaceId (the shape API-sent file callbacks provide). It resolves
// download info via the v2.0 storage API and saves the file to a local temp
// path, returning the path.
func (c *aiCardClient) downloadDentryFile(ctx context.Context, spaceID, dentryID int64, unionID, fileName string) (string, error) {
	path := fmt.Sprintf("/v2.0/storage/spaces/%d/dentries/%d/getDownloadInfo", spaceID, dentryID)
	raw, err := c.callRaw(ctx, http.MethodPost, path, map[string]any{
		"unionId": unionID,
	})
	if err != nil {
		return "", fmt.Errorf("getDownloadInfo spaceId=%d dentryId=%d: %w", spaceID, dentryID, err)
	}
	var parsed struct {
		ResourceURL string            `json:"resourceUrl"`
		HeadersMap  map[string]string `json:"headers"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil || strings.TrimSpace(parsed.ResourceURL) == "" {
		return "", fmt.Errorf("getDownloadInfo 未返回 resourceUrl: %s", truncateRunes(raw, 200))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.ResourceURL, nil)
	if err != nil {
		return "", err
	}
	for k, v := range parsed.HeadersMap {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("钉盘文件下载 HTTP %d", resp.StatusCode)
	}

	dir := filepath.Join(os.TempDir(), "dws-connect-media")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ext := filepath.Ext(fileName)
	if ext == "" {
		ext = mediaExt(parsed.ResourceURL, resp.Header.Get("Content-Type"))
	}
	dest := filepath.Join(dir, uuid.NewString()+ext)
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, io.LimitReader(resp.Body, mediaMaxDownloadBytes)); err != nil {
		_ = os.Remove(dest)
		return "", err
	}
	return dest, nil
}

// mediaExt picks a file extension from the response content type, falling
// back to the URL path, then ".png" (DingTalk screenshots default to png).
func mediaExt(rawURL, contentType string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "jpeg"), strings.Contains(ct, "jpg"):
		return ".jpg"
	case strings.Contains(ct, "gif"):
		return ".gif"
	case strings.Contains(ct, "webp"):
		return ".webp"
	}
	if u, err := url.Parse(rawURL); err == nil {
		if ext := strings.ToLower(filepath.Ext(u.Path)); len(ext) >= 2 && len(ext) <= 5 {
			return ext
		}
	}
	return ".png"
}
