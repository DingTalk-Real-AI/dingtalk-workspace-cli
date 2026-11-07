package helpers

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"time"
)

// loadChatUploadImageFile keeps the existing favorite-image limits and codecs
// while validating small images before they enter a Markdown message.
func loadChatUploadImageFile(path string) (*personalEmotionImage, error) {
	image, err := loadPersonalEmotionImageFile(path)
	if err != nil {
		return nil, err
	}
	if err := validateChatUploadImageContent(image); err != nil {
		return nil, err
	}
	return image, nil
}

func validateChatUploadImageContent(image *personalEmotionImage) error {
	data, err := base64.StdEncoding.DecodeString(image.content)
	if err != nil {
		return fmt.Errorf("图片内容编码无效: %w", err)
	}
	if !chatImageSignatureMatches(data, image.imageType) {
		return fmt.Errorf("图片内容与扩展名不匹配")
	}
	if err := validatePersonalEmotionImagePixels(data, image.imageType); err != nil {
		return err
	}
	return nil
}

func chatImageSignatureMatches(data []byte, imageType string) bool {
	switch imageType {
	case "jpg", "jpeg":
		return len(data) >= 3 && bytes.Equal(data[:3], []byte{0xff, 0xd8, 0xff})
	case "png":
		return len(data) >= 8 && bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n"))
	case "gif":
		return bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a"))
	case "webp":
		return len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP"))
	case "bmp":
		return bytes.HasPrefix(data, []byte("BM"))
	default:
		return false
	}
}

func uploadChatImage(ctx context.Context, image *personalEmotionImage, bizType string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	text, err := callMCPToolReturnTextOnServer(ctx, personalEmotionUploadServerID, personalEmotionUploadMediaTool, map[string]any{
		"content": image.content, "imageType": image.imageType, "bizType": bizType,
	})
	if err != nil {
		return "", fmt.Errorf("上传本地图片到 %s 失败: %w", personalEmotionUploadMediaDisplay, err)
	}
	if bizType == "CHAT_IMAGE" {
		return parseChatImageUploadMediaID(text)
	}
	return parsePersonalEmotionUploadMediaID(text)
}

func parseChatImageUploadMediaID(text string) (string, error) {
	return parseImageUploadMediaID(text, true)
}
