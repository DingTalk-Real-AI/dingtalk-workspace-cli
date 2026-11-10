package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageUpgradeDarwinAtomicRepair(t *testing.T) {
	for _, name := range []string{"success", "lookup", "read", "mkdir", "write", "sign", "rename", "quarantine"} {
		t.Run(name, func(t *testing.T) {
			// Exercise equivalent path spellings as well as the native separator.
			root := t.TempDir() + string(os.PathSeparator) + "."
			binary := filepath.Join(root, "dws")
			original := []byte("unsigned fixture")
			if err := os.WriteFile(binary, original, 0755); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(binary)
			if err != nil {
				t.Fatal(err)
			}
			fail := errors.New("fixture-failure")
			testseam.Swap(t, &upgradeLookPath, func(string) (string, error) { return "/codesign", nil })
			signed := false
			testseam.Swap(t, &upgradeCommandOutput, func(command string, args ...string) ([]byte, error) {
				staged := args[len(args)-1]
				parent, parentErr := os.Stat(filepath.Dir(filepath.Dir(staged)))
				rootInfo, rootErr := os.Stat(root)
				stagedInfo, stagedErr := os.Stat(staged)
				if parentErr != nil || rootErr != nil || stagedErr != nil || !os.SameFile(parent, rootInfo) || os.SameFile(before, stagedInfo) {
					t.Fatalf("signing must use a sibling staging directory: %s", staged)
				}
				if command == "xattr" {
					if name == "quarantine" {
						return nil, fail
					}
					return nil, nil
				}
				if command != "codesign" {
					t.Fatalf("unexpected repair command %s", command)
				}
				if name == "sign" {
					return []byte("fixture-failure"), fail
				}
				if data, err := os.ReadFile(binary); err != nil || string(data) != string(original) {
					t.Fatalf("target changed before signing completed: %q %v", data, err)
				}
				signed = true
				return nil, os.WriteFile(staged, []byte("signed fixture"), 0755)
			})
			switch name {
			case "lookup":
				testseam.Swap(t, &upgradeLookPath, func(string) (string, error) { return "", fail })
			case "read":
				testseam.Swap(t, &upgradeReadFile, func(string) ([]byte, error) { return nil, fail })
			case "mkdir":
				testseam.Swap(t, &upgradeMkdirTemp, func(string, string) (string, error) { return "", fail })
			case "write":
				testseam.Swap(t, &upgradeWriteFile, func(string, []byte, os.FileMode) error { return fail })
			case "rename":
				testseam.Swap(t, &upgradeRename, func(string, string) error { return fail })
			}
			err = repairDarwinBinary(binary)
			data, readErr := os.ReadFile(binary)
			if readErr != nil {
				t.Fatal(readErr)
			}
			after, statErr := os.Stat(binary)
			if statErr != nil {
				t.Fatal(statErr)
			}
			if name == "success" || name == "quarantine" {
				if err != nil || !signed || string(data) != "signed fixture" || os.SameFile(before, after) {
					t.Fatalf("repair did not atomically publish a fresh signed inode: %q %v", data, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "fixture-failure") || string(data) != string(original) || !os.SameFile(before, after) {
				t.Fatalf("failed repair changed the original binary or lost its error: %q %v", data, err)
			}
			if stages, err := filepath.Glob(filepath.Join(root, ".dws-codesign-*")); err != nil || len(stages) != 0 {
				t.Fatalf("repair left staging files: %v %v", stages, err)
			}
		})
	}
}

func TestCrossPlatformCoverageUpgradeDarwinPreservesRepairFailure(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "dws")
	if err := os.WriteFile(binary, []byte("fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &upgradeRuntimeGOOS, "darwin")
	calls := 0
	testseam.Swap(t, &upgradeTryExecVersion, func(string) ([]byte, error) {
		calls++
		return nil, errors.New("signal: killed")
	})
	failure := errors.New("signing fixture failure")
	testseam.Swap(t, &upgradeRepairDarwin, func(string) error { return failure })
	err := validateNewBinary(binary, "dev")
	if !errors.Is(err, failure) || calls != 1 || strings.Contains(err.Error(), "signal: killed") {
		t.Fatalf("repair failure lost its cause or retried an unrepaired binary: %v, calls=%d", err, calls)
	}
}
