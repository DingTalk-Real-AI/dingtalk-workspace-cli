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

package executor

import (
	"context"
	"testing"
)

func TestCrossPlatformCoverageInvocationSkipExecutionDuringDryRun(t *testing.T) {
	tests := []struct {
		name       string
		invocation Invocation
		want       bool
	}{
		{name: "normal invocation", invocation: Invocation{}, want: false},
		{name: "ordinary dry run", invocation: Invocation{DryRun: true}, want: true},
		{
			name: "explicit read-only dry run",
			invocation: Invocation{
				DryRun:                    true,
				AllowReadOnlyDuringDryRun: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.invocation.SkipExecutionDuringDryRun(); got != tt.want {
				t.Fatalf("SkipExecutionDuringDryRun() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestCrossPlatformCoverageEchoRunnerHonorsReadOnlyDryRunCapability(t *testing.T) {
	params := map[string]any{"dryRun": true}
	ordinary := Invocation{DryRun: true, Tool: "pat.batch_plan", Params: params}
	result, err := (EchoRunner{}).Run(context.Background(), ordinary)
	if err != nil {
		t.Fatalf("EchoRunner.Run(ordinary dry-run) error = %v", err)
	}
	if got := result.Response["dry_run"]; got != true {
		t.Fatalf("ordinary dry-run response = %#v, want dry_run=true", result.Response)
	}

	readOnly := ordinary
	readOnly.AllowReadOnlyDuringDryRun = true
	result, err = (EchoRunner{}).Run(context.Background(), readOnly)
	if err != nil {
		t.Fatalf("EchoRunner.Run(read-only dry-run) error = %v", err)
	}
	if result.Response != nil {
		t.Fatalf("read-only dry-run response = %#v, want execution to remain available", result.Response)
	}
}
