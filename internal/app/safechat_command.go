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

package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto"
	"github.com/spf13/cobra"
)

// 探针实跑（安恒密盾2020E1演示1组织，2026-08）确认的现网值：C 库回调
// goProxy 时给出的 url 与 domain 都指向 server.safeding.com。KeyServer 必须
// 是整条 URL：SDK 在配置非空时用它整体替换 C 库的 url。
const (
	defaultSafeChatKeyServer    = "https://server.safeding.com/DDSecureInter/getCorpSecureKey"
	defaultSafeChatRedirectHost = "server.safeding.com"
)

func newSafeChatCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "safechat",
		Short:             "安恒密盾消息加解密",
		Long:              "安恒密盾（safechat）消息加解密能力。selftest 走真实链路，仅在带 safechat 构建标签的二进制中可用。",
		DisableAutoGenTag: true,
	}
	cmd.AddCommand(newSafeChatSelfTestCommand())
	return cmd
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
	cmd.Flags().String("key-server", defaultSafeChatKeyServer, "安恒密钥服务地址（整条 URL，替换 C 库运行时自选值）")
	cmd.Flags().String("allowed-redirect-host", defaultSafeChatRedirectHost, "C 库回调 domain 的本地 host 核对值；留空跳过校验")
	cmd.Flags().String("text", "dws-safechat-selftest", "参与加解密往返的明文")
	cmd.Flags().String("keystore-dir", "", "密钥缓存目录（默认使用内置路径）")
	cmd.Flags().Bool("debug", false, "输出脱敏后的后端调试日志")
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
	keyServer, _ := cmd.Flags().GetString("key-server")
	redirectHost, _ := cmd.Flags().GetString("allowed-redirect-host")
	text, _ := cmd.Flags().GetString("text")
	keystoreDir, _ := cmd.Flags().GetString("keystore-dir")
	debug, _ := cmd.Flags().GetBool("debug")

	result := safeChatSelfTestResult{
		Available:      msgcrypto.Available(),
		BackendVersion: msgcrypto.BackendVersion,
	}
	fail := func(err error) error {
		result.Error = err.Error()
		emitSafeChatResult(cmd, jsonOut, &result)
		return errors.New(result.Error)
	}

	if strings.TrimSpace(keyServer) == "" {
		return fail(errors.New("--key-server 是必填项：密钥服务地址必须显式锁定，不能交给 C 库运行时自选"))
	}
	if !result.Available {
		return fail(errors.New("当前二进制未编译 safechat 后端，需要带 safechat 标签的 CGO 构建（参见 Makefile 的 check-safechat/test-safechat）"))
	}

	ctx := cmd.Context()
	snap, err := auth.NewOAuthProvider(defaultConfigDir(), nil).GetTokenSnapshot(ctx)
	if err != nil {
		return fail(fmt.Errorf("读取登录态失败（先 dws auth login）: %w", err))
	}
	result.CorpID = snap.CorpID

	cfg := msgcrypto.Config{
		AuthCode:            msgcrypto.NewPortalAuthCode(defaultConfigDir(), RawVersion()),
		KeyServer:           strings.TrimSpace(keyServer),
		AllowedRedirectHost: strings.TrimSpace(redirectHost),
		Debug:               debug,
	}
	if strings.TrimSpace(keystoreDir) != "" {
		cfg.KeystoreDir = strings.TrimSpace(keystoreDir)
	}
	if debug {
		cfg.Logf = func(format string, args ...any) {
			fmt.Fprintf(cmd.ErrOrStderr(), "[safechat] "+format+"\n", args...)
		}
	}
	if cfg.AllowedRedirectHost == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "[safechat] 提示：未设置 --allowed-redirect-host，跳过 C 库 domain 的本地核对")
	}

	cipher, err := msgcrypto.Open(ctx, cfg)
	if err != nil {
		return fail(fmt.Errorf("初始化加解密后端失败: %w", err))
	}
	defer cipher.Close()
	result.KeystoreDir = cfg.KeystoreDir

	staffID := strings.TrimSpace(snap.UserID)
	if staffID == "" {
		staffID = "dws-selftest"
	}

	start := time.Now()
	ciphertext, err := cipher.EncryptMessage(ctx, snap.CorpID, staffID, []byte(text))
	result.EncryptMs = time.Since(start).Milliseconds()
	if err != nil {
		return fail(fmt.Errorf("加密失败（取码或换密钥环节出错，详见错误链）: %w", err))
	}
	result.CiphertextLen = len(ciphertext)

	start = time.Now()
	plaintext, err := cipher.DecryptMessage(ctx, snap.CorpID, staffID, ciphertext)
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
