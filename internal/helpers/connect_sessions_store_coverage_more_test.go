package helpers

import (
	"errors"
	"os"
	"testing"
)

type failingSessionTempFile struct {
	chmodErr error
	writeErr error
	closeErr error
}

func (*failingSessionTempFile) Name() string              { return "sessions.tmp" }
func (f *failingSessionTempFile) Chmod(os.FileMode) error { return f.chmodErr }
func (f *failingSessionTempFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}
func (f *failingSessionTempFile) Close() error { return f.closeErr }

func TestCrossPlatformCoverageConnectSessionStoreFailureCoverage(t *testing.T) {
	originalRead := connectSessionReadFile
	originalMkdir := connectSessionMkdirAll
	originalCreate := connectSessionCreateTemp
	originalRename := connectSessionRename
	originalRemove := connectSessionRemove
	t.Cleanup(func() {
		connectSessionReadFile = originalRead
		connectSessionMkdirAll = originalMkdir
		connectSessionCreateTemp = originalCreate
		connectSessionRename = originalRename
		connectSessionRemove = originalRemove
	})
	boom := errors.New("session store failure")

	connectSessionReadFile = func(string) ([]byte, error) { return nil, boom }
	if got := loadConvSessionMap("sessions.json"); len(got) != 0 {
		t.Fatalf("read failure map = %#v", got)
	}
	connectSessionReadFile = originalRead

	connectSessionMkdirAll = func(string, os.FileMode) error { return boom }
	saveConvSessionMap("dir/sessions.json", map[string]string{"c": "s"})
	connectSessionMkdirAll = func(string, os.FileMode) error { return nil }
	connectSessionCreateTemp = func(string, string) (connectSessionTempFile, error) { return nil, boom }
	saveConvSessionMap("dir/sessions.json", map[string]string{"c": "s"})

	connectSessionRemove = func(string) error { return nil }
	for _, file := range []*failingSessionTempFile{
		{chmodErr: boom},
		{writeErr: boom},
		{closeErr: boom},
	} {
		connectSessionCreateTemp = func(string, string) (connectSessionTempFile, error) { return file, nil }
		saveConvSessionMap("dir/sessions.json", map[string]string{"c": "s"})
	}
	connectSessionCreateTemp = func(string, string) (connectSessionTempFile, error) { return &failingSessionTempFile{}, nil }
	connectSessionRename = func(string, string) error { return boom }
	saveConvSessionMap("dir/sessions.json", map[string]string{"c": "s"})
}
