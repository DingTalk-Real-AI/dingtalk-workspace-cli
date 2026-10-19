// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

const oidcTTYChildEnv = "DWS_TEST_OIDC_STDIN_TTY_CHILD"

func TestCrossPlatformCoverageSplitOIDCStdinTTY(t *testing.T) {
	if mode := os.Getenv(oidcTTYChildEnv); mode != "" {
		runOIDCStdinTTYChild(t, mode)
		return
	}
	for _, source := range []string{"pty", "named-tty", "redirected", "redirected-high-fd"} {
		for _, ending := range []string{"cancel", "deadline"} {
			mode := source + "/" + ending
			t.Run(mode, func(t *testing.T) {
				if source == "redirected-high-fd" {
					var limit unix.Rlimit
					if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
						t.Fatal(err)
					}
					if limit.Cur <= unix.FD_SETSIZE {
						t.Skip("descriptor limit cannot accommodate the high-FD case")
					}
				}
				// A subprocess with a controlling terminal is necessary: opening
				// /dev/tty is different from reading the PTY's /dev/ttysNNN device.
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				ready, notify, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer ready.Close()
				defer notify.Close()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCrossPlatformCoverageSplitOIDCStdinTTY$", "-test.count=1")
				cmd.Env = append(os.Environ(), oidcTTYChildEnv+"="+mode)
				cmd.ExtraFiles = []*os.File{notify}
				var out bytes.Buffer
				cmd.Stdout, cmd.Stderr = &out, &out
				terminal, err := pty.Start(cmd)
				if err != nil {
					t.Fatal(err)
				}
				defer terminal.Close()
				_ = notify.Close()
				// The child signals only after its canceled read has returned.
				// CommandContext kills a stuck child and closes this pipe, so a
				// regression cannot hang the parent test or leak a blocked reader.
				var signal [1]byte
				_, readyErr := io.ReadFull(ready, signal[:])
				var writeErr error
				if readyErr == nil {
					_, writeErr = terminal.Write([]byte("next\n\x04"))
				}
				if readyErr != nil || writeErr != nil {
					cancel()
				}
				waitErr := cmd.Wait()
				if readyErr != nil || writeErr != nil || waitErr != nil {
					t.Fatalf("terminal read failed: ready=%v write=%v process=%v\n%s", readyErr, writeErr, waitErr, out.String())
				}
			})
		}
	}
}

func runOIDCStdinTTYChild(t *testing.T, mode string) {
	t.Helper()
	input := os.Stdin
	if !strings.HasPrefix(mode, "pty/") {
		file, err := os.Open("/dev/tty")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		input = file
		if strings.HasPrefix(mode, "redirected") {
			minimumFD := 0
			if strings.HasPrefix(mode, "redirected-high-fd/") {
				minimumFD = unix.FD_SETSIZE
			}
			fd, err := unix.FcntlInt(file.Fd(), unix.F_DUPFD_CLOEXEC, minimumFD)
			if err != nil {
				t.Fatal(err)
			}
			// Shell redirection preserves the /dev/tty device but Go names
			// inherited stdin /dev/stdin, hiding the dependency's name check.
			input = os.NewFile(uintptr(fd), "/dev/stdin")
			defer input.Close()
		}
	}
	before, err := unix.IoctlGetTermios(int(input.Fd()), unix.TIOCGETA)
	if err != nil {
		t.Fatal(err)
	}
	// The parent supplies input without consuming terminal output. Disable
	// echo in this test fixture so macOS does not wait for it to drain when
	// the child exits; canonical input and Ctrl-D semantics stay enabled.
	before.Lflag &^= unix.ECHO
	if err := unix.IoctlSetTermios(int(input.Fd()), unix.TIOCSETA, before); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	want := context.Canceled
	if strings.HasSuffix(mode, "/deadline") {
		cancel()
		ctx, cancel = context.WithTimeout(t.Context(), 200*time.Millisecond)
		want = context.DeadlineExceeded
	} else {
		timer := time.AfterFunc(200*time.Millisecond, cancel)
		defer timer.Stop()
	}
	defer cancel()
	if _, err := readOIDCCodeInput(ctx, input); !errors.Is(err, want) {
		t.Fatalf("lost cancellation identity: got %v, want %v", err, want)
	}
	after, err := unix.IoctlGetTermios(int(input.Fd()), unix.TIOCGETA)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("cancellation closed the terminal or changed its mode: %v", err)
	}
	notify := os.NewFile(3, "parent-ready")
	if _, err := notify.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	_ = notify.Close()
	// Reuse the same terminal. A leftover reader could steal this input;
	// the trailing Ctrl-D also verifies canonical EOF remains intact.
	nextCtx, nextCancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer nextCancel()
	got, err := readOIDCCodeInput(nextCtx, input)
	if err != nil || string(got) != "next\n" {
		t.Fatalf("terminal cannot be reused after cancellation: got %q, err %v", got, err)
	}
}
