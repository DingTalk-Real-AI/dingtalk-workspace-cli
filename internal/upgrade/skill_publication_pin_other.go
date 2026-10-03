//go:build !linux

package upgrade

import "os"

// 本次修复限定 Linux inode 生命周期；其他平台的发布身份机制不变。
func pinSkillPathIdentity(string) (*os.File, error) { return nil, nil }
