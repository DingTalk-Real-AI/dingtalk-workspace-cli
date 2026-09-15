// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"errors"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageClientIDMetadataPrecedenceAndIsolation(t *testing.T) {
	prev := edition.Get()
	edition.Override(&edition.Hooks{})
	t.Cleanup(func() { edition.Override(prev) })
	testseam.Swap(t, &runtimeClientID, "")
	testseam.Swap(t, &appConfigResolveSecret, func(SecretInput) (string, error) {
		t.Error("metadata must never resolve a secret")
		return "", errors.New("keychain unavailable")
	})
	t.Setenv("DWS_CLIENT_ID", "env-client")
	t.Setenv("DWS_CLIENT_SECRET", "")
	first, second := t.TempDir(), t.TempDir()
	writeCredentialConfig(t, first, "first-client", SecretInput{Ref: &SecretRef{Source: "keychain", ID: "unavailable"}})
	writeCredentialConfig(t, second, "second-client", SecretInput{})
	for _, tc := range []struct{ dir, want string }{{first, "first-client"}, {second, "second-client"}, {first, "first-client"}, {t.TempDir(), "env-client"}} {
		if got := ClientIDMetadata(tc.dir); got != tc.want {
			t.Fatalf("got %q want %q", got, tc.want)
		}
	}
	edition.Override(&edition.Hooks{AuthClientID: "edition-client"})
	if got := ClientIDMetadata(first); got != "edition-client" {
		t.Fatalf("edition precedence: %q", got)
	}
	runtimeClientID = "flag-client"
	if got := ClientIDMetadata(first); got != "flag-client" {
		t.Fatalf("flag precedence: %q", got)
	}
	runtimeClientID = ""
	edition.Override(&edition.Hooks{})
	t.Setenv("DWS_CLIENT_ID", "")
	testseam.Swap(t, &defaultAuthClientID, "<unset>")
	if got := ClientIDMetadata(t.TempDir()); got != "" {
		t.Fatalf("placeholder must be absent: %q", got)
	}
	defaultAuthClientID = "built-in-client"
	if got := ClientIDMetadata(t.TempDir()); got != "built-in-client" {
		t.Fatalf("built-in fallback: %q", got)
	}
}
