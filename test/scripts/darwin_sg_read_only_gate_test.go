package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// runExtractDataConstFlags executes the release awk extractor against an
// `otool -l` capture and returns its flags words in output order. The awk
// interpreter is the same tool family (one-true-awk/BWK awk) the macOS
// publication runner uses, so the fixture results transfer directly.
func runExtractDataConstFlags(t *testing.T, fixture string) []string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repository root) error = %v", err)
	}
	awkFile := filepath.Join(root, "scripts", "release", "extract-data-const-flags.awk")
	data, err := os.ReadFile(filepath.Join(root, "test", "scripts", "testdata", fixture))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", fixture, err)
	}

	cmd := exec.Command("awk", "-f", awkFile)
	cmd.Stdin = strings.NewReader(string(data))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("awk -f extract-data-const-flags.awk < %s error = %v", fixture, err)
	}
	var flags []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			flags = append(flags, line)
		}
	}
	return flags
}

// sgReadOnlyMissing reports whether any captured __DATA_CONST flags word lacks
// SG_READ_ONLY (0x10), mirroring the publication gate's fail-closed loop.
func sgReadOnlyMissing(t *testing.T, flags []string) bool {
	for _, entry := range flags {
		value, err := strconv.ParseInt(entry, 0, 64)
		if err != nil {
			t.Fatalf("parse flags entry %q: %v", entry, err)
		}
		if value&0x10 == 0 {
			return true
		}
	}
	return false
}

// TestReleaseGateExtractsEveryDataConstSlice pins the #1441 publication gate
// against real `otool -l` captures: thin broken/healthy v1.0.62 binaries, the
// vendored universal runtime dylib, and a universal sample whose last slice
// lacks SG_READ_ONLY. The first-slice-only parser that this regression
// replaces accepted exactly that last fixture.
func TestReleaseGateExtractsEveryDataConstSlice(t *testing.T) {
	t.Parallel()

	cases := []struct {
		fixture string
		want    []string
		wantBad bool
	}{
		{
			fixture: "darwin-otool-broken-amd64-v1.0.62.txt",
			want:    []string{"0x0"},
			wantBad: true,
		},
		{
			fixture: "darwin-otool-healthy-arm64-v1.0.62.txt",
			want:    []string{"0x10"},
			wantBad: false,
		},
		{
			fixture: "darwin-otool-universal-runtime-dylib.txt",
			want:    []string{"0x10", "0x10"},
			wantBad: false,
		},
		{
			// arm64 slice first (0x10), amd64 slice last (0x0): the
			// first-slice-only parser returned 0x10 and wrongly passed.
			fixture: "darwin-otool-universal-mixed-bad-last-slice.txt",
			want:    []string{"0x10", "0x0"},
			wantBad: true,
		},
	}
	for _, tc := range cases {
		got := runExtractDataConstFlags(t, tc.fixture)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: extracted %v (%d entries), want %v", tc.fixture, got, len(got), tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("%s: entry %d = %q, want %q", tc.fixture, i, got[i], tc.want[i])
			}
		}
		if missing := sgReadOnlyMissing(t, got); missing != tc.wantBad {
			t.Fatalf("%s: gate missing-SG_READ_ONLY = %v, want %v", tc.fixture, missing, tc.wantBad)
		}
	}
}
