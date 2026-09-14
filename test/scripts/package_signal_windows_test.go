//go:build windows

package scripts_test

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureIsolatedProcessGroup(*exec.Cmd) {}

func processGroupID(int) (int, error) {
	return 0, errors.New("POSIX process groups are unavailable on Windows")
}

func signalProcessGroup(int, syscall.Signal) error {
	return errors.New("POSIX process groups are unavailable on Windows")
}

func startWithPTY(*exec.Cmd) (*os.File, error) {
	return nil, errors.New("POSIX pseudo terminals are unavailable on Windows")
}
