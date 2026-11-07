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

package errors

import (
	"errors"
	"slices"
	"testing"
)

func TestRecoveryActionsDoctorRouting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want []string
	}{
		{
			name: "auth keeps direct recovery and adds doctor",
			err: NewAuth("token expired",
				WithReason("auth_refresh_failed"),
				WithActions("dws auth login")),
			want: []string{"dws auth login", DoctorCommand},
		},
		{
			name: "network adds doctor",
			err:  NewAPI("dial failed", WithReason("connection_refused")),
			want: []string{DoctorCommand},
		},
		{
			name: "discovery network adds doctor",
			err:  NewDiscovery("dns failed", WithReason("dns_resolution_failed")),
			want: []string{DoctorCommand},
		},
		{
			name: "backend dependency adds doctor",
			err:  NewAPI("gateway failed", WithReason("backend_dependency_unavailable")),
			want: []string{DoctorCommand},
		},
		{
			name: "permission does not add doctor",
			err: NewAuth("permission denied",
				WithReason("http_403"),
				WithActions("检查资源权限")),
			want: []string{"检查资源权限"},
		},
		{
			name: "RPC permission reason does not add doctor",
			err:  NewAuth("denied", WithReason("rpc_forbidden")),
		},
		{
			name: "scope reason does not add doctor",
			err:  NewAuth("denied", WithReason("insufficient_scope")),
		},
		{
			name: "PAT timeout does not add doctor",
			err:  NewAuth("timed out", WithReason("pat_auth_timeout")),
		},
		{
			name: "confirmation reason does not add doctor",
			err:  NewAuth("confirm", WithReason("confirmation_required")),
		},
		{
			name: "validation does not add doctor",
			err:  NewValidation("bad flag", WithReason("unknown_flag")),
		},
		{
			name: "upstream business error does not add doctor",
			err:  NewAPI("system error", WithReason("upstream_internal_error")),
		},
		{
			name: "explicit doctor action is not duplicated by policy",
			err: NewAPI("timeout",
				WithReason("request_timeout"),
				WithActions(DoctorCommand)),
			want: []string{DoctorCommand},
		},
		{
			name: "plain error has no inferred action",
			err:  errors.New("plain"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := RecoveryActions(tt.err); !slices.Equal(got, tt.want) {
				t.Fatalf("RecoveryActions() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestHumanRecoveryActionsUsesReadableDoctorMode(t *testing.T) {
	t.Parallel()

	err := NewAuth("token expired", WithActions("dws auth login"))
	want := []string{"dws auth login", DoctorHumanCommand}
	if got := HumanRecoveryActions(err); !slices.Equal(got, want) {
		t.Fatalf("HumanRecoveryActions() = %#v, want %#v", got, want)
	}
	if got := RecoveryActions(err); !slices.Equal(got, []string{"dws auth login", DoctorCommand}) {
		t.Fatalf("RecoveryActions() = %#v, want machine doctor command", got)
	}
}
