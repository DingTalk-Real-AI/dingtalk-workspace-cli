// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package unit_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSheetCSVPutAutoConvertSkillContract(t *testing.T) {
	pairs := [][2]string{
		{
			"../../skills/mono/references/products/sheet.md",
			"../../skills/multi/dingtalk-misc/references/sheet.md",
		},
		{
			"../../skills/mono/references/products/sheet/sheet-write-data.md",
			"../../skills/multi/dingtalk-misc/references/sheet/sheet-write-data.md",
		},
		{
			"../../skills/mono/references/products/sheet/sheet-formula.md",
			"../../skills/multi/dingtalk-misc/references/sheet/sheet-formula.md",
		},
		{
			"../../skills/mono/references/products/sheet/sheet-batch-operations.md",
			"../../skills/multi/dingtalk-misc/references/sheet/sheet-batch-operations.md",
		},
	}

	for index, pair := range pairs {
		mono, err := os.ReadFile(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		multi, err := os.ReadFile(pair[1])
		if err != nil {
			t.Fatal(err)
		}
		// The sheet.md router has intentional mono/multi link substitutions;
		// every file in the paired Sheet reference tree is byte-identical.
		if index > 0 && !bytes.Equal(mono, multi) {
			t.Fatalf("mono/multi Sheet skill content differs: %s vs %s", pair[0], pair[1])
		}
		if bytes.Contains(mono, []byte("allText")) {
			t.Fatalf("retired public allText wording remains in %s", pair[0])
		}
	}

	writeDataPath := pairs[1][0]
	writeData, err := os.ReadFile(writeDataPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(writeData)
	for _, required := range []string{
		"保留前导零 / 不要转日期 / 按文本原样导入 / 禁止类型推断",
		"--auto-convert=false",
		"只关闭非公式字段的类型推断",
		"首字符为 `=` 的字段仍是公式",
		"`'=...` 则作为普通文本保留前置单引号",
		"省略 `--auto-convert`，使用默认 `true`",
		"不要给所有 `csv-put` 无条件添加该参数",
		"有明确列类型要求时改用 `table-put`",
		`--csv $'id,date,total\n001,2026/8/1,"=SUM(1,2)"'`,
		"`001`、`12.10`、`1E3`、`2026/8/1`、`85%`、`TRUE`",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("%s missing %q", writeDataPath, required)
		}
	}
	defaultExample := "dws sheet csv-put --node <NODE_ID> --sheet-id <SHEET_ID> --start-cell A1 \\\n    --csv 'name,score\\nAlice,95\\nBob,87'"
	if !strings.Contains(content, defaultExample) {
		t.Errorf("%s must retain a normal csv-put example that omits --auto-convert", writeDataPath)
	}

	formulaData, err := os.ReadFile(pairs[2][0])
	if err != nil {
		t.Fatal(err)
	}
	formula := string(formulaData)
	for _, required := range []string{
		"即使传 `--auto-convert=false`",
		"公式仍会执行",
		"`'=...` 是普通文本",
		"不禁用公式",
	} {
		if !strings.Contains(formula, required) {
			t.Errorf("%s missing %q", pairs[2][0], required)
		}
	}

	batchData, err := os.ReadFile(pairs[3][0])
	if err != nil {
		t.Fatal(err)
	}
	batch := string(batchData)
	for _, required := range []string{
		`"auto-convert":false`,
		"`autoConvert:false`",
		"首字符为 `=` 的字段仍按公式解析",
		"缺省或 `true` 保持现有自动类型转换行为",
	} {
		if !strings.Contains(batch, required) {
			t.Errorf("%s missing %q", pairs[3][0], required)
		}
	}

	for _, path := range []string{pairs[0][0], pairs[1][0], pairs[2][0], pairs[3][0]} {
		if filepath.Ext(path) != ".md" {
			t.Fatalf("unexpected Sheet skill source path %q", path)
		}
	}
}
