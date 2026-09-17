// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import "io"

// A snapshot reader must not block an atomic replacement by the employee worker.
// The open handle continues reading its original version after replacement.
func readEmployeeSnapshot(path string) ([]byte, error) {
	file, err := openEmployeeSnapshot(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}
