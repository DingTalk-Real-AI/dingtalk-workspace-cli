// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import "time"

var authFileRenameBusy = platformAuthFileRenameBusy

// Windows 读者可能短暂阻止替换。只重试同一个已写完的文件，最多等待
// 100ms；不删除旧文件，持续占用或其他错误仍返回给原事务处理。
func renameAuthFile(source, destination string, rename func(string, string) error) error {
	for attempt := 0; ; attempt++ {
		err := rename(source, destination)
		if err == nil || !authFileRenameBusy(err) || attempt == 10 {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}
