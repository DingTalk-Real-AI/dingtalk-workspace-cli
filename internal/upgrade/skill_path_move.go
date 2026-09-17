package upgrade

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

var (
	skillPathRename           = func(src, dst string) error { return upgradeRename(src, dst) }
	skillPathRenameNoReplace  = renameSkillPathNoReplace
	skillPathCopy             = copySkillPathLexically
	skillPathVerify           = verifySkillPathCopy
	skillPathRemoveAll        = os.RemoveAll
	skillPathMkdirAll         = os.MkdirAll
	skillPathMkdirTemp        = os.MkdirTemp
	skillPathChmod            = os.Chmod
	skillPathLstat            = os.Lstat
	skillPathMkdir            = os.Mkdir
	skillPathLink             = os.Link
	skillPathRemove           = os.Remove
	skillPathReadDir          = os.ReadDir
	skillPathFileIdentity     = skillPathFileIdentityImpl
	skillPathFileIncarnation  = skillPathFileIncarnationImpl
	skillPathSameFileIdentity = skillPathSameFileIdentityImpl
	skillPathReadlink         = os.Readlink
	skillPathSymlink          = os.Symlink
	skillPathOpen             = os.Open
	skillPathOpenFile         = os.OpenFile
	skillPathCopyBytes        = io.Copy
	skillPathSync             = func(file *os.File) error { return file.Sync() }
	skillPathWalkDir          = filepath.WalkDir
)

// moveSkillPathRecoverably moves one managed Skill path without weakening the
// backup contract when source and destination are on different filesystems.
// The cross-device path publishes a fully copied and verified sibling staging
// path before removing the source. Any failure before source removal leaves the
// source intact; a removal failure deliberately leaves both verified copies.
func moveSkillPathRecoverably(src, dst string) (err error) {
	if _, statErr := skillPathLstat(dst); statErr == nil {
		return fmt.Errorf("目标已存在: %s", dst)
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("检查移动目标失败 %s: %w", dst, statErr)
	}
	if err := skillPathMkdirAll(filepath.Dir(dst), dirPermShared); err != nil {
		return fmt.Errorf("创建移动目标目录失败 %s: %w", filepath.Dir(dst), err)
	}
	// moveSkillPathRecoverably is not a publication path; ignore any
	// claim identity returned by the rename — no ownership check follows
	// on this destination.
	if _, renameErr := skillPathRenameNoReplace(src, dst); renameErr == nil {
		return nil
	} else if !isCrossDeviceError(renameErr) {
		return fmt.Errorf("移动 Skill 路径失败 %s -> %s: %w", src, dst, renameErr)
	}

	stageRoot, err := skillPathMkdirTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".cross-device-")
	if err != nil {
		return fmt.Errorf("创建跨设备 Skill staging 失败 %s: %w", dst, err)
	}
	stage := filepath.Join(stageRoot, "payload")
	stageCleaned := false
	defer func() {
		if stageCleaned {
			return
		}
		makeSkillPathTreeWritable(stageRoot)
		if cleanupErr := skillPathRemoveAll(stageRoot); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("清理跨设备 Skill staging 失败 %s: %w", stageRoot, cleanupErr))
		}
	}()

	if err := skillPathCopy(src, stage); err != nil {
		return fmt.Errorf("复制跨设备 Skill staging 失败 %s: %w", stage, err)
	}
	if err := skillPathVerify(src, stage); err != nil {
		return fmt.Errorf("校验跨设备 Skill staging 失败 %s: %w", stage, err)
	}
	stageInfo, err := skillPathLstat(stage)
	if err != nil {
		return fmt.Errorf("检查跨设备 Skill staging 失败 %s: %w", stage, err)
	}
	if stageInfo.IsDir() {
		// Darwin refuses to rename a read-only directory even when both parent
		// directories are writable. The exact source mode was verified above;
		// temporarily add owner access for publication, then restore it before
		// the final verification and before the source is removed.
		if err := skillPathChmod(stage, stageInfo.Mode().Perm()|0o700); err != nil {
			return fmt.Errorf("准备跨设备 Skill staging 发布失败 %s: %w", stage, err)
		}
	}
	// Cross-device staging publishes via a self-owned staging directory,
	// so the claim identity is not consulted; discard it.
	if _, err := skillPathRenameNoReplace(stage, dst); err != nil {
		return fmt.Errorf("发布跨设备 Skill 备份失败 %s: %w", dst, err)
	}
	if stageInfo.IsDir() {
		if err := skillPathChmod(dst, stageInfo.Mode().Perm()); err != nil {
			return fmt.Errorf("恢复已发布 Skill 目录权限失败 %s: %w", dst, err)
		}
	}
	if err := skillPathVerify(src, dst); err != nil {
		return fmt.Errorf("校验已发布 Skill 备份失败 %s: %w", dst, err)
	}
	if err := skillPathRemoveAll(stageRoot); err != nil {
		return fmt.Errorf("清理已发布 Skill staging 失败 %s: %w", stageRoot, err)
	}
	stageCleaned = true
	if err := removePublishedSkillSource(src); err != nil {
		return fmt.Errorf("Skill 目标已发布但源路径删除失败（源 %s 与目标 %s 均保留）: %w", src, dst, err)
	}
	return nil
}

type skillPathDirMode struct {
	path string
	mode os.FileMode
}

func removePublishedSkillSource(src string) error {
	dirModes, err := prepareSkillPathTreeRemoval(src)
	if err != nil {
		return err
	}
	removeErr := skillPathRemoveAll(src)
	if removeErr == nil {
		if _, statErr := skillPathLstat(src); os.IsNotExist(statErr) {
			return nil
		} else if statErr == nil {
			removeErr = errors.New("源路径仍存在")
		} else {
			removeErr = fmt.Errorf("无法确认源路径已删除: %w", statErr)
		}
	}
	return errors.Join(removeErr, restoreSkillPathDirModes(dirModes))
}

func prepareSkillPathTreeRemoval(root string) ([]skillPathDirMode, error) {
	var modes []skillPathDirMode
	err := skillPathWalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := info.Mode().Perm()
		if mode&0o700 == 0o700 {
			return nil
		}
		if err := skillPathChmod(path, mode|0o700); err != nil {
			return err
		}
		modes = append(modes, skillPathDirMode{path: path, mode: mode})
		return nil
	})
	if err != nil {
		return nil, errors.Join(err, restoreSkillPathDirModes(modes))
	}
	return modes, nil
}

func restoreSkillPathDirModes(modes []skillPathDirMode) error {
	var restoreErr error
	for i := len(modes) - 1; i >= 0; i-- {
		item := modes[i]
		if _, err := skillPathLstat(item.path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			restoreErr = errors.Join(restoreErr, err)
			continue
		}
		if err := skillPathChmod(item.path, item.mode); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("恢复源目录权限失败 %s: %w", item.path, err))
		}
	}
	return restoreErr
}

func makeSkillPathTreeWritable(root string) {
	// Best effort only: RemoveAll below remains the source of truth and its
	// error is returned. This preparation lets it traverse read-only staging
	// directories left by a copy or verification failure.
	_ = skillPathWalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr == nil && entry.IsDir() {
			_ = skillPathChmod(path, 0o700)
		}
		return nil
	})
}

// RestoreSkillPath restores a previously recorded backup using the same
// rename-first, verified cross-device fallback as backup publication.
func RestoreSkillPath(backup, original string) error {
	return moveSkillPathRecoverably(backup, original)
}

func copySkillPathLexically(src, dst string) error {
	info, err := skillPathLstat(src)
	if err != nil {
		return err
	}
	switch {
	case info.IsDir():
		// Staging must remain writable while children are copied. Restore the
		// source mode only after the directory is complete so read-only Skill
		// directories (for example 0555) can still be migrated lexically.
		if err := skillPathMkdir(dst, 0o700); err != nil {
			return err
		}
		entries, err := skillPathReadDir(src)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copySkillPathLexically(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return os.Chmod(dst, info.Mode().Perm())
	case info.Mode()&os.ModeSymlink != 0:
		target, err := skillPathReadlink(src)
		if err != nil {
			return err
		}
		return skillPathSymlink(target, dst)
	case info.Mode().IsRegular():
		return copyRegularSkillFile(src, dst, info.Mode().Perm())
	default:
		return fmt.Errorf("不支持复制特殊 Skill 路径 %s（mode=%s）", src, info.Mode())
	}
}

func copyRegularSkillFile(src, dst string, mode os.FileMode) (err error) {
	in, err := skillPathOpen(src)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, in.Close()) }()
	out, err := skillPathOpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	if _, err := skillPathCopyBytes(out, in); err != nil {
		return err
	}
	if err := skillPathSync(out); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

func verifySkillPathCopy(src, dst string) error {
	srcInfo, err := skillPathLstat(src)
	if err != nil {
		return err
	}
	dstInfo, err := skillPathLstat(dst)
	if err != nil {
		return err
	}
	if srcInfo.Mode()&os.ModeType != dstInfo.Mode()&os.ModeType {
		return fmt.Errorf("路径类型不一致: %s (%s) != %s (%s)", src, srcInfo.Mode(), dst, dstInfo.Mode())
	}
	switch {
	case srcInfo.IsDir():
		if srcInfo.Mode().Perm() != dstInfo.Mode().Perm() {
			return fmt.Errorf("目录权限不一致: %s (%s) != %s (%s)", src, srcInfo.Mode().Perm(), dst, dstInfo.Mode().Perm())
		}
		srcNames, err := skillPathEntryNames(src)
		if err != nil {
			return err
		}
		dstNames, err := skillPathEntryNames(dst)
		if err != nil {
			return err
		}
		if len(srcNames) != len(dstNames) {
			return fmt.Errorf("目录项数量不一致: %s (%d) != %s (%d)", src, len(srcNames), dst, len(dstNames))
		}
		for i, name := range srcNames {
			if name != dstNames[i] {
				return fmt.Errorf("目录项不一致: %s != %s", name, dstNames[i])
			}
			if err := verifySkillPathCopy(filepath.Join(src, name), filepath.Join(dst, name)); err != nil {
				return err
			}
		}
		return nil
	case srcInfo.Mode()&os.ModeSymlink != 0:
		srcTarget, err := skillPathReadlink(src)
		if err != nil {
			return err
		}
		dstTarget, err := skillPathReadlink(dst)
		if err != nil {
			return err
		}
		if srcTarget != dstTarget {
			return fmt.Errorf("符号链接目标不一致: %q != %q", srcTarget, dstTarget)
		}
		return nil
	case srcInfo.Mode().IsRegular():
		if srcInfo.Mode().Perm() != dstInfo.Mode().Perm() {
			return fmt.Errorf("文件权限不一致: %s (%s) != %s (%s)", src, srcInfo.Mode().Perm(), dst, dstInfo.Mode().Perm())
		}
		if srcInfo.Size() != dstInfo.Size() {
			return fmt.Errorf("文件大小不一致: %s (%d) != %s (%d)", src, srcInfo.Size(), dst, dstInfo.Size())
		}
		srcDigest, err := skillPathFileDigest(src)
		if err != nil {
			return err
		}
		dstDigest, err := skillPathFileDigest(dst)
		if err != nil {
			return err
		}
		if srcDigest != dstDigest {
			return fmt.Errorf("文件内容摘要不一致: %s != %s", src, dst)
		}
		return nil
	default:
		return fmt.Errorf("不支持校验特殊 Skill 路径 %s（mode=%s）", src, srcInfo.Mode())
	}
}

func skillPathEntryNames(dir string) ([]string, error) {
	entries, err := skillPathReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func skillPathFileDigest(path string) ([sha256.Size]byte, error) {
	f, err := skillPathOpen(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := skillPathCopyBytes(hash, f); err != nil {
		return [sha256.Size]byte{}, err
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}
