// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

var errAicardTemplateInjected = errors.New("injected template failure")

func requireAicardTemplateError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestCrossPlatformCoverageAicardTemplateReadFailures(t *testing.T) {
	path := aicardTestFile(t, aicardTestSnapshot)
	for _, test := range []struct {
		name    string
		install func(*testing.T)
		want    string
	}{
		{"open", func(t *testing.T) {
			testseam.Swap(t, &aicardTemplateOpen, func(string) (*os.File, error) { return nil, errAicardTemplateInjected })
		}, "Cannot open template file"},
		{"read", func(t *testing.T) {
			testseam.Swap(t, &aicardTemplateReadAll, func(io.Reader) ([]byte, error) { return nil, errAicardTemplateInjected })
		}, "Cannot read template file"},
		{"grown after inspection", func(t *testing.T) {
			testseam.Swap(t, &aicardTemplateReadAll, func(io.Reader) ([]byte, error) {
				return make([]byte, aicardTemplateMaxBytes+1), nil
			})
		}, "including a leading UTF-8 BOM"},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.install(t)
			_, err := readAicardTemplateFile(path)
			requireAicardTemplateError(t, err, test.want)
		})
	}
}

func TestCrossPlatformCoverageAicardTemplateValidationFailures(t *testing.T) {
	invalid := `[{"version":"v1.0","createSurface":{"surfaceId":"s","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json","dataModel":{}}},{"version":"v1.0","updateComponents":{"surfaceId":"s","components":[{"id":"root","component":"Text","text":"x","invented":1}]}}]`
	for _, test := range []struct {
		name    string
		data    string
		install func(*testing.T)
		want    string
	}{
		{"protocol", aicardTestSnapshot, func(t *testing.T) {
			testseam.Swap(t, &aicardLoadProtocol, func() (*a2ui.Protocol, error) { return nil, errAicardTemplateInjected })
		}, "Cannot load the embedded A2UI protocol"},
		{"lint", aicardTestSnapshot, func(t *testing.T) {
			testseam.Swap(t, &aicardLint, func(*a2ui.Protocol, []byte, bool, bool) (a2ui.Report, error) {
				return a2ui.Report{}, errAicardTemplateInjected
			})
		}, "Cannot validate the template"},
		{"schema", invalid, func(*testing.T) {}, "Template violates the A2UI protocol"},
		{"preflight", aicardTestSnapshot, func(t *testing.T) {
			testseam.Swap(t, &aicardPreflight, func(*a2ui.Protocol, []string, string) (map[string]any, error) {
				return nil, errAicardTemplateInjected
			})
		}, "Template initialization is invalid"},
		{"normalization", aicardTestSnapshot, func(t *testing.T) {
			testseam.Swap(t, &aicardTemplateMarshal, func(any) ([]byte, error) { return nil, errAicardTemplateInjected })
		}, "Cannot normalize template content"},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.install(t)
			_, err := validateAicardTemplate([]byte(test.data))
			requireAicardTemplateError(t, err, test.want)
		})
	}
}

func executeAicardTemplateWithOutput(t *testing.T, caller edition.ToolCaller, commandOutput string, args ...string) error {
	t.Helper()
	testseam.Protect(t, &deps)
	InitDeps(caller)
	root := newAicardCommand()
	root.PersistentFlags().String("format", "json", "Output format")
	root.PersistentFlags().Bool("dry-run", false, "Local validation only")
	root.PersistentFlags().String("output", "", "Command output file")
	root.SilenceErrors, root.SilenceUsage = true, true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append(append([]string{"template"}, args...), "--output", commandOutput))
	ctx, _ := output.WithResultStore(t.Context())
	_, err := corecmd.ExecuteContextCForTest(root, ctx)
	return err
}

func TestCrossPlatformCoverageAicardTemplateOutputPathFailures(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	dir := t.TempDir()
	out := filepath.Join(dir, "card.json")
	get := []string{"get", "--template-id", "t", "--version", "1", "--out", out}

	t.Run("out", func(t *testing.T) {
		testseam.Swap(t, &aicardTemplateAbs, func(string) (string, error) { return "", errAicardTemplateInjected })
		caller := &aicardTemplateTestCaller{}
		_, err := executeAicardTemplate(t, caller, get...)
		requireAicardTemplateError(t, err, "Cannot resolve --out")
		if caller.invocations != 0 {
			t.Fatalf("unresolved --out invoked the service: %d", caller.invocations)
		}
	})
	t.Run("global output", func(t *testing.T) {
		result := filepath.Join(dir, "result.json")
		testseam.Swap(t, &aicardTemplateAbs, func(path string) (string, error) {
			if path == result {
				return "", errAicardTemplateInjected
			}
			return filepath.Abs(path)
		})
		caller := &aicardTemplateTestCaller{}
		requireAicardTemplateError(t, executeAicardTemplateWithOutput(t, caller, result, get...), "Cannot resolve --output")
		if caller.invocations != 0 {
			t.Fatalf("unresolved --output invoked the service: %d", caller.invocations)
		}
	})
	t.Run("non-string output flag", func(t *testing.T) {
		cmd := &cobra.Command{Use: "probe"}
		cmd.Flags().Bool("output", false, "")
		_, err := aicardTemplateCommandOutput(cmd)
		requireAicardTemplateError(t, err, "Cannot read --output")
	})
	t.Run("missing destination parent", func(t *testing.T) {
		if aicardTemplateOutputConflict(filepath.Join(dir, "missing", "card.json"), filepath.Join(dir, "other", "card.json")) {
			t.Fatal("missing parent directories were treated as one destination")
		}
	})
}

func TestCrossPlatformCoverageAicardTemplateWriteFailures(t *testing.T) {
	dir := t.TempDir()
	result := filepath.Join(dir, "result.json")
	for _, test := range []struct {
		name          string
		install       func(*testing.T)
		commandOutput string
	}{
		{"created file stat", func(t *testing.T) {
			testseam.Swap(t, &aicardTemplateStatFile, func(*os.File) (os.FileInfo, error) { return nil, errAicardTemplateInjected })
		}, result},
		{"command output stat", func(t *testing.T) {
			testseam.Swap(t, &aicardTemplateStatPath, func(string) (os.FileInfo, error) { return nil, errAicardTemplateInjected })
		}, result},
		{"write", func(t *testing.T) {
			testseam.Swap(t, &aicardTemplateWriteFile, func(*os.File, []byte) (int, error) { return 0, errAicardTemplateInjected })
		}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.install(t)
			path := filepath.Join(dir, strings.ReplaceAll(test.name, " ", "-")+".json")
			if err := writeAicardTemplateFile(path, []byte("template"), test.commandOutput); !errors.Is(err, errAicardTemplateInjected) {
				t.Fatalf("write error = %v", err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed write left a file: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageAicardTemplateGetRejectsCreatedOutputAlias(t *testing.T) {
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	dir := t.TempDir()
	out := filepath.Join(dir, "card.json")
	// Distinct names can alias only after creation on case-insensitive
	// filesystems; report the created file for --output on every platform.
	testseam.Swap(t, &aicardTemplateStatPath, func(string) (os.FileInfo, error) { return os.Stat(out) })
	caller := &aicardTemplateTestCaller{result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "file": aicardTestSnapshot}}}
	err := executeAicardTemplateWithOutput(t, caller, filepath.Join(dir, "result.json"), "get", "--template-id", "t", "--version", "1", "--out", out)
	requireAicardTemplateError(t, err, aicardTemplateOutputConflictMessage)
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("aliased destination left a file: %v", err)
	}
}

func TestCrossPlatformCoverageAicardTemplateUnicodeMalformedEscapes(t *testing.T) {
	// These byte sequences are not valid JSON; validation rejects them first,
	// so the scanner must still fail closed when called directly.
	for _, data := range []string{`"\`, `"\uZZZZ"`} {
		if aicardTemplateUnicodeValid([]byte(data)) {
			t.Fatalf("malformed escape accepted: %q", data)
		}
	}
}
