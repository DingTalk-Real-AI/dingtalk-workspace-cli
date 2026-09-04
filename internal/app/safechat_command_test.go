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
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto"
	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
	"github.com/spf13/cobra"
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
	if msgcrypto.Available() {
		t.Skip("safechat backend compiled in; unavailability path not reachable")
	}
	out, run := newSafeChatSelfTestForTest()
	err := run("key-server", "https://key.example.test", "json", "true")
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
	if msgcrypto.Available() {
		t.Skip("safechat backend compiled in; unavailability path not reachable")
	}
	out, run, _ := newSafeChatDecryptForTest()
	err := run("text", "somecipher", "json", "true")
	if !strings.Contains(err.Error(), "safechat") {
		t.Fatalf("error should explain the build tag, got: %v", err)
	}
	if !strings.Contains(out.String(), `"available":false`) {
		t.Fatalf("JSON output should carry available=false, got: %s", out.String())
	}
}

func TestCrossPlatformCoverageSafeChatStubAndMessageCryptoWiring(t *testing.T) {
	t.Run("selftest_requires_key_server", TestSafeChatSelfTestRequiresKeyServer)
	t.Run("selftest_unavailable", TestSafeChatSelfTestReportsUnavailableBackend)
	t.Run("decrypt_requires_input", TestSafeChatDecryptRequiresInput)
	t.Run("decrypt_rejects_multiple_inputs", TestSafeChatDecryptRejectsMultipleInputs)
	t.Run("decrypt_unavailable", TestSafeChatDecryptReportsUnavailableBackend)
	t.Run("safechat_command_excluded", func(t *testing.T) {
		if got := newSafeChatCommand(); got != nil {
			t.Fatalf("newSafeChatCommand() = %#v, want nil", got)
		}
		base := []*cobra.Command{{Use: "base"}}
		if got := appendOptionalCommand(base, nil); len(got) != 1 || got[0].Name() != "base" {
			t.Fatalf("append nil = %#v", got)
		}
		if got := appendOptionalCommand(base, &cobra.Command{Use: "extra"}); len(got) != 2 || got[1].Name() != "extra" {
			t.Fatalf("append command = %#v", got)
		}
		if cmd, _, err := NewRootCommand(context.Background()).Find([]string{"safechat"}); err == nil && cmd != nil && cmd.Name() == "safechat" {
			t.Fatalf("safechat command should be excluded from default root: %#v", cmd)
		}
	})
	t.Run("app_crypto_client_defaults", func(t *testing.T) {
		oldIdentity := appMessageCryptoCurrentIdentity
		oldOpen := appMessageCryptoOpenSession
		oldAvailable := appMessageCryptoAvailable
		t.Cleanup(func() {
			appMessageCryptoCurrentIdentity = oldIdentity
			appMessageCryptoOpenSession = oldOpen
			appMessageCryptoAvailable = oldAvailable
		})
		appMessageCryptoCurrentIdentity = func(context.Context, string) (msgcrypto.Identity, error) {
			return msgcrypto.Identity{CorpID: "corp-1", StaffID: "staff-1"}, nil
		}
		appMessageCryptoOpenSession = func(context.Context, msgcrypto.SessionOptions) (*msgcrypto.Session, error) {
			return &msgcrypto.Session{Cipher: appSafeChatFakeCipher{}, CorpID: "corp-1", StaffID: "staff-1"}, nil
		}
		appMessageCryptoAvailable = func() bool { return true }
		client := newAppMessageCryptoClient()
		if client == nil || client.PolicyCache == nil {
			t.Fatalf("client = %#v", client)
		}
		if !client.BackendReady() {
			t.Fatal("BackendReady() = false")
		}
		_, _ = client.Identity(context.Background(), t.TempDir())
		session, err := client.OpenSession(context.Background(), messagecrypto.SessionOptions{
			ConfigDir:           t.TempDir(),
			KeyServer:           " https://key.example.test ",
			AllowedRedirectHost: " redirect.example.test ",
		})
		if err != nil {
			t.Fatal(err)
		}
		if session.CorpID != "corp-1" || session.StaffID != "staff-1" || session.Cipher == nil {
			t.Fatalf("session = %#v", session)
		}
	})
	t.Run("app_crypto_client_open_error", func(t *testing.T) {
		oldOpen := appMessageCryptoOpenSession
		t.Cleanup(func() { appMessageCryptoOpenSession = oldOpen })
		appMessageCryptoOpenSession = func(context.Context, msgcrypto.SessionOptions) (*msgcrypto.Session, error) {
			return nil, errors.New("open failed")
		}
		client := newAppMessageCryptoClient()
		if _, err := client.OpenSession(context.Background(), messagecrypto.SessionOptions{}); err == nil || !strings.Contains(err.Error(), "open failed") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("first_non_empty", func(t *testing.T) {
		if got := firstNonEmptyAppCrypto("", "  cli-version  ", "fallback"); got != "cli-version" {
			t.Fatalf("firstNonEmptyAppCrypto() = %q", got)
		}
		if got := firstNonEmptyAppCrypto("", " "); got != "" {
			t.Fatalf("firstNonEmptyAppCrypto(empty) = %q", got)
		}
	})
	t.Run("emit_plain_error", func(t *testing.T) {
		var out bytes.Buffer
		cmd := newSafeChatDecryptCommand()
		cmd.SetOut(&out)
		err := emitUnavailableSafeChatError(cmd, false, "plain unavailable")
		if !errors.Is(err, errors.New("plain unavailable")) && !strings.Contains(err.Error(), "plain unavailable") {
			t.Fatalf("err = %v", err)
		}
		if !strings.Contains(out.String(), "plain unavailable") {
			t.Fatalf("output = %q", out.String())
		}
	})
	t.Run("validate_decrypt_file_source", func(t *testing.T) {
		if err := validateSafeChatDecryptInput(nil, "cipher.txt", ""); err != nil {
			t.Fatalf("file-only input should be accepted: %v", err)
		}
	})
}

type appSafeChatFakeCipher struct{}

func (appSafeChatFakeCipher) EncryptMessage(context.Context, string, string, []byte) ([]byte, error) {
	return nil, errors.New("not used")
}

func (appSafeChatFakeCipher) DecryptMessage(context.Context, string, string, []byte) ([]byte, error) {
	return nil, errors.New("not used")
}

func (appSafeChatFakeCipher) Close() error { return nil }
