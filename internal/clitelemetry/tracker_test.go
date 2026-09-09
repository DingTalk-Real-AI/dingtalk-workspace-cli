package clitelemetry

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/profilemetadata"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDefaultIdentityIsReadOnly(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    Identity
	}{
		{name: "valid", content: `{"version":3,"currentProfile":"corp:user","profiles":[{"name":"alpha","corpId":"corp","userId":"user","userName":" Alice ","clientId":"private-client"}]}`, want: Identity{UserID: "user", UserName: "Alice", CorpID: "corp"}},
		{name: "corrupt", content: `{`},
		{name: "future version", content: `{"version":999}`},
		{name: "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content := tc.content
			directory := t.TempDir()
			path := filepath.Join(directory, "profiles.json")
			if content != "" {
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want := tc.want
			if got := DefaultIdentity(directory); got != want {
				t.Fatalf("metadata identity = %#v, want %#v", got, want)
			}
			expectedEntries := 0
			if content != "" {
				expectedEntries = 1
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != expectedEntries {
				t.Fatalf("identity lookup changed metadata directory: %v, %v", entries, err)
			}
			if content != "" {
				data, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(data, []byte(content)) {
					t.Fatalf("identity lookup rewrote metadata: %v", err)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageDefaultIdentityRecoversFromPanic(t *testing.T) {
	testseam.Swap(t, &resolveReadOnlyProfile, func(string, string) (*profilemetadata.ProfileMetadata, error) {
		panic("metadata lookup exploded")
	})
	if got := DefaultIdentity(t.TempDir()); got != (Identity{}) {
		t.Fatalf("recovered identity = %#v", got)
	}
}
