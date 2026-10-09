package upgrade

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageSkillPublicationNoClobberAndOwnedRollback(t *testing.T) {
	t.Run("existing destination is never replaced", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		destination := filepath.Join(root, "destination")
		seedUpgradeSkill(t, staged, "new", false)
		seedUpgradeSkill(t, destination, "concurrent", false)

		if _, err := PublishSkillPathNoReplace(staged, destination); err == nil {
			t.Fatal("expected no-replace publication to fail")
		}
		assertUpgradeSkillContent(t, destination, "concurrent")
		assertUpgradeSkillContent(t, staged, "new")
	})

	t.Run("owned publication is removed", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		destination := filepath.Join(root, "destination")
		seedUpgradeSkill(t, staged, "new", false)
		publication, err := PublishSkillPathNoReplace(staged, destination)
		if err != nil {
			t.Fatal(err)
		}
		if err := RollbackSkillPathPublications([]SkillPathPublication{publication}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(destination); !os.IsNotExist(err) {
			t.Fatalf("owned publication remains: %v", err)
		}
	})

	t.Run("concurrent replacement is quarantined instead of deleted", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		destination := filepath.Join(root, "destination")
		seedUpgradeSkill(t, staged, "new", false)
		publication, err := PublishSkillPathNoReplace(staged, destination)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(destination); err != nil {
			t.Fatal(err)
		}
		// Even a byte-for-byte identical replacement is not owned by this
		// transaction: inode identity, not content alone, is authoritative.
		seedUpgradeSkill(t, destination, "new", false)
		replacement, statErr := os.Lstat(destination)
		if statErr != nil {
			t.Fatal(statErr)
		}
		t.Logf("replacement fixture: same_file=%t, original=%+v, replacement=%+v", os.SameFile(publication.identity, replacement), publication.identity.Sys(), replacement.Sys())

		err = RollbackSkillPathPublications([]SkillPathPublication{publication})
		if err == nil || !strings.Contains(err.Error(), "拒绝删除非本事务") {
			t.Fatalf("replacement rollback error = %v", err)
		}
		assertUpgradeSkillContent(t, destination, "new")
	})

	t.Run("tunneled replacement with same creation time is still refused", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		destination := filepath.Join(root, "destination")
		seedUpgradeSkill(t, staged, "new", false)
		// NTFS file tunneling can restore the original creation time for a
		// same-named recreation, defeating the incarnation check. The file ID
		// (or inode on Unix) must still differ and block the rollback.
		testseam.Swap(t, &skillPathFileIncarnation, func(os.FileInfo) string { return "tunneled" })
		publication, err := PublishSkillPathNoReplace(staged, destination)
		if err != nil {
			t.Fatal(err)
		}
		// Simulate the identity change that a concurrent replacement causes.
		// On Windows, skillPathIdentityProven compares the recorded file ID
		// from GetFileInformationByHandle with the live one and ignores the
		// SameFile seam; NTFS can reuse the freed MFT record for the
		// recreation, so the live probe is perturbed through the file-ID
		// seam as well. On Unix, where the file ID is advisory, this swap
		// replaces the os.SameFile check (which can return true for a
		// recreated file on tmpfs due to inode reuse) so the test is
		// deterministic across filesystems.
		testseam.Swap(t, &skillPathSameFileIdentity, func(_, _ os.FileInfo) bool { return false })
		originalFileIdentity := skillPathFileIdentity
		testseam.Swap(t, &skillPathFileIdentity, func(path string) string {
			return originalFileIdentity(path) + ":replaced"
		})
		if err := os.RemoveAll(destination); err != nil {
			t.Fatal(err)
		}
		seedUpgradeSkill(t, destination, "new", false)

		err = RollbackSkillPathPublications([]SkillPathPublication{publication})
		if err == nil || !strings.Contains(err.Error(), "拒绝删除非本事务") {
			t.Fatalf("tunneled replacement rollback error = %v", err)
		}
		assertUpgradeSkillContent(t, destination, "new")
	})
}

func TestCrossPlatformCoverageSkillPublicationOptionalPin(t *testing.T) {
	// 非 Linux 平台没有新增句柄；原来的身份与内容检查仍然是必需的。
	if !skillPathPinnedIdentityMatches(nil, nil) {
		t.Fatal("optional pin must preserve existing platform identity checks")
	}
	if err := ReleaseSkillPathPublications([]SkillPathPublication{{}}); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageSkillPublicationFailureEdges(t *testing.T) {
	failure := errors.New("injected publication failure")

	t.Run("platform no-replace path encoding", func(t *testing.T) {
		if _, err := renameSkillPathNoReplace("invalid\x00source", filepath.Join(t.TempDir(), "dest")); err == nil {
			t.Fatal("expected invalid source path failure")
		}
		source := filepath.Join(t.TempDir(), "source")
		if err := os.WriteFile(source, []byte("value"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := renameSkillPathNoReplace(source, "invalid\x00destination"); err == nil {
			t.Fatal("expected invalid destination path failure")
		}
	})

	t.Run("publish identity", func(t *testing.T) {
		if _, err := PublishSkillPathNoReplace(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "dest")); err == nil {
			t.Fatal("expected identity failure")
		}
	})

	t.Run("publish fingerprint", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		seedUpgradeSkill(t, staged, "value", false)
		originalLstat := skillPathLstat
		calls := 0
		testseam.Swap(t, &skillPathLstat, func(path string) (os.FileInfo, error) {
			calls++
			if calls == 2 {
				return nil, failure
			}
			return originalLstat(path)
		})
		if _, err := PublishSkillPathNoReplace(staged, filepath.Join(root, "dest")); !errors.Is(err, failure) {
			t.Fatal("expected fingerprint failure")
		}
	})

	t.Run("publish confirmation missing", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		destination := filepath.Join(root, "destination")
		seedUpgradeSkill(t, staged, "value", false)
		originalRename := skillPathRenameNoReplace
		testseam.Swap(t, &skillPathRenameNoReplace, func(source, target string) (string, error) {
			if _, err := originalRename(source, target); err != nil {
				return "", err
			}
			return "", os.RemoveAll(target)
		})
		if _, err := PublishSkillPathNoReplace(staged, destination); err == nil || !strings.Contains(err.Error(), "对象保留") {
			t.Fatalf("publish confirmation error = %v", err)
		}
	})

	t.Run("publish confirmation identity changed", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		destination := filepath.Join(root, "destination")
		seedUpgradeSkill(t, staged, "value", false)
		// A real rename consumed the staged path (fast path), but the object
		// at the destination must be proven to be the staged one. Whether a
		// same-content swap is observable through inode/file-ID comparison
		// depends on the filesystem — CI runners' ext4/overlayfs recycle
		// inodes eagerly and NTFS can immediately reuse the freed MFT record,
		// so reproducing the swap physically is not portable. Force the
		// failure through both underlying primitives the platform-specific
		// proof consults: os.SameFile (Unix) and the stable file ID
		// (Windows), pinning the confirmation's rejection on every platform.
		testseam.Swap(t, &skillPathSameFileIdentity, func(_, _ os.FileInfo) bool { return false })
		originalFileIdentity := skillPathFileIdentity
		stagedID := ""
		testseam.Swap(t, &skillPathFileIdentity, func(path string) string {
			id := originalFileIdentity(path)
			if stagedID == "" {
				stagedID = id
				return id
			}
			// The published probe must report a different object than the
			// staged one so the proof fails on both platforms.
			return id + ":swapped"
		})
		if _, err := PublishSkillPathNoReplace(staged, destination); err == nil || !strings.Contains(err.Error(), "staging 身份已变化") {
			t.Fatalf("publish identity error = %v", err)
		}
		assertUpgradeSkillContent(t, destination, "value")
	})

	t.Run("publish confirmation fingerprint", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		destination := filepath.Join(root, "destination")
		seedUpgradeSkill(t, staged, "value", false)
		originalRename := skillPathRenameNoReplace
		published := false
		testseam.Swap(t, &skillPathRenameNoReplace, func(source, target string) (string, error) {
			if _, err := originalRename(source, target); err != nil {
				return "", err
			}
			published = true
			return "", nil
		})
		originalReadDir := skillPathReadDir
		testseam.Swap(t, &skillPathReadDir, func(path string) ([]os.DirEntry, error) {
			if published && path == destination {
				return nil, failure
			}
			return originalReadDir(path)
		})
		if _, err := PublishSkillPathNoReplace(staged, destination); !errors.Is(err, failure) || !strings.Contains(err.Error(), "内容失败") {
			t.Fatalf("publish fingerprint confirmation error = %v", err)
		}
	})

	t.Run("rollback temp", func(t *testing.T) {
		testseam.Swap(t, &skillPathMkdirTemp, func(string, string) (string, error) { return "", failure })
		if err := RollbackSkillPathPublications([]SkillPathPublication{{Destination: "dest"}}); !errors.Is(err, failure) {
			t.Fatalf("temp error = %v", err)
		}
	})

	t.Run("missing destination cleanup", func(t *testing.T) {
		if err := RollbackSkillPathPublications([]SkillPathPublication{{Destination: filepath.Join(t.TempDir(), "missing")}}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("missing destination cleanup failure", func(t *testing.T) {
		testseam.Swap(t, &skillPathRemoveAll, func(string) error { return failure })
		if err := RollbackSkillPathPublications([]SkillPathPublication{{Destination: filepath.Join(t.TempDir(), "missing")}}); !errors.Is(err, failure) {
			t.Fatalf("cleanup error = %v", err)
		}
	})

	t.Run("quarantine rename", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		destination := filepath.Join(root, "destination")
		seedUpgradeSkill(t, staged, "value", false)
		publication, err := PublishSkillPathNoReplace(staged, destination)
		if err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &skillPathRename, func(string, string) error { return failure })
		if err := RollbackSkillPathPublications([]SkillPathPublication{publication}); !errors.Is(err, failure) {
			t.Fatalf("quarantine error = %v", err)
		}
	})

	t.Run("live verification", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		destination := filepath.Join(root, "destination")
		seedUpgradeSkill(t, staged, "value", false)
		publication, err := PublishSkillPathNoReplace(staged, destination)
		if err != nil {
			t.Fatal(err)
		}
		originalReadDir := skillPathReadDir
		testseam.Swap(t, &skillPathReadDir, func(path string) ([]os.DirEntry, error) {
			if path == destination {
				return nil, failure
			}
			return originalReadDir(path)
		})
		if err := RollbackSkillPathPublications([]SkillPathPublication{publication}); !errors.Is(err, failure) {
			t.Fatalf("live verification error = %v", err)
		}
	})

	t.Run("destination disappears before quarantine", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		destination := filepath.Join(root, "destination")
		seedUpgradeSkill(t, staged, "value", false)
		publication, err := PublishSkillPathNoReplace(staged, destination)
		if err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &skillPathRename, func(source, target string) error {
			if err := os.RemoveAll(source); err != nil {
				return err
			}
			return &os.PathError{Op: "rename", Path: source, Err: os.ErrNotExist}
		})
		if err := RollbackSkillPathPublications([]SkillPathPublication{publication}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("owned remove", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		destination := filepath.Join(root, "destination")
		seedUpgradeSkill(t, staged, "value", false)
		publication, err := PublishSkillPathNoReplace(staged, destination)
		if err != nil {
			t.Fatal(err)
		}
		originalRemove := skillPathRemoveAll
		testseam.Swap(t, &skillPathRemoveAll, func(path string) error {
			if filepath.Base(path) == "payload" {
				return failure
			}
			return originalRemove(path)
		})
		if err := RollbackSkillPathPublications([]SkillPathPublication{publication}); !errors.Is(err, failure) {
			t.Fatalf("remove error = %v", err)
		}
	})

	t.Run("quarantined fingerprint", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		destination := filepath.Join(root, "destination")
		seedUpgradeSkill(t, staged, "value", false)
		publication, err := PublishSkillPathNoReplace(staged, destination)
		if err != nil {
			t.Fatal(err)
		}
		originalLstat := skillPathLstat
		testseam.Swap(t, &skillPathLstat, func(path string) (os.FileInfo, error) {
			if filepath.Base(path) == "payload" {
				return nil, failure
			}
			return originalLstat(path)
		})
		if err := RollbackSkillPathPublications([]SkillPathPublication{publication}); !errors.Is(err, failure) {
			t.Fatalf("fingerprint error = %v", err)
		}
	})

	t.Run("mismatch restore and cleanup", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		destination := filepath.Join(root, "destination")
		seedUpgradeSkill(t, staged, "published", false)
		publication, err := PublishSkillPathNoReplace(staged, destination)
		if err != nil {
			t.Fatal(err)
		}
		originalRename := skillPathRename
		testseam.Swap(t, &skillPathRename, func(source, target string) error {
			if source == destination {
				if err := os.RemoveAll(source); err != nil {
					return err
				}
				seedUpgradeSkill(t, source, "replacement", false)
			}
			return originalRename(source, target)
		})
		originalRemove := skillPathRemoveAll
		testseam.Swap(t, &skillPathRemoveAll, func(path string) error {
			if strings.Contains(filepath.Base(path), ".destination.rollback-") {
				return failure
			}
			return originalRemove(path)
		})
		if err := RollbackSkillPathPublications([]SkillPathPublication{publication}); !errors.Is(err, failure) {
			t.Fatalf("mismatch cleanup error = %v", err)
		}
		assertUpgradeSkillContent(t, destination, "replacement")
	})

	t.Run("mismatch restore failure", func(t *testing.T) {
		root := t.TempDir()
		staged := filepath.Join(root, "staged")
		destination := filepath.Join(root, "destination")
		seedUpgradeSkill(t, staged, "published", false)
		publication, err := PublishSkillPathNoReplace(staged, destination)
		if err != nil {
			t.Fatal(err)
		}
		originalRename := skillPathRename
		testseam.Swap(t, &skillPathRename, func(source, target string) error {
			if source == destination {
				if err := os.RemoveAll(source); err != nil {
					return err
				}
				seedUpgradeSkill(t, source, "replacement", false)
			}
			return originalRename(source, target)
		})
		testseam.Swap(t, &skillPathRenameNoReplace, func(string, string) (string, error) { return "", failure })
		if err := RollbackSkillPathPublications([]SkillPathPublication{publication}); !errors.Is(err, failure) || !strings.Contains(err.Error(), "并发对象保留") {
			t.Fatalf("mismatch restore error = %v", err)
		}
	})

	t.Run("fingerprint read failures and lexical types", func(t *testing.T) {
		t.Run("directory ordering", func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range []string{"b", "a"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := fingerprintSkillPath(dir); err != nil {
				t.Fatal(err)
			}
		})
		t.Run("directory read", func(t *testing.T) {
			dir := t.TempDir()
			testseam.Swap(t, &skillPathReadDir, func(string) ([]os.DirEntry, error) { return nil, failure })
			if _, err := fingerprintSkillPath(dir); !errors.Is(err, failure) {
				t.Fatalf("read dir error = %v", err)
			}
		})
		t.Run("recursive child", func(t *testing.T) {
			dir := t.TempDir()
			child := filepath.Join(dir, "child")
			if err := os.WriteFile(child, []byte("value"), 0o644); err != nil {
				t.Fatal(err)
			}
			originalLstat := skillPathLstat
			testseam.Swap(t, &skillPathLstat, func(path string) (os.FileInfo, error) {
				if path == child {
					return nil, failure
				}
				return originalLstat(path)
			})
			if _, err := fingerprintSkillPath(dir); !errors.Is(err, failure) {
				t.Fatalf("child error = %v", err)
			}
		})
		t.Run("symlink read", func(t *testing.T) {
			link := filepath.Join(t.TempDir(), "link")
			if err := os.Symlink("missing", link); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			testseam.Swap(t, &skillPathReadlink, func(string) (string, error) { return "", failure })
			if _, err := fingerprintSkillPath(link); !errors.Is(err, failure) {
				t.Fatalf("readlink error = %v", err)
			}
		})
		t.Run("symlink success", func(t *testing.T) {
			link := filepath.Join(t.TempDir(), "link")
			if err := os.Symlink("missing", link); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			if _, err := fingerprintSkillPath(link); err != nil {
				t.Fatal(err)
			}
		})
		t.Run("file digest", func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "file")
			if err := os.WriteFile(file, []byte("value"), 0o644); err != nil {
				t.Fatal(err)
			}
			testseam.Swap(t, &skillPathOpen, func(string) (*os.File, error) { return nil, failure })
			if _, err := fingerprintSkillPath(file); !errors.Is(err, failure) {
				t.Fatalf("digest error = %v", err)
			}
		})
		t.Run("special", func(t *testing.T) {
			testseam.Swap(t, &skillPathLstat, func(string) (os.FileInfo, error) {
				return skillPathFakeInfo{mode: os.ModeNamedPipe}, nil
			})
			if err := fingerprintSkillPathInto(sha256.New(), "special"); err == nil {
				t.Fatal("expected special path error")
			}
		})
		t.Run("unknown platform identity shape", func(t *testing.T) {
			if got := skillPathFileIncarnation(skillPathFakeInfo{}); got == "" {
				t.Fatal("empty incarnation")
			}
		})
		t.Run("synthetic info does not match same-file identity", func(t *testing.T) {
			fake := skillPathFakeInfo{}
			if skillPathSameFileIdentity(fake, fake) {
				t.Fatal("synthetic info should not match")
			}
		})
	})
}

func TestCrossPlatformCoverageUpgradeRollbackRetainsConcurrentReplacement(t *testing.T) {
	home := t.TempDir()
	base := filepath.Join(home, ".agents", "skills")
	first := filepath.Join(base, "dingtalk-first")
	second := filepath.Join(base, "dingtalk-second")
	seedUpgradeSkill(t, first, "old first", false)
	seedUpgradeSkill(t, second, "old second", false)
	stageRoot := filepath.Join(base, ".stage")
	stagedFirst := filepath.Join(stageRoot, "dingtalk-first")
	stagedSecond := filepath.Join(stageRoot, "dingtalk-second")
	seedUpgradeSkill(t, stagedFirst, "new first", false)
	seedUpgradeSkill(t, stagedSecond, "new second", false)

	failure := errors.New("injected second publication failure")
	originalPublish := upgradePublishSkillPath
	calls := 0
	testseam.Swap(t, &upgradePublishSkillPath, func(staged, destination string) (SkillPathPublication, error) {
		calls++
		if calls == 2 {
			if err := os.RemoveAll(first); err != nil {
				t.Fatal(err)
			}
			seedUpgradeSkill(t, first, "concurrent", false)
			return SkillPathPublication{}, failure
		}
		return originalPublish(staged, destination)
	})

	err := publishStagedSkillSet(home, []stagedSkillDir{
		{staged: stagedFirst, dest: first},
		{staged: stagedSecond, dest: second},
	}, []string{first, second})
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "拒绝删除非本事务") {
		t.Fatalf("transaction error = %v", err)
	}
	assertUpgradeSkillContent(t, first, "concurrent")
	assertUpgradeSkillContent(t, second, "old second")
	matches, globErr := filepath.Glob(filepath.Join(home, ".dws", "skill-backups", "*", ".agents-skills-dingtalk-first", "SKILL.md"))
	if globErr != nil || len(matches) != 1 {
		t.Fatalf("retained old backup = %v, %v", matches, globErr)
	}
	content, readErr := os.ReadFile(matches[0])
	if readErr != nil || string(content) != "old first" {
		t.Fatalf("retained backup content = %q, %v", content, readErr)
	}
}

// 失败处理属于所有平台；只把真实 O_PATH/inode 生命周期验证留在 Linux 测试。
func TestCrossPlatformCoverageSkillPublicationPinFailure(t *testing.T) {
	for _, failure := range []string{"open", "closed", "confirmation"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			staged, destination := filepath.Join(root, "staged"), filepath.Join(root, "destination")
			if err := os.WriteFile(staged, []byte("new"), 0o600); err != nil {
				t.Fatal(err)
			}
			var held *os.File
			t.Cleanup(func() {
				if held != nil {
					_ = held.Close()
				}
			})
			testseam.Swap(t, &skillPathPinIdentity, func(path string) (*os.File, error) {
				if failure == "open" {
					return nil, os.ErrPermission
				}
				var err error
				// 在发布改名完成后打开目标，Windows 也无需持句柄删除或改名。
				held, err = os.Open(path)
				if err != nil {
					return nil, err
				}
				if failure == "closed" {
					if err := held.Close(); err != nil {
						t.Fatal(err)
					}
				}
				if failure == "confirmation" {
					original := skillPathLstat
					testseam.Swap(t, &skillPathLstat, func(path string) (os.FileInfo, error) {
						if path == destination {
							return nil, os.ErrPermission
						}
						return original(path)
					})
				}
				return held, nil
			})
			_, err := PublishSkillPathNoReplace(staged, destination)
			want := error(os.ErrPermission)
			if failure == "closed" {
				want = ErrSkillPathPublicationUncertain
			}
			if !errors.Is(err, want) {
				t.Fatalf("publication failure = %v, want %v", err, want)
			}
			if content, err := os.ReadFile(destination); err != nil || string(content) != "new" {
				t.Fatalf("uncertain destination changed: content=%q err=%v", content, err)
			}
			if held != nil {
				// Stat 在 Windows 返回原生句柄错误；重复 Close 统一返回 ErrClosed。
				if err := held.Close(); !errors.Is(err, os.ErrClosed) {
					t.Fatalf("failed publication leaked pin: %v", err)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageSkillPublicationPinCloseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	pin, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pin.Close() })
	// 不直接关闭底层 FD，以免并发运行时复用编号后误关其他文件。
	failure := errors.New("injected close failure")
	testseam.Swap(t, &skillPathCloseIdentityPin, func(*os.File) error { return failure })
	if err := ReleaseSkillPathPublications([]SkillPathPublication{{identityPin: pin}}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "new" {
		t.Fatalf("release changed published content: content=%q err=%v", content, err)
	}
}
