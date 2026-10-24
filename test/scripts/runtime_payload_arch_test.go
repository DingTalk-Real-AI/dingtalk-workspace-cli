package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRuntimePayloadPEArchitectureToolFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX policy script")
	}
	for _, tc := range []struct {
		name, objdumpArch, llvmArch, fileDesc string
		strict                                bool
		wantError                             string
	}{
		{"unknown GNU architecture falls back to file", "UNKNOWN!, flags 0x0000012f:", "", "PE32+ executable (DLL) (GUI) Aarch64, for MS Windows", false, ""},
		{"unknown GNU architecture falls back to LLVM", "UNKNOWN!, flags 0x0000012f:", "aarch64", "PE32+ executable for MS Windows (DLL)", false, ""},
		{"known GNU architecture needs no file architecture", "aarch64", "", "PE32+ executable for MS Windows (DLL)", false, ""},
		{"known wrong GNU architecture fails closed", "i386:x86-64", "aarch64", "PE32+ executable (DLL) (GUI) Aarch64, for MS Windows", false, "invalid Windows arm64 library architecture"},
		{"unknown tools may skip architecture inspection", "UNKNOWN!", "unknown", "PE32+ executable for MS Windows (DLL)", false, ""},
		{"unknown tools fail in strict mode", "UNKNOWN!", "unknown", "PE32+ executable for MS Windows (DLL)", true, "cannot inspect Windows arm64 library architecture"},
		{"file known wrong architecture fails closed", "UNKNOWN!", "", "PE32+ executable (DLL) (GUI) x86-64, for MS Windows", false, "invalid Windows arm64 library architecture"},
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
			if tc.wantError != "" {
				if err == nil || !strings.Contains(string(out), tc.wantError) {
					t.Fatalf("error = %v; want %q; output:\n%s", err, tc.wantError, out)
				}
			} else if err != nil || !strings.Contains(string(out), "Runtime payload verified:") {
				t.Fatalf("error = %v; output:\n%s", err, out)
			}
		})
	}
}
