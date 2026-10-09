// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeSkillUploadKeepsValidatedFile(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		for _, replacement := range []string{"regular", "symlink"} {
			t.Run(operation+"/"+replacement, func(t *testing.T) {
				if replacement == "symlink" && runtime.GOOS == "windows" {
					t.Skip("symlink creation requires additional Windows privileges; regular replacement is covered")
				}
				t.Chdir(t.TempDir())
				path := employeeZIPFixture(t, zip.FileHeader{Name: "SKILL.md", Method: zip.Store})
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile("private.txt", []byte("unvalidated-private-content"), 0600); err != nil {
					t.Fatal(err)
				}
				received := make(chan []byte, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					file, _, err := r.FormFile("file")
					if err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					defer file.Close()
					defer r.MultipartForm.RemoveAll()
					uploaded, err := io.ReadAll(file)
					if err != nil {
						t.Error(err)
					}
					received <- uploaded
					_, _ = io.WriteString(w, `{"fileUrl":"https://fixture.invalid/skill.zip"}`)
				}))
				defer server.Close()
				credentialCalls := 0
				uploader := deapAgentOpenAPISkillUploader{
					baseURL: server.URL, httpClient: server.Client(), validateTarget: func(string) error { return nil },
					resolveCredential: func(context.Context, string) (string, error) {
						credentialCalls++
						// Run after validation but before the old uploader opens the path.
						if err := os.Rename(path, path+".validated"); err != nil {
							if runtime.GOOS == "windows" && platformEmployeeRenameBusy(err) {
								return "fixture", nil // An open handle may prevent replacement on Windows.
							}
							return "", err
						}
						if replacement == "symlink" {
							return "fixture", os.Symlink("private.txt", path)
						}
						return "fixture", os.WriteFile(path, []byte("unvalidated-private-content"), 0600)
					},
				}
				testseam.Swap(t, &deapAgentSkillUploader, deapAgentSkillPackageUploader(uploader))
				caller, _ := newDeapAgentTestTree(t, false)
				caller.resultText = `{"success":true,"data":{"skillId":"skill-1"}}`
				cmd := newDeapAgentSkillCreateCommand()
				cmd.SetContext(context.Background())
				args := map[string]any{"agentUuid": "agent", "skillId": "skill-1", "file": path}
				if operation == "create" {
					err = deapAgentCallSkillCreate(cmd, deapAgentSkillCreateFileTool, args)
				} else {
					err = deapAgentCallSkillUpdate(cmd, deapAgentSkillUpdateTool, args)
				}
				if err != nil {
					t.Fatal(err)
				}
				if credentialCalls != 1 || len(caller.calls) != 1 {
					t.Fatalf("credential calls=%d resource calls=%d", credentialCalls, len(caller.calls))
				}
				uploaded := <-received
				if !bytes.Equal(uploaded, want) {
					t.Fatalf("uploaded unvalidated replacement: got %d bytes, want the validated %d-byte ZIP", len(uploaded), len(want))
				}
			})
		}
	}
}

func TestCrossPlatformCoverageEmployeeSkillValidationRejectsClosedHandle(t *testing.T) {
	t.Chdir(t.TempDir())
	path := employeeZIPFixture(t, zip.FileHeader{Name: "SKILL.md", Method: zip.Store})
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := deapAgentValidateOpenedSkillPackage(file, path); err == nil {
		t.Fatal("closed validation handle accepted")
	}
}

func TestCrossPlatformCoverageEmployeeSkillValidationRejectsDirectoryHandle(t *testing.T) {
	file, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := deapAgentValidateOpenedSkillPackage(file, file.Name()); err == nil {
		t.Fatal("nonregular validation handle accepted")
	}
}
