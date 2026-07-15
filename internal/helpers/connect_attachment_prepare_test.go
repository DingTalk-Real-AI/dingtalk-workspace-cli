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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareOpenCodeAttachmentsUsesStoryboardAndTranscript(t *testing.T) {
	previous := generateConnectVideoStoryboard
	t.Cleanup(func() { generateConnectVideoStoryboard = previous })
	storyboard := filepath.Join(t.TempDir(), "storyboard.jpg")
	if err := os.WriteFile(storyboard, []byte("jpeg"), 0o600); err != nil {
		t.Fatal(err)
	}
	generateConnectVideoStoryboard = func(_ context.Context, path string) (string, error) {
		if path != "/tmp/original.mov" {
			t.Fatalf("video path = %q", path)
		}
		return storyboard, nil
	}

	prompt, attachments := prepareOpenCodeAttachments(context.Background(),
		"视频路径 /tmp/original.mov；语音路径 /tmp/original.ogg；转写：你是谁？",
		[]connectMediaAttachment{
			{LocalPath: "/tmp/picture.jpg", FileName: "picture.jpg", MediaType: "image"},
			{LocalPath: "/tmp/original.ogg", FileName: "voice.ogg", MediaType: "audio"},
			{LocalPath: "/tmp/original.mov", FileName: "demo.mov", MediaType: "video"},
			{LocalPath: "/tmp/report.md", FileName: "report.md", MediaType: "file"},
		},
	)
	if len(attachments) != 3 {
		t.Fatalf("attachments = %#v, want image + video storyboard + file (audio omitted)", attachments)
	}
	if attachments[0].LocalPath != "/tmp/picture.jpg" || attachments[1].LocalPath != storyboard || attachments[1].MediaType != "image" || attachments[2].LocalPath != "/tmp/report.md" {
		t.Fatalf("attachments = %#v", attachments)
	}
	if strings.Contains(prompt, "/tmp/original.mov") || strings.Contains(prompt, "/tmp/original.ogg") {
		t.Fatalf("prompt still points OpenCode at unsupported original binary: %q", prompt)
	}
	for _, want := range []string{storyboard, "12 帧", "钉钉转写", "你是谁？"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %q", want, prompt)
		}
	}
}

func TestPrepareOpenCodeAttachmentsDoesNotSubmitVideoWhenStoryboardFails(t *testing.T) {
	previous := generateConnectVideoStoryboard
	t.Cleanup(func() { generateConnectVideoStoryboard = previous })
	generateConnectVideoStoryboard = func(context.Context, string) (string, error) {
		return "", os.ErrNotExist
	}
	prompt, attachments := prepareOpenCodeAttachments(context.Background(), "请看视频", []connectMediaAttachment{
		{LocalPath: "/tmp/original.mov", FileName: "demo.mov", MediaType: "video"},
	})
	if len(attachments) != 0 || !strings.Contains(prompt, "未能读取视频画面") {
		t.Fatalf("prompt=%q attachments=%#v", prompt, attachments)
	}
}
