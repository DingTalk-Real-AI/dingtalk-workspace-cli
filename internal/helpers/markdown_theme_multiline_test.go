// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"gopkg.in/yaml.v3"
)

func TestCrossPlatformCoverageMarkdownThemeMultilineMetadata(t *testing.T) {
	for _, tc := range []struct{ name, payload, want string }{
		{"double-quoted-title", "title: \"first\nx-we-markdown-theme: songyan\nlast\"\nother: keep\n", "title: \"first\nx-we-markdown-theme: songyan\nlast\"\nother: keep\nx-we-markdown-theme: qingya\n"},
		{"single-quoted-title", "title: 'first\nx-we-markdown-theme: songyan\nlast'\nother: keep\n", "title: 'first\nx-we-markdown-theme: songyan\nlast'\nother: keep\nx-we-markdown-theme: qingya\n"},
		{"real-theme-before", "x-we-markdown-theme: default\ntitle: \"first\nx-we-markdown-theme: songyan\nlast\"\n", "x-we-markdown-theme: qingya\ntitle: \"first\nx-we-markdown-theme: songyan\nlast\"\n"},
		{"real-theme-after", "title: \"first\nx-we-markdown-theme: songyan\nlast\"\nx-we-markdown-theme: default\n", "title: \"first\nx-we-markdown-theme: songyan\nlast\"\nx-we-markdown-theme: qingya\n"},
		{"duplicate-real-keys", "x-we-markdown-theme: default\ntitle: \"first\nx-we-markdown-theme: songyan\nlast\"\n'x-we-markdown-theme': juxia\n", "x-we-markdown-theme: qingya\ntitle: \"first\nx-we-markdown-theme: songyan\nlast\"\n"},
		{"other-literal", "title: |\n  x-we-markdown-theme: songyan\nother: keep\n", "title: |\n  x-we-markdown-theme: songyan\nother: keep\nx-we-markdown-theme: qingya\n"},
		{"other-folded", "title: >\n  x-we-markdown-theme: songyan\nother: keep\n", "title: >\n  x-we-markdown-theme: songyan\nother: keep\nx-we-markdown-theme: qingya\n"},
		{"owned-literal", "x-we-markdown-theme: |\n  songyan\ntitle: Keep\n", "x-we-markdown-theme: qingya\ntitle: Keep\n"},
		{"owned-folded", "x-we-markdown-theme: >-\n  old\n  theme\n# keep\ntitle: Keep\n", "x-we-markdown-theme: qingya\n# keep\ntitle: Keep\n"},
		{"owned-multiline-quoted", "x-we-markdown-theme: \"old\n  theme\"\ntitle: Keep\n", "x-we-markdown-theme: qingya\ntitle: Keep\n"},
		{"malformed-owned-fallback", "x-we-markdown-theme: \"unterminated\ntitle: Keep\n", "x-we-markdown-theme: qingya\ntitle: Keep\n"},
		{"quoted-crlf", "title: \"first\r\nx-we-markdown-theme: songyan\r\nlast\"\r\nother: keep\r\n", "title: \"first\r\nx-we-markdown-theme: songyan\r\nlast\"\r\nother: keep\r\nx-we-markdown-theme: qingya\n"},
		{"ordinary-root", "title: Keep\nx-we-markdown-theme: songyan\n", "title: Keep\nx-we-markdown-theme: qingya\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := "---\n" + tc.payload + "---\n# Body\n"
			want := "---\n" + tc.want + "---\n# Body\n"
			got := applyMarkdownTheme([]byte(input), "qingya")
			if string(got) != want {
				t.Fatalf("themed body=%q want=%q", got, want)
			}
			if again := applyMarkdownTheme(got, "qingya"); !bytes.Equal(again, got) {
				t.Fatal("theme update is not idempotent")
			}
			var metadata map[string]any
			if err := yaml.Unmarshal([]byte(tc.want), &metadata); err != nil {
				t.Fatal(err)
			}
			if metadata["x-we-markdown-theme"] != "qingya" {
				t.Fatal("expected output must contain actual root theme")
			}
		})
	}
}

func TestCrossPlatformCoverageMarkdownThemeMultilineUploadBytes(t *testing.T) {
	source := "---\ntitle: \"first\nx-we-markdown-theme: songyan\nlast\"\nother: keep\n---\n# Body\n"
	want := "---\ntitle: \"first\nx-we-markdown-theme: songyan\nlast\"\nother: keep\nx-we-markdown-theme: qingya\n---\n# Body\n"
	caller := &markdownDriveCaller{format: "json", steps: []markdownDriveStep{{text: `{"uploadId":"drive-key","resourceUrls":[{"url":"https://upload.test/drive"}]}`}, {text: `{"created":true}`}}}
	installMarkdownDriveDeps(t, caller)
	sourcePath := writeMarkdownDriveFixture(t, "input.md", source)
	calls := 0
	testseam.Swap(t, &httpPutFile, func(_ context.Context, _ string, _ map[string]string, path string, size int64) error {
		calls++
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(raw) != want || size != int64(len(want)) {
			return fmt.Errorf("actual upload bytes=%q size=%d want=%q size=%d", raw, size, want, len(want))
		}
		return nil
	})
	if err := executeMarkdownDriveCommand(t, newMarkdownCommand(), nil, "markdown", "create", "--file", sourcePath, "--space-id", "space-1", "--theme", "qingya"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("actual upload boundary calls=%d", calls)
	}
	raw, err := os.ReadFile(sourcePath)
	if err != nil || string(raw) != source {
		t.Fatalf("user source changed %q: %v", raw, err)
	}
}

func TestCrossPlatformCoverageMarkdownThemeNonAlignedYAMLFallback(t *testing.T) {
	for _, tc := range []struct{ name, separator string }{
		{"NEL", "\u0085"}, {"LS", "\u2028"}, {"PS", "\u2029"}, {"CR", "\r"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := "title: \"first" + tc.separator + "x-we-markdown-theme: songyan" + tc.separator + "last\"\nother: keep\n"
			input := "---\n" + payload + "---\n# Body\n"
			want := "---\n" + payload + "x-we-markdown-theme: qingya\n---\n# Body\n"
			got := applyMarkdownTheme([]byte(input), "qingya")
			if string(got) != want {
				t.Fatalf("legacy nonaligned metadata=%q want=%q", got, want)
			}
			if again := applyMarkdownTheme(got, "qingya"); !bytes.Equal(again, got) {
				t.Fatal("fallback is not idempotent")
			}
			var metadata map[string]any
			if err := yaml.Unmarshal([]byte(payload+"x-we-markdown-theme: qingya\n"), &metadata); err != nil {
				t.Fatal(err)
			}
			if metadata["x-we-markdown-theme"] != "qingya" {
				t.Fatal("fallback expected an actual root theme")
			}
		})
	}
}

func TestCrossPlatformCoverageMarkdownThemeComplexKeysRetained(t *testing.T) {
	payload := "? [label, category]\n: value\ntitle: Keep\nx-we-markdown-theme: songyan\n"
	wantPayload := "? [label, category]\n: value\ntitle: Keep\nx-we-markdown-theme: qingya\n"
	input := "---\n" + payload + "---\n# Body\n"
	want := "---\n" + wantPayload + "---\n# Body\n"
	got := applyMarkdownTheme([]byte(input), "qingya")
	if string(got) != want {
		t.Fatalf("complex YAML key changed: %q want=%q", got, want)
	}
	if again := applyMarkdownTheme(got, "qingya"); !bytes.Equal(again, got) {
		t.Fatal("complex-key update is not idempotent")
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(wantPayload), &document); err != nil {
		t.Fatal(err)
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode || root.Content[0].Kind != yaml.SequenceNode {
		t.Fatal("unrelated complex key was not retained")
	}
}
