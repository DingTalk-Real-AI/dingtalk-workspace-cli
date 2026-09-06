// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package launcher

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"testing"
)

// BenchmarkCanonicalCoreHash isolates the real Go file-reading/hash stage.
// DWS_BENCH_CORE must name a finalized native core including runtime payload.
// This is a warm-file diagnostic, not a launcher latency or authenticity gate.
func BenchmarkCanonicalCoreHash(b *testing.B) {
	path := os.Getenv("DWS_BENCH_CORE")
	if path == "" {
		b.Skip("set DWS_BENCH_CORE to a finalized core")
	}
	info, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		b.Fatal("core must be a nonempty regular file")
	}
	for _, size := range []int{0, 32 << 10, 128 << 10, 1 << 20} {
		name := "production-io-copy"
		if size != 0 {
			name = fmt.Sprintf("buffer-%d", size)
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(info.Size())
			var expected [sha256.Size]byte
			for b.Loop() {
				file, err := os.Open(path)
				if err != nil {
					b.Fatal(err)
				}
				hash := sha256.New()
				var read int64
				if size == 0 {
					read, err = io.Copy(hash, file)
				} else {
					// Hide File.WriteTo so CopyBuffer actually uses the buffer.
					read, err = io.CopyBuffer(hash, struct{ io.Reader }{file}, make([]byte, size))
				}
				closeErr := file.Close()
				if err != nil || closeErr != nil || read != info.Size() {
					b.Fatalf("hash: %d bytes, read %v, close %v", read, err, closeErr)
				}
				var digest [sha256.Size]byte
				copy(digest[:], hash.Sum(nil))
				if expected != [sha256.Size]byte{} && expected != digest {
					b.Fatal("core changed during measurement")
				}
				expected = digest
			}
		})
	}
}
