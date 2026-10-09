// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package errors

import stderrors "errors"

// localRecoveryError 记录生产方已经给出明确本地恢复方案，不改变 wire 分类。
type localRecoveryError struct{ cause error }

func (e *localRecoveryError) Error() string { return e.cause.Error() }
func (e *localRecoveryError) Unwrap() error { return e.cause }

// MarkLocalRecovery 仅供纠错候选或人工明确处理方案的生产方调用。
// 通用 --help 引导不算本地恢复方案；消费方不得根据错误或提示文案补写标记。
// 透明包装保留原错误文本、Cause、提示和退出码。
func MarkLocalRecovery(err error) error {
	if err == nil || HasLocalRecovery(err) {
		return err
	}
	return &localRecoveryError{cause: err}
}

// HasLocalRecovery 消费生产方声明的恢复事实，不推测错误或提示文案。
func HasLocalRecovery(err error) bool {
	var recovery *localRecoveryError
	return stderrors.As(err, &recovery)
}
