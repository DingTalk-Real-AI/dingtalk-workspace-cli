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
		wantProof string
		wantErrIs error
	}{
		{name: "updated", response: map[string]any{"result": map[string]any{"updated": true}}, wantProof: "updated=true"},
		{name: "affected", response: map[string]any{"data": map[string]any{"affectedCount": float64(1)}}, wantProof: "affectedCount=1"},
		{name: "boolean result", response: map[string]any{"result": true}, wantProof: "result=true"},
		{name: "matching id", response: map[string]any{"result": map[string]any{"bizId": "biz-1", "applied": true}}, wantProof: "applied=true"},
		{name: "false success has no write proof", response: map[string]any{"success": true, "errorCode": nil}, wantErrIs: ErrCardUpdateUnverified},
		{name: "explicitly not updated", response: map[string]any{"result": map[string]any{"updated": false}}, wantErrIs: ErrCardUpdateNotApplied},
		{name: "mismatched id", response: map[string]any{"result": map[string]any{"bizId": "biz-2", "updated": true}}, wantErrIs: ErrCardUpdateBizIDDrift},
		{name: "unrelated extension ignored", response: map[string]any{"extension": map[string]any{"updated": true}}, wantErrIs: ErrCardUpdateUnverified},
	} {
		t.Run(test.name, func(t *testing.T) {
			proof, err := VerifyStreamingCardUpdate("biz-1", test.response)
			if test.wantErrIs != nil {
				if !errors.Is(err, test.wantErrIs) {
					t.Fatalf("VerifyStreamingCardUpdate error = %v, want errors.Is(_, %v)", err, test.wantErrIs)
				}
				return
			}
			if err != nil || proof != test.wantProof {
				t.Fatalf("VerifyStreamingCardUpdate = %q, %v; want %q", proof, err, test.wantProof)
			}
		})
	}
}
