// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func employeeZIPFixture(t *testing.T, headers ...zip.FileHeader) string {
	t.Helper()
	f, err := os.Create("fixture.zip")
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for _, h := range headers {
		if _, err := w.CreateRaw(&h); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return "fixture.zip"
}

type employeeZIPReadCloser struct {
	io.Reader
	closeErr error
}

func (r employeeZIPReadCloser) Close() error { return r.closeErr }

type employeeZeroReader struct{}

func (employeeZeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestCrossPlatformCoverageEmployeeZIPFailureBoundaries(t *testing.T) {
	for _, scenario := range []string{"missing", "entries", "symlink", "declared-size", "directory", "open", "expanded-size", "read", "close"} {
		t.Run(scenario, func(t *testing.T) {
			t.Chdir(t.TempDir())
			headers := []zip.FileHeader{{Name: "SKILL.md", Method: zip.Store}}
			switch scenario {
			case "entries":
				for i := 0; i < deapAgentSkillMaxEntries; i++ {
					headers = append(headers, zip.FileHeader{Name: fmt.Sprint(i)})
				}
			case "symlink":
				headers[0].SetMode(os.ModeSymlink | 0600)
			case "declared-size":
				headers[0].UncompressedSize64 = deapAgentSkillMaxExpandedSize + 1
			case "directory":
				headers = append(headers, zip.FileHeader{Name: "scripts/"})
			}
			path := employeeZIPFixture(t, headers...)
			if scenario == "missing" {
				path = "missing.zip"
			}
			if scenario == "open" || scenario == "expanded-size" || scenario == "read" || scenario == "close" {
				testseam.Swap(t, &deapAgentOpenZIPEntry, func(*zip.File) (io.ReadCloser, error) {
					switch scenario {
					case "open":
						return nil, errors.New("damaged entry")
					case "expanded-size":
						return io.NopCloser(io.LimitReader(employeeZeroReader{}, deapAgentSkillMaxExpandedSize+1)), nil
					case "read":
						return io.NopCloser(employeeReadFailure{}), nil
					default:
						return employeeZIPReadCloser{Reader: strings.NewReader(""), closeErr: errors.New("close failed")}, nil
					}
				})
			}
			_, err := deapAgentValidateSkillPackage(path)
			if (err == nil) != (scenario == "directory") {
				t.Fatalf("validation=%v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeSkillFacadeBoundaries(t *testing.T) {
	for _, scenario := range []string{"invalid", "dry-run", "upload", "staged-upload", "create", "query"} {
		t.Run(scenario, func(t *testing.T) {
			t.Chdir(t.TempDir())
			path := employeeZIPFixture(t, zip.FileHeader{Name: "SKILL.md"})
			caller, _ := newDeapAgentTestTree(t, scenario == "dry-run")
			stub := &deapAgentSkillUploaderStub{fileURL: "https://fixture.invalid/skill.zip"}
			switch scenario {
			case "invalid":
				path = "missing.zip"
			case "upload":
				stub.err = errors.New("upload failed")
			case "staged-upload":
				stub.err = &deapAgentSkillStageError{Stage: "upload", Err: errors.New("failure")}
			case "create":
				caller.err = errors.New("create stage failed")
			case "query":
				caller.resultText = `{}`
			}
			testseam.Swap(t, &deapAgentSkillUploader, deapAgentSkillPackageUploader(stub))
			cmd := newDeapAgentSkillCreateCommand()
			cmd.SetContext(context.Background())
			err := deapAgentCallSkillCreate(cmd, "", map[string]any{"agentUuid": "agent", "file": path})
			if (err == nil) != (scenario == "dry-run") {
				t.Fatalf("create=%v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeSkillCredentialAndReadBoundaries(t *testing.T) {
	newDeapAgentTestTree(t, false)
	t.Run("no caller", func(t *testing.T) {
		testseam.Swap(t, &deps, nil)
		if _, err := (deapAgentOpenAPISkillUploader{}).temporaryCredential(context.Background(), "agent"); err == nil {
			t.Fatal("missing caller accepted")
		}
	})
	if _, err := (deapAgentOpenAPISkillUploader{}).temporaryCredential(context.Background(), "agent"); err == nil {
		t.Fatal("missing credential accepted")
	}
	for _, raw := range []string{"{", `{"success":false}`} {
		if _, err := deapAgentParseSkillUploadCredential(raw); err == nil {
			t.Fatal("invalid credential accepted")
		}
	}
	var absent *deapAgentSkillStageError
	if absent.Error() == "" || absent.Unwrap() != nil {
		t.Fatal("nil stage error contract")
	}
	if (&deapAgentSkillStageError{Stage: "upload"}).Error() == "" {
		t.Fatal("empty stage")
	}
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	if err := os.WriteFile(filepath.Join(os.Getenv("DWS_CONFIG_DIR"), "mcp_url"), []byte("http://%zz"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := (deapAgentOpenAPISkillUploader{}).uploadBaseURL(); err == nil {
		t.Fatal("invalid MCP URL accepted")
	}
	t.Chdir(t.TempDir())
	if err := os.WriteFile("config.json", []byte(`[]`), 0600); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &deapAgentReadFile, func(string) ([]byte, error) { return nil, errors.New("file replaced") })
	if _, err := deapAgentReadJSONFile("config.json", "config-file"); err == nil {
		t.Fatal("read failure accepted")
	}
	cmd := newDeapAgentMCPUpdateCommand()
	cmd.SetContext(context.Background())
	if err := deapAgentCallMCPUpdate(cmd, deapAgentMCPUpdateTool, map[string]any{"configFile": "missing.json"}); err == nil {
		t.Fatal("unreadable MCP update config accepted")
	}
}
