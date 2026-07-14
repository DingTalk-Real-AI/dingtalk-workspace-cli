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
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// mediaMaxDownloadBytes caps a single inbound attachment download. Real merged
// forwards can contain high-resolution screen recordings above 250 MiB, so the
// connector allows up to 512 MiB but rejects larger payloads explicitly. It
// must never silently truncate a file and then tell an agent it has the
// original content.
const mediaMaxDownloadBytes = 512 << 20

const mediaDownloadTimeout = 5 * time.Minute

// connectAttachmentMIME returns the best MIME type available for a downloaded
// attachment. DingTalk's download API returns an opaque URL, so the connector
// must recover the type from the preserved filename and, when necessary, the
// actual bytes before handing it to a multimodal backend.
func connectAttachmentMIME(path string) string {
	if typ := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); typ != "" {
		typ = strings.TrimSpace(strings.SplitN(typ, ";", 2)[0])
		// Generic .bin paths are common for DingTalk voice messages. Sniff their
		// bytes instead of telling a multimodal backend they are opaque binary.
		if typ != "application/octet-stream" {
			return typ
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if n == 0 {
		return "application/octet-stream"
	}
	return http.DetectContentType(buf[:n])
}

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

func stringField(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// explicitPictureDownloadCode only accepts the picture-specific field. A
// generic downloadCode can represent any kind of attachment and must not be
// classified as an image merely because it is downloadable.
func explicitPictureDownloadCode(content interface{}) string {
	m, ok := content.(map[string]interface{})
	if !ok {
		return ""
	}
	return stringField(m, "pictureDownloadCode")
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
	pictures, _ := richTextInboundMedia(content)
	return pictures
}

// richTextInboundMedia applies the same capability-based discovery to every
// inline node. A non-picture node with a locator is preserved as an
// attachment instead of being discarded because its type is new or unknown.
func richTextInboundMedia(content interface{}) (pictureCodes []string, files []fileInboundInfo) {
	m, ok := content.(map[string]interface{})
	if !ok {
		return nil, nil
	}
	items, ok := m["richText"].([]interface{})
	if !ok {
		return nil, nil
	}
	seenPictures := make(map[string]struct{})
	seenFiles := make(map[string]struct{})
	for _, item := range items {
		node, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		typeHint := strings.ToLower(stringField(node, "type", "msgType", "msgtype"))
		code := ""
		if explicitPictureDownloadCode(node) != "" {
			code = pictureDownloadCode(node)
		}
		if code == "" && (typeHint == "" || typeHint == "picture" || typeHint == "image") {
			code = pictureDownloadCode(node)
		}
		if code != "" {
			if _, exists := seenPictures[code]; !exists {
				seenPictures[code] = struct{}{}
				pictureCodes = append(pictureCodes, code)
			}
			continue
		}
		info := parseTypedFileInbound(typeHint, node)
		if !info.hasActionable() {
			continue
		}
		key := fileInboundKey(info)
		if _, exists := seenFiles[key]; !exists {
			seenFiles[key] = struct{}{}
			files = append(files, info)
		}
	}
	return pictureCodes, files
}

// fileInboundInfo carries everything a msgtype="file" callback might expose.
// Client-sent files (a user attaching a file in the DingTalk client) surface
// DownloadCode + FileName; API-sent files (`dws chat message send --msg-type
// file --dentry-id --space-id`) surface DentryID + SpaceID + FileName +
// FileType + FileSize + FilePath and NO DownloadCode. Both shapes have to be
// recognisable or the connector silently drops legitimate file messages.
type fileInboundInfo struct {
	DownloadCode string
	// MediaID + message/conversation IDs identify media embedded in a
	// forwarded chat record. DingTalk's Stream callback can erase these into
	// unknownMsgType, while the user-state message API still preserves them.
	MediaID            string
	OpenMessageID      string
	OpenConversationID string
	// FileID is the dentryUuid returned by the user-state conversation API for
	// forwarded files. It is resolved through drive.download_file.
	FileID    string
	FileName  string
	FileType  string
	FilePath  string
	MediaType string
	DentryID  int64
	SpaceID   int64
	FileSize  int64
}

func (f fileInboundInfo) hasActionable() bool {
	return strings.TrimSpace(f.DownloadCode) != "" ||
		(strings.TrimSpace(f.MediaID) != "" && strings.TrimSpace(f.OpenMessageID) != "" && strings.TrimSpace(f.OpenConversationID) != "") ||
		strings.TrimSpace(f.FileID) != "" ||
		(f.DentryID != 0 && f.SpaceID != 0)
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
	info.DownloadCode = stringField(m, "downloadCode", "fileDownloadCode")
	info.MediaID = stringField(m, "mediaId", "mediaID")
	info.OpenMessageID = stringField(m, "openMessageId", "openMessageID")
	info.OpenConversationID = stringField(m, "openConversationId", "openConversationID")
	info.FileID = stringField(m, "fileId", "fileID", "dentryUuid", "dentryUUID")
	info.FileName = stringField(m, "fileName", "name")
	info.FileType = stringField(m, "fileType")
	info.FilePath = stringField(m, "filePath")
	info.MediaType = stringField(m, "mediaType")
	info.DentryID = readInt64Field(m, "dentryId", "dentryID")
	info.SpaceID = readInt64Field(m, "spaceId", "spaceID")
	info.FileSize = readInt64Field(m, "fileSize", "size")
	if info.FileName == "" {
		info.FileName = "未知文件"
	}
	return info
}

// inboundMediaType normalizes the callback spellings used for downloadable
// non-picture media. The returned value is only used to make the agent prompt
// precise; download authorization still comes exclusively from downloadCode
// or dentryId+spaceId.
func inboundMediaType(msgtype string) string {
	switch strings.ToLower(strings.TrimSpace(msgtype)) {
	case "image", "picture":
		return "image"
	case "audio", "voice":
		return "audio"
	case "video":
		return "video"
	default:
		return "file"
	}
}

func parseTypedFileInbound(msgtype string, content interface{}) fileInboundInfo {
	info := parseFileInbound(content)
	if mediaType := strings.TrimSpace(msgtype); mediaType != "" {
		info.MediaType = inboundMediaType(mediaType)
	}
	if info.MediaType == "" || info.MediaType == "file" {
		info.MediaType = mediaTypeFromFileName(info.FileName)
	}
	if info.FileName == "未知文件" {
		switch info.MediaType {
		case "audio":
			info.FileName = "语音消息"
		case "video":
			info.FileName = "视频消息"
		}
	}
	return info
}

func fileInboundKey(info fileInboundInfo) string {
	switch {
	case strings.TrimSpace(info.DownloadCode) != "":
		return "download:" + strings.TrimSpace(info.DownloadCode)
	case strings.TrimSpace(info.MediaID) != "":
		return "media:" + strings.TrimSpace(info.MediaID) + ":" + strings.TrimSpace(info.OpenMessageID) + ":" + strings.TrimSpace(info.OpenConversationID)
	case strings.TrimSpace(info.FileID) != "":
		return "file:" + strings.TrimSpace(info.FileID)
	case info.DentryID != 0 && info.SpaceID != 0:
		return fmt.Sprintf("dentry:%d:%d", info.SpaceID, info.DentryID)
	default:
		return ""
	}
}

// chatRecordInboundMedia extracts every actionable attachment that DingTalk
// preserved in a msgtype=chatRecord callback. The observed callback encodes
// the record array as a JSON string under content.chatRecord; accepting an
// already-decoded array as well keeps the parser compatible with SDK changes.
//
// Some forwarded entries arrive as {"msgType":"unknownMsgType"} with no
// message id, download code, or storage id. Those entries are counted for
// diagnostics but cannot be recovered by the connector because the callback
// contains no locator for the original bytes.
func chatRecordInboundMedia(content interface{}) (pictureCodes []string, files []fileInboundInfo, unrecoverableCount int) {
	m, ok := content.(map[string]interface{})
	if !ok {
		return nil, nil, 0
	}

	var entries []interface{}
	switch raw := m["chatRecord"].(type) {
	case string:
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return nil, nil, 0
		}
	case []interface{}:
		entries = raw
	default:
		// A few callback variants call the decoded array "contents".
		if decoded, ok := m["contents"].([]interface{}); ok {
			entries = decoded
		}
	}

	seenPictures := make(map[string]struct{})
	seenFiles := make(map[string]struct{})
	for _, entry := range entries {
		node, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		msgtype := ""
		for _, key := range []string{"msgType", "msgtype", "type"} {
			if value, ok := node[key].(string); ok && strings.TrimSpace(value) != "" {
				msgtype = strings.ToLower(strings.TrimSpace(value))
				break
			}
		}
		// Attachment discovery is capability-based. The nested msgType is a
		// classification hint only; a future/unknown type with a valid locator
		// must still reach the backend with its original bytes.
		pictureCode := ""
		if explicitPictureDownloadCode(node) != "" {
			pictureCode = pictureDownloadCode(node)
		}
		if pictureCode == "" && (msgtype == "picture" || msgtype == "image") {
			pictureCode = pictureDownloadCode(node)
		}
		if pictureCode != "" {
			if _, exists := seenPictures[pictureCode]; !exists {
				seenPictures[pictureCode] = struct{}{}
				pictureCodes = append(pictureCodes, pictureCode)
			}
			continue
		}

		info := parseTypedFileInbound(msgtype, node)
		if info.hasActionable() {
			key := fileInboundKey(info)
			if _, exists := seenFiles[key]; !exists {
				seenFiles[key] = struct{}{}
				files = append(files, info)
			}
			continue
		}
		if msgtype == "unknownmsgtype" {
			unrecoverableCount++
		}
	}
	return pictureCodes, files, unrecoverableCount
}

func hasChatRecordPayload(content interface{}) bool {
	m, ok := content.(map[string]interface{})
	if !ok {
		return false
	}
	switch raw := m["chatRecord"].(type) {
	case string:
		return strings.TrimSpace(raw) != ""
	case []interface{}:
		return true
	}
	_, hasDecodedContents := m["contents"].([]interface{})
	return hasDecodedContents
}

// callbackInboundMedia discovers downloadable payloads from their locator
// fields instead of an allowlist of msgtype values. msgtype is retained only
// as a media classification hint, so newly introduced message types are
// forwarded immediately without requiring a connector release.
func callbackInboundMedia(msgtype string, content interface{}) (pictureCodes []string, files []fileInboundInfo, unrecoverableCount int) {
	seenPictures := make(map[string]struct{})
	seenFiles := make(map[string]struct{})
	addPicture := func(code string) {
		code = strings.TrimSpace(code)
		if code == "" {
			return
		}
		if _, exists := seenPictures[code]; exists {
			return
		}
		seenPictures[code] = struct{}{}
		pictureCodes = append(pictureCodes, code)
	}
	addFile := func(info fileInboundInfo) {
		if !info.hasActionable() {
			return
		}
		key := fileInboundKey(info)
		if key == "" {
			return
		}
		if _, picture := seenPictures[info.DownloadCode]; picture {
			return
		}
		if _, exists := seenFiles[key]; exists {
			return
		}
		seenFiles[key] = struct{}{}
		files = append(files, info)
	}

	richPictures, richFiles := richTextInboundMedia(content)
	for _, code := range richPictures {
		addPicture(code)
	}
	for _, info := range richFiles {
		addFile(info)
	}
	pictureCode := ""
	if explicitPictureDownloadCode(content) != "" {
		pictureCode = pictureDownloadCode(content)
	}
	if pictureCode == "" && (strings.EqualFold(msgtype, "picture") || strings.EqualFold(msgtype, "image")) {
		pictureCode = pictureDownloadCode(content)
	}
	addPicture(pictureCode)
	addFile(parseTypedFileInbound(msgtype, content))

	nestedPictures, nestedFiles, nestedUnknown := chatRecordInboundMedia(content)
	for _, code := range nestedPictures {
		addPicture(code)
	}
	for _, info := range nestedFiles {
		addFile(info)
	}
	return pictureCodes, files, nestedUnknown
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

// rawCallbackPrompt preserves message types whose payload is meaningful but
// has no locally recognised text/media shape (for example msgtype=chatRecord).
// The connector should not decide that such messages are empty: forwarding
// the type and JSON payload lets the backend model interpret new and complex
// DingTalk message formats without waiting for a CLI-side parser update.
func rawCallbackPrompt(msgtype string, content interface{}) string {
	b, err := json.Marshal(content)
	if err != nil {
		b = []byte(fmt.Sprintf("%v", content))
	}
	return fmt.Sprintf("用户发送了一条钉钉消息，msgtype=%q。请解析以下原始消息 JSON，提取有用信息并处理用户意图：\n%s", strings.TrimSpace(msgtype), b)
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
		for _, key := range []string{"text", "content", "markdown", "title", "recognition"} {
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
	return c.downloadMessageFileNamed(ctx, robotCode, downloadCode, "")
}

// downloadMessageFileNamed is downloadMessageFile with an optional original
// file name. Keeping its extension materially improves audio/video/file
// handling across local agents whose tool selection depends on the path.
func (c *aiCardClient) downloadMessageFileNamed(ctx context.Context, robotCode, downloadCode, fileName string) (string, error) {
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
	resp, err := connectMediaDownloadClient(c.httpClient).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("媒体下载 HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > mediaMaxDownloadBytes {
		return "", fmt.Errorf("媒体文件过大：%d 字节，最大允许 %d 字节", resp.ContentLength, mediaMaxDownloadBytes)
	}
	dir := filepath.Join(os.TempDir(), "dws-connect-media")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ext := filepath.Ext(strings.TrimSpace(fileName))
	if ext == "" {
		ext = mediaExt(parsed.DownloadUrl, resp.Header.Get("Content-Type"))
	}
	dest := filepath.Join(dir, uuid.NewString()+ext)
	if err := writeCompleteMediaFile(dest, resp.Body); err != nil {
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
	resp, err := connectMediaDownloadClient(c.httpClient).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("钉盘文件下载 HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > mediaMaxDownloadBytes {
		return "", fmt.Errorf("钉盘文件过大：%d 字节，最大允许 %d 字节", resp.ContentLength, mediaMaxDownloadBytes)
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
	if err := writeCompleteMediaFile(dest, resp.Body); err != nil {
		return "", err
	}
	return dest, nil
}

// writeCompleteMediaFile writes at most mediaMaxDownloadBytes and verifies the
// stream ended. Reading one byte past the cap distinguishes an exact-size file
// from a larger file; the latter is removed instead of leaving a corrupt local
// artifact that an agent could mistake for the original.
func writeCompleteMediaFile(dest string, src io.Reader) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(f, io.LimitReader(src, mediaMaxDownloadBytes+1))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(dest)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dest)
		return closeErr
	}
	if n > mediaMaxDownloadBytes {
		_ = os.Remove(dest)
		return fmt.Errorf("媒体文件超过最大允许大小 %d 字节，未保存截断文件", mediaMaxDownloadBytes)
	}
	return nil
}

func connectMediaDownloadClient(base *http.Client) *http.Client {
	if base == nil {
		return &http.Client{Timeout: mediaDownloadTimeout}
	}
	clone := *base
	if clone.Timeout <= 0 || clone.Timeout < mediaDownloadTimeout {
		clone.Timeout = mediaDownloadTimeout
	}
	return &clone
}

func cleanupConnectMediaAttachments(attachments []connectMediaAttachment) {
	root := filepath.Join(os.TempDir(), "dws-connect-media")
	for _, attachment := range attachments {
		path := filepath.Clean(strings.TrimSpace(attachment.LocalPath))
		if path == "." || filepath.Dir(path) != root {
			continue
		}
		_ = os.Remove(path)
	}
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
