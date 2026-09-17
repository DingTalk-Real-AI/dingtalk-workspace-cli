// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageExchangeValidationBeforePersistence(t *testing.T) {
	dir := externalExchangeTestConfig(t)
	if _, err := ExchangeExternalAuthCode(context.Background(), dir, ExternalExchangeRequest{}); err == nil {
		t.Fatal("empty external request accepted")
	}
	if _, err := ExchangeManagedAuthCode(context.Background(), dir, ManagedExchangeRequest{}); err == nil {
		t.Fatal("empty managed request accepted")
	}
	if err := persistManagedExchangeToken(dir, "corp:user", nil); err == nil {
		t.Fatal("nil token accepted")
	}
	if err := os.Mkdir(filepath.Join(dir, "profiles.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := persistManagedExchangeToken(dir, "corp:user", &TokenData{CorpID: "corp", UserID: "user"}); err == nil {
		t.Fatal("unreadable profile store accepted")
	}
	if err := persistExternalExchangeToken(dir, &TokenData{}); err == nil {
		t.Fatal("unreadable existing profiles overwritten")
	}
}

func TestCrossPlatformCoverageExchangeFailedDiscoveryAndSecretPairing(t *testing.T) {
	t.Run("no concrete application", func(t *testing.T) {
		dir := externalExchangeTestConfig(t)
		if _, _, _, err := resolveExternalExchangeClient(context.Background(), dir, "", "secret"); err == nil {
			t.Fatal("unpaired secret accepted")
		}
	})
	t.Run("configured application with explicit secret", func(t *testing.T) {
		dir := externalExchangeTestConfig(t)
		testseam.Swap(t, &runtimeClientID, "configured")
		id, secret, source, err := resolveExternalExchangeClient(context.Background(), dir, "", "explicit-secret")
		if err != nil || id != "configured" || secret != "explicit-secret" || source != "flag" {
			t.Fatalf("id=%s source=%s err=%v", id, source, err)
		}
	})
	t.Run("discovery fails closed", func(t *testing.T) {
		dir := externalExchangeTestConfig(t)
		externalExchangeTestServer(t, dir, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadRequest) })
		if _, _, err := discoverExternalExchangeClient(context.Background()); err == nil {
			t.Fatal("discovery failure accepted")
		}
	})
	t.Run("cannot snapshot secret", func(t *testing.T) {
		testseam.Swap(t, &authKeychainGet, func(string, string) (string, error) { return "", errors.New("unavailable") })
		if _, _, err := snapshotExchangeClientSecret("client"); err == nil {
			t.Fatal("unreadable credential accepted")
		}
	})
}

func TestCrossPlatformCoverageManagedExchangePreflightAndPersistFailures(t *testing.T) {
	request := ManagedExchangeRequest{ClientID: "app", AuthCode: "private-code", ExpectedUserID: "user", ExpectedCorpID: "corp", PreserveProfile: "supervisor:staff", ResolveIdentity: func(context.Context, string, string) (ManagedIdentity, error) {
		return ManagedIdentity{CorpID: "corp", UserID: "user"}, nil
	}}
	t.Run("preflight", func(t *testing.T) {
		dir := externalExchangeTestConfig(t)
		testseam.Swap(t, &managedExchangePreparePersistence, func(string) error { return errors.New("read-only directory") })
		if _, err := ExchangeManagedAuthCode(context.Background(), dir, request); err == nil || !strings.Contains(err.Error(), "safely updated") {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("persistence", func(t *testing.T) {
		dir := externalExchangeTestConfig(t)
		externalExchangeTestServer(t, dir, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"accessToken":"private-access","refreshToken":"private-refresh","expiresIn":7200,"corpId":"corp"}`)
		})
		testseam.Swap(t, &managedExchangePersistToken, func(string, string, *TokenData) error { return errors.New("storage unavailable") })
		_, err := ExchangeManagedAuthCode(context.Background(), dir, request)
		if err == nil || !strings.Contains(err.Error(), "save managed digital employee profile") || strings.Contains(err.Error(), "private-") {
			t.Fatalf("error=%v", err)
		}
	})
}
