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

package cli

import (
	"testing"
)

func TestSafetyForCLIPathKnownCommand(t *testing.T) {
	// dev app delete 是 destructive + high risk + user_required。
	s, ok := SafetyForCLIPath("dev app delete")
	if !ok {
		t.Fatal("SafetyForCLIPath(\"dev app delete\") returned ok=false; want true")
	}
	if s.Effect != "destructive" {
		t.Errorf("effect = %q, want destructive", s.Effect)
	}
	if s.Risk != "high" {
		t.Errorf("risk = %q, want high", s.Risk)
	}
	if s.Confirmation != "user_required" {
		t.Errorf("confirmation = %q, want user_required", s.Confirmation)
	}
	if !s.ShouldRender() {
		t.Error("ShouldRender() = false for destructive/high/user_required; want true")
	}
}

func TestSafetyForCLIPathUnknownSkips(t *testing.T) {
	// 不存在的命令 → ok=false。
	_, ok := SafetyForCLIPath("nonexistent fake command")
	if ok {
		t.Fatal("SafetyForCLIPath for nonexistent command returned ok=true; want false")
	}
}

func TestSafetyForCLIPathReadOnlyLowRiskSkips(t *testing.T) {
	// 找一个 read/low 风险命令,ShouldRender 应 false。
	// dev app list 是 read,默认 low risk。
	s, ok := SafetyForCLIPath("dev app list")
	if !ok {
		t.Skip("dev app list not in catalog; skipping")
	}
	if s.ShouldRender() {
		t.Errorf("ShouldRender() = true for effect=%s risk=%s; want false (read/low)", s.Effect, s.Risk)
	}
}

func TestSafetyForCLIPathTrimsWhitespace(t *testing.T) {
	// 带首尾空格的 cli_path 也能查到。
	s1, _ := SafetyForCLIPath("dev app delete")
	s2, ok2 := SafetyForCLIPath("  dev app delete  ")
	if !ok2 {
		t.Fatal("trimmed lookup failed; whitespace not trimmed")
	}
	if s1 != s2 {
		t.Errorf("whitespace-trimmed result differs: %+v vs %+v", s1, s2)
	}
}

func TestResolveMetaComplete(t *testing.T) {
	// dev app delete: destructive + high + user_required + 有 selection。
	m, ok := ResolveMeta("dev app delete")
	if !ok {
		t.Fatal("ResolveMeta(\"dev app delete\") returned ok=false")
	}
	if m.Identity.CLIPath != "dev app delete" {
		t.Errorf("CLIPath = %q", m.Identity.CLIPath)
	}
	if m.Safety.Effect != "destructive" {
		t.Errorf("Effect = %q, want destructive", m.Safety.Effect)
	}
	if len(m.Selection.UseWhen) == 0 && m.Selection.AgentSummary == "" {
		t.Error("Selection is empty; expected use_when or agent_summary")
	}
	t.Logf("dev app delete: canonical=%s safety=%+v selection(use_when=%d avoid_when=%d examples=%d)",
		m.Identity.Canonical, m.Safety, len(m.Selection.UseWhen), len(m.Selection.AvoidWhen), len(m.Selection.Examples))
}

func TestResolveMetaUnknownSkips(t *testing.T) {
	_, ok := ResolveMeta("nonexistent fake command")
	if ok {
		t.Fatal("ResolveMeta for unknown command returned ok=true")
	}
}
