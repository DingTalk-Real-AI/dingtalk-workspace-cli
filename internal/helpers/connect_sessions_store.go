// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

type connectSessionTempFile interface {
	Name() string
	Chmod(os.FileMode) error
	Write([]byte) (int, error)
	Close() error
}

var (
	connectSessionReadFile   = os.ReadFile
	connectSessionMkdirAll   = os.MkdirAll
	connectSessionCreateTemp = func(dir, pattern string) (connectSessionTempFile, error) { return os.CreateTemp(dir, pattern) }
	connectSessionRename     = os.Rename
	connectSessionRemove     = os.Remove
)

// connectSessionStorePath returns the on-disk location for a robot's
// conversation→session map, scoped by clientId so multiple bots on one machine
// stay isolated: <config dir>/connect/<clientId>/sessions.json. An empty
// clientId means "do not persist" (the caller keeps the map in memory only,
// preserving the pre-persistence behaviour). The clientId is sanitized with the
// same rule as the connect lock file so it is always filesystem-safe.
func connectSessionStorePath(clientID string) string {
	if clientID == "" {
		return ""
	}
	return filepath.Join(config.DefaultConfigDir(), "connect", sanitizeLockID(clientID), "sessions.json")
}

// loadConvSessionMap reads a persisted conversation→session map from path. It is
// deliberately forgiving: a missing file (first run) or a corrupt/unparseable
// file degrades to an empty map plus a warning, never an error or panic, so a
// bad file on disk can never block the connector from coming up. An empty path
// (persistence disabled) also yields an empty map.
func loadConvSessionMap(path string) map[string]string {
	if path == "" {
		return make(map[string]string)
	}
	raw, err := connectSessionReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "[connect][session][warn] 读取会话存档失败，按空会话起：%v\n", err)
		}
		return make(map[string]string)
	}
	m := make(map[string]string)
	if err := json.Unmarshal(raw, &m); err != nil {
		fmt.Fprintf(os.Stderr, "[connect][session][warn] 会话存档已损坏，按空会话起：%v\n", err)
		return make(map[string]string)
	}
	return m
}

// saveConvSessionMap atomically persists m to path with 0600 perms. It is
// best-effort: any failure (mkdir, marshal, write, rename) only logs a warning
// and returns — message handling must never block or fail because the session
// snapshot could not be written. The write is atomic (temp file in the same
// directory + rename) so a crash mid-write can never leave a half-written,
// later-unparseable file. An empty path is a no-op (persistence disabled).
func saveConvSessionMap(path string, m map[string]string) {
	if path == "" {
		return
	}
	dir := filepath.Dir(path)
	if err := connectSessionMkdirAll(dir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "[connect][session][warn] 创建会话存档目录失败，跳过本次落盘：%v\n", err)
		return
	}
	data, _ := json.Marshal(m)
	tmp, err := connectSessionCreateTemp(dir, "sessions-*.json.tmp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[connect][session][warn] 创建会话临时文件失败，跳过本次落盘：%v\n", err)
		return
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(config.FilePerm); err != nil {
		tmp.Close()
		_ = connectSessionRemove(tmpName)
		fmt.Fprintf(os.Stderr, "[connect][session][warn] 设置会话文件权限失败，跳过本次落盘：%v\n", err)
		return
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = connectSessionRemove(tmpName)
		fmt.Fprintf(os.Stderr, "[connect][session][warn] 写入会话临时文件失败，跳过本次落盘：%v\n", err)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = connectSessionRemove(tmpName)
		fmt.Fprintf(os.Stderr, "[connect][session][warn] 关闭会话临时文件失败，跳过本次落盘：%v\n", err)
		return
	}
	if err := connectSessionRename(tmpName, path); err != nil {
		_ = connectSessionRemove(tmpName)
		fmt.Fprintf(os.Stderr, "[connect][session][warn] 原子替换会话存档失败，跳过本次落盘：%v\n", err)
		return
	}
}
