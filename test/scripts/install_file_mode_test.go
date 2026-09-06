package scripts_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// GNU stat -f may emit a filesystem report for the real file before failing on
// the BSD format argument. Its stdout must not contaminate the successful retry.
func TestCrossPlatformCoverageInstallerPermissionStatFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer helpers")
	}
	for _, script := range []struct{ path, function string }{
		{"install.sh", "file_mode_decimal"},
		{"install-event.sh", "perm_of"},
		{"install-devapp.sh", "perm_of"},
	} {
		source := extractShellFunction(t, filepath.Join("..", "..", "scripts", script.path), script.function)
		for _, style := range []string{"-c", "-f"} {
			for _, mode := range []string{"644", "4755", "bad644", "888", "644\n755", "77777777777777777777"} {
				t.Run(script.path+"/"+style+"/"+mode, func(t *testing.T) {
					// Both implementations may print partial stdout when rejecting
					// the other dialect. Exercise both orders on every native host.
					stub := fmt.Sprintf("stat() {\n if [ \"$1\" = '%s' ]; then printf '%%s\\n' '%s'; else printf 'filesystem report from failed stat\\n'; return 1; fi\n}\n", style, mode)
					output, err := exec.Command("sh", "-c", source+"\n"+stub+script.function+" fixture\n").Output()
					if mode != "644" && mode != "4755" {
						if err == nil || len(output) != 0 {
							t.Fatalf("invalid mode emitted %q, err=%v", output, err)
						}
						return
					}
					want := mode + "\n"
					if script.function == "file_mode_decimal" {
						want = "420\n"
						if mode == "4755" {
							want = "2541\n" // preserve special bits; do not mask unsafe modes
						}
					}
					if err != nil || string(output) != want {
						t.Fatalf("permission = %q, err=%v; want %q", output, err, want)
					}
				})
			}
		}
	}
}
