package upgrade

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"sort"
)

// ErrSkillPathPublicationUncertain marks a publication whose transactional
// outcome cannot be determined safely: this transaction did occupy the
// destination at some point, but a stronger witness — the fresh mkdir-claim
// identity captured by the child-move fallback — now indicates that the
// object currently at the destination may belong to a concurrent writer.
// Callers must not automatically retract, replace, or retry-copy over the
// destination; instead they should surface the state so the user or a
// higher layer can inspect it. Errors wrapping this sentinel are safe to
// test with errors.Is.
var ErrSkillPathPublicationUncertain = errors.New("Skill 发布状态不确定")

// SkillPathPublication records enough immutable identity to prove that a
// destination still belongs to the transaction that published it. The
// fingerprint is intentionally private so callers cannot forge records.
type SkillPathPublication struct {
	Destination string
	fingerprint [sha256.Size]byte
	identity    os.FileInfo
	incarnation string
	fileID      string
}

// PublishSkillPathNoReplace atomically publishes a staged path without ever
// replacing a destination created after the backup phase.
//
// The rename returns a mkdir-claim identity when (and only when) the
// child-move fallback published dest. That identity is the tamperproof
// witness that dest is still the mkdir claim this transaction created
// (unlike the staged shell, whose presence alone is compatible with a
// wholesale replacement of dest by a concurrent writer). When the claim
// identity no longer matches dest, publication reports uncertain state
// (ErrSkillPathPublicationUncertain) and keeps dest — auto-retracting or
// overwriting would delete the concurrent writer's data.
func PublishSkillPathNoReplace(staged, destination string) (SkillPathPublication, error) {
	identity, err := skillPathLstat(staged)
	if err != nil {
		return SkillPathPublication{}, fmt.Errorf("读取待发布 Skill 身份失败 %s: %w", staged, err)
	}
	fingerprint, err := fingerprintSkillPath(staged)
	if err != nil {
		return SkillPathPublication{}, fmt.Errorf("计算待发布 Skill 身份失败 %s: %w", staged, err)
	}
	stagedFileID := skillPathFileIdentity(staged)
	claimIdentity, err := skillPathRenameNoReplace(staged, destination)
	if err != nil {
		return SkillPathPublication{}, fmt.Errorf("目标必须不存在的 Skill 发布失败 %s: %w", destination, err)
	}
	// If the child-move fallback published dest, the claim identity must
	// still describe the object at destination. A wholesale replacement of
	// dest between the mkdir claim and this check produces a different
	// identity — dest is a foreign object and must not be treated as this
	// transaction's publication. An unreadable live identity (dest already
	// vanished, or the platform cannot report one) defers to the standard
	// confirmation checks below instead of guessing.
	if claimIdentity != "" {
		if liveIdentity := skillPathFileIdentity(destination); liveIdentity != "" && liveIdentity != claimIdentity {
			return SkillPathPublication{}, fmt.Errorf("%w：child-move 发布后目标 %s 已被替换；目标保留", ErrSkillPathPublicationUncertain, destination)
		}
	}
	publishedIdentity, err := skillPathLstat(destination)
	if err != nil {
		return SkillPathPublication{}, fmt.Errorf("确认已发布 Skill 身份失败 %s（对象保留）: %w", destination, err)
	}
	publishedFingerprint, err := fingerprintSkillPath(destination)
	if err != nil {
		return SkillPathPublication{}, fmt.Errorf("确认已发布 Skill 内容失败 %s（对象保留）: %w", destination, err)
	}
	publishedFileID := skillPathFileIdentity(destination)
	// When claim identity is available (child-move path), it is the
	// authoritative witness of "dest is our object"; the staged inode
	// necessarily differs from the mkdir-claim inode. Only enforce the
	// staged-inode identity proof when there is no claim identity — that
	// is, when the atomic primitive or the fast-path rename consumed the
	// staged inode into dest.
	if claimIdentity == "" && !skillPathIdentityProven(identity, publishedIdentity, stagedFileID, publishedFileID) {
		return SkillPathPublication{}, fmt.Errorf("确认已发布 Skill 身份失败 %s（对象保留）: staging 身份已变化", destination)
	}
	if publishedFingerprint != fingerprint {
		return SkillPathPublication{}, fmt.Errorf("确认已发布 Skill 内容失败 %s（对象保留）: staging 内容已变化", destination)
	}
	return SkillPathPublication{
		Destination: destination,
		fingerprint: fingerprint,
		identity:    publishedIdentity,
		incarnation: skillPathFileIncarnation(publishedIdentity),
		fileID:      publishedFileID,
	}, nil
}

// RollbackSkillPathPublications removes only objects that can still be proven
// to have been published by this transaction. Each live path is first claimed
// into a private sibling quarantine. A concurrent replacement is restored when
// possible, otherwise retained in quarantine and reported explicitly.
func RollbackSkillPathPublications(publications []SkillPathPublication) error {
	var rollbackErr error
	for i := len(publications) - 1; i >= 0; i-- {
		if err := rollbackSkillPathPublication(publications[i]); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	return rollbackErr
}

func rollbackSkillPathPublication(publication SkillPathPublication) (err error) {
	destination := publication.Destination
	quarantineRoot, err := skillPathMkdirTemp(filepath.Dir(destination), "."+filepath.Base(destination)+".rollback-")
	if err != nil {
		return fmt.Errorf("创建 Skill 回滚隔离目录失败 %s: %w", destination, err)
	}
	quarantine := filepath.Join(quarantineRoot, "payload")
	cleanupRoot := func() error {
		if cleanupErr := skillPathRemoveAll(quarantineRoot); cleanupErr != nil {
			return fmt.Errorf("清理 Skill 回滚隔离目录失败 %s: %w", quarantineRoot, cleanupErr)
		}
		return nil
	}

	liveIdentity, liveIdentityErr := skillPathLstat(destination)
	if os.IsNotExist(liveIdentityErr) {
		return cleanupRoot()
	}
	liveFingerprint, liveFingerprintErr := fingerprintSkillPath(destination)
	liveVerificationErr := errors.Join(liveIdentityErr, liveFingerprintErr)
	if liveVerificationErr != nil {
		return errors.Join(
			fmt.Errorf("拒绝删除无法验证的 Skill %s: %w", destination, liveVerificationErr),
			cleanupRoot(),
		)
	}
	if publication.identity == nil ||
		!skillPathIdentityProven(publication.identity, liveIdentity, publication.fileID, skillPathFileIdentity(destination)) ||
		publication.incarnation != skillPathFileIncarnation(liveIdentity) ||
		liveFingerprint != publication.fingerprint {
		return errors.Join(
			fmt.Errorf("拒绝删除非本事务 Skill %s: 发布对象身份已变化", destination),
			cleanupRoot(),
		)
	}

	if err := skillPathRename(destination, quarantine); err != nil {
		cleanupErr := cleanupRoot()
		if os.IsNotExist(err) {
			return cleanupErr
		}
		return errors.Join(fmt.Errorf("隔离待回滚 Skill 失败 %s: %w", destination, err), cleanupErr)
	}

	actualIdentity, identityErr := skillPathLstat(quarantine)
	actual, fingerprintErr := fingerprintSkillPath(quarantine)
	if identityErr == nil && fingerprintErr == nil &&
		skillPathIdentityProven(publication.identity, actualIdentity, publication.fileID, skillPathFileIdentity(quarantine)) &&
		actual == publication.fingerprint {
		if removeErr := removePublishedSkillSource(quarantine); removeErr != nil {
			return fmt.Errorf("移除事务发布的 Skill 失败 %s（隔离于 %s）: %w", destination, quarantine, removeErr)
		}
		return cleanupRoot()
	}

	verificationErr := errors.Join(identityErr, fingerprintErr)
	if verificationErr == nil {
		verificationErr = errors.New("发布对象身份已变化")
	}
	// Rollback restore is not a publication path; ignore any claim
	// identity returned by the rename — no ownership check follows.
	if _, restoreErr := skillPathRenameNoReplace(quarantine, destination); restoreErr != nil {
		return fmt.Errorf("拒绝删除非本事务 Skill %s: %v；并发对象保留于 %s，恢复原路径失败: %w", destination, verificationErr, quarantine, restoreErr)
	}
	return errors.Join(
		fmt.Errorf("拒绝删除非本事务 Skill %s: %w", destination, verificationErr),
		cleanupRoot(),
	)
}

func fingerprintSkillPath(path string) ([sha256.Size]byte, error) {
	h := sha256.New()
	if err := fingerprintSkillPathInto(h, path); err != nil {
		return [sha256.Size]byte{}, err
	}
	var result [sha256.Size]byte
	copy(result[:], h.Sum(nil))
	return result, nil
}

func fingerprintSkillPathInto(h hash.Hash, path string) error {
	info, err := skillPathLstat(path)
	if err != nil {
		return err
	}
	writeFingerprintField(h, info.Mode().Type().String())
	writeFingerprintField(h, info.Mode().Perm().String())
	switch {
	case info.IsDir():
		entries, err := skillPathReadDir(path)
		if err != nil {
			return err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		writeFingerprintField(h, fmt.Sprintf("%d", len(entries)))
		for _, entry := range entries {
			writeFingerprintField(h, entry.Name())
			if err := fingerprintSkillPathInto(h, filepath.Join(path, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	case info.Mode()&os.ModeSymlink != 0:
		target, err := skillPathReadlink(path)
		if err != nil {
			return err
		}
		writeFingerprintField(h, target)
		return nil
	case info.Mode().IsRegular():
		digest, err := skillPathFileDigest(path)
		if err != nil {
			return err
		}
		writeFingerprintField(h, fmt.Sprintf("%d", info.Size()))
		_, err = h.Write(digest[:])
		return err
	default:
		return fmt.Errorf("不支持识别特殊 Skill 路径 %s（mode=%s）", path, info.Mode())
	}
}

func writeFingerprintField(h hash.Hash, value string) {
	_, _ = fmt.Fprintf(h, "%d:", len(value))
	_, _ = h.Write([]byte(value))
}
