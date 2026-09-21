// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageEmployeeUploadFailuresAreRedacted(t *testing.T) {
	file := filepath.Join(t.TempDir(), "skill.zip")
	if err := os.WriteFile(file, []byte("fixture bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, body string
		status     int
		ok         bool
	}{
		{"http rejection", `private upstream detail`, 403, false},
		{"invalid json", `{`, 200, false},
		{"business rejection", `{"success":false}`, 200, false},
		{"missing url", `{"success":true,"data":{}}`, 200, false},
		{"result envelope", `{"result":{"fileUrl":"https://fixture.invalid/package"}}`, 200, true},
		{"content envelope", `{"content":{"fileUrl":"https://fixture.invalid/package"}}`, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			u := deapAgentOpenAPISkillUploader{baseURL: server.URL, httpClient: server.Client(), validateTarget: func(string) error { return nil }, resolveCredential: func(context.Context, string) (string, error) { return "fixture-private-credential", nil }}
			got, err := u.Upload(context.Background(), "agent", file)
			if (err == nil) != tc.ok {
				t.Fatalf("url=%q err=%v", got, err)
			}
			if err != nil && (strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), server.URL)) {
				t.Fatalf("unsafe error=%v", err)
			}
			if tc.ok && got != "https://fixture.invalid/package" {
				t.Fatalf("url=%q", got)
			}
		})
	}
	t.Run("credential failure", func(t *testing.T) {
		u := deapAgentOpenAPISkillUploader{baseURL: "https://fixture.invalid", resolveCredential: func(context.Context, string) (string, error) { return "", errors.New("credential-secret") }}
		if _, err := u.Upload(context.Background(), "agent", file); err == nil || strings.Contains(err.Error(), "credential-secret") {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("missing package", func(t *testing.T) {
		u := deapAgentOpenAPISkillUploader{baseURL: "https://fixture.invalid", resolveCredential: func(context.Context, string) (string, error) { return "fixture", nil }}
		if _, err := u.Upload(context.Background(), "agent", file+".missing"); err == nil {
			t.Fatal("missing package accepted")
		}
	})
	t.Run("transport rejected before upload", func(t *testing.T) {
		u := deapAgentOpenAPISkillUploader{baseURL: "https://fixture.invalid", validateTarget: func(string) error { return errors.New("rejected") }, resolveCredential: func(context.Context, string) (string, error) { return "fixture", nil }}
		if _, err := u.Upload(context.Background(), "agent", file); err == nil {
			t.Fatal("target rejection ignored")
		}
	})
	t.Run("unknown environment", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("DWS_CONFIG_DIR", dir)
		if err := os.WriteFile(filepath.Join(dir, "mcp_url"), []byte("https://fixture.invalid"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := (deapAgentOpenAPISkillUploader{}).Upload(context.Background(), "agent", file); err == nil {
			t.Fatal("unknown environment accepted")
		}
	})
}

func TestCrossPlatformCoverageEmployeeConfigFileShapeAndSize(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, name := range []string{"missing.json", "directory.json", "oversize.json", "malformed.json", "object.json", "array.json"} {
		path := filepath.Join(dir, name)
		switch name {
		case "directory.json":
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		case "oversize.json":
			if err := os.WriteFile(path, []byte(strings.Repeat("x", deapAgentConfigFileMaxSize+1)), 0600); err != nil {
				t.Fatal(err)
			}
		case "malformed.json":
			if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
				t.Fatal(err)
			}
		case "object.json":
			if err := os.WriteFile(path, []byte(`{"name":"test"}`), 0600); err != nil {
				t.Fatal(err)
			}
		case "array.json":
			if err := os.WriteFile(path, []byte(`["test"]`), 0600); err != nil {
				t.Fatal(err)
			}
		}
		_, objectErr := deapAgentReadJSONObjectFile(name, "config-file")
		if (objectErr == nil) != (name == "object.json") {
			t.Fatalf("%s object=%v", name, objectErr)
		}
	}
	if _, err := deapAgentReadJSONFile("invalid\x00.json", "config-file"); err == nil {
		t.Fatal("unsafe path accepted")
	}
	if _, err := deapAgentValidateSkillPackage("invalid\x00.zip"); err == nil {
		t.Fatal("unsafe package path accepted")
	}
	for _, body := range []string{"{", `{}`} {
		if _, err := deapAgentParseSkillCreated([]byte(body)); err == nil {
			t.Fatal("invalid create result accepted")
		}
	}
	if got := deapAgentSkillStageFromResponse([]byte("CREATE STAGE failed"), "fallback"); got != "create" {
		t.Fatal(got)
	}
}
