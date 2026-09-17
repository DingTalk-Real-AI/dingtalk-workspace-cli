// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
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
	"github.com/spf13/cobra"
)

func executeMarkdownDiff(t *testing.T, args ...string) error {
	t.Helper()
	testseam.Protect(t, &os.Args)
	os.Args = append([]string{"dws", "markdown"}, args...)
	root := newMarkdownCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.Execute()
}

func TestCrossPlatformCoverageMarkdownDiffCommand(t *testing.T) {
	t.Run("oversized local file", func(t *testing.T) {
		testseam.Swap(t, &maxDiffFileSize, int64(8))
		big := filepath.Join(t.TempDir(), "big.md")
		if err := os.WriteFile(big, []byte("0123456789"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := checkFileSize(big); err == nil {
			t.Fatal("expected oversized local file")
		}
	})

	t.Run("download helpers and ensure type", func(t *testing.T) {
		testseam.Protect(t, &diffDownloadLimited)
		diffDownloadLimited = func(context.Context, string, map[string]string) ([]byte, error) {
			return []byte("left\n"), nil
		}
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"resourceUrl":"https://example.test/a","headers":{"X":"1"}}`},
		}})
		if _, err := downloadRemoteContent(context.Background(), "fid", 0); err != nil {
			t.Fatalf("download latest: %v", err)
		}
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"resourceUrl":"https://example.test/a"}`},
		}})
		if _, err := downloadRemoteContent(context.Background(), "nid", 3); err != nil {
			t.Fatalf("download version: %v", err)
		}
		diffDownloadLimited = func(context.Context, string, map[string]string) ([]byte, error) {
			return nil, errors.New("dl boom")
		}
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"resourceUrl":"https://example.test/a"}`},
		}})
		if _, err := downloadRemoteContent(context.Background(), "fid", 0); err == nil {
			t.Fatal("expected download limited error")
		}

		for _, tc := range []struct {
			ext string
			ok  bool
		}{
			{"md", true}, {"markdown", true}, {"", true},
			{"adoc", false}, {"axls", false}, {"amind", false}, {"adraw", false}, {"pdf", false},
		} {
			installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{
				{text: fmt.Sprintf(`{"result":{"name":"x","extension":%q}}`, tc.ext)},
			}})
			err := ensureMarkdownDiffType(context.Background(), "n1")
			if tc.ok && err != nil {
				t.Fatalf("ext %q: %v", tc.ext, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("ext %q: expected type error", tc.ext)
			}
		}
		for _, ext := range []string{"adoc", "axls", "amind", "adraw", "other"} {
			if describeDingTalkDocType(ext) == "" {
				t.Fatalf("describe %q empty", ext)
			}
		}
	})

	t.Run("defaultDiffDownloadLimited http paths", func(t *testing.T) {
		okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "4")
			_, _ = w.Write([]byte("data"))
		}))
		t.Cleanup(okSrv.Close)
		got, err := defaultDiffDownloadLimited(context.Background(), okSrv.URL, map[string]string{"X-Test": "1"})
		if err != nil || string(got) != "data" {
			t.Fatalf("ok download: %v %q", err, got)
		}

		badStatus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("nope"))
		}))
		t.Cleanup(badStatus.Close)
		if _, err := defaultDiffDownloadLimited(context.Background(), badStatus.URL, nil); err == nil {
			t.Fatal("expected non-200")
		}

		testseam.Swap(t, &maxDiffFileSize, int64(3))
		tooBigHeader := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("xxxx"))
		}))
		t.Cleanup(tooBigHeader.Close)
		if _, err := defaultDiffDownloadLimited(context.Background(), tooBigHeader.URL, nil); err == nil {
			t.Fatal("expected content-length guard")
		}

		chunkedBig := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// No Content-Length: force LimitReader path to observe oversize body.
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "no hijack", 500)
				return
			}
			conn, bufrw, err := hj.Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			payload := strings.Repeat("x", 16)
			_, _ = bufrw.WriteString("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n")
			_, _ = bufrw.WriteString(fmt.Sprintf("%x\r\n%s\r\n0\r\n\r\n", len(payload), payload))
			_ = bufrw.Flush()
		}))
		t.Cleanup(chunkedBig.Close)
		if _, err := defaultDiffDownloadLimited(context.Background(), chunkedBig.URL, nil); err == nil {
			t.Fatal("expected body size guard")
		}

		readFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hj, ok := w.(http.Hijacker)
			if !ok {
				return
			}
			conn, bufrw, err := hj.Hijack()
			if err != nil {
				return
			}
			_, _ = bufrw.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\n")
			_ = bufrw.Flush()
			_ = conn.Close() // truncate body → ReadAll error
		}))
		t.Cleanup(readFail.Close)
		maxDiffFileSize = 1000
		if _, err := defaultDiffDownloadLimited(context.Background(), readFail.URL, nil); err == nil {
			t.Fatal("expected read error")
		}

		if _, err := defaultDiffDownloadLimited(context.Background(), "http://%\x00", nil); err == nil {
			t.Fatal("expected bad url")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := defaultDiffDownloadLimited(ctx, okSrv.URL, nil); err == nil {
			t.Fatal("expected canceled ctx")
		}
	})

	t.Run("computeUnifiedDiff deletes", func(t *testing.T) {
		text, add, del, hunks, changed := computeUnifiedDiff("a\nb\n", "a\n", 2)
		if !changed || del < 1 || hunks < 1 || add != 0 || text == "" {
			t.Fatalf("delete-only diff: changed=%v add=%d del=%d hunks=%d", changed, add, del, hunks)
		}
	})

	t.Run("validation and dry-run modes", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{dry: true})
		if err := executeMarkdownDiff(t, "diff"); err == nil {
			t.Fatal("expected missing node")
		}
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--version", "0"); err == nil {
			t.Fatal("expected version>0")
		}
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--version2", "0"); err == nil {
			t.Fatal("expected version2>0")
		}
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--context", "-1"); err == nil {
			t.Fatal("expected context>=0")
		}
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--file", "a.md", "--version2", "2"); err == nil {
			t.Fatal("expected file/version2 mutex")
		}
		if err := executeMarkdownDiff(t, "diff", "--node", "n1"); err == nil {
			t.Fatal("expected remote needs version")
		}
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--version", "2"); err != nil {
			t.Fatalf("dry-run remote: %v", err)
		}
		if err := executeMarkdownDiff(t, "diff", "--url", "n1", "--file", "local.md"); err != nil {
			t.Fatalf("dry-run local latest: %v", err)
		}
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--version", "2", "--file", "local.md"); err != nil {
			t.Fatalf("dry-run local version: %v", err)
		}
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--version2", "3"); err != nil {
			t.Fatalf("dry-run version2-only: %v", err)
		}
	})

	t.Run("remote vs local and remote vs remote execute", func(t *testing.T) {
		testseam.Protect(t, &diffDownloadLimited)
		diffDownloadLimited = func(context.Context, string, map[string]string) ([]byte, error) {
			return []byte("alpha\n"), nil
		}
		local := filepath.Join(t.TempDir(), "right.md")
		if err := os.WriteFile(local, []byte("beta\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		installScriptedCaller(t, &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
			{text: `{"extension":"md"}`},
			{text: `{"resourceUrl":"https://example.test/l"}`},
		}})
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--file", local); err != nil {
			t.Fatalf("local json: %v", err)
		}

		installScriptedCaller(t, &scriptedToolCaller{format: "text", steps: []scriptedToolStep{
			{text: `{"extension":"md"}`},
			{text: `{"resourceUrl":"https://example.test/l"}`},
			{text: `{"resourceUrl":"https://example.test/r"}`},
		}})
		diffDownloadLimited = func(_ context.Context, url string, _ map[string]string) ([]byte, error) {
			if strings.Contains(url, "/r") {
				return []byte("alpha\n"), nil
			}
			return []byte("gamma\n"), nil
		}
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--version", "1", "--version2", "2"); err != nil {
			t.Fatalf("remote text changed: %v", err)
		}

		installScriptedCaller(t, &scriptedToolCaller{format: "text", steps: []scriptedToolStep{
			{text: `{"extension":"md"}`},
			{text: `{"resourceUrl":"https://example.test/l"}`},
			{text: `{"resourceUrl":"https://example.test/r"}`},
		}})
		diffDownloadLimited = func(context.Context, string, map[string]string) ([]byte, error) {
			return []byte("same\n"), nil
		}
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--version", "1", "--version2", "2"); err != nil {
			t.Fatalf("remote text unchanged: %v", err)
		}

		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"extension":"adoc"}`},
		}})
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--version", "1"); err == nil {
			t.Fatal("expected type guard")
		}

		testseam.Protect(t, &maxDiffFileSize)
		maxDiffFileSize = 2
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"extension":"md"}`},
		}})
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--file", local); err == nil {
			t.Fatal("expected local size fail")
		}
		maxDiffFileSize = 10 * 1024 * 1024

		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"extension":"md"}`},
			{text: `{"resourceUrl":"https://example.test/l"}`},
		}})
		diffDownloadLimited = func(context.Context, string, map[string]string) ([]byte, error) {
			return []byte("x\n"), nil
		}
		missingLocal := filepath.Join(t.TempDir(), "missing.md")
		// pass size check by writing then removing after checkFileSize... actually RunE checks size first then reads.
		// Create file for size check, then make ReadFile fail via directory path.
		dirAsFile := t.TempDir()
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--file", dirAsFile); err == nil {
			t.Fatal("expected read local dir failure")
		}
		_ = missingLocal

		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"extension":"md"}`},
			{text: `{"resourceUrl":"https://example.test/l"}`},
			{err: errors.New("right fail")},
		}})
		diffDownloadLimited = func(context.Context, string, map[string]string) ([]byte, error) {
			return []byte("x\n"), nil
		}
		// second download uses version tool — make parse fail on second call via empty resource
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"extension":"md"}`},
			{text: `{"resourceUrl":"https://example.test/l"}`},
			{text: `{}`},
		}})
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--version", "1", "--version2", "2"); err == nil {
			t.Fatal("expected right download parse fail")
		}
	})

	t.Run("json marshal and compute timeout", func(t *testing.T) {
		testseam.Protect(t, &diffDownloadLimited)
		testseam.Protect(t, &diffJSONMarshalIndent)
		testseam.Protect(t, &runMarkdownUnifiedDiff)
		testseam.Protect(t, &diffComputeTimeout)
		diffDownloadLimited = func(context.Context, string, map[string]string) ([]byte, error) {
			return []byte("a\n"), nil
		}
		installScriptedCaller(t, &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
			{text: `{"extension":"md"}`},
			{text: `{"resourceUrl":"https://example.test/l"}`},
			{text: `{"resourceUrl":"https://example.test/r"}`},
		}})
		diffJSONMarshalIndent = func(any, string, string) ([]byte, error) {
			return nil, errors.New("marshal boom")
		}
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--version", "1", "--version2", "2"); err == nil || !strings.Contains(err.Error(), "JSON") {
			t.Fatalf("expected marshal err, got %v", err)
		}

		diffJSONMarshalIndent = json.MarshalIndent
		runMarkdownUnifiedDiff = func(string, string, int) (string, int, int, int, bool) {
			time.Sleep(50 * time.Millisecond)
			return "", 0, 0, 0, false
		}
		diffComputeTimeout = time.Millisecond
		installScriptedCaller(t, &scriptedToolCaller{format: "json", steps: []scriptedToolStep{
			{text: `{"extension":"md"}`},
			{text: `{"resourceUrl":"https://example.test/l"}`},
			{text: `{"resourceUrl":"https://example.test/r"}`},
		}})
		if err := executeMarkdownDiff(t, "diff", "--node", "n1", "--version", "1", "--version2", "2"); err == nil || !strings.Contains(err.Error(), "超时") {
			t.Fatalf("expected timeout, got %v", err)
		}
	})

	t.Run("fetchFileInfo extension field", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"extension":".MD","name":"x.md"}`},
		}})
		info := fetchFileInfo(context.Background(), "n")
		if info.extension != "md" || info.name != "x.md" {
			t.Fatalf("info=%+v", info)
		}
	})
}

func TestCrossPlatformCoverageMailExportShareAndAtomicWrite(t *testing.T) {
	t.Run("message export dry-run and execute", func(t *testing.T) {
		cwd := t.TempDir()
		t.Chdir(cwd)

		installScriptedCaller(t, &scriptedToolCaller{dry: true})
		if err := executePR868Command(t, newMailCommand(),
			"message", "export", "--email", "u@c.com", "--id", "m1", "--filename", "named"); err != nil {
			t.Fatalf("export dry-run: %v", err)
		}
		installScriptedCaller(t, &scriptedToolCaller{dry: true})
		if err := executePR868Command(t, newMailCommand(),
			"message", "export", "--email", "u@c.com", "--id", "m1"); err != nil {
			t.Fatalf("export dry-run default name: %v", err)
		}

		caller := &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"result":{"message":{"subject":"Hello/World"}}}`},
			{text: `{"result":{"emlContent":"From: a\r\n\r\nbody"}}`},
		}}
		installScriptedCaller(t, caller)
		if err := executePR868Command(t, newMailCommand(),
			"message", "export", "--email", "u@c.com", "--id", "m1"); err != nil {
			t.Fatalf("export: %v", err)
		}
		if _, err := os.Stat("Hello_World.eml"); err != nil {
			t.Fatalf("missing eml: %v", err)
		}

		// exist without overwrite
		caller2 := &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"emlContent":"x"}`},
		}}
		installScriptedCaller(t, caller2)
		if err := executePR868Command(t, newMailCommand(),
			"message", "export", "--email", "u@c.com", "--id", "m1", "--filename", "Hello_World"); err == nil {
			t.Fatal("expected exist error")
		}

		caller3 := &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"emlContent":"y"}`},
		}}
		installScriptedCaller(t, caller3)
		if err := executePR868Command(t, newMailCommand(),
			"message", "export", "--email", "u@c.com", "--id", "m1", "--filename", "Hello_World", "--overwrite"); err != nil {
			t.Fatalf("overwrite: %v", err)
		}

		// subject fallback to message id when missing
		caller4 := &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"result":{}}`},
			{text: `{"emlContent":"z"}`},
		}}
		installScriptedCaller(t, caller4)
		if err := executePR868Command(t, newMailCommand(),
			"message", "export", "--email", "u@c.com", "--id", "msg-fallback"); err != nil {
			t.Fatalf("fallback name: %v", err)
		}
	})

	t.Run("share-to-chat paths", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{dry: true})
		if err := executePR868Command(t, newMailCommand(),
			"message", "share-to-chat", "--email", "u@c.com", "--id", "m1", "--users", "u1"); err != nil {
			t.Fatalf("share dry-run: %v", err)
		}
		installScriptedCaller(t, &scriptedToolCaller{dry: true})
		if err := executePR868Command(t, newMailCommand(),
			"message", "share-to-chat", "--email", "u@c.com", "--id", "m1"); err != nil {
			t.Fatalf("share dry-run no users: %v", err)
		}

		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{`}}})
		if err := executePR868Command(t, newMailCommand(),
			"message", "share-to-chat", "--email", "u@c.com", "--id", "m1", "--users", "u1", "--yes"); err == nil {
			t.Fatal("expected parse error")
		}

		// Piped stdin "yes" must not bypass the explicit --yes gate.
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"result":{"sign":"sig","riskMessage":"careful"}}`},
		}})
		if err := executeMailShare(t, strings.NewReader("yes\n"),
			"message", "share-to-chat", "--email", "u@c.com", "--id", "m1", "--users", "u1"); err == nil {
			t.Fatal("expected confirmation_required without --yes")
		}
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"sign":"sig"}`},
		}})
		if err := executeMailShare(t, strings.NewReader("yes\n"),
			"message", "share-to-chat", "--email", "u@c.com", "--id", "m1", "--users", "u1"); err == nil {
			t.Fatal("expected confirmation_required without --yes")
		}

		caller := &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"sign":"sig","riskMessage":"careful"}`},
			{text: `{"ok":true}`},
		}}
		installScriptedCaller(t, caller)
		if err := executePR868Command(t, newMailCommand(),
			"message", "share-to-chat", "--email", "u@c.com", "--id", "m1", "--users", "u1", "--yes"); err != nil {
			t.Fatalf("share with yes: %v", err)
		}

		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"ok":true}`}}})
		if err := executePR868Command(t, newMailCommand(),
			"message", "share-to-chat", "--email", "u@c.com", "--id", "m1", "--users", "u1", "--yes"); err != nil {
			t.Fatalf("json success path: %v", err)
		}
	})

	t.Run("calendar-event missing folder id", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{})
		if err := executePR868Command(t, newMailCommand(),
			"calendar-event", "list",
			"--email", "u@c.com",
			"--start", "2026-07-01T00:00:00Z",
			"--end", "2026-07-31T23:59:59Z",
		); err == nil {
			t.Fatal("expected missing id/folder-id")
		}
	})

	t.Run("sanitize and atomic write", func(t *testing.T) {
		if got := sanitizeMailFilename("  a/b\\c\x00  "); got != "a_b_c" {
			t.Fatalf("sanitize=%q", got)
		}
		if got := sanitizeMailFilename("   "); got != "mail" {
			t.Fatalf("empty sanitize=%q", got)
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "out.eml")
		if err := atomicWriteFile(path, []byte("one"), 0o600, false); err != nil {
			t.Fatal(err)
		}
		if err := atomicWriteFile(path, []byte("two"), 0o600, false); err == nil {
			t.Fatal("expected exist without overwrite")
		}
		if err := atomicWriteFile(path, []byte("two"), 0o600, true); err != nil {
			t.Fatal(err)
		}
		if err := atomicWriteFile(filepath.Join(dir, "no-such", "x.eml"), []byte("x"), 0o600, false); err == nil {
			t.Fatal("expected create temp fail")
		}

		testseam.Protect(t, &atomicCreateTemp)
		testseam.Protect(t, &atomicRemove)
		testseam.Protect(t, &atomicRename)
		atomicRemove = func(string) error { return nil }
		atomicCreateTemp = func(string, string) (atomicTempFile, error) {
			return &atomicFakeTemp{chmodErr: errors.New("chmod boom")}, nil
		}
		if err := atomicWriteFile(filepath.Join(dir, "c.eml"), []byte("x"), 0o600, false); err == nil || !strings.Contains(err.Error(), "权限") {
			t.Fatalf("chmod: %v", err)
		}
		atomicCreateTemp = func(string, string) (atomicTempFile, error) {
			return &atomicFakeTemp{writeErr: errors.New("write boom")}, nil
		}
		if err := atomicWriteFile(filepath.Join(dir, "w.eml"), []byte("x"), 0o600, false); err == nil || !strings.Contains(err.Error(), "写入") {
			t.Fatalf("write: %v", err)
		}
		atomicCreateTemp = func(string, string) (atomicTempFile, error) {
			return &atomicFakeTemp{syncErr: errors.New("sync boom")}, nil
		}
		if err := atomicWriteFile(filepath.Join(dir, "s.eml"), []byte("x"), 0o600, false); err == nil || !strings.Contains(err.Error(), "同步") {
			t.Fatalf("sync: %v", err)
		}
		atomicCreateTemp = func(string, string) (atomicTempFile, error) {
			return &atomicFakeTemp{closeErr: errors.New("close boom")}, nil
		}
		if err := atomicWriteFile(filepath.Join(dir, "cl.eml"), []byte("x"), 0o600, false); err == nil || !strings.Contains(err.Error(), "关闭") {
			t.Fatalf("close: %v", err)
		}
		atomicCreateTemp = func(string, string) (atomicTempFile, error) {
			return &atomicFakeTemp{}, nil
		}
		atomicRename = func(string, string) error { return errors.New("rename boom") }
		if err := atomicWriteFile(filepath.Join(dir, "r.eml"), []byte("x"), 0o600, true); err == nil || !strings.Contains(err.Error(), "重命名") {
			t.Fatalf("rename: %v", err)
		}
	})

	t.Run("export save non-exist failure", func(t *testing.T) {
		cwd := t.TempDir()
		t.Chdir(cwd)
		testseam.Swap(t, &atomicCreateTemp, func(string, string) (atomicTempFile, error) {
			return nil, errors.New("nospc")
		})
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"emlContent":"body"}`},
		}})
		if err := executePR868Command(t, newMailCommand(),
			"message", "export", "--email", "u@c.com", "--id", "m1", "--filename", "x"); err == nil || !strings.Contains(err.Error(), "保存文件失败") {
			t.Fatalf("expected save failure, got %v", err)
		}
	})
}

func executeMailShare(t *testing.T, in io.Reader, args ...string) error {
	t.Helper()
	testseam.Protect(t, &os.Args)
	os.Args = append([]string{"dws", "mail"}, args...)
	root := newMailCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetIn(in)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.Execute()
}

func TestCrossPlatformCoverageDiffEngineEdges(t *testing.T) {
	// Expand matches backward + negative context clamp + missing newline marker.
	old := []byte("a\nb\nc\nd\ne")
	neu := []byte("a\nb\nC\nd\ne\n")
	diff := UnifiedDiff("o", old, "n", neu, -3)
	if len(diff) == 0 {
		t.Fatal("expected diff with negative context")
	}
	diff2 := UnifiedDiff("o", []byte("same"), "n", []byte("same\nextra\n"), 2)
	if !strings.Contains(string(diff2), "No newline at end of file") && !strings.Contains(string(diff2), "+extra") {
		t.Fatalf("unexpected diff2=%q", diff2)
	}
	_ = UnifiedDiff("o", []byte("1\n2\n3\n4\n5\n6\n7\n"), "n", []byte("1\n2\n3\nX\n5\n6\n7\n"), 10)
	_ = UnifiedDiff("o", []byte("only-old\n"), "n", []byte("only-new\n"), 0)

	// Non-unique "common" lines before a unique anchor → backward expand (L90).
	_ = UnifiedDiff("o", []byte("OLD\ncommon\ncommon\nUNIQUE\n"), "n", []byte("NEW\ncommon\ncommon\nUNIQUE\n"), 1)
	_ = UnifiedDiff("o", []byte("OLD\ncommon\ncommon\nUNIQUE\ntail\n"), "n", []byte("NEW\ncommon\ncommon\nUNIQUE\ntail\nextra\n"), 2)
	// Large context after an early emitted chunk → chunk.x/y clamps + new-chunk prefix.
	_ = UnifiedDiff("o", []byte("A\nU\nZ\n"), "n", []byte("B\nU\nZ\n"), 10)
	_ = UnifiedDiff("o", []byte("U\nZ\n"), "n", []byte("B\nU\nZ\n"), 10)
	_ = UnifiedDiff("o", []byte("A\nU\nZ\n"), "n", []byte("U\nZ\n"), 10)
	_ = UnifiedDiff("o", []byte("A\n\n\nU\nrest\n"), "n", []byte("B\n\n\nU\nrest\nmore\n"), 5)
	if nonNeg(-3) != 0 || nonNeg(0) != 0 || nonNeg(4) != 4 {
		t.Fatalf("nonNeg")
	}
}

func TestCrossPlatformCoverageDriveLatestRemaining(t *testing.T) {
	items := []map[string]any{
		{"name": "a", "sortTime": int64(5), "rel_path": "same", "fileId": "2", "type": "file"},
		{"name": "b", "sortTime": int64(5), "rel_path": "same", "fileId": "1", "type": "file"},
		{"name": "c", "sortTime": int64(5), "rel_path": "z", "fileId": "3", "type": "file"},
		{"nodeType": "Folder", "name": "folder"},
	}
	got := applyDriveListLatest(items, 10, false)
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	if n, err := json.Number("99").Int64(); err != nil || n != 99 {
		t.Fatal(err)
	}
	if ms, ok := toMillis(json.Number("99")); !ok || ms != 99 {
		t.Fatalf("json.Number millis=%v %v", ms, ok)
	}
	if _, ok := toMillis(json.Number("-1")); ok {
		t.Fatal("negative json.Number")
	}
	if _, ok := toMillis(struct{}{}); ok {
		t.Fatal("unknown type")
	}

	// pagination + quiet=false progress + shortfall hint with pattern
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"name":"a.md","fileId":"1","type":"file"},{"name":"x.bin","fileId":"2","type":"file"},{"fileId":"3","type":"file"}],"nextToken":"n1"}`},
		{text: `{"items":[{"name":"b.md","fileName":"b.md","fileId":"4","type":"file"}],"nextToken":""}`},
	}}
	installScriptedCaller(t, caller)
	testseam.Protect(t, &os.Args)
	os.Args = []string{"dws", "drive", "list"}
	cmd := &cobra.Command{Use: "list"}
	cmd.SetContext(context.Background())
	if err := runDriveListLatest(cmd, map[string]any{"spaceId": "s"}, "folder", 5, "*.md", false); err != nil {
		t.Fatalf("latest paginate: %v", err)
	}

	// nil context uses Background
	caller2 := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"name":"a.md","fileId":"1","type":"file"}],"nextToken":""}`},
	}}
	installScriptedCaller(t, caller2)
	cmd2 := &cobra.Command{Use: "list"}
	if err := runDriveListLatest(cmd2, nil, "", 1, "", true); err != nil {
		t.Fatalf("nil ctx: %v", err)
	}
}

func TestCrossPlatformCoverageDriveListLatestBadFolder(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true, format: "json"})
	if err := executePR868Command(t, newDriveCommand(), "list", "--latest", "2", "--folder", "12345"); err == nil {
		t.Fatal("expected numeric folder rejection")
	}
}

func TestCrossPlatformCoverageDriveDepthLatestTruncatedAndSortTime(t *testing.T) {
	useDriveDepthArgs(t)
	var sb strings.Builder
	sb.WriteString(`{"items":[`)
	for i := 0; i < driveDepthMaxItems; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"fileId":"f%d","name":"file-%d.txt","type":"FILE","modifiedTime":%d}`, i, i, 1000+i)
	}
	sb.WriteString(`]}`)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: sb.String()}}}
	out := installDepthCaller(t, caller)
	cmd := &cobra.Command{Use: "list"}
	err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 2, driveListFilter{})
	if err == nil || !strings.Contains(err.Error(), "LATEST_SCAN_TRUNCATED") {
		t.Fatalf("err=%v, want LATEST_SCAN_TRUNCATED", err)
	}
	_ = out
}

func TestCrossPlatformCoverageWhiteboardSeams(t *testing.T) {
	testseam.Swap(t, &whiteboardJSONMarshal, func(any) ([]byte, error) { return nil, errors.New("boom") })
	if buildWhiteboardCardJSONML("b", "w") != "" {
		t.Fatal("expected empty on marshal fail")
	}

	testseam.Swap(t, &prepareWhiteboardCard, func(*cobra.Command, string) (string, error) {
		return "", errors.New("bad template")
	})
	installScriptedCaller(t, &scriptedToolCaller{})
	if err := executeWhiteboardCommand(t, "insert", "--node", "doc-1", "--yes"); err == nil || !strings.Contains(err.Error(), "白板卡片模板") {
		t.Fatalf("expected prepare fail, got %v", err)
	}

	testseam.Protect(t, &os.Args)
	os.Args = []string{"dws", "doc", "whiteboard"}

	// nil entry in blocks list
	installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"blocks":[null,{"blockId":"b","jsonml":"[\"card\",{\"metadata\":{\"id\":\"w\"}}]"}]}`},
	}})
	if _, err := queryWhiteboardCardNode(context.Background(), "n", "b"); err != nil {
		t.Fatalf("nil entry skip: %v", err)
	}
}
