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
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// chatRecordLookup records which entries DingTalk erased to unknownMsgType in
// a Stream callback. The indexes line up with forwardMessages returned by the
// user-state message API, which still exposes the original message metadata.
type chatRecordLookup struct {
	MsgID          string
	UnknownIndexes []int
}

type chatRecordToolCall func(context.Context, string, string, map[string]any) (string, error)

type chatRecordMessage struct {
	OpenMessageID      string              `json:"openMessageId"`
	OpenConversationID string              `json:"openConversationId"`
	CreateTime         string              `json:"createTime"`
	Content            string              `json:"content"`
	ForwardMessages    []chatRecordMessage `json:"forwardMessages"`
}

type chatRecordMessagesEnvelope struct {
	Result struct {
		Messages []chatRecordMessage `json:"messages"`
	} `json:"result"`
}

type chatRecordEnrichment struct {
	Prompt       string
	Files        []fileInboundInfo
	MissingCount int
}

var (
	chatRecordMediaIDPattern      = regexp.MustCompile(`\(?mediaId=([^\s)]+)\)?`)
	chatRecordFileIDPattern       = regexp.MustCompile(`(?i)\bfileId:\s*([^\s]+)`)
	chatRecordFileNamePattern     = regexp.MustCompile(`(?m)^\[文件\]\s*(.*?)(?:\s+fileId:|$)`)
	chatRecordDownloadHintPattern = regexp.MustCompile(`\s*注意：如需下载使用dws\s+(?:chat message download-media|drive download)命令下载\s*`)
)

func chatRecordEntries(content interface{}) []interface{} {
	m, ok := content.(map[string]interface{})
	if !ok {
		return nil
	}
	if entries, ok := m["chatRecord"].([]interface{}); ok {
		return entries
	}
	if raw, ok := m["chatRecord"].(string); ok {
		var entries []interface{}
		if json.Unmarshal([]byte(raw), &entries) == nil {
			return entries
		}
	}
	if entries, ok := m["contents"].([]interface{}); ok {
		return entries
	}
	return nil
}

func chatRecordUnknownIndexes(content interface{}) []int {
	entries := chatRecordEntries(content)
	indexes := make([]int, 0)
	for i, entry := range entries {
		node, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		for _, key := range []string{"msgType", "msgtype", "type"} {
			if value, ok := node[key].(string); ok && strings.EqualFold(strings.TrimSpace(value), "unknownMsgType") {
				info := parseTypedFileInbound("file", node)
				if !info.hasActionable() {
					indexes = append(indexes, i)
				}
				break
			}
		}
	}
	return indexes
}

// recoverChatRecordUnknowns resolves only the entries that were
// unknownMsgType in the Stream callback. It deliberately runs after the
// callback returns, so user-state MCP lookups cannot delay DingTalk's ACK.
func recoverChatRecordUnknowns(ctx context.Context, lookup chatRecordLookup, call chatRecordToolCall) (chatRecordEnrichment, error) {
	var enrichment chatRecordEnrichment
	if strings.TrimSpace(lookup.MsgID) == "" || len(lookup.UnknownIndexes) == 0 {
		return enrichment, nil
	}

	raw, err := call(ctx, "im", "list_messages_by_ids", map[string]any{
		"openMsgIds": []string{strings.TrimSpace(lookup.MsgID)},
	})
	if err != nil {
		return enrichment, fmt.Errorf("查询合并转发消息: %w", err)
	}
	envelope, err := parseChatRecordMessages(raw)
	if err != nil {
		return enrichment, fmt.Errorf("解析合并转发消息: %w", err)
	}
	var outer *chatRecordMessage
	for i := range envelope.Result.Messages {
		if envelope.Result.Messages[i].OpenMessageID == strings.TrimSpace(lookup.MsgID) {
			outer = &envelope.Result.Messages[i]
			break
		}
	}
	if outer == nil && len(envelope.Result.Messages) == 1 {
		outer = &envelope.Result.Messages[0]
	}
	if outer == nil {
		return enrichment, fmt.Errorf("未找到外层消息 %s", strings.TrimSpace(lookup.MsgID))
	}

	indexes := uniqueValidIndexes(lookup.UnknownIndexes, len(outer.ForwardMessages))
	if len(indexes) == 0 {
		return enrichment, fmt.Errorf("unknownMsgType 索引超出转发消息范围")
	}

	// list_messages_by_ids keeps mediaId for images/audio but can omit fileId
	// and even stamp forwarded files with the outer conversation ID. Query every
	// candidate source conversation visible in the same record and match by the
	// stable inner openMessageId to recover the actual fileId.
	resolvedByID := make(map[string]chatRecordMessage)
	needsFileLookup := false
	for _, index := range indexes {
		message := outer.ForwardMessages[index]
		resolvedByID[message.OpenMessageID] = message
		if looksLikeForwardedFile(message.Content) && chatRecordFileID(message.Content) == "" {
			needsFileLookup = true
		}
	}
	if needsFileLookup {
		enrichForwardedFileLocators(ctx, outer.ForwardMessages, resolvedByID, call)
	}

	lines := []string{"已通过钉钉消息接口补拉到合并转发中原先标记为 unknownMsgType 的内容："}
	for _, index := range indexes {
		message := outer.ForwardMessages[index]
		if resolved, ok := resolvedByID[message.OpenMessageID]; ok {
			message = resolved
		}
		lines = append(lines, fmt.Sprintf("%d. %s", index+1, humanChatRecordContent(message.Content)))
		info, ok := recoveredForwardAttachment(message)
		if !ok || !info.hasActionable() {
			if looksLikeForwardedAttachment(message.Content) {
				enrichment.MissingCount++
			}
			continue
		}
		enrichment.Files = append(enrichment.Files, info)
	}
	enrichment.Prompt = strings.Join(lines, "\n")
	return enrichment, nil
}

func parseChatRecordMessages(raw string) (chatRecordMessagesEnvelope, error) {
	var envelope chatRecordMessagesEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return envelope, err
	}
	return envelope, nil
}

func uniqueValidIndexes(indexes []int, length int) []int {
	seen := make(map[int]struct{}, len(indexes))
	out := make([]int, 0, len(indexes))
	for _, index := range indexes {
		if index < 0 || index >= length {
			continue
		}
		if _, ok := seen[index]; ok {
			continue
		}
		seen[index] = struct{}{}
		out = append(out, index)
	}
	sort.Ints(out)
	return out
}

func enrichForwardedFileLocators(ctx context.Context, all []chatRecordMessage, resolved map[string]chatRecordMessage, call chatRecordToolCall) {
	candidates := make(map[string]struct{})
	var earliest time.Time
	for _, message := range all {
		if conversationID := strings.TrimSpace(message.OpenConversationID); conversationID != "" {
			candidates[conversationID] = struct{}{}
		}
		if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(message.CreateTime), time.Local); err == nil && (earliest.IsZero() || parsed.Before(earliest)) {
			earliest = parsed
		}
	}
	if earliest.IsZero() {
		return
	}
	start := earliest.Add(-time.Minute).Format("2006-01-02 15:04:05")
	for conversationID := range candidates {
		raw, err := call(ctx, "chat", "list_conversation_message_v2", map[string]any{
			"openconversation_id": conversationID,
			"time":                start,
			"forward":             true,
			"limit":               50,
		})
		if err != nil {
			continue
		}
		envelope, err := parseChatRecordMessages(raw)
		if err != nil {
			continue
		}
		for _, message := range envelope.Result.Messages {
			current, wanted := resolved[message.OpenMessageID]
			if !wanted || chatRecordFileID(message.Content) == "" {
				continue
			}
			current.Content = message.Content
			current.OpenConversationID = message.OpenConversationID
			if strings.TrimSpace(current.CreateTime) == "" {
				current.CreateTime = message.CreateTime
			}
			resolved[message.OpenMessageID] = current
		}
	}
}

func recoveredForwardAttachment(message chatRecordMessage) (fileInboundInfo, bool) {
	content := strings.TrimSpace(message.Content)
	info := fileInboundInfo{
		OpenMessageID:      strings.TrimSpace(message.OpenMessageID),
		OpenConversationID: strings.TrimSpace(message.OpenConversationID),
	}
	if mediaID := chatRecordMediaID(content); mediaID != "" {
		info.MediaID = mediaID
		switch {
		case strings.Contains(content, "[图片消息]"):
			info.MediaType, info.FileName = "image", "转发图片"
		case strings.Contains(content, "[语音消息]"):
			info.MediaType, info.FileName = "audio", "转发语音.bin"
		case strings.Contains(content, "[视频消息]"):
			info.MediaType, info.FileName = "video", "转发视频.bin"
		default:
			info.MediaType, info.FileName = "file", "转发媒体.bin"
		}
		return info, true
	}
	if !looksLikeForwardedFile(content) {
		return fileInboundInfo{}, false
	}
	info.FileID = chatRecordFileID(content)
	info.FileName = chatRecordFileName(content)
	if info.FileName == "" {
		info.FileName = "转发文件"
	}
	info.MediaType = mediaTypeFromFileName(info.FileName)
	return info, true
}

func chatRecordMediaID(content string) string {
	match := chatRecordMediaIDPattern.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func chatRecordFileID(content string) string {
	match := chatRecordFileIDPattern.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func chatRecordFileName(content string) string {
	match := chatRecordFileNamePattern.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func looksLikeForwardedFile(content string) bool {
	return strings.Contains(content, "[文件]")
}

func looksLikeForwardedAttachment(content string) bool {
	return looksLikeForwardedFile(content) || chatRecordMediaID(content) != ""
}

func humanChatRecordContent(content string) string {
	content = chatRecordDownloadHintPattern.ReplaceAllString(content, "")
	content = chatRecordMediaIDPattern.ReplaceAllString(content, "")
	content = chatRecordFileIDPattern.ReplaceAllString(content, "")
	content = strings.TrimSpace(content)
	if content == "" {
		return "[无法提取文字内容]"
	}
	return content
}

func mediaTypeFromFileName(fileName string) string {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(fileName))) {
	case ".mp4", ".mov", ".m4v", ".avi", ".mkv", ".webm":
		return "video"
	case ".mp3", ".m4a", ".aac", ".wav", ".amr", ".ogg", ".flac":
		return "audio"
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".heic":
		return "image"
	default:
		return "file"
	}
}

func (c *aiCardClient) downloadRecoveredChatRecordFile(ctx context.Context, info fileInboundInfo) (string, error) {
	return c.downloadRecoveredChatRecordFileWithCall(ctx, info, callMCPToolReturnTextOnServer)
}

func (c *aiCardClient) downloadRecoveredChatRecordFileWithCall(ctx context.Context, info fileInboundInfo, call chatRecordToolCall) (string, error) {
	var raw string
	var err error
	switch {
	case strings.TrimSpace(info.MediaID) != "":
		raw, err = call(ctx, "im", "get_resource_download_url", map[string]any{
			"resourceType":       "mediaId",
			"resourceId":         strings.TrimSpace(info.MediaID),
			"openMessageId":      strings.TrimSpace(info.OpenMessageID),
			"openConversationId": strings.TrimSpace(info.OpenConversationID),
		})
	case strings.TrimSpace(info.FileID) != "":
		raw, err = call(ctx, "drive", "download_file", map[string]any{
			"fileId": strings.TrimSpace(info.FileID),
		})
	default:
		return "", fmt.Errorf("转发附件缺少 mediaId/fileId")
	}
	if err != nil {
		return "", err
	}
	resourceURL, headers, err := parseDownloadInfo(raw)
	if err != nil {
		return "", err
	}
	return downloadConnectURLToTemp(ctx, c.httpClient, resourceURL, headers, info.FileName)
}

func downloadConnectURLToTemp(ctx context.Context, client *http.Client, resourceURL string, headers map[string]string, fileName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resourceURL, nil)
	if err != nil {
		return "", err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := connectMediaDownloadClient(client).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("转发附件下载 HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > mediaMaxDownloadBytes {
		return "", fmt.Errorf("转发附件过大：%d 字节，最大允许 %d 字节", resp.ContentLength, mediaMaxDownloadBytes)
	}
	dir := filepath.Join(os.TempDir(), "dws-connect-media")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ext := filepath.Ext(filepath.Base(strings.TrimSpace(fileName)))
	if ext == "" {
		ext = mediaExt(resourceURL, resp.Header.Get("Content-Type"))
	}
	dest := filepath.Join(dir, uuid.NewString()+ext)
	if err := writeCompleteMediaFile(dest, resp.Body); err != nil {
		return "", err
	}
	return dest, nil
}
