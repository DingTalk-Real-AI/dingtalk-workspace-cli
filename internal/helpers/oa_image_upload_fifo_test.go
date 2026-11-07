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

//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package helpers

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// TestCrossPlatformCoverageOAImageUploadRejectsFIFOReplacedAfterStat 覆盖
// TOCTOU 的 open 阻塞场景：预检 Stat 看到普通 png（seam 注入），路径在 Stat
// 与 Open 之间被替换为 FIFO；真实 open（不经 seam）必须不阻塞在 FIFO 读端
// （O_NONBLOCK），随后由打开后 descriptor fstat 复核拒绝。若 O_NONBLOCK
// 防护缺失，open 将阻塞等待写者并触发下方超时失败，而非挂死整个测试进程。
func TestCrossPlatformCoverageOAImageUploadRejectsFIFOReplacedAfterStat(t *testing.T) {
	regularPath := mustWriteOAImageTempFile(t, "regular.png", oaImagePNGHeader+"png-body")
	info, err := os.Stat(regularPath)
	if err != nil {
		t.Fatalf("stat regular png: %v", err)
	}
	testseam.Swap(t, &imageUploadOSStat, func(string) (os.FileInfo, error) { return info, nil })

	fifoPath := filepath.Join(t.TempDir(), "evidence.png")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
		{text: `{"success":true,"mediaIdV2":"$v2$"}`},
	}}
	done := make(chan error, 1)
	go func() {
		_, err := executeOAAttachmentCommandCapturingOutput(t, caller,
			"approval", "image", "upload",
			"--file", fifoPath,
		)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "非普通文件") {
			t.Fatalf("error = %v, want 非普通文件", err)
		}
		if caller.calls != 0 {
			t.Fatalf("MCP calls = %d, want 0 (FIFO descriptor must reject before upload)", caller.calls)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("open 阻塞在 FIFO 读端：Stat 预检后路径被替换为 FIFO 的 O_NONBLOCK 防护缺失")
	}
}
