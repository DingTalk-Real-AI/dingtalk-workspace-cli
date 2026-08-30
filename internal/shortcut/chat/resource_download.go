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

package chat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var (
	resourceGetwd        = os.Getwd
	resourceAbs          = filepath.Abs
	resourceEvalSymlinks = filepath.EvalSymlinks
	resourceStat         = os.Stat
	resourceLstat        = os.Lstat
	resourceRel          = filepath.Rel
	resourceMkdir        = os.Mkdir
	resourceCreateTemp   = os.CreateTemp
	resourceCopy         = io.Copy
	resourceTempSync     = (*os.File).Sync
	resourceTempClose    = (*os.File).Close
	resourceRename       = replaceFileAtomically
	resourceLink         = os.Link
	resourceDownload     = downloadResourceAtomically
	resourceSecureClient = localio.SecureHTTPClient
)

// MessagesResourceDownload resolves a temporary IM resource URL and saves the
// bytes through a safe, atomic, no-clobber local-file workflow.
var MessagesResourceDownload = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+messages-resource-download",
	Product:     "im",
	Description: "安全下载消息资源（图片/视频/语音/文件）到本地",
	Intent: "当你需要拿到消息里的实际图片、视频、语音或钉盘文件，而不只是资源 ID 时使用；" +
		"mediaId 用消息和会话身份换取下载地址，fileId 复用钉盘下载能力，再安全写入工作目录内的相对路径。" +
		"默认不覆盖已有文件，只有显式传 --overwrite 才覆盖；下载采用整文件临时落盘后原子发布，不支持 Range 断点续传。按既有安全本地下载约定无需交互确认。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "type", Type: shortcut.FlagString, Default: "mediaId", Desc: "资源类型；--type mediaId 时必须同时提供 --message-id 和 --open-conversation-id；fileId 不需要消息上下文", Enum: []string{"mediaId", "fileId"}},
		{Name: "resource-id", Type: shortcut.FlagString, Desc: "消息中的 mediaId 或 fileId", Required: true},
		{Name: "message-id", Type: shortcut.FlagString, Desc: "mediaId 所属消息的 openMessageId；--type mediaId 时必须同时提供 --message-id 和 --open-conversation-id；fileId 不需要消息上下文"},
		{Name: "open-conversation-id", Type: shortcut.FlagString, Desc: "mediaId 所属会话的 openConversationId；--type mediaId 时必须同时提供 --message-id 和 --open-conversation-id；fileId 不需要消息上下文"},
		{Name: "output", Type: shortcut.FlagString, Default: ".", Desc: "工作目录内的相对路径；不允许绝对路径或 .. 逃逸"},
		{Name: "overwrite", Type: shortcut.FlagBool, Desc: "允许覆盖工作目录内已存在的目标文件（默认拒绝）"},
	},
	Constraints: []shortcut.Constraint{
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"output"},
			Description: "--output 必须是工作目录内的相对路径，不允许绝对路径或 .. 逃逸",
		},
		{
			Kind:        shortcut.ConstraintCustom,
			Flags:       []string{"type", "message-id", "open-conversation-id"},
			Description: "--type mediaId 时必须同时提供 --message-id 和 --open-conversation-id；fileId 不需要消息上下文",
		},
	},
	Tips: []string{
		`dws chat +messages-resource-download --resource-id <mediaId> --message-id <openMessageId> --open-conversation-id <openConversationId>`,
		`dws chat +messages-resource-download --type fileId --resource-id <fileId> --output ./downloads/`,
		`dws chat +messages-resource-download --resource-id <mediaId> --message-id <openMessageId> --open-conversation-id <openConversationId> --output ./downloads/`,
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if err := validateResourceDownloadOutput(rt.Str("output")); err != nil {
			return err
		}
		resourceType, _ := canonicalMessageResourceType(rt.Str("type"))
		if resourceType == "mediaId" &&
			(strings.TrimSpace(rt.Str("message-id")) == "" ||
				strings.TrimSpace(rt.Str("open-conversation-id")) == "") {
			return apperrors.NewValidation(
				"--type mediaId 时必须同时提供 --message-id 和 --open-conversation-id")
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		resourceType, _ := canonicalMessageResourceType(rt.Str("type"))
		plan := map[string]any{
			"resourceType":       resourceType,
			"resourceId":         rt.Str("resource-id"),
			"messageId":          rt.Str("message-id"),
			"openConversationId": rt.Str("open-conversation-id"),
			"output":             rt.Str("output"),
			"overwrite":          rt.Bool("overwrite"),
		}
		if rt.DryRun() {
			plan["dryRun"] = true
			plan["steps"] = []string{
				"resolve temporary download URL",
				"validate output path and no-clobber policy",
				"download to a temporary file",
				"atomically publish the completed file",
			}
			return rt.Output(plan)
		}

		data, err := resolveMessageResourceDownloadData(
			rt,
			resourceType,
			rt.Str("resource-id"),
			rt.Str("message-id"),
			rt.Str("open-conversation-id"),
		)
		if err != nil {
			return err
		}
		resourceURL, headers, err := resourceDownloadInfo(data)
		if err != nil {
			return err
		}
		cwd, err := resourceGetwd()
		if err != nil {
			return apperrors.NewInternal(fmt.Sprintf("读取工作目录失败: %v", err))
		}
		destPath, relativePath, err := resolveResourceDownloadPath(
			cwd,
			rt.Str("output"),
			resourceURL,
			rt.Bool("overwrite"),
			resourceDownloadPreferredName(data),
		)
		if err != nil {
			return err
		}
		size, err := resourceDownload(
			rt.Command().Context(), nil, resourceURL, headers, destPath, rt.Bool("overwrite"))
		if err != nil {
			return err
		}
		return rt.Output(map[string]any{
			"messageId":    rt.Str("message-id"),
			"resourceId":   rt.Str("resource-id"),
			"resourceType": resourceType,
			"localPath":    filepath.ToSlash(relativePath),
			"sizeBytes":    size,
		})
	},
}

func resolveMessageResourceDownloadData(
	rt *shortcut.RuntimeContext,
	resourceType, resourceID, messageID, conversationID string,
) (map[string]any, error) {
	resourceType, ok := canonicalMessageResourceType(resourceType)
	if !ok {
		return nil, apperrors.NewValidation(fmt.Sprintf(
			"不支持的消息资源类型 %q；仅支持 mediaId 或 fileId", resourceType))
	}
	if resourceType == "fileId" {
		return rt.CallMCPData("drive", "download_file", map[string]any{
			"fileId": resourceID,
		})
	}
	return rt.CallMCPData("im", "get_resource_download_url", map[string]any{
		"resourceType":       "mediaId",
		"resourceId":         resourceID,
		"openMessageId":      messageID,
		"openConversationId": conversationID,
	})
}

func canonicalMessageResourceType(resourceType string) (string, bool) {
	switch {
	case strings.EqualFold(strings.TrimSpace(resourceType), "mediaId"):
		return "mediaId", true
	case strings.EqualFold(strings.TrimSpace(resourceType), "fileId"):
		return "fileId", true
	default:
		return strings.TrimSpace(resourceType), false
	}
}

func validateResourceDownloadOutput(output string) error {
	return validateResourceDownloadOutputFlag(output, "--output")
}

func validateResourceDownloadOutputFlag(output, flagName string) error {
	output = strings.TrimSpace(output)
	if output == "" {
		return apperrors.NewValidation(flagName + " 不能为空")
	}
	if resourcePathIsAbsolute(output) {
		return apperrors.NewValidation(flagName + " 只接受工作目录内的相对路径")
	}
	if resourcePathEscapesBase(output) {
		return apperrors.NewValidation(flagName + " 不允许使用 .. 逃逸工作目录")
	}
	return nil
}

func resourcePathIsAbsolute(value string) bool {
	if filepath.IsAbs(value) {
		return true
	}
	portable := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if pathpkg.IsAbs(portable) {
		return true
	}
	return len(portable) >= 2 &&
		((portable[0] >= 'a' && portable[0] <= 'z') ||
			(portable[0] >= 'A' && portable[0] <= 'Z')) &&
		portable[1] == ':'
}

// resourcePathEscapesBase reports whether a relative path escapes its base.
// Normalize both separators so the check remains portable on every host OS.
func resourcePathEscapesBase(value string) bool {
	portable := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	clean := pathpkg.Clean(portable)
	return clean == ".." || strings.HasPrefix(clean, "../")
}

func resourceDownloadInfo(data map[string]any) (string, map[string]string, error) {
	for {
		result, ok := data["result"].(map[string]any)
		if !ok {
			break
		}
		data = result
	}

	resourceURL := ""
	switch value := data["resourceUrl"].(type) {
	case string:
		resourceURL = strings.TrimSpace(value)
	case []any:
		for _, item := range value {
			if candidate, ok := item.(string); ok && strings.TrimSpace(candidate) != "" {
				resourceURL = strings.TrimSpace(candidate)
				break
			}
		}
	}
	if resourceURL == "" {
		if value, ok := data["downloadUrl"].(string); ok {
			resourceURL = strings.TrimSpace(value)
		}
	}
	parsed, err := url.Parse(resourceURL)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return "", nil, apperrors.NewAPI(
			"资源下载接口未返回合法的 HTTPS 下载地址")
	}
	if parsed.Scheme == "http" && isAliyunOSSHost(parsed.Hostname()) {
		// The IM backend currently returns a signed plaintext OSS URL even
		// though the same object endpoint supports TLS. Upgrade only the
		// official OSS hostname; never rewrite arbitrary HTTP origins.
		parsed.Scheme = "https"
		resourceURL = parsed.String()
	}
	parsed, err = validateResourceDownloadURL(resourceURL)
	if err != nil {
		return "", nil, apperrors.NewAPI(fmt.Sprintf(
			"资源下载接口返回了不受信任的下载地址: %v", err))
	}
	resourceURL = parsed.String()

	headers := map[string]string{}
	if values, ok := data["headers"].(map[string]any); ok {
		for key, value := range values {
			if text, ok := value.(string); ok && strings.TrimSpace(key) != "" {
				headers[key] = text
			}
		}
	}
	return resourceURL, headers, nil
}

func resourceDownloadPreferredName(data map[string]any) string {
	for {
		result, ok := data["result"].(map[string]any)
		if !ok {
			break
		}
		data = result
	}
	name, _ := data["fileName"].(string)
	return strings.TrimSpace(name)
}

func isAliyunOSSHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if !strings.HasSuffix(host, ".aliyuncs.com") {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, ".aliyuncs.com"), ".") {
		if (label == "oss" || strings.HasPrefix(label, "oss-")) &&
			!strings.Contains(label, "internal") {
			return true
		}
	}
	return false
}

// validateResourceDownloadURL enforces the shared localio download URL
// policy: HTTPS URLs without userinfo, on any host or port, IP literals
// included. Resource URLs are always resolved from an authenticated MCP
// response (never user-supplied), and localio.SecureHTTPClient keeps TLS
// hostname verification plus redirect credential hygiene, mirroring the
// official GUI client which applies no client-side SSRF interception to
// downloads.
func validateResourceDownloadURL(rawURL string) (*url.URL, error) {
	parsed, err := localio.ValidateDownloadURL(rawURL)
	if err != nil {
		return nil, apperrors.NewValidation(err.Error())
	}
	return parsed, nil
}

func resolveResourceDownloadPath(
	baseDir, output, resourceURL string,
	overwrite bool,
	preferredName ...string,
) (absolutePath, relativePath string, err error) {
	if err := validateResourceDownloadOutput(output); err != nil {
		return "", "", err
	}
	baseDir, err = resourceAbs(baseDir)
	if err != nil {
		return "", "", apperrors.NewInternal(fmt.Sprintf("解析工作目录失败: %v", err))
	}
	realBase, err := resourceEvalSymlinks(baseDir)
	if err != nil {
		return "", "", apperrors.NewInternal(fmt.Sprintf("解析工作目录失败: %v", err))
	}

	rawOutput := strings.TrimSpace(output)
	directoryIntent := strings.HasSuffix(rawOutput, string(os.PathSeparator)) ||
		strings.HasSuffix(rawOutput, "/")
	output = filepath.Clean(rawOutput)
	candidate := filepath.Join(realBase, output)
	info, statErr := resourceStat(candidate)
	isDirectory := (statErr == nil && info.IsDir()) ||
		directoryIntent ||
		output == "."
	if isDirectory {
		candidate = filepath.Join(candidate, resourceDownloadFilename(resourceURL, preferredName...))
	}

	parent := filepath.Dir(candidate)
	if err := ensureResourceDownloadParent(realBase, parent); err != nil {
		return "", "", err
	}
	realParent, err := resourceEvalSymlinks(parent)
	if err != nil {
		return "", "", apperrors.NewInternal(fmt.Sprintf("解析输出目录失败: %v", err))
	}
	parentRel, err := resourceRel(realBase, realParent)
	if err != nil || resourcePathEscapesBase(parentRel) {
		return "", "", apperrors.NewValidation("--output 解析后逃逸工作目录")
	}

	absolutePath = filepath.Join(realParent, filepath.Base(candidate))
	if info, statErr := resourceLstat(absolutePath); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", "", apperrors.NewValidation("--output 目标不能是符号链接")
		}
		if info.IsDir() {
			return "", "", apperrors.NewValidation("--output 目标是目录，无法写入文件")
		}
		if !overwrite {
			return "", "", apperrors.NewValidation(
				"目标文件已存在；如确认覆盖请显式传 --overwrite")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", "", apperrors.NewInternal(fmt.Sprintf("检查输出文件失败: %v", statErr))
	}
	relativePath, err = resourceRel(realBase, absolutePath)
	if err != nil {
		return "", "", apperrors.NewInternal(fmt.Sprintf("解析输出相对路径失败: %v", err))
	}
	return absolutePath, relativePath, nil
}

func ensureResourceDownloadParent(baseDir, parent string) error {
	relative, err := resourceRel(baseDir, parent)
	if err != nil || resourcePathEscapesBase(relative) {
		return apperrors.NewValidation("--output 解析后逃逸工作目录")
	}
	if relative == "." {
		return nil
	}

	current := baseDir
	for _, part := range strings.Split(relative, string(os.PathSeparator)) {
		current = filepath.Join(current, part)
		info, statErr := resourceLstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			if mkdirErr := resourceMkdir(current, 0o755); mkdirErr != nil {
				// A concurrent creator may have won the race. Re-check the
				// resulting entry instead of following it implicitly.
				info, statErr = resourceLstat(current)
				if statErr != nil {
					return apperrors.NewInternal(fmt.Sprintf(
						"创建输出目录失败: %v", mkdirErr))
				}
			} else {
				continue
			}
		} else if statErr != nil {
			return apperrors.NewInternal(fmt.Sprintf(
				"检查输出目录失败: %v", statErr))
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return apperrors.NewValidation("--output 父目录不能是符号链接")
		}
		if !info.IsDir() {
			return apperrors.NewValidation("--output 父路径不是目录")
		}
	}
	return nil
}

func resourceDownloadFilename(resourceURL string, preferredName ...string) string {
	if len(preferredName) > 0 {
		if name := safeResourceDownloadFilename(preferredName[0]); name != "" {
			return name
		}
	}
	parsed, err := url.Parse(resourceURL)
	if err == nil {
		name, unescapeErr := url.PathUnescape(filepath.Base(parsed.Path))
		if unescapeErr == nil {
			if name = safeResourceDownloadFilename(name); name != "" {
				return name
			}
		}
	}
	return "download"
}

// safeResourceDownloadFilename returns a portable basename or an empty string
// for names that are unsafe or unusable on a supported platform. Server-provided
// file names are untrusted input, and downloads may be prepared on one OS then
// consumed on another.
func safeResourceDownloadFilename(raw string) string {
	normalized := strings.ReplaceAll(raw, "\\", "/")
	if strings.TrimSpace(normalized) != normalized {
		return ""
	}
	name := filepath.Base(normalized)
	if name == "" || name == "." || name == ".." ||
		strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return ""
	}
	for _, char := range name {
		if char < 0x20 || char == 0x7f || strings.ContainsRune(`<>:"/\|?*`, char) {
			return ""
		}
	}

	stem := name
	if index := strings.IndexByte(stem, '.'); index >= 0 {
		stem = stem[:index]
	}
	stem = strings.ToUpper(strings.TrimRight(stem, " ."))
	switch stem {
	case "CON", "PRN", "AUX", "NUL":
		return ""
	}
	if len(stem) == 4 &&
		(stem[:3] == "COM" || stem[:3] == "LPT") &&
		stem[3] >= '1' && stem[3] <= '9' {
		return ""
	}
	return name
}

func downloadResourceAtomically(
	ctx context.Context,
	client *http.Client,
	resourceURL string,
	headers map[string]string,
	destPath string,
	overwrite bool,
) (size int64, err error) {
	if client == nil {
		client = resourceSecureClient()
	}
	parsedResourceURL, err := validateResourceDownloadURL(resourceURL)
	if err != nil {
		return 0, err
	}
	// The resource URL and any credential headers are issued together by the
	// same authenticated MCP response, so the initial request forwards them
	// as-is even for dedicated-deployment storage hosts. The attack surface
	// is a redirect leaving the original host, which the header-stripping
	// CheckRedirect below covers.
	clientCopy := *client
	client = &clientCopy
	originalRedirect := client.CheckRedirect
	initialHost := strings.ToLower(parsedResourceURL.Hostname())
	headersConfinedToInitialHost := true
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if _, redirectErr := validateResourceDownloadURL(req.URL.String()); redirectErr != nil {
			return apperrors.NewValidation(fmt.Sprintf(
				"资源下载重定向指向了不受信任的地址: %v", redirectErr))
		}
		if len(via) >= 10 {
			return apperrors.NewAPI("资源下载重定向次数过多")
		}
		if !strings.EqualFold(req.URL.Hostname(), initialHost) {
			headersConfinedToInitialHost = false
		}
		// net/http rebuilds redirect headers from the initial request on every
		// hop. Once a chain leaves the original host, strip lower-service
		// headers on every later hop so a same-host redirect on the new origin
		// cannot silently restore them.
		if !headersConfinedToInitialHost {
			for key := range headers {
				req.Header.Del(key)
			}
		}
		if originalRedirect != nil {
			return originalRedirect(req, via)
		}
		return nil
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodGet, parsedResourceURL.String(), nil)
	if err != nil {
		return 0, apperrors.NewValidation(fmt.Sprintf("创建资源下载请求失败: %v", err))
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, apperrors.NewAPI(fmt.Sprintf("下载消息资源失败: %v", err))
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return 0, apperrors.NewAPI(fmt.Sprintf(
			"下载消息资源失败: HTTP %d", response.StatusCode))
	}

	parent := filepath.Dir(destPath)
	temp, err := resourceCreateTemp(parent, "."+filepath.Base(destPath)+".part-*")
	if err != nil {
		return 0, apperrors.NewInternal(fmt.Sprintf("创建下载临时文件失败: %v", err))
	}
	tempPath := temp.Name()
	defer func() {
		_ = resourceTempClose(temp)
		_ = os.Remove(tempPath)
	}()

	size, err = resourceCopy(temp, response.Body)
	if err != nil {
		return 0, apperrors.NewAPI(fmt.Sprintf("写入消息资源失败: %v", err))
	}
	if response.ContentLength >= 0 && size != response.ContentLength {
		return 0, apperrors.NewAPI(fmt.Sprintf(
			"消息资源大小校验失败: 下载 %d 字节，期望 %d 字节",
			size, response.ContentLength))
	}
	if err := resourceTempSync(temp); err != nil {
		return 0, apperrors.NewInternal(fmt.Sprintf("同步消息资源失败: %v", err))
	}
	if err := resourceTempClose(temp); err != nil {
		return 0, apperrors.NewInternal(fmt.Sprintf("关闭消息资源失败: %v", err))
	}
	if overwrite {
		if err := resourceRename(tempPath, destPath); err != nil {
			return 0, apperrors.NewInternal(fmt.Sprintf("发布消息资源失败: %v", err))
		}
		return size, nil
	}
	if err := resourceLink(tempPath, destPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return 0, apperrors.NewValidation(
				"目标文件已存在；如确认覆盖请显式传 --overwrite")
		}
		return 0, apperrors.NewInternal(fmt.Sprintf("发布消息资源失败: %v", err))
	}
	return size, nil
}

func init() {
	shortcut.Register(withReviewedChatShortcutContracts(MessagesResourceDownload)...)
}
