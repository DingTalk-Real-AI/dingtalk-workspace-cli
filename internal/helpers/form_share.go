package helpers

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"maps"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/skip2/go-qrcode"
	"github.com/spf13/cobra"
)

// ParseFormShareTime keeps both CLI entrances on the same explicit-timezone contract.
// Empty input is an error; only an explicit zero clears a time restriction.
func ParseFormShareTime(value, flagName string) (int64, error) {
	value = strings.TrimSpace(value)
	if millis, err := strconv.ParseInt(value, 10, 64); err == nil {
		return millis, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0, apperrors.NewValidation(fmt.Sprintf("--%s 只接受毫秒时间戳或带时区的 RFC3339 时间", flagName))
	}
	return parsed.UnixMilli(), nil
}

// ValidateFormShareViewMode keeps the client-side contract aligned with the
// server enum: 0 means the latest submission and 1 means submission history.
func ValidateFormShareViewMode(value int) error {
	if value != 0 && value != 1 {
		return apperrors.NewValidation("--submit-record-view-mode 只接受 0（最近一次）或 1（全部历史）")
	}
	return nil
}

// Keep the exact host list aligned with the reviewed MCP formCover protocol.
const formCoverAllowedHosts = "img.alicdn.com、static.dingtalk.com、down-cdn.dingtalk.com、alidocs.oss-cn-zhangjiakou.aliyuncs.com、alidocs2.oss-cn-zhangjiakou.aliyuncs.com、alidocs.oss-cn-hangzhou.aliyuncs.com"

// FormCoverURLDescription is shared by both CLI entrances and their Schema declarations.
const FormCoverURLDescription = "HTTPS 封面地址；主机名仅允许 " + formCoverAllowedHosts + "（精确匹配，不含子域名），仅默认端口或 443；其他图片需先上传为当前 Base 的图片资源；省略不修改，空值/空白值不支持清除；禁止空白、未编码的引号/括号、鉴权信息、查询参数、片段及 /core/api/resources 内部路径；文件名中的引号/括号须百分号编码；仅校验 URL 结构，不验证可访问性或图片内容；与本地文件/资源 ID 互斥"

// FormCoverResourceDescription keeps resource recovery subject to the same Base and identity.
const FormCoverResourceDescription = "当前 Base 已上传的非空图片资源 ID，不含首尾空白；服务端按当前身份转为公开封面，转存 URL 须满足 form-cover 的可信域名、端口及格式限制；权限或资源校验失败不更新配置，公开操作无法随后续配置失败回滚；先回读配置，已有符合限制的目标公开 URL 时人工核验后使用 form-cover"

// ValidateFormShareURL only checks URL structure, not reachability or image contents.
// An omitted flag preserves the current cover; an explicit empty value never clears it.
func ValidateFormShareURL(value string) error {
	if strings.TrimSpace(value) == "" {
		return apperrors.NewValidation("--form-cover 不支持空值或空白值，不支持清除封面；省略该参数可保留原封面")
	}
	parsed, err := url.Parse(value)
	// net/url folds the scheme to lower case; MCP's URI comparison requires literal https.
	if err != nil || strings.IndexFunc(value, unicode.IsSpace) >= 0 || !strings.HasPrefix(value, "https://") || parsed.Hostname() == "" || parsed.User != nil || parsed.ForceQuery || parsed.RawQuery != "" || strings.ContainsAny(value, "\\#\"'()<>[]{}|^`") {
		return apperrors.NewValidation("--form-cover 只接受不带空白、未编码引号/括号、鉴权信息、查询参数或片段的可信 HTTPS URL")
	}
	if !slices.Contains(strings.Split(formCoverAllowedHosts, "、"), strings.ToLower(parsed.Hostname())) {
		return apperrors.NewValidation("--form-cover 主机名必须精确匹配 Help/Schema 中的可信域名；其他图片请先上传为当前 Base 的图片资源")
	}
	if rawPort := parsed.Port(); rawPort != "" {
		port, err := strconv.Atoi(rawPort)
		if err != nil || port != 443 {
			return apperrors.NewValidation("--form-cover 仅允许默认端口或 443")
		}
	}
	// URL.Path is decoded; clean dot segments before checking the authenticated resource route.
	cleanPath := path.Clean("/" + parsed.Path)
	if cleanPath == "/core/api/resources" || strings.HasPrefix(cleanPath, "/core/api/resources/") {
		return apperrors.NewValidation("--form-cover 不接受 /core/api/resources 内部鉴权资源路径；请使用当前 Base 的 --form-cover-resource-id")
	}
	return nil
}

// writeFormShareURL only projects an enabled, unpunished configuration; UUID presence
// alone is insufficient because closing a form preserves its UUID.
func writeFormShareURL(cmd *cobra.Command, raw string) error {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return fmt.Errorf("get_share_form_config 返回无效 JSON: %w", err)
	}
	// Only unwrap known envelopes, never recursively pick a UUID out of error details.
	for _, key := range []string{"data", "result"} {
		if nested, ok := payload[key]; ok {
			return writeFormShareURL(cmd, string(nested))
		}
	}
	var config struct {
		UUID     string `json:"shareFormUuid"`
		Enabled  bool   `json:"enabled"`
		Punished bool   `json:"punished"`
	}
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return fmt.Errorf("分享配置字段类型无效: %w", err)
	}
	if !config.Enabled || config.Punished || strings.TrimSpace(config.UUID) == "" {
		return apperrors.NewValidation("表单分享未开启、被处罚或缺少 shareFormUuid，无法交付填写链接")
	}
	link := formShareURLPrefix + url.PathEscape(config.UUID)
	data := map[string]any{"shareFormUuid": config.UUID, "shareUrl": link, "copyableUrl": link}
	if cmd.Flags().Changed("qr-file") {
		png, err := qrcode.Encode(link, qrcode.Medium, 512)
		if err != nil {
			return fmt.Errorf("生成二维码失败: %w", err)
		}
		artifact, err := localio.PublishBytes(png, localio.PublishBytesOptions{BaseDir: ".", Output: mustGetFlag(cmd, "qr-file"), PreferredName: "form-qr.png"})
		if err != nil {
			return err
		}
		data["qrFile"] = artifact.AbsolutePath
	}
	// 由根执行边界统一输出，避免已启用统一契约的命令直接写信封却未提交 CommandResult。
	return output.StoreResult(cmd.Context(), output.Success(data))
}

// RunAitableFormShareUpdate shares cover validation and local-file orchestration with the Shortcut.
// Public resource conversion, authorization and read-back remain in MCP and its downstream services.
func RunAitableFormShareUpdate(cmd *cobra.Command, params map[string]any, update func(map[string]any) error) error {
	if value, present := params["formCover"]; present {
		cover, ok := value.(string)
		if !ok {
			return apperrors.NewValidation("--form-cover 必须是非空 HTTPS URL")
		}
		if err := ValidateFormShareURL(cover); err != nil {
			return err
		}
	}
	if value, present := params["formCoverResourceId"]; present {
		resourceID, ok := value.(string)
		if !ok || resourceID == "" || strings.TrimSpace(resourceID) != resourceID {
			return apperrors.NewValidation("--form-cover-resource-id 必须非空且不含首尾空白")
		}
	}
	if !cmd.Flags().Changed("form-cover-file") {
		return update(params)
	}
	local, err := openFloatImageLocalFileNamed(mustGetFlag(cmd, "form-cover-file"), "--form-cover-file")
	if err != nil {
		// Missing paths are invalid input; permission and other I/O errors retain their classification.
		if errors.Is(err, os.ErrNotExist) {
			return apperrors.NewValidation(err.Error())
		}
		return err
	}
	defer local.file.Close()
	if local.size > 10<<20 {
		return apperrors.NewValidation("封面图片不能超过 10 MiB")
	}
	_, format, err := image.DecodeConfig(io.LimitReader(local.file, 10<<20))
	// DecodeConfig 使用进程全局解码器；其他模块注册 BMP/WebP 不应扩大封面的支持范围。
	if err != nil || (format != "png" && format != "jpeg" && format != "gif") {
		return apperrors.NewValidation("封面必须是有效的 PNG/JPEG/GIF 图片")
	}
	local.mimeType = "image/" + format
	// 下游按扩展名区分图片资源；使用已识别的真实格式，不信任输入文件名。
	local.name = strings.TrimSuffix(local.name, filepath.Ext(local.name)) + "." + format
	if deps.Caller.DryRun() {
		params["formCoverResourceId"] = "<uploaded-image-resource-id>"
		return update(params)
	}
	// Reuse the document resource upload contract for the same Base. The server
	// receives the resource ID, not its authenticated URL or temporary signed URL.
	info, err := resolveFloatImageUploadInfo(cmd.Context(), params["baseId"], local)
	if err != nil {
		return err
	}
	if err := putFloatImageFileWithClient(cmd.Context(), newFloatImageUploadClient(), info.uploadURL, local); err != nil {
		return err
	}
	params["formCoverResourceId"] = info.resourceID
	if err := update(params); err != nil {
		return formCoverUpdateError(err, info.resourceID)
	}
	return nil
}

// Retain recovery metadata in the structured error, not only its human wrapper.
// Preserve classification and diagnostics, but disallow retrying the whole command:
// replay would upload another resource before attempting the configuration update.
func formCoverUpdateError(err error, resourceID string) error {
	recovery := fmt.Sprintf("封面已上传（resourceId=%s），配置结果未确认；先回读 form share get，勿重复上传或原样重放；资源公开无法随后续配置失败回滚", resourceID)
	detailsWithResource := func(details map[string]any) map[string]any {
		cloned := maps.Clone(details)
		if cloned == nil {
			cloned = map[string]any{}
		}
		cloned["formCoverResourceId"] = resourceID
		return cloned
	}
	var typed *apperrors.Error
	if errors.As(err, &typed) {
		cloned := *typed
		cloned.Message = recovery + ": " + err.Error()
		cloned.Hint = recovery + "。" + typed.Hint
		cloned.Details = detailsWithResource(typed.Details)
		cloned.Retryable = false
		cloned.RetryableSet = true
		cloned.RetryAfterSeconds = nil
		cloned.NextRetryAt = nil
		cloned.Cause = err
		return &cloned
	}
	var cliError *CLIError
	if errors.As(err, &cliError) {
		cloned := *cliError
		cloned.Message = recovery + ": " + cliError.Message
		cloned.Suggestion = recovery + "。" + cliError.Suggestion
		cloned.Details = detailsWithResource(cliError.Details)
		cloned.Cause = err
		return &cloned
	}
	return fmt.Errorf("%s: %w", recovery, err)
}
