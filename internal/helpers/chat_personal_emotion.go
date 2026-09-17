package helpers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "image/png"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
	"github.com/spf13/cobra"
	"golang.org/x/image/bmp"
	"golang.org/x/image/webp"
)

const (
	personalEmotionUnpinnedReason     = "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command."
	personalEmotionUploadServerID     = "dingtalk-file"
	personalEmotionUploadMediaTool    = "upload_media"
	personalEmotionUploadMediaDisplay = personalEmotionUploadServerID + "/" + personalEmotionUploadMediaTool
)

func newChatEmotionCommand() *cobra.Command {
	cmd := newGroupCommand(&cobra.Command{
		Use:   "emotion",
		Short: "个人收藏表情",
		Long:  "查询、发送和新增当前用户的个人收藏表情。",
		RunE:  groupRunE,
	})
	cmd.AddCommand(
		newChatEmotionListCommand(),
		newChatEmotionSendCommand(),
		newChatEmotionFavoriteCommand(),
	)
	return cmd
}

func newChatEmotionListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出个人收藏表情",
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPToolOnServer("im", "list_personal_emotions", map[string]any{})
		},
	}
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "chat", Name: "list_personal_emotions",
				CanonicalPath: "chat.list_personal_emotions", CLIPath: "chat emotion list", PrimaryCLIPath: "chat emotion list",
			},
			Description: "列出当前用户的个人收藏表情",
			Interface: &contract.InterfaceSpec{
				Mode: "composite", Availability: "available", Reason: personalEmotionUnpinnedReason,
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "列出当前用户的个人收藏表情",
				UseWhen:      []string{"需要查看当前用户已收藏的表情、获取 emotionId 或 mediaId 时"},
				AvoidWhen:    []string{"查询消息 reaction 使用 chat message list-emotion-replies"},
				Examples:     []string{"dws chat emotion list --format json"},
			},
		},
	})
	return cmd
}

func newChatEmotionSendCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send",
		Short: "发送个人收藏表情",
		Long: `发送当前用户的个人收藏表情。

⚠️ 重要：该接口会真实发送表情到目标会话，不可用于测试或试探性调用。调用前必须确认表情媒体 ID 和接收对象无误。`,
		Example: `  dws chat emotion send --media-id <mediaId> --group <openConversationId>
  dws chat emotion send --media-id <mediaId> --emotion-id <emotionId> --user <userId>
  dws chat emotion send --media-id <mediaId> --open-dingtalk-id <openDingTalkId> --uuid <idempotencyKey>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mediaID, _ := cmd.Flags().GetString("media-id")
			if strings.TrimSpace(mediaID) == "" {
				return fmt.Errorf("--media-id is required")
			}
			target, err := personalEmotionSendTarget(cmd)
			if err != nil {
				return err
			}
			payload := map[string]any{"mediaId": strings.TrimSpace(mediaID)}
			emotionID, _ := cmd.Flags().GetString("emotion-id")
			if strings.TrimSpace(emotionID) != "" {
				payload["emotionId"] = strings.TrimSpace(emotionID)
			}
			for key, value := range target {
				payload[key] = value
			}
			if uuid := strings.TrimSpace(flagOrFallback(cmd, "uuid", "idempotency-key")); uuid != "" {
				payload["uuid"] = uuid
			}
			return callMCPToolOnServer("im", "send_personal_emotion", payload)
		},
	}
	cmd.Flags().String("media-id", "", "表情媒体 ID (必填)")
	cmd.Flags().String("emotion-id", "", "表情 ID")
	cmd.Flags().String("conversation-id", "", "群聊 openConversationId")
	cmd.Flags().String("group", "", "群聊 openConversationId（--conversation-id 别名）")
	cmd.Flags().String("user", "", "单聊接收人 userId；CLI 会解析为 openDingTalkId")
	cmd.Flags().String("open-dingtalk-id", "", "单聊接收人 openDingTalkId")
	cmd.Flags().String("uuid", "", "幂等键")
	cmd.Flags().String("idempotency-key", "", "幂等键（--uuid 别名）")
	cmd.MarkFlagsMutuallyExclusive("conversation-id", "group")
	cli.AnnotateRuntimeConstraints(cmd, cli.RuntimeSchemaConstraints{
		MutuallyExclusive: [][]string{{"conversation-id", "group", "user", "open-dingtalk-id"}},
		RequireOneOf:      [][]string{{"conversation-id", "group", "user", "open-dingtalk-id"}},
	})
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: personalEmotionSendContract(),
	})
	return cmd
}

func newChatEmotionFavoriteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "favorite",
		Short: "新增个人收藏表情",
		Example: `  dws chat emotion favorite --media-id <mediaId> --name "赞"
  dws chat emotion favorite --media-id <mediaId> --source-conversation-id <cid> --source-message-id <mid>
  dws chat emotion favorite --file-path ./sticker.png --name "本地表情"`,
		RunE: runChatEmotionFavorite,
	}
	cmd.Flags().String("media-id", "", "待收藏 mediaId；与 --file-path 二选一必填")
	cmd.Flags().String("file-path", "", "本地图片路径 (jpg/jpeg/png/gif/webp/bmp，≤10MB；超过2MB会先自动压缩)；与 --media-id 二选一必填")
	cmd.Flags().String("name", "", "表情名称")
	cmd.Flags().String("source-conversation-id", "", "来源会话 ID；需与 --source-message-id 成对指定")
	cmd.Flags().String("source-message-id", "", "来源消息 ID；需与 --source-conversation-id 成对指定")
	cmd.MarkFlagsMutuallyExclusive("media-id", "file-path")
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: personalEmotionFavoriteContract(),
	})
	return cmd
}

func runChatEmotionFavorite(cmd *cobra.Command, _ []string) error {
	mediaID, _ := cmd.Flags().GetString("media-id")
	mediaID = strings.TrimSpace(mediaID)
	filePath, _ := cmd.Flags().GetString("file-path")
	filePath = strings.TrimSpace(filePath)
	if mediaID == "" && filePath == "" {
		return fmt.Errorf("one of --media-id or --file-path is required")
	}
	sourceConversationID, _ := cmd.Flags().GetString("source-conversation-id")
	sourceMessageID, _ := cmd.Flags().GetString("source-message-id")
	if err := validatePersonalEmotionSourcePair(sourceConversationID, sourceMessageID); err != nil {
		return err
	}

	if filePath != "" {
		image, err := loadPersonalEmotionImageFile(filePath)
		if err != nil {
			return err
		}
		if deps.Caller.DryRun() {
			deps.Out.PrintKeyValue("操作", "上传本地图片并新增个人收藏表情")
			deps.Out.PrintKeyValue("文件", image.path)
			deps.Out.PrintKeyValue("大小", fmt.Sprintf("%d bytes", image.size))
			deps.Out.PrintKeyValue("图片类型", image.imageType)
			if image.compressed {
				deps.Out.PrintKeyValue("预处理", "已自动压缩到 2MB 以内")
			}
			return nil
		}
		uploadedMediaID, err := uploadPersonalEmotionImage(cmd.Context(), image)
		if err != nil {
			return err
		}
		payload := buildPersonalEmotionFavoritePayload(uploadedMediaID, cmd)
		text, err := callMCPToolReturnTextOnServer(cmd.Context(), "im", "favorite_personal_emotion", payload)
		if err != nil {
			return fmt.Errorf("本地图片已上传 (mediaId=%s)，但收藏失败: %w；可用 --media-id %s 重试收藏，无需重新上传", uploadedMediaID, err, uploadedMediaID)
		}
		return renderPersonalEmotionFavoriteWithMediaID(text, uploadedMediaID)
	}

	payload := buildPersonalEmotionFavoritePayload(mediaID, cmd)
	return callMCPToolOnServer("im", "favorite_personal_emotion", payload)
}

func renderPersonalEmotionFavoriteWithMediaID(text, mediaID string) error {
	augmented := text
	var payload map[string]any
	if json.Unmarshal([]byte(text), &payload) == nil {
		if result, ok := payload["result"].(map[string]any); ok {
			existing, _ := result["mediaId"].(string)
			if strings.TrimSpace(existing) == "" {
				result["mediaId"] = mediaID
			}
		} else {
			existing, _ := payload["mediaId"].(string)
			if strings.TrimSpace(existing) == "" {
				payload["mediaId"] = mediaID
			}
		}
		encoded, _ := json.Marshal(payload)
		augmented = string(encoded)
	}
	return renderLegacyMCPText("favorite_personal_emotion", augmented, false)
}

func buildPersonalEmotionFavoritePayload(mediaID string, cmd *cobra.Command) map[string]any {
	payload := map[string]any{"mediaId": mediaID}
	name, _ := cmd.Flags().GetString("name")
	if strings.TrimSpace(name) != "" {
		payload["name"] = strings.TrimSpace(name)
	}
	sourceConversationID, _ := cmd.Flags().GetString("source-conversation-id")
	sourceMessageID, _ := cmd.Flags().GetString("source-message-id")
	if strings.TrimSpace(sourceConversationID) != "" {
		payload["sourceConversationId"] = strings.TrimSpace(sourceConversationID)
		payload["sourceMessageId"] = strings.TrimSpace(sourceMessageID)
	}
	return payload
}

func personalEmotionSendTarget(cmd *cobra.Command) (map[string]any, error) {
	groupID := strings.TrimSpace(flagOrFallback(cmd, "conversation-id", "group"))
	userID, _ := cmd.Flags().GetString("user")
	openDingTalkID, _ := cmd.Flags().GetString("open-dingtalk-id")
	userID = strings.TrimSpace(userID)
	openDingTalkID = strings.TrimSpace(openDingTalkID)
	specified := 0
	for _, value := range []string{groupID, userID, openDingTalkID} {
		if value != "" {
			specified++
		}
	}
	if specified != 1 {
		return nil, fmt.Errorf("--conversation-id, --user or --open-dingtalk-id is required; specify exactly one")
	}
	if groupID != "" {
		return map[string]any{"openConversationId": groupID}, nil
	}
	if openDingTalkID != "" {
		if err := targetresolver.ValidateExplicitOpenDingTalkID("--open-dingtalk-id", openDingTalkID); err != nil {
			return nil, err
		}
		return map[string]any{"receiverOpenDingTalkId": openDingTalkID}, nil
	}
	if isOpenDingTalkID(userID) {
		return map[string]any{"receiverOpenDingTalkId": userID}, nil
	}
	resolved, err := resolveOpenDingTalkID(cmd.Context(), userID)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve --user %q to openDingTalkId: %w; pass --open-dingtalk-id instead", userID, err)
	}
	return map[string]any{"receiverOpenDingTalkId": resolved}, nil
}

func validatePersonalEmotionSourcePair(sourceConversationID, sourceMessageID string) error {
	hasConversation := strings.TrimSpace(sourceConversationID) != ""
	hasMessage := strings.TrimSpace(sourceMessageID) != ""
	if hasConversation != hasMessage {
		return fmt.Errorf("--source-conversation-id and --source-message-id must be specified together")
	}
	return nil
}

const (
	personalEmotionImageMaxBytes          = 2 * 1024 * 1024
	personalEmotionImageAutoCompressBytes = 10 * 1024 * 1024
	personalEmotionImageMinCompressWidth  = 64
	personalEmotionImageMinCompressHeight = 64
	personalEmotionImageMaxDimension      = 8192
	personalEmotionImageMaxPixels         = 32_000_000
	personalEmotionGIFMaxFramePixels      = 64_000_000
)

// personalEmotionImageTypes maps lowercased file extensions to the
// upload_media imageType enum. jpg and jpeg stay distinct values because
// the remote tool echoes them separately.
var personalEmotionImageTypes = map[string]string{
	".jpg":  "jpg",
	".jpeg": "jpeg",
	".png":  "png",
	".gif":  "gif",
	".webp": "webp",
	".bmp":  "bmp",
}

var (
	personalEmotionOSStat       = os.Stat
	personalEmotionOpenFile     = openPersonalEmotionFile
	personalEmotionCompress     = compressPersonalEmotionImage
	personalEmotionDecodeConfig = image.DecodeConfig
	personalEmotionWebPDecode   = webp.Decode
	personalEmotionWebPConfig   = webp.DecodeConfig
	personalEmotionBMPDecode    = bmp.Decode
	personalEmotionBMPConfig    = bmp.DecodeConfig
	personalEmotionGIFConfig    = gif.DecodeConfig
	personalEmotionGIFDecodeAll = gif.DecodeAll
	personalEmotionGIFEncodeAll = gif.EncodeAll
	personalEmotionJPEGEncode   = jpeg.Encode
)

type personalEmotionReadableFile interface {
	io.Reader
	Stat() (os.FileInfo, error)
	Close() error
}

func openPersonalEmotionFile(filePath string) (personalEmotionReadableFile, error) {
	return os.Open(filePath)
}

func personalEmotionImageType(ext string) (string, bool) {
	imageType, ok := personalEmotionImageTypes[strings.ToLower(strings.TrimSpace(ext))]
	return imageType, ok
}

type personalEmotionImage struct {
	path       string
	size       int64
	imageType  string
	content    string
	compressed bool
}

func validatePersonalEmotionImageFile(filePath string) (int64, string, error) {
	info, err := personalEmotionOSStat(filePath)
	if err != nil {
		return 0, "", fmt.Errorf("--file-path cannot read local image %s: %w", filePath, err)
	}
	if info.IsDir() {
		return 0, "", fmt.Errorf("--file-path points to a directory, expected an image file: %s", filePath)
	}
	if !info.Mode().IsRegular() {
		return 0, "", fmt.Errorf("--file-path points to a non-regular file, expected an image file: %s", filePath)
	}
	imageType, ok := personalEmotionImageType(filepath.Ext(filePath))
	if !ok {
		return 0, "", fmt.Errorf("--file-path only supports jpg/jpeg/png/gif/webp/bmp images, got: %s", filePath)
	}
	if info.Size() > personalEmotionImageAutoCompressBytes {
		return 0, "", fmt.Errorf("--file-path image size %d bytes exceeds the 10MB automatic compression limit: %s；文件过大，自动压缩耗时长且容易失真，可让 AI 先压缩图片到 2MB 以内后再重试，GIF 需要保留动图帧", info.Size(), filePath)
	}
	return info.Size(), imageType, nil
}

func loadPersonalEmotionImageFile(filePath string) (*personalEmotionImage, error) {
	size, imageType, err := validatePersonalEmotionImageFile(filePath)
	if err != nil {
		return nil, err
	}
	file, err := personalEmotionOpenFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("--file-path cannot read local image %s: %w", filePath, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("--file-path cannot inspect local image %s: %w", filePath, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("--file-path points to a directory, expected an image file: %s", filePath)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("--file-path points to a non-regular file, expected an image file: %s", filePath)
	}
	data, err := io.ReadAll(io.LimitReader(file, personalEmotionImageAutoCompressBytes+1))
	if err != nil {
		return nil, fmt.Errorf("--file-path cannot read local image %s: %w", filePath, err)
	}
	size = int64(len(data))
	if size > personalEmotionImageAutoCompressBytes {
		return nil, fmt.Errorf("--file-path image size %d bytes exceeds the 10MB automatic compression limit: %s；文件过大，自动压缩耗时长且容易失真，可让 AI 先压缩图片到 2MB 以内后再重试，GIF 需要保留动图帧", size, filePath)
	}
	if int64(len(data)) > personalEmotionImageMaxBytes {
		compressed, compressedImageType, err := personalEmotionCompress(data, imageType)
		if err != nil {
			return nil, fmt.Errorf("--file-path image size %d bytes exceeds the 2MB limit and automatic compression failed: %w；可让 AI 先压缩图片到 2MB 以内后再重试，GIF 需要保留动图帧", len(data), err)
		}
		data = compressed
		imageType = compressedImageType
		size = int64(len(data))
		if size > personalEmotionImageMaxBytes {
			return nil, fmt.Errorf("--file-path image size %d bytes exceeds the 2MB limit after automatic compression；可让 AI 先压缩图片到 2MB 以内后再重试，GIF 需要保留动图帧", size)
		}
		return &personalEmotionImage{
			path:       filePath,
			size:       size,
			imageType:  imageType,
			content:    base64.StdEncoding.EncodeToString(data),
			compressed: true,
		}, nil
	}
	return &personalEmotionImage{
		path:      filePath,
		size:      int64(len(data)),
		imageType: imageType,
		content:   base64.StdEncoding.EncodeToString(data),
	}, nil
}

func compressPersonalEmotionImage(data []byte, imageType string) ([]byte, string, error) {
	switch imageType {
	case "jpg", "jpeg", "png", "webp", "bmp":
		if err := validatePersonalEmotionImagePixels(data, imageType); err != nil {
			return nil, "", err
		}
		compressed, err := compressPersonalEmotionStillImage(data, imageType)
		return compressed, "jpg", err
	case "gif":
		if err := validatePersonalEmotionImagePixels(data, imageType); err != nil {
			return nil, "", err
		}
		compressed, err := compressPersonalEmotionGIF(data)
		return compressed, "gif", err
	default:
		return nil, "", fmt.Errorf("%s 暂不支持自动压缩", imageType)
	}
}

func validatePersonalEmotionImagePixels(data []byte, imageType string) error {
	cfg, err := decodePersonalEmotionImageConfig(data, imageType)
	if err != nil {
		return fmt.Errorf("图片尺寸读取失败: %w", err)
	}
	if err := validatePersonalEmotionPixelBudget(int64(cfg.Width), int64(cfg.Height), personalEmotionImageMaxPixels); err != nil {
		return err
	}
	if imageType == "gif" {
		if err := validatePersonalEmotionGIFFramePixels(data); err != nil {
			return err
		}
	}
	return nil
}

func decodePersonalEmotionImageConfig(data []byte, imageType string) (image.Config, error) {
	reader := bytes.NewReader(data)
	switch imageType {
	case "webp":
		return personalEmotionWebPConfig(reader)
	case "bmp":
		return personalEmotionBMPConfig(reader)
	case "gif":
		return personalEmotionGIFConfig(reader)
	default:
		cfg, _, err := personalEmotionDecodeConfig(reader)
		return cfg, err
	}
}

func validatePersonalEmotionPixelBudget(width, height, maxPixels int64) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("图片像素尺寸无效")
	}
	if width > personalEmotionImageMaxDimension || height > personalEmotionImageMaxDimension {
		return fmt.Errorf("图片像素尺寸过大，超过自动压缩安全限制；可让 AI 先缩小图片尺寸后再重试")
	}
	if width > maxPixels/height {
		return fmt.Errorf("图片像素尺寸过大，超过自动压缩安全限制；可让 AI 先缩小图片尺寸后再重试")
	}
	return nil
}

func validatePersonalEmotionGIFFramePixels(data []byte) error {
	reader := bytes.NewReader(data)
	header := make([]byte, 13)
	if _, err := io.ReadFull(reader, header); err != nil {
		return fmt.Errorf("GIF 头部读取失败: %w", err)
	}
	if string(header[:3]) != "GIF" {
		return fmt.Errorf("GIF 头部无效")
	}
	if header[10]&0x80 != 0 {
		if err := skipPersonalEmotionGIFBytes(reader, personalEmotionGIFColorTableBytes(header[10])); err != nil {
			return err
		}
	}
	totalPixels := int64(0)
	for {
		blockType, err := reader.ReadByte()
		if err != nil {
			return fmt.Errorf("GIF 数据读取失败: %w", err)
		}
		switch blockType {
		case 0x2c:
			if err := addPersonalEmotionGIFFramePixels(reader, &totalPixels); err != nil {
				return err
			}
		case 0x21:
			if _, err := reader.ReadByte(); err != nil {
				return fmt.Errorf("GIF 扩展块读取失败: %w", err)
			}
			if err := skipPersonalEmotionGIFSubBlocks(reader); err != nil {
				return err
			}
		case 0x3b:
			return nil
		default:
			return fmt.Errorf("GIF 数据块无效")
		}
	}
}

func addPersonalEmotionGIFFramePixels(reader *bytes.Reader, totalPixels *int64) error {
	descriptor := make([]byte, 9)
	if _, err := io.ReadFull(reader, descriptor); err != nil {
		return fmt.Errorf("GIF 帧描述读取失败: %w", err)
	}
	width := int64(binary.LittleEndian.Uint16(descriptor[4:6]))
	height := int64(binary.LittleEndian.Uint16(descriptor[6:8]))
	if width <= 0 || height <= 0 || width > personalEmotionGIFMaxFramePixels/height {
		return fmt.Errorf("图片像素尺寸过大，超过自动压缩安全限制；可让 AI 先缩小图片尺寸后再重试")
	}
	framePixels := width * height
	if *totalPixels > personalEmotionGIFMaxFramePixels-framePixels {
		return fmt.Errorf("图片像素尺寸过大，超过自动压缩安全限制；可让 AI 先缩小图片尺寸后再重试")
	}
	*totalPixels += framePixels
	if descriptor[8]&0x80 != 0 {
		if err := skipPersonalEmotionGIFBytes(reader, personalEmotionGIFColorTableBytes(descriptor[8])); err != nil {
			return err
		}
	}
	if _, err := reader.ReadByte(); err != nil {
		return fmt.Errorf("GIF 图像数据读取失败: %w", err)
	}
	return skipPersonalEmotionGIFSubBlocks(reader)
}

func personalEmotionGIFColorTableBytes(packed byte) int64 {
	return 3 * (1 << ((packed & 0x07) + 1))
}

func skipPersonalEmotionGIFSubBlocks(reader *bytes.Reader) error {
	for {
		size, err := reader.ReadByte()
		if err != nil {
			return fmt.Errorf("GIF 子块读取失败: %w", err)
		}
		if size == 0 {
			return nil
		}
		if err := skipPersonalEmotionGIFBytes(reader, int64(size)); err != nil {
			return err
		}
	}
}

func skipPersonalEmotionGIFBytes(reader *bytes.Reader, n int64) error {
	if n < 0 || int64(reader.Len()) < n {
		return fmt.Errorf("GIF 数据不完整")
	}
	_, _ = reader.Seek(n, io.SeekCurrent)
	return nil
}

func compressPersonalEmotionStillImage(data []byte, imageType string) ([]byte, error) {
	src, err := decodePersonalEmotionStillImage(data, imageType)
	if err != nil {
		return nil, fmt.Errorf("图片解码失败: %w", err)
	}
	bounds := src.Bounds()
	srcWidth, srcHeight := bounds.Dx(), bounds.Dy()
	for _, quality := range []int{88, 84, 80, 76, 72, 68} {
		for _, percent := range []int{100, 92, 84, 76, 68, 60, 52, 44, 36, 30, 25} {
			width := maxInt(personalEmotionImageMinCompressWidth, srcWidth*percent/100)
			height := maxInt(personalEmotionImageMinCompressHeight, srcHeight*percent/100)
			scaled := resizeBilinear(src, width, height)
			var out bytes.Buffer
			if err := personalEmotionJPEGEncode(&out, scaled, &jpeg.Options{Quality: quality}); err != nil {
				return nil, fmt.Errorf("图片压缩失败: %w", err)
			}
			if int64(out.Len()) <= personalEmotionImageMaxBytes {
				return out.Bytes(), nil
			}
		}
	}
	return nil, fmt.Errorf("压缩后仍超过 2MB")
}

func decodePersonalEmotionStillImage(data []byte, imageType string) (image.Image, error) {
	reader := bytes.NewReader(data)
	switch imageType {
	case "webp":
		return personalEmotionWebPDecode(reader)
	case "bmp":
		return personalEmotionBMPDecode(reader)
	default:
		src, _, err := image.Decode(reader)
		return src, err
	}
}

func compressPersonalEmotionGIF(data []byte) ([]byte, error) {
	src, err := personalEmotionGIFDecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("GIF 解码失败: %w", err)
	}
	if len(src.Image) == 0 {
		return nil, fmt.Errorf("GIF 没有可压缩的动图帧")
	}
	srcWidth, srcHeight := src.Config.Width, src.Config.Height
	if srcWidth <= 0 || srcHeight <= 0 {
		return nil, fmt.Errorf("GIF 尺寸无效")
	}
	for _, frameStep := range []int{1, 2, 3, 4} {
		for _, percent := range []int{100, 92, 84, 76, 68, 60, 52, 44, 36, 30, 25} {
			width := maxInt(personalEmotionImageMinCompressWidth, srcWidth*percent/100)
			height := maxInt(personalEmotionImageMinCompressHeight, srcHeight*percent/100)
			compressed := encodePersonalEmotionGIFCandidate(src, width, height, frameStep)
			var out bytes.Buffer
			if err := personalEmotionGIFEncodeAll(&out, compressed); err != nil {
				return nil, fmt.Errorf("GIF 压缩失败: %w", err)
			}
			if int64(out.Len()) <= personalEmotionImageMaxBytes {
				return out.Bytes(), nil
			}
		}
	}
	return nil, fmt.Errorf("压缩后仍超过 2MB")
}

func encodePersonalEmotionGIFCandidate(src *gif.GIF, width, height, frameStep int) *gif.GIF {
	indexes := personalEmotionGIFFrameIndexes(len(src.Image), frameStep)
	compressed := clonePersonalEmotionGIFMeta(src, width, height)
	compressed.Delay = make([]int, 0, len(indexes))
	compressed.Disposal = make([]byte, 0, len(indexes))
	canvas := image.NewRGBA(image.Rect(0, 0, src.Config.Width, src.Config.Height))
	var saved *image.RGBA
	nextSampled := 0
	for frameIndex, frame := range src.Image {
		// 先应用上一帧 disposal，再为本帧保存 Previous 快照，顺序不可换。
		if frameIndex > 0 {
			switch personalEmotionGIFDisposal(src.Disposal, frameIndex-1) {
			case gif.DisposalBackground:
				personalEmotionGIFClearRect(canvas, src.Image[frameIndex-1].Rect)
			case gif.DisposalPrevious:
				if saved != nil {
					copy(canvas.Pix, saved.Pix)
				}
			}
		}
		if personalEmotionGIFDisposal(src.Disposal, frameIndex) == gif.DisposalPrevious {
			if saved == nil {
				saved = image.NewRGBA(canvas.Bounds())
			}
			copy(saved.Pix, canvas.Pix)
		}
		drawPersonalEmotionGIFPartialFrame(canvas, frame)
		if nextSampled < len(indexes) && indexes[nextSampled] == frameIndex {
			scaled := resizeBilinear(canvas, width, height)
			compressed.Image = append(compressed.Image, personalEmotionGIFPalettizeSnapshot(scaled))
			compressed.Delay = append(compressed.Delay, personalEmotionGIFDelay(src.Delay, frameIndex, indexes, nextSampled))
			compressed.Disposal = append(compressed.Disposal, gif.DisposalNone)
			nextSampled++
		}
	}
	return compressed
}

func drawPersonalEmotionGIFPartialFrame(canvas *image.RGBA, frame *image.Paletted) {
	rect := frame.Rect.Intersect(canvas.Bounds())
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			r, g, b, a := frame.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			canvas.SetRGBA(x, y, color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)})
		}
	}
}

func personalEmotionGIFClearRect(canvas *image.RGBA, rect image.Rectangle) {
	rect = rect.Intersect(canvas.Bounds())
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			canvas.SetRGBA(x, y, color.RGBA{})
		}
	}
}

func personalEmotionGIFPalettizeSnapshot(snapshot *image.RGBA) *image.Paletted {
	bounds := snapshot.Bounds()
	colors, transparentIndex := personalEmotionGIFSnapshotPalette(snapshot)
	paletted := image.NewPaletted(bounds, colors)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := snapshot.RGBAAt(x, y)
			if c.A < 128 {
				paletted.SetColorIndex(x, y, uint8(transparentIndex))
				continue
			}
			opaque := color.RGBA{
				R: uint8(uint32(c.R) * 255 / uint32(c.A)),
				G: uint8(uint32(c.G) * 255 / uint32(c.A)),
				B: uint8(uint32(c.B) * 255 / uint32(c.A)),
				A: 255,
			}
			paletted.SetColorIndex(x, y, uint8(colors.Index(opaque)))
		}
	}
	return paletted
}

func personalEmotionGIFSnapshotPalette(snapshot *image.RGBA) (color.Palette, int) {
	bounds := snapshot.Bounds()
	distinct := make([]color.RGBA, 0, 256)
	seen := make(map[color.RGBA]struct{}, 256)
	hasTransparent := false
	overflow := false
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := snapshot.RGBAAt(x, y)
			if c.A < 128 {
				hasTransparent = true
				continue
			}
			opaque := color.RGBA{
				R: uint8(uint32(c.R) * 255 / uint32(c.A)),
				G: uint8(uint32(c.G) * 255 / uint32(c.A)),
				B: uint8(uint32(c.B) * 255 / uint32(c.A)),
				A: 255,
			}
			if _, ok := seen[opaque]; ok {
				continue
			}
			if len(distinct) >= 256 {
				overflow = true
				continue
			}
			seen[opaque] = struct{}{}
			distinct = append(distinct, opaque)
		}
	}
	transparentCount := 0
	if hasTransparent {
		transparentCount = 1
	}
	if overflow || len(distinct)+transparentCount > 256 {
		if hasTransparent {
			return append(color.Palette{color.RGBA{}}, palette.Plan9[:255]...), 0
		}
		return append(color.Palette(nil), palette.Plan9...), -1
	}
	if len(distinct) == 0 {
		return color.Palette{color.RGBA{}}, 0
	}
	colors := make(color.Palette, 0, len(distinct)+transparentCount)
	if hasTransparent {
		colors = append(colors, color.RGBA{})
	}
	for _, c := range distinct {
		colors = append(colors, c)
	}
	if hasTransparent {
		return colors, 0
	}
	return colors, -1
}

func personalEmotionGIFFrameIndexes(frameCount, step int) []int {
	if step < 1 {
		step = 1
	}
	indexes := make([]int, 0, (frameCount+step-1)/step)
	for i := 0; i < frameCount; i += step {
		indexes = append(indexes, i)
	}
	if frameCount > 1 && len(indexes) < 2 {
		indexes = append(indexes, frameCount-1)
	}
	return indexes
}

func personalEmotionGIFDelay(delays []int, frameIndex int, indexes []int, pos int) int {
	if len(delays) == 0 {
		return 0
	}
	nextIndex := len(delays)
	if pos+1 < len(indexes) {
		nextIndex = indexes[pos+1]
	}
	total := 0
	for i := frameIndex; i < nextIndex && i < len(delays); i++ {
		total += delays[i]
	}
	return total
}

func personalEmotionGIFDisposal(disposals []byte, frameIndex int) byte {
	if frameIndex >= 0 && frameIndex < len(disposals) {
		return disposals[frameIndex]
	}
	return 0
}

func resizeBilinear(src image.Image, width, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	bounds := src.Bounds()
	srcWidth, srcHeight := bounds.Dx(), bounds.Dy()
	if srcWidth <= 1 || srcHeight <= 1 || width <= 1 || height <= 1 {
		return resizeNearest(src, width, height)
	}
	for y := 0; y < height; y++ {
		fy := float64(y) * float64(srcHeight-1) / float64(height-1)
		y0 := int(fy)
		y1 := minInt(y0+1, srcHeight-1)
		wy := fy - float64(y0)
		for x := 0; x < width; x++ {
			fx := float64(x) * float64(srcWidth-1) / float64(width-1)
			x0 := int(fx)
			x1 := minInt(x0+1, srcWidth-1)
			wx := fx - float64(x0)
			c00 := color.RGBAModel.Convert(src.At(bounds.Min.X+x0, bounds.Min.Y+y0)).(color.RGBA)
			c10 := color.RGBAModel.Convert(src.At(bounds.Min.X+x1, bounds.Min.Y+y0)).(color.RGBA)
			c01 := color.RGBAModel.Convert(src.At(bounds.Min.X+x0, bounds.Min.Y+y1)).(color.RGBA)
			c11 := color.RGBAModel.Convert(src.At(bounds.Min.X+x1, bounds.Min.Y+y1)).(color.RGBA)
			dst.SetRGBA(x, y, bilinearColor(c00, c10, c01, c11, wx, wy))
		}
	}
	return dst
}

func bilinearColor(c00, c10, c01, c11 color.RGBA, wx, wy float64) color.RGBA {
	return color.RGBA{
		R: bilinearChannel(c00.R, c10.R, c01.R, c11.R, wx, wy),
		G: bilinearChannel(c00.G, c10.G, c01.G, c11.G, wx, wy),
		B: bilinearChannel(c00.B, c10.B, c01.B, c11.B, wx, wy),
		A: bilinearChannel(c00.A, c10.A, c01.A, c11.A, wx, wy),
	}
}

func bilinearChannel(c00, c10, c01, c11 uint8, wx, wy float64) uint8 {
	top := float64(c00)*(1-wx) + float64(c10)*wx
	bottom := float64(c01)*(1-wx) + float64(c11)*wx
	return uint8(top*(1-wy) + bottom*wy + 0.5)
}

func resizeNearest(src image.Image, width, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	bounds := src.Bounds()
	srcWidth, srcHeight := bounds.Dx(), bounds.Dy()
	for y := 0; y < height; y++ {
		srcY := bounds.Min.Y + y*srcHeight/height
		for x := 0; x < width; x++ {
			srcX := bounds.Min.X + x*srcWidth/width
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clonePersonalEmotionGIFMeta(src *gif.GIF, width, height int) *gif.GIF {
	return &gif.GIF{
		Delay:           append([]int(nil), src.Delay...),
		LoopCount:       src.LoopCount,
		Disposal:        append([]byte(nil), src.Disposal...),
		Config:          image.Config{ColorModel: color.Palette(nil), Width: width, Height: height},
		BackgroundIndex: src.BackgroundIndex,
	}
}

func uploadPersonalEmotionImage(ctx context.Context, image *personalEmotionImage) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	text, err := callMCPToolReturnTextOnServer(ctx, personalEmotionUploadServerID, personalEmotionUploadMediaTool, map[string]any{
		"content":   image.content,
		"imageType": image.imageType,
		"bizType":   "chat_emoticon",
	})
	if err != nil {
		return "", fmt.Errorf("上传本地图片到 %s 失败: %w", personalEmotionUploadMediaDisplay, err)
	}
	return parsePersonalEmotionUploadMediaID(text)
}

func parsePersonalEmotionUploadMediaID(text string) (string, error) {
	var payload struct {
		Success   bool   `json:"success"`
		MediaIDV1 string `json:"mediaIdV1"`
		MediaIDV2 string `json:"mediaIdV2"`
		ErrorCode string `json:"errorCode"`
		ErrorMsg  string `json:"errorMsg"`
		LogID     string `json:"logId"`
		Message   string `json:"message"`
	}
	if err := unmarshalJSONUseNumber(text, &payload); err != nil {
		return "", fmt.Errorf("%s 返回值无法解析为 JSON: %w", personalEmotionUploadMediaDisplay, err)
	}
	if !payload.Success {
		detail := strings.TrimSpace(payload.ErrorMsg)
		if detail == "" {
			detail = payload.Message
		}
		code := strings.TrimSpace(payload.ErrorCode)
		if code != "" {
			detail = strings.TrimSpace(detail + " (errorCode=" + code + ")")
		}
		if detail == "" {
			detail = "服务端未返回错误详情"
		}
		if logID := strings.TrimSpace(payload.LogID); logID != "" {
			detail = strings.TrimSpace(detail + " (logId=" + logID + ")")
		}
		return "", fmt.Errorf("%s 上传失败: %s", personalEmotionUploadMediaDisplay, detail)
	}
	mediaID := strings.TrimSpace(payload.MediaIDV1)
	if mediaID == "" {
		mediaID = strings.TrimSpace(payload.MediaIDV2)
	}
	if mediaID == "" {
		return "", fmt.Errorf("%s 返回 success=true 但缺少 mediaIdV1/mediaIdV2", personalEmotionUploadMediaDisplay)
	}
	return mediaID, nil
}

func personalEmotionSendContract() LeafContract {
	return LeafContract{
		Identity: contract.ToolIdentitySpec{
			ProductID: "chat", Name: "send_personal_emotion",
			CanonicalPath: "chat.send_personal_emotion", CLIPath: "chat emotion send", PrimaryCLIPath: "chat emotion send",
		},
		Description: "以当前用户身份向群聊或单聊发送个人收藏表情",
		Interface: &contract.InterfaceSpec{
			Mode: "composite", Availability: "available", Reason: personalEmotionUnpinnedReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "以当前用户身份发送个人收藏表情",
			UseWhen:      []string{"用户明确要求发送个人收藏表情，且已提供 mediaId 或 emotionId 时"},
			AvoidWhen:    []string{"发送普通文本、Markdown 或文件时使用 chat message send"},
			Examples:     []string{"dws chat emotion send --media-id <mediaId> --group <openConversationId> --uuid <idempotencyKey>"},
		},
		Parameters: []contract.ParamDecl{
			{Name: "media-id", Property: "mediaId", Required: boolPtr(true)},
			{Name: "emotion-id", Property: "emotionId", Required: boolPtr(false)},
			{Name: "conversation-id", Property: "openConversationId", Required: boolPtr(false)},
			{Name: "group", Property: "openConversationId", Required: boolPtr(false)},
			{Name: "user", Property: "receiverOpenDingTalkId", Required: boolPtr(false)},
			{Name: "open-dingtalk-id", Property: "receiverOpenDingTalkId", Required: boolPtr(false)},
			{Name: "uuid", Property: "uuid", Required: boolPtr(false)},
			{Name: "idempotency-key", Property: "uuid", Required: boolPtr(false)},
		},
	}
}

func personalEmotionFavoriteContract() LeafContract {
	return LeafContract{
		Identity: contract.ToolIdentitySpec{
			ProductID: "chat", Name: "favorite_personal_emotion",
			CanonicalPath: "chat.favorite_personal_emotion", CLIPath: "chat emotion favorite", PrimaryCLIPath: "chat emotion favorite",
		},
		Description: "将 mediaId 或本地图片新增到当前用户的个人收藏表情（本地图片经钉钉文件服务 upload_media 上传后收藏）",
		Interface: &contract.InterfaceSpec{
			Mode: "composite", Availability: "available", Reason: personalEmotionUnpinnedReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "新增当前用户的个人收藏表情，支持已有 mediaId 或本地图片文件",
			UseWhen:      []string{"用户要把一个 mediaId 或本地图片文件 (jpg/jpeg/png/gif/webp/bmp，≤10MB；超过2MB会先自动压缩) 收藏为个人表情时"},
			AvoidWhen:    []string{"收藏消息使用 chat message add-favorite"},
			Examples: []string{
				`dws chat emotion favorite --media-id <mediaId> --name "赞"`,
				"dws chat emotion favorite --file-path ./sticker.png --name \"本地表情\"",
			},
		},
		Parameters: []contract.ParamDecl{
			{Name: "media-id", Property: "mediaId", Required: boolPtr(false)},
			{Name: "file-path", Required: boolPtr(false)},
			{Name: "name", Property: "name", Required: boolPtr(false)},
			{Name: "source-conversation-id", Property: "sourceConversationId", Required: boolPtr(false), RequiredWhen: "source-message-id is provided"},
			{Name: "source-message-id", Property: "sourceMessageId", Required: boolPtr(false), RequiredWhen: "source-conversation-id is provided"},
		},
	}
}
