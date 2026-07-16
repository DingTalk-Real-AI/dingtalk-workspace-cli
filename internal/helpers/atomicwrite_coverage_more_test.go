package helpers

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type atomicFakeTemp struct {
	chmodErr error
	writeErr error
	syncErr  error
	closeErr error
}

func (*atomicFakeTemp) Name() string                  { return "fake.tmp" }
func (f *atomicFakeTemp) Chmod(os.FileMode) error     { return f.chmodErr }
func (f *atomicFakeTemp) Write(p []byte) (int, error) { return len(p), f.writeErr }
func (f *atomicFakeTemp) Sync() error                 { return f.syncErr }
func (f *atomicFakeTemp) Close() error                { return f.closeErr }

type atomicErrorReader struct{}

func (atomicErrorReader) Read([]byte) (int, error) { return 0, errors.New("read") }

func TestCrossPlatformCoverageAtomicWriteRemainingFailures(t *testing.T) {
	origMkdir, origCreate := atomicMkdirAll, atomicCreateTemp
	origRemove, origRename := atomicRemove, atomicRename
	t.Cleanup(func() {
		atomicMkdirAll, atomicCreateTemp = origMkdir, origCreate
		atomicRemove, atomicRename = origRemove, origRename
	})

	atomicMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir") }
	if err := AtomicWrite("out/file", nil, 0o600); err == nil || !strings.Contains(err.Error(), "create directory") {
		t.Fatalf("mkdir err=%v", err)
	}
	atomicMkdirAll = origMkdir
	atomicCreateTemp = func(string, string) (atomicTempFile, error) { return nil, errors.New("create") }
	if err := AtomicWrite(filepath.Join(t.TempDir(), "file"), nil, 0o600); err == nil || !strings.Contains(err.Error(), "create temp") {
		t.Fatalf("create err=%v", err)
	}

	for _, tc := range []struct {
		name  string
		file  *atomicFakeTemp
		write func(atomicTempFile) error
		want  string
	}{
		{"chmod", &atomicFakeTemp{chmodErr: errors.New("chmod")}, func(atomicTempFile) error { return nil }, "set permissions"},
		{"write", &atomicFakeTemp{}, func(atomicTempFile) error { return errors.New("write") }, "write data"},
		{"sync", &atomicFakeTemp{syncErr: errors.New("sync")}, func(atomicTempFile) error { return nil }, "sync to disk"},
		{"close", &atomicFakeTemp{closeErr: errors.New("close")}, func(atomicTempFile) error { return nil }, "close temp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			atomicCreateTemp = func(string, string) (atomicTempFile, error) { return tc.file, nil }
			atomicRemove = func(string) error { return nil }
			if err := atomicWrite(filepath.Join(t.TempDir(), "file"), 0o600, tc.write); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want %q", err, tc.want)
			}
		})
	}

	atomicCreateTemp = func(string, string) (atomicTempFile, error) { return &atomicFakeTemp{}, nil }
	atomicRename = func(string, string) error { return errors.New("rename") }
	if err := atomicWrite(filepath.Join(t.TempDir(), "file"), 0o600, func(atomicTempFile) error { return nil }); err == nil || !strings.Contains(err.Error(), "rename to final") {
		t.Fatalf("rename err=%v", err)
	}

	atomicCreateTemp, atomicRename, atomicRemove = origCreate, origRename, origRemove
	if n, err := AtomicWriteFromReader(filepath.Join(t.TempDir(), "file"), atomicErrorReader{}, 0o600); err == nil || n != 0 {
		t.Fatalf("reader failure n=%d err=%v", n, err)
	}

	// Keep the io.Reader contract visible in this coverage-focused fake.
	var _ io.Reader = atomicErrorReader{}
}
