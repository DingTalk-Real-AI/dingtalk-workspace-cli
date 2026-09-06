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

package app

import (
	"context"
	"os"
	"testing"
)

// BenchmarkNewRootCommand measures building the real Cobra tree — roughly 800
// leaves with their flags, Schema annotations and PostMount work.
//
// It is the other half of the cold-start attribution: `dws version` never
// decodes the Schema catalog, so whatever it spends beyond process start is
// mostly this. Without splitting the two, an optimization aimed at JSON parsing
// could target the smaller cost.
func BenchmarkNewRootCommand(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		root := NewRootCommand()
		if root == nil {
			b.Fatal("nil root")
		}
	}
}

func BenchmarkProcessRootConstruction(b *testing.B) {
	b.Setenv("DWS_CONFIG_DIR", b.TempDir())
	b.Setenv("DO_NOT_TRACK", "1")
	for _, benchmark := range []struct {
		name string
		args []string
	}{
		{name: "root-help", args: []string{"dws", "--help"}},
		{name: "calendar-list", args: []string{"dws", "calendar", "book", "list", "--dry-run", "-f", "json"}},
		{name: "config-get", args: []string{"dws", "config", "get", "output"}},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			original := os.Args
			os.Args = benchmark.args
			b.Cleanup(func() { os.Args = original })
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				root := newProcessRootCommandWithEngine(context.Background(), nil)
				if root == nil {
					b.Fatal("nil root")
				}
			}
		})
	}
}
