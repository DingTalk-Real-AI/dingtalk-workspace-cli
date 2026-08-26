package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type floatImageToolCall struct {
	product string
	tool    string
	args    map[string]any
}

type floatImageTestCaller struct {
	dryRun bool
	format string
	calls  []floatImageToolCall
	call   func(context.Context, string, string, map[string]any, int) (*edition.ToolResult, error)
}

func (c *floatImageTestCaller) CallTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	copyArgs := make(map[string]any, len(args))
	for key, value := range args {
		copyArgs[key] = value
	}
	index := len(c.calls)
	c.calls = append(c.calls, floatImageToolCall{product: product, tool: tool, args: copyArgs})
	if c.call != nil {
		return c.call(ctx, product, tool, args, index)
	}
	return floatImageTextResult(`{"floatImage":{"id":"fi-1"}}`), nil
}

func (c *floatImageTestCaller) Format() string {
	if c.format == "" {
		return "json"
	}
	return c.format
}

func (c *floatImageTestCaller) DryRun() bool { return c.dryRun }
func (*floatImageTestCaller) Fields() string { return "" }
func (*floatImageTestCaller) JQ() string     { return "" }

type floatImageRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn floatImageRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func floatImageTextResult(text string) *edition.ToolResult {
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}
}

func floatImageCredentialResult(uploadURL string, nested bool) *edition.ToolResult {
	credential := map[string]any{
		"uploadUrl": uploadURL, "resourceId": "rid-1", "resourceUrl": "/core/api/resources/img/rid-1",
	}
	payload := any(credential)
	if nested {
		payload = map[string]any{"result": credential}
	}
	raw, _ := json.Marshal(payload)
	return floatImageTextResult(string(raw))
}

func floatImageCommandForTest(t *testing.T, name string) *cobra.Command {
	t.Helper()
	for _, command := range newFloatImageCmds() {
		if command.Name() == name {
			return command
		}
	}
	t.Fatalf("missing float-image command %s", name)
	return nil
}

func setFloatImageFlags(t *testing.T, command *cobra.Command, values map[string]string) {
	t.Helper()
	for name, value := range values {
		if err := command.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
}

func installFloatImageDeps(t *testing.T, caller *floatImageTestCaller, output io.Writer) {
	t.Helper()
	if output == nil {
		output = io.Discard
	}
	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: NewFormatterWithWriters(output, io.Discard)})
	previousArgs := os.Args
	os.Args = []string{"dws", "sheet"}
	t.Cleanup(func() { os.Args = previousArgs })
}

func writeFloatImageFixture(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func openFloatImageFixture(t *testing.T, path string) *floatImageLocalFile {
	t.Helper()
	local, err := openFloatImageLocalFile(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	t.Cleanup(func() { _ = local.file.Close() })
	return local
}

func TestCrossPlatformCoverageFloatImageInputValidation(t *testing.T) {
	create := floatImageCommandForTest(t, "create-float-image")
	if _, _, err := validateFloatImageCreateInput(create); err == nil || !strings.Contains(err.Error(), floatImageCreateInputError) {
		t.Fatalf("missing create input error = %v", err)
	}
	setFloatImageFlags(t, create, map[string]string{"file": "a.png", "src": "/img/a"})
	if _, _, err := validateFloatImageCreateInput(create); err == nil || !strings.Contains(err.Error(), floatImageCreateInputError) {
		t.Fatalf("conflicting create input error = %v", err)
	}
	createEmptyFile := floatImageCommandForTest(t, "create-float-image")
	setFloatImageFlags(t, createEmptyFile, map[string]string{"file": " ", "src": "/img/a"})
	if _, _, err := validateFloatImageCreateInput(createEmptyFile); err == nil || !strings.Contains(err.Error(), "--file 不能为空") {
		t.Fatalf("empty create file error = %v", err)
	}

	fileOnly := floatImageCommandForTest(t, "create-float-image")
	setFloatImageFlags(t, fileOnly, map[string]string{"file": " a.png "})
	if file, src, err := validateFloatImageCreateInput(fileOnly); err != nil || file != " a.png " || src != "" {
		t.Fatalf("file-only input = %q/%q/%v", file, src, err)
	}
	srcOnly := floatImageCommandForTest(t, "create-float-image")
	setFloatImageFlags(t, srcOnly, map[string]string{"src": " /img/a "})
	if file, src, err := validateFloatImageCreateInput(srcOnly); err != nil || file != "" || src != " /img/a " {
		t.Fatalf("src-only input = %q/%q/%v", file, src, err)
	}

	updateBoth := floatImageCommandForTest(t, "update-float-image")
	setFloatImageFlags(t, updateBoth, map[string]string{"file": "a.png", "src": "/img/a"})
	if _, _, err := validateFloatImageUpdateInput(updateBoth); err == nil || !strings.Contains(err.Error(), floatImageUpdateInputError) {
		t.Fatalf("conflicting update input error = %v", err)
	}
	updateEmptyFile := floatImageCommandForTest(t, "update-float-image")
	setFloatImageFlags(t, updateEmptyFile, map[string]string{"file": " "})
	if _, _, err := validateFloatImageUpdateInput(updateEmptyFile); err == nil || !strings.Contains(err.Error(), "--file 不能为空") {
		t.Fatalf("empty update file error = %v", err)
	}
	updateSrc := floatImageCommandForTest(t, "update-float-image")
	setFloatImageFlags(t, updateSrc, map[string]string{"src": ""})
	if file, src, err := validateFloatImageUpdateInput(updateSrc); err != nil || file != "" || src != "" {
		t.Fatalf("src update compatibility = %q/%q/%v", file, src, err)
	}
}

func TestCrossPlatformCoverageFloatImageLocalFileValidation(t *testing.T) {
	validPath := writeFloatImageFixture(t, "chart.png", "png-data")
	local, err := openFloatImageLocalFile(validPath)
	if err != nil {
		t.Fatal(err)
	}
	if local.name != "chart.png" || local.mimeType != "image/png" || local.size != int64(len("png-data")) {
		t.Fatalf("local metadata = %#v", local)
	}
	_ = local.file.Close()

	if _, err := openFloatImageLocalFile(filepath.Join(t.TempDir(), "missing.png")); err == nil || !strings.Contains(err.Error(), "无法读取") {
		t.Fatalf("missing file error = %v", err)
	}
	if _, err := openFloatImageLocalFile(t.TempDir()); err == nil || !strings.Contains(err.Error(), "普通文件") {
		t.Fatalf("directory error = %v", err)
	}
	emptyPath := writeFloatImageFixture(t, "empty.png", "")
	if _, err := openFloatImageLocalFile(emptyPath); err == nil || !strings.Contains(err.Error(), "不能为空文件") {
		t.Fatalf("empty error = %v", err)
	}

	validInfo, err := os.Stat(validPath)
	if err != nil {
		t.Fatal(err)
	}
	statValid := func(string) (os.FileInfo, error) { return validInfo, nil }
	openFailure := errors.New("open sentinel")
	if _, err := openFloatImageLocalFileWith(validPath, statValid, func(string) (*os.File, error) { return nil, openFailure }); !errors.Is(err, openFailure) {
		t.Fatalf("open error = %v", err)
	}
	closed, err := os.Open(validPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = closed.Close()
	if _, err := openFloatImageLocalFileWith(validPath, statValid, func(string) (*os.File, error) { return closed, nil }); err == nil || !strings.Contains(err.Error(), "已打开") {
		t.Fatalf("opened stat error = %v", err)
	}
	if _, err := openFloatImageLocalFileWith(validPath, statValid, func(string) (*os.File, error) { return os.Open(t.TempDir()) }); err == nil || !strings.Contains(err.Error(), "普通文件") {
		t.Fatalf("descriptor type error = %v", err)
	}
	if _, err := openFloatImageLocalFileWith(validPath, statValid, func(string) (*os.File, error) { return os.Open(emptyPath) }); err == nil || !strings.Contains(err.Error(), "不能为空文件") {
		t.Fatalf("descriptor size error = %v", err)
	}
}

func TestCrossPlatformCoverageFloatImageCredentialParsingAndURLPolicy(t *testing.T) {
	invalidResults := []*edition.ToolResult{
		nil,
		{},
		{Content: []edition.ContentBlock{{Type: "image", Text: "credential-secret"}}},
		floatImageTextResult("{"),
		floatImageTextResult(`{"uploadUrl":"https://upload.invalid/signed-secret","resourceId":"rid"}`),
		floatImageTextResult(`{"uploadUrl":"https://upload.invalid/signed-secret","resourceId":"bad\nvalue","resourceUrl":"/core/api/resources/img/rid"}`),
		floatImageTextResult(`{"uploadUrl":"https://upload.invalid/signed-secret","resourceId":"rid","resourceUrl":"https://resource.invalid/rid"}`),
		floatImageTextResult(`{"uploadUrl":"http://example.com/signed-secret","resourceId":"rid","resourceUrl":"/core/api/resources/img/rid"}`),
	}
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "")
	for index, result := range invalidResults {
		_, err := parseFloatImageUploadInfo(result)
		if err == nil {
			t.Fatalf("invalid result %d succeeded", index)
		}
		if strings.Contains(err.Error(), "signed-secret") || strings.Contains(err.Error(), "credential-secret") {
			t.Fatalf("invalid result %d leaked secret: %v", index, err)
		}
	}

	top := floatImageTextResult(`{"uploadUrl":"https://upload.invalid/path?signature=secret","resourceId":"rid-top","resourceUrl":"/core/api/resources/img/top"}`)
	info, err := parseFloatImageUploadInfo(top)
	if err != nil || info.resourceID != "rid-top" || info.resourceURL != "/core/api/resources/img/top" {
		t.Fatalf("top-level credential = %#v, %v", info, err)
	}
	nested := &edition.ToolResult{Content: []edition.ContentBlock{
		{Type: "text", Text: " "},
		{Type: "image", Text: "ignored"},
		{Type: "text", Text: `{"result":{"uploadUrl":"https://upload.invalid/nested","resourceId":"rid-nested","resourceUrl":"/core/api/resources/img/nested"}}`},
	}}
	info, err = parseFloatImageUploadInfo(nested)
	if err != nil || info.resourceID != "rid-nested" {
		t.Fatalf("nested credential = %#v, %v", info, err)
	}

	for _, value := range []string{"", "//host/path", "/path?secret=x", "/path#fragment", "/bad\npath"} {
		if validFloatImageResourceURL(value) {
			t.Errorf("resource URL %q unexpectedly valid", value)
		}
	}
	if !validFloatImageResourceURL("/core/api/resources/img/ok") || validFloatImageResourceID("") || validFloatImageResourceID("bad\x1bvalue") {
		t.Fatal("resource identifier validation mismatch")
	}

	for _, rawURL := range []string{"not a URL", "/relative", "https://user:pass@example.com/path", "https://example.com/path#fragment", "ftp://example.com/path", "http://example.com/path"} {
		if err := validateFloatImageUploadURL(rawURL); err == nil {
			t.Errorf("upload URL %q unexpectedly valid", rawURL)
		}
	}
	if err := validateFloatImageUploadURL("https://example.com/path?signature=secret"); err != nil {
		t.Fatalf("HTTPS URL rejected: %v", err)
	}
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	for _, rawURL := range []string{"http://localhost/path", "http://127.0.0.1/path", "http://[::1]/path"} {
		if err := validateFloatImageUploadURL(rawURL); err != nil {
			t.Errorf("loopback URL %q rejected: %v", rawURL, err)
		}
	}
	if err := validateFloatImageUploadURL("http://192.0.2.1/path"); err == nil || isFloatImageLoopbackHost("not-an-ip") || isFloatImageLoopbackHost("127.0.0.2") {
		t.Fatalf("non-loopback policy mismatch: %v", err)
	}
}

func TestCrossPlatformCoverageFloatImageHTTPPut(t *testing.T) {
	path := writeFloatImageFixture(t, "chart.png", "streamed-image")

	t.Run("nil client", func(t *testing.T) {
		local := openFloatImageFixture(t, path)
		if err := putFloatImageFileWithClient(context.Background(), nil, "https://upload.invalid", local); err == nil {
			t.Fatal("nil client succeeded")
		}
	})
	t.Run("closed file", func(t *testing.T) {
		local := openFloatImageFixture(t, path)
		_ = local.file.Close()
		if err := putFloatImageFileWithClient(context.Background(), http.DefaultClient, "https://upload.invalid", local); err == nil || !strings.Contains(err.Error(), "无法读取本地文件") {
			t.Fatalf("closed file error = %v", err)
		}
	})
	t.Run("invalid request", func(t *testing.T) {
		local := openFloatImageFixture(t, path)
		if err := putFloatImageFileWithClient(context.Background(), http.DefaultClient, ":", local); err == nil || !strings.Contains(err.Error(), "无法创建请求") {
			t.Fatalf("request error = %v", err)
		}
	})
	t.Run("transport secret redacted", func(t *testing.T) {
		local := openFloatImageFixture(t, path)
		client := &http.Client{Transport: floatImageRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("transport signed-secret https://upload.invalid")
		})}
		err := putFloatImageFileWithClient(context.Background(), client, "https://upload.invalid/signed-secret", local)
		if err == nil || strings.Contains(err.Error(), "signed-secret") || !strings.Contains(err.Error(), "网络请求失败") {
			t.Fatalf("transport error = %v", err)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		local := openFloatImageFixture(t, path)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		client := &http.Client{Transport: floatImageRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return nil, request.Context().Err()
		})}
		err := putFloatImageFileWithClient(ctx, client, "https://upload.invalid/path", local)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel error = %v", err)
		}
	})
	t.Run("client deadline", func(t *testing.T) {
		local := openFloatImageFixture(t, path)
		client := &http.Client{Transport: floatImageRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		})}
		err := putFloatImageFileWithClient(context.Background(), client, "https://upload.invalid/path", local)
		if err == nil || !strings.Contains(err.Error(), "超过传输时限") {
			t.Fatalf("deadline error = %v", err)
		}
	})
	t.Run("non-200 body redacted", func(t *testing.T) {
		local := openFloatImageFixture(t, path)
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(response, "response-body-secret")
		}))
		defer server.Close()
		err := putFloatImageFileWithClient(context.Background(), server.Client(), server.URL+"/signed-secret", local)
		if err == nil || !strings.Contains(err.Error(), "HTTP 403") || strings.Contains(err.Error(), "response-body-secret") || strings.Contains(err.Error(), "signed-secret") {
			t.Fatalf("status error = %v", err)
		}
	})
	t.Run("stream success", func(t *testing.T) {
		local := openFloatImageFixture(t, path)
		var method, contentType, body string
		var contentLength int64
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			method = request.Method
			contentType = request.Header.Get("Content-Type")
			contentLength = request.ContentLength
			raw, _ := io.ReadAll(request.Body)
			body = string(raw)
			response.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		if err := putFloatImageFileWithClient(context.Background(), server.Client(), server.URL, local); err != nil {
			t.Fatal(err)
		}
		if method != http.MethodPut || contentType != "image/png" || contentLength != int64(len("streamed-image")) || body != "streamed-image" {
			t.Fatalf("request = %s %s %d %q", method, contentType, contentLength, body)
		}
	})
	t.Run("redirect not followed", func(t *testing.T) {
		local := openFloatImageFixture(t, path)
		destinationCalls := 0
		destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { destinationCalls++ }))
		defer destination.Close()
		source := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.Header().Set("Location", destination.URL+"/location-secret")
			response.WriteHeader(http.StatusFound)
		}))
		defer source.Close()
		client := newFloatImageUploadClient()
		if client.Timeout != floatImageUploadTimeout {
			t.Fatalf("timeout = %v", client.Timeout)
		}
		err := putFloatImageFileWithClient(context.Background(), client, source.URL, local)
		if err == nil || !strings.Contains(err.Error(), "HTTP 302") || strings.Contains(err.Error(), "location-secret") || destinationCalls != 0 {
			t.Fatalf("redirect error/calls = %v/%d", err, destinationCalls)
		}
	})
}

func TestCrossPlatformCoverageFloatImageDryRun(t *testing.T) {
	path := writeFloatImageFixture(t, "chart.png", "image")
	for _, format := range []string{"json", "table"} {
		t.Run(format, func(t *testing.T) {
			caller := &floatImageTestCaller{dryRun: true, format: format}
			var output bytes.Buffer
			installFloatImageDeps(t, caller, &output)
			command := floatImageCommandForTest(t, "create-float-image")
			setFloatImageFlags(t, command, map[string]string{
				"node": "node-1", "sheet-id": "sheet-1", "file": path, "range": "A1", "width": "100", "height": "80",
			})
			if err := command.RunE(command, nil); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("dry-run remote calls = %d", len(caller.calls))
			}
			if format == "json" {
				var payload map[string]any
				if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
					t.Fatalf("decode dry-run: %v; output=%s", err, output.String())
				}
				arguments, _ := payload["arguments"].(map[string]any)
				if payload["tool"] != "create_float_image" || arguments["src"] != floatImagePlannedResource || payload["dry_run"] != true || payload["executed"] != false {
					t.Fatalf("dry-run payload = %#v", payload)
				}
				if _, exists := payload["preview_kind"]; exists {
					t.Fatalf("request preview unexpectedly declares preview_kind: %#v", payload)
				}
			} else if !strings.Contains(output.String(), "validate_local_file -> get_upload_credentials -> upload_file -> create_float_image") {
				t.Fatalf("pretty dry-run = %s", output.String())
			}
		})
	}
	for _, format := range []string{"json", "table"} {
		t.Run("update-"+format, func(t *testing.T) {
			caller := &floatImageTestCaller{dryRun: true, format: format}
			var output bytes.Buffer
			installFloatImageDeps(t, caller, &output)
			command := floatImageCommandForTest(t, "update-float-image")
			setFloatImageFlags(t, command, map[string]string{
				"node": "node-1", "sheet-id": "sheet-1", "float-image-id": "fi-1", "file": path, "width": "120",
			})
			if err := command.RunE(command, nil); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("update dry-run remote calls = %d", len(caller.calls))
			}
			if format == "json" {
				var payload map[string]any
				if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
					t.Fatalf("decode update dry-run: %v; output=%s", err, output.String())
				}
				arguments, _ := payload["arguments"].(map[string]any)
				if payload["tool"] != "update_float_image" || arguments["src"] != floatImagePlannedResource || arguments["floatImageId"] != "fi-1" || payload["dry_run"] != true || payload["executed"] != false {
					t.Fatalf("update dry-run payload = %#v", payload)
				}
			} else if !strings.Contains(output.String(), "validate_local_file -> get_upload_credentials -> upload_file -> update_float_image") {
				t.Fatalf("update pretty dry-run = %s", output.String())
			}
		})
	}
}

func TestCrossPlatformCoverageFloatImageCommandWorkflows(t *testing.T) {
	path := writeFloatImageFixture(t, "chart.png", "command-image")
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")

	t.Run("create src", func(t *testing.T) {
		caller := &floatImageTestCaller{}
		installFloatImageDeps(t, caller, io.Discard)
		command := floatImageCommandForTest(t, "create-float-image")
		setFloatImageFlags(t, command, map[string]string{
			"node": "node-1", "sheet-id": "sheet-1", "src": "/core/api/resources/img/existing", "range": "A1", "width": "100", "height": "80",
		})
		if err := command.RunE(command, nil); err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 1 || caller.calls[0].product != "sheet" || caller.calls[0].tool != "create_float_image" || caller.calls[0].args["src"] != "/core/api/resources/img/existing" {
			t.Fatalf("src calls = %#v", caller.calls)
		}
	})

	t.Run("create and update file", func(t *testing.T) {
		var uploadBodies []string
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			raw, _ := io.ReadAll(request.Body)
			uploadBodies = append(uploadBodies, string(raw))
			response.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		for _, commandName := range []string{"create-float-image", "update-float-image"} {
			t.Run(commandName, func(t *testing.T) {
				caller := &floatImageTestCaller{call: func(_ context.Context, product, tool string, _ map[string]any, index int) (*edition.ToolResult, error) {
					if index == 0 {
						if product != "doc" || tool != "get_doc_attachment_upload_info" {
							t.Fatalf("credential route = %s/%s", product, tool)
						}
						return floatImageCredentialResult(server.URL+"/signed-secret", commandName == "update-float-image"), nil
					}
					return floatImageTextResult(`{"floatImage":{"id":"fi-ok"}}`), nil
				}}
				var output bytes.Buffer
				installFloatImageDeps(t, caller, &output)
				command := floatImageCommandForTest(t, commandName)
				flags := map[string]string{"node": "node-1", "sheet-id": "sheet-1", "file": path}
				if commandName == "create-float-image" {
					flags["range"], flags["width"], flags["height"] = "A1", "100", "80"
				} else {
					flags["float-image-id"], flags["width"] = "fi-1", "120"
				}
				setFloatImageFlags(t, command, flags)
				if err := command.RunE(command, nil); err != nil {
					t.Fatal(err)
				}
				if len(caller.calls) != 2 || caller.calls[1].product != "sheet" || caller.calls[1].tool != strings.ReplaceAll(commandName, "-", "_") {
					t.Fatalf("workflow calls = %#v", caller.calls)
				}
				if caller.calls[0].args["fileName"] != "chart.png" || caller.calls[0].args["mimeType"] != "image/png" || caller.calls[1].args["src"] != "/core/api/resources/img/rid-1" {
					t.Fatalf("workflow args = %#v", caller.calls)
				}
				var payload map[string]any
				if err := json.Unmarshal(output.Bytes(), &payload); err != nil || payload["floatImage"] == nil {
					t.Fatalf("final output = %s, %v", output.String(), err)
				}
			})
		}
		if len(uploadBodies) != 2 || uploadBodies[0] != "command-image" || uploadBodies[1] != "command-image" {
			t.Fatalf("upload bodies = %#v", uploadBodies)
		}
	})

	t.Run("pretty progress", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(http.StatusOK) }))
		defer server.Close()
		caller := &floatImageTestCaller{format: "table", call: func(_ context.Context, _, _ string, _ map[string]any, index int) (*edition.ToolResult, error) {
			if index == 0 {
				return floatImageCredentialResult(server.URL, false), nil
			}
			return floatImageTextResult(`{"floatImage":{"id":"fi-pretty"}}`), nil
		}}
		var output bytes.Buffer
		installFloatImageDeps(t, caller, &output)
		command := floatImageCommandForTest(t, "create-float-image")
		setFloatImageFlags(t, command, map[string]string{"node": "node-1", "sheet-id": "sheet-1", "file": path, "range": "A1", "width": "100", "height": "80"})
		if err := command.RunE(command, nil); err != nil {
			t.Fatal(err)
		}
		for _, marker := range []string{"[1/3]", "resourceId", "[2/3]", "[3/3]"} {
			if !strings.Contains(output.String(), marker) {
				t.Fatalf("pretty output missing %q: %s", marker, output.String())
			}
		}
	})

	t.Run("validation and local error", func(t *testing.T) {
		caller := &floatImageTestCaller{}
		installFloatImageDeps(t, caller, io.Discard)
		create := floatImageCommandForTest(t, "create-float-image")
		setFloatImageFlags(t, create, map[string]string{"node": "node-1", "sheet-id": "sheet-1", "range": "A1", "width": "100", "height": "80"})
		if err := create.RunE(create, nil); err == nil || !strings.Contains(err.Error(), floatImageCreateInputError) {
			t.Fatalf("create validation = %v", err)
		}
		missing := floatImageCommandForTest(t, "create-float-image")
		setFloatImageFlags(t, missing, map[string]string{"node": "node-1", "sheet-id": "sheet-1", "file": filepath.Join(t.TempDir(), "missing.png"), "range": "A1", "width": "100", "height": "80"})
		if err := missing.RunE(missing, nil); err == nil || !strings.Contains(err.Error(), "无法读取") {
			t.Fatalf("local error = %v", err)
		}
		update := floatImageCommandForTest(t, "update-float-image")
		if err := update.RunE(update, nil); err == nil || !strings.Contains(err.Error(), floatImageUpdateFieldsError) {
			t.Fatalf("update fields error = %v", err)
		}
		conflict := floatImageCommandForTest(t, "update-float-image")
		setFloatImageFlags(t, conflict, map[string]string{"file": path, "src": "/img/a"})
		if err := conflict.RunE(conflict, nil); err == nil || !strings.Contains(err.Error(), floatImageUpdateInputError) {
			t.Fatalf("update conflict = %v", err)
		}
		missingIdentity := floatImageCommandForTest(t, "update-float-image")
		setFloatImageFlags(t, missingIdentity, map[string]string{"node": "node-1", "sheet-id": "sheet-1", "file": path})
		if err := missingIdentity.RunE(missingIdentity, nil); err == nil || !strings.Contains(err.Error(), "float-image-id") || len(caller.calls) != 0 {
			t.Fatalf("update identity error/calls = %v/%d", err, len(caller.calls))
		}
	})

	t.Run("credential and put failures", func(t *testing.T) {
		credentialCause := errors.New("credential transport")
		caller := &floatImageTestCaller{call: func(context.Context, string, string, map[string]any, int) (*edition.ToolResult, error) {
			return nil, credentialCause
		}}
		installFloatImageDeps(t, caller, io.Discard)
		command := floatImageCommandForTest(t, "create-float-image")
		setFloatImageFlags(t, command, map[string]string{"node": "node-1", "sheet-id": "sheet-1", "file": path, "range": "A1", "width": "100", "height": "80"})
		if err := command.RunE(command, nil); !errors.Is(err, credentialCause) {
			t.Fatalf("credential error = %v", err)
		}

		secret := "credential-response-secret"
		malformed := &floatImageTestCaller{call: func(context.Context, string, string, map[string]any, int) (*edition.ToolResult, error) {
			return floatImageTextResult(secret), nil
		}}
		installFloatImageDeps(t, malformed, io.Discard)
		command = floatImageCommandForTest(t, "create-float-image")
		setFloatImageFlags(t, command, map[string]string{"node": "node-1", "sheet-id": "sheet-1", "file": path, "range": "A1", "width": "100", "height": "80"})
		if err := command.RunE(command, nil); err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("malformed credential error = %v", err)
		}

		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(response, "put-body-secret")
		}))
		defer server.Close()
		putCaller := &floatImageTestCaller{call: func(context.Context, string, string, map[string]any, int) (*edition.ToolResult, error) {
			return floatImageCredentialResult(server.URL+"/signed-secret", false), nil
		}}
		installFloatImageDeps(t, putCaller, io.Discard)
		command = floatImageCommandForTest(t, "create-float-image")
		setFloatImageFlags(t, command, map[string]string{"node": "node-1", "sheet-id": "sheet-1", "file": path, "range": "A1", "width": "100", "height": "80"})
		if err := command.RunE(command, nil); err == nil || len(putCaller.calls) != 1 || strings.Contains(err.Error(), "put-body-secret") || strings.Contains(err.Error(), "signed-secret") {
			t.Fatalf("put error/calls = %v/%d", err, len(putCaller.calls))
		}
	})

	t.Run("partial final failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(http.StatusOK) }))
		defer server.Close()
		finalCause := errors.New("final operation sentinel")
		caller := &floatImageTestCaller{call: func(_ context.Context, _, _ string, _ map[string]any, index int) (*edition.ToolResult, error) {
			if index == 0 {
				return floatImageCredentialResult(server.URL+"/signed-secret", false), nil
			}
			return nil, finalCause
		}}
		installFloatImageDeps(t, caller, io.Discard)
		command := floatImageCommandForTest(t, "create-float-image")
		setFloatImageFlags(t, command, map[string]string{"node": "node-1", "sheet-id": "sheet-1", "file": path, "range": "A1", "width": "100", "height": "80"})
		err := command.RunE(command, nil)
		if !errors.Is(err, finalCause) || !strings.Contains(err.Error(), "resourceId=rid-1") || !strings.Contains(err.Error(), "list/get/UI") || strings.Contains(err.Error(), "signed-secret") {
			t.Fatalf("partial failure = %v", err)
		}
	})

	t.Run("update move only", func(t *testing.T) {
		caller := &floatImageTestCaller{}
		installFloatImageDeps(t, caller, io.Discard)
		command := floatImageCommandForTest(t, "update-float-image")
		setFloatImageFlags(t, command, map[string]string{"node": "node-1", "sheet-id": "sheet-1", "float-image-id": "fi-1", "range": "C5"})
		if err := command.RunE(command, nil); err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 1 || caller.calls[0].args["range"] != "C5" {
			t.Fatalf("move-only calls = %#v", caller.calls)
		}
	})
}

func TestCrossPlatformCoverageResolveFloatImageUploadInfo(t *testing.T) {
	path := writeFloatImageFixture(t, "chart.webp", "webp")
	local := openFloatImageFixture(t, path)
	caller := &floatImageTestCaller{call: func(ctx context.Context, product, tool string, args map[string]any, _ int) (*edition.ToolResult, error) {
		if ctx == nil || product != "doc" || tool != "get_doc_attachment_upload_info" {
			t.Fatalf("route/context = %s/%s/%v", product, tool, ctx)
		}
		if args["nodeId"] != "node-1" || args["fileName"] != "chart.webp" || args["mimeType"] != "image/webp" || args["fileSize"] != float64(4) {
			t.Fatalf("credential args = %#v", args)
		}
		return floatImageCredentialResult("https://upload.invalid/path", false), nil
	}}
	installFloatImageDeps(t, caller, io.Discard)
	info, err := resolveFloatImageUploadInfo(context.Background(), "node-1", local)
	if err != nil || info.resourceID != "rid-1" {
		t.Fatalf("resolve info = %#v/%v", info, err)
	}

	cause := fmt.Errorf("timeout synthetic")
	caller.call = func(context.Context, string, string, map[string]any, int) (*edition.ToolResult, error) {
		return nil, cause
	}
	if _, err := resolveFloatImageUploadInfo(context.Background(), "node-1", local); !errors.Is(err, cause) {
		t.Fatalf("resolve error = %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	caller.call = func(ctx context.Context, _ string, _ string, _ map[string]any, _ int) (*edition.ToolResult, error) {
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("credential context error = %v", ctx.Err())
		}
		return nil, ctx.Err()
	}
	if _, err := resolveFloatImageUploadInfo(cancelled, "node-1", local); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled credential error = %v", err)
	}
}

func TestCrossPlatformCoverageFloatImageUploadDeadlineDocumented(t *testing.T) {
	client := newFloatImageUploadClient()
	if client.Timeout != 5*time.Minute || client.CheckRedirect == nil {
		t.Fatalf("upload client = %#v", client)
	}
}
