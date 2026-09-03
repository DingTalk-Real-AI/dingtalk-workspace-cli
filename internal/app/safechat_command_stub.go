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

//go:build !safechat || !cgo

package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

const (
	defaultSafeChatKeyServer    = "https://server.safeding.com/DDSecureInter/getCorpSecureKey"
	defaultSafeChatRedirectHost = "server.safeding.com"
)

// newSafeChatCommand returns nil when safechat build tag is not enabled.
// The command is excluded from the CLI to keep the default binary small
// and avoid CGO dependency.
func newSafeChatCommand() *cobra.Command {
	return nil
}

func newSafeChatSelfTestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "selftest",
		Short:             "端到端自检（真实取码与密钥获取）",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE:              runUnavailableSafeChatSelfTest,
	}
	addSafeChatBackendFlags(cmd)
	cmd.Flags().String("text", "dws-safechat-selftest", "参与加解密往返的明文")
	cmd.Flags().Bool("json", false, "以 JSON 格式输出")
	return cmd
}

func newSafeChatDecryptCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "decrypt [密文]",
		Short:             "解密一条密文消息并输出明文",
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE:              runUnavailableSafeChatDecrypt,
	}
	addSafeChatBackendFlags(cmd)
	cmd.Flags().String("file", "", "密文文件路径（- 表示 stdin）")
	cmd.Flags().String("text", "", "密文内容")
	cmd.Flags().Bool("json", false, "以 JSON 格式输出")
	return cmd
}

func addSafeChatBackendFlags(cmd *cobra.Command) {
	cmd.Flags().String("key-server", defaultSafeChatKeyServer, "安恒密钥服务地址（整条 URL，替换 C 库运行时自选值）")
	cmd.Flags().String("allowed-redirect-host", defaultSafeChatRedirectHost, "C 库回调 domain 的本地 host 核对值；留空跳过校验")
	cmd.Flags().String("keystore-dir", "", "密钥缓存目录（默认使用内置路径）")
	cmd.Flags().Bool("debug", false, "输出脱敏后的后端调试日志")
}

func runUnavailableSafeChatSelfTest(cmd *cobra.Command, _ []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	keyServer, _ := cmd.Flags().GetString("key-server")
	if strings.TrimSpace(keyServer) == "" {
		return emitUnavailableSafeChatError(cmd, jsonOut, "--key-server 是必填项：密钥服务地址必须显式锁定，不能交给 C 库运行时自选")
	}
	return emitUnavailableSafeChatError(cmd, jsonOut, "当前二进制未编译 safechat 后端，需要带 safechat 标签的 CGO 构建")
}

func runUnavailableSafeChatDecrypt(cmd *cobra.Command, args []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	filePath, _ := cmd.Flags().GetString("file")
	textFlag, _ := cmd.Flags().GetString("text")
	if err := validateSafeChatDecryptInput(args, filePath, textFlag); err != nil {
		return emitUnavailableSafeChatError(cmd, jsonOut, err.Error())
	}
	return emitUnavailableSafeChatError(cmd, jsonOut, "当前二进制未编译 safechat 后端，需要带 safechat 标签的 CGO 构建")
}

func validateSafeChatDecryptInput(args []string, filePath, textFlag string) error {
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
		return errors.New("缺少密文输入：提供位置参数、--text 或 --file（- 表示 stdin）之一")
	}
	if sources > 1 {
		return errors.New("密文输入只能提供一个来源：位置参数、--text、--file 三选一")
	}
	return nil
}

func emitUnavailableSafeChatError(cmd *cobra.Command, jsonOut bool, message string) error {
	if jsonOut {
		payload := struct {
			Available bool   `json:"available"`
			Error     string `json:"error"`
		}{
			Available: false,
			Error:     message,
		}
		buf, _ := json.Marshal(payload)
		fmt.Fprintln(cmd.OutOrStdout(), string(buf))
		return errors.New(message)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "错误:      %s\n", message)
	return errors.New(message)
}
