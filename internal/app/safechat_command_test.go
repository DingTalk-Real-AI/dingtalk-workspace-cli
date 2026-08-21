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

package app

import (
	"bytes"
	"strings"
	"testing"
)

func newSafeChatSelfTestForTest() (*bytes.Buffer, func(...string) error) {
	var out bytes.Buffer
	cmd := newSafeChatSelfTestCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	set := func(pairs ...string) error { return cmd.RunE(cmd, nil) }
	_ = set
	flags := cmd.Flags()
	runner := func(kv ...string) error {
		for i := 0; i+1 < len(kv); i += 2 {
			if err := flags.Set(kv[i], kv[i+1]); err != nil {
				return err
			}
		}
		return cmd.RunE(cmd, nil)
	}
	return &out, runner
}

func TestSafeChatSelfTestRequiresKeyServer(t *testing.T) {
	out, run := newSafeChatSelfTestForTest()
	// 显式清空才触发校验：默认值已在命令里锁定为现网 Safeding 地址。
	err := run("key-server", "")
	if err == nil {
		t.Fatal("selftest with an explicitly emptied --key-server should fail")
	}
	if !strings.Contains(err.Error(), "key-server") {
		t.Fatalf("error should name --key-server, got: %v", err)
	}
	if !strings.Contains(out.String(), "--key-server") {
		t.Fatalf("output should record the missing flag, got: %s", out.String())
	}
}

func TestSafeChatSelfTestReportsUnavailableBackend(t *testing.T) {
	out, run := newSafeChatSelfTestForTest()
	err := run("key-server", "https://key.example.test", "json", "true")
	if err == nil {
		t.Skip("safechat backend compiled in; unavailability path not reachable")
	}
	if !strings.Contains(err.Error(), "safechat") {
		t.Fatalf("error should explain the build tag, got: %v", err)
	}
	if !strings.Contains(out.String(), `"available":false`) {
		t.Fatalf("JSON output should carry available=false, got: %s", out.String())
	}
}

func newSafeChatDecryptForTest() (*bytes.Buffer, func(kv ...string) error, func(args ...string) error) {
	var out bytes.Buffer
	cmd := newSafeChatDecryptCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	flags := cmd.Flags()
	withFlags := func(kv ...string) error {
		for i := 0; i+1 < len(kv); i += 2 {
			if err := flags.Set(kv[i], kv[i+1]); err != nil {
				return err
			}
		}
		return cmd.RunE(cmd, nil)
	}
	withArgs := func(args ...string) error {
		return cmd.RunE(cmd, args)
	}
	return &out, withFlags, withArgs
}

func TestSafeChatDecryptRequiresInput(t *testing.T) {
	_, _, runArgs := newSafeChatDecryptForTest()
	err := runArgs()
	if err == nil || !strings.Contains(err.Error(), "缺少密文输入") {
		t.Fatalf("decrypt without input should fail with a clear message, got: %v", err)
	}
}

func TestSafeChatDecryptRejectsMultipleInputs(t *testing.T) {
	var out bytes.Buffer
	cmd := newSafeChatDecryptCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Flags().Set("text", "xxx"); err != nil {
		t.Fatal(err)
	}
	err := cmd.RunE(cmd, []string{"yyy"})
	if err == nil || !strings.Contains(err.Error(), "三选一") {
		t.Fatalf("decrypt with both --text and positional should fail, got: %v", err)
	}
}

func TestSafeChatDecryptReportsUnavailableBackend(t *testing.T) {
	out, run, _ := newSafeChatDecryptForTest()
	err := run("text", "somecipher", "json", "true")
	if err == nil {
		t.Skip("safechat backend compiled in; unavailability path not reachable")
	}
	if !strings.Contains(err.Error(), "safechat") {
		t.Fatalf("error should explain the build tag, got: %v", err)
	}
	if !strings.Contains(out.String(), `"available":false`) {
		t.Fatalf("JSON output should carry available=false, got: %s", out.String())
	}
}
