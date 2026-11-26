// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package msgcrypto

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

type fakeTokenSnapshotProvider struct {
	snapshot *auth.TokenData
	err      error
}

func (f fakeTokenSnapshotProvider) GetTokenSnapshot(context.Context) (*auth.TokenData, error) {
	return f.snapshot, f.err
}

func TestCrossPlatformCoverageMessageCryptoSession(t *testing.T) {
	t.Run("close_nil_session", func(t *testing.T) {
		var session *Session
		if err := session.Close(); err != nil {
			t.Fatalf("Close() = %v", err)
		}
	})
	t.Run("close_nil_cipher", func(t *testing.T) {
		if err := (&Session{}).Close(); err != nil {
			t.Fatalf("Close() = %v", err)
		}
	})
	t.Run("close_cipher", func(t *testing.T) {
		cipher := &fakeCipher{}
		if err := (&Session{Cipher: cipher}).Close(); err != nil {
			t.Fatalf("Close() = %v", err)
		}
		if cipher.closeCount != 1 {
			t.Fatalf("closeCount = %d, want 1", cipher.closeCount)
		}
	})
	t.Run("config_dir_default", func(t *testing.T) {
		t.Setenv("DWS_CONFIG_DIR", t.TempDir())
		if got := configDirOrDefault(" "); !strings.Contains(got, "dws") && got != strings.TrimSpace(got) {
			t.Fatalf("configDirOrDefault(empty) = %q", got)
		}
		if got := configDirOrDefault(" /custom/dws "); got != "/custom/dws" {
			t.Fatalf("configDirOrDefault(explicit) = %q", got)
		}
	})
	t.Run("current_identity_without_login", func(t *testing.T) {
		identity, err := CurrentIdentity(context.Background(), t.TempDir())
		if err == nil {
			if strings.TrimSpace(identity.CorpID) == "" || strings.TrimSpace(identity.StaffID) == "" {
				t.Fatalf("CurrentIdentity() = %#v, want complete identity", identity)
			}
			return
		}
		if !strings.Contains(err.Error(), "读取登录态失败") && !errors.Is(err, ErrNoCorpID) {
			t.Fatalf("CurrentIdentity() = %v", err)
		}
	})
	t.Run("current_identity_default_config_dir", func(t *testing.T) {
		t.Setenv("DWS_CONFIG_DIR", t.TempDir())
		identity, err := CurrentIdentity(context.Background(), " ")
		if err == nil {
			if strings.TrimSpace(identity.CorpID) == "" || strings.TrimSpace(identity.StaffID) == "" {
				t.Fatalf("CurrentIdentity() = %#v, want complete identity", identity)
			}
			return
		}
		if !strings.Contains(err.Error(), "读取登录态失败") && !errors.Is(err, ErrNoCorpID) {
			t.Fatalf("CurrentIdentity() = %v", err)
		}
	})
	t.Run("current_identity_load_error", func(t *testing.T) {
		oldProvider := sessionNewOAuthProvider
		t.Cleanup(func() { sessionNewOAuthProvider = oldProvider })
		sessionNewOAuthProvider = func(string) tokenSnapshotProvider {
			return fakeTokenSnapshotProvider{err: errors.New("load failed")}
		}
		if _, err := CurrentIdentity(context.Background(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "读取登录态失败") {
			t.Fatalf("CurrentIdentity(load error) = %v", err)
		}
	})
	t.Run("current_identity_empty_corp_and_staff_fallback", func(t *testing.T) {
		oldProvider := sessionNewOAuthProvider
		t.Cleanup(func() { sessionNewOAuthProvider = oldProvider })
		sessionNewOAuthProvider = func(string) tokenSnapshotProvider {
			return fakeTokenSnapshotProvider{snapshot: &auth.TokenData{CorpID: "", UserID: "staff-1"}}
		}
		if _, err := CurrentIdentity(context.Background(), t.TempDir()); !errors.Is(err, ErrNoCorpID) {
			t.Fatalf("CurrentIdentity(empty corp) = %v", err)
		}

		sessionNewOAuthProvider = func(string) tokenSnapshotProvider {
			return fakeTokenSnapshotProvider{snapshot: &auth.TokenData{CorpID: "corp-1", UserID: ""}}
		}
		identity, err := CurrentIdentity(context.Background(), t.TempDir())
		if err != nil {
			t.Fatalf("CurrentIdentity(empty user) = %v", err)
		}
		if identity.CorpID != "corp-1" || identity.StaffID != "dws-safechat" {
			t.Fatalf("identity = %#v", identity)
		}
	})
	t.Run("open_session_unavailable_without_backend", func(t *testing.T) {
		if Available() {
			t.Skip("safechat backend is compiled in")
		}
		_, err := OpenSession(context.Background(), SessionOptions{ConfigDir: t.TempDir()})
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("OpenSession() = %v, want ErrUnavailable", err)
		}
	})
	t.Run("open_session_success_defaults", func(t *testing.T) {
		oldAvailable := sessionAvailable
		oldIdentity := sessionCurrentIdentity
		oldOpen := sessionOpenBackend
		t.Cleanup(func() {
			sessionAvailable = oldAvailable
			sessionCurrentIdentity = oldIdentity
			sessionOpenBackend = oldOpen
		})
		sessionAvailable = func() bool { return true }
		sessionCurrentIdentity = func(context.Context, string) (Identity, error) {
			return Identity{CorpID: "corp-1", StaffID: "staff-1"}, nil
		}
		var gotConfig Config
		sessionOpenBackend = func(_ context.Context, cfg Config) (Cipher, error) {
			gotConfig = cfg
			return &fakeCipher{}, nil
		}
		session, err := OpenSession(context.Background(), SessionOptions{
			ConfigDir:   " /tmp/dws-config ",
			CLIVersion:  "1.2.3",
			KeystoreDir: " /tmp/keys ",
		})
		if err != nil {
			t.Fatal(err)
		}
		if session.CorpID != "corp-1" || session.StaffID != "staff-1" || session.KeystoreDir != "/tmp/keys" {
			t.Fatalf("session = %#v", session)
		}
		if gotConfig.KeyServer != DefaultSafeChatKeyServer || gotConfig.AllowedRedirectHost != DefaultSafeChatRedirectHost ||
			gotConfig.KeystoreDir != "/tmp/keys" || gotConfig.AuthCode == nil {
			t.Fatalf("config = %#v", gotConfig)
		}
	})
	t.Run("open_session_identity_error", func(t *testing.T) {
		oldAvailable := sessionAvailable
		oldIdentity := sessionCurrentIdentity
		t.Cleanup(func() {
			sessionAvailable = oldAvailable
			sessionCurrentIdentity = oldIdentity
		})
		sessionAvailable = func() bool { return true }
		sessionCurrentIdentity = func(context.Context, string) (Identity, error) {
			return Identity{}, ErrNoCorpID
		}
		if _, err := OpenSession(context.Background(), SessionOptions{}); !errors.Is(err, ErrNoCorpID) {
			t.Fatalf("OpenSession() = %v, want ErrNoCorpID", err)
		}
	})
	t.Run("open_session_backend_errors", func(t *testing.T) {
		oldAvailable := sessionAvailable
		oldIdentity := sessionCurrentIdentity
		oldOpen := sessionOpenBackend
		t.Cleanup(func() {
			sessionAvailable = oldAvailable
			sessionCurrentIdentity = oldIdentity
			sessionOpenBackend = oldOpen
		})
		sessionAvailable = func() bool { return true }
		sessionCurrentIdentity = func(context.Context, string) (Identity, error) {
			return Identity{CorpID: "corp-1", StaffID: "staff-1"}, nil
		}
		sessionOpenBackend = func(context.Context, Config) (Cipher, error) {
			return nil, ErrUnavailable
		}
		if _, err := OpenSession(context.Background(), SessionOptions{}); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("OpenSession unavailable = %v", err)
		}
		sessionOpenBackend = func(context.Context, Config) (Cipher, error) {
			return nil, errors.New("backend failed")
		}
		if _, err := OpenSession(context.Background(), SessionOptions{}); err == nil || !strings.Contains(err.Error(), "初始化加解密后端失败") {
			t.Fatalf("OpenSession wrapped error = %v", err)
		}
	})
	t.Run("open_session_stateless_token_uses_ephemeral_keystore_and_cleans_up", func(t *testing.T) {
		oldAvailable := sessionAvailable
		oldIdentity := sessionCurrentIdentity
		oldOpen := sessionOpenBackend
		t.Cleanup(func() {
			sessionAvailable = oldAvailable
			sessionCurrentIdentity = oldIdentity
			sessionOpenBackend = oldOpen
		})
		sessionAvailable = func() bool { return true }
		sessionCurrentIdentity = func(context.Context, string) (Identity, error) {
			t.Fatal("stateless OpenSession must not read local OAuth identity")
			return Identity{}, nil
		}

		// Missing corpID fails with ErrNoCorpID without touching OAuth.
		if _, err := OpenSession(context.Background(), SessionOptions{
			AccessToken: "stateless-token",
		}); !errors.Is(err, ErrNoCorpID) {
			t.Fatalf("OpenSession(no corpID) = %v, want ErrNoCorpID", err)
		}

		var gotConfig Config
		cipher := &fakeCipher{}
		sessionOpenBackend = func(_ context.Context, cfg Config) (Cipher, error) {
			gotConfig = cfg
			return cipher, nil
		}
		session, err := OpenSession(context.Background(), SessionOptions{
			AccessToken: "stateless-token",
			ClientID:    "custom-client",
			CorpID:      "corp-stateless",
			CLIVersion:  "1.2.3",
		})
		if err != nil {
			t.Fatalf("OpenSession(stateless) = %v", err)
		}
		if session.CorpID != "corp-stateless" || session.StaffID != "dws-safechat" {
			t.Fatalf("session identity = %#v", session)
		}
		portal, ok := gotConfig.AuthCode.(*PortalAuthCode)
		if !ok || portal.AccessToken != "stateless-token" || portal.ClientID != "custom-client" || portal.snapshot != nil || portal.refresh != nil {
			t.Fatalf("authCode provider = %#v", gotConfig.AuthCode)
		}
		if session.KeystoreDir == "" {
			t.Fatal("expected ephemeral keystore dir")
		}
		if _, err := os.Stat(session.KeystoreDir); err != nil {
			t.Fatalf("ephemeral keystore dir should exist before Close: %v", err)
		}
		if err := session.Close(); err != nil {
			t.Fatalf("Close() = %v", err)
		}
		if cipher.closeCount != 1 {
			t.Fatalf("closeCount = %d, want 1", cipher.closeCount)
		}
		if _, err := os.Stat(session.KeystoreDir); !os.IsNotExist(err) {
			t.Fatalf("ephemeral keystore dir should be removed on Close, stat err = %v", err)
		}
	})
	t.Run("open_session_stateless_cleans_up_ephemeral_dir_on_backend_error", func(t *testing.T) {
		oldAvailable := sessionAvailable
		oldOpen := sessionOpenBackend
		oldMkdirTemp := sessionMkdirTemp
		t.Cleanup(func() {
			sessionAvailable = oldAvailable
			sessionOpenBackend = oldOpen
			sessionMkdirTemp = oldMkdirTemp
		})
		sessionAvailable = func() bool { return true }
		var createdDir string
		sessionMkdirTemp = func(dir, pattern string) (string, error) {
			d, err := os.MkdirTemp(t.TempDir(), pattern)
			createdDir = d
			return d, err
		}
		sessionOpenBackend = func(context.Context, Config) (Cipher, error) {
			return nil, errors.New("boom")
		}
		if _, err := OpenSession(context.Background(), SessionOptions{
			AccessToken: "stateless-token",
			CorpID:      "corp-stateless",
		}); err == nil {
			t.Fatal("expected error")
		}
		if createdDir == "" {
			t.Fatal("expected temp dir to be created")
		}
		if _, err := os.Stat(createdDir); !os.IsNotExist(err) {
			t.Fatalf("ephemeral dir should be cleaned on backend failure, stat err = %v", err)
		}

		// MkdirTemp error propagates cleanly.
		sessionMkdirTemp = func(string, string) (string, error) {
			return "", errors.New("no temp space")
		}
		if _, err := OpenSession(context.Background(), SessionOptions{
			AccessToken: "stateless-token",
			CorpID:      "corp-stateless",
		}); err == nil || !strings.Contains(err.Error(), "创建临时密钥目录失败") {
			t.Fatalf("mkdir temp err = %v", err)
		}
	})
	t.Run("open_session_stateless_preserves_explicit_keystore_dir", func(t *testing.T) {
		oldAvailable := sessionAvailable
		oldOpen := sessionOpenBackend
		t.Cleanup(func() {
			sessionAvailable = oldAvailable
			sessionOpenBackend = oldOpen
		})
		sessionAvailable = func() bool { return true }
		sessionOpenBackend = func(context.Context, Config) (Cipher, error) {
			return &fakeCipher{}, nil
		}
		explicitDir := t.TempDir()
		session, err := OpenSession(context.Background(), SessionOptions{
			AccessToken: "stateless-token",
			CorpID:      "corp-stateless",
			StaffID:     "staff-42",
			KeystoreDir: explicitDir,
		})
		if err != nil {
			t.Fatalf("OpenSession() = %v", err)
		}
		if session.KeystoreDir != explicitDir || session.StaffID != "staff-42" {
			t.Fatalf("session = %#v", session)
		}
		if err := session.Close(); err != nil {
			t.Fatalf("Close() = %v", err)
		}
		if _, err := os.Stat(explicitDir); err != nil {
			t.Fatalf("explicit keystore dir must not be deleted on Close: %v", err)
		}
	})
}
