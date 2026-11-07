// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package errors

import stderrors "errors"

// unknownInvocationError 只记录解析阶段确认的未知命令或参数，不参与 wire
// 分类。透明包装保留原错误的文本、Cause、提示和退出码。
type unknownInvocationError struct{ cause error }

func (e *unknownInvocationError) Error() string { return e.cause.Error() }
func (e *unknownInvocationError) Unwrap() error { return e.cause }

// MarkUnknownInvocation 只能由命令解析或命令解析结果的生产边界调用；
// 禁止根据业务错误的文案补写此标记。
func MarkUnknownInvocation(err error) error {
	if err == nil || IsUnknownInvocationError(err) {
		return err
	}
	return &unknownInvocationError{cause: err}
}

// IsUnknownInvocationError 消费解析阶段的事实，不检查错误文案。
func IsUnknownInvocationError(err error) bool {
	var unknown *unknownInvocationError
	return stderrors.As(err, &unknown)
}
