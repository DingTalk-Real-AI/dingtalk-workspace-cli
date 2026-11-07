// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build !windows

package helpers

func platformEmployeeRenameBusy(error) bool {
	return false
}
