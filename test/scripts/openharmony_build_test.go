package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenHarmonyBuildContractIsCompileOnlyAndPublic(t *testing.T) {
	root := repoRoot(t)
	buildScript := readTextFile(t, filepath.Join(root, "scripts", "dev", "build-openharmony.sh"))
	for _, want := range []string{
		"OHOS_GO",
		"openharmony/arm64",
		"GOOS=openharmony GOARCH=arm64 CGO_ENABLED=0 GOTOOLCHAIN=local",
		"DWS_PACKAGE_VERSION",
		"DWS_GIT_COMMIT",
		"DWS_BUILD_TIME",
		"release-build-time.sh",
		"verify-openharmony-artifact.sh",
	} {
		if !strings.Contains(buildScript, want) {
			t.Errorf("OpenHarmony build script is missing contract %q", want)
		}
	}
	for _, forbidden := range []string{"qwenwork", "Aone", "private", "provision-ohos-ndk", "safechat"} {
		if strings.Contains(strings.ToLower(buildScript), strings.ToLower(forbidden)) {
			t.Errorf("OpenHarmony public build script contains forbidden reference %q", forbidden)
		}
	}

	goreleaser := readTextFile(t, filepath.Join(root, ".goreleaser.yaml"))
	for _, target := range []string{
		"      - darwin",
		"      - linux",
		"      - windows",
		"      - amd64",
		"      - arm64",
	} {
		if !strings.Contains(goreleaser, target) {
			t.Errorf("GoReleaser contract is missing %q", target)
		}
	}
	if strings.Contains(goreleaser, "openharmony") {
		t.Fatal("OpenHarmony was admitted to the six-platform GoReleaser release contract")
	}

	releaseVerifier := readTextFile(t, filepath.Join(root, "scripts", "release", "verify-release-artifacts.sh"))
	for _, asset := range []string{
		"dws-darwin-amd64.tar.gz",
		"dws-darwin-arm64.tar.gz",
		"dws-linux-amd64.tar.gz",
		"dws-linux-arm64.tar.gz",
		"dws-windows-amd64.zip",
		"dws-windows-arm64.zip",
	} {
		if !strings.Contains(releaseVerifier, asset) {
			t.Errorf("release verifier is missing six-platform asset %q", asset)
		}
	}
	if strings.Contains(releaseVerifier, "openharmony") {
		t.Fatal("OpenHarmony was admitted to the release artifact verifier")
	}
}

func TestOpenHarmonyArtifactVerifierAcceptsStaticMetadataFixture(t *testing.T) {
	result := runOpenHarmonyVerifier(t, openHarmonyVerifierFixture{
		header:         "Type: EXEC (Executable file)\nMachine: AArch64",
		programHeaders: "Program Headers:\n  LOAD",
		dynamic:        "There is no dynamic section in this file.",
		metadata:       "build\tGOOS=openharmony\nbuild\tGOARCH=arm64\nbuild\tCGO_ENABLED=0",
		binary:         "v1.2.3\n0123456789abcdef0123456789abcdef01234567\n",
		commit:         "0123456789abcdef0123456789abcdef01234567",
	})
	if result.err != nil {
		t.Fatalf("verifier rejected valid fixture: %v\n%s", result.err, result.output)
	}
	if !strings.Contains(result.output, "Verified OpenHarmony artifact") {
		t.Fatalf("verifier output = %q, want success confirmation", result.output)
	}
}

func TestOpenHarmonyArtifactVerifierRejectsMissingVersion(t *testing.T) {
	result := runOpenHarmonyVerifier(t, openHarmonyVerifierFixture{
		header:         "Type: EXEC (Executable file)\nMachine: AArch64",
		programHeaders: "Program Headers:\n  LOAD",
		dynamic:        "There is no dynamic section in this file.",
		metadata:       "build\tGOOS=openharmony\nbuild\tGOARCH=arm64\nbuild\tCGO_ENABLED=0",
		binary:         "0123456789abcdef0123456789abcdef01234567\n",
		commit:         "0123456789abcdef0123456789abcdef01234567",
	})
	if result.err == nil || !strings.Contains(result.output, "does not contain expected version") {
		t.Fatalf("verifier result = %v, output = %q; want missing-version failure", result.err, result.output)
	}
}

func TestOpenHarmonyArtifactVerifierFailsClosed(t *testing.T) {
	tests := []struct {
		name           string
		header         string
		programHeaders string
		dynamic        string
		metadata       string
		readerFailure  string
		want           string
	}{
		{
			name:          "header reader failure",
			readerFailure: "header",
			want:          "could not read the OpenHarmony ELF header",
		},
		{
			name:   "dynamic ELF type",
			header: "Type: DYN (Position-Independent Executable file)\nMachine: AArch64",
			want:   "not ET_EXEC",
		},
		{
			name:   "wrong architecture",
			header: "Type: EXEC (Executable file)\nMachine: Advanced Micro Devices X86-64",
			want:   "not AArch64",
		},
		{
			name:           "interpreter segment",
			header:         "Type: EXEC (Executable file)\nMachine: AArch64",
			programHeaders: "Program Headers:\n  INTERP",
			want:           "interpreter or dynamic segment",
		},
		{
			name:           "dynamic segment",
			header:         "Type: EXEC (Executable file)\nMachine: AArch64",
			programHeaders: "Program Headers:\n  DYNAMIC",
			want:           "interpreter or dynamic segment",
		},
		{
			name:           "dynamic section reader failure",
			header:         "Type: EXEC (Executable file)\nMachine: AArch64",
			programHeaders: "Program Headers:\n  LOAD",
			readerFailure:  "dynamic",
			want:           "could not read the OpenHarmony ELF dynamic section",
		},
		{
			name:           "missing build metadata",
			header:         "Type: EXEC (Executable file)\nMachine: AArch64",
			programHeaders: "Program Headers:\n  LOAD",
			dynamic:        "There is no dynamic section in this file.",
			metadata:       "build\tGOOS=linux\nbuild\tGOARCH=amd64\nbuild\tCGO_ENABLED=1",
			want:           "missing GOOS=openharmony",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runOpenHarmonyVerifier(t, openHarmonyVerifierFixture{
				header:         test.header,
				programHeaders: test.programHeaders,
				dynamic:        test.dynamic,
				metadata:       test.metadata,
				readerFailure:  test.readerFailure,
				binary:         "v1.2.3\n0123456789abcdef0123456789abcdef01234567\n",
				commit:         "0123456789abcdef0123456789abcdef01234567",
			})
			if result.err == nil {
				t.Fatalf("verifier accepted invalid fixture; output:\n%s", result.output)
			}
			if !strings.Contains(result.output, test.want) {
				t.Fatalf("verifier output = %q, want %q", result.output, test.want)
			}
		})
	}
}

type openHarmonyVerifierFixture struct {
	header         string
	programHeaders string
	dynamic        string
	metadata       string
	readerFailure  string
	binary         string
	commit         string
}

type openHarmonyVerifierResult struct {
	output string
	err    error
}

func runOpenHarmonyVerifier(t *testing.T, fixture openHarmonyVerifierFixture) openHarmonyVerifierResult {
	t.Helper()
	root := repoRoot(t)
	toolDir := t.TempDir()
	writeVerifierTool(t, filepath.Join(toolDir, "file"), `#!/bin/sh
printf 'ELF 64-bit LSB executable, ARM aarch64\n'
`)
	writeVerifierTool(t, filepath.Join(toolDir, "readelf"), `#!/bin/sh
case "${1:-}" in
  -h)
    [ "${DWS_TEST_READER_FAILURE:-}" = header ] && exit 1
    printf '%s\n' "$DWS_TEST_HEADER"
    ;;
  -l)
    [ "${DWS_TEST_READER_FAILURE:-}" = program ] && exit 1
    printf '%s\n' "$DWS_TEST_PROGRAM_HEADERS"
    ;;
  -d)
    [ "${DWS_TEST_READER_FAILURE:-}" = dynamic ] && exit 1
    printf '%s\n' "$DWS_TEST_DYNAMIC"
    ;;
  *) exit 2 ;;
esac
`)
	writeVerifierTool(t, filepath.Join(toolDir, "go"), `#!/bin/sh
printf '%s\n' "$DWS_TEST_METADATA"
`)

	binary := filepath.Join(t.TempDir(), "dws-openharmony-arm64")
	mustWriteFile(t, binary, []byte(fixture.binary), 0o755)
	verifier := filepath.Join(root, "scripts", "dev", "verify-openharmony-artifact.sh")
	cmd := exec.Command(verifier, binary, "v1.2.3")
	cmd.Env = withEnv(os.Environ(), map[string]string{
		"DWS_EXPECTED_COMMIT":      fixture.commit,
		"DWS_TEST_HEADER":          fixture.header,
		"DWS_TEST_PROGRAM_HEADERS": fixture.programHeaders,
		"DWS_TEST_DYNAMIC":         fixture.dynamic,
		"DWS_TEST_METADATA":        fixture.metadata,
		"DWS_TEST_READER_FAILURE":  fixture.readerFailure,
		"PATH":                     toolDir + string(os.PathListSeparator) + "/usr/bin:/bin",
	})
	output, err := cmd.CombinedOutput()
	return openHarmonyVerifierResult{output: string(output), err: err}
}

func writeVerifierTool(t *testing.T, path, source string) {
	t.Helper()
	mustWriteFile(t, path, []byte(source), 0o755)
}

func withEnv(env []string, updates map[string]string) []string {
	result := append([]string(nil), env...)
	for key, value := range updates {
		prefix := key + "="
		found := false
		for i, item := range result {
			if strings.HasPrefix(item, prefix) {
				result[i] = prefix + value
				found = true
				break
			}
		}
		if !found {
			result = append(result, prefix+value)
		}
	}
	return result
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
