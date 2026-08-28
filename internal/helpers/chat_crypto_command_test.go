// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageChatCryptoCommand(t *testing.T) {
	old := chatCryptoClient
	t.Cleanup(func() { chatCryptoClient = old })

	t.Run("set_nil_client_restores_default", func(t *testing.T) {
		SetChatCryptoClient(nil)
		if chatCryptoClient == nil || chatCryptoClient.PolicyCache == nil {
			t.Fatalf("chatCryptoClient = %#v", chatCryptoClient)
		}
	})
	t.Run("decrypt_command_safechat_layer", func(t *testing.T) {
		SetChatCryptoClient(&messagecrypto.Client{
			OpenSession: func(context.Context, messagecrypto.SessionOptions) (*messagecrypto.Session, error) {
				return &messagecrypto.Session{Cipher: imReadFakeCipher{}, CorpID: "corp-1", StaffID: "staff-1"}, nil
			},
			BackendReady: func() bool { return true },
			PolicyCache:  messagecrypto.NewPolicyCache(nil),
		})
		cmd := newChatCryptoDecryptCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--text", "safe-cipher", "--layer", "safechat"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "ding:safe-cipher") {
			t.Fatalf("output = %q", out.String())
		}
	})
	t.Run("dry_run_runtime_blocks_write", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().Bool("dry-run", true, "")
		_, err := chatCryptoRuntime{cmd: cmd}.CallMCPWriteDataStrict("im", "batch_ding_decrypt_messages", map[string]any{})
		if err == nil || !strings.Contains(err.Error(), "--dry-run") {
			t.Fatalf("err = %v", err)
		}
		if !(chatCryptoRuntime{cmd: cmd}).DryRun() {
			t.Fatal("DryRun() = false, want true")
		}
	})
	t.Run("runtime_rejects_empty_write_response", func(t *testing.T) {
		previousDeps := deps
		t.Cleanup(func() { deps = previousDeps })
		InitDeps(&imReadResultCaller{responses: map[string]string{"batch_ding_decrypt_messages": " \n "}})
		cmd := &cobra.Command{}
		cmd.Flags().Bool("dry-run", false, "")
		_, err := chatCryptoRuntime{cmd: cmd}.CallMCPWriteDataStrict("im", "batch_ding_decrypt_messages", map[string]any{})
		if err == nil || !strings.Contains(err.Error(), "no business result") {
			t.Fatalf("empty write response err = %v", err)
		}
	})
	t.Run("parse_mcp_data", func(t *testing.T) {
		if _, err := parseChatCryptoMCPData("tool", "", errors.New("boom")); err == nil {
			t.Fatal("expected passthrough error")
		}
		if _, err := parseChatCryptoMCPData("tool", "{", nil); err == nil {
			t.Fatal("expected invalid json error")
		}
		got, err := parseChatCryptoMCPData("tool", `{"ok":true}`, nil)
		if err != nil || got["ok"] != true {
			t.Fatalf("parse = %#v, %v", got, err)
		}
	})
	t.Run("read_input_sources", func(t *testing.T) {
		cmd := newChatCryptoDecryptCommand()
		if _, err := readChatCryptoInput(cmd); err == nil {
			t.Fatal("expected missing input error")
		}
		if err := cmd.Flags().Set("text", "text"); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Flags().Set("file", "-"); err != nil {
			t.Fatal(err)
		}
		if _, err := readChatCryptoInput(cmd); err == nil {
			t.Fatal("expected multiple input error")
		}

		cmd = newChatCryptoDecryptCommand()
		dir := t.TempDir()
		oldWD, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chdir(oldWD) })
		path := filepath.Join(dir, "cipher.txt")
		if err := os.WriteFile(path, []byte(" file-cipher \n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Flags().Set("file", "./cipher.txt"); err != nil {
			t.Fatal(err)
		}
		got, err := readChatCryptoInput(cmd)
		if err != nil || string(got) != "file-cipher" {
			t.Fatalf("file input = %q, %v", got, err)
		}

		cmd = newChatCryptoDecryptCommand()
		emptyPath := filepath.Join(dir, "empty.txt")
		if err := os.WriteFile(emptyPath, []byte(" \n "), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Flags().Set("file", "./empty.txt"); err != nil {
			t.Fatal(err)
		}
		if _, err := readChatCryptoInput(cmd); err == nil || !strings.Contains(err.Error(), "输入内容为空") {
			t.Fatalf("empty file input err = %v", err)
		}

		cmd = newChatCryptoDecryptCommand()
		cmd.SetIn(strings.NewReader(" stdin-cipher \n"))
		if err := cmd.Flags().Set("file", "-"); err != nil {
			t.Fatal(err)
		}
		got, err = readChatCryptoInput(cmd)
		if err != nil || string(got) != "stdin-cipher" {
			t.Fatalf("stdin input = %q, %v", got, err)
		}
	})
}
