package runtimepayload

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/lock"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageAdjacentReuseRepairAndUpgrade(t *testing.T) {
	source := writePayloadFixture(t, "linux", "amd64")
	container, err := BuildContainer(source, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	path, err := MaterializeAdjacent(container, root, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != root {
		t.Fatal("not adjacent")
	}
	before, _ := os.Stat(path)
	if _, err = MaterializeAdjacent(container, root, "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if !os.SameFile(before, after) {
		t.Fatal("valid resource was replaced")
	}
	for _, scenario := range []string{"library", "ps", "version", "pending"} {
		t.Run(scenario, func(t *testing.T) {
			switch scenario {
			case "library":
				if err := os.WriteFile(path, []byte("damaged"), 0600); err != nil {
					t.Fatal(err)
				}
			case "ps":
				entries, _ := os.ReadDir(filepath.Join(root, "ps"))
				if err := os.Remove(filepath.Join(root, "ps", entries[0].Name())); err != nil {
					t.Fatal(err)
				}
			default:
				current, _, err := readOwnership(root, "linux", "amd64")
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "version" {
					current.Manifest.PayloadVersion = "20260825"
				} else {
					current.State = "pending"
				}
				data, _ := json.Marshal(current)
				if err := os.WriteFile(filepath.Join(root, ownershipName), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err != nil {
				t.Fatal(err)
			}
			current, owned, err := readOwnership(root, "linux", "amd64")
			if err != nil || !owned || current.State != "ready" || validateRootManifest(root, current.Manifest, "linux", "amd64") != nil {
				t.Fatal("repair did not commit validated resources")
			}
		})
	}
}

func TestCrossPlatformCoverageAdjacentProtectsUnknownResources(t *testing.T) {
	container, err := BuildContainer(writePayloadFixture(t, "linux", "amd64"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{"libx7k2m9p4q1w8.so", "ps", ownershipName, ".dws-runtime.lock"} {
		t.Run(entry, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, entry)
			if entry == "ps" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(path, "user-file")
			}
			if entry == ".dws-runtime.lock" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte("user data"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err == nil {
				t.Fatal("unknown resource accepted")
			}
			if entry != ".dws-runtime.lock" {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "user data" {
					t.Fatal("user file changed")
				}
			}
		})
	}
	t.Run("owned symlink", func(t *testing.T) {
		root := t.TempDir()
		path, err := MaterializeAdjacent(container, root, "linux", "amd64")
		if err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(t.TempDir(), "user-file")
		if err := os.WriteFile(outside, []byte("user data"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, path); err != nil {
			t.Skip("symlinks unavailable")
		}
		if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err == nil {
			t.Fatal("symlink accepted")
		}
		if _, err := os.Readlink(path); err != nil {
			t.Fatal("symlink replaced")
		}
	})
}

func TestCrossPlatformCoverageAdjacentInterruptedPublication(t *testing.T) {
	container, err := BuildContainer(writePayloadFixture(t, "linux", "amd64"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for failAt := 1; failAt <= 4; failAt++ {
		t.Run(string(rune('0'+failAt)), func(t *testing.T) {
			root := t.TempDir()
			t.Run("interrupt", func(t *testing.T) {
				calls := 0
				testseam.Swap(t, &renameAdjacent, func(from, to string) error {
					calls++
					if calls == failAt {
						return errors.New("sensitive/path")
					}
					return os.Rename(from, to)
				})
				if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err == nil || strings.Contains(err.Error(), "sensitive") {
					t.Fatal("publication must fail with neutral error")
				}
			})
			if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err != nil {
				t.Fatal("interrupted publication not recovered", err)
			}
		})
	}
}

func TestCrossPlatformCoverageAdjacentConcurrentAndBusy(t *testing.T) {
	container, err := BuildContainer(writePayloadFixture(t, "linux", "amd64"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	held, err := lock.TryAcquire(filepath.Join(root, ".dws-runtime.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err == nil {
		t.Fatal("busy directory accepted")
	}
	held.Close()
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			path, err := MaterializeAdjacent(container, root, "linux", "amd64")
			if err == nil && filepath.Dir(path) != root {
				t.Error("wrong directory")
			}
		}()
	}
	wait.Wait()
	if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageAdjacentReadOnlyAndInvalidContainer(t *testing.T) {
	container, err := BuildContainer(writePayloadFixture(t, "linux", "amd64"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MaterializeAdjacent(container, "relative", "linux", "amd64"); err == nil {
		t.Fatal("relative directory accepted")
	}
	if _, err := MaterializeAdjacent([]byte("invalid"), t.TempDir(), "linux", "amd64"); err == nil {
		t.Fatal("invalid container accepted")
	}
	if _, err := MaterializeAdjacent(container, t.TempDir(), "plan9", "amd64"); err == nil {
		t.Fatal("unsupported target accepted")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Unix permissions")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(root, 0700) })
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err == nil {
		t.Fatal("read-only directory accepted")
	}
}

func TestCrossPlatformCoverageAdjacentFailureBoundaries(t *testing.T) {
	container, err := BuildContainer(writePayloadFixture(t, "linux", "amd64"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("private failure")
	for _, scenario := range []string{"root", "lock stat", "walk", "temp", "extract", "stage target", "old rename", "target stat", "final validation", "nested symlink"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			if scenario == "walk" || scenario == "old rename" || scenario == "nested symlink" {
				if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "libx7k2m9p4q1w8.so"), []byte("damaged"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			goos, goarch := "linux", "amd64"
			switch scenario {
			case "root":
				root = filepath.Join(root, "missing")
			case "lock stat":
				testseam.Swap(t, &lstatAdjacent, func(path string) (os.FileInfo, error) {
					if strings.HasSuffix(path, ".lock") {
						return nil, failure
					}
					return os.Lstat(path)
				})
			case "walk":
				testseam.Swap(t, &walkAdjacent, func(path string, fn fs.WalkDirFunc) error { return fn(path, nil, failure) })
			case "temp":
				testseam.Swap(t, &makeCacheTemporary, func(string, string) (string, error) { return "", failure })
			case "extract":
				testseam.Swap(t, &extractPayload, func(io.Reader, string) error { return failure })
			case "stage target":
				goos = "darwin"
			case "old rename":
				testseam.Swap(t, &renameAdjacent, func(from, to string) error {
					if strings.Contains(to, "old-") {
						return failure
					}
					return os.Rename(from, to)
				})
			case "target stat":
				testseam.Swap(t, &lstatAdjacent, func(path string) (os.FileInfo, error) {
					if filepath.Base(path) == "libx7k2m9p4q1w8.so" {
						if _, err := os.Stat(filepath.Join(root, ownershipName)); err == nil {
							return nil, failure
						}
					}
					return os.Lstat(path)
				})
			case "final validation":
				testseam.Swap(t, &renameAdjacent, func(from, to string) error {
					if err := os.Rename(from, to); err != nil {
						return err
					}
					if to == filepath.Join(root, "ps") {
						return os.WriteFile(filepath.Join(root, "libx7k2m9p4q1w8.so"), []byte("damaged"), 0600)
					}
					return nil
				})
			case "nested symlink":
				entries, _ := os.ReadDir(filepath.Join(root, "ps"))
				path := filepath.Join(root, "ps", entries[0].Name())
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "libx7k2m9p4q1w8.so"), path); err != nil {
					t.Skip("symlink unavailable")
				}
			}
			if _, err := MaterializeAdjacent(container, root, goos, goarch); err == nil || strings.Contains(err.Error(), "private") {
				t.Fatal("failure must stay unavailable and redacted")
			}
		})
	}
}

func TestCrossPlatformCoverageOwnershipValidation(t *testing.T) {
	container, err := BuildContainer(writePayloadFixture(t, "linux", "amd64"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err != nil {
		t.Fatal(err)
	}
	original, _, err := readOwnership(root, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"directory", "large", "unreadable", "owner", "digest", "trailing JSON"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ownershipName)
			value := original
			if scenario == "owner" {
				value.Owner = "foreign"
			}
			if scenario == "digest" {
				value.PayloadSHA256 = "invalid"
			}
			data, _ := json.Marshal(value)
			if scenario == "large" {
				data = make([]byte, 8193)
			}
			if scenario == "trailing JSON" {
				data = append(data, []byte(" {}")...)
			}
			if scenario == "directory" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if scenario == "unreadable" {
				testseam.Swap(t, &readManifestFile, func(string) ([]byte, error) { return nil, errors.New("unreadable") })
			}
			if _, _, err := readOwnership(dir, "linux", "amd64"); err == nil {
				t.Fatal("invalid ownership accepted")
			}
		})
	}
	if err := writeOwnership(root, filepath.Join(root, "missing"), original); err == nil {
		t.Fatal("manifest write failure ignored")
	}
}

func TestCrossPlatformCoverageAdjacentProcessLock(t *testing.T) {
	if root := os.Getenv("DWS_RUNTIME_LOCK_TEST_ROOT"); root != "" {
		held, err := lock.TryAcquire(filepath.Join(root, ".dws-runtime.lock"))
		if err != nil {
			t.Fatal(err)
		}
		defer held.Close()
		fmt.Fprintln(os.Stdout, "locked")
		io.Copy(io.Discard, os.Stdin)
		return
	}
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestCrossPlatformCoverageAdjacentProcessLock$")
	command.Env = append(os.Environ(), "DWS_RUNTIME_LOCK_TEST_ROOT="+root)
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { input.Close(); command.Wait() })
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatal("child failed to acquire lock")
	}
	container, err := BuildContainer(writePayloadFixture(t, "linux", "amd64"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err == nil {
		t.Fatal("cross-process lock was ignored")
	}
	input.Close()
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := MaterializeAdjacent(container, root, "linux", "amd64"); err != nil {
		t.Fatal("released process lock remained busy")
	}
}
