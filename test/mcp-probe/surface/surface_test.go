package surface

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/mcp-probe/runner"
)

func TestParsePageAvailableCommands(t *testing.T) {
	t.Parallel()

	page := ParsePage([]string{"aitable"}, `管理钉钉 AI 表格：Base 管理、数据表、字段、记录、模板搜索。

命令结构:
  dws aitable base       [list|search|get|create|update|delete]     Base 管理
  dws aitable table      [get|create|update|delete]                 数据表管理
  dws aitable field      [get|create|update|delete]                 字段管理
  dws aitable record     [query|create|update|delete]               记录管理
  dws aitable attachment upload                                     附件上传准备
  dws aitable template   search                                     模板搜索

Usage:
  dws aitable [flags]
  dws aitable [command]

Available Commands:
  attachment  附件管理
  base        Base 管理
  field       字段管理
  record      记录管理
  table       数据表管理
  template    模板搜索

Flags:
  -h, --help   help for aitable

Global Flags:
      --debug           显示调试日志
`)

	want := []string{
		"attachment",
		"base",
		"field",
		"record",
		"table",
		"template",
	}

	if got := commandNames(page.Available); !reflect.DeepEqual(got, want) {
		t.Fatalf("commandNames(ParsePage().Available) = %#v, want %#v", got, want)
	}
}

func TestParsePageLocalFlagsIgnoreDescriptionsAndTypes(t *testing.T) {
	t.Parallel()

	page := ParsePage([]string{"chat", "search"}, `根据名称搜索会话列表

Usage:
  dws chat search [flags]

Examples:
  dws chat search --query "项目冲刺"

Flags:
      --cursor string   分页游标 (首页留空)
  -h, --help            help for search
      --query string    搜索关键词 (必填)

Global Flags:
      --debug           显示调试日志
`)

	want := []FlagEntry{
		{Names: []string{"--cursor"}},
		{Names: []string{"-h", "--help"}},
		{Names: []string{"--query"}},
	}

	if !reflect.DeepEqual(page.LocalFlags, want) {
		t.Fatalf("ParsePage().LocalFlags = %#v, want %#v", page.LocalFlags, want)
	}
}

func TestComparePageIgnoresSummaryAndDescriptions(t *testing.T) {
	t.Parallel()

	expected := Page{
		Path: []string{"aitable"},
		Available: []CommandEntry{
			{Name: "base", Description: "Base 管理"},
		},
		LocalFlags: []FlagEntry{
			{Names: []string{"--page"}},
			{Names: []string{"-h", "--help"}},
		},
		Summary: "真相源摘要",
	}
	actual := Page{
		Path: []string{"aitable"},
		Available: []CommandEntry{
			{Name: "base", Description: "基础管理"},
		},
		LocalFlags: []FlagEntry{
			{Names: []string{"--page"}},
			{Names: []string{"-h", "--help"}},
		},
		Summary: "候选摘要",
	}

	diff := ComparePage(expected, actual)
	if !diff.Equal {
		t.Fatalf("ComparePage() Equal = false, want true, details=%#v", diff.Details)
	}
}

func TestParsePageIgnoresWrappedFlagDescriptionLines(t *testing.T) {
	t.Parallel()

	page := ParsePage([]string{"aitable", "record", "query"}, `查询记录

Usage:
  dws aitable record query [flags]

Flags:
      --sort string   排序条件 JSON 数组，按数组顺序依次生效
                      [{"fieldId":"fldPriorityId","direction":"asc"}]
  -h, --help          help for query
`)

	want := []FlagEntry{
		{Names: []string{"--sort"}},
		{Names: []string{"-h", "--help"}},
	}
	if !reflect.DeepEqual(page.LocalFlags, want) {
		t.Fatalf("ParsePage().LocalFlags = %#v, want %#v", page.LocalFlags, want)
	}
}

func TestComparePageDetectsAvailableCommandsMismatch(t *testing.T) {
	t.Parallel()

	expected := Page{
		Path: []string{"aitable"},
		Available: []CommandEntry{
			{Name: "base", Description: "Base 管理"},
		},
	}
	actual := Page{
		Path: []string{"aitable"},
		Available: []CommandEntry{
			{Name: "table", Description: "基础管理"},
		},
	}

	diff := ComparePage(expected, actual)
	if diff.Equal {
		t.Fatalf("ComparePage() Equal = true, want false")
	}
	if len(diff.Details) == 0 || diff.Details[0] != "available commands mismatch" {
		t.Fatalf("ComparePage() Details = %#v, want available commands mismatch", diff.Details)
	}
}

func TestComparePageDetectsLocalFlagsMismatch(t *testing.T) {
	t.Parallel()

	expected := Page{
		Path: []string{"chat", "search"},
		LocalFlags: []FlagEntry{
			{Names: []string{"--cursor"}},
			{Names: []string{"-h", "--help"}},
			{Names: []string{"--query"}},
		},
	}
	actual := Page{
		Path: []string{"chat", "search"},
		LocalFlags: []FlagEntry{
			{Names: []string{"--cursor"}},
			{Names: []string{"-h", "--help"}},
			{Names: []string{"--json"}},
			{Names: []string{"--params"}},
			{Names: []string{"--query"}},
		},
	}

	diff := ComparePage(expected, actual)
	if diff.Equal {
		t.Fatalf("ComparePage() Equal = true, want false")
	}
	if len(diff.Details) == 0 || diff.Details[0] != "local flags mismatch" {
		t.Fatalf("ComparePage() Details = %#v, want local flags mismatch", diff.Details)
	}
}

func TestComparePageIgnoresLocalFlagTypes(t *testing.T) {
	t.Parallel()

	expected := Page{
		Path: []string{"todo", "task", "create"},
		LocalFlags: []FlagEntry{
			{Names: []string{"--executors"}},
			{Names: []string{"--priority"}},
		},
	}
	actual := Page{
		Path: []string{"todo", "task", "create"},
		LocalFlags: []FlagEntry{
			{Names: []string{"--executors"}},
			{Names: []string{"--priority"}},
		},
	}

	diff := ComparePage(expected, actual)
	if !diff.Equal {
		t.Fatalf("ComparePage() Equal = false, want true, details=%#v", diff.Details)
	}
}

func TestComparePagePassesForIdenticalSurface(t *testing.T) {
	t.Parallel()

	expected := Page{
		Path: []string{"chat", "search"},
		Available: []CommandEntry{
			{Name: "search", Description: "根据名称搜索会话列表"},
		},
		LocalFlags: []FlagEntry{
			{Names: []string{"--cursor"}},
			{Names: []string{"-h", "--help"}},
			{Names: []string{"--query"}},
		},
	}

	diff := ComparePage(expected, expected)
	if !diff.Equal {
		t.Fatalf("ComparePage() Equal = false, want true, details=%#v", diff.Details)
	}
}

func TestCrawlDiscoversNestedPagesFromAvailableCommands(t *testing.T) {
	t.Parallel()

	scriptPath := writeFakeHelpBinary(t)
	r := &runner.Runner{
		DWSBinary: scriptPath,
		ExtraEnv:  []string{"PATH=/usr/bin:/bin"},
		Timeout:   2 * time.Second,
	}

	pages, err := Crawl(context.Background(), r)
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	gotPaths := make([][]string, 0, len(pages))
	for _, page := range pages {
		gotPaths = append(gotPaths, page.Path)
	}

	wantPaths := [][]string{
		nil,
		{"aitable"},
		{"chat"},
		{"chat", "search"},
	}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("Crawl() paths = %#v, want %#v", gotPaths, wantPaths)
	}

	searchPage := pages[len(pages)-1]
	wantFlags := []FlagEntry{
		{Names: []string{"--cursor"}},
		{Names: []string{"-h", "--help"}},
		{Names: []string{"--query"}},
	}
	if !reflect.DeepEqual(searchPage.LocalFlags, wantFlags) {
		t.Fatalf("leaf flags = %#v, want %#v", searchPage.LocalFlags, wantFlags)
	}
}

func commandNames(entries []CommandEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name)
	}
	return out
}

func TestParsePageIgnoresIndentedJSONExamplesInFlagsSection(t *testing.T) {
	t.Parallel()

	raw := `查询指定表格中的记录

Usage:
  dws aitable record query [flags]

Flags:
      --base-id string      Base ID
      --sort string         排序条件 JSON 数组，按数组顺序依次生效
                            
                            每个元素：{"fieldId":"<fieldId>","direction":"asc|desc"}
                            
                            示例：
                            [
                              {"fieldId":"fldPriorityId","direction":"asc"},
                              {"fieldId":"fldDueDateId",  "direction":"desc"}
                            ]
  -h, --help                help for query
      --table-id string     Table ID
`

	page := ParsePage([]string{"aitable", "record", "query"}, raw)
	want := []FlagEntry{
		{Names: []string{"--base-id"}},
		{Names: []string{"--sort"}},
		{Names: []string{"-h", "--help"}},
		{Names: []string{"--table-id"}},
	}
	if !reflect.DeepEqual(page.LocalFlags, want) {
		t.Fatalf("LocalFlags = %#v, want %#v", page.LocalFlags, want)
	}
}

func writeFakeHelpBinary(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "fake-dws")
	content := `#!/bin/sh
case "$*" in
  "--help")
    cat <<'EOF'
DWS CLI

Usage:
  dws [command]

Available Commands:
  aitable     AI 表格操作
  chat        群聊 / 会话 / 群组管理

Flags:
  -h, --help   help for dws
EOF
    ;;
  "aitable --help")
    cat <<'EOF'
AI 表格操作

Usage:
  dws aitable [flags]
  dws aitable [command]

Flags:
  -h, --help   help for aitable
EOF
    ;;
  "chat --help")
    cat <<'EOF'
群聊 / 会话 / 群组管理

Usage:
  dws chat [flags]
  dws chat [command]

Available Commands:
  search      根据名称搜索会话列表

Flags:
  -h, --help   help for chat
EOF
    ;;
  "chat search --help")
    cat <<'EOF'
根据名称搜索会话列表

Usage:
  dws chat search [flags]

Flags:
      --cursor string   分页游标 (首页留空)
  -h, --help            help for search
      --query string    搜索关键词 (必填)

Global Flags:
      --debug           显示调试日志
EOF
    ;;
  *)
    echo "unexpected args: $*" >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
