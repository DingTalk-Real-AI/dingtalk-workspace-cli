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

package helpers

import "testing"

func TestValidateNaming(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		vendor  string
		extName string
		wantErr bool
	}{
		{name: "valid", vendor: "dingtalk", extName: "oa-plus"},
		{name: "short-vendor", vendor: "d", extName: "oa-plus", wantErr: true},
		{name: "invalid-vendor", vendor: "DingTalk", extName: "oa-plus", wantErr: true},
		{name: "invalid-name", vendor: "dingtalk", extName: "oa_plus", wantErr: true},
		{name: "empty-name", vendor: "dingtalk", extName: "", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateNaming(tc.vendor, tc.extName)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateNaming(%q, %q) error = nil, want failure", tc.vendor, tc.extName)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateNaming(%q, %q) error = %v, want nil", tc.vendor, tc.extName, err)
			}
		})
	}
}
