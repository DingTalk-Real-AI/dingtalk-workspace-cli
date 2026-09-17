// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package localio

import (
	"fmt"
	"path/filepath"
)

const defaultPublishBytesLimit = 64 << 20

var verifyPublishTarget = func(target *downloadTarget) error { return target.verifyParent() }

// PublishBytesOptions controls safe no-clobber publication beneath BaseDir.
type PublishBytesOptions struct {
	BaseDir       string
	Output        string
	PreferredName string
	MaxBytes      int64
}

// PublishBytes writes an in-memory artifact through the same symlink-safe,
// fsync and atomic no-clobber path used by remote downloads.
func PublishBytes(payload []byte, opts PublishBytesOptions) (DownloadResult, error) {
	limit := opts.MaxBytes
	if limit <= 0 {
		limit = defaultPublishBytesLimit
	}
	if int64(len(payload)) > limit {
		return DownloadResult{}, fmt.Errorf("LOCAL_OUTPUT_TOO_LARGE: 内容大小 %d 超过上限 %d 字节", len(payload), limit)
	}
	target, err := openDownloadTarget(opts.BaseDir, opts.Output, "", opts.PreferredName)
	if err != nil {
		return DownloadResult{}, err
	}
	defer target.close()
	if err := verifyPublishTarget(target); err != nil {
		return DownloadResult{}, err
	}
	tmp, tmpName, err := createDownloadTemp(target.parentRoot)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("创建输出临时文件失败: %w", err)
	}
	cleanup := func() {
		_ = tmp.Close()
		_ = target.parentRoot.Remove(tmpName)
	}
	if _, err := tmp.Write(payload); err != nil {
		cleanup()
		return DownloadResult{}, fmt.Errorf("写入输出临时文件失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return DownloadResult{}, fmt.Errorf("同步输出临时文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return DownloadResult{}, fmt.Errorf("关闭输出临时文件失败: %w", err)
	}
	if err := verifyPublishTarget(target); err != nil {
		cleanup()
		return DownloadResult{}, err
	}
	if err := publishTempFile(target.parentRoot, tmpName, target.destinationName); err != nil {
		cleanup()
		return DownloadResult{}, err
	}
	return DownloadResult{AbsolutePath: target.absolutePath, RelativePath: filepath.ToSlash(target.relativePath), SizeBytes: int64(len(payload))}, nil
}
