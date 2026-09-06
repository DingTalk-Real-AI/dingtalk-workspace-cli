// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const partialIdentityHelper = "DWS_TEST_LAUNCHER_PARTIAL_IDENTITY"

func TestCrossPlatformCoveragePartialEmbeddedSchemaIdentityFailsInRealProcess(t *testing.T) {
	if os.Getenv(partialIdentityHelper) == "1" {
		version = "v1.2.3"
		commit = strings.Repeat("a", 40)
		buildTime = "2026-09-06T00:00:00Z"
		edition = "open"
		coreSHA256 = strings.Repeat("b", 64)
		coreSize = "1"
		schemaCacheEdition = "open"
		os.Args = []string{"dws", "--version"}
		main()
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestCrossPlatformCoveragePartialEmbeddedSchemaIdentityFailsInRealProcess$", "-test.count=1")
	command.Env = append(os.Environ(), partialIdentityHelper+"=1", "DO_NOT_TRACK=1")
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !strings.Contains(string(output), "invalid_schema_identity") ||
		!strings.Contains(string(output), "partial Schema cache identity") ||
		!strings.Contains(string(output), "configuration validate release identity") ||
		!errors.As(err, &exit) || exit.ExitCode() != 125 {
		t.Fatalf("partial launcher result: err=%v output=%q", err, output)
	}
}
