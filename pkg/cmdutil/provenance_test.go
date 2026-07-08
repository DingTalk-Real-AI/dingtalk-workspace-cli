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

package cmdutil

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestEnvelopeSourceProvenance(t *testing.T) {
	t.Parallel()
	if IsEnvelopeSourced(nil) {
		t.Fatal("nil command should not be envelope sourced")
	}

	cmd := &cobra.Command{Use: "chat"}
	if IsEnvelopeSourced(cmd) {
		t.Fatal("unstamped command should not be envelope sourced")
	}

	MarkEnvelopeSource(cmd)
	if !IsEnvelopeSourced(cmd) {
		t.Fatal("stamped command should be envelope sourced")
	}
	if got := cmd.Annotations[SourceAnnotation]; got != SourceEnvelope {
		t.Fatalf("SourceAnnotation = %q, want %q", got, SourceEnvelope)
	}
}

func TestMarkEnvelopeSourceNilDoesNotPanic(t *testing.T) {
	t.Parallel()
	MarkEnvelopeSource(nil)
}
