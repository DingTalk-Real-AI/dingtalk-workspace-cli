// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
	"github.com/spf13/cobra"
)

var chatCryptoClient = messagecrypto.DefaultClient()

// SetChatCryptoClient injects the app-owned SafeChat/Ding crypto client.
func SetChatCryptoClient(client *messagecrypto.Client) {
	if client == nil {
		chatCryptoClient = messagecrypto.DefaultClient()
		return
	}
	chatCryptoClient = client
}

type chatCryptoRuntime struct {
	cmd *cobra.Command
}

func (r chatCryptoRuntime) CallMCPReadData(product, tool string, params map[string]any) (map[string]any, error) {
	text, err := CallMCPReadToolTextOnServerContext(r.cmd.Context(), product, tool, params)
	return parseChatCryptoMCPData(tool, text, err)
}

func (r chatCryptoRuntime) CallMCPWriteDataStrict(product, tool string, params map[string]any) (map[string]any, error) {
	if commandBoolFlag(r.cmd, "dry-run") {
		return nil, apperrors.NewValidation(fmt.Sprintf("--dry-run 下禁止执行 %s/%s", product, tool))
	}
	text, err := callMCPToolReturnTextOnServer(r.cmd.Context(), product, tool, params)
	if strings.TrimSpace(text) == "" && err == nil {
		return nil, apperrors.NewAPI("MCP write tool returned no business result; the remote effect is unknown",
			apperrors.WithOperation(product+"/"+tool),
			apperrors.WithOrigin("mcp"),
			apperrors.WithFailureStage("response_validation"),
			apperrors.WithRetryable(false),
			apperrors.WithReason("empty_tool_response"),
		)
	}
	return parseChatCryptoMCPData(tool, text, err)
}

func (r chatCryptoRuntime) DryRun() bool {
	return commandBoolFlag(r.cmd, "dry-run")
}

func newChatCryptoCommand() *cobra.Command {
	root := newGroupCommand(&cobra.Command{
		Use:   "crypto",
		Short: "消息三方解密",
		Long:  "解密消息三方密文。Ding 层通过 IM MCP server 在服务端完成，DWS 只执行本地 SafeChat 层。",
		RunE:  groupRunE,
	})
	decryptCmd := newChatCryptoDecryptCommand()
	root.AddCommand(decryptCmd)
	return root
}

func newChatCryptoDecryptCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decrypt",
		Short: "解密一条三方密文消息",
		Args:  cobra.NoArgs,
		RunE:  runChatCryptoDecrypt,
	}
	addChatCryptoTextInputFlags(cmd)
	cmd.Flags().String("layer", "full", "解密层: full|safechat|ding")
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "chat",
				Name:           "chat_crypto_decrypt",
				CanonicalPath:  "chat.chat_crypto_decrypt",
				CLIPath:        "chat crypto decrypt",
				PrimaryCLIPath: "chat crypto decrypt",
			},
			Description: "解密一条三方密文消息，默认执行 SafeChat 到服务端 Ding 解密的完整链路",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "DWS composite command: local SafeChat decrypt yields Ding ciphertext, then IM MCP server performs Ding decrypt.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "解密一条三方密文消息",
				UseWhen:      []string{"用户明确提供一条三方密文并要求解密或诊断 SafeChat/Ding 层时"},
				AvoidWhen:    []string{"批量读取消息时优先使用列表/搜索命令，DWS 会按服务端策略自动处理解密"},
				Examples:     []string{"dws chat crypto decrypt --text <ciphertext> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "file", Property: "ciphertext"},
				{Name: "layer", Property: "layer", Enum: []string{"full", "safechat", "ding"}},
				{Name: "text", Property: "ciphertext"},
			},
		},
	})
	return cmd
}

func runChatCryptoDecrypt(cmd *cobra.Command, _ []string) error {
	ciphertext, err := readChatCryptoInput(cmd)
	if err != nil {
		return err
	}
	layer, _ := cmd.Flags().GetString("layer")
	result, err := chatCryptoClient.DecryptInbound(cmd.Context(), chatCryptoRuntime{cmd: cmd}, messagecrypto.Options{
		Layer:      layer,
		Ciphertext: string(ciphertext),
	})
	if err != nil {
		return err
	}
	return writeCommandPayload(cmd, map[string]any{
		"ok":         true,
		"layer":      result.Layer,
		"plaintext":  result.Plaintext,
		"keyVersion": result.KeyVersion,
	})
}

func addChatCryptoTextInputFlags(cmd *cobra.Command) {
	cmd.Flags().String("text", "", "输入文本")
	cmd.Flags().String("file", "", "输入文件路径；- 表示 stdin")
}

func readChatCryptoInput(cmd *cobra.Command) ([]byte, error) {
	text, _ := cmd.Flags().GetString("text")
	filePath, _ := cmd.Flags().GetString("file")
	sources := 0
	if strings.TrimSpace(text) != "" {
		sources++
	}
	if strings.TrimSpace(filePath) != "" {
		sources++
	}
	if sources != 1 {
		return nil, apperrors.NewValidation("--text 与 --file 必须且只能指定一个")
	}
	var raw []byte
	var err error
	if strings.TrimSpace(filePath) == "-" {
		raw, err = io.ReadAll(cmd.InOrStdin())
	} else if strings.TrimSpace(filePath) != "" {
		safePath, safeErr := apperrors.SafeInputPath(filePath)
		if safeErr != nil {
			return nil, safeErr
		}
		raw, err = os.ReadFile(safePath)
	} else {
		raw = []byte(text)
	}
	if err != nil {
		return nil, fmt.Errorf("读取输入失败: %w", err)
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, apperrors.NewValidation("输入内容为空")
	}
	return raw, nil
}

func parseChatCryptoMCPData(tool, text string, err error) (map[string]any, error) {
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := unmarshalJSONUseNumber(text, &out); err != nil {
		return nil, fmt.Errorf("解析 %s 返回失败: %w", tool, err)
	}
	return out, nil
}

var _ messagecrypto.Runtime = chatCryptoRuntime{}
