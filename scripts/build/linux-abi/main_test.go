// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"bytes"
	"debug/elf"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageLinuxABIReleaseLibraries(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			path := filepath.Join("..", "..", "..", "third_party", "runtimepayload", "20260825", "linux", arch, "libx7k2m9p4q1w8.so")
			var stderr bytes.Buffer
			if code := run([]string{path}, &stderr); code != 0 {
				t.Fatalf("bundled library rejected: %d, %s", code, &stderr)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(data, []byte("GLIBC_2.17")) {
				t.Fatal("fixture must contain the baseline symbol version")
			}
			// Preserve the ELF layout and version table offsets while raising the
			// actual version requirement, as a newer linker/sysroot would do.
			data = bytes.ReplaceAll(data, []byte("GLIBC_2.17"), []byte("GLIBC_2.18"))
			incompatible := filepath.Join(t.TempDir(), "newer-glibc.so")
			if err := os.WriteFile(incompatible, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if code := run([]string{path, incompatible}, &stderr); code != 1 || !strings.Contains(stderr.String(), "requires GLIBC_2.18") {
				t.Fatalf("newer glibc requirement not rejected: %d, %s", code, &stderr)
			}
			file, err := elf.NewFile(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			// Break the dynamic-symbol string-table reference while keeping a
			// readable ELF header. A malformed symbol table must fail closed.
			sectionOffset := file.ByteOrder.Uint64(data[40:48])
			for i, section := range file.Sections {
				if section.Name == ".dynsym" {
					file.ByteOrder.PutUint32(data[sectionOffset+uint64(i)*64+40:], ^uint32(0))
				}
			}
			if err := os.WriteFile(incompatible, data, 0o600); err != nil {
				t.Fatal(err)
			}
			stderr.Reset()
			if code := run([]string{incompatible}, &stderr); code != 1 || !strings.Contains(stderr.String(), "string table") {
				t.Fatalf("invalid symbol table not rejected: %d, %s", code, &stderr)
			}
		})
	}
}

func TestCrossPlatformCoverageLinuxABIVersions(t *testing.T) {
	for _, version := range []string{"GLIBC_2.2.5", "GLIBC_2.14", "GLIBC_2.17", "GLIBC_2.17.0"} {
		if !compatibleGLIBC(version) {
			t.Errorf("compatible version %s rejected", version)
		}
	}
	for _, version := range []string{"GLIBC_2.17.1", "GLIBC_2.18", "GLIBC_2.34", "GLIBC_3.0", "GLIBC_PRIVATE", "GLIBC_ABI_DT_RELR", "GLIBC_2.-1", "GLIBC_2.17.0.1"} {
		if compatibleGLIBC(version) {
			t.Errorf("incompatible or unknown version %s accepted", version)
		}
	}
}

func TestCrossPlatformCoverageLinuxABIMainAndInvalidFiles(t *testing.T) {
	testseam.Swap(t, &os.Args, []string{"linux-abi"})
	code := -1
	testseam.Swap(t, &exitProcess, func(value int) { code = value })
	main()
	if code != 2 {
		t.Fatalf("missing arguments exit = %d", code)
	}
	var stderr bytes.Buffer
	if code := run([]string{filepath.Join(t.TempDir(), "missing")}, &stderr); code != 1 {
		t.Fatalf("missing ELF exit = %d", code)
	}
}
