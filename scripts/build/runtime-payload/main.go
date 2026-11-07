// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtimepayload"
)

var exitProcess = os.Exit

func main() {
	exitProcess(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		usage(stderr)
		return 2
	}
	var err error
	switch args[0] {
	case "generate":
		if len(args) != 4 {
			usage(stderr)
			return 2
		}
		capacity, parseErr := strconv.Atoi(args[3])
		if parseErr != nil {
			err = fmt.Errorf("invalid capacity: %w", parseErr)
		} else {
			err = runtimepayload.WriteContainer(args[1], args[2], capacity)
		}
	case "inject":
		if len(args) != 3 {
			usage(stderr)
			return 2
		}
		err = runtimepayload.InjectFile(args[1], args[2])
	case "materialize":
		if len(args) != 5 {
			usage(stderr)
			return 2
		}
		var path string
		path, err = runtimepayload.MaterializeFile(args[1], args[2], args[3], args[4])
		if err == nil {
			fmt.Fprintln(stdout, path)
		}
	default:
		usage(stderr)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "runtime payload: %v\n", err)
		return 1
	}
	return 0
}

func usage(stderr io.Writer) {
	fmt.Fprintln(stderr, "usage: runtime-payload generate <output> <payload-root> <capacity>")
	fmt.Fprintln(stderr, "       runtime-payload inject <dws-binary> <payload-root>")
	fmt.Fprintln(stderr, "       runtime-payload materialize <dws-binary> <cache-root> <goos> <goarch>")
}
