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

package output

// Phase I：信封渲染字节稳定性（B188；AC-07）。落盘策略：轮8裁决⑩新文件。
//
// 契约卫生的可复现前提：同一信封输入，多次渲染必须逐字节一致。Go 的 map
// 迭代顺序随机、encoding/json 对 map 按键排序序列化——本测试用大 map 载荷
// 反复渲染，锁死「字节稳定」这一性质：任何引入非确定性渲染（如依赖 map
// 迭代序、时间戳、指针地址、随机序）的改动都会让多次渲染产生字节漂移，
// 从而命中断言。golden 比对（B157）锁的是「与基准一致」，本测试锁的是
// 「自身多次渲染一致」，两者互补。

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
)

// buildLargeMapPayload 构造一个多键 map 载荷：键数足够多，使 map 迭代顺序
// 随机性的影响面充分暴露（小 map 可能偶然同序，掩盖非确定性）。
func buildLargeMapPayload(n int) map[string]any {
	m := make(map[string]any, n)
	for i := 0; i < n; i++ {
		m[fmt.Sprintf("key_%03d", i)] = map[string]any{
			"id":     fmt.Sprintf("id_%03d", i),
			"name":   fmt.Sprintf("name_%03d", i),
			"weight": i,
			"active": i%2 == 0,
		}
	}
	return m
}

// renderEnvelopeBytes 用生产路径同一序列化函数渲染信封（两空格缩进）。
func renderEnvelopeBytes(t *testing.T, env *Envelope) []byte {
	t.Helper()
	data, err := jsonutil.MarshalIndent(env, "", "  ")
	if err != nil {
		t.Fatalf("jsonutil.MarshalIndent: %v", err)
	}
	return data
}

// TestEnvelopeRenderByteStabilityAcrossRepeatedRenders 是 B188 的核心断言：
// 同一信封反复渲染 N 次，每次输出逐字节相同（防 map 序抖动）。
func TestEnvelopeRenderByteStabilityAcrossRepeatedRenders(t *testing.T) {
	// 载荷用大 map，放大迭代序随机性；envelope 字段声明序是确定的，
	// 但 data 内的 map 若被非确定性渲染即会漂移。
	env := NewSuccessEnvelope(buildLargeMapPayload(64))
	env.Meta = &Meta{Count: NewCount(64)}

	first := renderEnvelopeBytes(t, env)
	if len(first) == 0 {
		t.Fatal("render produced empty output")
	}
	const iterations = 50
	for i := 1; i < iterations; i++ {
		got := renderEnvelopeBytes(t, env)
		if !bytes.Equal(got, first) {
			t.Fatalf("render #%d drifted byte-for-byte from render #0 (map-order jitter / nondeterminism): first diff at byte %d",
				i, firstDiffIndex(first, got))
		}
	}
}

// firstDiffIndex 返回两个字节切片首个不同位置的索引（全等返回 -1），
// 便于定位漂移起点。
func firstDiffIndex(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return n
	}
	return -1
}

// TestEnvelopeRenderByteStabilityAcrossAllFourOutcomes 把字节稳定性断言扩展到
// 四类 outcome（含携带 map 载荷的 pending/partial），确保各形态渲染均稳定。
func TestEnvelopeRenderByteStabilityAcrossAllFourOutcomes(t *testing.T) {
	envs := map[string]*Envelope{
		"success":         NewSuccessEnvelope(buildLargeMapPayload(40)),
		"pending":         newStabilityPendingFixture(),
		"partial_failure": newStabilityPartialFixture(t),
		"failure":         NewFailureEnvelope(&ErrorInfo{Type: "api", Code: 90018, Message: "rate limited"}),
	}
	for name, env := range envs {
		t.Run(name, func(t *testing.T) {
			first := renderEnvelopeBytes(t, env)
			for i := 1; i < 20; i++ {
				got := renderEnvelopeBytes(t, env)
				if !bytes.Equal(got, first) {
					t.Fatalf("%s render #%d drifted (nondeterminism): first diff at byte %d",
						name, i, firstDiffIndex(first, got))
				}
			}
		})
	}
}

func newStabilityPendingFixture() *Envelope {
	env := NewPendingEnvelope(&OperationInfo{ID: "t_stab", State: OperationStateProcessing, NextCommand: "dws op get t_stab"})
	env.Data = buildLargeMapPayload(40)
	return env
}

func newStabilityPartialFixture(t *testing.T) *Envelope {
	t.Helper()
	succeeded := make([]any, 0, 8)
	for i := 0; i < 8; i++ {
		succeeded = append(succeeded, map[string]any{
			"id":      fmt.Sprintf("s_%03d", i),
			"detail":  buildLargeMapPayload(8), // 每条 succeeded 内嵌多键 map
			"message": fmt.Sprintf("msg_%03d", i),
		})
	}
	pd, err := NewPartialData(len(succeeded)+1,
		succeeded,
		[]PartialFailedEntry{{ID: "f_001", Error: &ErrorInfo{Type: "api", Code: 40001}}},
		nil,
	)
	if err != nil {
		t.Fatalf("NewPartialData: %v", err)
	}
	return NewPartialEnvelope(pd)
}

// TestEnvelopeRenderByteStabilityIndependentOfConstructionOrder 验证字节稳定
// 不依赖构造顺序：两个字段插入顺序不同但内容等价的 map，渲染结果逐字节一致
// （encoding/json 对 map 按键排序，故构造序不影响 wire——锁死这一性质）。
func TestEnvelopeRenderByteStabilityIndependentOfConstructionOrder(t *testing.T) {
	// 正向插入。
	a := map[string]any{}
	for _, k := range []string{"alpha", "beta", "gamma", "delta"} {
		a[k] = k + "_value"
	}
	// 逆向插入（同一键集，不同插入序）。
	b := map[string]any{}
	for _, k := range []string{"delta", "gamma", "beta", "alpha"} {
		b[k] = k + "_value"
	}
	gotA := renderEnvelopeBytes(t, NewSuccessEnvelope(a))
	gotB := renderEnvelopeBytes(t, NewSuccessEnvelope(b))
	if !bytes.Equal(gotA, gotB) {
		t.Fatalf("construction order leaked into wire bytes (map keys must be sorted on marshal):\n--- a ---\n%s\n--- b ---\n%s", gotA, gotB)
	}
}
