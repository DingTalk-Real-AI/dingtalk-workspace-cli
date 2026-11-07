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
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/muesli/cancelreader"
	"golang.org/x/sys/unix"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
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

// Unlike the controlling-/dev/tty child tests above, these reads run in the
// coverage process and reach the native isatty/select path directly.
func TestCrossPlatformCoverageSplitOIDCStdinNativeTTY(t *testing.T) {
	for _, highFD := range []bool{false, true} {
		name := "low-fd"
		if highFD {
			name = "high-fd"
		}
		t.Run(name, func(t *testing.T) {
			for _, ending := range []string{"read", "cancel", "deadline"} {
				t.Run(ending, func(t *testing.T) {
					master, input, before := openOIDCStdinNativePTY(t, highFD)
					if ending != "read" {
						ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
						want := context.Canceled
						if ending == "deadline" {
							cancel()
							ctx, cancel = context.WithTimeout(t.Context(), 200*time.Millisecond)
							want = context.DeadlineExceeded
						} else {
							timer := time.AfterFunc(200*time.Millisecond, cancel)
							defer timer.Stop()
						}
						defer cancel()
						if _, err := readOIDCCodeInput(ctx, input); !errors.Is(err, want) {
							t.Fatalf("native terminal cancellation: got %v, want %v", err, want)
						}
						assertOIDCStdinNativeTTYMode(t, input, before)
					}
					// Read a canonical line followed by Ctrl-D, also proving that a
					// canceled reader cannot steal input from the next read.
					if _, err := master.Write([]byte("next\n\x04")); err != nil {
						t.Fatal(err)
					}
					assertOIDCStdinNativeInput(t, input, "next\n")
					assertOIDCStdinNativeTTYMode(t, input, before)
				})
			}
		})
	}
}

func TestCrossPlatformCoverageSplitOIDCStdinNativeTTYDescriptorOwnership(t *testing.T) {
	for _, highFD := range []bool{false, true} {
		name := "low-fd"
		if highFD {
			name = "high-fd"
		}
		t.Run(name, func(t *testing.T) {
			master, input, before := openOIDCStdinNativePTY(t, highFD)
			reader, cancelRead, cleanup, err := newOIDCFileInput(input)
			cleaned := false
			if cleanup != nil {
				t.Cleanup(func() {
					if !cleaned {
						cleanup()
					}
				})
			}
			if err != nil {
				t.Fatal(err)
			}
			native, ok := reader.(oidcTerminalInput)
			if !ok || native.file != input || cancelRead == nil {
				t.Fatalf("PTY did not use the borrowing native reader: %T", reader)
			}
			if native.fd < 0 || native.fd >= unix.FD_SETSIZE {
				t.Fatalf("select descriptor out of range: %d", native.fd)
			}
			if highFD {
				if native.fd == int(input.Fd()) || cleanup == nil {
					t.Fatal("high-FD input did not own a separate low-FD duplicate")
				}
				flags, err := unix.FcntlInt(uintptr(native.fd), unix.F_GETFD, 0)
				if err != nil || flags&unix.FD_CLOEXEC == 0 {
					t.Fatalf("duplicate is not open and close-on-exec: flags=%d, err=%v", flags, err)
				}
			} else if native.fd != int(input.Fd()) || cleanup != nil {
				t.Fatal("low-FD input must borrow the original without cleanup")
			}
			assertOIDCStdinNativeTTYMode(t, input, before)
			completed := make(chan struct{})
			close(completed)
			cancelRead(completed)
			select {
			case <-native.canceled:
			default:
				t.Fatal("native cancellation did not close its signal")
			}
			if cleanup != nil {
				cleanup()
				cleaned = true
				if _, err := unix.FcntlInt(uintptr(native.fd), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
					t.Fatalf("cleanup left the owned duplicate open: %v", err)
				}
			}
			assertOIDCStdinNativeTTYMode(t, input, before)
			if _, err := master.Write([]byte("next\n\x04")); err != nil {
				t.Fatal(err)
			}
			assertOIDCStdinNativeInput(t, input, "next\n")
			assertOIDCStdinNativeTTYMode(t, input, before)
		})
	}
}

func TestCrossPlatformCoverageSplitOIDCStdinNativeFileInputFailures(t *testing.T) {
	t.Run("stat", func(t *testing.T) {
		input, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = writer.Close() })
		if err := input.Close(); err != nil {
			t.Fatal(err)
		}
		reader, cancelRead, cleanup, err := newOIDCFileInput(input)
		if !errors.Is(err, os.ErrClosed) || reader != nil || cancelRead != nil || cleanup != nil {
			t.Fatalf("closed input Stat: reader=%T, err=%v", reader, err)
		}
	})
	t.Run("cancelreader-construction", func(t *testing.T) {
		input, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = input.Close() })
		t.Cleanup(func() { _ = writer.Close() })
		t.Run("failure", func(t *testing.T) {
			want := errors.New("cancelreader construction failed")
			calls := 0
			testseam.Swap(t, &oidcInputCancelReader, func(got io.Reader) (cancelreader.CancelReader, error) {
				calls++
				if got != input {
					t.Fatal("cancelreader received a different input")
				}
				return nil, want
			})
			reader, cancelRead, cleanup, err := newOIDCFileInput(input)
			if !errors.Is(err, want) || calls != 1 || reader != nil || cancelRead != nil || cleanup != nil {
				t.Fatalf("cancelreader construction: calls=%d, reader=%T, err=%v", calls, reader, err)
			}
		})
		// The seam is restored before reusing the borrowed pipe.
		if _, err := writer.Write([]byte("next\n")); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		assertOIDCStdinNativeInput(t, input, "next\n")
	})
	for _, failure := range []string{"duplicate-error", "duplicate-still-high"} {
		t.Run(failure, func(t *testing.T) {
			master, input, before := openOIDCStdinNativePTY(t, true)
			t.Run("failure", func(t *testing.T) {
				want := unix.EMFILE
				duplicateFD := -1
				if failure == "duplicate-still-high" {
					var err error
					duplicateFD, err = unix.FcntlInt(input.Fd(), unix.F_DUPFD_CLOEXEC, unix.FD_SETSIZE)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						if duplicateFD >= 0 {
							_ = unix.Close(duplicateFD)
						}
					})
				}
				calls := 0
				testseam.Swap(t, &oidcInputDuplicateFD, func(fd uintptr, cmd, minimum int) (int, error) {
					calls++
					if fd != input.Fd() || cmd != unix.F_DUPFD_CLOEXEC || minimum != 0 {
						t.Fatalf("unexpected duplicate request: fd=%d, cmd=%d, minimum=%d", fd, cmd, minimum)
					}
					if failure == "duplicate-error" {
						return -1, want
					}
					return duplicateFD, nil
				})
				reader, cancelRead, cleanup, err := newOIDCFileInput(input)
				if duplicateFD >= 0 {
					if _, err := unix.FcntlInt(uintptr(duplicateFD), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
						t.Fatalf("rejected high duplicate was not closed: %v", err)
					}
					// Production closed it; do not close a potentially reused FD.
					duplicateFD = -1
				}
				if err == nil || calls != 1 || reader != nil || cancelRead != nil || cleanup != nil {
					t.Fatalf("terminal duplication: calls=%d, reader=%T, err=%v", calls, reader, err)
				}
				if failure == "duplicate-error" {
					if !errors.Is(err, want) {
						t.Fatalf("lost duplicate syscall error: %v", err)
					}
				} else if !strings.Contains(err.Error(), "no descriptor below") {
					t.Fatalf("unexpected high duplicate error: %v", err)
				}
			})
			assertOIDCStdinNativeTTYMode(t, input, before)
			if _, err := master.Write([]byte("next\n\x04")); err != nil {
				t.Fatal(err)
			}
			assertOIDCStdinNativeInput(t, input, "next\n")
			assertOIDCStdinNativeTTYMode(t, input, before)
		})
	}
}

func TestCrossPlatformCoverageSplitOIDCStdinNativeReadBoundaries(t *testing.T) {
	for _, boundary := range []string{"empty", "already-canceled", "eintr", "select-error", "cancel-after-select"} {
		t.Run(boundary, func(t *testing.T) {
			input, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = input.Close() })
			t.Cleanup(func() { _ = writer.Close() })
			if input.Fd() >= unix.FD_SETSIZE {
				t.Fatal("native Read fixture requires a descriptor below FD_SETSIZE")
			}
			if _, err := writer.Write([]byte("ready\n")); err != nil {
				t.Fatal(err)
			}
			canceled := make(chan struct{})
			stopRead := sync.OnceFunc(func() { close(canceled) })
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			stopTimeout := context.AfterFunc(ctx, stopRead)
			defer stopTimeout()
			if boundary == "empty" || boundary == "already-canceled" {
				stopRead()
			}
			calls := 0
			testseam.Swap(t, &oidcInputSelect, func(nfd int, read, write, except *unix.FdSet, timeout *unix.Timeval) (int, error) {
				calls++
				switch boundary {
				case "empty", "already-canceled", "select-error":
					return 0, unix.EIO
				case "eintr":
					if calls == 1 {
						// A syscall may mutate the set before EINTR. The retry
						// must rebuild it before making the real select call.
						*read = unix.FdSet{}
						return 0, unix.EINTR
					}
				}
				n, err := unix.Select(nfd, read, write, except, timeout)
				if boundary == "cancel-after-select" && err == nil {
					if n <= 0 || !read.IsSet(int(input.Fd())) {
						t.Fatal("cancellation fixture did not select the ready input")
					}
					stopRead()
				}
				return n, err
			})
			reader := oidcTerminalInput{file: input, fd: int(input.Fd()), canceled: canceled}
			buffer := make([]byte, 32)
			if boundary == "empty" {
				buffer = nil
			}
			n, err := reader.Read(buffer)
			if ctx.Err() != nil {
				t.Fatalf("native boundary read reached its safety deadline: %v", ctx.Err())
			}
			wantCalls, wantN := 0, 0
			var wantErr error
			remaining := "ready\n"
			switch boundary {
			case "already-canceled":
				wantErr = context.Canceled
			case "eintr":
				wantCalls, wantN = 2, len("ready\n")
				remaining = ""
			case "select-error":
				wantCalls, wantErr = 1, unix.EIO
			case "cancel-after-select":
				wantCalls, wantErr = 1, context.Canceled
			}
			if n != wantN || !errors.Is(err, wantErr) || calls != wantCalls {
				t.Fatalf("native Read: n=%d, err=%v, selects=%d; want n=%d, err=%v, selects=%d", n, err, calls, wantN, wantErr, wantCalls)
			}
			if n > 0 && string(buffer[:n]) != "ready\n" {
				t.Fatalf("native Read changed input: %q", buffer[:n])
			}
			// A canceled or failed select must not consume queued bytes or
			// close the caller's file. EOF keeps this reuse check bounded too.
			if _, err := writer.Write([]byte("next\n")); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			assertOIDCStdinNativeInput(t, input, remaining+"next\n")
		})
	}
}

func openOIDCStdinNativePTY(t *testing.T, highFD bool) (*os.File, *os.File, *unix.Termios) {
	t.Helper()
	if highFD {
		var limit unix.Rlimit
		if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
			t.Fatal(err)
		}
		if limit.Cur <= unix.FD_SETSIZE {
			t.Skip("descriptor limit cannot accommodate the high-FD case")
		}
	}
	master, terminal, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = master.Close() })
	t.Cleanup(func() { _ = terminal.Close() })
	if terminal.Fd() >= unix.FD_SETSIZE {
		t.Fatal("PTY fixture requires a descriptor below FD_SETSIZE")
	}
	before, err := unix.IoctlGetTermios(int(terminal.Fd()), unix.TIOCGETA)
	if err != nil {
		t.Fatal(err)
	}
	if before.Lflag&unix.ICANON == 0 || before.Cc[unix.VEOF] != '\x04' {
		t.Fatal("PTY fixture requires canonical input with Ctrl-D EOF")
	}
	// Only the fixture disables echo; keep canonical mode and EOF unchanged.
	before.Lflag &^= unix.ECHO
	if err := unix.IoctlSetTermios(int(terminal.Fd()), unix.TIOCSETA, before); err != nil {
		t.Fatal(err)
	}
	input := terminal
	if highFD {
		fd, err := unix.FcntlInt(terminal.Fd(), unix.F_DUPFD_CLOEXEC, unix.FD_SETSIZE)
		if err != nil {
			t.Fatal(err)
		}
		// Match inherited stdin's name while retaining a real terminal FD.
		input = os.NewFile(uintptr(fd), "/dev/stdin")
		t.Cleanup(func() { _ = input.Close() })
	}
	return master, input, before
}

func assertOIDCStdinNativeTTYMode(t *testing.T, input *os.File, before *unix.Termios) {
	t.Helper()
	after, err := unix.IoctlGetTermios(int(input.Fd()), unix.TIOCGETA)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("native input was closed or its terminal mode changed: %v", err)
	}
}

func assertOIDCStdinNativeInput(t *testing.T, input *os.File, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	got, err := readOIDCCodeInput(ctx, input)
	if err != nil || string(got) != want {
		t.Fatalf("borrowed native input is not reusable: got %q, err=%v, want %q", got, err, want)
	}
	if _, err := input.Stat(); err != nil {
		t.Fatalf("read closed the borrowed input: %v", err)
	}
}
