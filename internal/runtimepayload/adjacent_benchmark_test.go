package runtimepayload

import (
	"runtime"
	"testing"
)

// Use the shipped payload so the benchmark includes realistic decompression,
// filesystem and checksum costs without initializing the native SDK.
func BenchmarkRuntimePayloadWarm(b *testing.B) {
	if _, err := LibraryName(runtime.GOOS, runtime.GOARCH); err != nil {
		b.Skip(err)
	}
	for _, method := range []struct {
		name string
		run  func([]byte, string, string, string) (string, error)
	}{
		{"Adjacent", MaterializeAdjacent},
		{"Cache", Materialize},
	} {
		b.Run(method.name, func(b *testing.B) {
			root := b.TempDir()
			container := Embedded()
			if _, err := method.run(container, root, runtime.GOOS, runtime.GOARCH); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := method.run(container, root, runtime.GOOS, runtime.GOARCH); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
