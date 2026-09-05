//go:build windows

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package launcher

import (
	"errors"
	"io"
	"os/exec"
)

func platformDelegate(path string, argv, environment []string, cwd string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	arguments := []string(nil)
	if len(argv) > 1 {
		arguments = argv[1:]
	}
	command := exec.Command(path, arguments...)
	command.Args = append([]string(nil), argv...)
	if len(command.Args) == 0 {
		command.Args = []string{path}
	}
	command.Env = environment
	command.Dir = cwd
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}
