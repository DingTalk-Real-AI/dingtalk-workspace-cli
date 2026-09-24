// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build !unix

package delivery

import "fmt"

func platformProcessAlive(pid int) (bool, error) {
	return false, fmt.Errorf("process liveness check is unavailable for pid %d", pid)
}
