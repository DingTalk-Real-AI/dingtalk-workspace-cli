package upgrade

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// errNoReplaceRenameUnsupported marks a platform that has no atomic no-replace
// rename primitive at all.
var errNoReplaceRenameUnsupported = errors.New("平台不支持原子 no-replace rename")

var skillPathRenameNoReplaceAtomic = renameSkillPathNoReplaceAtomic

// renameSkillPathNoReplace moves source onto destination and never replaces an
// object that already occupies destination. On success it also returns the
// fresh mkdir-claim identity captured by the child-move directory fallback
// (empty when the atomic primitive or the empty-target fast path published
// dest, or when the platform cannot report a stable file identity). The
// publication path uses that identity as a tamperproof witness that dest is
// still the mkdir claim this transaction created; every other caller ignores
// it.
//
// The atomic primitives (Linux RENAME_NOREPLACE, Darwin RENAME_EXCL) require
// support from the underlying filesystem and report EINVAL/ENOTSUP when it is
// missing — rename(2) lists only ext4, btrfs, tmpfs and cifs, so NFS, FUSE and
// overlayfs homes reject the flag. Failing there would abort the whole install
// on those filesystems, so the fallback uses truly atomic no-clobber primitives
// that work on every POSIX filesystem: os.Mkdir for directories (fails with
// EEXIST if the path is occupied), os.Link for regular files (fails with
// EEXIST if the path is occupied), and os.Symlink for symbolic links (same
// EEXIST guarantee). Neither primitive has a TOCTOU window.
func renameSkillPathNoReplace(source, destination string) (string, error) {
	err := skillPathRenameNoReplaceAtomic(source, destination)
	if err == nil {
		return "", nil
	}
	if !errors.Is(err, errNoReplaceRenameUnsupported) && !isNoReplaceRenameUnsupported(err) {
		return "", err
	}
	return renameSkillPathNoReplaceFallback(source, destination)
}

// renameSkillPathNoReplaceFallback is used when the kernel no-replace rename
// flag is unavailable. It dispatches on the source type to pick an atomic
// no-clobber primitive that does not require filesystem support. When the
// directory slow path publishes via child-move, the returned claim identity
// is the destination file identity captured immediately after mkdir; every
// other outcome returns an empty claim identity.
func renameSkillPathNoReplaceFallback(source, destination string) (string, error) {
	info, err := skillPathLstat(source)
	if err != nil {
		return "", fmt.Errorf("读取源 Skill 身份失败 %s: %w", source, err)
	}
	switch {
	case info.IsDir():
		return renameSkillDirNoReplaceFallback(source, destination)
	case info.Mode().IsRegular():
		return "", renameSkillFileNoReplaceFallback(source, destination)
	default:
		return "", &os.LinkError{
			Op: "rename", Old: source, New: destination,
			Err: fmt.Errorf("%w: 源路径类型不支持降级发布", errNoReplaceRenameUnsupported),
		}
	}
}

// renameSkillDirNoReplaceFallback claims the destination once with mkdir and
// never unlinks it for the remainder of the transaction. The fast path
// renames the source over the claim; on platforms that permit replacing an
// empty directory (Linux) that rename only ever touches the claim we own.
// Platforms that refuse to rename over any directory (macOS returns EEXIST
// even for an empty target) take the slow path instead — but only after
// proving the claim is still empty: a rename that failed because a
// concurrent writer landed inside the claim must abort with the destination
// retained, never fall through to child moves whose per-entry primitives
// would collide with (and rollback would delete) the foreign data. The slow
// path moves the source children into the claim one by one, each through an
// atomic no-clobber primitive (mkdir claim for directories, os.Link for
// files, os.Symlink for links), so a same-name concurrent entry aborts the
// move and a foreign entry of any name — detected by re-reading the claim —
// retains the destination instead of letting the rollback delete it. The
// destination claim is removed only while still empty, so a foreign entry
// always survives a failed publication. On success the source is fully
// consumed either way: the fast-path rename moves it onto the claim, and
// the child-move slow path removes the emptied source shell after the last
// child relocates.
//
// The returned claim identity is non-empty only when the child-move slow
// path published dest and the platform can report a stable file identity;
// the fast-path rename consumes the mkdir claim (dest inherits the source
// inode) and returns "", as does an unsuccessful publish. Callers that need
// a tamperproof ownership witness (see PublishSkillPathNoReplace) compare
// the returned identity against skillPathFileIdentity(destination) at the
// moment of the ownership check — a wholesale replacement of dest after
// the claim produces a different identity.
//
// The old fallback (mkdir → rename → remove → rename) had a TOCTOU window
// between the second remove and the second rename during which a concurrent
// writer could create a file or empty directory at dest that the second
// os.Rename would silently overwrite. This implementation removes that
// window: the claim is never unlinked between the fast-path rename attempt
// and publication, and child-move relies exclusively on atomic no-clobber
// primitives that fail with EEXIST rather than replace.
func renameSkillDirNoReplaceFallback(source, destination string) (string, error) {
	sourceInfo, err := skillPathLstat(source)
	if err != nil {
		return "", fmt.Errorf("读取源 Skill 身份失败 %s: %w", source, err)
	}
	if err := skillPathMkdir(destination, 0o700); err != nil {
		if os.IsExist(err) {
			return "", &os.LinkError{Op: "rename", Old: source, New: destination, Err: os.ErrExist}
		}
		return "", fmt.Errorf("声明 Skill 发布目标失败 %s: %w", destination, err)
	}
	// Capture the claim identity immediately after mkdir but before any
	// children move in. A concurrent replacement of destination (unlink +
	// recreate) after this point produces a different identity.
	claimID := skillPathFileIdentity(destination)
	if err := skillPathRename(source, destination); err == nil {
		// Fast path consumed the source: destination inode is now the
		// staged inode, not the mkdir claim. Return no claim identity —
		// the caller falls back to the standard staged-identity proof.
		return "", nil
	}
	claimEntries, claimErr := skillPathReadDir(destination)
	if claimErr != nil {
		return "", errors.Join(
			fmt.Errorf("降级发布 Skill 目录失败 %s: %w", destination, claimErr),
			removeClaimIfEmpty(destination),
		)
	}
	if len(claimEntries) > 0 {
		// A concurrent writer populated the empty claim before we could
		// take the slow path. Falling through to child-move would let
		// its per-entry no-clobber primitives collide with the foreign
		// data; abort with the destination retained instead.
		return "", fmt.Errorf("降级发布中止，目标在声明后被并发写入 %s: %w", destination, os.ErrExist)
	}
	if err := moveSkillDirChildrenIntoClaim(source, destination, sourceInfo.Mode().Perm()); err != nil {
		return "", fmt.Errorf("降级发布 Skill 目录失败 %s: %w", destination, err)
	}
	// Child-move consumed every source entry, so the emptied source shell is
	// ours by construction. os.Remove refuses a non-empty directory, so a
	// concurrent writer inside the staging tree fails the publication loudly
	// instead of being silently discarded with the shell.
	if err := skillPathRemove(source); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("清理已迁移 Skill 源目录失败 %s: %w", source, err)
	}
	// Child-move succeeded: dest is still the mkdir claim we created.
	return claimID, nil
}

// moveSkillDirChildNoReplace moves one child of a staged directory into the
// claimed destination through an atomic no-clobber primitive: directories
// recursively claim with mkdir, regular files publish via os.Link, and
// symlinks are recreated with os.Symlink. Every primitive fails with EEXIST
// when the destination entry was taken concurrently, so no step can replace
// a foreign object.
func moveSkillDirChildNoReplace(source, destination string) error {
	info, err := skillPathLstat(source)
	if err != nil {
		return fmt.Errorf("读取源 Skill 目录项失败 %s: %w", source, err)
	}
	switch {
	case info.IsDir():
		// The nested fallback consumes the nested source on success — the
		// fast-path rename moves it away and the child-move slow path
		// removes the emptied shell. The nested claim identity is not a
		// witness for this transaction (only the top-level destination's
		// identity is), so it is discarded here.
		_, err := renameSkillDirNoReplaceFallback(source, destination)
		return err
	case info.Mode().IsRegular():
		return renameSkillFileNoReplaceFallback(source, destination)
	case info.Mode()&os.ModeSymlink != 0:
		return renameSkillSymlinkNoReplaceFallback(source, destination)
	default:
		return &os.LinkError{
			Op: "rename", Old: source, New: destination,
			Err: fmt.Errorf("%w: 源路径类型不支持降级发布", errNoReplaceRenameUnsupported),
		}
	}
}

// removeClaimIfEmpty deletes the claimed destination only while it still
// holds no entries. A foreign entry that appeared after the claim must never
// be removed: the rollback has no ownership proof for it.
func removeClaimIfEmpty(destination string) error {
	entries, err := skillPathReadDir(destination)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("检查降级发布目标失败 %s: %w", destination, err)
	}
	if len(entries) > 0 {
		return nil
	}
	if err := skillPathRemove(destination); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("清理降级发布目标失败 %s: %w", destination, err)
	}
	return nil
}

// moveSkillDirChildrenIntoClaim moves every top-level child of source into
// the claimed destination — each through moveSkillDirChildNoReplace, so a
// concurrently created same-name entry fails atomically — and restores the
// source mode on the claim. The claim is re-read after the moves: an entry
// count above the source's means a different-named foreign object landed
// mid-move, and the publication aborts with the destination retained. Any
// failure moves the already relocated children back and removes the claim
// only while it is empty, leaving the source intact and foreign entries in
// place.
func moveSkillDirChildrenIntoClaim(source, destination string, sourceMode os.FileMode) error {
	entries, err := skillPathReadDir(source)
	if err != nil {
		return errors.Join(
			fmt.Errorf("读取源 Skill 目录失败 %s: %w", source, err),
			removeClaimIfEmpty(destination),
		)
	}
	moved := make([]string, 0, len(entries))
	rollback := func() error {
		var rollbackErr error
		for i := len(moved) - 1; i >= 0; i-- {
			if err := skillPathRename(filepath.Join(destination, moved[i]), filepath.Join(source, moved[i])); err != nil {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		}
		rollbackErr = errors.Join(rollbackErr, removeClaimIfEmpty(destination))
		return rollbackErr
	}
	for _, entry := range entries {
		name := entry.Name()
		if err := moveSkillDirChildNoReplace(filepath.Join(source, name), filepath.Join(destination, name)); err != nil {
			return errors.Join(fmt.Errorf("迁移 Skill 目录项失败 %s: %w", name, err), rollback())
		}
		moved = append(moved, name)
	}
	live, liveErr := skillPathReadDir(destination)
	if liveErr != nil {
		return errors.Join(
			fmt.Errorf("确认降级发布目标失败 %s: %w", destination, liveErr),
			rollback(),
		)
	}
	if len(live) > len(entries) {
		return errors.Join(
			fmt.Errorf("降级发布中止，目标出现并发目录项 %s: %w", destination, os.ErrExist),
			rollback(),
		)
	}
	if err := skillPathChmod(destination, sourceMode); err != nil {
		return errors.Join(
			fmt.Errorf("恢复已发布 Skill 目录权限失败 %s: %w", destination, err),
			rollback(),
		)
	}
	return nil
}

// renameSkillFileNoReplaceFallback uses os.Link, which atomically fails if the
// destination exists, then removes the source to complete the move.
func renameSkillFileNoReplaceFallback(source, destination string) error {
	if err := skillPathLink(source, destination); err != nil {
		if os.IsExist(err) {
			return &os.LinkError{Op: "rename", Old: source, New: destination, Err: os.ErrExist}
		}
		return fmt.Errorf("发布 Skill 文件失败 %s: %w", destination, err)
	}
	return skillPathRemove(source)
}

// renameSkillSymlinkNoReplaceFallback recreates the link at the destination
// with os.Symlink — atomic, EEXIST when taken — and then removes the source.
func renameSkillSymlinkNoReplaceFallback(source, destination string) error {
	target, err := skillPathReadlink(source)
	if err != nil {
		return fmt.Errorf("读取源 Skill 链接失败 %s: %w", source, err)
	}
	if err := skillPathSymlink(target, destination); err != nil {
		if os.IsExist(err) {
			return &os.LinkError{Op: "rename", Old: source, New: destination, Err: os.ErrExist}
		}
		return fmt.Errorf("发布 Skill 链接失败 %s: %w", destination, err)
	}
	return skillPathRemove(source)
}
