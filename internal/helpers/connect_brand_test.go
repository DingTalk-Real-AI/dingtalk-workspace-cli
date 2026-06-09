// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import "testing"

// TestBrandReply covers the qoderwork identity rewrite using the exact replies
// captured from a real qodercli (QoderWork.app) headless run.
func TestBrandReply(t *testing.T) {
	const persona = "我是 QoderWork 助手，钉钉群里的智能助手。"
	cases := []struct {
		name    string
		channel string
		in      string
		want    string
	}{
		{
			name:    "real reply 1 — whole sentence is the self-intro",
			channel: "qoderwork",
			in:      "我是 Qoder，一个帮助你完成软件工程任务的交互式命令行工具。",
			want:    persona,
		},
		{
			name:    "real reply 2 — self-intro then capability text kept",
			channel: "qoderwork",
			in:      "我是 Qoder，一个交互式命令行工具，帮助用户完成软件工程任务。我可以协助你编写代码、调试问题。",
			want:    persona + "我可以协助你编写代码、调试问题。",
		},
		{
			name:    "real reply 3 — self-intro then newline block kept",
			channel: "qoderwork",
			in:      "我是 Qoder，一个交互式命令行工具，主要帮助用户完成软件工程相关的任务。\n\n我的核心能力包括：",
			want:    persona + "\n\n我的核心能力包括：",
		},
		{
			name:    "english self-intro",
			channel: "qoderwork",
			in:      "I am Qoder, an interactive CLI tool that helps with software engineering.",
			want:    persona,
		},
		{
			name:    "mid-text Qoder mention is NOT rewritten",
			channel: "qoderwork",
			in:      "Qoder 是一家做 AI 编程工具的公司，它的产品不错。",
			want:    "Qoder 是一家做 AI 编程工具的公司，它的产品不错。",
		},
		{
			name:    "normal answer untouched",
			channel: "qoderwork",
			in:      "1+1 等于 2。",
			want:    "1+1 等于 2。",
		},
		{
			name:    "other channels pass through even if they say Qoder",
			channel: "codebuddy",
			in:      "我是 Qoder，一个命令行工具。",
			want:    "我是 Qoder，一个命令行工具。",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := brandReply(tc.channel, tc.in); got != tc.want {
				t.Fatalf("brandReply(%q, %q)\n  got:  %q\n  want: %q", tc.channel, tc.in, got, tc.want)
			}
		})
	}
}
