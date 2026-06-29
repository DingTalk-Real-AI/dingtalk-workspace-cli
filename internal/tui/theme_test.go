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

package tui

import "testing"

func TestPlainRuneWidthSkipsANSIAndCountsCJK(t *testing.T) {
	got := PlainRuneWidth("\x1b[34m钉钉\x1b[0m CLI")
	if got != 8 {
		t.Fatalf("PlainRuneWidth() = %d, want 8", got)
	}
}

func TestPlainRuneWidthCountsFullwidthHangulAndEmoji(t *testing.T) {
	got := PlainRuneWidth("Ａ한🟦")
	if got != 6 {
		t.Fatalf("PlainRuneWidth() = %d, want 6", got)
	}
}

func TestPadRightANSIUsesVisibleWidth(t *testing.T) {
	got := PadRightANSI("\x1b[34m钉钉\x1b[0m", 6)
	if PlainRuneWidth(got) != 6 {
		t.Fatalf("visible width = %d, want 6", PlainRuneWidth(got))
	}
}
