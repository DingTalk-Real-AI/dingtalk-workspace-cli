package helpers

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestProductCommandMatrixCoverage complements the curated examples by walking
// every public product command, including leaves that intentionally have no
// Example text. It supplies required flags and a small set of positional/flag
// variants, exercising validation, compatibility aliases, and response parsing.
func TestCrossPlatformCoverageProductCommandMatrixCoverage(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(workingDirectory) })

	fixture := filepath.Join(work, "fixture.txt")
	if err := os.WriteFile(fixture, []byte(`[{"recordId":"record-id","cells":{"field-id":"value"}}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	confirmations := filepath.Join(work, "confirmations.txt")
	if err := os.WriteFile(confirmations, []byte(strings.Repeat("yes\n", 8192)), 0o600); err != nil {
		t.Fatal(err)
	}
	stdin, err := os.Open(confirmations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close() })
	noPath := filepath.Join(work, "no.txt")
	if err := os.WriteFile(noPath, []byte("no\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	noStdin, err := os.Open(noPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = noStdin.Close() })
	emptyPath := filepath.Join(work, "empty.txt")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	emptyStdin, err := os.Open(emptyPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = emptyStdin.Close() })

	previousDeps := deps
	previousArgs := os.Args
	previousStdin := os.Stdin
	previousPut := httpPutFile
	previousGet := httpGetFile
	t.Cleanup(func() {
		deps = previousDeps
		os.Args = previousArgs
		os.Stdin = previousStdin
		httpPutFile = previousPut
		httpGetFile = previousGet
	})

	caller := &productExampleCaller{}
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	os.Stdin = stdin
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
	httpGetFile = func(_ context.Context, _ string, _ map[string]string, destination string) error {
		if destination == "" {
			return nil
		}
		return os.WriteFile(destination, []byte("download"), 0o600)
	}

	selected := map[string]bool{
		"agoal": true, "aisearch": true, "aitable": true, "attendance": true,
		"calendar": true, "chat": true, "contact": true, "devdoc": true,
		"ding": true, "doc": true, "drive": true, "live": true,
		"mail": true, "minutes": true, "oa": true, "report": true,
		"sheet": true, "todo": true, "wiki": true,
	}
	invocations := 0
	for _, root := range NewPublicCommands(&captureRunner{}) {
		if !selected[root.Name()] {
			continue
		}
		installExampleGlobalFlags(root)
		root.SilenceErrors = true
		root.SilenceUsage = true
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		os.Args = []string{"dws", root.Name(), "--yes"}

		walkCommandMatrix(root, nil, func(command *cobra.Command, path []string) {
			// Hidden cross-product proxies expect the commands to be mounted below
			// the real dws root. Executing them on an isolated product root would
			// resolve the proxy itself instead of its peer product.
			if command.DisableFlagParsing {
				return
			}
			base := append(append([]string(nil), path...), "--yes=true")
			command.Flags().VisitAll(func(flag *pflag.Flag) {
				if _, required := flag.Annotations[cobra.BashCompOneRequiredFlag]; required {
					base = append(base, matrixFlagArg(flag, fixture))
				}
			})

			variants := [][]string{
				base,
				matrixWithoutFlag(base, "yes"),
			}
			allFlags := append([]string(nil), base...)
			var localFlags []*pflag.Flag
			if len(command.Commands()) == 0 {
				variants = append(variants,
					append(append([]string(nil), base...), "item-id"),
					append(append([]string(nil), base...), "item-id", "second-id"),
					append(append([]string(nil), base...), "item-id", "second-id", "third-id"),
				)
			}
			command.Flags().VisitAll(func(flag *pflag.Flag) {
				if matrixGlobalFlag(flag.Name) || (flag.Name == "fields" && command.LocalNonPersistentFlags().Lookup(flag.Name) == nil) {
					return
				}
				localFlags = append(localFlags, flag)
				candidate := append(append([]string(nil), base...), matrixFlagArg(flag, fixture))
				allFlags = append(allFlags, matrixFlagArg(flag, fixture))
				variants = append(variants,
					candidate,
					append(append([]string(nil), candidate...), "item-id"),
					append(append([]string(nil), candidate...), "item-id", "second-id"),
				)
				for _, invalid := range matrixInvalidFlagArgs(flag) {
					withoutFlag := matrixWithoutFlag(base, flag.Name)
					variants = append(variants,
						append(append([]string(nil), withoutFlag...), invalid),
						append(append(append([]string(nil), withoutFlag...), invalid), "item-id"),
					)
				}
				for _, semantic := range matrixSemanticFlagArgs(flag) {
					withoutFlag := matrixWithoutFlag(base, flag.Name)
					variants = append(variants, append(append([]string(nil), withoutFlag...), semantic))
				}
			})
			for left := 0; left < len(localFlags); left++ {
				for right := left + 1; right < len(localFlags); right++ {
					withoutPair := matrixWithoutFlag(matrixWithoutFlag(base, localFlags[left].Name), localFlags[right].Name)
					variants = append(variants, append(withoutPair,
						matrixFlagArg(localFlags[left], fixture),
						matrixFlagArg(localFlags[right], fixture),
					))
					for _, semantic := range matrixSemanticFlagArgs(localFlags[left]) {
						variants = append(variants, append(append([]string(nil), withoutPair...), semantic, matrixFlagArg(localFlags[right], fixture)))
					}
					for _, semantic := range matrixSemanticFlagArgs(localFlags[right]) {
						variants = append(variants, append(append([]string(nil), withoutPair...), matrixFlagArg(localFlags[left], fixture), semantic))
					}
				}
			}
			variants = append(variants, allFlags, append(append([]string(nil), allFlags...), "item-id"))

			for index, invocation := range variants {
				invocations++
				caller.mode = ""
				resetCommandFlags(root)
				root.SetArgs(invocation)
				ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
				_ = root.ExecuteContext(ctx)
				cancel()

				if index == 0 {
					for _, mode := range []string{"list-shapes", "flat-record", "nested-data", "empty-result", "empty-content", "non-text", "invalid-json", "transport-error"} {
						caller.mode = mode
						resetCommandFlags(root)
						root.SetArgs(invocation)
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
						_ = root.ExecuteContext(ctx)
						cancel()
					}
				}
			}

			noYes := matrixWithoutFlag(base, "yes")
			for _, input := range []*os.File{noStdin, emptyStdin} {
				_, _ = input.Seek(0, 0)
				os.Stdin = input
				caller.mode = ""
				caller.dry = false
				resetCommandFlags(root)
				root.SetArgs(noYes)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
				_ = root.ExecuteContext(ctx)
				cancel()
			}
			os.Stdin = stdin
			caller.dry = true
			resetCommandFlags(root)
			root.SetArgs(base)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			_ = root.ExecuteContext(ctx)
			cancel()
			caller.dry = false
		})
	}
	if invocations < 1500 {
		t.Fatalf("command matrix executed %d invocations", invocations)
	}
}

func matrixWithoutFlag(args []string, name string) []string {
	prefix := "--" + name
	out := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		if args[index] == prefix {
			index++
			continue
		}
		if strings.HasPrefix(args[index], prefix+"=") {
			continue
		}
		out = append(out, args[index])
	}
	return out
}

func matrixInvalidFlagArgs(flag *pflag.Flag) []string {
	prefix := "--" + flag.Name + "="
	switch flag.Value.Type() {
	case "bool":
		return []string{prefix + "false"}
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64", "count":
		return []string{prefix + "-1", prefix + "0", prefix + "999999999"}
	case "duration":
		return []string{prefix + "0s", prefix + "-1s"}
	default:
		return []string{prefix, prefix + "invalid", prefix + "0", prefix + "false"}
	}
}

func matrixSemanticFlagArgs(flag *pflag.Flag) []string {
	prefix := "--" + flag.Name + "="
	var values []string
	switch strings.ToLower(flag.Name) {
	case "direction":
		values = []string{"newer", "older", "sideways"}
	case "grant-type":
		values = []string{"once", "session", "timed", "permanent", "invalid"}
	case "mode":
		values = []string{"append", "overwrite", "replace", "invalid"}
	case "scope":
		values = []string{"企业", "全公司", "所有人", "chat.read", "invalid"}
	case "setting-scene":
		values = []string{"checkRemind", "fastCheck", "checkResultNotify", "lackRemind", "personalAttendStatNotify", "bossAttendStatNotify", "invalid"}
	case "content-format":
		values = []string{"markdown", "jsonml", "plain", "invalid"}
	case "role-types":
		values = []string{"creator", "executor", "participant", "creator,executor", "invalid"}
	case "action":
		values = []string{"read", "unread", "markRead", "markUnread", "archive", "invalid"}
	case "operation":
		values = []string{"include", "exclude", "eq", "invalid"}
	case "status":
		values = []string{"true", "false", "pending", "success", "failed", "active", "invalid"}
	case "type":
		values = []string{"text", "image", "file", "link", "markdown", "singleSelect", "multiSelect", "invalid"}
	case "base-time":
		values = []string{"dueTime", "customTime", "invalid"}
	case "view-type":
		values = []string{"Grid", "Kanban", "Gantt", "Gallery", "Form", "invalid"}
	case "recurrence-range-type":
		values = []string{"noEnd", "endDate", "numbered", "invalid"}
	case "recurrence-type":
		values = []string{"daily", "weekly", "absoluteMonthly", "relativeMonthly", "absoluteYearly", "invalid"}
	}
	args := make([]string, 0, len(values))
	for _, value := range values {
		args = append(args, prefix+value)
	}
	return args
}

func walkCommandMatrix(command *cobra.Command, path []string, visit func(*cobra.Command, []string)) {
	if command == nil {
		return
	}
	if len(path) > 0 {
		visit(command, path)
	}
	for _, child := range command.Commands() {
		walkCommandMatrix(child, append(append([]string(nil), path...), child.Name()), visit)
	}
}

func matrixGlobalFlag(name string) bool {
	switch name {
	case "format", "jq", "timeout", "dry-run", "yes", "help":
		return true
	default:
		return false
	}
}

func matrixFlagArg(flag *pflag.Flag, fixture string) string {
	name := strings.ToLower(flag.Name)
	value := "value"
	switch flag.Value.Type() {
	case "bool":
		value = "true"
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64", "count":
		value = "1"
	case "duration":
		value = "1s"
	case "stringSlice", "stringArray":
		value = "item-id"
	default:
		switch {
		case name == "records":
			value = `[{"recordId":"record-id","cells":{"field-id":"value"}}]`
		case name == "cells":
			value = `{"field-id":"value"}`
		case name == "fields":
			value = `[{"fieldName":"Name","type":"text"}]`
		case name == "options":
			value = `[{"name":"Option"}]`
		case name == "sort":
			value = `[{"fieldId":"field-id","direction":"asc"}]`
		case name == "values" || name == "rows":
			value = `[["value"]]`
		case name == "conditions":
			value = `[{"object":"from","or":[{"and":[{"operation":"include","value":"user@example.com"}]}]}]`
		case name == "properties":
			value = `{"position":{"row":1,"col":1},"dimensions":{"width":100,"height":100},"chart":{"type":"line","series":[{"value":["A1:A2"]}]}}`
		case name == "filters":
			value = `{"fieldId":"field-id","operator":"eq","value":"value"}`
		case name == "range":
			value = "A1:B2"
		case name == "action":
			value = "markRead"
		case name == "operation":
			value = "include"
		case strings.Contains(name, "file"), strings.Contains(name, "path"), strings.Contains(name, "attachment"), strings.Contains(name, "image"):
			value = fixture
		case strings.Contains(name, "email") || name == "from" || name == "to" || name == "cc" || name == "bcc":
			value = "user@example.com"
		case strings.Contains(name, "url") || strings.Contains(name, "webhook"):
			value = "https://example.invalid/item"
		case strings.Contains(name, "date"):
			value = "2026-01-02"
		case strings.Contains(name, "time"):
			value = "2026-01-02 03:04:05"
		case strings.Contains(name, "json") || strings.Contains(name, "properties") || strings.Contains(name, "conditions"):
			value = "{}"
		case strings.Contains(name, "type"):
			value = "text"
		case strings.Contains(name, "status"):
			value = "active"
		case strings.Contains(name, "role"):
			value = "member"
		case strings.Contains(name, "permission") || strings.Contains(name, "perm"):
			value = "read"
		case strings.Contains(name, "ids"):
			value = "item-id,second-id"
		case strings.Contains(name, "id") || strings.Contains(name, "cursor") || strings.Contains(name, "token"):
			value = "item-id"
		}
	}
	return "--" + flag.Name + "=" + value
}
