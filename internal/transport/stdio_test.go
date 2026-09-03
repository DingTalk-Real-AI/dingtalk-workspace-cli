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

package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

func TestCrossPlatformCoverageStdioListToolsRecordsRawBytesAndPreservesSchemaNumbers(t *testing.T) {
	t.Parallel()

	resultJSON := `{"tools":[{"name":"numbers","inputSchema":{"const":9007199254740993},"outputSchema":{"const":1.2300}}],"metadata":{"ignored":true}}`
	responseJSON := `{"jsonrpc":"2.0","id":1,"result":` + resultJSON + `}` + "\n"
	client := &StdioClient{
		started: true,
		stdin:   &stdioTestWriteCloser{},
		stdout:  bufio.NewReader(strings.NewReader(responseJSON)),
	}

	result, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if result.RawResultBytes != len(resultJSON) {
		t.Fatalf("RawResultBytes = %d, want %d", result.RawResultBytes, len(resultJSON))
	}
	if result.RawResponseBytes != len(responseJSON) {
		t.Fatalf("RawResponseBytes = %d, want %d", result.RawResponseBytes, len(responseJSON))
	}
	if value, ok := result.Tools[0].InputSchema["const"].(json.Number); !ok || value.String() != "9007199254740993" {
		t.Fatalf("input schema const = %#v, want exact json.Number", result.Tools[0].InputSchema["const"])
	}
	if value, ok := result.Tools[0].OutputSchema["const"].(json.Number); !ok || value.String() != "1.2300" {
		t.Fatalf("output schema const = %#v, want exact json.Number", result.Tools[0].OutputSchema["const"])
	}
}

func TestCrossPlatformCoverageReadBoundedStdioLine(t *testing.T) {
	line, tooLarge, err := readBoundedStdioLine(bufio.NewReaderSize(strings.NewReader("1234\n"), 2), 5)
	if err != nil || tooLarge || string(line) != "1234\n" {
		t.Fatalf("exact limit = %q, %t, %v", line, tooLarge, err)
	}
	line, tooLarge, err = readBoundedStdioLine(bufio.NewReaderSize(strings.NewReader("12345\n"), 2), 5)
	if err != nil || !tooLarge || line != nil {
		t.Fatalf("over limit = %q, %t, %v", line, tooLarge, err)
	}
	fragmented := strings.Repeat("x", 100) + "\n"
	line, tooLarge, err = readBoundedStdioLine(bufio.NewReaderSize(strings.NewReader(fragmented), 16), len(fragmented))
	if err != nil || tooLarge || string(line) != fragmented {
		t.Fatalf("fragmented line = %q, %t, %v", line, tooLarge, err)
	}
	line, tooLarge, err = readBoundedStdioLine(bufio.NewReader(edgeStdioErrorReader{}), 5)
	if !errors.Is(err, errStdioRead) || tooLarge || line != nil {
		t.Fatalf("read error = %q, %t, %v", line, tooLarge, err)
	}
}

func TestCrossPlatformCoverageStdioRejectsDuplicateAndTypeInvalidEnvelopes(t *testing.T) {
	for _, response := range []string{
		`{"jsonrpc":"2.0","id":1,"result":null,"result":{}}` + "\n",
		`[]` + "\n",
	} {
		client := &StdioClient{
			started: true,
			stdin:   &stdioTestWriteCloser{},
			stdout:  bufio.NewReader(strings.NewReader(response)),
		}
		if err := client.call(context.Background(), "test", nil, nil); err == nil {
			t.Fatalf("invalid response %q succeeded", response)
		}
	}
}

func TestCrossPlatformCoverageStdioOversizeInvalidatesProcess(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$", "-test.count=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper process: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	stdin := &stdioTestWriteCloser{}
	client := &StdioClient{
		cmd:         cmd,
		stdin:       stdin,
		stdout:      bufio.NewReaderSize(strings.NewReader(strings.Repeat("x", config.MaxResponseBodySize+1)), 64*1024),
		started:     true,
		initialized: true,
		initResult:  InitializeResult{ProtocolVersion: "test"},
	}

	if err := client.call(context.Background(), "test", nil, nil); err == nil || !strings.Contains(err.Error(), "response exceeds safety limit") {
		t.Fatalf("oversize call error = %v", err)
	}
	if client.started || client.initialized || client.initResult.ProtocolVersion != "" || stdin.closeCalls != 1 {
		t.Fatalf("invalidated client = %#v, stdin closes = %d", client, stdin.closeCalls)
	}

	client.invalidateProcessLocked()
}

func TestCrossPlatformCoverageStdioCancellationInvalidatesProcess(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrossPlatformCoverageStdioBlockingHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "DWS_STDIO_BLOCKING_HELPER=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	client := &StdioClient{
		cmd:         cmd,
		stdin:       stdin,
		stdout:      bufio.NewReader(stdout),
		started:     true,
		initialized: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := client.call(ctx, "test", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled call error = %v", err)
	}
	if client.started || client.initialized {
		t.Fatalf("cancelled client was not invalidated: %#v", client)
	}
}

func TestCrossPlatformCoverageStdioRejectsMismatchedResponseID(t *testing.T) {
	stdin := &stdioTestWriteCloser{}
	client := &StdioClient{
		stdin:       stdin,
		stdout:      bufio.NewReader(strings.NewReader(`{"jsonrpc":"2.0","id":99,"result":null}` + "\n")),
		started:     true,
		initialized: true,
	}
	if err := client.call(context.Background(), "test", nil, nil); err == nil || !strings.Contains(err.Error(), "does not match request id") {
		t.Fatalf("mismatched response error = %v", err)
	}
	if client.started || client.initialized || stdin.closeCalls != 1 {
		t.Fatalf("mismatched response did not invalidate client: %#v", client)
	}
}

func TestCrossPlatformCoverageStdioBlockingHelper(t *testing.T) {
	if os.Getenv("DWS_STDIO_BLOCKING_HELPER") != "1" {
		return
	}
	time.Sleep(time.Hour)
}

type stdioTestWriteCloser struct {
	strings.Builder
	closeCalls int
}

func (w *stdioTestWriteCloser) Close() error {
	w.closeCalls++
	return nil
}

var errStdioRead = errors.New("stdio test read error")

type edgeStdioErrorReader struct{}

func (edgeStdioErrorReader) Read([]byte) (int, error) { return 0, errStdioRead }
