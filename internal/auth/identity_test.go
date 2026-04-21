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

package auth

import (
	"bytes"
	"fmt"
	"regexp"
	"testing"
)

var uuidV4Regexp = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// TestGenerateUUID_FormatIsV4 verifies the primary crypto/rand path
// produces well-formed, non-colliding UUID v4 values.
func TestGenerateUUID_FormatIsV4(t *testing.T) {
	const n = 100
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		u := generateUUID()
		if !uuidV4Regexp.MatchString(u) {
			t.Fatalf("generateUUID() = %q, does not match UUID v4 regex", u)
		}
		if _, dup := seen[u]; dup {
			t.Fatalf("generateUUID() returned duplicate %q within %d draws", u, n)
		}
		seen[u] = struct{}{}
	}
}

// TestFillUUIDFromFallback_NoCollision drives the fallback path directly
// and asserts it does not collapse agents onto a shared identity even
// when crypto/rand has failed.
func TestFillUUIDFromFallback_NoCollision(t *testing.T) {
	const n = 100
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		var buf [16]byte
		fillUUIDFromFallback(buf[:])
		buf[6] = (buf[6] & 0x0f) | 0x40
		buf[8] = (buf[8] & 0x3f) | 0x80
		u := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
			buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
		seen[u] = struct{}{}
	}
	// Tolerate the astronomically small chance of a single collision
	// but refuse anything worse than that.
	if len(seen) < n-1 {
		t.Fatalf("fallback produced only %d distinct UUIDs out of %d; expected >= %d", len(seen), n, n-1)
	}
}

// TestFillUUIDFromFallback_NotAllZero guards against regressing to the
// historical "all zero / fixed constant" behaviour when crypto/rand is
// unavailable.
func TestFillUUIDFromFallback_NotAllZero(t *testing.T) {
	const legacyZeroUUID = "00000000-0000-4000-8000-000000000000"
	for i := 0; i < 10; i++ {
		var buf [16]byte
		fillUUIDFromFallback(buf[:])
		if bytes.Equal(buf[:], make([]byte, 16)) {
			t.Fatalf("fallback produced all-zero buffer on iteration %d", i)
		}
		buf[6] = (buf[6] & 0x0f) | 0x40
		buf[8] = (buf[8] & 0x3f) | 0x80
		u := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
			buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
		if u == legacyZeroUUID {
			t.Fatalf("fallback regressed to legacy constant UUID %q", u)
		}
	}
}
