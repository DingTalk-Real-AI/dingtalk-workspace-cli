// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestMultiHandlerWritesToBoth(t *testing.T) {
	t.Parallel()

	var buf1, buf2 bytes.Buffer
	h1 := slog.NewTextHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelDebug})
	h2 := slog.NewJSONHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelDebug})

	logger := slog.New(NewMultiHandler(h1, h2))
	logger.Info("hello", "key", "value")

	if !strings.Contains(buf1.String(), "hello") {
		t.Errorf("handler1 missing message: %s", buf1.String())
	}
	if !strings.Contains(buf2.String(), "hello") {
		t.Errorf("handler2 missing message: %s", buf2.String())
	}
}

func TestMultiHandlerRespectsIndividualLevels(t *testing.T) {
	t.Parallel()

	var debugBuf, warnBuf bytes.Buffer
	debugHandler := slog.NewTextHandler(&debugBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
	warnHandler := slog.NewTextHandler(&warnBuf, &slog.HandlerOptions{Level: slog.LevelWarn})

	logger := slog.New(NewMultiHandler(debugHandler, warnHandler))

	logger.Debug("debug msg")
	logger.Warn("warn msg")

	// debugBuf should have both
	if !strings.Contains(debugBuf.String(), "debug msg") {
		t.Error("debug handler should capture debug msg")
	}
	if !strings.Contains(debugBuf.String(), "warn msg") {
		t.Error("debug handler should capture warn msg")
	}

	// warnBuf should only have warn
	if strings.Contains(warnBuf.String(), "debug msg") {
		t.Error("warn handler should NOT capture debug msg")
	}
	if !strings.Contains(warnBuf.String(), "warn msg") {
		t.Error("warn handler should capture warn msg")
	}
}

func TestMultiHandlerEnabled(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	warnHandler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	debugHandler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	mh := NewMultiHandler(warnHandler, debugHandler)

	// Should be enabled at DEBUG because at least one handler accepts it
	if !mh.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("should be enabled at DEBUG when one handler accepts it")
	}
}

func TestMultiHandlerWithAttrs(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	mh := NewMultiHandler(h)

	logger := slog.New(mh.WithAttrs([]slog.Attr{slog.String("component", "test")}))
	logger.Info("with attrs")

	if !strings.Contains(buf.String(), "component") {
		t.Error("WithAttrs not propagated")
	}
}

func TestMultiHandlerWithGroup(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	mh := NewMultiHandler(h)

	logger := slog.New(mh.WithGroup("grp"))
	logger.Info("grouped", "k", "v")

	if !strings.Contains(buf.String(), "grp") {
		t.Error("WithGroup not propagated")
	}
}
