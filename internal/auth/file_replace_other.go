//go:build !windows

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

func platformAuthFileRenameBusy(error) bool { return false }
