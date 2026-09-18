// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard/opennodes"
)

type failedRenderReader struct{}

func (failedRenderReader) Read([]byte) (int, error) { return 0, errors.New("input failed") }

func TestCrossPlatformCoverageWhiteboardCreateDigestFailureStopsWrite(t *testing.T) {
	caller := &whiteboardTestCaller{}
	installWhiteboardTestCaller(t, caller)
	testseam.Swap(t, &whiteboardCreateSourceDigest, func(*opennodes.Source) (string, error) { return "", errors.New("digest failed") })
	_, err := callStandaloneWhiteboardCreateResult(nil, "", map[string]any{"source": `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[]}`})
	if err == nil || len(caller.calls) != 0 {
		t.Fatalf("digest error=%v calls=%v", err, caller.calls)
	}
}

func TestCrossPlatformCoverageWhiteboardRenderInputLimits(t *testing.T) {
	oversized := strings.Repeat("x", maximumWhiteboardRenderSourceBytes+1)
	if _, err := readWhiteboardRenderSource("{" + oversized); err == nil {
		t.Fatal("oversized inline accepted")
	}
	testseam.Swap(t, &whiteboardRenderStdin, io.Reader(failedRenderReader{}))
	if _, err := readWhiteboardRenderSource("-"); err == nil {
		t.Fatal("stdin error lost")
	}
	testseam.Swap(t, &whiteboardRenderStdin, io.Reader(strings.NewReader(oversized)))
	if _, err := readWhiteboardRenderSource("-"); err == nil {
		t.Fatal("oversized stdin accepted")
	}
	path := writeWhiteboardFixture(t, oversized)
	if _, err := readWhiteboardRenderSource("@" + path); err == nil {
		t.Fatal("oversized file accepted")
	}
}

func TestCrossPlatformCoverageWhiteboardRenderOutputFailures(t *testing.T) {
	source := &opennodes.Source{Nodes: []map[string]any{{"id": "n", "type": "shape"}}}
	for _, args := range []map[string]any{
		{}, {"source": source}, {"source": source, "output": "preview.png"},
		{"source": &opennodes.Source{Nodes: make([]map[string]any, 10001)}, "output": "preview.svg"},
		{"source": &opennodes.Source{Nodes: []map[string]any{{"id": "n", "type": "shape", "extra": make(chan int)}}}, "output": "preview.svg"},
	} {
		if _, err := callWhiteboardRenderResult(nil, "", args); err == nil {
			t.Fatal("invalid render accepted")
		}
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := callWhiteboardRenderResult(nil, "", map[string]any{"source": source, "output": filepath.Join(file, "child.svg")}); err == nil {
		t.Fatal("invalid parent accepted")
	}
	target := filepath.Join(dir, "directory.svg")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := callWhiteboardRenderResult(nil, "", map[string]any{"source": source, "output": target, "force": true}); err == nil {
		t.Fatal("directory overwrite accepted")
	}
	t.Run("unresolvable working directory", func(t *testing.T) {
		testseam.Swap(t, &whiteboardRenderAbsPath, func(string) (string, error) { return "", errors.New("getwd failed") })
		if _, err := callWhiteboardRenderResult(nil, "", map[string]any{"source": source, "output": "relative.svg"}); err == nil {
			t.Fatal("missing working directory accepted")
		}
	})
}
