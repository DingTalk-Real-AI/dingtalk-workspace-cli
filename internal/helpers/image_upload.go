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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// 业务通用图片上传原语（执行 helper 与声明辅助，不构造 *cobra.Command）。
// 以 chat_personal_emotion.go 的现有上传链为蓝本复制抽取；bizType、大小上限与
// 产物用途按业务参数化。叶子命令由各业务域用 NewLeafCommand 声明，经 Call 钩子
// 接入 runImageUpload（首个调用方：OA 审批图片 oa approval image upload，见
// oa_image_upload.go）；本文件不得提供参数化命令工厂拼真 API（见 leaf.go 规则）。
// 表情链路文件与调用方保持零改动，是否回并去重由后续单独评估。

const (
	imageUploadServerID = "dingtalk-file"
	imageUploadTool     = "upload_media"
	imageUploadDisplay  = imageUploadServerID + "/" + imageUploadTool
	// imageUploadAutoCompressMaxBytes 是可走自动压缩的源文件硬上限（对齐表情链路 10MB）。
	imageUploadAutoCompressMaxBytes = 10 * 1024 * 1024
)

// imageCompress 复用表情链路的压缩实现（compressPersonalEmotionImage，同包私有函数、
// 零改动复用，不迁移表情链路的调用方）。
// 测试经 testseam.Swap 替换。
var imageCompress = compressPersonalEmotionImage

// imageUploadOSStat / imageUploadOpenFile 抽象本地文件系统边界（同表情链路
// personalEmotion* seam 模式）；测试经 testseam.Swap 注入 stat/open/读取失败。
var (
	imageUploadOSStat   = os.Stat
	imageUploadOpenFile = openImageUploadFile
)

// imageUploadReadableFile 是上传源文件的最小读取面（读取 + 元数据 + 关闭）。
type imageUploadReadableFile interface {
	io.Reader
	Stat() (os.FileInfo, error)
	Close() error
}

func openImageUploadFile(filePath string) (imageUploadReadableFile, error) {
	// O_NONBLOCK：路径在预检 Stat 与 Open 之间被替换为 FIFO 时，不带该标志的
	// 读端 open 会无限阻塞等待写者，打开后 descriptor fstat 复核永远执行不到。
	// 带 O_NONBLOCK 打开后由 fstat 复核统一拒绝非普通文件；该标志对普通文件的
	// 读取无语义影响（POSIX），Windows 的 syscall.Open 忽略此位。
	return os.OpenFile(filePath, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}

// imageUploadExts 与 dingtalk-file 服务端声明的图片类型集合一致（小写扩展名 → imageType）。
var imageUploadExts = map[string]string{
	".jpg":  "jpg",
	".jpeg": "jpeg",
	".png":  "png",
	".gif":  "gif",
	".webp": "webp",
	".bmp":  "bmp",
}

// imageUploadSpec 承载一个业务域图片上传原语的执行参数与稳定身份。help 文案
// 与 Contract 其余声明写在各业务域的 NewLeafCommand 叶子旁；本结构只保留执行
// 差异与身份数据，供 runImageUpload / imageUploadFlagUsage / imageUploadDataSchema
// 与叶子声明引用。
type imageUploadSpec struct {
	// BizType 是 upload_media 的业务场景标识（如 approval_image），由服务端按业务开通；
	// 未开通时命令以服务端错误如实失败，不做 bizType 回退。
	BizType string
	// MaxBytes 是上传图片的硬上限（字节）。
	MaxBytes int64
	// Compress 为 true 时，>MaxBytes（≤10MB）的图片来源先自动压缩（有损，静图转为
	// jpg，GIF 保留动图帧）至 ≤MaxBytes 再上传；为 false 时超 MaxBytes 直接拒绝。
	// 隐含约束（当前压缩实现固定行为，新接入方须知）：压缩目标值固定为 ≤2MB（零改动
	// 复用表情压缩链 compressPersonalEmotionImage），故声明 MaxBytes<2MB 时压缩产物可能
	// 仍超限而拒绝、MaxBytes>2MB 时会过度压缩；自动压缩读取上限固定为 10MB
	// （imageUploadAutoCompressMaxBytes），故 MaxBytes 不得超过 10MB。需要其他目标值时
	// 应先将压缩链参数化，再声明对应 MaxBytes。
	Compress bool
	// MediaUsage 描述产物 mediaIdV2 的业务用途（用于 Result schema 与错误文案），
	// 形如「组装 DDPhotoField value（数组转义字符串）」。
	MediaUsage string
	// Identity 是叶子命令的稳定身份（产品、canonical path、CLI path），供叶子
	// Contract 声明与 dry-run plan 输出引用。
	Identity contract.ToolIdentitySpec
}

// imageUploadFile 是校验通过后的待上传图片。
type imageUploadFile struct {
	size       int64
	imageType  string
	content    string // base64 编码后的文件字节
	compressed bool   // 上传前是否经过自动压缩（>MaxBytes 触发）
}

// imageUploadContentMatchesImageType 校验文件内容 magic bytes 与扩展名声明的
// imageType 一致（webp 需 RIFF 头 + 偏移 8 处的 WEBP 标识）。防止把非图片字节
// 改扩展名后直传；≤MaxBytes 的直传路径与压缩路径同样核对。
func imageUploadContentMatchesImageType(data []byte, imageType string) bool {
	switch imageType {
	case "jpg", "jpeg":
		return len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF
	case "png":
		return len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n"
	case "gif":
		return len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a")
	case "webp":
		return len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP"
	case "bmp":
		return len(data) >= 2 && data[0] == 'B' && data[1] == 'M'
	}
	return false
}

// loadImageUploadFile 校验并读取本地图片：扩展名白名单、常规文件、内容 magic
// bytes 与扩展名一致；≤spec.MaxBytes 直传；spec.Compress 为 true 时 >MaxBytes
// （≤10MB）自动压缩至 ≤MaxBytes，为 false 时超 MaxBytes 直接拒绝。
func loadImageUploadFile(filePath string, spec imageUploadSpec) (*imageUploadFile, error) {
	imageType, ok := imageUploadExts[strings.ToLower(filepath.Ext(filePath))]
	if !ok {
		return nil, apperrors.NewValidation(fmt.Sprintf("--file 仅支持 jpg/jpeg/png/gif/webp/bmp 图片: %s", filePath))
	}
	info, err := imageUploadOSStat(filePath)
	if err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("--file 无法读取本地图片 %s: %v", filePath, err))
	}
	if info.IsDir() {
		return nil, apperrors.NewValidation(fmt.Sprintf("--file 指向目录，需要图片文件: %s", filePath))
	}
	if !info.Mode().IsRegular() {
		return nil, apperrors.NewValidation(fmt.Sprintf("--file 指向非普通文件，需要图片文件: %s", filePath))
	}
	file, err := imageUploadOpenFile(filePath)
	if err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("--file 无法读取本地图片 %s: %v", filePath, err))
	}
	defer file.Close()
	// 打开后的 descriptor 复核（TOCTOU：Stat 与 Open 之间路径可能被替换为 FIFO/
	// 设备文件，后续 ReadAll 会阻塞）；与表情链路 loadPersonalEmotionImageFile 的
	// fstat 复核同构。
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("--file 无法读取本地图片 %s: %v", filePath, err))
	}
	if fileInfo.IsDir() {
		return nil, apperrors.NewValidation(fmt.Sprintf("--file 指向目录，需要图片文件: %s", filePath))
	}
	if !fileInfo.Mode().IsRegular() {
		return nil, apperrors.NewValidation(fmt.Sprintf("--file 指向非普通文件，需要图片文件: %s", filePath))
	}
	readCap := spec.MaxBytes
	if spec.Compress {
		readCap = imageUploadAutoCompressMaxBytes
	}
	data, err := io.ReadAll(io.LimitReader(file, readCap+1))
	if err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("--file 无法读取本地图片 %s: %v", filePath, err))
	}
	if len(data) == 0 {
		return nil, apperrors.NewValidation(fmt.Sprintf("--file 图片文件为空: %s", filePath))
	}
	if !imageUploadContentMatchesImageType(data, imageType) {
		return nil, apperrors.NewValidation(fmt.Sprintf("--file 文件内容与扩展名声明的 %s 图片格式不符，需要真实图片文件: %s", imageType, filePath))
	}
	maxMB := spec.MaxBytes / (1024 * 1024)
	if int64(len(data)) > readCap {
		return nil, apperrors.NewValidation(fmt.Sprintf("--file 图片大小 %d 字节超过 %dMB 上限: %s；可让 AI 先压缩图片到 %dMB 以内后重试，GIF 需要保留动图帧", len(data), readCap/(1024*1024), filePath, maxMB))
	}
	image := &imageUploadFile{
		size:      int64(len(data)),
		imageType: imageType,
		content:   base64.StdEncoding.EncodeToString(data),
	}
	if int64(len(data)) <= spec.MaxBytes {
		return image, nil
	}
	compressed, compressedType, err := imageCompress(data, imageType)
	if err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("--file 图片大小 %d 字节超过 %dMB 上限且自动压缩失败: %v；可让 AI 先压缩图片到 %dMB 以内后重试，GIF 需要保留动图帧", len(data), maxMB, err, maxMB))
	}
	if len(compressed) > int(spec.MaxBytes) {
		return nil, apperrors.NewValidation(fmt.Sprintf("--file 图片大小 %d 字节超过 %dMB 上限且自动压缩后仍超限: %s；可让 AI 先压缩图片到 %dMB 以内后重试，GIF 需要保留动图帧", len(data), maxMB, filePath, maxMB))
	}
	image.size = int64(len(compressed))
	image.imageType = compressedType
	image.content = base64.StdEncoding.EncodeToString(compressed)
	image.compressed = true
	return image, nil
}

// uploadImageMedia 调用 dingtalk-file/upload_media 上传图片字节，bizType 取自 spec。
func uploadImageMedia(ctx context.Context, spec imageUploadSpec, image *imageUploadFile) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	text, err := callMCPToolReturnTextOnServer(ctx, imageUploadServerID, imageUploadTool, map[string]any{
		"content":   image.content,
		"imageType": image.imageType,
		"bizType":   spec.BizType,
	})
	if err != nil {
		return "", fmt.Errorf("上传本地图片到 %s 失败: %w", imageUploadDisplay, err)
	}
	return text, nil
}

// parseImageUploadResult 解析 upload_media 返回：必须有 mediaIdV2（下游业务组装依赖
// V2 形态），mediaIdV1 仅作辅助留痕。
func parseImageUploadResult(text string, mediaUsage string) (map[string]any, error) {
	var payload struct {
		Success   bool   `json:"success"`
		MediaIDV1 string `json:"mediaIdV1"`
		MediaIDV2 string `json:"mediaIdV2"`
		BizType   string `json:"bizType"`
		ImageType string `json:"imageType"`
		ErrorCode string `json:"errorCode"`
		ErrorMsg  string `json:"errorMsg"`
		LogID     string `json:"logId"`
		Message   string `json:"message"`
	}
	if err := unmarshalJSONUseNumber(text, &payload); err != nil {
		return nil, fmt.Errorf("%s 返回值无法解析为 JSON: %w", imageUploadDisplay, err)
	}
	if !payload.Success {
		detail := strings.TrimSpace(payload.ErrorMsg)
		if detail == "" {
			detail = payload.Message
		}
		if code := strings.TrimSpace(payload.ErrorCode); code != "" {
			detail = strings.TrimSpace(detail + " (errorCode=" + code + ")")
		}
		if detail == "" {
			detail = "服务端未返回错误详情"
		}
		if logID := strings.TrimSpace(payload.LogID); logID != "" {
			detail = strings.TrimSpace(detail + " (logId=" + logID + ")")
		}
		return nil, fmt.Errorf("%s 上传失败: %s", imageUploadDisplay, detail)
	}
	mediaIDV2 := strings.TrimSpace(payload.MediaIDV2)
	if mediaIDV2 == "" {
		return nil, fmt.Errorf("%s 返回 success=true 但缺少 mediaIdV2，无法%s", imageUploadDisplay, mediaUsage)
	}
	result := map[string]any{
		"mediaIdV2": mediaIDV2,
	}
	if mediaIDV1 := strings.TrimSpace(payload.MediaIDV1); mediaIDV1 != "" {
		result["mediaIdV1"] = mediaIDV1
	}
	return result, nil
}

// runImageUpload 端到端上传：本地校验读取 → dingtalk-file/upload_media → 返回
// mediaIdV2 供业务方组装提交。由各业务域叶子的 Call 钩子调用。
func runImageUpload(spec imageUploadSpec, cmd *cobra.Command) error {
	filePath := strings.TrimSpace(mustGetFlag(cmd, "file"))
	if filePath == "" {
		return apperrors.NewValidation("--file 不能为空")
	}
	image, err := loadImageUploadFile(filePath, spec)
	if err != nil {
		return err
	}

	// --dry-run：本地校验读取已完成，在远程调用前 early return 输出 plan 预览。
	// 本命令是 RolloutUnifiedActive，必须经 output.StoreResult 存入统一结果；
	// 不伪造 mediaIdV2——它只能由远程调用返回。
	if deps.Caller.DryRun() {
		return output.StoreResult(cmd.Context(), output.Success(map[string]any{
			"dry_run":      true,
			"executed":     false,
			"preview_kind": "plan",
			"operation":    spec.Identity.Name,
			"source":       spec.Identity.ProductID,
			"file":         filePath,
			"image_type":   image.imageType,
			"size":         image.size,
			"compressed":   image.compressed,
			"steps": []map[string]any{
				{
					"tool":   imageUploadDisplay,
					"args":   map[string]any{"bizType": spec.BizType, "imageType": image.imageType, "size": image.size},
					"status": "planned",
				},
			},
		}, output.WithDryRun()))
	}

	text, err := uploadImageMedia(cmd.Context(), spec, image)
	if err != nil {
		return err
	}
	result, err := parseImageUploadResult(text, spec.MediaUsage)
	if err != nil {
		return err
	}
	result["bizType"] = spec.BizType
	result["imageType"] = image.imageType
	result["size"] = image.size
	result["compressed"] = image.compressed
	result["file"] = filePath
	return output.StoreResult(cmd.Context(), output.Success(result))
}

// imageUploadFlagUsage 生成 --file flag 的 usage 文案（由各业务域的叶子声明引用）。
func imageUploadFlagUsage(spec imageUploadSpec) string {
	if spec.Compress {
		return fmt.Sprintf("本地图片路径 (jpg/jpeg/png/gif/webp/bmp，≤%dMB；超过 %dMB 自动压缩) (必填)", imageUploadAutoCompressMaxBytes/(1024*1024), spec.MaxBytes/(1024*1024))
	}
	return fmt.Sprintf("本地图片路径 (jpg/jpeg/png/gif/webp/bmp，≤%dMB，超过请先压缩) (必填)", spec.MaxBytes/(1024*1024))
}

// imageUploadDataSchema 生成上传结果的 Result dataSchema（mediaIdV2 用途与
// bizType 随业务声明）。
func imageUploadDataSchema(spec imageUploadSpec) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"type":"object","description":"图片上传业务结果或未执行的上传计划","oneOf":[{"type":"object","description":"图片上传结果","properties":{"mediaIdV2":{"type":"string","description":"图片媒体 ID，用于%s"},"mediaIdV1":{"type":"string","description":"旧版媒体 ID（仅服务端返回时携带）"},"bizType":{"type":"string","description":"上传业务场景标识（%s）"},"imageType":{"type":"string","description":"实际上传的图片类型（jpg/jpeg/png/gif/webp/bmp；自动压缩后静图为 jpg）"},"size":{"type":"integer","description":"实际上传的图片字节数"},"compressed":{"type":"boolean","description":"上传前是否经自动压缩（源图超过上限时触发，有损）"},"file":{"type":"string","description":"源文件路径"}},"required":["mediaIdV2","bizType","imageType","size","compressed"],"additionalProperties":true,"not":{"required":["dry_run"]}},{"type":"object","description":"本地图片已校验的上传计划，未执行上传","properties":{"dry_run":{"type":"boolean","const":true,"description":"仅预览上传"},"executed":{"type":"boolean","const":false,"description":"未执行远程上传"},"preview_kind":{"type":"string","const":"plan","description":"本地处理后的上传计划"},"operation":{"type":"string","const":"%s","description":"待执行的操作"},"source":{"type":"string","const":"%s","description":"业务产品"},"file":{"type":"string","minLength":1,"description":"已校验的源文件路径"},"image_type":{"type":"string","enum":["jpg","jpeg","png","gif","webp","bmp"],"description":"待上传的图片类型"},"size":{"type":"integer","minimum":0,"description":"待上传的字节数"},"compressed":{"type":"boolean","description":"是否已本地压缩"},"steps":{"type":"array","description":"待执行的上传步骤","minItems":1,"maxItems":1,"items":{"type":"object","properties":{"tool":{"type":"string","const":"%s","description":"上传服务与工具"},"args":{"type":"object","description":"上传计划参数，不包含图片正文","properties":{"bizType":{"type":"string","const":"%s","description":"上传业务场景"},"imageType":{"type":"string","description":"待上传的图片类型"},"size":{"type":"integer","minimum":0,"description":"待上传的字节数"}},"required":["bizType","imageType","size"],"additionalProperties":false},"status":{"type":"string","const":"planned","description":"步骤尚未执行"}},"required":["tool","args","status"],"additionalProperties":false}}},"required":["dry_run","executed","preview_kind","operation","source","file","image_type","size","compressed","steps"],"additionalProperties":false}]}`, spec.MediaUsage, spec.BizType, spec.Identity.Name, spec.Identity.ProductID, imageUploadDisplay, spec.BizType))
}
