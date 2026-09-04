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

//go:build cgo && (darwin || linux || windows) && (amd64 || arm64)

package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto"
	"github.com/spf13/cobra"
)

// 探针实跑（安恒密盾2020E1演示1组织，2026-08）确认的现网值：C 库回调
// goProxy 时给出的 url 与 domain 都指向 server.safeding.com。KeyServer 必须
// 是整条 URL：SDK 在配置非空时用它整体替换 C 库的 url。
const (
	defaultSafeChatKeyServer    = msgcrypto.DefaultSafeChatKeyServer
	defaultSafeChatRedirectHost = msgcrypto.DefaultSafeChatRedirectHost
)

func newSafeChatCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "safechat",
		Short:             "安恒密盾消息加解密",
		Long:              "安恒密盾（safechat）消息加解密能力：selftest 走端到端自检，decrypt 解密密文消息。官方及默认 CGO 构建已内置后端。",
		DisableAutoGenTag: true,
	}
	corecmd.ApplyGroupPolicy(cmd, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
	cmd.AddCommand(newSafeChatSelfTestCommand())
	cmd.AddCommand(newSafeChatDecryptCommand())
	return cmd
}

// addSafeChatBackendFlags 注册两个子命令共享的后端开关。
func addSafeChatBackendFlags(cmd *cobra.Command) {
	cmd.Flags().String("key-server", defaultSafeChatKeyServer, "安恒密钥服务地址（整条 URL，替换 C 库运行时自选值）")
	cmd.Flags().String("allowed-redirect-host", defaultSafeChatRedirectHost, "C 库回调 domain 的本地 host 核对值；留空跳过校验")
	cmd.Flags().String("keystore-dir", "", "密钥缓存目录（默认使用内置路径）")
	cmd.Flags().Bool("debug", false, "输出脱敏后的后端调试日志")
}

// safeChatSession 是一次已打开的后端会话：当前登录组织的 cipher 与身份。
type safeChatSession struct {
	cipher      msgcrypto.Cipher
	corpID      string
	staffID     string
	keystoreDir string
}

// startSafeChatSession 校验开关、读取当前登录组织并打开后端。调用方负责 Close。
func startSafeChatSession(cmd *cobra.Command) (*safeChatSession, error) {
	keyServer, _ := cmd.Flags().GetString("key-server")
	redirectHost, _ := cmd.Flags().GetString("allowed-redirect-host")
	keystoreDir, _ := cmd.Flags().GetString("keystore-dir")
	debug, _ := cmd.Flags().GetBool("debug")

	if strings.TrimSpace(keyServer) == "" {
		return nil, errors.New("--key-server 是必填项：密钥服务地址必须显式锁定，不能交给 C 库运行时自选")
	}
	if !msgcrypto.Available() {
		return nil, errors.New("当前二进制未编译 safechat 后端；请使用官方产物或启用 CGO 重新构建")
	}

	ctx := cmd.Context()
	session, err := msgcrypto.OpenSession(ctx, msgcrypto.SessionOptions{
		ConfigDir:           defaultConfigDir(),
		CLIVersion:          RawVersion(),
		KeyServer:           strings.TrimSpace(keyServer),
		AllowedRedirectHost: strings.TrimSpace(redirectHost),
		KeystoreDir:         strings.TrimSpace(keystoreDir),
		Debug:               debug,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(cmd.ErrOrStderr(), "[safechat] "+format+"\n", args...)
		},
	})
	if strings.TrimSpace(redirectHost) == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "[safechat] 提示：未设置 --allowed-redirect-host，跳过 C 库 domain 的本地核对")
	}
	if err != nil {
		return nil, err
	}
	return &safeChatSession{
		cipher:      session.Cipher,
		corpID:      session.CorpID,
		staffID:     session.StaffID,
		keystoreDir: session.KeystoreDir,
	}, nil
}

func newSafeChatSelfTestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "selftest",
		Short: "端到端自检（真实取码与密钥获取）",
		Long: "对当前登录组织执行一次真实的加解密往返：\n" +
			"  1. C 库加密缺密钥时回调 goProxy\n" +
			"  2. goProxy 向 portal POST /oauth2/vendorAuthCode 取一次性 authCode\n" +
			"  3. 用 code 向 --key-server 换密钥材料并写入 keystore\n" +
			"  4. 完成加密并把密文解回原文\n" +
			"成功即代表 DWS 端到端链路可用。先用 dws auth login 切换到目标组织。",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE:              runSafeChatSelfTest,
	}
	addSafeChatBackendFlags(cmd)
	cmd.Flags().String("text", "dws-safechat-selftest", "参与加解密往返的明文")
	cmd.Flags().Bool("json", false, "以 JSON 格式输出")
	return cmd
}

type safeChatSelfTestResult struct {
	Available      bool   `json:"available"`
	BackendVersion string `json:"backendVersion"`
	CorpID         string `json:"corpId,omitempty"`
	RoundTrip      bool   `json:"roundTrip"`
	CiphertextLen  int    `json:"ciphertextLen,omitempty"`
	EncryptMs      int64  `json:"encryptMs,omitempty"`
	DecryptMs      int64  `json:"decryptMs,omitempty"`
	KeystoreDir    string `json:"keystoreDir,omitempty"`
	Error          string `json:"error,omitempty"`
}

func runSafeChatSelfTest(cmd *cobra.Command, _ []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	text, _ := cmd.Flags().GetString("text")

	result := safeChatSelfTestResult{
		Available:      msgcrypto.Available(),
		BackendVersion: msgcrypto.BackendVersion,
	}
	fail := func(err error) error {
		result.Error = err.Error()
		emitSafeChatResult(cmd, jsonOut, &result)
		return errors.New(result.Error)
	}

	session, err := startSafeChatSession(cmd)
	if err != nil {
		return fail(err)
	}
	defer session.cipher.Close()
	result.CorpID = session.corpID
	result.KeystoreDir = session.keystoreDir

	start := time.Now()
	ciphertext, err := session.cipher.EncryptMessage(cmd.Context(), session.corpID, session.staffID, []byte(text))
	result.EncryptMs = time.Since(start).Milliseconds()
	if err != nil {
		return fail(fmt.Errorf("加密失败（取码或换密钥环节出错，详见错误链）: %w", err))
	}
	result.CiphertextLen = len(ciphertext)

	start = time.Now()
	plaintext, err := session.cipher.DecryptMessage(cmd.Context(), session.corpID, session.staffID, ciphertext)
	result.DecryptMs = time.Since(start).Milliseconds()
	if err != nil {
		return fail(fmt.Errorf("解密失败: %w", err))
	}
	result.RoundTrip = bytes.Equal(plaintext, []byte(text))
	if !result.RoundTrip {
		return fail(errors.New("解密结果与原文不一致"))
	}

	emitSafeChatResult(cmd, jsonOut, &result)
	return nil
}

func emitSafeChatResult(cmd *cobra.Command, jsonOut bool, result *safeChatSelfTestResult) {
	w := cmd.OutOrStdout()
	if jsonOut {
		buf, _ := json.Marshal(result)
		fmt.Fprintln(w, string(buf))
		return
	}
	fmt.Fprintf(w, "后端:      %s\n", result.BackendVersion)
	if result.CorpID != "" {
		fmt.Fprintf(w, "组织:      %s\n", result.CorpID)
	}
	if result.KeystoreDir != "" {
		fmt.Fprintf(w, "keystore:  %s\n", result.KeystoreDir)
	}
	if result.CiphertextLen > 0 {
		fmt.Fprintf(w, "加密:      %d 字节密文（含取码+换密钥耗时 %dms）\n", result.CiphertextLen, result.EncryptMs)
		fmt.Fprintf(w, "解密:      回环一致（%dms）\n", result.DecryptMs)
	}
	if result.Error != "" {
		fmt.Fprintf(w, "错误:      %s\n", result.Error)
		return
	}
	fmt.Fprintln(w, "结果:      ✅ 端到端链路可用")
}

func newSafeChatDecryptCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decrypt [密文]",
		Short: "解密一条密文消息并输出明文",
		Long: "读取安恒密盾密文（群消息原文，含信封），走真实后端解密，明文写 stdout（可管道）。\n" +
			"输入三选一：位置参数、--text、--file（- 表示 stdin）。\n" +
			"热 keystore 直接解密不发起网络请求；冷 keystore 会触发取码与密钥获取。",
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE:              runSafeChatDecrypt,
	}
	addSafeChatBackendFlags(cmd)
	cmd.Flags().String("file", "", "密文文件路径（- 表示 stdin）")
	cmd.Flags().String("text", "", "密文内容")
	cmd.Flags().Bool("json", false, "以 JSON 格式输出")
	return cmd
}

type safeChatDecryptResult struct {
	Available      bool   `json:"available"`
	BackendVersion string `json:"backendVersion"`
	CorpID         string `json:"corpId,omitempty"`
	CiphertextLen  int    `json:"ciphertextLen,omitempty"`
	PlaintextLen   int    `json:"plaintextLen,omitempty"`
	Plaintext      string `json:"plaintext,omitempty"`
	Error          string `json:"error,omitempty"`
}

func runSafeChatDecrypt(cmd *cobra.Command, args []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	filePath, _ := cmd.Flags().GetString("file")
	textFlag, _ := cmd.Flags().GetString("text")

	result := safeChatDecryptResult{
		Available:      msgcrypto.Available(),
		BackendVersion: msgcrypto.BackendVersion,
	}
	fail := func(err error) error {
		result.Error = err.Error()
		emitSafeChatDecryptResult(cmd, jsonOut, &result)
		return errors.New(result.Error)
	}

	ciphertext, err := readSafeChatCiphertext(cmd, args, filePath, textFlag)
	if err != nil {
		return fail(err)
	}
	result.CiphertextLen = len(ciphertext)

	session, err := startSafeChatSession(cmd)
	if err != nil {
		return fail(err)
	}
	defer session.cipher.Close()
	result.CorpID = session.corpID

	plaintext, err := session.cipher.DecryptMessage(cmd.Context(), session.corpID, session.staffID, ciphertext)
	if err != nil {
		return fail(fmt.Errorf("解密失败: %w", err))
	}
	result.Plaintext = string(plaintext)
	result.PlaintextLen = len(plaintext)

	emitSafeChatDecryptResult(cmd, jsonOut, &result)
	return nil
}

// readSafeChatCiphertext 解析 decrypt 的输入：位置参数 / --text / --file（- 为
// stdin）三选一，去除首尾空白（文件与管道带来的换行）。
func readSafeChatCiphertext(cmd *cobra.Command, args []string, filePath, textFlag string) ([]byte, error) {
	sources := 0
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		sources++
	}
	if strings.TrimSpace(textFlag) != "" {
		sources++
	}
	if strings.TrimSpace(filePath) != "" {
		sources++
	}
	if sources == 0 {
		return nil, errors.New("缺少密文输入：提供位置参数、--text 或 --file（- 表示 stdin）之一")
	}
	if sources > 1 {
		return nil, errors.New("密文输入只能提供一个来源：位置参数、--text、--file 三选一")
	}

	var raw []byte
	switch {
	case strings.TrimSpace(filePath) != "":
		var err error
		if strings.TrimSpace(filePath) == "-" {
			raw, err = io.ReadAll(os.Stdin)
		} else {
			raw, err = os.ReadFile(strings.TrimSpace(filePath))
		}
		if err != nil {
			return nil, fmt.Errorf("读取密文文件失败: %w", err)
		}
	case strings.TrimSpace(textFlag) != "":
		raw = []byte(textFlag)
	default:
		raw = []byte(args[0])
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("密文内容为空")
	}
	return trimmed, nil
}

func emitSafeChatDecryptResult(cmd *cobra.Command, jsonOut bool, result *safeChatDecryptResult) {
	w := cmd.OutOrStdout()
	if jsonOut {
		buf, _ := json.Marshal(result)
		fmt.Fprintln(w, string(buf))
		return
	}
	if result.Error != "" {
		fmt.Fprintf(w, "错误:      %s\n", result.Error)
		return
	}
	fmt.Fprint(w, result.Plaintext)
	if !strings.HasSuffix(result.Plaintext, "\n") {
		fmt.Fprintln(w)
	}
}
