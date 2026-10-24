package scripts_test

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRuntimePayloadPEArchitectureIgnoresHostToolGuesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX policy script")
	}
	for _, tc := range []struct {
		name, objdumpArch, llvmArch, fileDesc string
		strict                                bool
	}{
		{"unknown GNU architecture with file support", "UNKNOWN!, flags 0x0000012f:", "", "PE32+ executable (DLL) (GUI) Aarch64, for MS Windows", false},
		{"unknown GNU architecture with LLVM support", "UNKNOWN!, flags 0x0000012f:", "aarch64", "PE32+ executable for MS Windows (DLL)", false},
		{"known GNU architecture needs no file architecture", "aarch64", "", "PE32+ executable for MS Windows (DLL)", false},
		{"wrong GNU guess does not override PE header", "i386:x86-64", "aarch64", "PE32+ executable (DLL) (GUI) Aarch64, for MS Windows", false},
		{"unknown tools still verify PE architecture", "UNKNOWN!", "unknown", "PE32+ executable for MS Windows (DLL)", false},
		{"strict mode verifies PE architecture without host support", "UNKNOWN!", "unknown", "PE32+ executable for MS Windows (DLL)", true},
		{"wrong file guess does not override PE header", "UNKNOWN!", "", "PE32+ executable (DLL) (GUI) x86-64, for MS Windows", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			writeTool := func(name, script string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nset -eu\n"+script), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			// Keep the real pinned libraries and checksum checks. Only the host
			// inspection tools vary, as they do between macOS and Linux runners.
			exports := "for symbol in k9Xm2pQv d4Rw7Lnz h6Yb3Jtq m8Vc5Kxf p2Zn9Gsa t5Qe1Hud b7Uj4Myr f3Wi8Olc; do printf ' T %s\\n' \"$symbol\"; done\n"
			writeTool("nm", exports)
			objdump := func(arch string) string {
				probe := "printf '%s\\n' 'architecture: " + arch + "'\n"
				if arch == "" {
					probe = "exit 1\n"
				}
				return "if [ \"$1\" = -p ]; then\n" + exports + "exit 0\nfi\n" +
					"case \"$2\" in\n*/windows/arm64/*)\n" +
					probe + ";;\n" +
					"*) printf '%s\\n' 'architecture: i386:x86-64' ;;\nesac\n"
			}
			writeTool("objdump", objdump(tc.objdumpArch))
			writeTool("llvm-objdump", objdump(tc.llvmArch))
			writeTool("file", "case \"$1\" in\n"+
				"*/darwin/*) desc='Mach-O universal binary' ;;\n"+
				"*/linux/amd64/*) desc='ELF 64-bit LSB shared object, x86-64' ;;\n"+
				"*/linux/arm64/*) desc='ELF 64-bit LSB shared object, ARM aarch64' ;;\n"+
				"*/windows/amd64/*) desc='PE32+ executable (DLL) x86-64' ;;\n"+
				"*/windows/arm64/*) desc='"+tc.fileDesc+"' ;;\nesac\n"+
				"printf '%s: %s\\n' \"$1\" \"$desc\"\n")
			args := []string{filepath.Join(repoRoot(t), "scripts", "policy", "check-runtime-payload.sh")}
			if !tc.strict {
				args = append(args, "--allow-unsupported-tools")
			}
			cmd := exec.Command("sh", args...)
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(out), "Runtime payload verified:") {
				t.Fatalf("error = %v; output:\n%s", err, out)
			}
		})
	}
}

func TestRuntimePayloadPEMachineFieldRejectsWrongOrMalformedHeaders(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX policy script")
	}
	script := readTextFile(t, filepath.Join(repoRoot(t), "scripts", "policy", "check-runtime-payload.sh"))
	start := strings.Index(script, "check_pe_arch() {")
	if start < 0 {
		t.Fatal("missing PE architecture verifier")
	}
	end := strings.Index(script[start:], "\n}\n")
	if end < 0 {
		t.Fatal("missing PE architecture verifier end")
	}
	// Exercise the production verifier with PE fixtures; full-script tests above
	// cover its invocation against the real, checksum-pinned payloads.
	verify := "fail() { printf '%s\\n' \"$*\" >&2; exit 1; }\n" + script[start:start+end+3] + "\ncheck_pe_arch sample.dll \"$1\" test\n"
	for _, tc := range []struct {
		name, expected, wantError string
		machine                   uint16
		malformed                 string
	}{
		{"amd64", "6486", "", 0x8664, ""},
		{"arm64", "64aa", "", 0xaa64, ""},
		{"amd64 in arm64 slot", "64aa", "invalid test library architecture", 0x8664, ""},
		{"arm64 in amd64 slot", "6486", "invalid test library architecture", 0xaa64, ""},
		{"bad PE signature", "64aa", "invalid test PE signature", 0xaa64, "signature"},
		{"truncated DOS header", "64aa", "invalid test PE offset", 0xaa64, "dos"},
		{"truncated PE header", "64aa", "invalid test PE header", 0xaa64, "pe"},
		{"PE offset beyond EOF", "64aa", "cannot read test PE header", 0xaa64, "offset"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			const offset = 260 // Exercise both low bytes of the little-endian offset.
			data := make([]byte, offset+6)
			copy(data, "MZ")
			binary.LittleEndian.PutUint32(data[60:], offset)
			copy(data[offset:], "PE\x00\x00")
			binary.LittleEndian.PutUint16(data[offset+4:], tc.machine)
			switch tc.malformed {
			case "signature":
				data[offset] = 'X'
			case "dos":
				data = data[:62]
			case "pe":
				data = data[:offset+5]
			case "offset":
				binary.LittleEndian.PutUint32(data[60:], 4096)
			}
			if err := os.WriteFile(filepath.Join(dir, "sample.dll"), data, 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", "-eu", "-c", verify, "verify-pe", tc.expected)
			cmd.Env = append(os.Environ(), "PAYLOAD="+dir)
			out, err := cmd.CombinedOutput()
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("error = %v; output:\n%s", err, out)
				}
			} else if err == nil || !strings.Contains(string(out), tc.wantError) {
				t.Fatalf("error = %v; want %q; output:\n%s", err, tc.wantError, out)
			}
		})
	}
}
