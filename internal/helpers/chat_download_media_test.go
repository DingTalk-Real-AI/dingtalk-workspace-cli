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
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChatDownloadMediaJSONPreservesLegacyResult(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	previousHTTPGetFile := httpGetFile
	os.Args = []string{"dws", "chat"}
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
		httpGetFile = previousHTTPGetFile
	})

	const downloadURL = "https://download.example.test/photo.jpg?token=one&part=two"
	caller := &scriptedToolCaller{
		format: "json",
		steps: []scriptedToolStep{{
			text: `{"resourceUrl":"` + downloadURL + `"}`,
		}},
	}
	InitDeps(caller)
	var stdout bytes.Buffer
	deps.Out = &Formatter{w: &stdout, errW: io.Discard}

	outputDir := t.TempDir()
	wantOutput := filepath.Join(outputDir, "photo.jpg")
	httpGetFile = func(_ context.Context, gotURL string, _ map[string]string, gotOutput string) error {
		if gotURL != downloadURL {
			t.Fatalf("download URL = %q, want %q", gotURL, downloadURL)
		}
		if gotOutput != wantOutput {
			t.Fatalf("download output = %q, want %q", gotOutput, wantOutput)
		}
		return nil
	}

	root := newChatCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"message", "download-media",
		"--type=mediaId",
		"--resource-id=resource",
		"--message-id=message",
		"--open-conversation-id=conversation",
		"--output=" + outputDir,
	})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	var got struct {
		Success     bool   `json:"success"`
		DownloadURL string `json:"downloadUrl"`
		Output      string `json:"output"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if !got.Success || got.DownloadURL != downloadURL || got.Output != wantOutput {
		t.Fatalf("result = %#v, want success=true downloadUrl=%q output=%q", got, downloadURL, wantOutput)
	}
	if strings.Contains(stdout.String(), "[INFO]") {
		t.Fatalf("JSON stdout contains progress text: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "?token=one&part=two") {
		t.Fatalf("downloadUrl was escaped or changed: %s", stdout.String())
	}
}
