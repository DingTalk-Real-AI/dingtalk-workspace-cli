package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws attendance — 考勤
// ──────────────────────────────────────────────────────────

// jsonStringToMap 尝试将 JSON 字符串解析为 map；如果解析失败则原样透传为字符串。
func jsonStringToMap(s string) any {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err == nil {
		return m
	}
	return s
}

// printGroupModifyDeeplink 从 create_group_setting 的响应中提取 corpId 和 groupId，
// 拼接并输出一条钉钉 PC 端跳转链接，帮助用户直接验证刚创建的考勤组。
// 响应字段可能位置：root / root.result / root.result.groupSettingVO 等。
// 任一关键字段缺失时不输出链接，避免误导。
func printGroupModifyDeeplink(resp map[string]any) {
	if resp == nil {
		return
	}
	corpID := pickStringField(resp, "corpId")
	groupID := pickInt64StringField(resp, "groupId", "id")
	if corpID == "" || groupID == "" {
		return
	}
	link := fmt.Sprintf(
		"dingtalk://dingtalkclient/page/link?url=https%%3A%%2F%%2Fhrmregister.dingtalk.com%%2Fsubapp%%2Fattend%%2Findex%%3Fcode%%3Dattend%%26corpId%%3D%s%%26ddtab%%3Dtrue%%26from%%3Dattend%%23%%2FgroupModify%%3Fid%%3D%s",
		corpID, groupID,
	)
	deps.Out.PrintInfo("考勤组创建成功，点击以下链接在钉钉 PC 端查看：")
	deps.Out.PrintDim(link)
}

// pickStringField 从 map 中按 root → result → result.groupSettingVO/groupVO 顺序查找指定字段，
// 能转为字符串则返回字符串。
func pickStringField(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s := toFlatString(v); s != "" {
			return s
		}
	}
	if r, ok := m["result"].(map[string]any); ok {
		if v, ok := r[key]; ok {
			if s := toFlatString(v); s != "" {
				return s
			}
		}
		for _, nestedKey := range []string{"groupSettingVO", "groupVO"} {
			if vo, ok := r[nestedKey].(map[string]any); ok {
				if v, ok := vo[key]; ok {
					if s := toFlatString(v); s != "" {
						return s
					}
				}
			}
		}
	}
	return ""
}

// pickInt64StringField 同 pickStringField，按顺序匹配首个有值的 key，适配 number/int64 类型 ID 字段。
func pickInt64StringField(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s := pickStringField(m, key); s != "" {
			return s
		}
	}
	return ""
}

// toFlatString 将常见 JSON 标量字段转为字符串形式。
func toFlatString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	case json.Number:
		return t.String()
	}
	return ""
}

// parseUserList parses a comma-separated user ID string into a slice,
// trimming whitespace and filtering empty entries.
func parseUserList(usersStr string) []string {
	parts := strings.Split(usersStr, ",")
	userIds := make([]string, 0, len(parts))
	for _, u := range parts {
		if s := strings.TrimSpace(u); s != "" {
			userIds = append(userIds, s)
		}
	}
	return userIds
}

// parseDateToTimestamp parses a date string and returns millisecond timestamp.
// Supports formats: "YYYY-MM-DD" and "YYYY-MM-DD HH:mm:ss".
func parseDateToTimestamp(dateStr, paramName string) (int64, error) {
	dateStr = strings.TrimSpace(dateStr)

	// Try full datetime format first
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", dateStr, time.Local); err == nil {
		return t.UnixMilli(), nil
	}

	// Try date-only format (set to start/end of day based on parameter)
	if t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local); err == nil {
		// For begin time, use 00:00:00; for end time, use 23:59:59
		if strings.Contains(strings.ToLower(paramName), "end") {
			return t.Add(23*time.Hour + 59*time.Minute + 59*time.Second).UnixMilli(), nil
		}
		return t.UnixMilli(), nil
	}

	return 0, fmt.Errorf("invalid --%s format, use YYYY-MM-DD or YYYY-MM-DD HH:mm:ss (e.g. 2026-04-01)", paramName)
}

// int64FlagOrFallback reads an int64 flag by primary name; if zero/unchanged,
// falls back through aliases in order, returning the first non-zero value.
func int64FlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) int64 {
	if cmd.Flags().Changed(primary) {
		if v, _ := cmd.Flags().GetInt64(primary); v != 0 {
			return v
		}
	}
	for _, alias := range aliases {
		if cmd.Flags().Changed(alias) {
			if v, _ := cmd.Flags().GetInt64(alias); v != 0 {
				return v
			}
		}
	}
	v, _ := cmd.Flags().GetInt64(primary)
	return v
}

// intFlagOrFallback reads an int flag by primary name; if zero/unchanged,
// falls back through aliases in order, returning the first non-zero value.
func intFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) int {
	if cmd.Flags().Changed(primary) {
		if v, _ := cmd.Flags().GetInt(primary); v != 0 {
			return v
		}
	}
	for _, alias := range aliases {
		if cmd.Flags().Changed(alias) {
			if v, _ := cmd.Flags().GetInt(alias); v != 0 {
				return v
			}
		}
	}
	// Return the primary's default value
	v, _ := cmd.Flags().GetInt(primary)
	return v
}

// boolFlagOrFallback reads a bool flag by primary name; if unchanged/false,
// falls back through aliases in order, returning the first true value.
func boolFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) bool {
	if cmd.Flags().Changed(primary) {
		if v, _ := cmd.Flags().GetBool(primary); v {
			return v
		}
	}
	for _, alias := range aliases {
		if cmd.Flags().Changed(alias) {
			if v, _ := cmd.Flags().GetBool(alias); v {
				return v
			}
		}
	}
	v, _ := cmd.Flags().GetBool(primary)
	return v
}

// normalizeWorkDate converts workDate to yyyy-MM-dd HH:mm:ss format.
// Supports: string (YYYY-MM-DD or YYYY-MM-dd HH:mm:ss), number (timestamp).
func normalizeWorkDate(workDate any) (string, error) {
	switch v := workDate.(type) {
	case string:
		v = strings.TrimSpace(v)
		// Already in correct format
		if _, err := time.ParseInLocation("2006-01-02 15:04:05", v, time.Local); err == nil {
			return v, nil
		}
		// Date only format, append 00:00:00
		if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
			return t.Format("2006-01-02 15:04:05"), nil
		}
		return "", fmt.Errorf("invalid date string format: %s, use YYYY-MM-DD or YYYY-MM-dd HH:mm:ss", v)
	case float64:
		// JSON number (timestamp in milliseconds)
		ts := int64(v)
		return time.UnixMilli(ts).Format("2006-01-02 15:04:05"), nil
	case int64:
		return time.UnixMilli(v).Format("2006-01-02 15:04:05"), nil
	case int:
		return time.UnixMilli(int64(v)).Format("2006-01-02 15:04:05"), nil
	default:
		return "", fmt.Errorf("workDate must be string or number, got: %T", workDate)
	}
}

// convertClassCheckTime traverses classVO and converts any "HH:mm" string
// checkTime fields to millisecond timestamps (1970-01-01 HH:mm in UTC+8 的 Unix 毫秒时间戳).
// 例如 08:00 → 0, 09:00 → 3600000, 17:00 → 32400000
// Paths: classVO["sections"][*]["times"][*]["checkTime"]
//
//	classVO["setting"]["topRestTimeList"][*]["checkTime"]
func convertClassCheckTime(classVO map[string]any) {
	cst := time.FixedZone("CST", 8*3600)
	// Helper: convert a single checkTime value
	convertOne := func(obj map[string]any) {
		v, ok := obj["checkTime"]
		if !ok || v == nil {
			return
		}
		switch ct := v.(type) {
		case string:
			ct = strings.TrimSpace(ct)
			if t, err := time.ParseInLocation("2006-01-02 15:04", "1970-01-01 "+ct, cst); err == nil {
				obj["checkTime"] = float64(t.UnixMilli())
			}
		case float64:
			// already a number, no conversion needed
		}
	}

	// Convert sections[*].times[*].checkTime
	if sections, ok := classVO["sections"].([]any); ok {
		for _, sec := range sections {
			if secMap, ok := sec.(map[string]any); ok {
				if times, ok := secMap["times"].([]any); ok {
					for _, t := range times {
						if tMap, ok := t.(map[string]any); ok {
							convertOne(tMap)
						}
					}
				}
			}
		}
	}

	// Convert setting.topRestTimeList[*].checkTime
	if setting, ok := classVO["setting"].(map[string]any); ok {
		if restList, ok := setting["topRestTimeList"].([]any); ok {
			for _, item := range restList {
				if itemMap, ok := item.(map[string]any); ok {
					convertOne(itemMap)
				}
			}
		}
	}
}

type selfSettingSaveFlagSpec struct {
	flagName     string
	requestField string
	valueType    string
	description  string
	scenes       []string
}

const (
	selfSettingSaveBool = "bool"
	selfSettingSaveInt  = "int"
	selfSettingSaveJSON = "json"
)

var selfSettingSaveFlagSpecs = []selfSettingSaveFlagSpec{
	{flagName: "check-remind-setting", requestField: "checkRemindSetting", valueType: selfSettingSaveJSON, description: "打卡提醒 DING 渠道设置 JSON，如 {\"onDutyRemind\":{\"openRemind\":true,\"remindMinutes\":10}}", scenes: []string{"checkRemind"}},
	{flagName: "check-remind-user-on-duty", requestField: "checkRemindUserOnDuty", valueType: selfSettingSaveBool, description: "打卡提醒工作通知渠道：用户个人上班打卡提醒开关", scenes: []string{"checkRemind"}},
	{flagName: "check-remind-user-off-duty", requestField: "checkRemindUserOffDuty", valueType: selfSettingSaveBool, description: "打卡提醒工作通知渠道：用户个人下班打卡提醒开关", scenes: []string{"checkRemind"}},
	{flagName: "enable-onduty-check-remind-of-pc", requestField: "enableOndutyCheckRemindOfPc", valueType: selfSettingSaveBool, description: "PC 端弹窗渠道：上班打卡提醒开关", scenes: []string{"checkRemind"}},
	{flagName: "enable-offduty-check-remind-of-pc", requestField: "enableOffdutyCheckRemindOfPc", valueType: selfSettingSaveBool, description: "PC 端弹窗渠道：下班打卡提醒开关", scenes: []string{"checkRemind"}},

	{flagName: "onduty-check-type", requestField: "ondutyCheckType", valueType: selfSettingSaveInt, description: "上班极速打卡方式：1 提醒打卡，2 不提醒且不自动打卡，3 自动打卡", scenes: []string{"fastCheck"}},
	{flagName: "offduty-check-type", requestField: "offdutyCheckType", valueType: selfSettingSaveInt, description: "下班极速打卡方式：1 提醒打卡，2 不提醒且不自动打卡，3 自动打卡", scenes: []string{"fastCheck"}},
	{flagName: "onduty-remind-start-min", requestField: "ondutyRemindStartMin", valueType: selfSettingSaveInt, description: "上班打卡提醒开始时间，单位：分钟", scenes: []string{"fastCheck"}},
	{flagName: "onduty-remind-end-min", requestField: "ondutyRemindEndMin", valueType: selfSettingSaveInt, description: "上班打卡提醒结束时间，单位：分钟", scenes: []string{"fastCheck"}},
	{flagName: "offduty-remind-start-min", requestField: "offdutyRemindStartMin", valueType: selfSettingSaveInt, description: "下班打卡提醒开始时间，单位：分钟", scenes: []string{"fastCheck"}},
	{flagName: "offduty-remind-end-min", requestField: "offdutyRemindEndMin", valueType: selfSettingSaveInt, description: "下班打卡提醒结束时间，单位：分钟", scenes: []string{"fastCheck"}},
	{flagName: "fast-check-late-need-confirm", requestField: "fastCheckLateNeedConfirm", valueType: selfSettingSaveBool, description: "上班极速打卡：迟到时是否需要二次确认", scenes: []string{"fastCheck"}},
	{flagName: "can-update-off-duty", requestField: "canUpdateOffDuty", valueType: selfSettingSaveBool, description: "下班极速打卡：是否允许用户更新下班打卡设置", scenes: []string{"fastCheck"}},
	{flagName: "voice-remind-switch", requestField: "voiceRemindSwitch", valueType: selfSettingSaveBool, description: "极速打卡提示音开关", scenes: []string{"fastCheck"}},
	{flagName: "vibration-remind-switch", requestField: "vibrationRemindSwitch", valueType: selfSettingSaveBool, description: "极速打卡震动提醒开关", scenes: []string{"fastCheck"}},

	{flagName: "check-result-msg", requestField: "checkResultMsg", valueType: selfSettingSaveInt, description: "打卡结果通知开关：0 关闭，1 开启", scenes: []string{"checkResultNotify"}},

	{flagName: "lack-send-todo-msg", requestField: "lackSendTodoMsg", valueType: selfSettingSaveInt, description: "缺卡提醒待办渠道：0 关闭，null 或 1 开启", scenes: []string{"lackRemind"}},
	{flagName: "lack-remind-user", requestField: "lackRemindUser", valueType: selfSettingSaveInt, description: "缺卡提醒工作通知渠道：0 关闭，null 或 1 开启", scenes: []string{"lackRemind"}},

	{flagName: "person-daily-report-switch", requestField: "personDailyReportSwitch", valueType: selfSettingSaveInt, description: "个人考勤统计日报推送开关：0 关闭，1 开启", scenes: []string{"personalAttendStatNotify"}},
	{flagName: "person-week-report-type", requestField: "personWeekReportType", valueType: selfSettingSaveInt, description: "个人考勤统计周报通知渠道：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮", scenes: []string{"personalAttendStatNotify"}},
	{flagName: "person-month-report-type", requestField: "personMonthReportType", valueType: selfSettingSaveInt, description: "个人考勤统计月报通知渠道：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮", scenes: []string{"personalAttendStatNotify"}},

	{flagName: "boss-push-start-min", requestField: "bossPushStartMin", valueType: selfSettingSaveInt, description: "团队考勤统计日报推送开始时间，单位：分钟；-1 表示关闭日报推送", scenes: []string{"bossAttendStatNotify"}},
	{flagName: "boss-week-report-type", requestField: "bossWeekReportType", valueType: selfSettingSaveInt, description: "团队考勤统计周报通知渠道：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮", scenes: []string{"bossAttendStatNotify"}},
	{flagName: "boss-month-report-type", requestField: "bossMonthReportType", valueType: selfSettingSaveInt, description: "团队考勤统计月报通知渠道：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮", scenes: []string{"bossAttendStatNotify"}},
}

func registerSelfSettingSaveFlags(cmd *cobra.Command) {
	for _, spec := range selfSettingSaveFlagSpecs {
		switch spec.valueType {
		case selfSettingSaveBool:
			cmd.Flags().Bool(spec.flagName, false, spec.description)
		case selfSettingSaveInt:
			cmd.Flags().Int(spec.flagName, 0, spec.description)
		case selfSettingSaveJSON:
			cmd.Flags().String(spec.flagName, "", spec.description)
		}
	}
}

func collectSelfSettingSaveRequest(cmd *cobra.Command, settingScene string, userID string) (map[string]any, error) {
	request := map[string]any{
		"settingScene": settingScene,
		"userId":       userID,
	}
	validSceneFieldCount := 0
	invalidFlagNames := make([]string, 0)

	for _, spec := range selfSettingSaveFlagSpecs {
		if !cmd.Flags().Changed(spec.flagName) {
			continue
		}
		if !selfSettingFlagSupportsScene(spec, settingScene) {
			invalidFlagNames = append(invalidFlagNames, "--"+spec.flagName)
			continue
		}

		value, err := readSelfSettingSaveFlagValue(cmd, spec)
		if err != nil {
			return nil, err
		}
		request[spec.requestField] = value
		validSceneFieldCount++
	}

	if len(invalidFlagNames) > 0 {
		return nil, fmt.Errorf("%s cannot be used with --setting-scene %s", strings.Join(invalidFlagNames, ", "), settingScene)
	}
	if validSceneFieldCount == 0 {
		return nil, fmt.Errorf("at least one %s setting field is required", settingScene)
	}
	return request, nil
}

func selfSettingFlagSupportsScene(spec selfSettingSaveFlagSpec, settingScene string) bool {
	for _, scene := range spec.scenes {
		if scene == settingScene {
			return true
		}
	}
	return false
}

func readSelfSettingSaveFlagValue(cmd *cobra.Command, spec selfSettingSaveFlagSpec) (any, error) {
	switch spec.valueType {
	case selfSettingSaveBool:
		value, _ := cmd.Flags().GetBool(spec.flagName)
		return value, nil
	case selfSettingSaveInt:
		value, _ := cmd.Flags().GetInt(spec.flagName)
		return value, nil
	case selfSettingSaveJSON:
		rawValue := strings.TrimSpace(mustGetFlag(cmd, spec.flagName))
		if rawValue == "" {
			return nil, fmt.Errorf("--%s cannot be empty", spec.flagName)
		}
		var value map[string]any
		if err := json.Unmarshal([]byte(rawValue), &value); err != nil {
			return nil, fmt.Errorf("invalid --%s JSON: %w", spec.flagName, err)
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported value type %q for --%s", spec.valueType, spec.flagName)
	}
}

func isAttendanceSettingSceneAllowed(settingScene string) bool {
	switch settingScene {
	case "checkRemind", "fastCheck", "checkResultNotify", "lackRemind", "personalAttendStatNotify", "bossAttendStatNotify":
		return true
	default:
		return false
	}
}

func validateGlobalSettingScope(cmd *cobra.Command) error {
	if err := validateRequiredFlags(cmd, "scope"); err != nil {
		return err
	}

	scope := strings.TrimSpace(mustGetFlag(cmd, "scope"))
	switch scope {
	case "企业", "全公司", "所有人":
		return nil
	default:
		return fmt.Errorf("invalid --scope %q, must explicitly be one of: 企业, 全公司, 所有人", scope)
	}
}

type globalSettingSaveFlagSpec struct {
	flagName     string
	requestField string
	valueType    string
	description  string
	scenes       []string
}

var globalSettingSaveFlagSpecs = []globalSettingSaveFlagSpec{
	{flagName: "check-remind-corp", requestField: "checkRemindCorp", valueType: selfSettingSaveBool, description: "打卡提醒企业总开关", scenes: []string{"checkRemind"}},
	{flagName: "check-remind-pc-corp", requestField: "checkRemindPcCorp", valueType: selfSettingSaveBool, description: "打卡提醒 PC 端弹窗企业总开关", scenes: []string{"checkRemind"}},

	{flagName: "fast-check-corp", requestField: "fastCheckCorp", valueType: selfSettingSaveBool, description: "极速打卡企业总开关", scenes: []string{"fastCheck"}},

	{flagName: "enable-check-cert-push", requestField: "enableCheckCertPush", valueType: selfSettingSaveBool, description: "打卡结果通知企业总开关", scenes: []string{"checkResultNotify"}},

	{flagName: "lack-remind-corp", requestField: "lackRemindCorp", valueType: selfSettingSaveBool, description: "缺卡提醒企业总开关", scenes: []string{"lackRemind"}},

	{flagName: "enable-personal-daily-report", requestField: "enablePersonalDailyReport", valueType: selfSettingSaveBool, description: "个人考勤统计通知日报企业总开关", scenes: []string{"personalAttendStatNotify"}},
	{flagName: "enable-personal-weekly-report", requestField: "enablePersonalWeeklyReport", valueType: selfSettingSaveBool, description: "个人考勤统计通知周报企业开关，钉邮渠道", scenes: []string{"personalAttendStatNotify"}},
	{flagName: "enable-personal-weekly-report-card", requestField: "enablePersonalWeeklyReportCard", valueType: selfSettingSaveBool, description: "个人考勤统计通知周报企业开关，工作通知渠道", scenes: []string{"personalAttendStatNotify"}},
	{flagName: "enable-personal-monthly-report", requestField: "enablePersonalMonthlyReport", valueType: selfSettingSaveBool, description: "个人考勤统计通知月报企业总开关", scenes: []string{"personalAttendStatNotify"}},

	{flagName: "boss-daily-report-type", requestField: "bossDailyReportType", valueType: selfSettingSaveInt, description: "团队考勤统计通知日报发送渠道类型：0 全关闭，1 开启", scenes: []string{"bossAttendStatNotify"}},
	{flagName: "boss-weekly-report-type", requestField: "bossWeeklyReportType", valueType: selfSettingSaveInt, description: "团队考勤统计通知周报发送渠道类型：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮", scenes: []string{"bossAttendStatNotify"}},
	{flagName: "boss-monthly-report-type", requestField: "bossMonthlyReportType", valueType: selfSettingSaveInt, description: "团队考勤统计通知月报发送渠道类型：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮", scenes: []string{"bossAttendStatNotify"}},
}

func registerGlobalSettingSaveFlags(cmd *cobra.Command) {
	for _, spec := range globalSettingSaveFlagSpecs {
		switch spec.valueType {
		case selfSettingSaveBool:
			cmd.Flags().Bool(spec.flagName, false, spec.description)
		case selfSettingSaveInt:
			cmd.Flags().Int(spec.flagName, 0, spec.description)
		}
	}
}

func collectGlobalSettingSaveRequest(cmd *cobra.Command, settingScene string) (map[string]any, error) {
	request := map[string]any{
		"settingScene": settingScene,
	}
	validSceneFieldCount := 0
	invalidFlagNames := make([]string, 0)

	for _, spec := range globalSettingSaveFlagSpecs {
		if !cmd.Flags().Changed(spec.flagName) {
			continue
		}
		if !globalSettingFlagSupportsScene(spec, settingScene) {
			invalidFlagNames = append(invalidFlagNames, "--"+spec.flagName)
			continue
		}

		value, err := readGlobalSettingSaveFlagValue(cmd, spec)
		if err != nil {
			return nil, err
		}
		request[spec.requestField] = value
		validSceneFieldCount++
	}

	if len(invalidFlagNames) > 0 {
		return nil, fmt.Errorf("%s cannot be used with --setting-scene %s", strings.Join(invalidFlagNames, ", "), settingScene)
	}
	if validSceneFieldCount == 0 {
		return nil, fmt.Errorf("at least one %s global setting field is required", settingScene)
	}
	return request, nil
}

func globalSettingFlagSupportsScene(spec globalSettingSaveFlagSpec, settingScene string) bool {
	for _, scene := range spec.scenes {
		if scene == settingScene {
			return true
		}
	}
	return false
}

func readGlobalSettingSaveFlagValue(cmd *cobra.Command, spec globalSettingSaveFlagSpec) (any, error) {
	switch spec.valueType {
	case selfSettingSaveBool:
		value, _ := cmd.Flags().GetBool(spec.flagName)
		return value, nil
	case selfSettingSaveInt:
		value, _ := cmd.Flags().GetInt(spec.flagName)
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported value type %q for --%s", spec.valueType, spec.flagName)
	}
}

func newAttendanceCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "attendance",
		Short: "考勤打卡 / 排班 / 统计",
		Long: `管理钉钉考勤：查询个人考勤详情、班次查询、排班管理、获取考勤统计摘要、查询考勤组与规则。

子命令:
  record   考勤记录（个人考勤详情）
  check    打卡查询（打卡结果、打卡流水）
  approve  审批单查询（请假、加班、出差、补卡）
  shift    班次查询（员工当天打卡安排）
  schedule 排班管理（排班制考勤组排班记录导入与查询）
  class    班次规则（查询班次定义列表）
  summary      获取考勤统计摘要
  rules        查询考勤组与考勤规则
  selfsetting    个人规则设置项（get 查询，save 更新，包括打卡提醒、极速打卡、打卡结果通知、缺卡提醒、个人考勤统计通知、团队考勤统计通知）
  globalsetting  全局规则设置项（get 查询，save 更新，仅管理员可调用，包括打卡提醒、极速打卡、打卡结果通知、缺卡提醒、个人考勤统计通知、团队考勤统计通知）
  vacation       查询当前用户假期规则列表、查询员工假期余额、查询假期余额变更记录`,
		RunE: groupRunE,
	}

	// ── record ───────────────────────────────────────────────

	attendanceRecordCmd := &cobra.Command{Use: "record", Short: "考勤记录", RunE: groupRunE}

	attendanceRecordGetCmd := &cobra.Command{
		Use:     "get",
		Short:   "查询个人考勤详情",
		Example: `  dws attendance record get --user USER_ID --date 2026-03-08  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "user", "date"); err != nil {
				return err
			}
			dateStr := mustGetFlag(cmd, "date")
			t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
			if err != nil {
				return fmt.Errorf("invalid date format, use YYYY-MM-DD (e.g. 2026-03-08): %w", err)
			}
			workDate := t.UnixMilli()
			return callMCPTool("get_user_attendance_record", map[string]any{
				"userId":   mustGetFlag(cmd, "user"),
				"workDate": workDate,
			})
		},
	}

	// ── check ────────────────────────────────────────────────

	attendanceCheckCmd := &cobra.Command{Use: "check", Short: "打卡查询", RunE: groupRunE}

	// MCP tool: query_check_result
	attendanceCheckResultCmd := &cobra.Command{
		Use:   "result",
		Short: "查询打卡结果",
		Long: `查询指定用户的打卡结果记录。
返回每条记录含：用户 ID、工作日期、时间结果（Normal/Late/Early/Absenteeism/NotSigned）、
位置结果、计划打卡时间、实际打卡时间、打卡流水 ID。
时间跨度不超过 1 个月，最多 100 人。`,
		Example: `  dws attendance check result --users userId1,userId2 --start 2026-04-01 --end 2026-04-30 --limit 50`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "users"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "start", "from"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "end", "to"); err != nil {
				return err
			}
			fromStr := flagOrFallback(cmd, "start", "from")
			toStr := flagOrFallback(cmd, "end", "to")
			fromT, err := time.ParseInLocation("2006-01-02", fromStr, time.Local)
			if err != nil {
				return fmt.Errorf("invalid --start date format, use YYYY-MM-DD: %w", err)
			}
			toT, err := time.ParseInLocation("2006-01-02", toStr, time.Local)
			if err != nil {
				return fmt.Errorf("invalid --end date format, use YYYY-MM-DD: %w", err)
			}
			usersStr := mustGetFlag(cmd, "users")
			var userIds []string
			for _, u := range strings.Split(usersStr, ",") {
				if s := strings.TrimSpace(u); s != "" {
					userIds = append(userIds, s)
				}
			}
			if len(userIds) > 100 {
				return fmt.Errorf("userIds 不能超过 100 人")
			}
			offset, _ := cmd.Flags().GetInt("offset")
			if offset < 0 {
				return fmt.Errorf("offset 必须 >= 0")
			}
			limit, _ := cmd.Flags().GetInt("limit")
			if limit < 1 || limit > 1000 {
				return fmt.Errorf("limit 必须在 1-1000 之间")
			}
			return callMCPToolOnServer("attendance-wukong", "query_check_result", map[string]any{
				"QueryCheckResultRequest": map[string]any{
					"userIds":      userIds,
					"workDateFrom": fromT.Format("2006-01-02 15:04:05"),
					"workDateTo":   toT.Format("2006-01-02 15:04:05"),
					"offset":       offset,
					"limit":        limit,
				},
			})
		},
	}

	// MCP tool: query_check_record
	attendanceCheckRecordCmd := &cobra.Command{
		Use:   "record",
		Short: "查询打卡流水",
		Long: `查询指定用户的打卡流水记录。
返回每条记录含：用户 ID、实际打卡时间、打卡地址、打卡经纬度、
打卡类型（OnDuty/OffDuty）、定位方式（Map/Wifi/etc）。
时间跨度不超过 1 个月。`,
		Example: `  dws attendance check record --users userId1 --start 2026-04-01 --end 2026-04-30`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "users"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "start", "from"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "end", "to"); err != nil {
				return err
			}
			fromStr := flagOrFallback(cmd, "start", "from")
			toStr := flagOrFallback(cmd, "end", "to")
			fromT, err := time.ParseInLocation("2006-01-02", fromStr, time.Local)
			if err != nil {
				return fmt.Errorf("invalid --start date format, use YYYY-MM-DD: %w", err)
			}
			toT, err := time.ParseInLocation("2006-01-02", toStr, time.Local)
			if err != nil {
				return fmt.Errorf("invalid --end date format, use YYYY-MM-DD: %w", err)
			}
			usersStr := mustGetFlag(cmd, "users")
			var userIds []string
			for _, u := range strings.Split(usersStr, ",") {
				if s := strings.TrimSpace(u); s != "" {
					userIds = append(userIds, s)
				}
			}
			return callMCPToolOnServer("attendance-wukong", "query_check_record", map[string]any{
				"QueryCheckRecordRequest": map[string]any{
					"userIds":       userIds,
					"checkDateFrom": fromT.Format("2006-01-02 15:04:05"),
					"checkDateTo":   toT.Format("2006-01-02 15:04:05"),
				},
			})
		},
	}

	// ── approve ────────────────────────────────────────────────

	attendanceApproveCmd := &cobra.Command{Use: "approve", Short: "审批单查询", RunE: groupRunE}

	// 审批类型关键词到 bizType 数字映射
	// 注意：服务端 bizType=2 同时覆盖 出差 与 外出（合并为同一类），
	// 故 trip / travel / business_trip / 出差 / 外出 全部映射到 2。
	// 如需在提交入口区分外出（TRAVEL）与出差（OUT），请使用 attendance approve templates。
	approveTypeMapping := map[string]int{
		"overtime":      1, // 加班
		"加班":            1,
		"trip":          2, // 出差/外出（合并）
		"travel":        2,
		"business_trip": 2,
		"business-trip": 2,
		"出差":            2,
		"外出":            2,
		"leave":         3, // 请假
		"请假":            3,
		"patch":         4, // 补卡
		"repair_check":  4,
		"repair-check":  4,
		"补卡":            4,
	}

	approveTemplateTypeMapping := map[string]string{
		"repair_check":  "REPAIR_CHECK",
		"repair-check":  "REPAIR_CHECK",
		"patch":         "REPAIR_CHECK",
		"补卡":            "REPAIR_CHECK",
		"leave":         "LEAVE",
		"请假":            "LEAVE",
		"overtime":      "OVERTIME",
		"加班":            "OVERTIME",
		"travel":        "TRAVEL",
		"外出":            "TRAVEL",
		"out":           "OUT",
		"trip":          "OUT",
		"business_trip": "OUT",
		"business-trip": "OUT",
		"出差":            "OUT",
	}

	// MCP tool: query_user_approve
	attendanceApproveListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询用户审批单（补卡/加班/请假/出差外出）",
		Long: `查询指定用户的考勤业务审批单记录（补卡 / 加班 / 请假 / 出差外出）。
审批类型 --types 支持以下关键词（多个用英文逗号分隔）：
  - overtime / 加班                                            → bizType=1
  - trip / travel / business_trip / business-trip / 出差 / 外出  → bizType=2（出差与外出在查询接口合并为同一类，不再细分）
  - leave / 请假                                                → bizType=3
  - patch / repair-check / repair_check / 补卡                 → bizType=4
说明：
  - 服务端查询接口 bizType=2 同时覆盖出差与外出，传入 trip / travel / 出差 / 外出 任一别名都会返回这两类合并的记录；
  - 如需在提交入口区分外出（TRAVEL）与出差（OUT），请改用 dws attendance approve templates --type travel|out；
  - 返回每条记录含：用户 ID、审批标签、审批子类型、审批类型、生效时间、时长、时长单位、流程实例 ID。`,
		Example: `  dws attendance approve list --users userId1 --types overtime,leave --start 2026-04-01 --end 2026-04-30
  dws attendance approve list --users userId1 --types trip --start 2026-04-01 --end 2026-04-30      # 同时返回出差与外出
  dws attendance approve list --users userId1 --types 加班,请假,补卡 --start 2026-04-01 --end 2026-04-30`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "users", "types"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "start", "from"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "end", "to"); err != nil {
				return err
			}
			fromStr := flagOrFallback(cmd, "start", "from")
			toStr := flagOrFallback(cmd, "end", "to")
			fromT, err := time.ParseInLocation("2006-01-02", fromStr, time.Local)
			if err != nil {
				return fmt.Errorf("invalid --start date format, use YYYY-MM-DD: %w", err)
			}
			toT, err := time.ParseInLocation("2006-01-02", toStr, time.Local)
			if err != nil {
				return fmt.Errorf("invalid --end date format, use YYYY-MM-DD: %w", err)
			}
			usersStr := mustGetFlag(cmd, "users")
			var userIds []string
			for _, u := range strings.Split(usersStr, ",") {
				if s := strings.TrimSpace(u); s != "" {
					userIds = append(userIds, s)
				}
			}
			typesStr := mustGetFlag(cmd, "types")
			var bizTypes []int
			for _, t := range strings.Split(typesStr, ",") {
				if s := strings.TrimSpace(t); s != "" {
					bizType, ok := approveTypeMapping[strings.ToLower(s)]
					if !ok {
						return fmt.Errorf("无效的审批类型: %s，支持: overtime/加班、trip/travel/business_trip/出差/外出（查询时合并为一类）、leave/请假、patch/repair-check/补卡", s)
					}
					bizTypes = append(bizTypes, bizType)
				}
			}
			return callMCPToolOnServer("attendance-wukong", "query_user_approve", map[string]any{
				"QueryUserApproveRequest": map[string]any{
					"userIds":  userIds,
					"bizTypes": bizTypes,
					"fromDate": fromT.Format("2006-01-02 15:04:05"),
					"toDate":   toT.Format("2006-01-02 15:04:05"),
				},
			})
		},
	}

	// MCP tool: query_at_approve_template
	attendanceApproveTemplatesCmd := &cobra.Command{
		Use:   "templates",
		Short: "查询补卡/请假/加班/外出/出差审批提交链接",
		Long: `当用户需要提交补卡、请假、加班、外出、出差时，查询对应考勤审批表单模板，返回审批提交跳转链接。
审批类型支持：repair-check/patch/补卡、leave/请假、overtime/加班、travel/外出、out/trip/出差，也支持直接传 REPAIR_CHECK、LEAVE、OVERTIME、TRAVEL、OUT。
向用户展示提交入口时，不要直接裸露 submitUrl，应使用 Markdown 可点击链接格式：[表单名称](submitUrl)。`,
		Example: `  dws attendance approve templates --type leave
  dws attendance approve templates --type REPAIR_CHECK
  dws attendance approve templates --type 加班
  dws attendance approve templates --type travel    # 外出，等价 --type TRAVEL
  dws attendance approve templates --type 出差       # 出差，等价 --type OUT`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "type"); err != nil {
				return err
			}
			approveTypeInput := strings.TrimSpace(mustGetFlag(cmd, "type"))
			approveType, ok := approveTemplateTypeMapping[strings.ToLower(approveTypeInput)]
			if !ok {
				approveType = strings.ToUpper(approveTypeInput)
			}
			switch approveType {
			case "REPAIR_CHECK", "LEAVE", "OVERTIME", "TRAVEL", "OUT":
			default:
				return fmt.Errorf("无效的审批类型: %s，支持: repair-check/patch/补卡、leave/请假、overtime/加班、travel/外出、out/trip/出差，或 REPAIR_CHECK/LEAVE/OVERTIME/TRAVEL/OUT", approveTypeInput)
			}
			return callMCPToolOnServer("attendance-wukong", "query_at_approve_template", map[string]any{
				"approveType": approveType,
			})
		},
	}

	// ── shift ────────────────────────────────────────────────

	attendanceShiftCmd := &cobra.Command{
		Use:   "shift",
		Short: "班次查询",
		Long: `查询员工班次信息（班次 = 员工当天的打卡安排）。
返回每条记录含：用户 ID、工作日期、打卡类型（OnDuty/OffDuty）、计划打卡时间、是否休息日。`,
		RunE: groupRunE,
	}

	// MCP tool: batch_get_employee_shifts
	attendanceShiftListCmd := &cobra.Command{
		Use:   "list",
		Short: "批量查询员工班次信息",
		Long: `批量查询多个员工在指定日期的考勤班次信息。
返回每条记录含：用户 ID、工作日期、打卡类型（OnDuty/OffDuty）、
计划打卡时间、是否休息日。间隔不超过 7 天，最多 50 人。`,
		Example: `  dws attendance shift list --users userId1,userId2 --start 2026-03-03 --end 2026-03-07  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "users", "start", "end"); err != nil {
				return err
			}
			startStr, endStr := mustGetFlag(cmd, "start"), mustGetFlag(cmd, "end")
			startT, err := time.ParseInLocation("2006-01-02", startStr, time.Local)
			if err != nil {
				return fmt.Errorf("invalid --start date format, use YYYY-MM-DD: %w", err)
			}
			endT, err := time.ParseInLocation("2006-01-02", endStr, time.Local)
			if err != nil {
				return fmt.Errorf("invalid --end date format, use YYYY-MM-DD: %w", err)
			}
			userIds := parseUserList(mustGetFlag(cmd, "users"))
			return callMCPTool("batch_get_employee_shifts", map[string]any{
				"userIds":      userIds,
				"fromDateTime": startT.UnixMilli(),
				"toDateTime":   endT.UnixMilli(),
			})
		},
	}

	// ── class ────────────────────────────────────────────────

	attendanceClassCmd := &cobra.Command{Use: "class", Short: "班次规则", RunE: groupRunE}

	// MCP tool: get_class_list
	attendanceClassSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "查询当前用户可管理的所有班次详情",
		Long: `查询当前用户有权管理的班次列表（班次定义），支持按名称搜索和类型过滤。
所有参数均为可选项：
  --page         页码（从 1 开始，默认 1）
  --limit        每页条数（最大 200，默认 20）
  --query        班次名称关键字，模糊搜索
  --filter-type  班次类型：ALL（全部班次）/ MINE_OWN（我负责的），默认 ALL`,
		Example: `  dws attendance class search
  dws attendance class search --query "早班" --filter-type MINE_OWN
  dws attendance class search --page 1 --limit 50`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pageQuery := map[string]any{}
			if v := intFlagOrFallback(cmd, "page", "page-index"); v > 0 {
				pageQuery["pageIndex"] = v
			}
			if v := intFlagOrFallback(cmd, "limit", "page-size", "size"); v > 0 {
				pageQuery["pageSize"] = v
			}
			shiftParam := map[string]any{}
			if v := flagOrFallback(cmd, "query", "name"); v != "" {
				shiftParam["searchName"] = v
			}
			if v, _ := cmd.Flags().GetString("filter-type"); v != "" {
				shiftParam["filterType"] = v
			}
			params := map[string]any{}
			if len(pageQuery) > 0 {
				params["PageQuery"] = pageQuery
			}
			if len(shiftParam) > 0 {
				params["ShiftParamVO"] = shiftParam
			}
			return callMCPToolOnServer("attendance-wukong", "get_class_list", params)
		},
	}

	// MCP tool: get_class_detail
	attendanceClassGetCmd := &cobra.Command{
		Use:     "get",
		Short:   "根据班次 ID 查询班次详情",
		Long:    `调用 MCP 工具 get_class_detail，根据班次 ID 查询该班次的详细信息。--class-id 必填。`,
		Example: `  dws attendance class get --class-id 1170996821`,
		RunE: func(cmd *cobra.Command, args []string) error {
			classID, _ := cmd.Flags().GetInt64("class-id")
			if classID == 0 {
				return fmt.Errorf("flag --class-id is required")
			}
			return callMCPToolOnServer("attendance-wukong", "get_class_detail", map[string]any{
				"classId": classID,
			})
		},
	}

	// MCP tool: create_class_setting
	attendanceClassCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建班次",
		Long: `调用 MCP 工具 create_class_setting，创建一个新的班次。
--name 和 --sections 必填，其余参数可选。由于保存班次耗时较久，指令需要带上 --timeout 10
--yes 需要用户二次确认之后才允许带入，参考attendance.md中对于创建班次的说明

常用 flag：
  --name       班次名称（必填）
  --owner      班次负责人 userId（可选）
  --class-vo   完整 TopAtClassVO JSON 字符串（可选，用于传入复杂结构）

当使用 --class-vo 传入完整 JSON 时，--name 和 --owner 仍可覆写 JSON 中的同名字段。
checkTime 字段统一使用 "HH:mm" 格式（如 "09:00"），CLI 自动转换为服务端所需的时间戳。

--class-vo JSON 可用字段说明：

  name       string    班次名称（必填）
  owner      string    班次负责人 userId
  sections   []object  班次上下班时间段（必填），支持多段上下班，每段包含 times 数组：
    - times  []object  每段有且只能有两个对象（上班+下班），每个对象字段：
        checkType  string   打卡类型：OnDuty（上班）/ OffDuty（下班）（必填）
        checkTime  string   打卡时间，格式 "HH:mm"（必填，如 "09:00"、"17:30"）
        across     number   是否跨天：0（不跨天）/ 1（跨天）（必填）
        freeCheck  bool     是否免打卡
        beginMin   number   允许最早提前打卡时间（分钟，-1 表示不限制）
        endMin     number   允许最晚打卡时间（分钟，-1 表示不限制）
  setting    object    班次其他配置：
    - seriousLateMinutes      number  晚到多少分钟记为严重迟到
    - absenteeismLateMinutes  number  晚到多少分钟记为旷工迟到
    - attendDays              number  本班次记为多少天出勤（支持两位小数）
    - topRestTimeList         []object  班次休息时间（仅 sections 只有一段上下班时可用，最多 3 段休息时间）：
        checkType  string  休息类型：OnDuty（开始休息）/ OffDuty（结束休息）
        checkTime  string  休息时间，格式 "HH:mm"（如 "12:00"、"13:00"）
        across     number  是否跨天：0/1`,
		Example: `  dws attendance class create --name "早班" --class-vo '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"08:00","across":0},{"checkType":"OffDuty","checkTime":"17:00","across":0}]}]}' --timeout 10
  dws attendance class create --name "晚班" --owner userId1 --class-vo '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"14:00","across":0},{"checkType":"OffDuty","checkTime":"22:00","across":0}]}]}' --timeout 10
  # 带休息时段（12:00-13:00 午休）
  dws attendance class create --name "标准班" --class-vo '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"09:00","across":0},{"checkType":"OffDuty","checkTime":"18:00","across":0}]}],"setting":{"topRestTimeList":[{"checkType":"OnDuty","checkTime":"12:00","across":0},{"checkType":"OffDuty","checkTime":"13:00","across":0}]}}' --timeout 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 先解析 --class-vo JSON，作为基础
			classVO := map[string]any{}
			if raw, _ := cmd.Flags().GetString("class-vo"); raw != "" {
				if err := json.Unmarshal([]byte(raw), &classVO); err != nil {
					return fmt.Errorf("invalid --class-vo JSON: %w", err)
				}
			}

			// 单字段 flag 覆写
			if cmd.Flags().Changed("name") {
				v, _ := cmd.Flags().GetString("name")
				classVO["name"] = v
			}
			if cmd.Flags().Changed("owner") {
				v, _ := cmd.Flags().GetString("owner")
				classVO["owner"] = v
			}

			// 校验必填字段
			if classVO["name"] == nil || classVO["name"] == "" {
				return fmt.Errorf("--name 是必填项，请指定班次名称")
			}
			if classVO["sections"] == nil {
				return fmt.Errorf("--class-vo 中必须包含 sections 字段（班次上下班时间段）")
			}

			// 自动转换 checkTime："HH:mm" → 毫秒时间戳
			convertClassCheckTime(classVO)

			// 确认提示（除非 --yes）
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				fmt.Printf("About to create class: %v\n", classVO["name"])
				fmt.Print("Confirm create? (yes/no): ")
				var answer string
				if _, err := fmt.Scanln(&answer); err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "yes" && answer != "y" {
					return fmt.Errorf("create cancelled")
				}
			}

			return callMCPToolOnServer("attendance-wukong", "create_class_setting", map[string]any{
				"TopAtClassVO": classVO,
			})
		},
	}

	// MCP tool: update_class_setting
	attendanceClassUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新班次",
		Long: `调用 MCP 工具 update_class_setting，更新一个已有的班次。
--class-id 必填；其余参数均可选，仅需对要修改的字段进行赋値，未传的字段会自动从已有配置补充。由于保存班次耗时较久，指令需要带上 --timeout 10
--yes 需要用户二次确认之后才允许带入，参考attendance.md中对于更新班次的说明

常用 flag：
  --class-id   班次 ID（必填）
  --name       班次名称（可选，不传则保持原值）
  --owner      班次负责人 userId（可选，不传则保持原值）
  --class-vo   完整 TopAtClassVO JSON 字符串（可选，用于传入复杂结构）

当使用 --class-vo 传入完整 JSON 时，--name 和 --owner 仍可覆写 JSON 中的同名字段。
checkTime 字段统一使用 "HH:mm" 格式（如 "09:00"），CLI 自动转换为服务端所需的时间戳。

--class-vo JSON 可用字段说明：

  name       string    班次名称（可选，不传则保持原值）
  owner      string    班次负责人 userId（可选，不传则保持原值）
  sections   []object  班次上下班时间段（可选，不传则保持原值），支持多段上下班，每段包含 times 数组：
    - times  []object  每段有且只能有两个对象（上班+下班），每个对象字段：
        checkType  string   打卡类型：OnDuty（上班）/ OffDuty（下班）（必填）
        checkTime  string   打卡时间，格式 "HH:mm"（必填，如 "09:00"、"17:30"）
        across     number   是否跨天：0（不跨天）/ 1（跨天）（必填）
        freeCheck  bool     是否免打卡
        beginMin   number   允许最早提前打卡时间（分钟，-1 表示不限制）
        endMin     number   允许最晚打卡时间（分钟，-1 表示不限制）
  setting    object    班次其他配置：
    - seriousLateMinutes      number  晚到多少分钟记为严重迟到
    - absenteeismLateMinutes  number  晚到多少分钟记为旷工迟到
    - attendDays              number  本班次记为多少天出勤（支持两位小数）
    - topRestTimeList         []object  班次休息时间（仅 sections 只有一段上下班时可用，最多 3 段休息时间）：
        checkType  string  休息类型：OnDuty（开始休息）/ OffDuty（结束休息）
        checkTime  string  休息时间，格式 "HH:mm"（如 "12:00"、"13:00"）
        across     number  是否跨天：0/1`,
		Example: `  dws attendance class update --class-id 1170996821 --name "新早班" --timeout 10
  dws attendance class update --class-id 1170996821 --class-vo '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"08:30","across":0},{"checkType":"OffDuty","checkTime":"17:30","across":0}]}]}' --timeout 10
  # 带休息时段（12:00-13:00 午休）
  dws attendance class update --class-id 1170996821 --class-vo '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"09:00","across":0},{"checkType":"OffDuty","checkTime":"18:00","across":0}]}],"setting":{"topRestTimeList":[{"checkType":"OnDuty","checkTime":"12:00","across":0},{"checkType":"OffDuty","checkTime":"13:00","across":0}]}}' --timeout 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 校验 --class-id 必填
			classID := int64FlagOrFallback(cmd, "class-id", "id")
			if classID == 0 {
				return fmt.Errorf("flag --class-id is required")
			}

			// 先解析 --class-vo JSON，作为基础
			classVO := map[string]any{}
			if raw, _ := cmd.Flags().GetString("class-vo"); raw != "" {
				if err := json.Unmarshal([]byte(raw), &classVO); err != nil {
					return fmt.Errorf("invalid --class-vo JSON: %w", err)
				}
			}

			// 将 classId 注入到 classVO 中
			classVO["id"] = classID

			// 单字段 flag 覆写
			if cmd.Flags().Changed("name") {
				v, _ := cmd.Flags().GetString("name")
				classVO["name"] = v
			}
			if cmd.Flags().Changed("owner") {
				v, _ := cmd.Flags().GetString("owner")
				classVO["owner"] = v
			}

			// 自动转换 checkTime："HH:mm" → 毫秒时间戳
			convertClassCheckTime(classVO)

			// 确认提示（除非 --yes）
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				fmt.Printf("About to update class: %v (classId: %d)\n", classVO["name"], classID)
				fmt.Print("Confirm update? (yes/no): ")
				var answer string
				if _, err := fmt.Scanln(&answer); err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "yes" && answer != "y" {
					return fmt.Errorf("update cancelled")
				}
			}

			return callMCPToolOnServer("attendance-wukong", "update_class_setting", map[string]any{
				"TopAtClassVO": classVO,
			})
		},
	}

	// ── adjustment-rule ────────────────────────────────────

	attendanceAdjustmentCmd := &cobra.Command{Use: "adjustment", Short: "补卡规则", RunE: groupRunE}

	// MCP tool: get_adjustment_rule_detail
	attendanceAdjustmentGetCmd := &cobra.Command{
		Use:   "get",
		Short: "根据补卡规则主键 ID 查询补卡规则详情",
		Long: `调用 MCP 工具 get_adjustment_rule_detail，根据补卡规则主键 ID 查询对应的补卡规则详情。
--adjustment-id 必填。注意：已被删除或被更新覆盖的补卡规则无法查询到。`,
		Example: `  dws attendance adjustment get --adjustment-id 12345`,
		RunE: func(cmd *cobra.Command, args []string) error {
			adjustmentID, _ := cmd.Flags().GetInt64("adjustment-id")
			if adjustmentID == 0 {
				return fmt.Errorf("flag --adjustment-id is required")
			}
			return callMCPToolOnServer("attendance-wukong", "get_adjustment_rule_detail", map[string]any{
				"adjustmentId": adjustmentID,
			})
		},
	}

	// MCP tool: get_adjustment_rule
	attendanceAdjustmentSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "查询当前用户可管理的补卡规则列表",
		Long: `根据用户权限查询补卡规则列表，入参封装在 ATRuleQueryParam 对象中。
  --query  补卡规则名称关键字，模糊搜索（可选）
  --page   页码（从 1 开始，默认 1）（可选）
  --limit  每页条数（200 以内，默认 20）（可选）`,
		Example: `  dws attendance adjustment search --page 1 --limit 20
  dws attendance adjustment search --query "标准" --page 1 --limit 50`,
		RunE: func(cmd *cobra.Command, args []string) error {
			param := map[string]any{}
			if v := flagOrFallback(cmd, "query", "name"); v != "" {
				param["name"] = v
			}
			currentPage := intFlagOrFallback(cmd, "page", "current-page")
			if currentPage <= 0 {
				currentPage = 1
			}
			pageSize := intFlagOrFallback(cmd, "limit", "size")
			if pageSize <= 0 {
				pageSize = 20
			}
			param["currentPage"] = currentPage
			param["pageSize"] = pageSize
			return callMCPToolOnServer("attendance-wukong", "get_adjustment_rule", map[string]any{
				"ATRuleQueryParam": param,
			})
		},
	}

	// ── overtime-rule ──────────────────────────────────────

	attendanceOvertimeCmd := &cobra.Command{Use: "overtime", Short: "加班规则", RunE: groupRunE}

	// MCP tool: get_overtime_rule_detail
	attendanceOvertimeGetCmd := &cobra.Command{
		Use:   "get",
		Short: "根据加班规则主键 ID 查询加班规则详情",
		Long: `调用 MCP 工具 get_overtime_rule_detail，根据加班规则主键 ID 查询对应的加班规则详情。
--overtime-id 必填。已被删除或更新覆盖的加班规则也可以查到。`,
		Example: `  dws attendance overtime get --overtime-id 12345`,
		RunE: func(cmd *cobra.Command, args []string) error {
			overtimeID, _ := cmd.Flags().GetInt64("overtime-id")
			if overtimeID == 0 {
				return fmt.Errorf("flag --overtime-id is required")
			}
			return callMCPToolOnServer("attendance-wukong", "get_overtime_rule_detail", map[string]any{
				"overtimeId": overtimeID,
			})
		},
	}

	// MCP tool: get_overtime_rule
	attendanceOvertimeSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "查询当前用户可管理的加班规则列表",
		Long: `根据用户权限查询加班规则列表，入参封装在 ATRuleQueryParam 对象中。
  --query  加班规则名称关键字，模糊搜索（可选）
  --page   页码（从 1 开始，默认 1）（可选）
  --limit  每页条数（200 以内，默认 20）（可选）`,
		Example: `  dws attendance overtime search --page 1 --limit 20
  dws attendance overtime search --query "节假日" --page 1 --limit 50`,
		RunE: func(cmd *cobra.Command, args []string) error {
			param := map[string]any{}
			if v := flagOrFallback(cmd, "query", "name"); v != "" {
				param["name"] = v
			}
			currentPage := intFlagOrFallback(cmd, "page", "current-page")
			if currentPage <= 0 {
				currentPage = 1
			}
			pageSize := intFlagOrFallback(cmd, "limit", "size")
			if pageSize <= 0 {
				pageSize = 20
			}
			param["currentPage"] = currentPage
			param["pageSize"] = pageSize
			return callMCPToolOnServer("attendance-wukong", "get_overtime_rule", map[string]any{
				"ATRuleQueryParam": param,
			})
		},
	}

	// ── group ──────────────────────────────────────────────

	attendanceGroupCmd := &cobra.Command{Use: "group", Short: "考勤组", RunE: groupRunE}

	// MCP tool: get_simple_groups
	attendanceGroupSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "查询当前用户可管理的考勤组列表",
		Long: `根据当前用户的权限和筛选条件获取考勤组简要信息。
CLI 会在未传筛选条件时补齐默认查询字段，在未传分页参数时补齐 page=1、limit=20，以满足服务端非空对象约束。
  --query           考勤组名称关键字，模糊搜索
  --type            考勤组类型：FIXED（固定班制）/ TURN（排班制）/ NONE（自由工时）
  --query-position  是否查询地理定位和 Wifi 名称（默认 false）
  --query-ble       是否查询蓝牙设备列表（默认 false）
  --page            页码（从 1 开始，默认 1）
  --limit           每页条数（200 以内，默认 20）`,
		Example: `  dws attendance group search --query "研发"
  dws attendance group search --type FIXED --limit 50
  dws attendance group search --page 1 --limit 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			searchParam := map[string]any{}
			if v := flagOrFallback(cmd, "query", "name"); v != "" {
				searchParam["queryStr"] = v
			}
			if v, _ := cmd.Flags().GetString("type"); v != "" {
				searchParam["type"] = v
			}
			if cmd.Flags().Changed("query-position") {
				v, _ := cmd.Flags().GetBool("query-position")
				searchParam["queryPositionAndWifiNames"] = v
			}
			if cmd.Flags().Changed("query-ble") {
				v, _ := cmd.Flags().GetBool("query-ble")
				searchParam["queryBleDeviceList"] = v
			}
			// 服务端要求 param 不能为 null，如果用户未传任何过滤条件，补齐两个 bool 默认字段
			if len(searchParam) == 0 {
				searchParam["queryPositionAndWifiNames"] = false
				searchParam["queryBleDeviceList"] = false
			}
			pageQuery := map[string]any{}
			if cmd.Flags().Changed("page") || cmd.Flags().Changed("page-index") {
				if v := intFlagOrFallback(cmd, "page", "page-index"); v > 0 {
					pageQuery["pageIndex"] = v
				}
			}
			if cmd.Flags().Changed("limit") || cmd.Flags().Changed("size") {
				if v := intFlagOrFallback(cmd, "limit", "size"); v > 0 {
					pageQuery["pageSize"] = v
				}
			}
			// PageQuery 两个子字段均必填，未显式传参时补全默认值
			if _, ok := pageQuery["pageIndex"]; !ok {
				pageQuery["pageIndex"] = 1
			}
			if _, ok := pageQuery["pageSize"]; !ok {
				pageQuery["pageSize"] = 20
			}
			return callMCPToolOnServer("attendance-wukong", "get_simple_groups", map[string]any{
				"param":     searchParam,
				"pageQuery": pageQuery,
			})
		},
	}

	// MCP tool: get_group_detail
	attendanceGroupGetCmd := &cobra.Command{
		Use:   "get",
		Short: "根据考勤组 ID 查询考勤组全量信息",
		Long: `调用 MCP 工具 get_group_detail，根据考勤组 ID 查询该考勤组的全量信息。
--group-id 必填。如果只需查询成员、打卡地址、蓝牙、Wifi 子集，请使用 group filtered-get 以节省查询成本。`,
		Example: `  dws attendance group get --group-id 123456`,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := int64FlagOrFallback(cmd, "group-id", "id")
			if groupID == 0 {
				return fmt.Errorf("flag --group-id (or --id) is required")
			}
			return callMCPToolOnServer("attendance-wukong", "get_group_detail", map[string]any{
				"groupId": groupID,
			})
		},
	}

	// MCP tool: get_group_filtered_detail
	attendanceGroupFilteredGetCmd := &cobra.Command{
		Use:   "filtered-get",
		Short: "根据考勤组 ID 按需查询成员/打卡地址/蓝牙/Wifi 信息",
		Long: `调用 MCP 工具 get_group_filtered_detail，根据考勤组 ID 仅返回关心的字段子集，
强烈建议在仅需查询成员信息、打卡地址、蓝牙、Wifi 时使用，节省查询成本。
--group-id 必填；四个过滤字段均可选（不传则默认 false，即不查询该字段）。
  --member    是否查询考勤组成员信息
  --position  是否查询打卡地址
  --wifi      是否查询打卡 Wifi
  --bles      是否查询打卡蓝牙`,
		Example: `  dws attendance group filtered-get --group-id 123456 --member
  dws attendance group filtered-get --group-id 123456 --position --wifi`,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := int64FlagOrFallback(cmd, "group-id", "id")
			if groupID == 0 {
				return fmt.Errorf("flag --group-id (or --id) is required")
			}
			member, _ := cmd.Flags().GetBool("member")
			position, _ := cmd.Flags().GetBool("position")
			wifi, _ := cmd.Flags().GetBool("wifi")
			bles, _ := cmd.Flags().GetBool("bles")
			return callMCPToolOnServer("attendance-wukong", "get_group_filtered_detail", map[string]any{
				"groupId": groupID,
				"GroupResultFilter": map[string]any{
					"includeMember":   member,
					"includePosition": position,
					"includeWifi":     wifi,
					"includeBles":     bles,
				},
			})
		},
	}

	// MCP tool: update_group_member
	attendanceGroupUpdateMembersCmd := &cobra.Command{
		Use:   "update-members",
		Short: "更新考勤组成员（添加/删除考勤人员、部门、无需考勤人员）",
		Long: `调用 MCP 工具 update_group_member，对指定考勤组的成员进行增删操作。
--group-id 必填；所有成员/部门参数均可选，但至少需要传一个变更项。由于保存考勤组耗时较久，指令需要带上--timeout 10
每次调用各参数最多传 20 个 ID。
--yes 需要用户二次确认之后才允许带入，参考attendance.md中对于更新考勤组成员的说明
  --add-users           添加考勤人员 userId 列表，逗号分隔
  --remove-users        删除考勤人员 userId 列表，逗号分隔
  --add-extra-users     添加无需考勤的人员 userId 列表，逗号分隔
  --remove-extra-users  删除无需考勤的成员 userId 列表，逗号分隔
  --add-depts           添加考勤部门 ID 列表，逗号分隔，若要添加全公司，根部门id为-1
  --remove-depts        删除考勤部门 ID 列表，逗号分隔，全公司根部门id为-1`,
		Example: `  dws attendance group update-members --group-id 123456 --add-users userId1,userId2 --timeout 10
  dws attendance group update-members --group-id 123456 --remove-users userId1 --timeout 10
  dws attendance group update-members --group-id 123456 --add-depts deptId1 --remove-users userId2 --timeout 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID, _ := cmd.Flags().GetInt64("group-id")
			if groupID == 0 {
				return fmt.Errorf("flag --group-id is required")
			}

			updateParam := map[string]any{}

			if v, _ := cmd.Flags().GetString("add-users"); v != "" {
				updateParam["addUsers"] = parseUserList(v)
			}
			if v, _ := cmd.Flags().GetString("remove-users"); v != "" {
				updateParam["removeUsers"] = parseUserList(v)
			}
			if v, _ := cmd.Flags().GetString("add-extra-users"); v != "" {
				updateParam["addExtraUsers"] = parseUserList(v)
			}
			if v, _ := cmd.Flags().GetString("remove-extra-users"); v != "" {
				updateParam["removeExtraUsers"] = parseUserList(v)
			}
			if v, _ := cmd.Flags().GetString("add-depts"); v != "" {
				updateParam["addDepts"] = parseUserList(v)
			}
			if v, _ := cmd.Flags().GetString("remove-depts"); v != "" {
				updateParam["removeDepts"] = parseUserList(v)
			}

			if len(updateParam) == 0 {
				return fmt.Errorf("至少需要指定一个变更项：--add-users / --remove-users / --add-extra-users / --remove-extra-users / --add-depts / --remove-depts")
			}

			// 确认提示（除非 --yes）
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				fmt.Printf("About to update members of attendance group: %d\n", groupID)
				fmt.Print("Confirm update? (yes/no): ")
				var answer string
				if _, err := fmt.Scanln(&answer); err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "yes" && answer != "y" {
					return fmt.Errorf("update cancelled")
				}
			}

			return callMCPToolOnServer("attendance-wukong", "update_group_member", map[string]any{
				"groupId":     groupID,
				"updateParam": updateParam,
			})
		},
	}

	// MCP tool: create_group_setting
	attendanceGroupCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建考勤组",
		Long: `调用 MCP 工具 create_group_setting，创建一个新的考勤组。
--name 和 --type 必填，其余参数可选（未传字段使用默认值）。由于创建考勤组耗时较久，指令需要带上 --timeout 10
--yes 需要用户二次确认之后才允许带入，参考attendance.md中对于创建考勤组的说明

常用 flag：
  --name                   考勤组名称（必填）
  --type                   考勤组类型：FIXED（固定班制）/ TURN（排班制）/ NONE（自由工时）（必填，显式校验）
  --owner                  考勤组主负责人 userId（可选）
  --group-vo               完整 groupVO JSON 字符串（可选，用于传入复杂子对象，会与 --name/--type/--owner 合并）

当使用 --group-vo 传入完整 JSON 时，--name、--type、--owner 仍可覆写 JSON 中的同名字段。

【无条件必填字段】
  name      string  考勤组名称
  type      string  考勤组类型：FIXED / TURN / NONE

【条件必填字段（type=FIXED 固定班制时）】
  workDayClassList    []number  工作日班次列表，不能为空。
                             一共7个值，分别代表周日到周六每天的班次id，为0表示当天休息。
                             例如 [0,1279240003,0,0,0,0,0] 表示周一的班次id为1279240003，其余日子都是休息
  defaultClassId      number   默认班次 ID，不能为 null

条件必填字段通过 --group-vo JSON 传入，CLI 在 --type=FIXED 时会显式校验这些字段。

--group-vo 可用字段说明（与更新考勤组一致，均可选，只需包含要设置的字段）：

【基础信息】
  id            number    考勤组id（创建时禁止传入，由服务端分配）
  name          string    考勤组名称（必填）
  type          string    考勤组类型：FIXED（固定班制）/ TURN（排班制）/ NONE（自由工时）（必填）
  owner         string    考勤组主负责人 userId
  managerList   []string  考勤组子负责人 userId 列表
  skipHolidays  bool      节假日自动排休（只有固定班制和自由工时考勤组生效）
  defaultGroup  bool      是否默认考勤组
  classIds      []number  所选班次 id 列表(只有固定班制和排班制才有班次，自由工时没有)

【打卡范围与定位】
  trimDistance                      number    定位允许微调距离（米）
  enablePositionOfGps               bool      打卡是否允许开启 GPS 定位
  enablePositionOfWifi              bool      是否开启 wifi 定位
  enablePositionOfBle               bool      是否启用蓝牙定位

  positions      []object  考勤打卡地址列表（地址定位打卡用），每项字段：
    - title      string    地址标题
    - address    string    地址详情（完整地址文本）
    - latitude   number    纬度
    - longitude  number    经度
    - offset     number    该地址允许的打卡范围，单位米

  wifis          []object  打卡方式中的 wifi 详情列表，每项字段：
    - ssid       string    wifi 的 ssid（wifi名称）
    - macAddr    string    wifi 的 macAddr（MAC 地址）
    - groupId    number    考勤组id

  bleDeviceVOList  []object  已选择的蓝牙设备列表，每项字段：
    - name        string    设备名称
    - deviceUid   number    设备 uid
    - sn          string    设备 sn 序列号
    - productType string    对外的设备类型
    - devServId   number    内部设备类型

【外勤打卡设置】
  enableOutsideCheck                bool      是否可以外勤打卡
  enableOutsideCameraCheck          bool      是否开启外勤打卡必须拍照
  enableOutsideRemark               bool      是否开启外勤打卡必须填写备注
  enableOutsideApply                bool      是否开启外勤打卡必须审批
  forbidHideOutSideAddress          bool      禁止隐藏外勤打卡地址的功能，false表示允许用户隐藏外勤打卡地址
  enableOutSideUpdateNormalCheck    bool      下班时间：允许外勤卡更新内勤卡
  enableOnDutyNormalUpdateOutsideCheck bool   上班时间：允许内勤卡更新外勤卡
  outsideCheckApproveMode           string    外勤打卡需审批执行模式：
                                                NO_NEED_APPROVE（无需审批）
                                                APPROVE_FIRST（先审批后打卡）
                                                CHECK_FIRST（先打卡后审批）
                                                APPROVE_EVERYTIME（每次打卡都需要审批）
  outSideCheckApplyType             number    外勤审批范围：1（全天外勤打卡）/ 2（上班外勤打卡）/ 3（下班外勤打卡）

【打卡方式】
  enableCameraCheck    bool      是否开启拍照打卡
  openCameraCheck      bool      是否开启拍照打卡（与 enableCameraCheck 含义相同）
  openFaceCheck        bool      是否开启人脸打卡
  enableFaceStrictMode bool      是否开启人脸严格模式（强制活体检测）
  enableFaceBeauty     bool      是否开启美颜
  onlyMachineCheck     bool      只允许考勤机打卡
  permitMaxBeaconCount number    允许绑定的智点设备数量
  disableCheckWhenRest bool      休息日打卡需审批（只在固定班制和排班制生效，true表示打卡需要提交审批单）


【固定班制设置（FIXED）】
  defaultClassId              number    固定班制的默认班次 id（type=FIXED 时必填）
  workDayClassList            []number  固定班制的工作日班次 id 列表，一共7个值，分别代表周日到周六每天的班次id，为0表示当天休息，例如[0,1279240003,0,0,0,0,0]表示周一的班次id为1279240003，其余日子都是休息

【排班制设置（TURN）】
  disableCheckWithoutSchedule bool      未排班时员工可打卡（true=未排班时禁止打卡）
  enableEmpSelectClass        bool      未排班时，员工可选班次打卡
  enableScheduleAutoMatch     bool      未排班时，系统自动匹配班次
  ----以下是排班制设置排班周期的，非必要，不是所有排班制考勤组都需要设置，不设置就是按照classIds中的班次由管理员自行排班-----
  cycleDays           number    排班循环天数，支持大小周（14天）
  startCycleDate      number    用户设置的大小周开始时间（时间戳，毫秒）
  cycleScheduleList   []object  循环排班设置列表，每项字段：
    - cycleName  string    周期名字（如"第一周"）
    - groupId    number    考勤组 id
    - isValid    string    是否有效：Y（有效）/ N（无效）
    - itemList   []object  排班设置列表，每项字段：
        - classId    number  班次 id
        - className  string  班次名称
        - isValid    string  是否有效（Y/N）

【自由工时设置（NONE）】
  workDays                    []number  自由工时的工作日，如 [1,2,3,4,5] 表示周一到周五（1=周一，7=周日）
  freeCheckDayStartMinOffset  number    自由打卡每日开始时间，距离 0 点的分钟数（如 480 表示 08:00 开始）
  freeCheckCoreTime           number    自由工时考勤组的打卡最短工作时长（分钟）
  freeCheckDemandWorkMinutes  number    自由工时考勤组要求的打卡时长（分钟）

  freeCheckSettingVO   object  自由工时打卡设置，包含字段：
    - freeCheckType                    string  自由工时打卡类型：
                                                 CYCLE（上下班交替打卡）
                                                 MAX_TIME_UPDATE（最大时间打卡，第一次为上班，后续均为下班卡）
    - freeWorkDayLackSwitch            bool    自由工时打卡规则：工作日没打卡记为缺卡（true=记缺卡）
    - freeOnDutyLackMinOffset          number  自由工时打卡规则：上班缺卡生成时间偏移分钟数
    - freeOffDutyLackMinOffset         number  自由工时打卡规则：下班缺卡生成时间偏移分钟数
    - delimitOffsetMinutesBetweenDays  number  自由工时前后两天的分割线（距 0 点分钟数，超过此时间则归属第二天）
    - freeCheckGapVO                   object  自由工时/休息日打卡间隔设置：
        - onOffCheckGapMinutes  number  上班打卡后，多久才可以打下班卡（分钟）
        - offOnCheckGapMinutes  number  下班打卡后，多久才可以打上班卡（分钟）

  freeGroupSpecialDayVO   object  自由工时考勤组特殊日期设置，包含字段：
    - specialOnDutyDays   []number  特殊需要打卡日期的时间戳列表（毫秒，原本休息日需额外打卡的日期）
    - specialOffDutyDays  []number  特殊无需打卡日期的时间戳列表（毫秒，原本工作日可免打卡的日期）`,
		Example: `  dws attendance group create --name "研发考勤组" --type FIXED --group-vo '{"defaultClassId":1170996821,"workDayClassList":[0,1170996821,0,0,0,0,0]}' --timeout 10
  dws attendance group create --name "排班组" --type TURN --timeout 10
  dws attendance group create --name "自由工时组" --type NONE --timeout 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 校验 --type 必填且合法
			groupType, _ := cmd.Flags().GetString("type")
			if groupType == "" {
				return fmt.Errorf("--type 是必填项，考勤组类型必须为 FIXED（固定班制）、TURN（排班制）或 NONE（自由工时）")
			}
			validTypes := map[string]bool{"FIXED": true, "TURN": true, "NONE": true}
			if !validTypes[groupType] {
				return fmt.Errorf("--type 值 %q 不合法，必须为 FIXED（固定班制）、TURN（排班制）或 NONE（自由工时）", groupType)
			}

			// 校验 --name 必填
			groupName, _ := cmd.Flags().GetString("name")
			if groupName == "" {
				return fmt.Errorf("--name 是必填项，请指定考勤组名称")
			}

			// 解析 --group-vo JSON，作为基础
			groupVO := map[string]any{}
			if raw, _ := cmd.Flags().GetString("group-vo"); raw != "" {
				if err := json.Unmarshal([]byte(raw), &groupVO); err != nil {
					return fmt.Errorf("invalid --group-vo JSON: %w", err)
				}
			}

			// 单字段 flag 覆写（优先级高于 --group-vo）
			if cmd.Flags().Changed("name") {
				groupVO["name"] = groupName
			}
			if cmd.Flags().Changed("type") {
				groupVO["type"] = groupType
			}
			if cmd.Flags().Changed("owner") {
				v, _ := cmd.Flags().GetString("owner")
				groupVO["owner"] = v
			}

			// 确保必填字段已填入 groupVO
			if groupVO["name"] == nil || groupVO["name"] == "" {
				return fmt.Errorf("考勤组名称(name)是必填项")
			}
			if groupVO["type"] == nil || groupVO["type"] == "" {
				return fmt.Errorf("考勤组类型(type)是必填项")
			}

			// type=FIXED 时校验条件必填字段
			if groupVO["type"] == "FIXED" {
				workDayClassList, ok := groupVO["workDayClassList"]
				if !ok || workDayClassList == nil {
					return fmt.Errorf("固定班制(FIXED)时 --group-vo 必须包含 workDayClassList（工作日班次列表，不能为空）")
				}
				if list, ok := workDayClassList.([]any); ok && len(list) == 0 {
					return fmt.Errorf("workDayClassList 不能为空数组，至少需包含一个班次 ID")
				}
				if defaultClassID, ok := groupVO["defaultClassId"]; !ok || defaultClassID == nil {
					return fmt.Errorf("固定班制(FIXED)时 --group-vo 必须包含 defaultClassId（默认班次 ID，不能为 null）")
				}
			}

			// 确认提示（除非 --yes）
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				fmt.Printf("About to create attendance group: %v (type: %v)\n", groupVO["name"], groupVO["type"])
				fmt.Print("Confirm create? (yes/no): ")
				var answer string
				if _, err := fmt.Scanln(&answer); err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "yes" && answer != "y" {
					return fmt.Errorf("create cancelled")
				}
			}

			// 调用创建接口并拿到响应文本，DryRun 走原有路径（预览参数不调用）。
			payload := map[string]any{"groupVO": groupVO}
			if deps.Caller.DryRun() {
				return callMCPToolOnServer("attendance-wukong", "create_group_setting", payload)
			}
			ctx := context.Background()
			respText, err := callMCPToolReturnTextOnServer(ctx, "attendance-wukong", "create_group_setting", payload)
			if err != nil {
				return err
			}

			// 输出响应（与公共 callMCPToolOnServer 路径保持一致）
			var parsed any
			parsedOk := json.Unmarshal([]byte(respText), &parsed) == nil
			if deps.Caller.Format() == "json" && parsedOk {
				if err := deps.Out.PrintJSON(parsed); err != nil {
					return err
				}
			} else {
				deps.Out.PrintRaw(respText)
			}

			// 仅在响应中同时拿到 corpId 与 groupId 时，额外输出钉钉 PC 端跳转链接
			if parsedOk {
				if respMap, ok := parsed.(map[string]any); ok {
					printGroupModifyDeeplink(respMap)
				}
			}
			return nil
		},
	}

	// MCP tool: update_group
	attendanceGroupUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新考勤组配置（仅修改需要变更的字段）",
		Long: `调用 MCP 工具 update_group_setting，更新考勤组配置信息。
--group-id 必填；其余参数均可选，仅需对要修改的字段进行赋値，未传的字段会自动从已有配置补充。由于保存考勤组耗时较久，指令需要带上--timeout 10
--yes 需要用户二次确认之后才允许带入，参考attendance.md中对于更新考勤组配置的说明

常用简单字段可直接用 flag 传入；复杂子对象（如打卡地址、wifi、蓝牙设备、循环排班等）请用 --group-vo JSON 字符串传入完整 groupVO 对象覆写。

常用 flag：
  --name                   考勤组名称
  --type                   考勤组类型：FIXED（固定班制）/ TURN（排班制）/ NONE（自由工时）
  --owner                  考勤组主负责人 userId
  --classIds               []number  所选班次 id 列表
  --enable-outside-check   是否允许外勤打卡 (true/false)
  --group-vo               完整 groupVO JSON 字符串（用于修改复杂子对象，会与其他 flag 合并，重复字段以 flag 为准）

--group-vo 可用字段说明（均可选，只需包含要修改的字段）：

【基础信息】
  id            number    考勤组id，不随更新变化
  name          string    考勤组名称
  type          string    考勤组类型：FIXED（固定班制）/ TURN（排班制）/ NONE（自由工时）
  owner         string    考勤组主负责人 userId
  managerList   []string  考勤组子负责人 userId 列表
  skipHolidays  bool      节假日自动排休（只有固定班制和自由工时考勤组生效）
  defaultGroup  bool      是否默认考勤组
  classIds      []number  所选班次 id 列表(只有固定班制和排班制才有班次，自由工时没有)

【打卡范围与定位】
  trimDistance                      number    定位允许微调距离（米）
  enablePositionOfGps               bool      打卡是否允许开启 GPS 定位
  enablePositionOfWifi              bool      是否开启 wifi 定位
  enablePositionOfBle               bool      是否启用蓝牙定位

  positions      []object  考勤打卡地址列表（地址定位打卡用），每项字段：
    - title      string    地址标题
    - address    string    地址详情（完整地址文本）
    - latitude   number    纬度
    - longitude  number    经度
    - offset     number    该地址允许的打卡范围，单位米

  wifis          []object  打卡方式中的 wifi 详情列表，每项字段：
    - ssid       string    wifi 的 ssid（wifi名称）
    - macAddr    string    wifi 的 macAddr（MAC 地址）
    - groupId    number    考勤组id

  bleDeviceVOList  []object  已选择的蓝牙设备列表，每项字段：
    - name        string    设备名称
    - deviceUid   number    设备 uid
    - sn          string    设备 sn 序列号
    - productType string    对外的设备类型
    - devServId   number    内部设备类型

【外勤打卡设置】
  enableOutsideCheck                bool      是否可以外勤打卡
  enableOutsideCameraCheck          bool      是否开启外勤打卡必须拍照
  enableOutsideRemark               bool      是否开启外勤打卡必须填写备注
  enableOutsideApply                bool      是否开启外勤打卡必须审批
  forbidHideOutSideAddress          bool      禁止隐藏外勤打卡地址的功能，false表示允许用户隐藏外勤打卡地址
  enableOutSideUpdateNormalCheck    bool      下班时间：允许外勤卡更新内勤卡
  enableOnDutyNormalUpdateOutsideCheck bool   上班时间：允许内勤卡更新外勤卡
  outsideCheckApproveMode           string    外勤打卡需审批执行模式：
                                                NO_NEED_APPROVE（无需审批）
                                                APPROVE_FIRST（先审批后打卡）
                                                CHECK_FIRST（先打卡后审批）
                                                APPROVE_EVERYTIME（每次打卡都需要审批）
  outSideCheckApplyType             number    外勤审批范围：1（全天外勤打卡）/ 2（上班外勤打卡）/ 3（下班外勤打卡）

【打卡方式】
  enableCameraCheck    bool      是否开启拍照打卡
  openCameraCheck      bool      是否开启拍照打卡（与 enableCameraCheck 含义相同）
  openFaceCheck        bool      是否开启人脸打卡
  enableFaceStrictMode bool      是否开启人脸严格模式（强制活体检测）
  enableFaceBeauty     bool      是否开启美颜
  onlyMachineCheck     bool      只允许考勤机打卡
  permitMaxBeaconCount number    允许绑定的智点设备数量
  disableCheckWhenRest bool      休息日打卡需审批（只在固定班制和排班制生效，true表示打卡需要提交审批单）


【固定班制设置（FIXED）】
  defaultClassId              number    固定班制的默认班次 id
  workDayClassList            []number  固定班制的工作日班次 id 列表，一共7个值，分别代表周日到周六每天的班次id，为0表示当天休息，例如[0,1279240003,0,0,0,0,0]表示周一的班次id为1279240003，其余日子都是休息

【排班制设置（TURN）】
  disableCheckWithoutSchedule bool      未排班时员工可打卡（true=未排班时禁止打卡）
  enableEmpSelectClass        bool      未排班时，员工可选班次打卡
  enableScheduleAutoMatch     bool      未排班时，系统自动匹配班次
  ----以下是排班制设置排班周期的，非必要，不是所有排班制考勤组都需要设置，不设置就是按照classIds中的班次由管理员自行排班-----
  cycleDays           number    排班循环天数，支持大小周（14天）
  startCycleDate      number    用户设置的大小周开始时间（时间戳，毫秒）
  cycleScheduleList   []object  循环排班设置列表，每项字段：
    - cycleName  string    周期名字（如"第一周"）
    - groupId    number    考勤组 id
    - isValid    string    是否有效：Y（有效）/ N（无效）
    - itemList   []object  排班设置列表，每项字段：
        - classId    number  班次 id
        - className  string  班次名称
        - isValid    string  是否有效（Y/N）

【自由工时设置（NONE）】
  workDays                    []number  自由工时的工作日，如 [1,2,3,4,5] 表示周一到周五（1=周一，7=周日）
  freeCheckDayStartMinOffset  number    自由打卡每日开始时间，距离 0 点的分钟数（如 480 表示 08:00 开始）
  freeCheckCoreTime           number    自由工时考勤组的打卡最短工作时长（分钟）
  freeCheckDemandWorkMinutes  number    自由工时考勤组要求的打卡时长（分钟）

  freeCheckSettingVO   object  自由工时打卡设置，包含字段：
    - freeCheckType                    string  自由工时打卡类型：
                                                 CYCLE（上下班交替打卡）
                                                 MAX_TIME_UPDATE（最大时间打卡，第一次为上班，后续均为下班卡）
    - freeWorkDayLackSwitch            bool    自由工时打卡规则：工作日没打卡记为缺卡（true=记缺卡）
    - freeOnDutyLackMinOffset          number  自由工时打卡规则：上班缺卡生成时间偏移分钟数
    - freeOffDutyLackMinOffset         number  自由工时打卡规则：下班缺卡生成时间偏移分钟数
    - delimitOffsetMinutesBetweenDays  number  自由工时前后两天的分割线（距 0 点分钟数，超过此时间则归属第二天）
    - freeCheckGapVO                   object  自由工时/休息日打卡间隔设置：
        - onOffCheckGapMinutes  number  上班打卡后，多久才可以打下班卡（分钟）
        - offOnCheckGapMinutes  number  下班打卡后，多久才可以打上班卡（分钟）

  freeGroupSpecialDayVO   object  自由工时考勤组特殊日期设置，包含字段：
    - specialOnDutyDays   []number  特殊需要打卡日期的时间戳列表（毫秒，原本休息日需额外打卡的日期）
    - specialOffDutyDays  []number  特殊无需打卡日期的时间戳列表（毫秒，原本工作日可免打卡的日期）`,
		Example: `  dws attendance group update --group-id 123456 --name "研发考勤组" --timeout 10
  dws attendance group update --group-id 123456 --owner userId1 --timeout 10
  dws attendance group update --group-id 123456 --classIds '[1374234767]' --timeout 10
  dws attendance group update --group-id 123456 --group-vo '{"positions":[{"title":"总部","address":"北京市...","latitude":39.9,"longitude":116.4,"offset":200}]}' --timeout 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID, _ := cmd.Flags().GetInt64("group-id")
			if groupID == 0 {
				return fmt.Errorf("flag --group-id is required")
			}

			// 先解析 --group-vo JSON，作为基础
			groupVO := map[string]any{}
			if raw, _ := cmd.Flags().GetString("group-vo"); raw != "" {
				if err := json.Unmarshal([]byte(raw), &groupVO); err != nil {
					return fmt.Errorf("invalid --group-vo JSON: %w", err)
				}
			}

			// 单字段 flag 覆写（优先级高于 --group-vo）
			if cmd.Flags().Changed("name") {
				v, _ := cmd.Flags().GetString("name")
				groupVO["name"] = v
			}
			if cmd.Flags().Changed("type") {
				v, _ := cmd.Flags().GetString("type")
				groupVO["type"] = v
			}
			if cmd.Flags().Changed("owner") {
				v, _ := cmd.Flags().GetString("owner")
				groupVO["owner"] = v
			}
			if cmd.Flags().Changed("enable-outside-check") {
				raw, _ := cmd.Flags().GetString("enable-outside-check")
				v, err := strconv.ParseBool(raw)
				if err != nil {
					return fmt.Errorf("--enable-outside-check 值必须为 true 或 false")
				}
				groupVO["enableOutsideCheck"] = v
			}
			if cmd.Flags().Changed("classIds") {
				raw, _ := cmd.Flags().GetString("classIds")
				var ids []any
				if err := json.Unmarshal([]byte(raw), &ids); err != nil {
					return fmt.Errorf("invalid --classIds JSON array: %w", err)
				}
				groupVO["classIds"] = ids
			}

			if len(groupVO) == 0 {
				return fmt.Errorf("至少需要指定一个修改项：--name / --type / --owner / --enable-outside-check / --classIds / --group-vo")
			}

			// 服务端要求 groupVO 中必须包含 id 字段
			groupVO["id"] = groupID

			// 确认提示（除非 --yes）
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				fmt.Printf("About to update attendance group: %d\n", groupID)
				fmt.Print("Confirm update? (yes/no): ")
				var answer string
				if _, err := fmt.Scanln(&answer); err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "yes" && answer != "y" {
					return fmt.Errorf("update cancelled")
				}
			}

			return callMCPToolOnServer("attendance-wukong", "update_group_setting", map[string]any{
				"groupId": groupID,
				"groupVO": groupVO,
			})
		},
	}

	// ── summary ──────────────────────────────────────────────
	attendanceSummaryCmd := &cobra.Command{
		Use:   "summary",
		Short: "查询某个人的考勤统计摘要",
		Long: `查询某个人的考勤统计摘要。--user、--date、--stats-type 均必填。
statsType 统计类型支持：week（周统计）、month（月统计）。`,
		Example: `  dws attendance summary --user USER_ID --date 2026-03-12 --stats-type week  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "user", "date", "stats-type"); err != nil {
				return err
			}
			userID := mustGetFlag(cmd, "user")
			dateStr := mustGetFlag(cmd, "date")
			statsType := mustGetFlag(cmd, "stats-type")

			// 解析日期为毫秒时间戳（queryDate 为 number 类型）
			queryDate, err := parseDateToTimestamp(dateStr, "date")
			if err != nil {
				return err
			}

			vo := map[string]any{
				"userId":    userID,
				"queryDate": queryDate,
				"statsType": statsType,
			}
			return callMCPToolOnServer("attendance-wukong", "get_user_attendance_summary", vo)
		},
	}

	// ── rules ────────────────────────────────────────────────

	// MCP tool: query_attendance_group_or_rules
	attendanceRulesCmd := &cobra.Command{
		Use:   "rules",
		Short: "查询考勤组与考勤规则",
		Long: `调用 MCP 工具 query_attendance_group_or_rules 查询考勤组/考勤规则。
例如：我属于哪个考勤组、打卡范围是什么、弹性工时怎么算。`,
		Example: `  dws attendance rules --date 2026-03-14
  dws attendance rules --date "2026-03-14 09:00:00"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "date"); err != nil {
				return err
			}
			dateStr := mustGetFlag(cmd, "date")
			// 支持 YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss，统一转为 yyyy-MM-dd HH:mm:ss
			var dateFormatted string
			if t, err := time.ParseInLocation("2006-01-02 15:04:05", dateStr, time.Local); err == nil {
				dateFormatted = t.Format("2006-01-02 15:04:05")
			} else if t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local); err == nil {
				dateFormatted = t.Format("2006-01-02 15:04:05")
			} else {
				return fmt.Errorf("invalid --date format, use YYYY-MM-DD or yyyy-MM-dd HH:mm:ss (e.g. 2026-03-14 or 2026-03-14 09:00:00)")
			}
			return callMCPTool("query_attendance_group_or_rules", map[string]any{
				"date": dateFormatted,
			})
		},
	}

	// ── selfsetting ─────────────────────────────────────────────
	attendanceSelfSettingCmd := &cobra.Command{
		Use:   "selfsetting",
		Short: "个人规则设置",
		Long: `个人规则设置相关命令。

子命令:
  get   查询个人规则设置，包括打卡提醒、极速打卡、打卡结果通知、缺卡提醒、个人考勤统计通知、团队考勤统计通知等设置项。
  save  更新保存个人规则设置；settingScene 必填，且对应场景至少传入一个设置字段。`,
		RunE: groupRunE,
	}

	// MCP tool: query_self_setting
	attendanceSelfSettingGetCmd := &cobra.Command{
		Use:   "get",
		Short: "查询个人规则设置",
		Long: `查询个人规则设置，包括打卡提醒、极速打卡、打卡结果通知、缺卡提醒、个人考勤统计通知、团队考勤统计通知等设置项。

入参字段:
  --setting-scene  查询设置项，枚举值包括：
                   checkRemind（打卡提醒）
                   fastCheck（极速打卡）
                   checkResultNotify（打卡结果通知）
                   lackRemind（缺卡提醒）
                   personalAttendStatNotify（个人考勤统计通知）
                   bossAttendStatNotify（团队考勤统计通知）
  --user           必填，查询用户 ID，对应 MCP 参数 userId

说明：corpId 和 opUserId 由当前登录上下文自动补齐，不需要通过 CLI 入参传入。

出参字段说明:
  顶层返回 ServiceResult:
    success  请求是否成功
    code     请求结果码
    message  请求结果说明
    result   个人规则设置结果；服务端可能根据 --setting-scene 仅返回对应设置项相关字段

  result 通用字段:
    corpId  企业 ID
    userId  用户 ID

  settingScene=checkRemind（打卡提醒）相关字段:
    checkRemindSetting                 DING 提醒渠道设置
    checkRemindSetting.onDutyRemind    上班打卡提醒设置，包含 openRemind、remindMinutes(负数标识几分钟前提醒，正数几分钟后提醒)
    checkRemindSetting.offDutyRemind   下班打卡提醒设置，包含 openRemind、remindMinutes(负数标识几分钟前提醒，正数几分钟后提醒)
    checkRemindUserOnDuty              工作通知渠道：用户个人上班打卡提醒开关
    checkRemindUserOffDuty             工作通知渠道：用户个人下班打卡提醒开关
    enableOndutyCheckRemindOfPc        PC 端弹窗渠道：上班打卡提醒开关
    enableOffdutyCheckRemindOfPc       PC 端弹窗渠道：下班打卡提醒开关

  settingScene=fastCheck（极速打卡）相关字段:
    ondutyCheckType                    上班打卡方式：1 提醒打卡，2 不提醒且不自动打卡，3 自动打卡
    ondutyRemindStartMin               上班打卡提醒开始时间，单位：分钟
    ondutyRemindEndMin                 上班打卡提醒结束时间，单位：分钟
    fastCheckLateNeedConfirm           迟到时是否需要二次确认
    offdutyCheckType                   下班打卡方式：1 提醒打卡，2 不提醒且不自动打卡，3 自动打卡
    offdutyRemindStartMin              下班打卡提醒开始时间，单位：分钟
    offdutyRemindEndMin                下班打卡提醒结束时间，单位：分钟
    canUpdateOffDuty                   是否允许用户更新下班打卡设置
    voiceRemindSwitch                  极速打卡提示音开关
    vibrationRemindSwitch              极速打卡震动提醒开关

  settingScene=checkResultNotify（打卡结果通知）相关字段:
    checkResultMsg                     打卡结果通知开关：0 关闭，1 开启

  settingScene=lackRemind（缺卡提醒）相关字段:
    lackSendTodoMsg                    待办提醒开关：0 关闭，null 或 1 开启
    lackRemindUser                     工作通知渠道：用户个人缺卡提醒开关：0 关闭，null 或 1 开启

  settingScene=personalAttendStatNotify（个人考勤统计通知）相关字段:
    personDailyReportSwitch            日报推送开关：0 关闭，1 开启
    personWeekReportType               周报通知渠道：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮
    personMonthReportType              月报通知渠道：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮

  settingScene=bossAttendStatNotify（团队考勤统计通知）相关字段:
    bossPushStartMin                   日报推送开始时间，单位：分钟；-1 表示关闭日报推送
    bossWeekReportType                 周报通知渠道：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮
    bossMonthReportType                月报通知渠道：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮`,
		Example: `  dws attendance selfsetting get --setting-scene checkRemind --user USER_ID
  dws attendance selfsetting get --setting-scene fastCheck --user USER_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "setting-scene", "user"); err != nil {
				return err
			}
			settingScene := mustGetFlag(cmd, "setting-scene")
			allowedSettingScenes := map[string]bool{
				"checkRemind":              true,
				"fastCheck":                true,
				"checkResultNotify":        true,
				"lackRemind":               true,
				"personalAttendStatNotify": true,
				"bossAttendStatNotify":     true,
			}
			if !allowedSettingScenes[settingScene] {
				return fmt.Errorf("invalid --setting-scene %q, supported: checkRemind, fastCheck, checkResultNotify, lackRemind, personalAttendStatNotify, bossAttendStatNotify", settingScene)
			}

			userID := mustGetFlag(cmd, "user")

			request := map[string]any{
				"settingScene": settingScene,
				"userId":       userID,
			}
			return callMCPToolOnServer("attendance-wukong", "query_self_setting", map[string]any{
				"RuleMcpQuerySelfSettingRequest": request,
			})
		},
	}

	// MCP tool: save_self_setting
	attendanceSelfSettingSaveCmd := &cobra.Command{
		Use:   "save",
		Short: "更新保存个人规则设置",
		Long: `更新保存个人规则设置，包括打卡提醒、极速打卡、打卡结果通知、缺卡提醒、个人考勤统计通知、团队考勤统计通知等设置项。

入参字段:
  --setting-scene  必填，枚举值包括 checkRemind、fastCheck、checkResultNotify、lackRemind、personalAttendStatNotify、bossAttendStatNotify
  --user           必填，更新用户 ID，对应 MCP 参数 userId

按 settingScene 传入对应设置字段，且至少传一个字段：
  checkRemind:
    --check-remind-setting
    --check-remind-user-on-duty
    --check-remind-user-off-duty
    --enable-onduty-check-remind-of-pc
    --enable-offduty-check-remind-of-pc
  fastCheck:
    --onduty-check-type
    --offduty-check-type
    --onduty-remind-start-min
    --onduty-remind-end-min
    --offduty-remind-start-min
    --offduty-remind-end-min
    --fast-check-late-need-confirm
    --can-update-off-duty
    --voice-remind-switch
    --vibration-remind-switch
  checkResultNotify:
    --check-result-msg
  lackRemind:
    --lack-send-todo-msg
    --lack-remind-user
  personalAttendStatNotify:
    --person-daily-report-switch
    --person-week-report-type
    --person-month-report-type
  bossAttendStatNotify:
    --boss-push-start-min
    --boss-week-report-type
    --boss-month-report-type

说明：corpId 和 opUserId 由当前登录上下文自动补齐，不需要通过 CLI 入参传入。
返回 ServiceResult，包含 success、code、message、result；result 为 boolean，表示保存是否成功。`,
		Example: `  dws attendance selfsetting save --setting-scene checkResultNotify --user USER_ID --check-result-msg 1
  dws attendance selfsetting save --setting-scene fastCheck --user USER_ID --onduty-check-type 3 --voice-remind-switch=true
  dws attendance selfsetting save --setting-scene checkRemind --user USER_ID --check-remind-user-on-duty=false --check-remind-setting '{"onDutyRemind":{"openRemind":true,"remindMinutes":10}}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "setting-scene", "user"); err != nil {
				return err
			}
			settingScene := mustGetFlag(cmd, "setting-scene")
			allowedSettingScenes := map[string]bool{
				"checkRemind":              true,
				"fastCheck":                true,
				"checkResultNotify":        true,
				"lackRemind":               true,
				"personalAttendStatNotify": true,
				"bossAttendStatNotify":     true,
			}
			if !allowedSettingScenes[settingScene] {
				return fmt.Errorf("invalid --setting-scene %q, supported: checkRemind, fastCheck, checkResultNotify, lackRemind, personalAttendStatNotify, bossAttendStatNotify", settingScene)
			}

			userID := mustGetFlag(cmd, "user")
			request, err := collectSelfSettingSaveRequest(cmd, settingScene, userID)
			if err != nil {
				return err
			}

			// 确认提示（除非 --yes）
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				fmt.Printf("About to save self setting [%s] for user: %s\n", settingScene, userID)
				fmt.Print("Confirm save? (yes/no): ")
				var answer string
				if _, err := fmt.Scanln(&answer); err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "yes" && answer != "y" {
					return fmt.Errorf("save cancelled")
				}
			}

			return callMCPToolOnServer("attendance-wukong", "save_self_setting", map[string]any{
				"RuleMcpSaveSelfSettingRequest": request,
			})
		},
	}

	// ── globalsetting ────────────────────────────────────────
	attendanceGlobalSettingCmd := &cobra.Command{
		Use:   "globalsetting",
		Short: "全局规则设置（仅管理员）",
		Long: `全局规则设置相关命令，仅管理员可以调用。

子命令:
  get   查询全局规则设置，包括打卡提醒、极速打卡、打卡结果通知、缺卡提醒、个人考勤统计通知、团队考勤统计通知等设置项。
  save  更新保存全局规则设置；settingScene 必填，且对应场景至少传入一个设置字段。`,
		RunE: groupRunE,
	}

	// MCP tool: query_global_setting
	attendanceGlobalSettingGetCmd := &cobra.Command{
		Use:   "get",
		Short: "查询全局规则设置（仅管理员）",
		Long: `查询全局规则设置，仅管理员可以调用。

入参字段:
  --setting-scene  查询设置项，枚举值包括 checkRemind、fastCheck、checkResultNotify、lackRemind、personalAttendStatNotify、bossAttendStatNotify
  --scope          必填，必须明确输入 企业、全公司 或 所有人，用于确认本次查询面向全局范围

说明：corpId 和 opUserId 由当前登录上下文自动补齐，不需要通过 CLI 入参传入。`,
		Example: `  dws attendance globalsetting get --scope 企业 --setting-scene checkRemind
  dws attendance globalsetting get --scope 全公司 --setting-scene bossAttendStatNotify`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "setting-scene"); err != nil {
				return err
			}
			if err := validateGlobalSettingScope(cmd); err != nil {
				return err
			}
			settingScene := mustGetFlag(cmd, "setting-scene")
			if !isAttendanceSettingSceneAllowed(settingScene) {
				return fmt.Errorf("invalid --setting-scene %q, supported: checkRemind, fastCheck, checkResultNotify, lackRemind, personalAttendStatNotify, bossAttendStatNotify", settingScene)
			}

			return callMCPToolOnServer("attendance-wukong", "query_global_setting", map[string]any{
				"RuleMcpQueryGlobalSettingRequest": map[string]any{
					"settingScene": settingScene,
				},
			})
		},
	}

	// MCP tool: save_global_setting
	attendanceGlobalSettingSaveCmd := &cobra.Command{
		Use:   "save",
		Short: "更新保存全局规则设置（仅管理员）",
		Long: `更新保存全局规则设置，仅管理员可以调用。

入参字段:
  --setting-scene  必填，枚举值包括 checkRemind、fastCheck、checkResultNotify、lackRemind、personalAttendStatNotify、bossAttendStatNotify
  --scope          必填，必须明确输入 企业、全公司 或 所有人，用于确认本次更新面向全局范围

按 settingScene 传入对应设置字段，且至少传一个字段：
  checkRemind:
    --check-remind-corp
    --check-remind-pc-corp
  fastCheck:
    --fast-check-corp
  checkResultNotify:
    --enable-check-cert-push
  lackRemind:
    --lack-remind-corp
  personalAttendStatNotify:
    --enable-personal-daily-report
    --enable-personal-weekly-report
    --enable-personal-weekly-report-card
    --enable-personal-monthly-report
  bossAttendStatNotify:
    --boss-daily-report-type
    --boss-weekly-report-type
    --boss-monthly-report-type

说明：corpId 和 opUserId 由当前登录上下文自动补齐，不需要通过 CLI 入参传入。
返回 ServiceResult，result 为 boolean，表示保存是否成功。`,
		Example: `  dws attendance globalsetting save --scope 企业 --setting-scene checkRemind --check-remind-corp=true --yes
  dws attendance globalsetting save --scope 全公司 --setting-scene fastCheck --fast-check-corp=false --yes
  dws attendance globalsetting save --scope 所有人 --setting-scene bossAttendStatNotify --boss-daily-report-type 1 --boss-weekly-report-type 3 --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "setting-scene"); err != nil {
				return err
			}
			if err := validateGlobalSettingScope(cmd); err != nil {
				return err
			}
			settingScene := mustGetFlag(cmd, "setting-scene")
			if !isAttendanceSettingSceneAllowed(settingScene) {
				return fmt.Errorf("invalid --setting-scene %q, supported: checkRemind, fastCheck, checkResultNotify, lackRemind, personalAttendStatNotify, bossAttendStatNotify", settingScene)
			}

			request, err := collectGlobalSettingSaveRequest(cmd, settingScene)
			if err != nil {
				return err
			}

			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				fmt.Printf("About to save global setting [%s]\n", settingScene)
				fmt.Print("Confirm save? (yes/no): ")
				var answer string
				if _, err := fmt.Scanln(&answer); err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "yes" && answer != "y" {
					return fmt.Errorf("save cancelled")
				}
			}

			return callMCPToolOnServer("attendance-wukong", "save_global_setting", map[string]any{
				"RuleMcpSaveGlobalSettingRequest": request,
			})
		},
	}

	// ── report ──────────────────────────────────────────────

	attendanceReportCmd := &cobra.Command{
		Use:   "report",
		Short: "查询考勤报表和结果",
		Long: `考勤 MCP 报表接口，仅对管理员开放

子命令:
  columns     获取企业考勤字段列表
  query-data  根据字段查询考勤数据
  query-leave 查询用户假期数据`,
		RunE: groupRunE,
	}

	// MCP tool: get_report_columns
	reportColumnsCmd := &cobra.Command{
		Use:   "columns",
		Short: "获取企业考勤字段列表",
		Long: `根据操作者的列权限，过滤并返回其有权查看的考勤字段列表。
操作者必须是管理员，否则返回权限错误。`,
		Example: `  dws attendance report columns`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPToolOnServer("attendance-wukong", "get_report_columns", map[string]any{})
		},
	}

	// MCP tool: get_report_columns_value
	reportQueryDataCmd := &cobra.Command{
		Use:   "query-data",
		Short: "根据字段查询考勤数据",
		Long: `根据字段查询考勤数据，含列权限过滤和用户查看权限校验。
--columns 为字段 ID 列表（逗号分隔），可通过 dws attendance report columns 获取。
--users 最多 20 人，传入userId，--start 到 --end 不超过 32 天。`,
		Example: `  dws attendance report query-data \
    --users userId1,userId2 --columns 1001,1002 --start "2026-03-01 00:00:00" --end "2026-03-31 23:59:59"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "users", "columns", "start", "end"); err != nil {
				return err
			}
			startStr, endStr := mustGetFlag(cmd, "start"), mustGetFlag(cmd, "end")
			if _, err := time.ParseInLocation("2006-01-02 15:04:05", startStr, time.Local); err != nil {
				return fmt.Errorf("invalid --start date format, use yyyy-MM-dd HH:mm:ss: %w", err)
			}
			if _, err := time.ParseInLocation("2006-01-02 15:04:05", endStr, time.Local); err != nil {
				return fmt.Errorf("invalid --end date format, use yyyy-MM-dd HH:mm:ss: %w", err)
			}
			var userIds []string
			for _, u := range strings.Split(mustGetFlag(cmd, "users"), ",") {
				if s := strings.TrimSpace(u); s != "" {
					userIds = append(userIds, s)
				}
			}
			var columnIds []int64
			for _, c := range strings.Split(mustGetFlag(cmd, "columns"), ",") {
				c = strings.TrimSpace(c)
				if c == "" {
					continue
				}
				var columnId int64
				if _, err := fmt.Sscanf(c, "%d", &columnId); err != nil {
					return fmt.Errorf("invalid column id %q, must be a number: %w", c, err)
				}
				columnIds = append(columnIds, columnId)
			}
			return callMCPToolOnServer("attendance-wukong", "get_report_columns_value", map[string]any{
				"McpQueryParam": map[string]any{
					"targetUserIds": userIds,
					"columnIds":     columnIds,
					"fromDate":      startStr,
					"toDate":        endStr,
				},
			})
		},
	}

	// MCP tool: get_leave_time_by_leave_names
	reportQueryLeaveCmd := &cobra.Command{
		Use:   "query-leave",
		Short: "查询用户假期数据",
		Long: `查询用户假期数据，含用户查看权限校验。
--leave-names 为假期类型名称列表（逗号分隔），不传则查询所有假期类型。
--users 最多 20 人，传入userId，--start 到 --end 不超过 32 天。`,
		Example: `  dws attendance report query-leave \
    --users userId1,userId2 --leave-names 年假,病假 --start "2026-03-01 00:00:00" --end "2026-03-31 23:59:59"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "users", "start", "end"); err != nil {
				return err
			}
			startStr, endStr := mustGetFlag(cmd, "start"), mustGetFlag(cmd, "end")
			if _, err := time.ParseInLocation("2006-01-02 15:04:05", startStr, time.Local); err != nil {
				return fmt.Errorf("invalid --start date format, use yyyy-MM-dd HH:mm:ss: %w", err)
			}
			if _, err := time.ParseInLocation("2006-01-02 15:04:05", endStr, time.Local); err != nil {
				return fmt.Errorf("invalid --end date format, use yyyy-MM-dd HH:mm:ss: %w", err)
			}
			var userIds []string
			for _, u := range strings.Split(mustGetFlag(cmd, "users"), ",") {
				if s := strings.TrimSpace(u); s != "" {
					userIds = append(userIds, s)
				}
			}
			var leaveNames []string
			if raw := mustGetFlag(cmd, "leave-names"); raw != "" {
				for _, n := range strings.Split(raw, ",") {
					if s := strings.TrimSpace(n); s != "" {
						leaveNames = append(leaveNames, s)
					}
				}
			}
			return callMCPToolOnServer("attendance-wukong", "get_leave_time_by_leave_names", map[string]any{
				"McpLeaveQueryParam": map[string]any{
					"targetUserIds": userIds,
					"leaveNames":    leaveNames,
					"fromDate":      startStr,
					"toDate":        endStr,
				},
			})
		},
	}

	// ── 假期 vacation ───────────────────────────────────────────────

	attendanceVacationCmd := &cobra.Command{
		Use:   "vacation",
		Short: "假期管理",
		Long: `管理钉钉假期：查询假期规则列表、查询员工假期余额、查询假期余额变更记录。

子命令:
  types        查询当前用户假期规则列表
  update-type  更新假期规则（仅支持无额度模式）
  balance      查询指定员工假期余额
  save-balance 更新员工假期余额
  records      查询指定员工假期余额变更记录`,
		RunE: groupRunE,
	}

	// ── 假期规则 types ─────────────────────────────────────────

	// MCP tool: get_leave_types
	vacationTypesCmd := &cobra.Command{
		Use:   "types",
		Short: "查询当前用户假期规则列表",
		Long: `调用 MCP 工具 get_leave_types 查询当前用户可用的假期规则列表。
例如：年假、事假、病假等假期类型及对应规则。`,
		Example: `  dws attendance vacation types`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPToolOnServer("attendance-wukong", "get_leave_types", map[string]any{
				"McpLeaveTypeRequest": map[string]any{},
			})
		},
	}

	// ── 假期余额 balance ───────────────────────────────────────

	// MCP tool: get_leave_balance_quota
	vacationBalanceCmd := &cobra.Command{
		Use:   "balance",
		Short: "查询指定员工假期余额",
		Long: `调用 MCP 工具 get_leave_balance_quota 查询指定员工的假期余额。
例如：查询某员工年假还剩多少、病假额度等。`,
		Example: `  dws attendance vacation balance --users userId1,userId2 --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "users"); err != nil {
				return err
			}
			usersStr := mustGetFlag(cmd, "users")
			var targetUserIds []string
			for _, u := range strings.Split(usersStr, ",") {
				if s := strings.TrimSpace(u); s != "" {
					targetUserIds = append(targetUserIds, s)
				}
			}
			req := map[string]any{
				"targetUserIds": targetUserIds,
			}
			if leaveCode := mustGetFlag(cmd, "leave-code"); leaveCode != "" {
				req["leaveCode"] = leaveCode
			}
			return callMCPToolOnServer("attendance-wukong", "get_leave_balance_quota", map[string]any{
				"McpLeaveBalanceRequest": req,
			})
		},
	}

	// ── 假期记录 records ───────────────────────────────────────

	// MCP tool: get_leave_balance_records
	vacationRecordsCmd := &cobra.Command{
		Use:   "records",
		Short: "查询指定员工假期余额变更记录",
		Long: `调用 MCP 工具 get_leave_balance_records 查询指定员工的假期余额变更记录。
例如：查询某员工年假变更历史、请假扣减记录等。`,
		Example: `  dws attendance vacation records --user USER_ID --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --start 2026-04-01 --end 2026-04-22  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "user", "start", "end"); err != nil {
				return err
			}
			startStr, endStr := mustGetFlag(cmd, "start"), mustGetFlag(cmd, "end")
			startT, err := time.ParseInLocation("2006-01-02", startStr, time.Local)
			if err != nil {
				return fmt.Errorf("invalid --start date format, use YYYY-MM-DD: %w", err)
			}
			endT, err := time.ParseInLocation("2006-01-02", endStr, time.Local)
			if err != nil {
				return fmt.Errorf("invalid --end date format, use YYYY-MM-DD: %w", err)
			}
			req := map[string]any{
				"targetUserId":   mustGetFlag(cmd, "user"),
				"startTimeStamp": startT.UnixMilli(),
				"endTimeStamp":   endT.UnixMilli(),
			}
			if leaveCode := mustGetFlag(cmd, "leave-code"); leaveCode != "" {
				req["leaveCode"] = leaveCode
			}
			return callMCPToolOnServer("attendance-wukong", "get_leave_balance_records_v2", map[string]any{
				"McpLeaveRecordRequest": req,
			})
		},
	}

	// ── 假期规则更新 update-type ─────────────────────────────────────────

	// MCP tool: save_leave_type
	vacationUpdateTypeCmd := &cobra.Command{
		Use:   "update-type",
		Short: "更新假期规则",
		Long: `调用 MCP 工具 save_leave_type 更新已有假期规则。
--leave-code 必填，指定要更新的假期规则编码。
其他字段均为可选，仅需传入要修改的字段。

参数说明：
  --leave-code        假期编码（必填）
  --name              假期名称（可选）
  --unit              假期单位：day/halfDay/hour（可选）
  --paid              是否带薪假期（可选）
  --per-hours         一天折算小时数（可选）
  --when-can-leave    新员工请假规则：entry/formal（可选）
  --visibility-rules  适用范围规则 JSON 数组（可选）
  --user-say-yes      用户已确认，跳过交互式确认提示（Agent 调用时传 true 前必须完成用户二次确认）

--visibility-rules 适用范围语义（HSF 反序列化约定，必读）：
  - 不传：不修改原有可见范围。
  - 改为指定范围：传 [{"type":"staff|dept|label|employee_type","visible":["id1","id2"]}, ...]，
    每条规则 type 和 visible 都必须非空。
  - 改为全公司可见（哨兵约定）：必须显式传 [{"type":"dept","visible":["-1"]}]，
    服务端识别后会落库为空可见范围（即全公司）。
  - 反例（CLI 会拦截报错，避免被服务端静默忽略）：
    * 空数组 []、[{}]、[{"type":"dept","visible":[]}] 等被视为未传，不会生效为全公司可见；
    * 若要清空可见范围，必须显式传哨兵值 ["-1"]。`,
		Example: `  # 更新假期规则名称
  dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --name "事假（修改版）"

  # 更新假期单位
  dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --unit hour --per-hours 8

  # 改为指定部门可见
  dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --visibility-rules '[{"type":"dept","visible":["1","2","3"]}]'

  # 改为全公司可见（必须显式传哨兵值 "-1"，空数组 [] 不生效）
  dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --visibility-rules '[{"type":"dept","visible":["-1"]}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "leave-code"); err != nil {
				return err
			}

			// 检查除 leave-code 外是否至少传了一个更新字段
			updateFieldCount := 0
			if cmd.Flags().Changed("name") {
				updateFieldCount++
			}
			if cmd.Flags().Changed("unit") {
				updateFieldCount++
			}
			if cmd.Flags().Changed("paid") {
				updateFieldCount++
			}
			if cmd.Flags().Changed("per-hours") {
				updateFieldCount++
			}
			if cmd.Flags().Changed("when-can-leave") {
				updateFieldCount++
			}
			if cmd.Flags().Changed("visibility-rules") {
				updateFieldCount++
			}
			if updateFieldCount == 0 {
				return fmt.Errorf("至少需要指定一个更新字段：--name / --unit / --paid / --per-hours / --when-can-leave / --visibility-rules")
			}

			// 提前解析并校验 --visibility-rules：HSF 端无法区分「空数组」与「未传」，
			// 需在 CLI 层拦截空数组 / 全部无效规则，避免被服务端静默忽略。
			// 任一规则满足 type=dept && visible 含 "-1" 则视为哨兵（改为全公司可见）。
			var visibilityRules []map[string]any
			var visibilityRulesSentinel bool
			if cmd.Flags().Changed("visibility-rules") {
				visibilityRulesJSON := strings.TrimSpace(mustGetFlag(cmd, "visibility-rules"))
				if visibilityRulesJSON != "" {
					var raw []map[string]any
					if err := json.Unmarshal([]byte(visibilityRulesJSON), &raw); err != nil {
						return fmt.Errorf("invalid --visibility-rules JSON format: %w", err)
					}
					if len(raw) == 0 {
						return fmt.Errorf("--visibility-rules 不能是空数组 []：HSF 端无法区分 [] 与未传，会被视为「不修改可见范围」。如需改为全公司可见，请显式传 [{\"type\":\"dept\",\"visible\":[\"-1\"]}]")
					}
					for _, r := range raw {
						if visibilityRulesSentinel {
							break
						}
						t, _ := r["type"].(string)
						if t != "dept" {
							continue
						}
						vis, _ := r["visible"].([]any)
						for _, v := range vis {
							if s, ok := v.(string); ok && strings.TrimSpace(s) == "-1" {
								visibilityRulesSentinel = true
								break
							}
						}
					}
					if visibilityRulesSentinel {
						// 命中哨兵：只传哨兵规则，服务端会落库为空可见范围（全公司）
						visibilityRules = []map[string]any{
							{"type": "dept", "visible": []string{"-1"}},
						}
					} else {
						// 过滤无效规则（type 缺失或 visible 为空）
						valid := make([]map[string]any, 0, len(raw))
						for _, r := range raw {
							t, _ := r["type"].(string)
							if strings.TrimSpace(t) == "" {
								continue
							}
							vis, ok := r["visible"].([]any)
							if !ok || len(vis) == 0 {
								continue
							}
							valid = append(valid, r)
						}
						if len(valid) == 0 {
							return fmt.Errorf("--visibility-rules 中所有规则均无效（type 缺失或 visible 为空），服务端会视为「不修改可见范围」。如需改为全公司可见，请显式传 [{\"type\":\"dept\",\"visible\":[\"-1\"]}]")
						}
						visibilityRules = valid
					}
				}
			}

			// 确认提示（除非 --user-say-yes）
			yes := boolFlagOrFallback(cmd, "user-say-yes", "yes")
			if !yes {
				leaveCode := mustGetFlag(cmd, "leave-code")
				fmt.Printf("即将更新假期规则：\n")
				fmt.Printf("  编码：%s\n", leaveCode)
				if cmd.Flags().Changed("name") {
					fmt.Printf("  名称：%s\n", mustGetFlag(cmd, "name"))
				}
				if cmd.Flags().Changed("unit") {
					fmt.Printf("  单位：%s\n", mustGetFlag(cmd, "unit"))
				}
				if cmd.Flags().Changed("paid") {
					paid, _ := cmd.Flags().GetBool("paid")
					fmt.Printf("  带薪：%v\n", paid)
				}
				if cmd.Flags().Changed("visibility-rules") {
					fmt.Printf("  适用范围：%s\n", mustGetFlag(cmd, "visibility-rules"))
					if visibilityRulesSentinel {
						fmt.Println("    → 命中哨兵，将改为「全公司可见」（清空可见范围）")
					}
				}
				fmt.Print("确认更新？: ")

				var answer string
				if _, err := fmt.Scanln(&answer); err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "yes" && answer != "y" {
					return fmt.Errorf("update cancelled")
				}
			}

			// 构建 MCP 请求参数
			req := map[string]any{
				"leaveCode": mustGetFlag(cmd, "leave-code"),
			}

			// 可选参数（仅传入 Changed 的字段）
			if cmd.Flags().Changed("name") {
				req["leaveName"] = mustGetFlag(cmd, "name")
			}
			if cmd.Flags().Changed("unit") {
				req["leaveUnit"] = mustGetFlag(cmd, "unit")
			}
			if cmd.Flags().Changed("paid") {
				if paid, _ := cmd.Flags().GetBool("paid"); paid {
					req["paidLeave"] = true
				} else {
					req["paidLeave"] = false
				}
			}
			if cmd.Flags().Changed("per-hours") {
				if perHours, _ := cmd.Flags().GetInt("per-hours"); perHours > 0 {
					req["perHoursInDay"] = perHours
				}
			}
			if cmd.Flags().Changed("when-can-leave") {
				if whenCanLeave := mustGetFlag(cmd, "when-can-leave"); whenCanLeave != "" {
					req["whenCanLeave"] = whenCanLeave
				}
			}

			// visibility-rules 已在 RunE 顶部解析与校验，这里只需赋值
			if visibilityRules != nil {
				req["visibilityRules"] = visibilityRules
			}

			return callMCPToolOnServer("attendance-wukong", "save_leave_type", map[string]any{
				"McpSaveLeaveTypeRequest": req,
			})
		},
	}

	// ── 假期余额保存 save-balance ─────────────────────────────────────────

	// MCP tool: update_leave_balance
	vacationSaveBalanceCmd := &cobra.Command{
		Use:   "save-balance",
		Short: "设置员工假期余额",
		Long: `调用 MCP 工具 update_leave_balance 设置指定员工的假期余额数量。

重要说明：这是设置（SET）接口，传入的值会替换当前余额，而不是增加（ADD）。

参数说明：
  --target       目标员工工号（必填）
  --leave-code   假期编码（必填）
  --num          余额数量（必填，如8天传8，7.5天传7.5）
  --reason       变更原因（必填，最长100字符）
  --start        有效期开始日期 YYYY-MM-DD
  --end          有效期结束日期 YYYY-MM-DD
  --user-say-yes 用户已确认，跳过交互式确认提示（Agent 调用时传 true 前必须完成用户二次确认）

注意：
  - 余额数量在传递给 MCP 时会自动乘以100（如8天传800，7.5天传750）
  - 此接口为 SET 操作：传入值会直接设置为新的余额，而非累加到现有余额`,
		Example: `  # 设置员工年假余额为8天
  dws attendance vacation save-balance --target user001 --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --num 8 --reason "年度发放"

  # 设置有效期
  dws attendance vacation save-balance --target user001 --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --num 8 --reason "年度发放" --start 2024-01-01 --end 2024-12-31`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "target", "leave-code", "num", "reason"); err != nil {
				return err
			}

			// 确认提示（除非 --user-say-yes）
			yes := boolFlagOrFallback(cmd, "user-say-yes", "yes")
			if !yes {
				targetUser := mustGetFlag(cmd, "target")
				leaveCode := mustGetFlag(cmd, "leave-code")
				numStr := mustGetFlag(cmd, "num")
				reason := mustGetFlag(cmd, "reason")

				fmt.Printf("即将设置员工假期余额：\n")
				fmt.Printf("  目标员工：%s\n", targetUser)
				fmt.Printf("  假期编码：%s\n", leaveCode)
				fmt.Printf("  余额数量：%s\n", numStr)
				fmt.Printf("  变更原因：%s\n", reason)

				startStr := mustGetFlag(cmd, "start")
				endStr := mustGetFlag(cmd, "end")
				if startStr != "" && endStr != "" {
					fmt.Printf("  有效期：%s 至 %s\n", startStr, endStr)
				}

				fmt.Print("确认设置？: ")

				var answer string
				if _, err := fmt.Scanln(&answer); err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "yes" && answer != "y" {
					return fmt.Errorf("save cancelled")
				}
			}

			// 构建 MCP 请求参数
			// quotaNum 需要乘以100（如8天传800，7.5天传750）
			numStr := mustGetFlag(cmd, "num")
			numFloat, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return fmt.Errorf("invalid --num format: %w (expected number like '8' or '7.5')", err)
			}
			quotaNum := int(numFloat * 100)

			req := map[string]any{
				"targetUserId": mustGetFlag(cmd, "target"),
				"leaveCode":    mustGetFlag(cmd, "leave-code"),
				"quotaNum":     strconv.Itoa(quotaNum),
				"reason":       mustGetFlag(cmd, "reason"),
			}

			// 可选参数：有效期日期转换为毫秒时间戳
			if startStr := mustGetFlag(cmd, "start"); startStr != "" {
				startTs, err := parseDateToTimestamp(startStr, "startTime")
				if err != nil {
					return err
				}
				req["startTime"] = startTs
			}
			if endStr := mustGetFlag(cmd, "end"); endStr != "" {
				endTs, err := parseDateToTimestamp(endStr, "endTime")
				if err != nil {
					return err
				}
				req["endTime"] = endTs
			}

			return callMCPToolOnServer("attendance-wukong", "update_leave_balance", map[string]any{
				"McpUpdateLeaveBalanceRequest": req,
			})
		},
	}

	// record
	attendanceRecordGetCmd.Flags().String("user", "", "钉钉用户 ID (必填)")
	attendanceRecordGetCmd.Flags().String("date", "", "查询日期，格式 YYYY-MM-DD (必填)")
	attendanceRecordCmd.AddCommand(attendanceRecordGetCmd)

	// check result (query_check_result)
	attendanceCheckResultCmd.Flags().String("users", "", "用户 ID 列表，逗号分隔，最多 100 个 (必填)")
	attendanceCheckResultCmd.Flags().String("start", "", "起始日期，格式 YYYY-MM-DD (必填)")
	attendanceCheckResultCmd.Flags().String("end", "", "结束日期，格式 YYYY-MM-DD，不超过 1 个月 (必填)")
	attendanceCheckResultCmd.Flags().String("from", "", "起始日期（--start 的别名）")
	attendanceCheckResultCmd.Flags().String("to", "", "结束日期（--end 的别名）")
	_ = attendanceCheckResultCmd.Flags().MarkHidden("from")
	_ = attendanceCheckResultCmd.Flags().MarkHidden("to")
	attendanceCheckResultCmd.Flags().Int("offset", 0, "分页偏移量，默认 0（可选）")
	attendanceCheckResultCmd.Flags().Int("limit", 100, "分页大小，默认 100，范围 1-1000（可选）")
	attendanceCheckCmd.AddCommand(attendanceCheckResultCmd)

	// check record (query_check_record)
	attendanceCheckRecordCmd.Flags().String("users", "", "用户 ID 列表，逗号分隔 (必填)")
	attendanceCheckRecordCmd.Flags().String("start", "", "起始日期，格式 YYYY-MM-DD (必填)")
	attendanceCheckRecordCmd.Flags().String("end", "", "结束日期，格式 YYYY-MM-DD，不超过 1 个月 (必填)")
	attendanceCheckRecordCmd.Flags().String("from", "", "起始日期（--start 的别名）")
	attendanceCheckRecordCmd.Flags().String("to", "", "结束日期（--end 的别名）")
	_ = attendanceCheckRecordCmd.Flags().MarkHidden("from")
	_ = attendanceCheckRecordCmd.Flags().MarkHidden("to")
	attendanceCheckCmd.AddCommand(attendanceCheckRecordCmd)

	// approve list (query_user_approve)
	attendanceApproveListCmd.Flags().String("users", "", "用户 ID 列表，逗号分隔 (必填)")
	attendanceApproveListCmd.Flags().String("types", "", "审批类型，逗号分隔：overtime/trip/leave/patch (必填)")
	attendanceApproveListCmd.Flags().String("start", "", "起始日期，格式 YYYY-MM-DD (必填)")
	attendanceApproveListCmd.Flags().String("end", "", "结束日期，格式 YYYY-MM-DD (必填)")
	attendanceApproveListCmd.Flags().String("from", "", "起始日期（--start 的别名）")
	attendanceApproveListCmd.Flags().String("to", "", "结束日期（--end 的别名）")
	_ = attendanceApproveListCmd.Flags().MarkHidden("from")
	_ = attendanceApproveListCmd.Flags().MarkHidden("to")
	attendanceApproveCmd.AddCommand(attendanceApproveListCmd)

	// approve templates (query_at_approve_template)
	attendanceApproveTemplatesCmd.Flags().String("type", "", "审批类型：repair-check/patch/补卡、leave/请假、overtime/加班，或 REPAIR_CHECK/LEAVE/OVERTIME（必填）")
	attendanceApproveCmd.AddCommand(attendanceApproveTemplatesCmd)

	// shift list (batch_get_employee_shifts)
	attendanceShiftListCmd.Flags().String("users", "", "用户 ID 列表，逗号分隔，最多 50 个 (必填)")
	attendanceShiftListCmd.Flags().String("start", "", "开始日期，格式 YYYY-MM-DD (必填)")
	attendanceShiftListCmd.Flags().String("end", "", "结束日期，格式 YYYY-MM-DD (必填)")
	attendanceShiftCmd.AddCommand(attendanceShiftListCmd)

	// ── schedule ──────────────────────────────────────────────

	attendanceScheduleCmd := &cobra.Command{
		Use:   "schedule",
		Short: "排班管理",
		Long: `排班制考勤组的排班记录导入与查询（排班 = 为员工安排具体工作日期和班次）。
子命令:
  import  导入排班记录到排班制考勤组
  get     获取指定用户的排班记录`,
		RunE: groupRunE,
	}

	// schedule import (generateTurnSchedule)
	scheduleImportCmd := &cobra.Command{
		Use:   "import",
		Short: "导入排班记录到排班制考勤组",
		Long: `为排班制考勤组导入排班记录。传入 JSON 数组格式排班数据，
每条记录必填字段：
  - userId:    用户ID（必填）
  - workDate:  排班日期，格式 yyyy-MM-dd HH:mm:ss（必填）
  - classId:   班次ID（必填）
  - isRest:    是否为排休，取值 Y/N（必填）

示例：
  dws attendance schedule import --groupId 123456 \
    --scheduleVOS '[{"userId":"user001","workDate":"2026-04-22 09:00:00","classId":123,"isRest":"N"}]' \
    --user-say-yes`,
		Example: `  dws attendance schedule import --groupId GROUP_ID --scheduleVOS JSON_STRING [--user-say-yes]`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "groupId", "group-id"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "scheduleVOS", "schedules"); err != nil {
				return err
			}
			groupID := flagOrFallback(cmd, "groupId", "group-id")
			schedulesJSON := flagOrFallback(cmd, "scheduleVOS", "schedules")

			// 解析 scheduleVOS JSON
			var scheduleVOS []map[string]any
			if err := json.Unmarshal([]byte(schedulesJSON), &scheduleVOS); err != nil {
				return fmt.Errorf("invalid --scheduleVOS JSON format: %w", err)
			}

			// 验证非空
			if len(scheduleVOS) == 0 {
				return fmt.Errorf("--scheduleVOS must contain at least one schedule entry")
			}

			// 验证每条记录的必填字段
			requiredFields := []string{"userId", "workDate", "classId", "isRest"}
			for i, sched := range scheduleVOS {
				for _, field := range requiredFields {
					if _, ok := sched[field]; !ok {
						return fmt.Errorf("schedule[%d] missing required field: %s (required: userId, workDate, classId, isRest)", i, field)
					}
				}
			}

			// 转换 workDate 格式为 yyyy-MM-dd HH:mm:ss
			for i, sched := range scheduleVOS {
				if workDate, ok := sched["workDate"]; ok {
					normalized, err := normalizeWorkDate(workDate)
					if err != nil {
						return fmt.Errorf("schedule[%d] workDate format error: %w", i, err)
					}
					sched["workDate"] = normalized
				}
			}

			// 确认提示（除非 --user-say-yes）
			yes := boolFlagOrFallback(cmd, "user-say-yes", "yes")
			if !yes {
				fmt.Printf("About to import %d schedule(s) to attendance group: %s\n", len(scheduleVOS), groupID)
				fmt.Print("Confirm import? (yes/no): ")
				var answer string
				if _, err := fmt.Scanln(&answer); err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "yes" && answer != "y" {
					return fmt.Errorf("import cancelled")
				}
			}

			// 转换 groupId 为 int64
			groupIDInt, err := strconv.ParseInt(groupID, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid --groupId format, must be a number: %w", err)
			}

			return callMCPToolOnServer("attendance-wukong", "generateTurnSchedule", map[string]any{
				"GenerateTurnScheduleRequest": map[string]any{
					"groupId":     groupIDInt,
					"scheduleVOS": scheduleVOS,
					"param": map[string]any{
						"useHistoryGroupAndShift": false,
					},
				},
			})
		},
	}

	scheduleImportCmd.Flags().String("groupId", "", "考勤组ID（必填）")
	scheduleImportCmd.Flags().String("group-id", "", "考勤组ID（--groupId 的别名）")
	scheduleImportCmd.Flags().String("scheduleVOS", "", "排班记录 JSON 数组（必填）")
	scheduleImportCmd.Flags().String("schedules", "", "排班记录 JSON 数组（--scheduleVOS 的别名）")
	_ = scheduleImportCmd.Flags().MarkHidden("group-id")
	_ = scheduleImportCmd.Flags().MarkHidden("schedules")
	scheduleImportCmd.Flags().Bool("user-say-yes", false, "用户已确认，跳过交互式确认提示（Agent 调用时传 true 前必须完成用户二次确认）")
	scheduleImportCmd.Flags().Bool("yes", false, "跳过确认提示（--user-say-yes 的隐藏别名）")
	_ = scheduleImportCmd.Flags().MarkHidden("yes")

	attendanceScheduleCmd.AddCommand(scheduleImportCmd)

	// schedule get (getScheduleByRange)
	scheduleGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取指定用户的排班记录",
		Long: `获取指定用户在一段时间内的排班记录。
返回每条记录包含 userId、classId、workDate、className、checkBeginTime、checkEndTime 等字段。

日期格式支持：
  - YYYY-MM-DD（如 2026-04-01）
  - YYYY-MM-DD HH:mm:ss（如 2026-04-01 09:00:00）

示例：
  dws attendance schedule get --users user001,user002 --start 2026-04-01 --end 2026-04-30`,
		Example: `  dws attendance schedule get --users USER_IDS --start BEGIN_DATE --end END_DATE`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "users", "userIdList"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "start", "workDateBegin"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "end", "workDateEnd"); err != nil {
				return err
			}

			// 解析用户ID列表（优先 --users，兼容 --userIdList）
			userIds := parseUserList(flagOrFallback(cmd, "users", "userIdList"))

			// 解析日期参数并转换为时间戳（毫秒）
			beginStr := flagOrFallback(cmd, "start", "workDateBegin")
			beginTs, err := parseDateToTimestamp(beginStr, "start")
			if err != nil {
				return err
			}

			endStr := flagOrFallback(cmd, "end", "workDateEnd")
			endTs, err := parseDateToTimestamp(endStr, "end")
			if err != nil {
				return err
			}

			return callMCPToolOnServer("attendance-wukong", "getScheduleByRange", map[string]any{
				"GetScheduleByRangeRequest": map[string]any{
					"userIdList":    userIds,
					"workDateBegin": beginTs,
					"workDateEnd":   endTs,
				},
			})
		},
	}

	scheduleGetCmd.Flags().String("users", "", "用户ID列表，逗号分隔（必填）")
	scheduleGetCmd.Flags().String("userIdList", "", "用户ID列表（--users 的别名）")
	scheduleGetCmd.Flags().String("start", "", "开始日期，格式 YYYY-MM-DD 或 YYYY-MM-DD HH:mm:ss（必填）")
	scheduleGetCmd.Flags().String("workDateBegin", "", "开始日期（--start 的别名）")
	scheduleGetCmd.Flags().String("end", "", "结束日期，格式 YYYY-MM-DD 或 YYYY-MM-DD HH:mm:ss（必填）")
	scheduleGetCmd.Flags().String("workDateEnd", "", "结束日期（--end 的别名）")
	// 隐藏别名 flag，不在 --help 中显示
	_ = scheduleGetCmd.Flags().MarkHidden("userIdList")
	_ = scheduleGetCmd.Flags().MarkHidden("workDateBegin")
	_ = scheduleGetCmd.Flags().MarkHidden("workDateEnd")

	attendanceScheduleCmd.AddCommand(scheduleGetCmd)

	// class search (get_class_list)
	attendanceClassSearchCmd.Flags().Int("page", 1, "页码，从 1 开始（可选）")
	attendanceClassSearchCmd.Flags().Int("page-index", 0, "页码（--page 的别名）")
	attendanceClassSearchCmd.Flags().Int("limit", 20, "每页条数，最大 200（可选）")
	attendanceClassSearchCmd.Flags().Int("page-size", 0, "每页条数（--limit 的别名）")
	attendanceClassSearchCmd.Flags().Int("size", 0, "每页条数（--limit 的别名）")
	_ = attendanceClassSearchCmd.Flags().MarkHidden("page-index")
	_ = attendanceClassSearchCmd.Flags().MarkHidden("page-size")
	_ = attendanceClassSearchCmd.Flags().MarkHidden("size")
	attendanceClassSearchCmd.Flags().String("query", "", "班次名称关键字，模糊搜索（可选）")
	attendanceClassSearchCmd.Flags().String("name", "", "班次名称关键字（--query 的别名）")
	_ = attendanceClassSearchCmd.Flags().MarkHidden("name")
	attendanceClassSearchCmd.Flags().String("filter-type", "", "班次类型：ALL 全部班次 / MINE_OWN 我负责的（可选）")
	attendanceClassCmd.AddCommand(attendanceClassSearchCmd)

	// class get (get_class_detail)
	attendanceClassGetCmd.Flags().Int64("class-id", 0, "班次 ID（必填）")
	attendanceClassCmd.AddCommand(attendanceClassGetCmd)

	// class create (create_class_setting)
	attendanceClassCreateCmd.Flags().String("name", "", "班次名称（必填）")
	attendanceClassCreateCmd.Flags().String("owner", "", "班次负责人 userId（可选）")
	attendanceClassCreateCmd.Flags().String("class-vo", "", "完整 TopAtClassVO JSON 字符串，包含 sections 等复杂子对象（必填）")
	attendanceClassCreateCmd.Flags().Bool("yes", false, "跳过确认提示")
	attendanceClassCmd.AddCommand(attendanceClassCreateCmd)

	// class update (update_class_setting)
	attendanceClassUpdateCmd.Flags().Int64("class-id", 0, "班次 ID（必填）")
	attendanceClassUpdateCmd.Flags().Int64("id", 0, "班次 ID（--class-id 的别名）")
	_ = attendanceClassUpdateCmd.Flags().MarkHidden("id")
	attendanceClassUpdateCmd.Flags().String("name", "", "班次名称（可选，不传则保持原值）")
	attendanceClassUpdateCmd.Flags().String("owner", "", "班次负责人 userId（可选，不传则保持原值）")
	attendanceClassUpdateCmd.Flags().String("class-vo", "", "完整 TopAtClassVO JSON 字符串，包含 sections 等复杂子对象（可选）")
	attendanceClassUpdateCmd.Flags().Bool("yes", false, "跳过确认提示")
	attendanceClassCmd.AddCommand(attendanceClassUpdateCmd)

	// adjustment get (get_adjustment_rule_detail)
	attendanceAdjustmentGetCmd.Flags().Int64("adjustment-id", 0, "补卡规则主键 ID（必填）")
	attendanceAdjustmentCmd.AddCommand(attendanceAdjustmentGetCmd)

	// adjustment-rule search (get_adjustment_rule)
	attendanceAdjustmentSearchCmd.Flags().String("query", "", "补卡规则名称关键字，模糊搜索（可选）")
	attendanceAdjustmentSearchCmd.Flags().String("name", "", "补卡规则名称关键字（--query 的别名）")
	attendanceAdjustmentSearchCmd.Flags().Int("page", 1, "页码，从 1 开始（默认 1，可选）")
	attendanceAdjustmentSearchCmd.Flags().Int("current-page", 0, "页码（--page 的别名）")
	attendanceAdjustmentSearchCmd.Flags().Int("limit", 20, "每页条数，200 以内（默认 20，可选）")
	attendanceAdjustmentSearchCmd.Flags().Int("size", 0, "每页条数（--limit 的别名）")
	_ = attendanceAdjustmentSearchCmd.Flags().MarkHidden("name")
	_ = attendanceAdjustmentSearchCmd.Flags().MarkHidden("current-page")
	_ = attendanceAdjustmentSearchCmd.Flags().MarkHidden("size")
	attendanceAdjustmentCmd.AddCommand(attendanceAdjustmentSearchCmd)

	// overtime get (get_overtime_rule_detail)
	attendanceOvertimeGetCmd.Flags().Int64("overtime-id", 0, "加班规则主键 ID（必填）")
	attendanceOvertimeCmd.AddCommand(attendanceOvertimeGetCmd)

	// overtime-rule search (get_overtime_rule)
	attendanceOvertimeSearchCmd.Flags().String("query", "", "加班规则名称关键字，模糊搜索（可选）")
	attendanceOvertimeSearchCmd.Flags().String("name", "", "加班规则名称关键字（--query 的别名）")
	attendanceOvertimeSearchCmd.Flags().Int("page", 1, "页码，从 1 开始（默认 1，可选）")
	attendanceOvertimeSearchCmd.Flags().Int("current-page", 0, "页码（--page 的别名）")
	attendanceOvertimeSearchCmd.Flags().Int("limit", 20, "每页条数，200 以内（默认 20，可选）")
	attendanceOvertimeSearchCmd.Flags().Int("size", 0, "每页条数（--limit 的别名）")
	_ = attendanceOvertimeSearchCmd.Flags().MarkHidden("name")
	_ = attendanceOvertimeSearchCmd.Flags().MarkHidden("current-page")
	_ = attendanceOvertimeSearchCmd.Flags().MarkHidden("size")
	attendanceOvertimeCmd.AddCommand(attendanceOvertimeSearchCmd)

	// group search (get_simple_groups)
	attendanceGroupSearchCmd.Flags().String("query", "", "考勤组名称关键字，模糊搜索（可选）")
	attendanceGroupSearchCmd.Flags().String("name", "", "考勤组名称关键字（--query 的别名）")
	attendanceGroupSearchCmd.Flags().String("type", "", "考勤组类型：FIXED 固定班制 / TURN 排班制 / NONE 自由工时（可选）")
	attendanceGroupSearchCmd.Flags().Bool("query-position", false, "是否查询地理定位和 Wifi 名称（可选）")
	attendanceGroupSearchCmd.Flags().Bool("query-ble", false, "是否查询蓝牙设备列表（可选）")
	attendanceGroupSearchCmd.Flags().Int("page", 1, "页码，从 1 开始（默认 1，可选）")
	attendanceGroupSearchCmd.Flags().Int("page-index", 0, "页码（--page 的别名）")
	attendanceGroupSearchCmd.Flags().Int("limit", 20, "每页条数，200 以内（默认 20，可选）")
	attendanceGroupSearchCmd.Flags().Int("size", 0, "每页条数（--limit 的别名）")
	_ = attendanceGroupSearchCmd.Flags().MarkHidden("name")
	_ = attendanceGroupSearchCmd.Flags().MarkHidden("page-index")
	_ = attendanceGroupSearchCmd.Flags().MarkHidden("size")
	attendanceGroupCmd.AddCommand(attendanceGroupSearchCmd)

	// group get (get_group_detail)
	attendanceGroupGetCmd.Flags().Int64("group-id", 0, "考勤组 ID（必填）")
	attendanceGroupGetCmd.Flags().Int64("id", 0, "考勤组 ID（--group-id 的别名）")
	_ = attendanceGroupGetCmd.Flags().MarkHidden("id")
	attendanceGroupCmd.AddCommand(attendanceGroupGetCmd)

	// group filtered-get (get_group_filtered_detail)
	attendanceGroupFilteredGetCmd.Flags().Int64("group-id", 0, "考勤组 ID（必填）")
	attendanceGroupFilteredGetCmd.Flags().Int64("id", 0, "考勤组 ID（--group-id 的别名）")
	_ = attendanceGroupFilteredGetCmd.Flags().MarkHidden("id")
	attendanceGroupFilteredGetCmd.Flags().Bool("member", false, "是否查询考勤组成员信息（可选）")
	attendanceGroupFilteredGetCmd.Flags().Bool("position", false, "是否查询打卡地址（可选）")
	attendanceGroupFilteredGetCmd.Flags().Bool("wifi", false, "是否查询打卡 Wifi（可选）")
	attendanceGroupFilteredGetCmd.Flags().Bool("bles", false, "是否查询打卡蓝牙（可选）")
	attendanceGroupCmd.AddCommand(attendanceGroupFilteredGetCmd)

	// group update-members (update_group_member)
	attendanceGroupUpdateMembersCmd.Flags().Int64("group-id", 0, "考勤组 ID（必填）")
	attendanceGroupUpdateMembersCmd.Flags().String("add-users", "", "添加考勤人员 userId 列表，逗号分隔，最多 20 个（可选）")
	attendanceGroupUpdateMembersCmd.Flags().String("remove-users", "", "删除考勤人员 userId 列表，逗号分隔，最多 20 个（可选）")
	attendanceGroupUpdateMembersCmd.Flags().String("add-extra-users", "", "添加无需考勤的人员 userId 列表，逗号分隔，最多 20 个（可选）")
	attendanceGroupUpdateMembersCmd.Flags().String("remove-extra-users", "", "删除无需考勤的成员 userId 列表，逗号分隔，最多 20 个（可选）")
	attendanceGroupUpdateMembersCmd.Flags().String("add-depts", "", "添加考勤部门 ID 列表，逗号分隔，最多 20 个（可选）")
	attendanceGroupUpdateMembersCmd.Flags().String("remove-depts", "", "删除考勤部门 ID 列表，逗号分隔，最多 20 个（可选）")
	attendanceGroupUpdateMembersCmd.Flags().Bool("yes", false, "跳过确认提示")
	attendanceGroupCmd.AddCommand(attendanceGroupUpdateMembersCmd)

	// group update (update_group)
	attendanceGroupUpdateCmd.Flags().Int64("group-id", 0, "考勤组 ID（必填）")
	attendanceGroupUpdateCmd.Flags().String("name", "", "考勤组名称（可选）")
	attendanceGroupUpdateCmd.Flags().String("type", "", "考勤组类型：FIXED 固定班制 / TURN 排班制 / NONE 自由工时（可选）")
	attendanceGroupUpdateCmd.Flags().String("owner", "", "考勤组主负责人 userId（可选）")
	attendanceGroupUpdateCmd.Flags().String("enable-outside-check", "", "是否允许外勤打卡，传 true 或 false（可选）")
	attendanceGroupUpdateCmd.Flags().String("classIds", "", "所选班次 id 列表，JSON 数组格式，如 '[123,456]'（可选）")
	attendanceGroupUpdateCmd.Flags().String("group-vo", "", "完整 groupVO JSON 字符串，用于修改复杂子对象（可选）")
	attendanceGroupUpdateCmd.Flags().Bool("yes", false, "跳过确认提示")
	attendanceGroupCmd.AddCommand(attendanceGroupUpdateCmd)

	// group create (create_group_setting)
	attendanceGroupCreateCmd.Flags().String("name", "", "考勤组名称（必填）")
	attendanceGroupCreateCmd.Flags().String("type", "", "考勤组类型：FIXED（固定班制）/ TURN（排班制）/ NONE（自由工时）（必填）")
	attendanceGroupCreateCmd.Flags().String("owner", "", "考勤组主负责人 userId（可选）")
	attendanceGroupCreateCmd.Flags().String("corp-id", "", "企业 corpId（可选，不传时由登录上下文自动补齐）")
	attendanceGroupCreateCmd.Flags().String("group-vo", "", "完整 groupVO JSON 字符串（可选，用于传入复杂子对象，会与 --name/--type/--owner 合并）")
	attendanceGroupCreateCmd.Flags().Bool("yes", false, "跳过确认提示")
	attendanceGroupCmd.AddCommand(attendanceGroupCreateCmd)

	// summary (get_attendance_summary)
	attendanceSummaryCmd.Flags().String("user", "", "钉钉用户 ID（必填）")
	attendanceSummaryCmd.Flags().String("date", "", "查询日期，格式 YYYY-MM-DD 或 YYYY-MM-DD HH:mm:ss（必填）")
	attendanceSummaryCmd.Flags().String("stats-type", "", "统计类型：week（周统计）/ month（月统计）（必填）")
	attendanceSummaryCmd.Flags().String("tag-name", "", "旧版兼容参数（已废弃，不参与考勤摘要查询）")
	_ = attendanceSummaryCmd.Flags().MarkDeprecated("tag-name", "--tag-name 不再参与 attendance summary 查询，请移除该参数")
	_ = attendanceSummaryCmd.Flags().MarkHidden("tag-name")

	// rules (query_attendance_group_or_rules)
	attendanceRulesCmd.Flags().String("date", "", "考勤日期，格式 YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss (必填)")

	// selfsetting get (query_self_setting)
	attendanceSelfSettingGetCmd.Flags().String("setting-scene", "", "查询设置项：checkRemind/fastCheck/checkResultNotify/lackRemind/personalAttendStatNotify/bossAttendStatNotify（必填）")
	attendanceSelfSettingGetCmd.Flags().String("user", "", "查询用户 ID（必填）")

	// selfsetting save (save_self_setting)
	attendanceSelfSettingSaveCmd.Flags().String("setting-scene", "", "更新设置项：checkRemind/fastCheck/checkResultNotify/lackRemind/personalAttendStatNotify/bossAttendStatNotify（必填）")
	attendanceSelfSettingSaveCmd.Flags().String("user", "", "更新用户 ID（必填）")
	attendanceSelfSettingSaveCmd.Flags().Bool("yes", false, "跳过确认提示")
	registerSelfSettingSaveFlags(attendanceSelfSettingSaveCmd)
	attendanceSelfSettingCmd.AddCommand(attendanceSelfSettingGetCmd, attendanceSelfSettingSaveCmd)

	// globalsetting get (query_global_setting)
	attendanceGlobalSettingGetCmd.Flags().String("setting-scene", "", "查询设置项：checkRemind/fastCheck/checkResultNotify/lackRemind/personalAttendStatNotify/bossAttendStatNotify（必填）")
	attendanceGlobalSettingGetCmd.Flags().String("scope", "", "全局范围确认，必须明确输入：企业/全公司/所有人（必填）")

	// globalsetting save (save_global_setting)
	attendanceGlobalSettingSaveCmd.Flags().String("setting-scene", "", "更新设置项：checkRemind/fastCheck/checkResultNotify/lackRemind/personalAttendStatNotify/bossAttendStatNotify（必填）")
	attendanceGlobalSettingSaveCmd.Flags().String("scope", "", "全局范围确认，必须明确输入：企业/全公司/所有人（必填）")
	attendanceGlobalSettingSaveCmd.Flags().Bool("yes", false, "跳过确认提示")
	registerGlobalSettingSaveFlags(attendanceGlobalSettingSaveCmd)
	attendanceGlobalSettingCmd.AddCommand(attendanceGlobalSettingGetCmd, attendanceGlobalSettingSaveCmd)

	// report query-data (query_data_for_mcp)
	reportQueryDataCmd.Flags().String("users", "", "目标用户 userID 列表，逗号分隔，最多 20 人（必填）")
	reportQueryDataCmd.Flags().String("columns", "", "字段 ID 列表，逗号分隔，可通过 report columns 获取（必填）")
	reportQueryDataCmd.Flags().String("start", "", "开始日期，格式 yyyy-MM-dd HH:mm:ss（必填）")
	reportQueryDataCmd.Flags().String("end", "", "结束日期，格式 yyyy-MM-dd HH:mm:ss（必填）")

	// report query-leave (query_leave_data_for_mcp)
	reportQueryLeaveCmd.Flags().String("users", "", "目标用户 userID 列表，逗号分隔，最多 20 人（必填）")
	reportQueryLeaveCmd.Flags().String("leave-names", "", "假期类型名称列表，逗号分隔，不填则查询所有假期类型（选填）")
	reportQueryLeaveCmd.Flags().String("start", "", "开始日期，格式 yyyy-MM-dd HH:mm:ss（必填）")
	reportQueryLeaveCmd.Flags().String("end", "", "结束日期，格式 yyyy-MM-dd HH:mm:ss（必填）")

	attendanceReportCmd.AddCommand(
		reportColumnsCmd,
		reportQueryDataCmd,
		reportQueryLeaveCmd,
	)

	// vacation balance (get_leave_balance_quota)
	vacationBalanceCmd.Flags().String("users", "", "目标员工 ID 列表，逗号分隔 (必填)")
	vacationBalanceCmd.Flags().String("leave-code", "", "假期规则 code (必填，服务端要求非空，不传返回 INVALID_PARAMS)")

	// vacation records (get_leave_balance_records)
	vacationRecordsCmd.Flags().String("user", "", "指定查询员工 ID (必填)")
	vacationRecordsCmd.Flags().String("leave-code", "", "假期规则 code (必填，服务端要求非空，不传返回 INVALID_PARAMS)")
	vacationRecordsCmd.Flags().String("start", "", "查询开始日期，格式 YYYY-MM-DD (必填)")
	vacationRecordsCmd.Flags().String("end", "", "查询结束日期，格式 YYYY-MM-DD (必填)")

	// vacation update-type (save_leave_type)
	vacationUpdateTypeCmd.Flags().String("leave-code", "", "假期编码（必填）")
	vacationUpdateTypeCmd.Flags().String("name", "", "假期名称（可选）")
	vacationUpdateTypeCmd.Flags().String("unit", "", "假期单位：day/halfDay/hour（可选）")
	vacationUpdateTypeCmd.Flags().Bool("paid", false, "是否带薪假期（可选）")
	vacationUpdateTypeCmd.Flags().Int("per-hours", 0, "一天折算小时数（可选）")
	vacationUpdateTypeCmd.Flags().String("when-can-leave", "", "新员工请假规则：entry/formal（可选）")
	vacationUpdateTypeCmd.Flags().String("visibility-rules", "", "适用范围规则 JSON 数组（可选）。不传=不改；[{\"type\":\"dept\",\"visible\":[\"-1\"]}]=全公司可见（哨兵）；其余=改为指定范围。空数组/无效规则会报错")
	vacationUpdateTypeCmd.Flags().Bool("user-say-yes", false, "用户已确认，跳过交互式确认提示（Agent 调用时传 true 前必须完成用户二次确认）")
	vacationUpdateTypeCmd.Flags().Bool("yes", false, "跳过确认提示（--user-say-yes 的隐藏别名）")
	_ = vacationUpdateTypeCmd.Flags().MarkHidden("yes")

	// vacation save-balance (update_leave_balance)
	vacationSaveBalanceCmd.Flags().String("target", "", "目标员工工号（必填）")
	vacationSaveBalanceCmd.Flags().String("leave-code", "", "假期编码（必填）")
	vacationSaveBalanceCmd.Flags().String("num", "", "余额数量（必填）")
	vacationSaveBalanceCmd.Flags().String("reason", "", "变更原因（必填）")
	vacationSaveBalanceCmd.Flags().String("start", "", "有效期开始日期 YYYY-MM-DD")
	vacationSaveBalanceCmd.Flags().String("end", "", "有效期结束日期 YYYY-MM-DD")
	vacationSaveBalanceCmd.Flags().Bool("user-say-yes", false, "用户已确认，跳过交互式确认提示（Agent 调用时传 true 前必须完成用户二次确认）")
	vacationSaveBalanceCmd.Flags().Bool("yes", false, "跳过确认提示（--user-say-yes 的隐藏别名）")
	_ = vacationSaveBalanceCmd.Flags().MarkHidden("yes")

	attendanceVacationCmd.AddCommand(
		vacationTypesCmd,
		vacationUpdateTypeCmd,
		vacationBalanceCmd,
		vacationSaveBalanceCmd,
		vacationRecordsCmd,
	)

	// ── checkin ──────────────────────────────────────────────

	checkinCmd := &cobra.Command{
		Use:   "checkin",
		Short: "签到管理",
		Long: `签到记录的查询。
子命令:
  records  查询指定员工的签到记录`,
		RunE: groupRunE,
	}

	// MCP tool: queryUserRecordByStaffIds
	checkinRecordsCmd := &cobra.Command{
		Use:   "records",
		Short: "查询指定员工的签到记录",
		Long: `查询指定员工在一段时间内的签到记录。
权限说明：
  - Boss / 超级管理员 / 超级子管理员：可查看全公司员工
  - 子管理员：可查看管理范围内的员工及自己
  - 部门主管：可查看所管理部门的员工及自己
  - 普通员工：只能查询自己

日期格式：yyyy-MM-dd HH:mm:ss（如 2026-04-01 09:00:00）`,
		Example: `  dws attendance checkin records --operator-corp-id dingXXXXXX --operator-staff-id op001 --staff-ids user001,user002 --start "2026-04-01 00:00:00" --end "2026-04-07 00:00:00"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "operator-corp-id", "operator-staff-id", "staff-ids", "start", "end"); err != nil {
				return err
			}

			operatorCorpId := mustGetFlag(cmd, "operator-corp-id")
			operatorStaffId := mustGetFlag(cmd, "operator-staff-id")

			staffIds := parseUserList(mustGetFlag(cmd, "staff-ids"))
			if len(staffIds) == 0 {
				return fmt.Errorf("--staff-ids must contain at least one staff ID")
			}

			startStr := mustGetFlag(cmd, "start")
			startT, err := time.ParseInLocation("2006-01-02 15:04:05", startStr, time.Local)
			if err != nil {
				return fmt.Errorf("invalid --start format, use yyyy-MM-dd HH:mm:ss (e.g. 2026-04-01 09:00:00): %w", err)
			}

			endStr := mustGetFlag(cmd, "end")
			endT, err := time.ParseInLocation("2006-01-02 15:04:05", endStr, time.Local)
			if err != nil {
				return fmt.Errorf("invalid --end format, use yyyy-MM-dd HH:mm:ss (e.g. 2026-04-30 23:59:59): %w", err)
			}

			if endT.Before(startT) {
				return fmt.Errorf("--end must not be earlier than --start")
			}

			return callMCPToolOnServer("attendance-wukong", "get_checkin_record", map[string]any{
				"QueryMcpUserRecordRequest": map[string]any{
					"operatorCorpId":  operatorCorpId,
					"operatorStaffId": operatorStaffId,
					"staffIds":        staffIds,
					"startTime":       startStr,
					"endTime":         endStr,
				},
			})
		},
	}

	checkinRecordsCmd.Flags().String("operator-corp-id", "", "操作者企业 ID（必填）")
	checkinRecordsCmd.Flags().String("operator-staff-id", "", "操作者员工 ID（必填）")
	checkinRecordsCmd.Flags().String("staff-ids", "", "目标员工 ID 列表, 逗号分隔（必填），最多100人")
	checkinRecordsCmd.Flags().String("start", "", "开始时间, 格式 yyyy-MM-dd HH:mm:ss（必填），开始到结束最多7天")
	checkinRecordsCmd.Flags().String("end", "", "结束时间, 格式 yyyy-MM-dd HH:mm:ss（必填），开始到结束最多7天")

	checkinCmd.AddCommand(checkinRecordsCmd)

	// ── boss-check ──────────────────────────────────────────────

	// MCP tool: boss_check
	bossCheckCmd := &cobra.Command{
		Use:   "boss-check",
		Short: "BOSS 改签打卡记录",
		Long: `调用 MCP 工具 boss_check 修改员工打卡记录。管理员可修改打卡时间、打卡结果等信息。

参数说明：
  --plan-id        排班ID（与 --result-id 二选一）
                   来源：通过 dws attendance schedule get 获取，返回结果的 id 字段
                   示例：id: 948964045503 表示某天下班打卡的排班记录ID

  --result-id      打卡结果ID（与 --plan-id 二选一，优先使用）
                   来源：暂不支持（record get 未返回此字段），建议使用 --plan-id

  --time           新打卡时间，格式 yyyy-MM-dd HH:mm（可选）
  --result         打卡结果：Normal/TimesResultA/TimesResultB/TimesResultC/TimesResultD/TimesResultE/TimesResultF（可选）
  --absent-min     缺勤时长（分钟），当 result 为异常状态时需传值（可选）
  --remark         备注，最长500字符（可选）
  --user-say-yes   用户已确认，跳过交互式确认提示

打卡结果枚举值：
  Normal         正常
  TimesResultA   迟到
  TimesResultB   早退
  TimesResultC   缺卡
  TimesResultD   迟到+早退
  TimesResultE   缺卡+早退
  TimesResultF   迟到+缺卡

获取 planId 步骤：
  1. 查询排班记录：dws attendance schedule get --userIdList USER_ID --workDateBegin DATE --workDateEnd DATE
  2. 从返回结果中找到对应打卡类型（OnDuty=上班，OffDuty=下班）的 id 字段
  3. 使用该 id 作为 --plan-id 参数`,
		Example: `  # 通过排班ID改签，指定打卡时间
  dws attendance boss-check --plan-id 123456 --time "2025-04-21 08:30"

  # 通过结果ID改签，修改打卡结果为正常
  dws attendance boss-check --result-id 789012 --result Normal

  # 修改打卡结果为迟到，并设置缺勤时长
  dws attendance boss-check --result-id 789012 --result TimesResultA --absent-min 30

  # 添加备注
  dws attendance boss-check --result-id 789012 --remark "管理员改签"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 检查 plan-id 和 result-id 至少传一个
			planID := mustGetFlag(cmd, "plan-id")
			resultID := mustGetFlag(cmd, "result-id")
			if planID == "" && resultID == "" {
				return fmt.Errorf("--plan-id 和 --result-id 至少传一个")
			}

			// 确认提示（除非 --user-say-yes）
			yes, _ := cmd.Flags().GetBool("user-say-yes")
			if !yes {
				fmt.Printf("即将 BOSS 改签：\n")
				if resultID != "" {
					fmt.Printf("  打卡结果ID：%s\n", resultID)
				} else {
					fmt.Printf("  排班ID：%s\n", planID)
				}
				if cmd.Flags().Changed("time") {
					fmt.Printf("  新打卡时间：%s\n", mustGetFlag(cmd, "time"))
				}
				if cmd.Flags().Changed("result") {
					fmt.Printf("  新打卡结果：%s\n", mustGetFlag(cmd, "result"))
				}
				if cmd.Flags().Changed("absent-min") {
					absentMin, _ := cmd.Flags().GetInt("absent-min")
					fmt.Printf("  缺勤时长：%d 分钟\n", absentMin)
				}
				if cmd.Flags().Changed("remark") {
					fmt.Printf("  备注：%s\n", mustGetFlag(cmd, "remark"))
				}
				fmt.Print("是否确认执行？: ")
				var answer string
				fmt.Scanln(&answer)
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "yes" && answer != "y" {
					return fmt.Errorf("操作取消")
				}
			}

			// 构建 MCP 调用参数
			req := map[string]any{}

			// planId 和 resultId 二选一，优先 resultId
			if resultID != "" {
				req["resultId"] = resultID
			} else if planID != "" {
				req["planId"] = planID
			}

			// 可选参数
			if cmd.Flags().Changed("time") {
				req["bossCheckTime"] = mustGetFlag(cmd, "time")
			}
			if cmd.Flags().Changed("result") {
				req["result"] = mustGetFlag(cmd, "result")
			}
			if cmd.Flags().Changed("absent-min") {
				absentMin, _ := cmd.Flags().GetInt("absent-min")
				req["absentMin"] = absentMin
			}
			if cmd.Flags().Changed("remark") {
				req["remark"] = mustGetFlag(cmd, "remark")
			}

			return callMCPToolOnServer("attendance-wukong", "boss_check", map[string]any{
				"BossCheckMcpRequest": req,
			})
		},
	}

	bossCheckCmd.Flags().String("plan-id", "", "排班ID（与 --result-id 二选一）")
	bossCheckCmd.Flags().String("result-id", "", "打卡结果ID（与 --plan-id 二选一，优先使用）")
	bossCheckCmd.Flags().String("time", "", "新打卡时间，格式 yyyy-MM-dd HH:mm（可选）")
	bossCheckCmd.Flags().String("result", "", "打卡结果：Normal/TimesResultA/TimesResultB/TimesResultC/TimesResultD/TimesResultE/TimesResultF（可选）")
	bossCheckCmd.Flags().Int("absent-min", 0, "缺勤时长（分钟）（可选）")
	bossCheckCmd.Flags().String("remark", "", "备注，最长500字符（可选）")
	bossCheckCmd.Flags().Bool("user-say-yes", false, "用户已确认，跳过交互式确认提示")

	root.AddCommand(
		attendanceRecordCmd,
		attendanceCheckCmd,
		attendanceApproveCmd,
		attendanceShiftCmd,
		attendanceScheduleCmd,
		attendanceClassCmd,
		attendanceAdjustmentCmd,
		attendanceOvertimeCmd,
		attendanceGroupCmd,
		attendanceSummaryCmd,
		attendanceRulesCmd,
		attendanceSelfSettingCmd,
		attendanceGlobalSettingCmd,
		attendanceReportCmd,
		attendanceVacationCmd,
		checkinCmd,
		bossCheckCmd,
	)
	return root
}
