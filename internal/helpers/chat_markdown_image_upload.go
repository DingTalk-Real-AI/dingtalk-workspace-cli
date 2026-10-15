package helpers

import (
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"sync"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func prepareMarkdownImages(refs []markdownImageRef) ([]*personalEmotionImage, map[string]int, error) {
	images := make([]*personalEmotionImage, 0)
	paths := make(map[string]int)
	digests := make(map[[sha256.Size]byte]int)
	for _, ref := range refs {
		if ref.path == "" {
			continue
		}
		if _, exists := paths[ref.path]; exists {
			continue
		}
		image, err := loadChatUploadImageFile(ref.path)
		if err != nil {
			return nil, nil, markdownImageReadError(err)
		}
		digest := sha256.Sum256([]byte(image.content))
		if existing, ok := digests[digest]; ok {
			paths[ref.path] = existing
			continue
		}
		paths[ref.path] = len(images)
		digests[digest] = len(images)
		images = append(images, image)
	}
	return images, paths, nil
}

// Use fixed diagnostics so nested filesystem errors cannot expose source data.
func markdownImageReadError(err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return errors.New("无法读取本地图片：文件不存在。请根据 DWS 进程工作目录核对附件的相对路径")
	case errors.Is(err, fs.ErrPermission):
		return errors.New("无法读取本地图片：无读取权限。请确认 DWS 进程具有附件读取权限")
	default:
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			return errors.New("无法读取本地图片。请检查文件状态与 DWS 进程的访问权限")
		}
		return errors.New("本地图片校验失败。请检查文件类型、图片格式、大小和尺寸是否符合要求")
	}
}

// Keep machine-readable failure classification without copying untrusted diagnostics.
func markdownImageUploadError(err error) error {
	result := &apperrors.Error{Category: apperrors.CategoryAPI, Message: "图片上传失败，消息未发送。请检查上传权限、参数或服务状态", Reason: "image_upload_failed"}
	var source *apperrors.Error
	if !errors.As(err, &source) {
		return result
	}
	switch source.Category {
	case apperrors.CategoryAPI, apperrors.CategoryAuth, apperrors.CategoryValidation, apperrors.CategoryDiscovery, apperrors.CategoryInternal:
		result.Category = source.Category
	}
	switch source.Reason {
	case "business_error", "mcp_tool_error", "not_authenticated", "permission_denied", "rate_limited", "timeout":
		result.Reason = source.Reason
	}
	switch source.ServerDiag.ServerErrorCode {
	case "PARAM_ERROR", "INVALID_PARAM", "PERMISSION_DENIED", "NO_PERMISSION", "RATE_LIMIT_EXCEEDED", "TIMEOUT_ERROR", "INTERNAL_ERROR":
		result.ServerDiag.ServerErrorCode = source.ServerDiag.ServerErrorCode
	}
	result.Retryable = source.Retryable
	result.RetryableSet = source.RetryableSet
	result.RPCCode = source.RPCCode
	return result
}

func uploadMarkdownImages(ctx context.Context, images []*personalEmotionImage, upload func(context.Context, *personalEmotionImage) (string, error)) ([]string, error) {
	if upload == nil {
		upload = func(ctx context.Context, image *personalEmotionImage) (string, error) {
			return uploadChatImage(ctx, image, "CHAT_IMAGE")
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ids := make([]string, len(images))
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	var firstErr error
	var mu sync.Mutex
	for index, image := range images {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			id, err := upload(ctx, image)
			if err == nil && strings.TrimSpace(id) == "" {
				err = fmt.Errorf("图片上传未返回 mediaId")
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
				cancel()
			}
			ids[index] = id
		})
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func replaceMarkdownImageRefs(body string, refs []markdownImageRef) string {
	slices.SortFunc(refs, func(a, b markdownImageRef) int { return cmp.Compare(a.start, b.start) })
	var out strings.Builder
	last := 0
	for _, ref := range refs {
		out.WriteString(body[last:ref.start])
		out.WriteString(ref.mediaID)
		last = ref.end
	}
	out.WriteString(body[last:])
	return out.String()
}
