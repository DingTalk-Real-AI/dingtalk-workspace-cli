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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	connectVideoStoryboardFrames = 12
	connectVideoStoryboardWidth  = 480
	connectVideoStoryboardMax    = 16 << 20
)

var generateConnectVideoStoryboard = buildConnectVideoStoryboard

var (
	connectStoryboardLookPath = exec.LookPath
	connectStoryboardCommand  = exec.CommandContext
	connectStoryboardMkdirAll = os.MkdirAll
	connectStoryboardStat     = os.Stat
	connectStoryboardRemove   = os.Remove
)

// prepareOpenCodeAttachments adapts media that OpenCode's file-part bridge
// cannot safely send to the provider. OpenCode expands video files into the
// request in-process (a real 267 MiB merged-forward recording caused Bun to
// OOM), and the current bridge does not submit video/* as native multimodal
// video even when the selected model supports it. A full-duration, evenly
// sampled storyboard gives the visual model the actual sequence without
// sacrificing the separately downloaded original file.
//
// The selected iDEALab model has no audio input modality. DingTalk's message
// API normally supplies a speech-recognition transcript in the recovered
// prompt, so the opaque audio bytes are omitted from OpenCode's file parts
// instead of provoking a binary-file error.
func prepareOpenCodeAttachments(ctx context.Context, prompt string, attachments []connectMediaAttachment) (string, []connectMediaAttachment) {
	prepared := make([]connectMediaAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		mediaType := inboundMediaType(attachment.MediaType)
		switch mediaType {
		case "video":
			storyboard, err := generateConnectVideoStoryboard(ctx, attachment.LocalPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[connect][media] OpenCode 视频故事板生成失败，跳过不安全的原视频 file part: %v\n", err)
				prompt += "\n（原视频已完整下载，但当前 OpenCode 无法安全提交视频，且关键帧故事板生成失败；请明确告知用户本轮未能读取视频画面。）"
				continue
			}
			name := strings.TrimSuffix(strings.TrimSpace(attachment.FileName), filepath.Ext(attachment.FileName))
			if name == "" {
				name = "转发视频"
			}
			proxy := connectMediaAttachment{
				LocalPath: storyboard,
				FileName:  name + ".storyboard.jpg",
				MediaType: "image",
			}
			prompt = strings.ReplaceAll(prompt, attachment.LocalPath, storyboard)
			prompt += "\n（原视频已完整下载；为避免 OpenCode 展开大视频导致内存溢出，已按完整时长均匀抽取 12 帧并生成故事板图片。请按从左到右、从上到下的时间顺序分析画面。）"
			prepared = append(prepared, proxy)
			if info, err := os.Stat(attachment.LocalPath); err == nil {
				fmt.Fprintf(os.Stderr, "[connect][media] OpenCode 视频故事板已生成: 原始=%d 字节 故事板=%s\n", info.Size(), storyboard)
			}
		case "audio":
			prompt = strings.ReplaceAll(prompt, attachment.LocalPath, "[语音原文件已完整下载，当前模型使用钉钉转写]")
			prompt += "\n（语音原文件已完整下载；当前 OpenCode 模型不接收音频 file part，请优先依据上述钉钉语音转写处理。）"
		default:
			prepared = append(prepared, attachment)
		}
	}
	return prompt, prepared
}

func prepareConnectForwarderAttachments(fwd forwarder, prompt string, attachments []connectMediaAttachment) (string, []connectMediaAttachment) {
	if _, isOpenCode := fwd.(*opencodeForwarder); !isOpenCode {
		return prompt, attachments
	}
	ctx, cancel := context.WithTimeout(context.Background(), mediaDownloadTimeout)
	defer cancel()
	return prepareOpenCodeAttachments(ctx, prompt, attachments)
}

func buildConnectVideoStoryboard(ctx context.Context, videoPath string) (string, error) {
	ffmpegPath, err := connectStoryboardLookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("未安装 ffmpeg")
	}
	ffprobePath, err := connectStoryboardLookPath("ffprobe")
	if err != nil {
		return "", fmt.Errorf("未安装 ffprobe")
	}

	probe := connectStoryboardCommand(ctx, ffprobePath,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)
	rawDuration, err := probe.Output()
	if err != nil {
		return "", fmt.Errorf("ffprobe 读取视频时长失败: %w", err)
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(rawDuration)), 64)
	if err != nil || duration <= 0 {
		return "", fmt.Errorf("ffprobe 返回无效视频时长 %q", strings.TrimSpace(string(rawDuration)))
	}
	interval := duration / connectVideoStoryboardFrames
	if interval < 0.5 {
		interval = 0.5
	}

	dir := filepath.Join(os.TempDir(), "dws-connect-media")
	if err := connectStoryboardMkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, uuid.NewString()+".storyboard.jpg")
	filter := fmt.Sprintf("fps=1/%.6f,scale=%d:-2,tile=4x3:padding=4:margin=4", interval, connectVideoStoryboardWidth)
	cmd := connectStoryboardCommand(ctx, ffmpegPath,
		"-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-i", videoPath,
		"-vf", filter,
		"-frames:v", "1",
		"-q:v", "3",
		dest,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = connectStoryboardRemove(dest)
		return "", fmt.Errorf("ffmpeg 生成视频故事板失败: %w (%s)", err, truncateRunes(strings.TrimSpace(string(output)), 300))
	}
	info, err := connectStoryboardStat(dest)
	if err != nil {
		return "", fmt.Errorf("视频故事板未生成: %w", err)
	}
	if info.Size() <= 0 || info.Size() > connectVideoStoryboardMax {
		_ = connectStoryboardRemove(dest)
		return "", fmt.Errorf("视频故事板大小异常: %d 字节", info.Size())
	}
	return dest, nil
}
