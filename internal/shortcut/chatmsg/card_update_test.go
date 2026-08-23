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

package chatmsg

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestCrossPlatformCoverageNormalizeCardBizID(t *testing.T) {
	for _, test := range []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "opaque id", raw: "  card-token-1  ", want: "card-token-1"},
		{name: "opaque unicode remains server owned", raw: "中文乱串", want: "中文乱串"},
		{name: "empty", raw: "  ", wantErr: true},
		{name: "placeholder", raw: "<bizId>", wantErr: true},
		{name: "internal space", raw: "card token", wantErr: true},
		{name: "control", raw: "card\ntoken", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeCardBizID(test.raw)
			if test.wantErr {
				if err == nil {
					t.Fatalf("NormalizeCardBizID(%q) unexpectedly succeeded", test.raw)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("NormalizeCardBizID(%q) = %q, %v; want %q", test.raw, got, err, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageVerifyStreamingCardUpdate(t *testing.T) {
	for _, test := range []struct {
		name      string
		response  map[string]any
		want      CardUpdateVerification
		wantErrIs error
	}{
		{name: "updated", response: map[string]any{"result": map[string]any{"updated": true}}, want: CardUpdateVerification{Accepted: true, Verified: true, Evidence: "updated=true"}},
		{name: "affected", response: map[string]any{"data": map[string]any{"affectedCount": float64(1)}}, want: CardUpdateVerification{Accepted: true, Verified: true, Evidence: "affectedCount=1"}},
		{name: "boolean result", response: map[string]any{"result": true}, want: CardUpdateVerification{Accepted: true, Verified: true, Evidence: "result=true"}},
		{name: "boolean false result", response: map[string]any{"result": false}, wantErrIs: ErrCardUpdateNotApplied},
		{name: "matching id", response: map[string]any{"result": map[string]any{"bizId": "biz-1", "applied": true}}, want: CardUpdateVerification{Accepted: true, Verified: true, Evidence: "applied=true"}},
		{name: "conflicting evidence", response: map[string]any{"updated": true, "applied": false}, wantErrIs: ErrCardUpdateUnverified},
		{name: "zero affected", response: map[string]any{"affectedCount": 0}, wantErrIs: ErrCardUpdateNotApplied},
		{name: "success acknowledgement", response: map[string]any{"success": true, "errorCode": nil}, want: CardUpdateVerification{Accepted: true, Verified: false, Evidence: "success=true"}},
		{name: "success acknowledgement with empty error code", response: map[string]any{"success": true, "errorCode": "  "}, want: CardUpdateVerification{Accepted: true, Verified: false, Evidence: "success=true"}},
		{name: "success without explicit error code", response: map[string]any{"success": true}, want: CardUpdateVerification{Accepted: true, Verified: false, Evidence: "success=true"}},
		{name: "success conflicts with error code", response: map[string]any{"success": true, "errorCode": "InternalError"}, wantErrIs: ErrCardUpdateUnverified},
		{name: "success conflicts with numeric error code", response: map[string]any{"success": true, "errorCode": float64(500)}, wantErrIs: ErrCardUpdateUnverified},
		{name: "error code without success", response: map[string]any{"errorCode": "InternalError"}, wantErrIs: ErrCardUpdateNotApplied},
		{name: "failed acknowledgement", response: map[string]any{"success": false}, wantErrIs: ErrCardUpdateNotApplied},
		{name: "explicitly not updated", response: map[string]any{"result": map[string]any{"updated": false}}, wantErrIs: ErrCardUpdateNotApplied},
		{name: "mismatched id", response: map[string]any{"result": map[string]any{"bizId": "biz-2", "updated": true}}, wantErrIs: ErrCardUpdateBizIDDrift},
		{name: "unrelated extension ignored", response: map[string]any{"extension": map[string]any{"updated": true}}, wantErrIs: ErrCardUpdateUnverified},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := VerifyStreamingCardUpdate("biz-1", test.response)
			if test.wantErrIs != nil {
				if !errors.Is(err, test.wantErrIs) {
					t.Fatalf("VerifyStreamingCardUpdate error = %v, want errors.Is(_, %v)", err, test.wantErrIs)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("VerifyStreamingCardUpdate = %#v, %v; want %#v", got, err, test.want)
			}
		})
	}
}

func TestCrossPlatformCoverageCardUpdateCountScalarVariants(t *testing.T) {
	for _, test := range []struct {
		value any
		want  int64
		ok    bool
	}{
		{value: int(1), want: 1, ok: true},
		{value: int32(2), want: 2, ok: true},
		{value: int64(3), want: 3, ok: true},
		{value: float32(4), want: 4, ok: true},
		{value: float32(4.5), want: 4, ok: false},
		{value: float64(5), want: 5, ok: true},
		{value: float64(5.5), want: 5, ok: false},
		{value: json.Number("6"), want: 6, ok: true},
		{value: json.Number("6.5"), ok: false},
		{value: "7", ok: false},
	} {
		got, ok := cardUpdateCount(test.value)
		if got != test.want || ok != test.ok {
			t.Errorf("cardUpdateCount(%#v) = (%d, %v), want (%d, %v)", test.value, got, ok, test.want, test.ok)
		}
	}
}
