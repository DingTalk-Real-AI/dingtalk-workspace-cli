// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package skills

import (
	"encoding/json"
	"errors"
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func TestAITableDatasourceUpdateDocumentsOptionalConfigPreservation(t *testing.T) {
	data, err := FS.ReadFile("multi/dingtalk-aitable/references/aitable/aitable-datasource.md")
	if err != nil {
		t.Fatal(err)
	}
	_, update, ok := strings.Cut(string(data), "### +datasource-update")
	if !ok {
		t.Fatal("missing datasource update guide")
	}
	update, _, _ = strings.Cut(update, "### +datasource-sync")
	for _, required := range []string{
		"| `--source-config` | 否 |",
		"省略时先读取当前 sourceConfig 并原样提交",
		"读取失败、配置缺失或格式异常时不执行更新",
		"显式提供配置时不进行这次读取",
		"dws aitable datasource update",
	} {
		if !strings.Contains(update, required) {
			t.Errorf("datasource update guide missing %q", required)
		}
	}
}

func TestAITableImportGoldenRoutesDoNotBypassConfirmation(t *testing.T) {
	unsafeExample := regexp.MustCompile("dws aitable \\+import-file[^`\\n]*--yes")
	if err := fs.WalkDir(FS, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := FS.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(data), "\n")
		for lineNumber := 0; lineNumber < len(lines); lineNumber++ {
			command := lines[lineNumber]
			if !strings.Contains(command, "dws aitable +import-file") {
				continue
			}
			for strings.HasSuffix(strings.TrimSpace(command), "\\") && lineNumber+1 < len(lines) {
				lineNumber++
				command += " " + strings.TrimSpace(lines[lineNumber])
			}
			if unsafeExample.MatchString(command) {
				t.Errorf("%s:%d pre-populates --yes: %s", path, lineNumber+1, command)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path     string
		required []string
	}{
		{
			path: "multi/dingtalk-aitable/SKILL.md",
			required: []string{
				"dws aitable +import-file --base-id <BASE_ID> --file <FILE_PATH>",
				"首次调用先触发 Runtime 确认门禁",
			},
		},
		{
			path: "mono/references/products/aitable/aitable-export-import.md",
			required: []string{
				"dws aitable +import-file --base-id <BASE_ID> --file data.xlsx",
				"首次调用先触发 Runtime 确认门禁",
				"再由执行方追加 `--yes`",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			data, err := FS.ReadFile(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)
			for _, required := range tt.required {
				if !strings.Contains(content, required) {
					t.Fatalf("%s missing %q", tt.path, required)
				}
			}
		})
	}
}

func TestAITableWorkflowDisableGoldenRoutesDoNotBypassConfirmation(t *testing.T) {
	if err := fs.WalkDir(FS, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := FS.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(data), "\n")
		for lineNumber := 0; lineNumber < len(lines); lineNumber++ {
			command := lines[lineNumber]
			if !strings.Contains(command, "workflow disable") {
				continue
			}
			for strings.HasSuffix(strings.TrimSpace(command), "\\") && lineNumber+1 < len(lines) {
				lineNumber++
				command += " " + strings.TrimSpace(lines[lineNumber])
			}
			if strings.Contains(command, "--yes") {
				t.Errorf("%s:%d pre-populates --yes for workflow disable: %s", path, lineNumber+1, command)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAITableCommentDeleteGoldenRoutesDoNotBypassConfirmation(t *testing.T) {
	for _, path := range []string{
		"multi/dingtalk-aitable/references/aitable.md",
		"multi/dingtalk-aitable/references/aitable/aitable-comment.md",
		"mono/references/products/aitable.md",
		"mono/references/products/aitable/aitable-comment.md",
	} {
		t.Run(path, func(t *testing.T) {
			data, err := FS.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for lineNumber, line := range strings.Split(string(data), "\n") {
				if !strings.Contains(line, "comment delete") {
					continue
				}
				found = true
				if strings.Contains(line, "--comment-key --yes") {
					t.Fatalf("%s:%d pre-populates --yes in required parameters: %s", path, lineNumber+1, line)
				}
				if !strings.Contains(line, "确认后再追加 `--yes`") {
					t.Fatalf("%s:%d must explain the post-confirmation --yes step: %s", path, lineNumber+1, line)
				}
			}
			if !found {
				t.Fatalf("%s missing comment delete guidance", path)
			}
		})
	}
}

func TestAITableCommentPaginationGuidanceUsesUnifiedEnvelope(t *testing.T) {
	tests := []struct {
		path     string
		required []string
	}{
		{
			path: "multi/dingtalk-aitable/SKILL.md",
			required: []string{
				"空评论页仍读取 `meta.pagination`",
				"仅 `meta.pagination.endpoint_exhausted=true` 时停止",
				"将 `meta.pagination.next_token` 原样传给下一次 `--cursor`",
			},
		},
		{
			path: "multi/dingtalk-aitable/references/aitable/aitable-comment.md",
			required: []string{
				"`data.comments=[]` 不代表结束",
				"只有 `meta.pagination.endpoint_exhausted=true` 才能停止",
				"将 `meta.pagination.next_token` 原样作为下一次 `--cursor`",
				"不要从业务 `data` 读取已移除的 `hasMore` 或 `nextToken`",
			},
		},
		{
			path: "mono/references/products/aitable/aitable-comment.md",
			required: []string{
				"`data.comments=[]` 不代表结束",
				"只有 `meta.pagination.endpoint_exhausted=true` 才能停止",
				"将 `meta.pagination.next_token` 原样作为下一次 `--cursor`",
				"不要从业务 `data` 读取已移除的 `hasMore` 或 `nextToken`",
			},
		},
		{
			path: "multi/dingtalk-aitable/references/aitable.md",
			required: []string{
				"空 `data.comments` 不代表结束",
				"仅 `meta.pagination.endpoint_exhausted=true` 时停止",
				"按 `meta.pagination.next_token` 续页",
			},
		},
		{
			path: "mono/references/products/aitable.md",
			required: []string{
				"空 `data.comments` 不代表结束",
				"仅 `meta.pagination.endpoint_exhausted=true` 时停止",
				"按 `meta.pagination.next_token` 续页",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			data, err := FS.ReadFile(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)
			for _, required := range tt.required {
				if !strings.Contains(content, required) {
					t.Errorf("%s missing %q", tt.path, required)
				}
			}
			if strings.Contains(content, "按 hasMore/nextToken 续页") ||
				strings.Contains(content, "只有 `hasMore=false` 才能停止") {
				t.Errorf("%s retains legacy comment pagination guidance", tt.path)
			}
		})
	}
}

// Embedded guidance tests guard usage contracts; Agent evals verify behavior.
func TestAITableDashboardWorkflowIsSynchronizedAndEmbedded(t *testing.T) {
	var workflows []string
	for _, path := range []string{
		"multi/dingtalk-aitable/references/aitable/aitable-dashboard-chart.md",
		"mono/references/products/aitable/aitable-dashboard-chart.md",
	} {
		data, err := FS.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		_, workflow, ok := strings.Cut(string(data), "## 分析仪表盘执行闭环\n")
		if !ok {
			t.Fatalf("%s missing dashboard workflow", path)
		}
		workflow, _, ok = strings.Cut(workflow, "\n## 要点")
		if !ok {
			t.Fatalf("%s missing workflow boundary", path)
		}
		workflows = append(workflows, workflow)
		for _, rule := range []struct {
			name  string
			terms []string
		}{
			{"configuration", []string{"复制完整 `data.templates.TYPE`", "chart get", "完整副本", "false/none/null/自定义值", "不重建最小 config"}},
			{"optional documentation", []string{"data.documentation.common", "data.documentation.byType.TYPE", "可选说明", "不因缺说明停止", "停止受影响步骤", "不伪造说明"}},
			{"statistics", []string{"同范围的服务端统计", "不下载全表", "record-count", "unique-count", "statsType=AVG"}},
			{"layout", []string{"JSON number `2`", "string `\"2\"`", "独立坐标系", "默认宽度不是固定占比", "前端默认纵向压缩", "不默认 arrange"}},
			{"acceptance", []string{"geometryVerified", "sizeCheckedChartIds", "未列出或未提供覆盖信息", "固定视口", "页面未验收", "保留显式自定义配色"}},
			{"recovery", []string{"更新原 chartId", "默认最多两轮", "明确写前拒绝", "只读对账", "禁止切换批建/串行路径重放"}},
		} {
			for _, term := range rule.terms {
				if !strings.Contains(workflow, term) {
					t.Errorf("%s missing %s usage rule %q", path, rule.name, term)
				}
			}
		}
		commands := regexp.MustCompile(`(?m)^dws aitable \+chart-templates\b`).FindAllString(workflow, -1)
		if len(commands) != 1 {
			t.Errorf("%s should give one targeted template example, got %d", path, len(commands))
		}
		for _, internal := range []string{"本地白名单", "活动公式语义校验", "VOC-six", "data={SUMMARY:config", "get_dashboard_widget_templates"} {
			if strings.Contains(workflow, internal) {
				t.Errorf("%s includes implementation or fixture detail %q", path, internal)
			}
		}
		metricExample := regexp.MustCompile("(?s)```json\\n(.*?)\\n```").FindStringSubmatch(workflow)
		if len(metricExample) != 2 {
			t.Fatalf("%s missing a JSON field-average example", path)
		}
		var metric struct {
			MeasureType string `json:"measureType"`
			Measure     []struct {
				Value         string `json:"value"`
				ExternalValue []struct {
					Type  string `json:"type"`
					Value string `json:"value"`
				} `json:"externalValue"`
			} `json:"measure"`
		}
		if err := json.Unmarshal([]byte(metricExample[1]), &metric); err != nil {
			t.Fatal(err)
		}
		if metric.MeasureType != "field" || len(metric.Measure) != 1 || metric.Measure[0].Value == "" ||
			len(metric.Measure[0].ExternalValue) != 1 || metric.Measure[0].ExternalValue[0].Type != "formula" || metric.Measure[0].ExternalValue[0].Value != "average" {
			t.Fatalf("%s field-average example changes metric semantics: %#v", path, metric)
		}
		for _, row := range []string{
			"| SUMMARY | 4×4 / 2×4 | 16×12 / 8×8 |",
			"| HISTOGRAM、PIE、BAR、LINE | 4×5 / 3×4 | 16×15 / 12×12 |",
		} {
			if !strings.Contains(workflow, row) {
				t.Errorf("%s changes confirmed component sizes: missing %s", path, row)
			}
		}
	}
	if workflows[0] != workflows[1] {
		t.Fatal("mono/multi dashboard usage rules have drifted")
	}
	for _, path := range []string{
		"multi/dingtalk-aitable/scripts/extract_chart_template.py",
		"mono/scripts/extract_chart_template.py",
	} {
		if _, err := fs.Stat(FS, path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("superseded dashboard extractor must not be embedded: %s (err=%v)", path, err)
		}
	}
}

func TestCrossPlatformCoverageAITableDashboardOverviewRoutesToOnDemandGuide(t *testing.T) {
	var navigations []string
	for _, overviewPath := range []string{
		"mono/references/products/aitable.md",
		"multi/dingtalk-aitable/references/aitable.md",
	} {
		data, err := FS.ReadFile(overviewPath)
		if err != nil {
			t.Fatal(err)
		}
		index, complex, ok := strings.Cut(string(data), "## 复杂操作\n")
		if !ok {
			t.Fatalf("%s missing complex operation boundary", overviewPath)
		}
		for _, command := range []string{"dashboard config-example", "chart widgets-example"} {
			if !strings.Contains(index, "| `"+command+"` |") {
				t.Errorf("%s must retain %s in the low-frequency index", overviewPath, command)
			}
		}
		_, section, ok := strings.Cut(complex, "### 仪表盘 / 图表（按需配置）\n")
		if !ok {
			t.Fatalf("%s missing on-demand dashboard navigation", overviewPath)
		}
		section, _, ok = strings.Cut(section, "\n### ")
		if !ok {
			t.Fatalf("%s missing dashboard section boundary", overviewPath)
		}
		navigation, _, ok := strings.Cut(section, "\n要点：")
		if !ok {
			t.Fatalf("%s missing dashboard navigation boundary", overviewPath)
		}
		navigations = append(navigations, navigation)
		for _, required := range []string{
			"按需读取 [aitable-dashboard-chart.md](./aitable/aitable-dashboard-chart.md)",
			"普通 Dashboard CRUD 直接使用命令，不进入复杂配置流程",
			"已有合法 config 且指标/布局语义明确时不重复读模板",
			"需要构造配置时仅获取所需类型模板",
			"Dashboard 完整 config 结构缺失时才读取对应模板",
			"具体读取路径以专项指南为准，不预读两份全量模板",
		} {
			if !strings.Contains(navigation, required) {
				t.Errorf("%s missing on-demand rule %q", overviewPath, required)
			}
		}
		for _, forbidden := range []string{"```", "先看配置模板", "dws aitable dashboard config-example", "dws aitable chart widgets-example"} {
			if strings.Contains(section, forbidden) {
				t.Errorf("%s retains superseded template sequence %q", overviewPath, forbidden)
			}
		}
		guidePath := strings.TrimSuffix(overviewPath, ".md") + "/aitable-dashboard-chart.md"
		guide, err := FS.ReadFile(guidePath)
		if err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{
			"已有符合当前 leaf Schema 的合法 config 且指标/布局语义明确时直接创建/更新",
			"Dashboard 完整 config 缺少结构时才调用一次 `dashboard config-example`",
			"仅在完整 config 结构缺失时调用一次",
			"实际只请求任务所需类型",
		} {
			if !strings.Contains(string(guide), required) {
				t.Errorf("%s conflicts with overview navigation: missing %q", guidePath, required)
			}
		}
		if strings.Contains(string(guide), "创建前先调此命令") {
			t.Errorf("%s retains unconditional template guidance", guidePath)
		}
	}
	if navigations[0] != navigations[1] {
		t.Fatal("mono/multi dashboard overview navigation has drifted")
	}
}

func TestAITableDashboardNavigationKeepsOrdinaryCRUDDirect(t *testing.T) {
	for _, tt := range []struct {
		path      string
		required  []string
		forbidden []string
	}{
		{
			path:      "mono/SKILL.md",
			required:  []string{"[aitable.md](./references/products/aitable.md)"},
			forbidden: []string{"dashboard-chart", "分析仪表盘执行闭环"},
		},
		{
			path: "mono/references/products/aitable.md",
			required: []string{
				"### dashboard & chart → 详见 [aitable-dashboard-chart.md](./aitable/aitable-dashboard-chart.md)",
				"新建/重构分析仪表盘", "异常复验", "仅统计步骤可补读 record-stats 或 psql",
				"普通 Dashboard CRUD 直接使用下表命令", "普通 Dashboard CRUD 直接使用命令",
			},
			forbidden: []string{"## 分析仪表盘执行闭环"},
		},
		{
			path: "multi/dingtalk-aitable/SKILL.md",
			required: []string{
				"数据分析，以及 PostgreSQL/SQL/SELECT/JOIN 或跨表关联查询，先读 [aitable-psql.md]",
				"保留显式 layout，创建后不默认 arrange", "普通 Dashboard CRUD 仍直达",
			},
			forbidden: []string{"分析仪表盘构建/重构和异常复验先读", "按统计→模板→写入→配置/数值/页面验收闭环执行"},
		},
	} {
		t.Run(tt.path, func(t *testing.T) {
			data, err := FS.ReadFile(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)
			for _, required := range tt.required {
				if !strings.Contains(content, required) {
					t.Errorf("missing scoped dashboard navigation %q", required)
				}
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(content, forbidden) {
					t.Errorf("retains top-level dashboard promotion %q", forbidden)
				}
			}
			if tt.path == "multi/dingtalk-aitable/SKILL.md" {
				before, onDemand, ok := strings.Cut(content, "## 按需加载（复杂 JSON 与恢复语义）\n")
				if !ok {
					t.Fatal("missing on-demand navigation")
				}
				for _, scoped := range []string{"新建/重构分析仪表盘", "异常复验", "仅统计步骤可补读 record-stats 或 psql", "此限定例外优先于通用直达、数据分析及单 Reference 规则"} {
					if strings.Contains(before, scoped) || !strings.Contains(onDemand, scoped) {
						t.Errorf("dashboard guidance must stay in on-demand navigation: %q", scoped)
					}
				}
			}
		})
	}
}
