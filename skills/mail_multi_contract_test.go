// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package skills

import (
	"strings"
	"testing"
)

func TestMailMultiDestructiveAndAttachmentContracts(t *testing.T) {
	data, err := FS.ReadFile("multi/dingtalk-mail/references/09-mail.md")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	skillData, err := FS.ReadFile("multi/dingtalk-mail/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	storedContent := content + "\n" + string(skillData)

	for _, required := range []string{
		"沿 `nextCursor` 遍历全部匹配页",
		"不能断言不存在",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("mail multi contract missing %q", required)
		}
	}

	for _, forbidden := range []string{
		"mail message batch-delete --email <邮箱> --ids <id1,id2,...> --yes",
		"dws mail message batch-delete ... --yes --format json",
		"dws mail thread batch-trash ... --yes --format json",
	} {
		if strings.Contains(storedContent, forbidden) {
			t.Fatalf("mail multi contract contains copyable destructive command %q", forbidden)
		}
	}
}
