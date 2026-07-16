package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type productExampleCaller struct {
	calls  int
	mode   string
	dry    bool
	format string
}

func (c *productExampleCaller) CallTool(_ context.Context, _, _ string, _ map[string]any) (*edition.ToolResult, error) {
	c.calls++
	switch c.mode {
	case "transport-error":
		return nil, fmt.Errorf("synthetic tool transport failure")
	case "invalid-json":
		return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: "{"}}}, nil
	case "empty-content":
		return &edition.ToolResult{}, nil
	case "non-text":
		return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "image"}}}, nil
	}
	record := map[string]any{
		"id": "item-id", "openId": "open-id", "unionId": "union-id", "name": "Example",
		"title": "Example", "text": "content", "content": "content", "status": "SUCCESS",
		"type": "text", "url": "https://example.invalid/item", "downloadUrl": "https://example.invalid/download",
		"createdAt": "2026-01-02T03:04:05Z", "updatedAt": "2026-01-02T03:04:05Z",
		"startTime": "2026-01-02T03:04:05Z", "endTime": "2026-01-02T04:04:05Z",
		"fileName": "fixture.txt", "dentryId": float64(1), "spaceId": float64(2),
		"messageId": "message-id", "docId": "doc-id", "workspaceId": "workspace-id",
		"sheetId": "sheet-id", "tableId": "table-id", "baseId": "base-id",
		"recordId": "record-id", "taskId": "task-id", "jobId": "job-id",
		"eventId": "event-id", "calendarId": "calendar-id", "roomId": "room-id",
		"conversationId": "conversation-id", "openConversationId": "cid-item-id",
		"processInstanceId": "process-id", "reportId": "report-id", "templateId": "template-id",
		"todoId": "todo-id", "mediaId": "media-id", "attachmentId": "attachment-id",
		"userId": "user-id", "email": "user@example.com", "code": float64(0), "errcode": float64(0),
	}
	dataMap := cloneStringAnyMap(record)
	for key, value := range map[string]any{
		"success": true, "status": "SUCCESS", "taskStatus": "SUCCESS",
		"hasMore": false, "nextCursor": "", "cursor": "",
		"items": []any{record}, "records": []any{record}, "data": []any{record}, "list": []any{record},
		"result": record, "downloadUrl": "https://example.invalid/download", "resourceUrl": "https://example.invalid/resource",
		"message":       map[string]any{"id": "message-id"},
		"emailAccounts": []any{map[string]any{"email": "user@example.com", "type": "PERSONAL"}},
	} {
		dataMap[key] = value
	}
	resultMap := cloneStringAnyMap(record)
	for key, value := range map[string]any{
		"success": true, "status": "SUCCESS", "taskStatus": "SUCCESS", "hasMore": false,
		"nextCursor": "", "items": []any{record}, "records": []any{record}, "data": []any{record},
		"downloadUrl": "https://example.invalid/download", "resourceUrl": "https://example.invalid/resource",
		"uploadKey": "upload-key", "jobId": "job-id", "taskId": "task-id",
		"message":       map[string]any{"id": "message-id"},
		"emailAccounts": []any{map[string]any{"email": "user@example.com", "type": "PERSONAL"}},
	} {
		resultMap[key] = value
	}
	payload := map[string]any{
		"success":      true,
		"result":       resultMap,
		"data":         dataMap,
		"items":        []any{record},
		"records":      []any{record},
		"hasMore":      false,
		"nextCursor":   "",
		"status":       "SUCCESS",
		"taskStatus":   "SUCCESS",
		"downloadUrl":  "https://example.invalid/download",
		"resourceUrl":  "https://example.invalid/resource",
		"presignedUrl": "https://example.invalid/upload",
		"uploadKey":    "upload-key",
		"jobId":        "job-id",
		"taskId":       "task-id",
		"messageId":    "message-id",
		"fileName":     "fixture.txt",
		"dentryId":     float64(1),
		"spaceId":      float64(2),
		"code":         float64(0),
		"errcode":      float64(0),
	}
	if c.mode == "list-shapes" {
		payload["result"] = []any{record}
		payload["data"] = []any{record}
	}
	if c.mode == "flat-record" {
		payload = record
	}
	if c.mode == "nested-data" {
		payload = map[string]any{"data": map[string]any{"result": resultMap}}
	}
	if c.mode == "empty-result" {
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: string(raw)}}}, nil
}

func (c *productExampleCaller) Format() string {
	if c.format == "" {
		return "json"
	}
	return c.format
}
func (c *productExampleCaller) DryRun() bool { return c.dry }
func (*productExampleCaller) Fields() string { return "" }
func (*productExampleCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageProductCommandExamplesAreExecutableContracts(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("use isolated working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(workingDirectory) })

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
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
	httpGetFile = func(_ context.Context, _ string, _ map[string]string, destPath string) error {
		if destPath == "" {
			return nil
		}
		return os.WriteFile(destPath, []byte("example"), 0o600)
	}
	confirmationInput := t.TempDir() + "/confirmations.txt"
	if err := os.WriteFile(confirmationInput, []byte(strings.Repeat("yes\n", 2048)), 0o600); err != nil {
		t.Fatalf("write confirmation input: %v", err)
	}
	confirmationFile, err := os.Open(confirmationInput)
	if err != nil {
		t.Fatalf("open confirmation input: %v", err)
	}
	t.Cleanup(func() { _ = confirmationFile.Close() })
	os.Stdin = confirmationFile

	commands := NewPublicCommands(&captureRunner{})
	executed := 0
	malformedExecuted := 0
	for _, root := range commands {
		installExampleGlobalFlags(root)
		root.SilenceErrors = true
		root.SilenceUsage = true
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		os.Args = []string{"dws", root.Name(), "--yes"}

		for _, invocation := range commandExampleInvocations(root) {
			executed++
			caller.mode = ""
			resetCommandFlags(root)
			root.SetArgs(invocation)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			err := root.ExecuteContext(ctx)
			cancel()
			if isCommandSyntaxError(err) {
				t.Errorf("dws %s %s: %v", root.Name(), strings.Join(invocation, " "), err)
			}

			for _, mode := range []string{"list-shapes", "flat-record", "nested-data", "empty-result", "empty-content", "non-text", "transport-error", "invalid-json"} {
				caller.mode = mode
				resetCommandFlags(root)
				root.SetArgs(invocation)
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				_ = root.ExecuteContext(ctx)
				cancel()
			}
			caller.mode = ""

			for _, malformed := range malformedExampleInvocations(invocation) {
				malformedExecuted++
				resetCommandFlags(root)
				root.SetArgs(malformed)
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				_ = root.ExecuteContext(ctx)
				cancel()
			}
		}
	}

	if executed < 400 {
		t.Fatalf("executed %d product examples, want at least 400", executed)
	}
	if caller.calls < 250 {
		t.Fatalf("product examples made %d tool calls, want at least 250", caller.calls)
	}
	if malformedExecuted < 300 {
		t.Fatalf("executed %d malformed example variants, want at least 300", malformedExecuted)
	}
}

func malformedExampleInvocations(invocation []string) [][]string {
	var variants [][]string
	for index, token := range invocation {
		if !strings.HasPrefix(token, "--") {
			continue
		}
		if equal := strings.IndexByte(token, '='); equal >= 0 {
			variant := append([]string(nil), invocation...)
			variant[index] = token[:equal+1] + malformedFlagValue(token[equal+1:])
			variants = append(variants, variant)
			continue
		}
		if index+1 >= len(invocation) || strings.HasPrefix(invocation[index+1], "-") {
			continue
		}
		variant := append([]string(nil), invocation...)
		variant[index+1] = malformedFlagValue(invocation[index+1])
		variants = append(variants, variant)
	}
	return variants
}

func malformedFlagValue(value string) string {
	for _, r := range value {
		if r < '0' || r > '9' {
			return "invalid"
		}
	}
	return "-1"
}

func commandExampleInvocations(root *cobra.Command) [][]string {
	var invocations [][]string
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		for _, line := range logicalExampleLines(command.Example) {
			tokens, err := splitExampleCommand(line)
			if err != nil || len(tokens) < 2 || tokens[0] != "dws" || tokens[1] != root.Name() {
				continue
			}
			if unsupportedExampleTokens(tokens) {
				continue
			}
			invocations = append(invocations, tokens[2:])
		}
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)
	return invocations
}

func logicalExampleLines(example string) []string {
	physical := strings.Split(example, "\n")
	logical := make([]string, 0, len(physical))
	var current string
	for _, line := range physical {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if current != "" {
			current += " " + line
		} else {
			current = line
		}
		if strings.HasSuffix(current, "\\") {
			current = strings.TrimSpace(strings.TrimSuffix(current, "\\"))
			continue
		}
		if index := strings.Index(current, "dws "); index >= 0 {
			logical = append(logical, current[index:])
		}
		current = ""
	}
	if current != "" {
		if index := strings.Index(current, "dws "); index >= 0 {
			logical = append(logical, current[index:])
		}
	}
	return logical
}

func splitExampleCommand(line string) ([]string, error) {
	var tokens []string
	var token strings.Builder
	var quote rune
	escaped := false
	tokenStarted := false
	flush := func() {
		if tokenStarted {
			tokens = append(tokens, token.String())
			token.Reset()
			tokenStarted = false
		}
	}
	for _, r := range line {
		if escaped {
			token.WriteRune(r)
			tokenStarted = true
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				token.WriteRune(r)
				tokenStarted = true
			}
			continue
		}
		switch {
		case r == '\'' || r == '"':
			quote = r
			tokenStarted = true
		case r == '#':
			flush()
			return tokens, nil
		case unicode.IsSpace(r):
			flush()
		default:
			token.WriteRune(r)
			tokenStarted = true
		}
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("unterminated escape or quote")
	}
	flush()
	return tokens, nil
}

func installExampleGlobalFlags(root *cobra.Command) {
	flags := root.PersistentFlags()
	if flags.Lookup("format") == nil {
		flags.String("format", "json", "test global output format")
	}
	if flags.Lookup("fields") == nil {
		flags.String("fields", "", "test global field projection")
	}
	if flags.Lookup("jq") == nil {
		flags.String("jq", "", "test global jq filter")
	}
	if flags.Lookup("timeout") == nil {
		flags.Int("timeout", 30, "test global timeout")
	}
	if flags.Lookup("dry-run") == nil {
		flags.Bool("dry-run", false, "test global dry-run")
	}
	if flags.Lookup("yes") == nil {
		flags.Bool("yes", false, "test global confirmation bypass")
	}
}

func unsupportedExampleTokens(tokens []string) bool {
	for _, token := range tokens {
		if strings.ContainsAny(token, "|`$") || strings.HasPrefix(token, "[") || strings.HasSuffix(token, "]") {
			return true
		}
	}
	return false
}

func resetCommandFlags(command *cobra.Command) {
	reset := func(flag *pflag.Flag) {
		_ = flag.Value.Set(flag.DefValue)
		flag.Changed = false
	}
	command.Flags().VisitAll(reset)
	command.PersistentFlags().VisitAll(reset)
	for _, child := range command.Commands() {
		resetCommandFlags(child)
	}
}

func isCommandSyntaxError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"unknown command", "unknown flag", "flag needs an argument", "requires at least",
		"requires exactly", "accepts 1 arg", "accepts 2 arg", "required flag",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}
