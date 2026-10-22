//go:build windows

package junction

import (
	"errors"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestJunctionSubstituteName(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{`\\?\UNC\server\share\dir`, `\??\UNC\server\share\dir\`},
		{`\\?\C:\dir`, `\??\C:\dir\`},
		{`\\server\share\dir`, `\??\UNC\server\share\dir\`},
		{`C:\dir\`, `\??\C:\dir\`},
		{`C:\dir`, `\??\C:\dir\`},
	} {
		if got := JunctionSubstituteName(tc.input); got != tc.want {
			t.Fatalf("JunctionSubstituteName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestMountPointReparseBufferValidation(t *testing.T) {
	if _, err := MountPointReparseBuffer("bad\x00sub", "print"); err == nil {
		t.Fatal("expected NUL substitute error")
	}
	if _, err := MountPointReparseBuffer("sub", "bad\x00print"); err == nil {
		t.Fatal("expected NUL print name error")
	}
	if _, err := MountPointReparseBuffer(strings.Repeat("a", 20000), "print"); err == nil || !strings.Contains(err.Error(), "junction path too long") {
		t.Fatalf("expected oversized path error, got %v", err)
	}
}

func TestCrossPlatformCoverageJunctionCreateRejectsIncompleteOps(t *testing.T) {
	full := Ops{
		Abs:                func(path string) (string, error) { return path, nil },
		Mkdir:              func(string, os.FileMode) error { return nil },
		Remove:             func(string) error { return nil },
		UTF16PtrFromString: windows.UTF16PtrFromString,
		CreateFile: func(*uint16, uint32, uint32, *windows.SecurityAttributes, uint32, uint32, windows.Handle) (windows.Handle, error) {
			return 0, errors.New("unused")
		},
		DeviceIoControl: func(windows.Handle, uint32, *byte, uint32, *byte, uint32, *uint32, *windows.Overlapped) error {
			return errors.New("unused")
		},
	}
	for name, clear := range map[string]func(*Ops){
		"abs":               func(ops *Ops) { ops.Abs = nil },
		"mkdir":             func(ops *Ops) { ops.Mkdir = nil },
		"remove":            func(ops *Ops) { ops.Remove = nil },
		"utf16":             func(ops *Ops) { ops.UTF16PtrFromString = nil },
		"create-file":       func(ops *Ops) { ops.CreateFile = nil },
		"device-io-control": func(ops *Ops) { ops.DeviceIoControl = nil },
	} {
		ops := full
		clear(&ops)
		if err := Create(`C:\target`, `C:\link`, ops); err == nil || !strings.Contains(err.Error(), "junction operations are incomplete") {
			t.Fatalf("%s op missing: expected incomplete ops error, got %v", name, err)
		}
	}
}
