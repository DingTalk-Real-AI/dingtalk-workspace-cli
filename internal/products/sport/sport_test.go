package sport

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// sportAllTools 是已上架 MCP 服务的 22 个工具名及其 CLI 路径；产品命令树必须一一覆盖。
var sportAllTools = map[string]string{
	"get_user_org_list":                "sport org orgs",
	"get_org_step_participate_status":  "sport org status",
	"list_emp_step_ranking":            "sport step emp-ranking",
	"list_dept_step_ranking":           "sport step dept-ranking",
	"list_emp_ranking_in_dept":         "sport step dept-emp-ranking",
	"get_emp_step_ext_info":            "sport step emp-info",
	"get_emp_step_infos":               "sport step emp-infos",
	"get_dept_step_infos_by_dept_id":   "sport step dept-infos",
	"get_org_health_step_info":         "sport step org-info",
	"get_org_total_step_count":         "sport step org-total",
	"create_competition":               "sport competition create",
	"get_competition_config":           "sport competition config",
	"get_competition_rule_detail":      "sport competition rule-detail",
	"list_competition_instances":       "sport competition instances",
	"get_current_competition_instance": "sport competition current-instance",
	"get_next_competition_instance":    "sport competition next-instance",
	"send_competition_group_msg":       "sport competition announce",
	"get_signup_count":                 "sport stats signup",
	"count_daily_participants":         "sport stats participants-daily",
	"count_total_participants":         "sport stats participants",
	"count_reached_users":              "sport stats reached",
	"list_daily_participation_trend":   "sport stats trend",
}

// sportCollectLeafPaths 收集命令树中全部叶子命令的完整路径（空格分隔）。
func sportCollectLeafPaths(cmd *cobra.Command) map[string]bool {
	out := map[string]bool{}
	var walk func(current *cobra.Command, path string)
	walk = func(current *cobra.Command, path string) {
		name := current.Name()
		if path != "" {
			name = path + " " + name
		}
		if len(current.Commands()) == 0 {
			out[name] = true
			return
		}
		for _, sub := range current.Commands() {
			if sub.Name() == cobra.ShellCompRequestCmd || sub.Name() == "help" {
				continue
			}
			walk(sub, name)
		}
	}
	walk(cmd, "")
	return out
}

func TestSportCommandTreeCoversAllPublishedTools(t *testing.T) {
	root := NewCommand()
	paths := sportCollectLeafPaths(root)
	if len(paths) != len(sportAllTools) {
		t.Fatalf("sport 命令树叶子数 = %d，期望 %d：%v", len(paths), len(sportAllTools), paths)
	}
	for tool, cliPath := range sportAllTools {
		if !paths[cliPath] {
			t.Fatalf("工具 %s 对应的命令 %s 不在命令树中", tool, cliPath)
		}
	}
}

func TestSportCreateFlagsBindFlatToolArgsKeys(t *testing.T) {
	root := NewCommand()
	create := findSportSubcommand(t, root, "competition", "create")
	for _, flagName := range []string{"name", "sport-type", "competition-type", "start-date", "end-date", "goal-value", "target-day-value", "group-id", "need-signup", "allow-free-join", "cover-image-url"} {
		if create.Flags().Lookup(flagName) == nil {
			t.Fatalf("create 缺少 flag --%s", flagName)
		}
	}
	// 平铺键经 Call 钩子重排为嵌套 CompetitionCreateMcpRequest。
	payload := sportBuildCreatePayload(map[string]any{
		"userId":          "0123456789",
		"competitionName": "走路挑战赛",
		"competitionType": "FIXED",
		"dailyLimit":      "9000",
		"periodType":      "WEEK", // 透传键只复制白名单内的字段
	})
	request, ok := payload["CompetitionCreateMcpRequest"].(map[string]any)
	if !ok {
		t.Fatalf("payload 缺少 CompetitionCreateMcpRequest 对象")
	}
	if payload["userId"] != "0123456789" {
		t.Fatalf("userId 未透传：%v", payload["userId"])
	}
	if request["competitionName"] != "走路挑战赛" || request["dailyLimit"] != "9000" {
		t.Fatalf("嵌套对象字段缺失：%v", request)
	}
	if _, leaked := request["periodType"]; leaked {
		t.Fatalf("非白名单键 periodType 不应进入嵌套请求")
	}
	if _, leaked := payload["competitionName"]; leaked {
		t.Fatalf("平铺键 competitionName 不应残留在顶层")
	}
}

func TestSportValidateCreateArgsBlocksRecurringAndBadDates(t *testing.T) {
	cases := []struct {
		name      string
		overrides map[string]string
		wantErr   string
	}{
		{"阻断RECURRING类型", map[string]string{"competition-type": "RECURRING"}, "FIXED"},
		{"阻断periodType", map[string]string{"period-type": "WEEK"}, "阻断"},
		{"阻断cycleCount", map[string]string{"cycle-count": "2"}, "阻断"},
		{"日期非8位", map[string]string{"start-date": "2026-11-11"}, "yyyyMMdd"},
		{"日期不真实", map[string]string{"start-date": "20260230"}, "真实日期"},
		{"结束早于开始", map[string]string{"end-date": "20261110"}, "不得早于"},
		{"达标天数超期", map[string]string{"target-day-value": "25"}, "单期实际天数"},
		{"目标非数字", map[string]string{"goal-value": "abc"}, "goal-value"},
		{"上限小于目标", map[string]string{"daily-limit": "7000"}, "大于等于"},
	}
	base := map[string]string{
		"user-id": "0123456789", "name": "走路挑战赛", "sport-type": "WALK",
		"competition-type": "FIXED", "start-date": "20261111", "end-date": "20261130",
		"goal-value": "8000", "target-day-value": "18", "group-id": "123456",
		"need-signup": "false", "allow-free-join": "false",
		"cover-image-url": "https://gw.alicdn.com/cover.png",
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := findSportSubcommand(t, NewCommand(), "competition", "create")
			for key, value := range base {
				if err := cmd.Flags().Set(key, value); err != nil {
					t.Fatalf("设置 --%s: %v", key, err)
				}
			}
			for key, value := range tc.overrides {
				if err := cmd.Flags().Set(key, value); err != nil {
					t.Fatalf("设置 --%s: %v", key, err)
				}
			}
			err := sportValidateCreateArgs(cmd, nil)
			if err == nil {
				t.Fatalf("期望报错 %q，实际通过", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("错误 %q 未包含 %q", err.Error(), tc.wantErr)
			}
		})
	}

	valid := findSportSubcommand(t, NewCommand(), "competition", "create")
	for key, value := range base {
		if err := valid.Flags().Set(key, value); err != nil {
			t.Fatalf("设置 --%s: %v", key, err)
		}
	}
	if err := valid.Flags().Set("daily-limit", "9000"); err != nil {
		t.Fatalf("设置 --daily-limit: %v", err)
	}
	if err := sportValidateCreateArgs(valid, nil); err != nil {
		t.Fatalf("合法参数不应报错: %v", err)
	}
}

func TestSportValidateDateHelpers(t *testing.T) {
	if err := sportValidateDate("d", "20261109"); err != nil {
		t.Fatalf("合法日期被拒: %v", err)
	}
	// 闰年 20260229 合法，20260229 平年（如 2027）非法。
	if err := sportValidateDate("d", "20240229"); err != nil {
		t.Fatalf("闰日被拒: %v", err)
	}
	if err := sportValidateDate("d", "20270229"); err == nil {
		t.Fatalf("平年闰日应被拒")
	}
	if sportCompareNumbers("8000", "8000.000000") != 0 || sportCompareNumbers("9000", "8000") != 1 {
		t.Fatalf("定点比较结果不符合预期")
	}
	if _, ok := sportParsePositiveInt("18"); !ok {
		t.Fatalf("正整数解析失败")
	}
	if _, ok := sportParsePositiveInt("1.5"); ok {
		t.Fatalf("小数不应通过整数解析")
	}
}

func findSportSubcommand(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()
	current := root
	for _, name := range path {
		found := false
		for _, sub := range current.Commands() {
			if sub.Name() == name {
				current, found = sub, true
				break
			}
		}
		if !found {
			t.Fatalf("找不到子命令 %q（路径 %v）", name, path)
		}
	}
	return current
}
