// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"os"
	"strconv"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/launcher"
)

var (
	version    = "dev"
	commit     = "unknown"
	edition    = "open"
	coreSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	coreSize   = "0"
)

func main() {
	size, err := strconv.ParseInt(coreSize, 10, 64)
	if err != nil {
		size = -1
	}
	os.Exit(launcher.Main(launcher.Options{Version: version, Commit: commit, Edition: edition, CoreSHA256: coreSHA256, CoreSize: size}))
}
