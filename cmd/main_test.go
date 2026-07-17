package main

import (
	"os"
	"testing"
)

func TestCrossPlatformCoverageMainExitsWithSuccessfulVersionCommand(t *testing.T) {
	previousExit := exit
	previousArgs := os.Args
	t.Cleanup(func() {
		exit = previousExit
		os.Args = previousArgs
	})

	called := false
	code := -1
	exit = func(value int) {
		called = true
		code = value
	}
	os.Args = []string{"dws", "version"}
	main()
	if !called || code != 0 {
		t.Fatalf("main exit = called %v, code %d", called, code)
	}
}
