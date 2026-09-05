//go:build unix

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package launcher

import (
	"errors"
	"os"
)

func coreExecutableName() string           { return "dws-core" }
func environmentKeysCaseInsensitive() bool { return false }

func validateCoreExecutable(info os.FileInfo) error {
	if info.Mode().Perm()&0o111 == 0 {
		return errors.New("core is not executable")
	}
	return nil
}
