// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto"
	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
)

func TestSafeChatBackendIsInternalToMessageCrypto(t *testing.T) {
	root := NewRootCommand(context.Background())
	for _, command := range root.Commands() {
		if command.Name() == "safechat" {
			t.Fatal("SafeChat backend must not be exposed as a top-level command")
		}
	}
	command, _, err := root.Find([]string{"chat", "crypto", "decrypt"})
	if err != nil || command == nil || command.CommandPath() != "dws chat crypto decrypt" {
		t.Fatalf("message crypto command = %#v, %v", command, err)
	}
}

func TestCrossPlatformCoverageMessageCryptoWiring(t *testing.T) {
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
}

type appSafeChatFakeCipher struct{}

func (appSafeChatFakeCipher) EncryptMessage(context.Context, string, string, []byte) ([]byte, error) {
	return nil, errors.New("not used")
}

func (appSafeChatFakeCipher) DecryptMessage(context.Context, string, string, []byte) ([]byte, error) {
	return nil, errors.New("not used")
}

func (appSafeChatFakeCipher) Close() error { return nil }
