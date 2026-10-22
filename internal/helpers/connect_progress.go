// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"unicode/utf8"
)

// agentProgress contains only public assistant text or an opaque tool lifecycle.
// Arguments, results and reasoning must never enter this interface.
type agentProgress struct {
	ID, Kind, Text string
	Done           bool
}

type progressForwarder interface {
	forwarder
	// Callbacks are synchronous and ordered, and stop before this method returns.
	forwardProgress(context.Context, string, string, func(agentProgress), ...connectMediaAttachment) (string, error)
}

type agentProgressKey struct{}

func agentProgressSink(ctx context.Context) func(agentProgress) {
	f, _ := ctx.Value(agentProgressKey{}).(func(agentProgress))
	return f
}

// Only previews are bounded. The forwarder's final return remains the full answer.
const agentProgressPreviewBytes = 32 * 1024

func boundedPublicText(text string, limit int, suffix string) string {
	if len(text) <= limit {
		return text
	}
	end := limit
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + suffix
}
func publicProgressPreview(text string) string {
	// A preview can remain visible after cancellation or an unconfirmed write.
	// Do not promise ongoing generation or eventual full delivery in its text.
	return boundedPublicText(text, agentProgressPreviewBytes, "\n\n（当前仅展示部分正文。）")
}
