package scripts_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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

	packageScript := readTextFile(t, filepath.Join(root, "scripts", "dev", "package-openharmony.sh"))
	for _, want := range []string{
		"build-openharmony.sh",
		"binary-sign-tool",
		"-selfSign",
		"enforce_static_elf",
		"dws-openharmony-arm64.tar.gz",
		"checksums.txt",
		"gzip -n",
	} {
		if !strings.Contains(packageScript, want) {
			t.Errorf("OpenHarmony package script is missing contract %q", want)
		}
	}
	for _, forbidden := range []string{"qwenwork", "Aone", "private"} {
		if strings.Contains(strings.ToLower(packageScript), strings.ToLower(forbidden)) {
			t.Errorf("OpenHarmony public package script contains forbidden reference %q", forbidden)
		}
	}

	provisionScript := readTextFile(t, filepath.Join(root, "scripts", "dev", "provision-ohos-go.sh"))
	for _, want := range []string{
		"OHOS_GO_ARCHIVE",
		"OHOS_GO_SHA256",
		"OHOS_GO_ROOT",
		"go1.26.7",
		"openharmony/arm64",
	} {
		if !strings.Contains(provisionScript, want) {
			t.Errorf("OpenHarmony provision script is missing contract %q", want)
		}
	}
	for _, forbidden := range []string{"qwenwork", "aone", "alibaba-inc.com", "sha256_default", "toolchain/ohos"} {
		if strings.Contains(strings.ToLower(provisionScript), strings.ToLower(forbidden)) {
			t.Errorf("OpenHarmony public provision script contains forbidden reference %q", forbidden)
		}
	}

	workflow := readTextFile(t, filepath.Join(root, ".github", "workflows", "openharmony.yml"))
	for _, want := range []string{
		"ubuntu-latest",
		"OHOS_GO_ARCHIVE: https://github.com/typefield/dingtalk-workspace-cli/releases/download/ohos-go1.26.7-toolchain/1.26_ohos_golang_go_cross.tar.gz",
		"OHOS_GO_SHA256: e757acdc005098f1debc888cdbaa13e26faf48e2bcf38baa17cbc5df49d8d125",
		"DWS_REQUIRE_ELF_STATIC",
		"package-openharmony.sh",
		"actions/upload-artifact@v4",
		"checksums.txt",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("OpenHarmony workflow is missing contract %q", want)
		}
	}
	if strings.Contains(strings.ToLower(workflow), "alibaba-inc.com") {
		t.Errorf("OpenHarmony public workflow contains forbidden reference to internal infrastructure")
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

func TestOpenHarmonyPackageEndToEnd(t *testing.T) {
	root := repoRoot(t)
	outputDir := t.TempDir()
	toolDir := t.TempDir()

	writeVerifierTool(t, filepath.Join(toolDir, "file"), `#!/bin/sh
printf 'ELF 64-bit LSB executable, ARM aarch64\n'
`)
	writeVerifierTool(t, filepath.Join(toolDir, "readelf"), `#!/bin/sh
case "${1:-}" in
  -h) printf 'Type: EXEC (Executable file)\nMachine: AArch64\n' ;;
  -l) printf 'Program Headers:\n  LOAD\n' ;;
  -d) printf 'There is no dynamic section in this file.\n' ;;
  *) exit 2 ;;
esac
`)

	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("resolve host go: %v", err)
	}

	writeVerifierTool(t, filepath.Join(toolDir, "go"), fmt.Sprintf(`#!/bin/sh
if [ "${1:-}" = "version" ] && [ "${2:-}" = "-m" ]; then
  printf 'build\tGOOS=openharmony\nbuild\tGOARCH=arm64\nbuild\tCGO_ENABLED=0\n'
  exit 0
fi
exec %q "$@"
`, realGo))

	fakeOhosGo := filepath.Join(toolDir, "fake-ohos-go")
	writeVerifierTool(t, fakeOhosGo, `#!/usr/bin/env bash
set -eu
if [ "${1:-}" = tool ] && [ "${2:-}" = dist ] && [ "${3:-}" = list ]; then
  printf 'openharmony/arm64\n'
  exit 0
fi
if [ "${1:-}" = build ]; then
  out=""
  version=""
  commit=""
  while [ "$#" -gt 0 ]; do
    case "$1" in
      -o) out="$2"; shift 2 ;;
      -ldflags=*)
        for f in ${1#-ldflags=}; do
          case "$f" in
            *internal/app.version=*) version="${f#*=}" ;;
            *internal/app.gitCommit=*) commit="${f#*=}" ;;
          esac
        done
        shift ;;
      *) shift ;;
    esac
  done
  [ -n "$out" ]
  python3 - "$out" "$version" "$commit" <<'PY'
import struct, sys
out, version, commit = sys.argv[1], sys.argv[2], sys.argv[3]
strings = b"\0.text\0.shstrtab\0"
shoff = 0x200
header = bytearray(64)
header[:4] = b"\x7fELF"
header[4:7] = bytes((2, 1, 1))
struct.pack_into("<HHIQQQIHHHHHH", header, 16, 2, 183, 1, 0, 0, shoff, 0, 64, 0, 0, 64, 3, 2)
sections = bytearray(64 * 3)
struct.pack_into("<IIQQQQIIQQ", sections, 64, 1, 1, 0, 0, 0x100, 1, 0, 0, 1, 0)
struct.pack_into("<IIQQQQIIQQ", sections, 128, 7, 3, 0, 0, 0x101, len(strings), 0, 0, 1, 0)
meta = f"{version}\n{commit}\n".encode()
data = header + bytearray(0x100 - len(header)) + b"\xc3" + strings + meta
if len(data) < shoff:
    data += b"\0" * (shoff - len(data))
data += sections
open(out, "wb").write(data)
PY
  chmod +x "$out"
  exit 0
fi
exit 2
`)

	pkgScript := filepath.Join(root, "scripts", "dev", "package-openharmony.sh")
	env := withEnv(os.Environ(), map[string]string{
		"OHOS_GO":             fakeOhosGo,
		"DWS_PACKAGE_VERSION": "v1.2.3",
		"PATH":                toolDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	})

	runPkg := func() ([]byte, string) {
		cmd := exec.Command(pkgScript, outputDir)
		cmd.Dir = root
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("package-openharmony.sh failed: %v\n%s", err, out)
		}
		archivePath := filepath.Join(outputDir, "dws-openharmony-arm64.tar.gz")
		archiveBytes, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("read archive: %v", err)
		}
		checksumsPath := filepath.Join(outputDir, "checksums.txt")
		checksumsBytes, err := os.ReadFile(checksumsPath)
		if err != nil {
			t.Fatalf("read checksums: %v", err)
		}
		return archiveBytes, string(checksumsBytes)
	}

	firstArchive, firstChecksums := runPkg()

	digest := sha256.Sum256(firstArchive)
	expectedDigest := hex.EncodeToString(digest[:])
	expectedChecksum := fmt.Sprintf("%s  dws-openharmony-arm64.tar.gz\n", expectedDigest)
	if firstChecksums != expectedChecksum {
		t.Errorf("checksums = %q, want %q", firstChecksums, expectedChecksum)
	}

	gz, err := gzip.NewReader(bytes.NewReader(firstArchive))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	foundFiles := make(map[string]bool)
	var dwsBinary []byte
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		foundFiles[hdr.Name] = true
		if hdr.Name == "dws" {
			dwsBinary, err = io.ReadAll(tr)
			if err != nil {
				t.Fatalf("read dws from archive: %v", err)
			}
		}
	}
	for _, expectedFile := range []string{"LICENSE", "NOTICE", "README.md", "CHANGELOG.md", "dws"} {
		if !foundFiles[expectedFile] {
			t.Errorf("archive missing file %q", expectedFile)
		}
	}

	if len(dwsBinary) < 64 || string(dwsBinary[:4]) != "\x7fELF" {
		t.Fatalf("dws binary in archive is not ELF: len=%d", len(dwsBinary))
	}

	secondArchive, secondChecksums := runPkg()
	if !bytes.Equal(firstArchive, secondArchive) {
		t.Fatal("package-openharmony.sh is not bit-for-bit reproducible across runs")
	}
	if firstChecksums != secondChecksums {
		t.Fatal("checksums differ across runs")
	}
}

func TestOpenHarmonyProvisionRejectsMissingArchive(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "dev", "provision-ohos-go.sh"))
	cmd.Env = withEnv(os.Environ(), map[string]string{
		"OHOS_GO_ARCHIVE": filepath.Join(t.TempDir(), "missing.tar.gz"),
		"OHOS_GO_ROOT":    filepath.Join(t.TempDir(), "root"),
		"OHOS_GO_WORK":    filepath.Join(t.TempDir(), "work"),
	})
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("provision accepted a missing archive; output:\n%s", output)
	}
	if !strings.Contains(string(output), "archive does not exist") {
		t.Fatalf("provision output = %q, want missing-archive failure", output)
	}
}

func TestOpenHarmonyProvisionRejectsShaMismatch(t *testing.T) {
	root := repoRoot(t)
	archive := filepath.Join(t.TempDir(), "ohos-go.tar.gz")
	if err := os.WriteFile(archive, []byte("not a toolchain"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "dev", "provision-ohos-go.sh"))
	cmd.Env = withEnv(os.Environ(), map[string]string{
		"OHOS_GO_ARCHIVE": archive,
		"OHOS_GO_SHA256":  "0000000000000000000000000000000000000000000000000000000000000000",
		"OHOS_GO_ROOT":    filepath.Join(t.TempDir(), "root"),
		"OHOS_GO_WORK":    filepath.Join(t.TempDir(), "work"),
	})
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("provision accepted a sha mismatch; output:\n%s", output)
	}
	if !strings.Contains(string(output), "sha256 mismatch") {
		t.Fatalf("provision output = %q, want sha mismatch failure", output)
	}
}

func TestOpenHarmonyProvisionCachesValidatedToolchain(t *testing.T) {
	root := repoRoot(t)
	toolDir := t.TempDir()

	fakeGo := filepath.Join(toolDir, "fake-go")
	writeVerifierTool(t, fakeGo, `#!/usr/bin/env bash
set -eu
if [ "${1:-}" = version ]; then
  printf 'go version go1.26.7 linux/amd64\n'
  exit 0
fi
if [ "${1:-}" = tool ] && [ "${2:-}" = dist ] && [ "${3:-}" = list ]; then
  printf 'openharmony/arm64\n'
  exit 0
fi
if [ "${1:-}" = env ] && [ "${2:-}" = GOROOT ]; then
  dirname="$(cd "$(dirname "$0")/.." && pwd)"
  printf '%s\n' "$dirname"
  exit 0
fi
if [ "${1:-}" = build ]; then
  out=""
  version=""
  commit=""
  while [ "$#" -gt 0 ]; do
    case "$1" in
      -o) out="$2"; shift 2 ;;
      -ldflags=*)
        for f in ${1#-ldflags=}; do
          case "$f" in
            *internal/app.version=*) version="${f#*=}" ;;
            *internal/app.gitCommit=*) commit="${f#*=}" ;;
          esac
        done
        shift ;;
      *) shift ;;
    esac
  done
  [ -n "$out" ]
  python3 - "$out" "$version" "$commit" <<'PY'
import struct, sys
out, version, commit = sys.argv[1], sys.argv[2], sys.argv[3]
strings = b"\0.text\0.shstrtab\0"
shoff = 0x200
header = bytearray(64)
header[:4] = b"\x7fELF"
header[4:7] = bytes((2, 1, 1))
struct.pack_into("<HHIQQQIHHHHHH", header, 16, 2, 183, 1, 0, 0, shoff, 0, 64, 0, 0, 64, 3, 2)
sections = bytearray(64 * 3)
struct.pack_into("<IIQQQQIIQQ", sections, 64, 1, 1, 0, 0, 0x100, 1, 0, 0, 1, 0)
struct.pack_into("<IIQQQQIIQQ", sections, 128, 7, 3, 0, 0, 0x101, len(strings), 0, 0, 1, 0)
meta = f"{version}\n{commit}\n".encode()
data = header + bytearray(0x100 - len(header)) + b"\xc3" + strings + meta
if len(data) < shoff:
    data += b"\0" * (shoff - len(data))
data += sections
open(out, "wb").write(data)
PY
  chmod +x "$out"
  exit 0
fi
exit 2
`)

	archive := filepath.Join(toolDir, "ohos-go.tar.gz")
	writeTarballToolchain(t, archive, "ohos-go", fakeGo)

	provisionRoot := filepath.Join(t.TempDir(), "root")
	provisionEnv := func(work string) []string {
		return withEnv(os.Environ(), map[string]string{
			"OHOS_GO":         "",
			"OHOS_GO_ARCHIVE": archive,
			"OHOS_GO_SHA256":  "",
			"OHOS_GO_ROOT":    provisionRoot,
			"OHOS_GO_WORK":    work,
		})
	}
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "dev", "provision-ohos-go.sh"))
	cmd.Env = provisionEnv(filepath.Join(t.TempDir(), "work"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("provision failed: %v\n%s", err, output)
	}
	wantBin := filepath.Join(provisionRoot, "bin", "go") + "\n"
	if !strings.HasSuffix(string(output), wantBin) {
		t.Fatalf("provision output = %q, want suffix %q", output, wantBin)
	}
	if _, err := os.Stat(filepath.Join(provisionRoot, "bin", "go")); err != nil {
		t.Fatalf("provisioned toolchain missing bin/go: %v", err)
	}

	// A second run must be served from the cache without re-extracting.
	cmd2 := exec.Command("bash", filepath.Join(root, "scripts", "dev", "provision-ohos-go.sh"))
	cmd2.Env = provisionEnv(filepath.Join(t.TempDir(), "work2"))
	rerun, err := cmd2.CombinedOutput()
	if err != nil {
		t.Fatalf("cached provision failed: %v\n%s", err, rerun)
	}
	if !strings.HasSuffix(string(rerun), wantBin) {
		t.Fatalf("cached provision output = %q, want suffix %q", rerun, wantBin)
	}

	// build-openharmony.sh must pick up the provisioned toolchain when
	// OHOS_GO is unset, and the resulting artifact must pass verification.
	builtPath := filepath.Join(t.TempDir(), "built")
	shimDir := t.TempDir()
	writeVerifierTool(t, filepath.Join(shimDir, "go"), fmt.Sprintf(`#!/usr/bin/env bash
if [ "${1:-}" = version ] && [ "${2:-}" = -m ]; then
  printf 'build\tGOOS=openharmony\nbuild\tGOARCH=arm64\nbuild\tCGO_ENABLED=0\n'
  exit 0
fi
exec %q "$@"
`, filepath.Join(provisionRoot, "bin", "go")))
	writeVerifierTool(t, filepath.Join(shimDir, "file"), `#!/bin/sh
printf 'ELF 64-bit LSB executable, ARM aarch64\n'
`)
	writeVerifierTool(t, filepath.Join(shimDir, "readelf"), `#!/bin/sh
case "${1:-}" in
  -h) printf 'Type: EXEC (Executable file)\nMachine: AArch64\n' ;;
  -l) printf 'Program Headers:\n  LOAD\n' ;;
  -d) printf 'There is no dynamic section in this file.\n' ;;
  *) exit 2 ;;
esac
`)

	buildEnv := withEnv(os.Environ(), map[string]string{
		"OHOS_GO_ARCHIVE":     "",
		"OHOS_GO_ROOT":        provisionRoot,
		"OHOS_GO_WORK":        filepath.Join(t.TempDir(), "work3"),
		"OHOS_OUTPUT":         builtPath,
		"DWS_PACKAGE_VERSION": "v1.2.3",
		"PATH":                shimDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
	// Strip OHOS_GO so the build must provision.
	filtered := make([]string, 0, len(buildEnv))
	for _, item := range buildEnv {
		if !strings.HasPrefix(item, "OHOS_GO=") {
			filtered = append(filtered, item)
		}
	}
	buildEnv = filtered

	cmd3 := exec.Command("bash", filepath.Join(root, "scripts", "dev", "build-openharmony.sh"))
	cmd3.Env = buildEnv
	buildOut, err := cmd3.CombinedOutput()
	if err != nil {
		t.Fatalf("build-openharmony.sh with provisioned toolchain failed: %v\n%s", err, buildOut)
	}
	if !strings.Contains(string(buildOut), "Built ") {
		t.Fatalf("build output = %q, want Built confirmation", buildOut)
	}
	if _, err := os.Stat(builtPath); err != nil {
		t.Fatalf("built artifact missing: %v", err)
	}
}

func writeTarballToolchain(t *testing.T, archivePath, rootDir, fakeGoPath string) {
	t.Helper()
	goBytes, err := os.ReadFile(fakeGoPath)
	if err != nil {
		t.Fatalf("read fake go: %v", err)
	}
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	if err := tw.WriteHeader(&tar.Header{
		Name: rootDir + "/bin/go",
		Mode: 0o755,
		Size: int64(len(goBytes)),
	}); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(goBytes); err != nil {
		t.Fatalf("tar write go: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	if err := os.WriteFile(archivePath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
}
