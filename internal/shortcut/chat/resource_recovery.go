package chat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type resourceURLResolver func() (string, map[string]string, error)

func downloadWithRecovery(ctx context.Context, client *http.Client, initialURL string, initialHeaders map[string]string, dest string, overwrite bool, partSize int64, retries, delayMS int, resolve resourceURLResolver) (int64, error) {
	if partSize == 0 {
		return resourceDownload(ctx, client, initialURL, initialHeaders, dest, overwrite)
	}
	temp, err := os.CreateTemp(filepath.Dir(dest), "."+filepath.Base(dest)+".range-*")
	if err != nil {
		return 0, err
	}
	defer func() { temp.Close(); os.Remove(temp.Name()) }()
	currentURL, headers := initialURL, initialHeaders
	var offset, total int64
	total = -1
	etag := ""
	for total < 0 || offset < total {
		var chunk []byte
		completePart := false
		for attempt := 0; attempt <= retries; attempt++ {
			if attempt > 0 {
				timer := time.NewTimer(time.Duration(delayMS) * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return 0, ctx.Err()
				case <-timer.C:
				}
				currentURL, headers, err = resolve()
				if err != nil {
					return 0, err
				}
			}
			safe, u, e := scopedResourceHTTPClient(client, currentURL, headers)
			if e != nil {
				return 0, e
			}
			req, e := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			if e != nil {
				return 0, e
			}
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+partSize-1))
			req.Header.Set("Accept-Encoding", "identity")
			if etag != "" {
				req.Header.Set("If-Range", etag)
			}
			resp, e := safe.Do(req)
			if e != nil {
				err = e
				if ctx.Err() != nil {
					return 0, ctx.Err()
				}
				continue
			}
			// A full response after If-Range must restart the entire atomic download; never append it to partial bytes.
			if resp.StatusCode == http.StatusOK || (offset == 0 && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable) {
				resp.Body.Close()
				return downloadResourceAtomically(ctx, client, currentURL, headers, dest, overwrite)
			}
			if resp.StatusCode == 403 || resp.StatusCode == 429 || resp.StatusCode >= 500 {
				resp.Body.Close()
				err = apperrors.NewAPI(fmt.Sprintf("资源分段HTTP %d", resp.StatusCode))
				continue
			}
			if resp.StatusCode != http.StatusPartialContent {
				resp.Body.Close()
				return 0, apperrors.NewAPI(fmt.Sprintf("资源分段返回HTTP %d", resp.StatusCode))
			}
			var start, end, size int64
			_, e = fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes %d-%d/%d", &start, &end, &size)
			version := resp.Header.Get("ETag")
			if offset == 0 && (version == "" || strings.HasPrefix(version, "W/")) {
				resp.Body.Close()
				return downloadResourceAtomically(ctx, client, currentURL, headers, dest, overwrite)
			}
			if size > partSize*10000 {
				resp.Body.Close()
				return 0, apperrors.NewValidation("分段数量超过10000，请增大--part-size")
			}
			if e != nil || resp.Header.Get("Content-Range") != fmt.Sprintf("bytes %d-%d/%d", start, end, size) || start != offset || end < start || end-start+1 > partSize || size <= end || (total >= 0 && total != size) || (etag != "" && etag != version) {
				resp.Body.Close()
				return 0, apperrors.NewAPI("资源版本、总长或字节范围变化，停止以防混拼")
			}
			data, e := io.ReadAll(io.LimitReader(resp.Body, partSize+1))
			resp.Body.Close()
			if e != nil || int64(len(data)) != end-start+1 {
				err = apperrors.NewAPI("资源片段截断或长度不匹配")
				continue
			}
			if resp.ContentLength >= 0 && resp.ContentLength != int64(len(data)) {
				return 0, apperrors.NewAPI("资源片段Content-Length不一致")
			}
			if etag == "" {
				etag = version
				total = size
			}
			chunk = data
			completePart = true
			break
		}
		if !completePart {
			if err == nil {
				err = apperrors.NewAPI("资源片段未完成")
			}
			return 0, err
		}
		if _, err = resourceCopy(temp, bytes.NewReader(chunk)); err != nil {
			return 0, err
		}
		offset += int64(len(chunk))
	}
	if err = resourceTempSync(temp); err != nil {
		return 0, err
	}
	if err = resourceTempClose(temp); err != nil {
		return 0, err
	}
	if overwrite {
		err = resourceRename(temp.Name(), dest)
	} else {
		err = resourceLink(temp.Name(), dest)
	}
	if errors.Is(err, os.ErrExist) {
		return 0, apperrors.NewValidation("目标已存在；需要覆盖时显式--overwrite")
	}
	if err != nil {
		return 0, err
	}
	return offset, nil
}
