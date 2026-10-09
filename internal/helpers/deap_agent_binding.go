// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/spf13/cobra"
)

// Filesystem boundaries shared by employee lifecycle operations. Tests inject
// failures here instead of depending on Unix permissions or filesystem races.
var employeeReadFile = os.ReadFile
var employeeStat = os.Stat
var employeeReadDir = os.ReadDir
var employeeOpenFile = os.OpenFile
var employeeMarshalJSON = json.Marshal
var employeeMarshalIndent = json.MarshalIndent
var employeeAbsPath = filepath.Abs

// digitalEmployeeBindingPath 使用 Profile 摘要隔离多员工配置；文件内容仍保留
// 精确 Profile 用于读回校验，但不保存任何 Token 或授权码。
func digitalEmployeeBindingPath(configDir, profile string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(profile)))
	return filepath.Join(configDir, "digital-employees", fmt.Sprintf("%x.json", digest[:]))
}

func saveDigitalEmployeeBinding(configDir string, binding digitalEmployeeBinding) error {
	lock, err := auth.AcquireDualLock(context.Background(), filepath.Join(configDir, "digital-employees", digitalEmployeeScope(binding.DWSProfile), "binding-lock"))
	if err != nil {
		return err
	}
	defer lock.Release()
	if err := checkDigitalEmployeeBinding(configDir, binding.DWSProfile, binding.AgentUUID, bindingChannel(binding)); err != nil {
		return err
	}
	if binding.SchemaVersion != 1 || !validMachineString(binding.AgentUUID) || !validMachineString(binding.DWSProfile) ||
		!validMachineString(binding.OperatorOpenDingTalkID) {
		return fmt.Errorf("invalid digital employee binding")
	}
	data, err := employeeMarshalIndent(binding, "", "  ")
	if err != nil {
		return fmt.Errorf("encode digital employee binding: %w", err)
	}
	return AtomicWriteJSON(digitalEmployeeBindingPath(configDir, binding.DWSProfile), append(data, '\n'))
}

func validateEmployeeMachineBinding(cmd *cobra.Command, agentUUID string) error {
	profile := strings.TrimSpace(auth.RuntimeProfile())
	if profile == "" {
		return fmt.Errorf("channel requires an explicit digital employee --profile")
	}
	b, err := deapChannelLoadBinding(deapConnectConfigDir(), profile)
	if err != nil || b.DWSProfile != profile || b.AgentUUID != agentUUID || bindingChannel(b) != devAppStringFlag(cmd, "channel") {
		return fmt.Errorf("channel does not match digital employee binding")
	}
	return nil
}

func loadDigitalEmployeeBinding(configDir, profile string) (digitalEmployeeBinding, error) {
	var binding digitalEmployeeBinding
	data, err := employeeReadFile(digitalEmployeeBindingPath(configDir, profile))
	if err != nil {
		return binding, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&binding); err != nil {
		return digitalEmployeeBinding{}, fmt.Errorf("invalid digital employee binding")
	}
	if binding.SchemaVersion != 1 || binding.DWSProfile != strings.TrimSpace(profile) || !validMachineString(binding.AgentUUID) ||
		!validMachineString(binding.OperatorOpenDingTalkID) {
		return digitalEmployeeBinding{}, fmt.Errorf("invalid digital employee binding")
	}
	return binding, nil
}
