// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// linux-abi verifies release ELF files without executing foreign binaries.
package main

import (
	"debug/elf"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

var exitProcess = os.Exit

func main() {
	exitProcess(run(os.Args[1:], os.Stderr))
}

func run(paths []string, stderr io.Writer) int {
	if len(paths) == 0 {
		fmt.Fprintln(stderr, "usage: linux-abi <ELF file>...")
		return 2
	}
	for _, path := range paths {
		if err := verify(path); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", path, err)
			return 1
		}
	}
	return 0
}

func verify(path string) error {
	file, err := elf.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	symbols, err := file.ImportedSymbols()
	if err != nil {
		return err
	}
	for _, symbol := range symbols {
		if strings.HasPrefix(symbol.Version, "GLIBC_") && !compatibleGLIBC(symbol.Version) {
			return fmt.Errorf("%s requires %s; release baseline is GLIBC_2.17", symbol.Name, symbol.Version)
		}
	}
	return nil
}

func compatibleGLIBC(version string) bool {
	parts := strings.Split(strings.TrimPrefix(version, "GLIBC_"), ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	var parsed [3]int
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return false
		}
		parsed[i] = value
	}
	// Match the oldest glibc required by the bundled Linux runtime libraries.
	baseline := [3]int{2, 17, 0}
	for i := range baseline {
		if parsed[i] != baseline[i] {
			return parsed[i] < baseline[i]
		}
	}
	return true
}
