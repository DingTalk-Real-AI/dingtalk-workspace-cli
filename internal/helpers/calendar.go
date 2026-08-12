package helpers

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws calendar — 日历产品命令组
// ──────────────────────────────────────────────────────────

// calendarInfoHintSubCmd builds a hidden disambiguation subcommand that prints
// a warning-level "Did you mean" hint to stderr (instead of returning an Error)
// and exits 0. Scoped to calendar.go on purpose so the shared cmdutil.HintSubCmd
// used by other products keeps returning errors as before.
//
// The `suggestion` argument should be the bare corrected command (no leading
// "use:" / "hint:" prefix); the helper wraps it with the standard "Did you
// mean:" line to keep all calendar hints consistent.
func calendarInfoHintSubCmd(use, suggestion string) *cobra.Command {
	c := hintSubCmd(use, suggestion)
	c.DisableFlagParsing = true
	c.RunE = func(cmd *cobra.Command, args []string) error {
		fmt.Fprintf(os.Stderr, "warning: command %q does not exist\n  hint: %s\t %s\n more: %s \n",
			cmd.Parent().CommandPath()+" "+use,
			suggestion,
			"[MUST] use --help to see command detail",
			"'dws calendar --help' to see all available commands")
		return nil
	}
	return c
}

// installUnknownVerbFallback makes `group` emit a consistent warning-style
// "Did you mean" hint whenever the caller types an unknown subcommand under
// that group, regardless of whether extra flags follow. This is a blanket
// safety net that covers every verb we never thought to pre-register via
// calendarInfoHintSubCmd (e.g. `dws calendar room query --min-duration 30`).
//
// Two Cobra knobs make this work together:
//   - FParseErrWhitelist.UnknownFlags=true stops pflag from aborting with
//     "unknown flag: --xxx" before RunE ever runs.
//   - Args=cobra.ArbitraryArgs lets Cobra pass the bad verb through as the
//     first positional arg instead of rejecting it.
//
// If the user types a *known* subcommand, Cobra still dispatches to that
// child's RunE as usual; this fallback only fires when resolution stops at
// `group` with leftover args.
func installUnknownVerbFallback(group *cobra.Command) {
	group.FParseErrWhitelist.UnknownFlags = true
	group.Args = cobra.ArbitraryArgs

	// Override HelpFunc so that `<group> <unknown-verb> --help` also shows
	// the "unknown subcommand" error instead of silently printing help.
	// Cobra intercepts --help before RunE, so without this the fallback
	// would never fire when --help is present.
	origHelp := group.HelpFunc()
	group.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == group {
			// HelpFunc receives os.Args[1:] (full arg slice without binary).
			// Strip tokens matching the resolved command path to get actual
			// leftover args that should be checked for unknown verbs.
			depth := len(strings.Fields(cmd.CommandPath())) - 1
			leftover := stripCommandPrefix(args, depth)
			if bad := findUnknownVerb(cmd, leftover); bad != "" {
				printUnknownSubcmdError(cmd, bad)
				return
			}
			origHelp(cmd, args)
			return
		}
		// For non-group commands, render base help then apply the safety
		// annotation. Recursion safety hinges on NOT calling
		// cmd.Root().HelpFunc(): in test trees calendar IS the root, so that
		// would re-enter this wrapper. origHelp was captured before any
		// wrapping and is the plain cobra renderer.
		origHelp(cmd, args)
		cli.RenderSafetyAnnotation(cmd)
	})

	prev := group.RunE
	group.RunE = func(cmd *cobra.Command, args []string) error {
		// Unknown flags whitelisted by pflag may leak into args. Pick the first
		// non-flag token as the offending verb.
		if bad := findUnknownVerb(cmd, args); bad != "" {
			printUnknownSubcmdError(cmd, bad)
			return nil
		}
		// No unknown verb found. Since FParseErrWhitelist.UnknownFlags silently
		// swallows bad flags, scan the original os.Args for flags unregistered
		// on this command and report them explicitly.
		if flag := findUnknownFlag(cmd); flag != "" {
			fmt.Fprintf(os.Stderr, "Error: unknown flag: %s\n", flag)
			fmt.Fprintf(os.Stderr, "  hint: Run '%s --help' to see available options\n", cmd.CommandPath())
			return nil
		}
		if prev != nil {
			return prev(cmd, args)
		}
		return cmd.Help()
	}
}

// findUnknownVerb returns the first positional arg that is not a registered
// subcommand (or alias) of cmd. Returns "" if all args are flags or known.
func findUnknownVerb(cmd *cobra.Command, args []string) string {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		isKnown := false
		for _, c := range cmd.Commands() {
			if c.Name() == a {
				isKnown = true
				break
			}
			for _, alias := range c.Aliases {
				if alias == a {
					isKnown = true
					break
				}
			}
			if isKnown {
				break
			}
		}
		if !isKnown {
			return a
		}
	}
	return ""
}

// printUnknownSubcmdError prints the standard "unknown subcommand" error to
// stderr with available commands and a did-you-mean hint.
func printUnknownSubcmdError(cmd *cobra.Command, bad string) {
	var available []string
	for _, c := range cmd.Commands() {
		if !c.Hidden && c.Name() != "help" {
			available = append(available, c.Name())
		}
	}
	fmt.Fprintf(os.Stderr, "Error: unknown subcommand %q for %q\n", bad, cmd.CommandPath())
	fmt.Fprintf(os.Stderr, "  available: %s\n", strings.Join(available, ", "))
	if s := cmd.SuggestionsFor(bad); len(s) > 0 {
		fmt.Fprintf(os.Stderr, "  hint: did you mean %q\n", cmd.CommandPath()+" "+s[0])
	} else {
		fmt.Fprintf(os.Stderr, "  hint: %s --help\n", cmd.CommandPath())
	}
}

// stripCommandPrefix strips the first `depth` non-flag tokens from args.
// This is needed because Cobra's HelpFunc receives os.Args[1:] (the full arg
// slice without the binary name), including the resolved command path tokens.
// depth should be len(strings.Fields(cmd.CommandPath())) - 1.
func stripCommandPrefix(args []string, depth int) []string {
	skipped := 0
	for i, a := range args {
		if skipped >= depth {
			return args[i:]
		}
		if !strings.HasPrefix(a, "-") {
			skipped++
		}
	}
	return nil
}

// findUnknownFlag scans os.Args for flags that are not registered on cmd.
// Returns the first offending flag token (e.g. "--today") or "".
func findUnknownFlag(cmd *cobra.Command) string {
	depth := len(strings.Fields(cmd.CommandPath())) - 1
	leftover := stripCommandPrefix(os.Args[1:], depth)
	for i := 0; i < len(leftover); i++ {
		a := leftover[i]
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "--") {
			name := a[2:]
			if eqIdx := strings.Index(name, "="); eqIdx >= 0 {
				name = name[:eqIdx]
			}
			if name == "help" {
				continue
			}
			if cmd.Flags().Lookup(name) == nil {
				return a
			}
		} else if strings.HasPrefix(a, "-") && a != "-" {
			ch := a[1:2]
			if ch == "h" {
				continue
			}
			if cmd.Flags().ShorthandLookup(ch) == nil {
				return a
			}
		}
	}
	return ""
}

func newCalendarCommand() *cobra.Command {
	// Product-level Agent routing Decl (migrated from selection/calendar.json
	// products.calendar). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "calendar",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "管理日历本、日程、参与人、附件、会议室，并查询人员或会议室闲忙状态。",
			UseWhen: []string{
				"需要查询或管理钉钉日历、日程、参与人、会议室和闲忙信息时。",
			},
			AvoidWhen: []string{
				"目标是聊天消息、待办或审批，而不是日历与日程资源时。",
			},
		},
	})
	root := &cobra.Command{
		Use:   "calendar",
		Short: "日历日程 / 会议室 / 闲忙",
		Long: `管理钉钉日历：日程、参会人、会议室、闲忙、附件、日历本、访问权限。调用前必须先使用 --help 查看参数结构。

命令结构:
  dws calendar event       [list|get|create|update|delete|suggest|respond|instances]  日程管理
  dws calendar attendee    [list|add|delete]                 参会人管理
  dws calendar room        [search|add|delete|list-groups]   会议室管理
  dws calendar busy        search                           闲忙查询 (可查人、查会议室)
  dws calendar attachment  add                              日程附件管理
  dws calendar book        [list|get|search|update]          日历本管理
  dws calendar acl         [list|add|delete]                 日历访问权限管理`,
		RunE: groupRunE,
	}

	// ── event: 日程 ─────────────────────────────────────────────

	eventCmd := &cobra.Command{Use: "event", Short: "日程管理", RunE: groupRunE}

	eventListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询日程列表",
		Long: `查询当前用户可访问的日程列表。**注意** 不传 --start/--end 时将查询今天的日程（00:00:00 ~ 23:59:59）。
		**calendar_id**: 不指定calendar_id时，默认查询当前用户主日历下的日程。也可指定calendar_id查询他人共享或者已订阅的日历。calendar_id可通过 dws calendar book list 查询。
		**权限**：查询共享日历下的日程时，至少要有reader权限。
		`,
		Example: `  dws calendar event list
  dws calendar event list --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T18:00:00+08:00"
  dws calendar event list --start "2026-03-10T00:00:00+08:00" --end "2026-03-31T23:59:59+08:00" --limit 50
  dws calendar event list --calendar-id primary
  dws calendar event list --cursor "<nextCursor从上一次查询结果获取>"
  dws calendar event list --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			var startTime, endTime int64
			var now time.Time
			if v := flagOrFallback(cmd, "start", "time-min", "min-time", "start-time", "startTime", "start_time", "start-date", "startDate"); v != "" {
				var err error
				startTime, err = parseISOTimeToMillis("start", v)
				if err != nil {
					return err
				}
				toolArgs["startTime"] = startTime
			} else {
				now = time.Now()
				startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UnixMilli()
				toolArgs["startTime"] = startTime
			}
			if v := flagOrFallback(cmd, "end", "time-max", "max-time", "end-time", "endTime", "end_time", "end-date", "endDate"); v != "" {
				var err error
				endTime, err = parseISOTimeToMillis("end", v)
				if err != nil {
					return err
				}
				toolArgs["endTime"] = endTime
			} else {
				if now.IsZero() {
					now = time.Now()
				}
				endTime = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location()).UnixMilli()
				toolArgs["endTime"] = endTime
			}
			if err := validateTimeRange(startTime, endTime); err != nil {
				return err
			}
			if v := flagOrFallback(cmd, "calendar-id", "calendarId", "calendar"); v != "" {
				toolArgs["calendarId"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "next-cursor", "nextCursor", "page-token", "pageToken", "next-token"); v != "" {
				toolArgs["cursor"] = v
			}
			if lim, _ := cmd.Flags().GetInt("limit"); lim > 0 {
				toolArgs["limit"] = lim
			} else if lim, _ := cmd.Flags().GetInt("max-results"); lim > 0 {
				toolArgs["limit"] = lim
			} else if lim, _ := cmd.Flags().GetInt("maxResults"); lim > 0 {
				toolArgs["limit"] = lim
			} else if lim, _ := cmd.Flags().GetInt("page-size"); lim > 0 {
				toolArgs["limit"] = lim
			} else if lim, _ := cmd.Flags().GetInt("size"); lim > 0 {
				toolArgs["limit"] = lim
			} else if lim, _ := cmd.Flags().GetInt("count"); lim > 0 {
				toolArgs["limit"] = lim
			}
			return callSortedCalendarEvents(cmd, "list_calendar_events", toolArgs)
		},
	}
	DeclareLeafMetadata(eventListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "list_calendar_events",
				CanonicalPath:  "calendar.list_calendar_events",
				CLIPath:        "calendar event list",
				PrimaryCLIPath: "calendar event list",
			},
			Description: "查询当前用户日程列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "list_calendar_events"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询当前用户日程列表",
				UseWhen:      []string{"需要按起止时间检索指定日历本下的日程列表时（未传起止默认查当天）"},
				AvoidWhen: []string{
					"已知 eventId 要详情时改用 dws calendar event get",
					"要创建日程时改用 dws calendar event create",
				},
				Examples: []string{
					"dws calendar event list --start \"2026-03-10T14:00:00+08:00\" --end \"2026-03-10T18:00:00+08:00\"",
					"dws calendar event list --calendar-id primary",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "end", Property: "endTime"},
				{Name: "start", Property: "startTime"},
			},
		},
	})

	eventGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取日程详情",
		Example: `  dws calendar event get --id EVENT_ID  # 查询 eventId: dws calendar event list
  dws calendar event get --id EVENT_ID --calendar-id primary`,
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID, err := mustFlagOrFallback(cmd, "id", "event", "event-id", "eventId")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"eventId": eventID}
			if v := flagOrFallback(cmd, "calendar-id", "calendarId", "calendar"); v != "" {
				toolArgs["calendarId"] = v
			}
			return callMCPTool("get_calendar_detail", toolArgs)
		},
	}
	DeclareLeafMetadata(eventGetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "get_calendar_detail",
				CanonicalPath:  "calendar.get_calendar_detail",
				CLIPath:        "calendar event get",
				PrimaryCLIPath: "calendar event get",
			},
			Description: "查询日程详情",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "get_calendar_detail"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询日程详情",
				UseWhen:      []string{"已知 eventId，需要查看日程明细时"},
				AvoidWhen: []string{
					"要按时间查列表时改用 dws calendar event list",
					"要创建/删除时改用 create/delete",
				},
				Examples: []string{
					"dws calendar event get --id <EVENT_ID>",
					"dws calendar event get --id <EVENT_ID> --calendar-id primary",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "eventId"},
			},
		},
	})

	eventCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建日程",
		Long: `创建日程。

周期日程说明：--recurrence-* 不是彼此独立的参数。一旦指定任一 --recurrence-* 标志，就必须一次性提供**完整**的循环规则，
至少包含 --recurrence-type、--recurrence-interval(>0) 与 --recurrence-range-type，否则命令会被拒绝执行。`,
		Example: `  dws calendar event create --title "Q1 复盘会" \
    --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00"
  dws calendar event create --title "周会" \
    --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00" \
    --attendees userId1,userId2
  dws calendar event create --title "项目评审" \
    --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00" \
    --rooms roomId1,roomId2  # roomId 必须来自 dws calendar room search 返回
  dws calendar event create --title "每日站会" \
    --start "2026-03-10T09:00:00+08:00" --end "2026-03-10T09:30:00+08:00" \
    --recurrence-type daily --recurrence-interval 1 --recurrence-range-type numbered --recurrence-count 10
  dws calendar event create --title "团队周会" \
    --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00" \
    --calendar-id <SHARED_CALENDAR_ID>  # 在指定日历本（如共享日历）下创建日程`,
		RunE: func(cmd *cobra.Command, args []string) error {
			title, err := mustFlagOrFallback(cmd, "title", "summary")
			if err != nil {
				return err
			}
			startStr, err := mustFlagOrFallback(cmd, "start", "start-time", "startTime", "start_time", "start-date", "startDate")
			if err != nil {
				return err
			}
			endStr, err := mustFlagOrFallback(cmd, "end", "end-time", "endTime", "end_time", "end-date", "endDate")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"summary":       title,
				"startDateTime": startStr,
				"endDateTime":   endStr,
			}
			if v, _ := cmd.Flags().GetString("timezone"); v != "" {
				toolArgs["timeZone"] = v
			}
			if v := flagOrFallback(cmd, "desc", "description"); v != "" {
				toolArgs["description"] = v
			}
			if v := flagOrFallback(cmd, "attendees", "users", "user-ids", "userIds", "user", "attendee", "attendee-ids", "attendeeIds", "participants", "participant-ids", "participantIds", "members"); v != "" {
				toolArgs["attendees"] = parseCSVValues(v)
			}
			if v, _ := cmd.Flags().GetString("open-dingtalk-ids"); v != "" {
				toolArgs["openDingTalkIds"] = parseCSVValues(v)
			}
			if v := flagOrFallback(cmd, "rooms", "room-ids", "roomIds"); v != "" {
				toolArgs["roomIds"] = parseCSVValues(v)
			}
			recurrence, err := buildRecurrence(cmd)
			if err != nil {
				return err
			}
			if recurrence != nil {
				toolArgs["recurrence"] = recurrence
			}
			if v, _ := cmd.Flags().GetString("rich-text-desc"); v != "" {
				toolArgs["richTextDescription"] = v
			}
			if v, _ := cmd.Flags().GetString("location"); v != "" {
				toolArgs["location"] = v
			}
			if v := flagOrFallback(cmd, "free-busy", "freeBusy", "freebusy"); v != "" {
				toolArgs["freeBusy"] = v
			}
			if v := flagOrFallback(cmd, "calendar-id", "calendarId", "calendar"); v != "" {
				toolArgs["calendarId"] = v
			}
			if v, _ := cmd.Flags().GetString("remind-minutes"); v != "" {
				toolArgs["reminders"] = buildReminders(v)
			}
			return callMCPTool("create_calendar_event", toolArgs)
		},
	}
	DeclareLeafMetadata(eventCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "create_calendar_event",
				CanonicalPath:  "calendar.create_calendar_event",
				CLIPath:        "calendar event create",
				PrimaryCLIPath: "calendar event create",
			},
			Description: "创建日程",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "create_calendar_event"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建日程",
				UseWhen:      []string{"需要创建新日程（时间、可选参会人/提醒/会议室/循环规则）并提取 eventId 时"},
				AvoidWhen: []string{
					"只需查询日程列表或详情时改用 event list/get",
					"只改已有日程字段时改用 event update；改参会人/会议室用 attendee/room 命令",
				},
				Examples: []string{
					"dws calendar event create --title \"Q1 复盘会\" --start \"2026-03-10T14:00:00+08:00\" --end \"2026-03-10T15:00:00+08:00\"",
					"dws calendar event create --title \"周会\" --start \"2026-03-10T14:00:00+08:00\" --end \"2026-03-10T15:00:00+08:00\" --attendees userId1,userId2",
				},
			},
			Parameters: append([]contract.ParamDecl{
				{Name: "title", Property: "summary", Required: boolPtr(true)},
				{Name: "start", Property: "startDateTime", Required: boolPtr(true)},
				{Name: "end", Property: "endDateTime", Required: boolPtr(true)},
				{Name: "desc", Property: "description"},
				{Name: "open-dingtalk-ids", Property: "openDingTalkIds"},
				{Name: "remind-minutes", Property: "reminders"},
				{Name: "rich-text-desc", Property: "richTextDescription"},
				{Name: "rooms", Property: "roomIds"},
				{Name: "timezone", Property: "timeZone"},
			}, calendarRecurrenceParamDecls()...),
		},
	})

	eventUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "修改日程",
		Long: `支持修改标题、描述、时间、地点、忙碌状态等。如需修改会议室，请使用 dws calendar room [add|delete]；如需修改参会人，请使用 dws calendar attendee [add|delete]。

		修改周期日程的循环规则时，--recurrence-* 系列必须整体传入：只传其中一个（比如只改 --recurrence-count）会把循环规则覆盖成不完整的状态。
		至少包含 --recurrence-type、--recurrence-interval(>0) 与 --recurrence-range-type，否则命令会被拒绝执行。
		如果只想微调已有周期日程的某一个循环字段，请先通过 dws calendar event get --id <ID> 读取现有 recurrence，然后在命令中重新提供完整的 pattern + range 字段集合。`,
		Example: `  dws calendar event update --id EVENT_ID --title "新标题"  # 查询 eventId: dws calendar event list
  dws calendar event update --id EVENT_ID --desc "新描述" --timezone Asia/Tokyo
  dws calendar event update --id EVENT_ID --recurrence-type daily --recurrence-interval 1 --recurrence-range-type numbered --recurrence-count 5`,
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID, err := mustFlagOrFallback(cmd, "id", "event", "event-id", "eventId")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"eventId": eventID}
			if v := flagOrFallback(cmd, "title", "summary"); v != "" {
				toolArgs["summary"] = v
			}
			if v := flagOrFallback(cmd, "start", "start-time", "startTime", "start_time", "start-date", "startDate"); v != "" {
				toolArgs["startDateTime"] = v
			}
			if v := flagOrFallback(cmd, "end", "end-time", "endTime", "end_time", "end-date", "endDate"); v != "" {
				toolArgs["endDateTime"] = v
			}
			if v := flagOrFallback(cmd, "desc", "description"); v != "" {
				toolArgs["description"] = v
			}
			if v, _ := cmd.Flags().GetString("timezone"); v != "" {
				toolArgs["timeZone"] = v
			}
			recurrence, err := buildRecurrence(cmd)
			if err != nil {
				return err
			}
			if recurrence != nil {
				toolArgs["recurrence"] = recurrence
			}
			if v, _ := cmd.Flags().GetString("rich-text-desc"); v != "" {
				toolArgs["richTextDescription"] = v
			}
			if v, _ := cmd.Flags().GetString("location"); v != "" {
				toolArgs["location"] = v
			}
			if v, _ := cmd.Flags().GetString("free-busy"); v != "" {
				toolArgs["freeBusy"] = v
			}
			if v := flagOrFallback(cmd, "calendar-id", "calendarId", "calendar"); v != "" {
				toolArgs["calendarId"] = v
			}
			return callMCPTool("update_calendar_event", toolArgs)
		},
	}
	DeclareLeafMetadata(eventUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "update_calendar_event",
				CanonicalPath:  "calendar.update_calendar_event",
				CLIPath:        "calendar event update",
				PrimaryCLIPath: "calendar event update",
			},
			Description: "修改日程",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "update_calendar_event"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "修改日程",
				UseWhen:      []string{"已知 eventId 且具备组织者权限，需要修改标题、时间、地点、描述等字段时"},
				AvoidWhen: []string{
					"改参会人请用 attendee add/delete；改会议室请用 room add/delete",
					"目标字段未确认时不要更新",
				},
				Examples: []string{
					"dws calendar event update --id <EVENT_ID> --title \"新标题\"",
					"dws calendar event update --id <EVENT_ID> --desc \"新描述\" --timezone Asia/Tokyo",
				},
			},
			Parameters: append([]contract.ParamDecl{
				{Name: "desc", Property: "description"},
				{Name: "end", Property: "endDateTime"},
				{Name: "id", Property: "eventId"},
				{Name: "rich-text-desc", Property: "richTextDescription"},
				{Name: "start", Property: "startDateTime"},
				{Name: "timezone", Property: "timeZone"},
				{Name: "title", Property: "summary"},
			}, calendarRecurrenceParamDecls()...),
		},
	})

	eventDeleteCmd := &cobra.Command{
		Use:     "delete",
		Short:   "删除日程",
		Example: `  dws calendar event delete --id EVENT_ID  # 查询 eventId: dws calendar event list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID, err := mustFlagOrFallback(cmd, "id", "event", "event-id", "eventId")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"eventId": eventID}
			if v := flagOrFallback(cmd, "calendar-id", "calendarId", "calendar"); v != "" {
				toolArgs["calendarId"] = v
			}
			return callMCPTool("delete_calendar_event", toolArgs)
		},
	}
	DeclareLeafMetadata(eventDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "delete_calendar_event",
				CanonicalPath:  "calendar.delete_calendar_event",
				CLIPath:        "calendar event delete",
				PrimaryCLIPath: "calendar event delete",
			},
			Description: "删除指定日程",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "delete_calendar_event"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "删除指定日程",
				UseWhen:      []string{"用户明确要求删除/取消指定日程，且 eventId 已确认时（组织者删除会通知所有参与者）"},
				AvoidWhen: []string{
					"只需从自己日历移除参与身份且非组织者场景需区分影响范围",
					"目标 eventId 未确认或仍需保留日程时不要删除",
				},
				Examples: []string{"dws calendar event delete --id <id>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "eventId"},
			},
		},
	})

	eventSuggestCmd := &cobra.Command{
		Use:   "suggest",
		Short: "建议日程时间",
		Long:  `对于非明确时间或一段时间范围的约会场景，可基于所有参会人的忙闲状态，推荐多个可用的时间块方案，用于解决会议时间协调问题。`,
		Example: `  dws calendar event suggest --users userId1,userId2 --duration 60
  dws calendar event suggest --start "2026-03-10T09:00:00+08:00" --end "2026-03-10T18:00:00+08:00" --users userId1
  dws calendar event suggest --users userId1 --duration 30 --timezone Asia/Tokyo`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v := flagOrFallback(cmd, "start", "time-min", "min-time", "start-time", "startTime", "start_time", "start-date", "startDate"); v != "" {
				toolArgs["start"] = v
			}
			if v := flagOrFallback(cmd, "end", "time-max", "max-time", "end-time", "endTime", "end_time", "end-date", "endDate"); v != "" {
				toolArgs["end"] = v
			}
			if v, _ := cmd.Flags().GetString("timezone"); v != "" {
				toolArgs["timeZone"] = v
			}
			if v := flagOrFallback(cmd, "users", "user-ids", "userIds", "attendees", "attendee-ids", "attendeeIds", "attendee", "user", "participants", "participant-ids", "participantIds", "members"); v != "" {
				toolArgs["attendeeUserIds"] = parseCSVValues(v)
			}
			if v, _ := cmd.Flags().GetString("duration"); v != "" {
				toolArgs["durationMinutes"] = v
			}
			return callMCPTool("list_suggested_event_times", toolArgs)
		},
	}
	DeclareLeafMetadata(eventSuggestCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "list_suggested_event_times",
				CanonicalPath:  "calendar.list_suggested_event_times",
				CLIPath:        "calendar event suggest",
				PrimaryCLIPath: "calendar event suggest",
			},
			Description: "建议日程时间",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "list_suggested_event_times"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "建议日程时间",
				UseWhen:      []string{"时间未定，需要基于参会人闲忙推荐可用时间块时"},
				AvoidWhen: []string{
					"时间已确定只需创建日程时改用 event create",
					"只查闲忙片段时改用 busy search",
				},
				Examples: []string{
					"dws calendar event suggest --users userId1,userId2 --duration 60",
					"dws calendar event suggest --start \"2026-03-10T09:00:00+08:00\" --end \"2026-03-10T18:00:00+08:00\" --users userId1",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "duration", Property: "durationMinutes"},
				{Name: "timezone", Property: "timeZone"},
				{Name: "users", Property: "attendeeUserIds"},
			},
		},
	})

	eventRespondCmd := &cobra.Command{
		Use:   "respond",
		Short: "响应日程（接受/拒绝/暂定）",
		Long:  `作为日程参会人，设置自己的响应状态（接受、拒绝、暂定）。注意：订阅日历下的日程无参会人，因此不可响应`,
		Example: `  dws calendar event respond --id EVENT_ID --status accepted
  dws calendar event respond --id EVENT_ID --status declined
  dws calendar event respond --id EVENT_ID --status tentative
  # 查询 eventId: dws calendar event list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID, err := mustFlagOrFallback(cmd, "id", "event", "event-id", "eventId")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "status"); err != nil {
				return err
			}
			status := mustGetFlag(cmd, "status")
			valid := map[string]bool{"needsAction": true, "accepted": true, "declined": true, "tentative": true}
			if !valid[status] {
				return &CLIError{
					Code:       CodeMCPToolError,
					Message:    "invalid --status value: " + status,
					Suggestion: "可选值: needsAction(未操作), accepted(接受), declined(拒绝), tentative(暂定)",
				}
			}
			toolArgs := map[string]any{
				"eventId":        eventID,
				"responseStatus": status,
			}
			if v := flagOrFallback(cmd, "calendar-id", "calendarId", "calendar"); v != "" {
				toolArgs["calendarId"] = v
			}
			return callMCPTool("respond", toolArgs)
		},
	}
	DeclareLeafMetadata(eventRespondCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "respond",
				CanonicalPath:  "calendar.respond",
				CLIPath:        "calendar event respond",
				PrimaryCLIPath: "calendar event respond",
			},
			Description: "响应日程",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "respond"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "响应日程",
				UseWhen:      []string{"作为参会人需要设置自己的响应状态（accepted/declined/tentative）时"},
				AvoidWhen: []string{
					"是组织者要改日程内容时改用 event update",
					"订阅日历日程不可响应",
				},
				Examples: []string{
					"dws calendar event respond --id <EVENT_ID> --status accepted",
					"dws calendar event respond --id <EVENT_ID> --status declined",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "eventId"},
				{Name: "status", Property: "responseStatus"},
			},
		},
	})

	// ── attendee: 参会人 (曾用名: participant) ─────────────────

	participantCmd := &cobra.Command{
		Use:     "attendee",
		Aliases: []string{"participant"},
		Short:   "日程参会人管理",
		Long:    "管理日程的参会人。alias：`participant`，仍作为别名保留，历史调用无需改动。",
		RunE:    groupRunE,
	}

	participantListCmd := &cobra.Command{
		Use:     "list",
		Short:   "查看参会人",
		Long:    `查看日程的参会人列表。注意：订阅日历下的日程无参会人，因此不可查看`,
		Example: `  dws calendar attendee list --event EVENT_ID  # 查询 eventId: dws calendar event list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID, err := mustFlagOrFallback(cmd, "event", "id", "event-id", "eventId")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"eventId": eventID,
			}
			if v := flagOrFallback(cmd, "calendar-id", "calendarId", "calendar"); v != "" {
				toolArgs["calendarId"] = v
			}
			return callMCPTool("get_calendar_participants", toolArgs)
		},
	}
	DeclareLeafMetadata(participantListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "get_calendar_participants",
				CanonicalPath:  "calendar.get_calendar_participants",
				CLIPath:        "calendar attendee list",
				PrimaryCLIPath: "calendar attendee list",
			},
			Description: "获取日程参与人",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "get_calendar_participants"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取日程参与人",
				UseWhen:      []string{"已知 eventId，需要查看参会人列表及状态时（订阅日历日程无参会人）"},
				AvoidWhen:    []string{"要添加/移除参会人时改用 attendee add/delete"},
				Examples:     []string{"dws calendar attendee list --event <EVENT_ID>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "calendar-id", Property: "calendarId"},
				{Name: "event", Property: "eventId"},
			},
		},
	})

	participantAddCmd := &cobra.Command{
		Use:   "add",
		Short: "添加参会人",
		Long:  `添加日程的参会人。注意：订阅日历下的日程无参会人，因此不可添加`,
		Example: `  dws calendar attendee add --event EVENT_ID --attendees userId1,userId2
  dws calendar attendee add --event EVENT_ID --attendees userId1 --optional
  # 查询 eventId: dws calendar event list
  # 查询 userId: dws contact user search --keyword "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID, err := mustFlagOrFallback(cmd, "event", "id", "event-id", "eventId")
			if err != nil {
				return err
			}
			attendees, err := mustFlagOrFallback(cmd, "attendees", "users", "user-ids", "userIds", "attendee-ids", "attendeeIds", "attendee", "user", "participants", "participant-ids", "participantIds", "members")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"eventId":        eventID,
				"attendeesToAdd": parseCSVValues(attendees),
			}
			if v, _ := cmd.Flags().GetBool("optional"); v {
				toolArgs["optional"] = true
			}
			if v := flagOrFallback(cmd, "calendar-id", "calendarId", "calendar"); v != "" {
				toolArgs["calendarId"] = v
			}
			return callMCPTool("add_calendar_participant", toolArgs)
		},
	}
	DeclareLeafMetadata(participantAddCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "add_calendar_participant",
				CanonicalPath:  "calendar.add_calendar_participant",
				CLIPath:        "calendar attendee add",
				PrimaryCLIPath: "calendar attendee add",
			},
			Description: "添加日程参与人",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "add_calendar_participant"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "添加日程参与人",
				UseWhen:      []string{"已知 eventId，需要向已有日程批量添加参会人时（订阅日历日程无参会人）"},
				AvoidWhen: []string{
					"要查看参会人时改用 dws calendar attendee list",
					"要移除参会人时改用 dws calendar attendee delete",
				},
				Examples: []string{
					"dws calendar attendee add --event <EVENT_ID> --attendees <USER_ID_1>,<USER_ID_2>",
					"dws calendar attendee add --event <EVENT_ID> --attendees <USER_ID> --optional",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "attendees", Property: "attendeesToAdd"},
				{Name: "calendar-id", Property: "calendarId"},
				{Name: "event", Property: "eventId"},
			},
		},
	})

	participantDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "移除参会人",
		Long:  `移除日程的参会人。注意：订阅日历下的日程无参会人，因此不可移除`,
		Example: `  dws calendar attendee delete --event EVENT_ID --attendees userId1
  # 查询 eventId: dws calendar event list
  # 查询 userId: dws contact user search --keyword "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID, err := mustFlagOrFallback(cmd, "event", "id", "event-id", "eventId")
			if err != nil {
				return err
			}
			attendees, err := mustFlagOrFallback(cmd, "attendees", "users", "user-ids", "userIds", "attendee-ids", "attendeeIds", "attendee", "user", "participants", "participant-ids", "participantIds", "members")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"eventId":           eventID,
				"attendeesToRemove": parseCSVValues(attendees),
			}
			if v := flagOrFallback(cmd, "calendar-id", "calendarId", "calendar"); v != "" {
				toolArgs["calendarId"] = v
			}
			return callMCPTool("remove_calendar_participant", toolArgs)
		},
	}
	DeclareLeafMetadata(participantDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "remove_calendar_participant",
				CanonicalPath:  "calendar.remove_calendar_participant",
				CLIPath:        "calendar attendee delete",
				PrimaryCLIPath: "calendar attendee delete",
			},
			Description: "删除日程参与人",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "remove_calendar_participant"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "删除日程参与人",
				UseWhen:      []string{"已知 eventId，需要从日程批量移除参会人时（订阅日历日程无参会人）"},
				AvoidWhen: []string{
					"要添加参会人时改用 attendee add",
					"要查看参会人时改用 attendee list",
				},
				Examples: []string{"dws calendar attendee delete --attendees <attendees> --event <event>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "attendees", Property: "attendeesToRemove"},
				{Name: "calendar-id", Property: "calendarId"},
				{Name: "event", Property: "eventId"},
			},
		},
	})

	// ── room: 会议室 ────────────────────────────────────────────

	roomCmd := &cobra.Command{Use: "room", Short: "会议室管理", RunE: groupRunE}

	roomSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "搜索会议室 (按名称搜索或按时间段查可用会议室)",
		Long: `
			**搜索会议室** 支持两种模式：

			1. **按名称搜索**（仅传 --room-name，不传 --start/--end）：
			   根据会议室名称模糊搜索，返回所有匹配的会议室，不检查可用性。
			   适用于"找到某个会议室"的场景。

			2. **按时间段搜索可用会议室**（传 --start/--end，或不传任何参数）：
			   查询指定时间段内所有可用的会议室。不传 --start/--end 时默认查询当前时间起 1 小时内的会议室。
			   注意，大部分会议室仅在工作时间可用，如果检索时间不在工作时间可能查不到任何结果。
			   时间约束：start 必须是未来的时间（API 限制：start can not less current time）。
				- 若传入的 --start 早于当前时间，CLI 自动修正为 now+1min。
				- 若传入的 --end 早于当前时间，视为无效请求返回错误（无法检索过去的时间段）。

			使用 --room-name 按会议室名称过滤时，CLI 原样透传给服务端做模糊匹配。调用方（模型/脚本）在传入前应主动精简名称，剔除用户口语里常带的通用后缀（如「会议室」「大会议室」「小会议室」），只保留核心专名，避免因后缀导致匹配不到。例如用户说「永澄亭会议室」时，应传 --room-name 永澄亭。
			如果知道roomId，想查该会议室的预订记录，直接用dws calendar busy search 指令。
			此指令搜索到的会议室结果中，有两个值需要注意：
				- customApprovalProcess: true - 表示该会议室设置了自定义审批流程，只能通过客户端完成预订。
				- supportRecurring: true - 表示该会议室支持循环预定；false - 表示不支持循环预定，直接加入到循环日程会失败。
			`,
		Example: `  # 按名称搜索（不检查可用性，返回所有匹配的会议室）
  dws calendar room search --room-name 永澄亭   # 用户说「永澄亭会议室」时，调用方需先精简为「永澄亭」

  # 按时间段搜索可用会议室
  dws calendar room search --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00"
  dws calendar room search --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00" --group-id GROUP_ID
  dws calendar room search   # 不传 --start/--end 时默认当前时间起 1 小时

  # 名称 + 时间段：搜索指定名称的可用会议室
  dws calendar room search --room-name 永澄亭 --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00"

  # 分页
  dws calendar room search --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00" --limit 20 --page 0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve room name (common to both modes)
			roomName := strings.TrimSpace(flagOrFallback(cmd, "room-name", "roomName", "name", "query"))

			// Resolve time flags to determine routing
			startStr := flagOrFallback(cmd, "start", "time-min", "min-time", "start-time", "startTime", "start_time", "start-date", "startDate")
			endStr := flagOrFallback(cmd, "end", "time-max", "max-time", "end-time", "endTime", "end_time", "end-date", "endDate")

			// ── Mode 1: Name-only search ──
			// Triggered when --room-name is provided AND neither --start nor --end is provided.
			// Calls search_rooms MCP tool: fuzzy name search, no availability check, no time constraint.
			if roomName != "" && startStr == "" && endStr == "" {
				toolArgs := map[string]any{
					"roomName": roomName,
				}
				return callSearchRoomsByName(cmd, toolArgs)
			}

			// ── Mode 2: Availability search ──
			// Triggered when --start/--end is provided, or when no --room-name is given.
			// Calls query_available_meeting_room MCP tool: returns only rooms free in the time range.
			now := time.Now()
			startProvided := startStr != ""
			endProvided := endStr != ""
			startCorrected := false

			// --- 时间约束：API 要求 startTime 必须是未来时间 ---
			// 若用户传了 --end 且 end <= now，直接报错（无法检索过去的时间段）
			if endStr != "" {
				if endT, e := time.Parse(time.RFC3339, endStr); e == nil && !endT.After(now) {
					return &CLIError{
						Code:       CodeMCPToolError,
						Message:    "--end 不能早于当前时间，无法检索过去的时间段",
						Suggestion: "请传入一个未来的结束时间",
					}
				}
			}

			if startStr == "" {
				// 留 1 分钟缓冲，避免请求到达服务端时 start < now 被拒 (filterStartTime can not less current time)
				startStr = now.Add(1 * time.Minute).Format(time.RFC3339)
			} else {
				// 用户显式传了 --start 但已过期 → 自动修正为 now+1min
				if startT, e := time.Parse(time.RFC3339, startStr); e == nil && !startT.After(now) {
					startStr = now.Add(1 * time.Minute).Format(time.RFC3339)
					startCorrected = true
				}
			}
			if endStr == "" {
				endStr = now.Add(1 * time.Hour).Format(time.RFC3339)
			}

			startTime, err := parseISOTimeToMillis("start", startStr)
			if err != nil {
				return err
			}
			endTime, err := parseISOTimeToMillis("end", endStr)
			if err != nil {
				return err
			}
			if err := validateTimeRange(startTime, endTime); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"startTime": startTime,
				"endTime":   endTime,
			}
			// mcp里并无此参数
			if v, _ := cmd.Flags().GetBool("available"); v {
				toolArgs["needAvailable"] = true
			}
			if v, _ := cmd.Flags().GetString("group-id"); v != "" {
				toolArgs["groupId"] = v
			}
			if roomName != "" {
				toolArgs["roomName"] = roomName
			}
			if v := flagOrFallback(cmd, "limit", "page-size", "size", "count"); v != "" {
				toolArgs["pageSize"] = v
			}
			if v := flagOrFallback(cmd, "page", "page-index", "page-num", "page-no"); v != "" {
				toolArgs["pageIndex"] = v
			}
			return callMeetingRoomSearchResult(cmd, toolArgs, startProvided, endProvided, startCorrected)
		},
	}
	DeclareLeafMetadata(roomSearchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "query_available_meeting_room",
				CanonicalPath:  "calendar.query_available_meeting_room",
				CLIPath:        "calendar room search",
				PrimaryCLIPath: "calendar room search",
			},
			Description: "查询空闲或按名搜索会议室",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "query_available_meeting_room"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询空闲或按名搜索会议室",
				UseWhen:      []string{"需要按名称模糊搜会议室，或按时间段查询全程空闲且可预订的会议室时"},
				AvoidWhen: []string{
					"要完成预订时在取得 roomId 后改用 room add",
					"只要会议室分组时改用 room list-groups",
				},
				Examples: []string{
					"dws calendar room search --start \"2026-03-10T14:00:00+08:00\" --end \"2026-03-10T15:00:00+08:00\"",
					"dws calendar room search --start \"2026-03-10T14:00:00+08:00\" --end \"2026-03-10T15:00:00+08:00\" --group-id <GROUP_ID>",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "end", Property: "endTime"},
				{Name: "limit", Property: "pageSize"},
				{Name: "page", Property: "pageIndex"},
				{Name: "start", Property: "startTime"},
			},
		},
	})

	roomAddCmd := &cobra.Command{
		Use:   "add",
		Short: "预定会议室",
		Long:  `room是会议室，用于线下开会场景。将room加入到日程完成预订`,
		Example: `  dws calendar room add --event EVENT_ID --rooms roomId1,roomId2
  # 查询 eventId: dws calendar event list
  # 查询会议室: dws calendar room list-groups`,
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID, err := mustFlagOrFallback(cmd, "event", "id", "event-id", "eventId")
			if err != nil {
				return err
			}
			rooms, err := mustFlagOrFallback(cmd, "rooms", "room-ids", "roomIds")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"eventId": eventID,
				"roomIds": parseCSVValues(rooms),
			}
			if v := flagOrFallback(cmd, "calendar-id", "calendarId", "calendar"); v != "" {
				toolArgs["calendarId"] = v
			}
			return callMCPTool("add_meeting_room", toolArgs)
		},
	}
	DeclareLeafMetadata(roomAddCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "add_meeting_room",
				CanonicalPath:  "calendar.add_meeting_room",
				CLIPath:        "calendar room add",
				PrimaryCLIPath: "calendar room add",
			},
			Description: "为日程预订会议室",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "add_meeting_room"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "为日程预订会议室",
				UseWhen:      []string{"已知 eventId 与 roomId，需要为已有日程预订线下会议室时"},
				AvoidWhen: []string{
					"要搜索空闲会议室时先用 dws calendar room search",
					"要取消会议室预订时改用 dws calendar room delete",
				},
				Examples: []string{
					"dws calendar room add --event <EVENT_ID> --rooms <ROOM_ID>",
					"dws calendar room add --event <EVENT_ID> --rooms <ROOM_ID> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "event", Property: "eventId"},
				{Name: "rooms", Property: "roomIds"},
			},
		},
	})

	roomDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "移除会议室",
		Example: `  dws calendar room delete --event EVENT_ID --rooms roomId1
  # 查询 eventId: dws calendar event list
  # 查询会议室: dws calendar room list-groups`,
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID, err := mustFlagOrFallback(cmd, "event", "id", "event-id", "eventId")
			if err != nil {
				return err
			}
			rooms, err := mustFlagOrFallback(cmd, "rooms", "room-ids", "roomIds")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"eventId": eventID,
				"roomIds": parseCSVValues(rooms),
			}
			if v := flagOrFallback(cmd, "calendar-id", "calendarId", "calendar"); v != "" {
				toolArgs["calendarId"] = v
			}
			return callMCPTool("delete_meeting_room", toolArgs)
		},
	}
	DeclareLeafMetadata(roomDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "delete_meeting_room",
				CanonicalPath:  "calendar.delete_meeting_room",
				CLIPath:        "calendar room delete",
				PrimaryCLIPath: "calendar room delete",
			},
			Description: "移除日程中的会议室",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "delete_meeting_room"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "移除日程中的会议室",
				UseWhen:      []string{"已知 eventId 与 roomId，需要取消日程会议室预订并释放占用时"},
				AvoidWhen: []string{
					"要新增预订时改用 dws calendar room add",
					"eventId/roomId 未确认时不要移除",
				},
				Examples: []string{"dws calendar room delete --event <event> --rooms <rooms>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "event", Property: "eventId"},
				{Name: "rooms", Property: "roomIds"},
			},
		},
	})

	roomListGroupsCmd := &cobra.Command{
		Use:   "list-groups",
		Short: "会议室分组列表",
		Example: `  dws calendar room list-groups
  dws calendar room list-groups --limit 20 --page 0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v := flagOrFallback(cmd, "limit", "page-size", "size", "count"); v != "" {
				toolArgs["pageSize"] = v
			}
			if v := flagOrFallback(cmd, "page", "page-index", "page-num", "page-no"); v != "" {
				toolArgs["pageIndex"] = v
			}
			if len(toolArgs) == 0 {
				return callMeetingRoomMCPTool("list_meeting_room_groups", nil)
			}
			return callMeetingRoomMCPTool("list_meeting_room_groups", toolArgs)
		},
	}
	DeclareLeafMetadata(roomListGroupsCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "list_meeting_room_groups",
				CanonicalPath:  "calendar.list_meeting_room_groups",
				CLIPath:        "calendar room list-groups",
				PrimaryCLIPath: "calendar room list-groups",
			},
			Description: "查询会议室分组列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "list_meeting_room_groups"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询会议室分组列表",
				UseWhen:      []string{"需要分页查看企业会议室分组（groupId/parentId）时"},
				AvoidWhen:    []string{"要搜空闲会议室时改用 dws calendar room search"},
				Examples: []string{
					"dws calendar room list-groups",
					"dws calendar room list-groups --limit 20 --page 0",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "limit", Property: "pageSize"},
				{Name: "page", Property: "pageIndex"},
			},
		},
	})

	// ── busy: 闲忙 ──────────────────────────────────────────────

	busyCmd := &cobra.Command{Use: "busy", Short: "闲忙查询", RunE: groupRunE}

	busySearchCmd := &cobra.Command{
		Use:   "search",
		Short: "查询用户 / 会议室闲忙状态",
		Long:  `查询指定用户或会议室在给定时间范围内的闲忙状态。--users 与 --rooms 至少指定其一，也可以同时指定（查询用户+会议室）。`,
		Example: `  dws calendar busy search --users userId1,userId2 --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T18:00:00+08:00"
  dws calendar busy search --rooms roomId1,roomId2 --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T18:00:00+08:00"
  dws calendar busy search --users userId1 --rooms roomId1 --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T18:00:00+08:00"
  # 查询 userId: dws contact user search --keyword "姓名"
  # 查询 roomId: dws calendar room search 或 dws calendar room list-groups`,
		RunE: func(cmd *cobra.Command, args []string) error {
			users := flagOrFallback(cmd, "users", "user-ids", "userIds", "attendees", "attendee-ids", "attendeeIds", "attendee", "user", "participants", "participant-ids", "participantIds", "members")
			rooms := flagOrFallback(cmd, "rooms", "room-ids", "roomIds")
			if strings.TrimSpace(users) == "" && strings.TrimSpace(rooms) == "" {
				return &CLIError{
					Code:       CodeMissingParam,
					Message:    "--users 与 --rooms 至少指定其一",
					Suggestion: "查询用户闲忙用 --users userId1,userId2；查询会议室闲忙用 --rooms roomId1,roomId2；两者可同时指定",
				}
			}
			startRaw, err := mustFlagOrFallback(cmd, "start", "time-min", "min-time", "start-time", "startTime", "start_time", "start-date", "startDate")
			if err != nil {
				return err
			}
			endRaw, err := mustFlagOrFallback(cmd, "end", "time-max", "max-time", "end-time", "endTime", "end_time", "end-date", "endDate")
			if err != nil {
				return err
			}
			startTime, err := parseISOTimeToMillis("start", startRaw)
			if err != nil {
				return err
			}
			endTime, err := parseISOTimeToMillis("end", endRaw)
			if err != nil {
				return err
			}
			if err := validateTimeRange(startTime, endTime); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"startTime": startTime,
				"endTime":   endTime,
			}
			if strings.TrimSpace(users) != "" {
				toolArgs["userIds"] = parseCSVValues(users)
			}
			if strings.TrimSpace(rooms) != "" {
				toolArgs["roomIds"] = parseCSVValues(rooms)
			}
			return callFilteredBusyStatus(cmd, toolArgs)
		},
	}
	DeclareLeafMetadata(busySearchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "query_busy_status",
				CanonicalPath:  "calendar.query_busy_status",
				CLIPath:        "calendar busy search",
				PrimaryCLIPath: "calendar busy search",
			},
			Description: "查询用户或会议室闲忙",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "query_busy_status"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询用户或会议室闲忙",
				UseWhen:      []string{"需要查询指定用户和/或会议室在时间范围内的已占用时段时（users 与 rooms 至少其一）"},
				AvoidWhen:    []string{"需要创建或改日程时不要使用；本命令只返回闲忙时间片段"},
				Examples: []string{
					"dws calendar busy search --users <USER_ID_1>,<USER_ID_2> --start \"2026-03-10T14:00:00+08:00\" --end \"2026-03-10T18:00:00+08:00\"",
					"dws calendar busy search --rooms <ROOM_ID_1>,<ROOM_ID_2> --start \"2026-03-10T14:00:00+08:00\" --end \"2026-03-10T18:00:00+08:00\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "end", Property: "endTime"},
				{Name: "rooms", Property: "roomIds"},
				{Name: "start", Property: "startTime"},
				{Name: "users", Property: "userIds"},
			},
		},
	})

	// ── attachment: 附件 ────────────────────────────────────────

	attachmentCmd := &cobra.Command{Use: "attachment", Short: "日程附件管理", RunE: groupRunE}

	attachmentAddCmd := &cobra.Command{
		Use:   "add",
		Short: "添加日程附件",
		Long:  `为指定日程添加附件。需先用钉盘上传文件得到 fileId，再以 <fileId>:<name> 形式逗号分隔传入。注意：订阅日历下的日程不支持添加附件`,
		Example: `  dws calendar attachment add --event EVENT_ID --files fileId1:report.pdf,fileId2:slides.pptx
  # 查询 eventId: dws calendar event list
  # 上传文件并获取 fileId: dws drive 相关命令`,
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID, err := mustFlagOrFallback(cmd, "event", "id", "event-id", "eventId")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "files"); err != nil {
				return err
			}
			raw := mustGetFlag(cmd, "files")
			parts := parseCSVValues(raw)
			attachments := make([]map[string]any, 0, len(parts))
			for _, p := range parts {
				idx := strings.Index(p, ":")
				if idx <= 0 || idx >= len(p)-1 {
					return &CLIError{
						Code:    CodeMCPToolError,
						Message: "invalid --files entry: " + p,
						Suggestion: "每个附件必须以 <fileId>:<name> 形式提供，多个用逗号分隔，例如 " +
							"--files fileId1:report.pdf,fileId2:slides.pptx",
					}
				}
				attachments = append(attachments, map[string]any{
					"id":   strings.TrimSpace(p[:idx]),
					"name": strings.TrimSpace(p[idx+1:]),
				})
			}
			toolArgs := map[string]any{
				"eventId":     eventID,
				"attachments": attachments,
			}
			if v := flagOrFallback(cmd, "calendar-id", "calendarId", "calendar"); v != "" {
				toolArgs["calendarId"] = v
			}
			return callMCPTool("add_attachments", toolArgs)
		},
	}
	DeclareLeafMetadata(attachmentAddCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "add_attachments",
				CanonicalPath:  "calendar.add_attachments",
				CLIPath:        "calendar attachment add",
				PrimaryCLIPath: "calendar attachment add",
			},
			Description: "为日程添加附件",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "add_attachments"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "为日程添加附件",
				UseWhen:      []string{"已知 eventId，已有钉盘 fileId，需要为日程添加附件时（订阅日历日程不支持）"},
				AvoidWhen: []string{
					"尚未上传钉盘文件取得 fileId 时不要调用",
					"只需查询日程时改用 dws calendar event get",
				},
				Examples: []string{
					"dws calendar attachment add --event <EVENT_ID> --files <FILE_ID>:report.pdf,<FILE_ID2>:slides.pptx",
					"dws calendar attachment add --event <EVENT_ID> --files <FILE_ID>:report.pdf --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "event", Property: "eventId"},
				{Name: "files", Property: "attachments"},
			},
		},
	})

	// ── acl: 日历访问权限 ─────────────────────────────────────────

	aclCmd := &cobra.Command{Use: "acl", Short: "管理我的日历访问权限（共享给他人）", RunE: groupRunE}

	aclListCmd := &cobra.Command{
		Use:     "list",
		Short:   "查询我的日历共享给了谁",
		Long:    `查询当前用户主日历的访问控制列表——即"我的日历共享给了哪些人、各自什么权限"。注意：这不是查日历本列表（那是 book list）。`,
		Example: `  dws calendar acl list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("list_acls", nil)
		},
	}
	DeclareLeafMetadata(aclListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "list_acls",
				CanonicalPath:  "calendar.list_acls",
				CLIPath:        "calendar acl list",
				PrimaryCLIPath: "calendar acl list",
			},
			Description: "查询主日历访问控制列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "list_acls"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询主日历访问控制列表",
				UseWhen:      []string{"需要查看主日历共享给了谁、各自权限时（不是日历本列表）"},
				AvoidWhen:    []string{"要列日历本时改用 dws calendar book list"},
				Examples:     []string{"dws calendar acl list"},
			},
		},
	})

	aclAddCmd := &cobra.Command{
		Use:   "add",
		Short: "把我的日历共享给某人",
		Long:  `将当前用户的日历共享给指定用户，授予对方查看忙闲/标题/详情或编辑权限。`,
		Example: `  dws calendar acl add --user USER_ID --privilege reader
  dws calendar acl add --user USER_ID --privilege writer --no-notification`,
		RunE: func(cmd *cobra.Command, args []string) error {
			userID, err := mustFlagOrFallback(cmd, "user", "user-id", "userId")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "privilege"); err != nil {
				return err
			}
			privilege := mustGetFlag(cmd, "privilege")
			valid := map[string]bool{"free_busy_reader": true, "title_reader": true, "reader": true, "writer": true}
			if !valid[privilege] {
				return &CLIError{
					Code:       CodeMCPToolError,
					Message:    "invalid --privilege value: " + privilege,
					Suggestion: "可选值: free_busy_reader(查看忙闲), title_reader(查看标题), reader(查看详情), writer(创建和编辑)",
				}
			}
			toolArgs := map[string]any{
				"userId":    userID,
				"privilege": privilege,
			}
			if v, _ := cmd.Flags().GetBool("no-notification"); v {
				toolArgs["sendNotification"] = false
			}
			return callMCPTool("add_acl", toolArgs)
		},
	}

	aclDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除日历访问权限",
		Long:  `删除已经授予的日历访问控制权限。`,
		Example: `  dws calendar acl delete --acl-id ACL_ID
  # 查询 aclId: dws calendar acl list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			aclID, err := mustFlagOrFallback(cmd, "acl-id", "aclId", "id")
			if err != nil {
				return err
			}
			return callMCPTool("delete_acl", map[string]any{"aclId": aclID})
		},
	}

	// ── book: 日历本 ────────────────────────────────────────────

	bookCmd := &cobra.Command{Use: "book", Short: "日历本管理（我能看哪些日历）", RunE: groupRunE}

	bookListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询用户的日历列表",
		Long: `查询当前用户的所有日历，结果范围：用户自己的日历、已订阅的公共/团队日历、他人共享的日历。主日历 id 固定为 "primary"。
			共享日历本中有来自 xxx 的，且权限大于reader，那么通过 event list --calendar-id <xxx的日历本id> 可查到xxx完整的日程安排
		`,
		Example: `  dws calendar book list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("list_calendars", nil)
		},
	}
	DeclareLeafMetadata(bookListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "list_calendars",
				CanonicalPath:  "calendar.list_calendars",
				CLIPath:        "calendar book list",
				PrimaryCLIPath: "calendar book list",
			},
			Description: "查询用户日历列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "list_calendars"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询用户日历列表",
				UseWhen:      []string{"需要列出自己的、已订阅的、他人共享的日历本（主日历 id=primary）时"},
				AvoidWhen: []string{
					"已知日历 id 要详情时改用 dws calendar book get",
					"要按名称模糊搜索时改用 dws calendar book search",
				},
				Examples: []string{"dws calendar book list"},
			},
		},
	})

	bookGetCmd := &cobra.Command{
		Use:   "get",
		Short: "查询指定日历本",
		Long:  `根据日历id查询指定日历的信息。用户主日历本id固定为 "primary"。`,
		Example: `  dws calendar book get --id primary
  dws calendar book get --id CALENDAR_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			calendarID, err := mustFlagOrFallback(cmd, "id", "calendar-id", "calendarId")
			if err != nil {
				return err
			}
			return callMCPTool("get_calendar", map[string]any{"calendarId": calendarID})
		},
	}
	DeclareLeafMetadata(bookGetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "get_calendar",
				CanonicalPath:  "calendar.get_calendar",
				CLIPath:        "calendar book get",
				PrimaryCLIPath: "calendar book get",
			},
			Description: "查询指定日历信息",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "get_calendar"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询指定日历信息",
				UseWhen:      []string{"已知日历 id（主日历固定 primary），需要查询该日历本信息时"},
				AvoidWhen: []string{
					"要列全部日历时改用 dws calendar book list",
					"要按名称搜索日历本时改用 dws calendar book search",
				},
				Examples: []string{
					"dws calendar book get --id primary",
					"dws calendar book get --id CALENDAR_ID",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "calendarId"},
			},
		},
	})

	bookSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "搜索日历本",
		Long:  `搜索当前用户拥有的日历本，支持按日历本名模糊搜索。获取全部日历请使用 list。`,
		Example: `  dws calendar book search --query "项目"
  dws calendar book search --query "团队周报"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			query, err := mustFlagOrFallback(cmd, "query", "keyword", "keywords", "search")
			if err != nil {
				return err
			}
			return callMCPTool("search_calendar", map[string]any{"query": query})
		},
	}
	DeclareLeafMetadata(bookSearchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "search_calendar",
				CanonicalPath:  "calendar.search_calendar",
				CLIPath:        "calendar book search",
				PrimaryCLIPath: "calendar book search",
			},
			Description: "搜索日历本列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "search_calendar"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "搜索日历本列表",
				UseWhen:      []string{"需要按日历本名模糊搜索当前用户拥有的日历本时"},
				AvoidWhen: []string{
					"要获取全部日历时改用 dws calendar book list",
					"已知 id 要详情时改用 book get",
				},
				Examples: []string{
					"dws calendar book search --query \"项目\"",
					"dws calendar book search --query \"团队周报\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "query", Property: "query"},
			},
		},
	})

	bookUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新指定日历本",
		Long:  `更新日历信息，最低权限要求：privilege == "owner"。注意：用户主日历本 以及 他人共享的日历本 不支持更新。`,
		Example: `  dws calendar book update --id CALENDAR_ID --summary "新日历名"
  dws calendar book update --id CALENDAR_ID --desc "日历描述"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			calendarID, err := mustFlagOrFallback(cmd, "id", "calendar-id", "calendarId")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"calendarId": calendarID}
			if v := flagOrFallback(cmd, "summary", "title"); v != "" {
				toolArgs["summary"] = v
			}
			if v := flagOrFallback(cmd, "desc", "description"); v != "" {
				toolArgs["description"] = v
			}
			return callMCPTool("update_calendar", toolArgs)
		},
	}

	// ListEvent flags
	eventListCmd.Flags().String("start", "", "开始时间 ISO-8601 (例如 2026-03-10T14:00:00+08:00)")
	eventListCmd.Flags().String("end", "", "结束时间 ISO-8601 (例如 2026-03-10T18:00:00+08:00)")
	eventListCmd.Flags().String("time-min", "", "")
	_ = eventListCmd.Flags().MarkHidden("time-min")
	eventListCmd.Flags().String("time-max", "", "")
	_ = eventListCmd.Flags().MarkHidden("time-max")
	eventListCmd.Flags().String("min-time", "", "")
	_ = eventListCmd.Flags().MarkHidden("min-time")
	eventListCmd.Flags().String("max-time", "", "")
	_ = eventListCmd.Flags().MarkHidden("max-time")
	eventListCmd.Flags().String("start-time", "", "")
	_ = eventListCmd.Flags().MarkHidden("start-time")
	eventListCmd.Flags().String("end-time", "", "")
	_ = eventListCmd.Flags().MarkHidden("end-time")
	eventListCmd.Flags().String("startTime", "", "")
	_ = eventListCmd.Flags().MarkHidden("startTime")
	eventListCmd.Flags().String("endTime", "", "")
	_ = eventListCmd.Flags().MarkHidden("endTime")
	eventListCmd.Flags().String("start_time", "", "")
	_ = eventListCmd.Flags().MarkHidden("start_time")
	eventListCmd.Flags().String("end_time", "", "")
	_ = eventListCmd.Flags().MarkHidden("end_time")
	eventListCmd.Flags().String("start-date", "", "")
	_ = eventListCmd.Flags().MarkHidden("start-date")
	eventListCmd.Flags().String("end-date", "", "")
	_ = eventListCmd.Flags().MarkHidden("end-date")
	eventListCmd.Flags().String("startDate", "", "")
	_ = eventListCmd.Flags().MarkHidden("startDate")
	eventListCmd.Flags().String("endDate", "", "")
	_ = eventListCmd.Flags().MarkHidden("endDate")
	eventListCmd.Flags().String("calendar-id", "", "日历 ID (默认 primary 主日历，仅在查询其他日历本时填写)")
	eventListCmd.Flags().String("calendarId", "", "")
	_ = eventListCmd.Flags().MarkHidden("calendarId")
	eventListCmd.Flags().String("calendar", "", "")
	_ = eventListCmd.Flags().MarkHidden("calendar")
	eventListCmd.Flags().String("cursor", "", "分页游标 (首次查询无需传入，仅翻页时传入上一次返回的 nextCursor)")
	eventListCmd.Flags().String("next-cursor", "", "")
	_ = eventListCmd.Flags().MarkHidden("next-cursor")
	eventListCmd.Flags().String("nextCursor", "", "")
	_ = eventListCmd.Flags().MarkHidden("nextCursor")
	eventListCmd.Flags().String("page-token", "", "")
	_ = eventListCmd.Flags().MarkHidden("page-token")
	eventListCmd.Flags().String("pageToken", "", "")
	_ = eventListCmd.Flags().MarkHidden("pageToken")
	eventListCmd.Flags().String("next-token", "", "")
	_ = eventListCmd.Flags().MarkHidden("next-token")

	eventListCmd.Flags().Int("limit", 0, "每页返回条数 (默认 100，最大 100)")
	eventListCmd.Flags().Int("max-results", 0, "")
	_ = eventListCmd.Flags().MarkHidden("max-results")
	eventListCmd.Flags().Int("maxResults", 0, "")
	_ = eventListCmd.Flags().MarkHidden("maxResults")
	eventListCmd.Flags().Int("page-size", 0, "")
	_ = eventListCmd.Flags().MarkHidden("page-size")
	eventListCmd.Flags().Int("size", 0, "")
	_ = eventListCmd.Flags().MarkHidden("size")
	eventListCmd.Flags().Int("count", 0, "")
	_ = eventListCmd.Flags().MarkHidden("count")
	// GetEvent flags
	eventGetCmd.Flags().String("id", "", "日程 ID (必填)")
	eventGetCmd.Flags().String("event", "", "")
	_ = eventGetCmd.Flags().MarkHidden("event")
	eventGetCmd.Flags().String("event-id", "", "")
	_ = eventGetCmd.Flags().MarkHidden("event-id")
	eventGetCmd.Flags().String("eventId", "", "")
	_ = eventGetCmd.Flags().MarkHidden("eventId")
	eventGetCmd.Flags().String("calendar-id", "", "日历 ID (默认 primary 主日历)")
	eventGetCmd.Flags().String("calendarId", "", "")
	_ = eventGetCmd.Flags().MarkHidden("calendarId")
	eventGetCmd.Flags().String("calendar", "", "")
	_ = eventGetCmd.Flags().MarkHidden("calendar")

	// CreateEvent flags
	eventCreateCmd.Flags().String("title", "", "日程标题 (必填，最大2048字符)")
	eventCreateCmd.Flags().String("summary", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("summary")
	eventCreateCmd.Flags().String("start", "", "开始时间 ISO-8601 (必填，例如 2026-03-10T14:00:00+08:00)")
	eventCreateCmd.Flags().String("end", "", "结束时间 ISO-8601 (必填，例如 2026-03-10T15:00:00+08:00)")
	eventCreateCmd.Flags().String("start-time", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("start-time")
	eventCreateCmd.Flags().String("end-time", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("end-time")
	eventCreateCmd.Flags().String("startTime", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("startTime")
	eventCreateCmd.Flags().String("endTime", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("endTime")
	eventCreateCmd.Flags().String("start_time", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("start_time")
	eventCreateCmd.Flags().String("end_time", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("end_time")
	eventCreateCmd.Flags().String("start-date", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("start-date")
	eventCreateCmd.Flags().String("end-date", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("end-date")
	eventCreateCmd.Flags().String("startDate", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("startDate")
	eventCreateCmd.Flags().String("endDate", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("endDate")
	eventCreateCmd.Flags().String("timezone", "", "时区 IANA 格式 (例如 Asia/Shanghai，默认 Asia/Shanghai)")
	eventCreateCmd.Flags().String("desc", "", "日程描述 (最大5000字符)")
	eventCreateCmd.Flags().String("description", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("description")
	eventCreateCmd.Flags().String("attendees", "", "参会人 userId 列表，逗号分隔 (最多500人)，日程组织人自动放入参会人列表，无需传入userId")
	eventCreateCmd.Flags().String("users", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("users")
	eventCreateCmd.Flags().String("user-ids", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("user-ids")
	eventCreateCmd.Flags().String("userIds", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("userIds")
	eventCreateCmd.Flags().String("user", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("user")
	eventCreateCmd.Flags().String("attendee", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("attendee")
	eventCreateCmd.Flags().String("attendee-ids", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("attendee-ids")
	eventCreateCmd.Flags().String("attendeeIds", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("attendeeIds")
	eventCreateCmd.Flags().String("participants", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("participants")
	eventCreateCmd.Flags().String("participant-ids", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("participant-ids")
	eventCreateCmd.Flags().String("participantIds", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("participantIds")
	eventCreateCmd.Flags().String("members", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("members")
	eventCreateCmd.Flags().String("open-dingtalk-ids", "", "openDingTalkId 列表，逗号分隔 (与 --attendees 至少传一个)")
	eventCreateCmd.Flags().String("rooms", "", "会议室 roomId 列表，逗号分隔 (创建时直接预定，roomId 必须来自“room search”返回，若是循环会议，必须设置recurrence-end-date，避免长期预订)")
	eventCreateCmd.Flags().String("room-ids", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("room-ids")
	eventCreateCmd.Flags().String("roomIds", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("roomIds")
	eventCreateCmd.Flags().String("recurrence-type", "", "[recurrence整体必填] 循环类型: daily|weekly|absoluteMonthly|relativeMonthly|absoluteYearly；一旦使用 --recurrence-* 任一 flag，必须同时提供完整的 pattern+range 字段集合")
	eventCreateCmd.Flags().Int("recurrence-interval", 0, "[recurrence整体必填] 循环间隔 (>0；如 daily 时表示每N天，weekly 时表示每N周，以此类推)")
	eventCreateCmd.Flags().String("recurrence-days-of-week", "", "[recurrence.pattern] 周几: sunday,monday,...,saturday (weekly/relativeMonthly 时必填)")
	eventCreateCmd.Flags().Int("recurrence-day-of-month", 0, "[recurrence.pattern] 每月第几天 (absoluteMonthly/absoluteYearly 时必填)")
	eventCreateCmd.Flags().String("recurrence-index", "", "[recurrence.pattern] 每月第几周: first|second|third|fourth|last (relativeMonthly 时必填)")
	eventCreateCmd.Flags().String("recurrence-first-day-of-week", "", "[recurrence.pattern] 一周起始日，默认 sunday")
	eventCreateCmd.Flags().String("recurrence-range-type", "", "[recurrence整体必填] 循环范围: noEnd|endDate|numbered；与 --recurrence-type 必须成对出现")
	eventCreateCmd.Flags().String("recurrence-end-date", "", "[recurrence.range] 循环结束时间 ISO-8601 (range-type=endDate 时必填)")
	eventCreateCmd.Flags().Int("recurrence-count", 0, "[recurrence.range] 循环次数 (range-type=numbered 时必填)")
	eventCreateCmd.Flags().String("rich-text-desc", "", "html格式的富文本类型日程描述，用于复杂内容的展示")
	eventCreateCmd.Flags().String("location", "", "地点信息（纯文本备注，如‘3号楼A区’；不等于预订会议室）")
	eventCreateCmd.Flags().String("free-busy", "", "此日程的忙碌状态，默认值为busy。busy - 在忙闲视图中，此日程时间段为忙碌; free - 此日程不占用忙闲")
	eventCreateCmd.Flags().String("freeBusy", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("freeBusy")
	eventCreateCmd.Flags().String("freebusy", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("freebusy")
	eventCreateCmd.Flags().String("calendar-id", "", "日历 ID (可选，默认 primary 主日历；指定共享/订阅日历本时填写，可通过 book list 获取)")
	eventCreateCmd.Flags().String("calendarId", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("calendarId")
	eventCreateCmd.Flags().String("calendar", "", "")
	_ = eventCreateCmd.Flags().MarkHidden("calendar")
	eventCreateCmd.Flags().String("remind-minutes", "", "日程开始前提醒，逗号分隔分钟数 (可选，例如 5,10,15 表示开始前5/10/15分钟提醒；不传则默认15分钟提醒)")

	// UpdateEvent flags
	eventUpdateCmd.Flags().String("id", "", "日程 ID (必填)")
	eventUpdateCmd.Flags().String("event", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("event")
	eventUpdateCmd.Flags().String("event-id", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("event-id")
	eventUpdateCmd.Flags().String("eventId", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("eventId")
	eventUpdateCmd.Flags().String("title", "", "新标题")
	eventUpdateCmd.Flags().String("summary", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("summary")
	eventUpdateCmd.Flags().String("start", "", "新开始时间 ISO-8601")
	eventUpdateCmd.Flags().String("end", "", "新结束时间 ISO-8601")
	eventUpdateCmd.Flags().String("start-time", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("start-time")
	eventUpdateCmd.Flags().String("end-time", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("end-time")
	eventUpdateCmd.Flags().String("startTime", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("startTime")
	eventUpdateCmd.Flags().String("endTime", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("endTime")
	eventUpdateCmd.Flags().String("start_time", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("start_time")
	eventUpdateCmd.Flags().String("end_time", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("end_time")
	eventUpdateCmd.Flags().String("start-date", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("start-date")
	eventUpdateCmd.Flags().String("end-date", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("end-date")
	eventUpdateCmd.Flags().String("startDate", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("startDate")
	eventUpdateCmd.Flags().String("endDate", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("endDate")
	eventUpdateCmd.Flags().String("desc", "", "新描述 (最大5000字符)")
	eventUpdateCmd.Flags().String("description", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("description")
	eventUpdateCmd.Flags().String("timezone", "", "时区 IANA 格式 (例如 Asia/Shanghai)")
	eventUpdateCmd.Flags().String("recurrence-type", "", "[recurrence整体必填] 循环类型: daily|weekly|absoluteMonthly|relativeMonthly|absoluteYearly；MCP 不合并部分字段，修改任一循环字段都要重传完整 pattern+range")
	eventUpdateCmd.Flags().Int("recurrence-interval", 0, "[recurrence整体必填] 循环间隔 (>0；如 daily 时表示每N天，weekly 时表示每N周)")
	eventUpdateCmd.Flags().String("recurrence-days-of-week", "", "[recurrence.pattern] 周几: sunday,monday,...,saturday (weekly/relativeMonthly 时必填)")
	eventUpdateCmd.Flags().Int("recurrence-day-of-month", 0, "[recurrence.pattern] 每月第几天 (absoluteMonthly/absoluteYearly 时必填)")
	eventUpdateCmd.Flags().String("recurrence-index", "", "[recurrence.pattern] 每月第几周: first|second|third|fourth|last (relativeMonthly 时必填)")
	eventUpdateCmd.Flags().String("recurrence-first-day-of-week", "", "[recurrence.pattern] 一周起始日，默认 sunday")
	eventUpdateCmd.Flags().String("recurrence-range-type", "", "[recurrence整体必填] 循环范围: noEnd|endDate|numbered；与 --recurrence-type 必须成对出现")
	eventUpdateCmd.Flags().String("recurrence-end-date", "", "[recurrence.range] 循环结束时间 ISO-8601 (range-type=endDate 时必填)")
	eventUpdateCmd.Flags().Int("recurrence-count", 0, "[recurrence.range] 循环次数 (range-type=numbered 时必填)")
	eventUpdateCmd.Flags().String("rich-text-desc", "", "html格式的富文本类型日程描述，用于复杂内容的展示")
	eventUpdateCmd.Flags().String("location", "", "地点信息（纯文本备注，如‘3号楼A区’；不等于预订会议室）")
	eventUpdateCmd.Flags().String("free-busy", "", "修改此日程的忙碌状态，无需修改则不传。busy - 在忙闲视图中，此日程时间段为忙碌; free - 此日程不占用忙闲")
	eventUpdateCmd.Flags().String("calendar-id", "", "日历 ID (可选，默认 primary 主日历；指定其他日历本时填写，可通过 book list 获取)")
	eventUpdateCmd.Flags().String("calendarId", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("calendarId")
	eventUpdateCmd.Flags().String("calendar", "", "")
	_ = eventUpdateCmd.Flags().MarkHidden("calendar")

	// DeleteEvent flags
	eventDeleteCmd.Flags().String("id", "", "日程 ID (必填)")
	eventDeleteCmd.Flags().String("event", "", "")
	_ = eventDeleteCmd.Flags().MarkHidden("event")
	eventDeleteCmd.Flags().String("event-id", "", "")
	_ = eventDeleteCmd.Flags().MarkHidden("event-id")
	eventDeleteCmd.Flags().String("eventId", "", "")
	_ = eventDeleteCmd.Flags().MarkHidden("eventId")
	eventDeleteCmd.Flags().String("calendar-id", "", "日历 ID (可选，默认 primary 主日历；指定其他日历本时填写，可通过 book list 获取)")
	eventDeleteCmd.Flags().String("calendarId", "", "")
	_ = eventDeleteCmd.Flags().MarkHidden("calendarId")
	eventDeleteCmd.Flags().String("calendar", "", "")
	_ = eventDeleteCmd.Flags().MarkHidden("calendar")

	// RespondEvent flags
	eventRespondCmd.Flags().String("id", "", "日程 ID (必填)")
	eventRespondCmd.Flags().String("event", "", "")
	_ = eventRespondCmd.Flags().MarkHidden("event")
	eventRespondCmd.Flags().String("event-id", "", "")
	_ = eventRespondCmd.Flags().MarkHidden("event-id")
	eventRespondCmd.Flags().String("eventId", "", "")
	_ = eventRespondCmd.Flags().MarkHidden("eventId")
	eventRespondCmd.Flags().String("status", "", "响应状态: needsAction(未操作)|accepted(接受)|declined(拒绝)|tentative(暂定) (必填)")
	eventRespondCmd.Flags().String("calendar-id", "", "日历 ID (可选，默认 primary 主日历；指定其他日历本时填写，可通过 book list 获取)，注意：订阅日历下的日程无参会人，因此不可响应")
	eventRespondCmd.Flags().String("calendarId", "", "")
	_ = eventRespondCmd.Flags().MarkHidden("calendarId")
	eventRespondCmd.Flags().String("calendar", "", "")
	_ = eventRespondCmd.Flags().MarkHidden("calendar")

	// SuggestEvent flags
	eventSuggestCmd.Flags().String("start", "", "推荐时间范围开始 ISO-8601 (默认当前时间)")
	eventSuggestCmd.Flags().String("end", "", "推荐时间范围结束 ISO-8601 (默认次日18点)")
	eventSuggestCmd.Flags().String("time-min", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("time-min")
	eventSuggestCmd.Flags().String("time-max", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("time-max")
	eventSuggestCmd.Flags().String("min-time", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("min-time")
	eventSuggestCmd.Flags().String("max-time", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("max-time")
	eventSuggestCmd.Flags().String("start-time", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("start-time")
	eventSuggestCmd.Flags().String("end-time", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("end-time")
	eventSuggestCmd.Flags().String("startTime", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("startTime")
	eventSuggestCmd.Flags().String("endTime", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("endTime")
	eventSuggestCmd.Flags().String("start_time", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("start_time")
	eventSuggestCmd.Flags().String("end_time", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("end_time")
	eventSuggestCmd.Flags().String("start-date", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("start-date")
	eventSuggestCmd.Flags().String("end-date", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("end-date")
	eventSuggestCmd.Flags().String("startDate", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("startDate")
	eventSuggestCmd.Flags().String("endDate", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("endDate")
	eventSuggestCmd.Flags().String("timezone", "", "时区 IANA 格式 (默认 Asia/Shanghai)")
	eventSuggestCmd.Flags().String("users", "", "参会人 userId 列表，逗号分隔")
	eventSuggestCmd.Flags().String("user-ids", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("user-ids")
	eventSuggestCmd.Flags().String("userIds", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("userIds")
	eventSuggestCmd.Flags().String("attendees", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("attendees")
	eventSuggestCmd.Flags().String("attendee-ids", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("attendee-ids")
	eventSuggestCmd.Flags().String("attendeeIds", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("attendeeIds")
	eventSuggestCmd.Flags().String("attendee", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("attendee")
	eventSuggestCmd.Flags().String("user", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("user")
	eventSuggestCmd.Flags().String("participants", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("participants")
	eventSuggestCmd.Flags().String("participant-ids", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("participant-ids")
	eventSuggestCmd.Flags().String("participantIds", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("participantIds")
	eventSuggestCmd.Flags().String("members", "", "")
	_ = eventSuggestCmd.Flags().MarkHidden("members")
	eventSuggestCmd.Flags().String("duration", "", "日程持续时间 (分钟，默认30)")

	eventInstancesCmd := &cobra.Command{
		Use:   "instances",
		Short: "查询循环日程的实例列表",
		Long: `查询指定重复性日程（SeriesMaster）在指定时间范围内的所有实例。
**注意**：此接口只能查询重复性日程的实例；若传入的是普通非循环日程，将查不到任何实例信息。
必须传入 --id 指定重复性日程的 eventId（即 SeriesMaster 的 eventId，可通过 event list 获取）。
不传 --start/--end 时，默认查询今天（00:00:00 ~ 23:59:59）的实例。`,
		Example: `  dws calendar event instances --id EVENT_ID
  dws calendar event instances --id EVENT_ID --start "2026-03-10T00:00:00+08:00" --end "2026-03-31T23:59:59+08:00"
  dws calendar event instances --id EVENT_ID --limit 50
  dws calendar event instances --id EVENT_ID --cursor "<nextCursor>"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			eventID, err := mustFlagOrFallback(cmd, "id", "event", "event-id", "eventId")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"eventId": eventID}
			var startTime, endTime int64
			var now time.Time
			if v := flagOrFallback(cmd, "start", "time-min", "min-time", "start-time", "startTime", "start_time", "start-date", "startDate"); v != "" {
				startTime, err = parseISOTimeToMillis("start", v)
				if err != nil {
					return err
				}
				toolArgs["startTime"] = startTime
			} else {
				now = time.Now()
				startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UnixMilli()
				toolArgs["startTime"] = startTime
			}
			if v := flagOrFallback(cmd, "end", "time-max", "max-time", "end-time", "endTime", "end_time", "end-date", "endDate"); v != "" {
				endTime, err = parseISOTimeToMillis("end", v)
				if err != nil {
					return err
				}
				toolArgs["endTime"] = endTime
			} else {
				if now.IsZero() {
					now = time.Now()
				}
				endTime = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location()).UnixMilli()
				toolArgs["endTime"] = endTime
			}
			if err := validateTimeRange(startTime, endTime); err != nil {
				return err
			}
			if v := flagOrFallback(cmd, "calendar-id", "calendarId", "calendar"); v != "" {
				toolArgs["calendarId"] = v
			}
			if v := flagOrFallback(cmd, "cursor", "next-cursor", "nextCursor", "page-token", "pageToken", "next-token"); v != "" {
				toolArgs["cursor"] = v
			}
			if lim, _ := cmd.Flags().GetInt("limit"); lim > 0 {
				toolArgs["limit"] = lim
			} else if lim, _ := cmd.Flags().GetInt("max-results"); lim > 0 {
				toolArgs["limit"] = lim
			} else if lim, _ := cmd.Flags().GetInt("maxResults"); lim > 0 {
				toolArgs["limit"] = lim
			} else if lim, _ := cmd.Flags().GetInt("page-size"); lim > 0 {
				toolArgs["limit"] = lim
			} else if lim, _ := cmd.Flags().GetInt("size"); lim > 0 {
				toolArgs["limit"] = lim
			} else if lim, _ := cmd.Flags().GetInt("count"); lim > 0 {
				toolArgs["limit"] = lim
			}
			return callSortedCalendarEvents(cmd, "list_event_instances", toolArgs)
		},
	}
	DeclareLeafMetadata(eventInstancesCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "calendar",
				Name:           "list_event_instances",
				CanonicalPath:  "calendar.list_event_instances",
				CLIPath:        "calendar event instances",
				PrimaryCLIPath: "calendar event instances",
			},
			Description: "查询循环日程的实例列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "calendar", RPCName: "list_event_instances"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询循环日程在时间范围内展开的实例",
				UseWhen:      []string{"已知循环日程 eventId（SeriesMaster），需要列出某时间窗内的实例时"},
				AvoidWhen: []string{
					"普通非循环日程请用 dws calendar event get / list",
					"未知 eventId 时先 dws calendar event list",
				},
				Examples: []string{
					"dws calendar event instances --id <EVENT_ID>",
					"dws calendar event instances --id <EVENT_ID> --start \"2026-03-10T00:00:00+08:00\" --end \"2026-03-31T23:59:59+08:00\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "eventId", Required: boolPtr(true)},
				{Name: "start", Property: "startTime"},
				{Name: "end", Property: "endTime"},
				{Name: "calendar-id", Property: "calendarId"},
				{Name: "cursor", Property: "cursor"},
				{Name: "limit", Property: "limit", InterfaceType: "integer"},
			},
		},
	})

	// InstancesEvent flags (aligned with event list aliases)
	eventInstancesCmd.Flags().String("id", "", "日程 ID (必填)")
	eventInstancesCmd.Flags().String("event", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("event")
	eventInstancesCmd.Flags().String("event-id", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("event-id")
	eventInstancesCmd.Flags().String("eventId", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("eventId")
	eventInstancesCmd.Flags().String("start", "", "开始时间 ISO-8601 (例如 2026-03-10T00:00:00+08:00)")
	eventInstancesCmd.Flags().String("time-min", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("time-min")
	eventInstancesCmd.Flags().String("min-time", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("min-time")
	eventInstancesCmd.Flags().String("start-time", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("start-time")
	eventInstancesCmd.Flags().String("startTime", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("startTime")
	eventInstancesCmd.Flags().String("start_time", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("start_time")
	eventInstancesCmd.Flags().String("start-date", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("start-date")
	eventInstancesCmd.Flags().String("startDate", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("startDate")
	eventInstancesCmd.Flags().String("end", "", "结束时间 ISO-8601 (例如 2026-03-31T23:59:59+08:00)")
	eventInstancesCmd.Flags().String("time-max", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("time-max")
	eventInstancesCmd.Flags().String("max-time", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("max-time")
	eventInstancesCmd.Flags().String("end-time", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("end-time")
	eventInstancesCmd.Flags().String("endTime", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("endTime")
	eventInstancesCmd.Flags().String("end_time", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("end_time")
	eventInstancesCmd.Flags().String("end-date", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("end-date")
	eventInstancesCmd.Flags().String("endDate", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("endDate")
	eventInstancesCmd.Flags().String("calendar-id", "", "日历 ID (可选，默认 primary 主日历；指定其他日历本时填写，可通过 book list 获取)")
	eventInstancesCmd.Flags().String("calendarId", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("calendarId")
	eventInstancesCmd.Flags().String("calendar", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("calendar")
	eventInstancesCmd.Flags().String("cursor", "", "分页游标 (首次查询无需传入，仅翻页时传入上一次返回的 nextCursor)")
	eventInstancesCmd.Flags().String("next-cursor", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("next-cursor")
	eventInstancesCmd.Flags().String("nextCursor", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("nextCursor")
	eventInstancesCmd.Flags().String("page-token", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("page-token")
	eventInstancesCmd.Flags().String("pageToken", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("pageToken")
	eventInstancesCmd.Flags().String("next-token", "", "")
	_ = eventInstancesCmd.Flags().MarkHidden("next-token")
	eventInstancesCmd.Flags().Int("limit", 0, "每页返回条数 (默认 100，最大 100)")
	eventInstancesCmd.Flags().Int("max-results", 0, "")
	_ = eventInstancesCmd.Flags().MarkHidden("max-results")
	eventInstancesCmd.Flags().Int("maxResults", 0, "")
	_ = eventInstancesCmd.Flags().MarkHidden("maxResults")
	eventInstancesCmd.Flags().Int("page-size", 0, "")
	_ = eventInstancesCmd.Flags().MarkHidden("page-size")
	eventInstancesCmd.Flags().Int("size", 0, "")
	_ = eventInstancesCmd.Flags().MarkHidden("size")
	eventInstancesCmd.Flags().Int("count", 0, "")
	_ = eventInstancesCmd.Flags().MarkHidden("count")

	eventCmd.AddCommand(eventListCmd, eventGetCmd, eventCreateCmd, eventUpdateCmd, eventDeleteCmd, eventSuggestCmd, eventRespondCmd, eventInstancesCmd)

	// participant
	participantCmd.PersistentFlags().String("event", "", "日程 ID (必填)")
	participantCmd.PersistentFlags().String("id", "", "")
	_ = participantCmd.PersistentFlags().MarkHidden("id")
	participantCmd.PersistentFlags().String("event-id", "", "")
	_ = participantCmd.PersistentFlags().MarkHidden("event-id")
	participantCmd.PersistentFlags().String("eventId", "", "")
	_ = participantCmd.PersistentFlags().MarkHidden("eventId")
	participantCmd.PersistentFlags().String("calendar-id", "", "日历 ID (可选，默认 primary 主日历；指定其他日历本时填写，可通过 book list 获取)。注意：订阅日历下的日程无参会人")
	participantCmd.PersistentFlags().String("calendarId", "", "")
	_ = participantCmd.PersistentFlags().MarkHidden("calendarId")
	participantCmd.PersistentFlags().String("calendar", "", "")
	_ = participantCmd.PersistentFlags().MarkHidden("calendar")

	// AddAttendee flags
	participantAddCmd.Flags().String("attendees", "", "参会人 userId 列表，逗号分隔 (必填，最多500人)")
	participantAddCmd.Flags().String("users", "", "")
	_ = participantAddCmd.Flags().MarkHidden("users")
	participantAddCmd.Flags().String("user-ids", "", "")
	_ = participantAddCmd.Flags().MarkHidden("user-ids")
	participantAddCmd.Flags().String("userIds", "", "")
	_ = participantAddCmd.Flags().MarkHidden("userIds")
	participantAddCmd.Flags().String("attendee-ids", "", "")
	_ = participantAddCmd.Flags().MarkHidden("attendee-ids")
	participantAddCmd.Flags().String("attendeeIds", "", "")
	_ = participantAddCmd.Flags().MarkHidden("attendeeIds")
	participantAddCmd.Flags().String("attendee", "", "")
	_ = participantAddCmd.Flags().MarkHidden("attendee")
	participantAddCmd.Flags().String("user", "", "")
	_ = participantAddCmd.Flags().MarkHidden("user")
	participantAddCmd.Flags().String("participants", "", "")
	_ = participantAddCmd.Flags().MarkHidden("participants")
	participantAddCmd.Flags().String("participant-ids", "", "")
	_ = participantAddCmd.Flags().MarkHidden("participant-ids")
	participantAddCmd.Flags().String("participantIds", "", "")
	_ = participantAddCmd.Flags().MarkHidden("participantIds")
	participantAddCmd.Flags().String("members", "", "")
	_ = participantAddCmd.Flags().MarkHidden("members")
	participantAddCmd.Flags().Bool("optional", false, "参会人可选 (默认必选参会人)")

	// DeleteAttendee flags
	participantDeleteCmd.Flags().String("attendees", "", "参会人 userId 列表，逗号分隔 (必填)")
	participantDeleteCmd.Flags().String("users", "", "")
	_ = participantDeleteCmd.Flags().MarkHidden("users")
	participantDeleteCmd.Flags().String("user-ids", "", "")
	_ = participantDeleteCmd.Flags().MarkHidden("user-ids")
	participantDeleteCmd.Flags().String("userIds", "", "")
	_ = participantDeleteCmd.Flags().MarkHidden("userIds")
	participantDeleteCmd.Flags().String("attendee-ids", "", "")
	_ = participantDeleteCmd.Flags().MarkHidden("attendee-ids")
	participantDeleteCmd.Flags().String("attendeeIds", "", "")
	_ = participantDeleteCmd.Flags().MarkHidden("attendeeIds")
	participantDeleteCmd.Flags().String("attendee", "", "")
	_ = participantDeleteCmd.Flags().MarkHidden("attendee")
	participantDeleteCmd.Flags().String("user", "", "")
	_ = participantDeleteCmd.Flags().MarkHidden("user")
	participantDeleteCmd.Flags().String("participants", "", "")
	_ = participantDeleteCmd.Flags().MarkHidden("participants")
	participantDeleteCmd.Flags().String("participant-ids", "", "")
	_ = participantDeleteCmd.Flags().MarkHidden("participant-ids")
	participantDeleteCmd.Flags().String("participantIds", "", "")
	_ = participantDeleteCmd.Flags().MarkHidden("participantIds")
	participantDeleteCmd.Flags().String("members", "", "")
	_ = participantDeleteCmd.Flags().MarkHidden("members")
	participantCmd.AddCommand(participantListCmd, participantAddCmd, participantDeleteCmd)

	// room
	// SearchRoom flags
	roomSearchCmd.Flags().String("start", "", "开始时间 ISO-8601 (可选，不传则默认当前时间+1分钟，例如 2026-03-10T14:00:00+08:00)")
	roomSearchCmd.Flags().String("end", "", "结束时间 ISO-8601 (可选，不传则默认当前时间+1小时，例如 2026-03-10T15:00:00+08:00)")
	roomSearchCmd.Flags().String("time-min", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("time-min")
	roomSearchCmd.Flags().String("time-max", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("time-max")
	roomSearchCmd.Flags().String("min-time", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("min-time")
	roomSearchCmd.Flags().String("max-time", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("max-time")
	roomSearchCmd.Flags().String("start-time", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("start-time")
	roomSearchCmd.Flags().String("end-time", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("end-time")
	roomSearchCmd.Flags().String("startTime", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("startTime")
	roomSearchCmd.Flags().String("endTime", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("endTime")
	roomSearchCmd.Flags().String("start_time", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("start_time")
	roomSearchCmd.Flags().String("end_time", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("end_time")
	roomSearchCmd.Flags().String("start-date", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("start-date")
	roomSearchCmd.Flags().String("end-date", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("end-date")
	roomSearchCmd.Flags().String("startDate", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("startDate")
	roomSearchCmd.Flags().String("endDate", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("endDate")
	// mcp里无此参数
	roomSearchCmd.Flags().Bool("available", false, "仅查空闲会议室")
	_ = roomSearchCmd.Flags().MarkHidden("available")
	roomSearchCmd.Flags().String("group-id", "", "会议室分组ID（可选，留空查根目录；会议室超100条时先用 list-groups 获取分组再按分组查询）")
	roomSearchCmd.Flags().String("room-name", "", "按会议室名称过滤（可选，服务端模糊匹配。调用方需先剔除用户口语后缀如「会议室/大会议室/小会议室」，仅传核心专名以提升命中率，例如用户说「永澄亭会议室」应传「永澄亭」）")
	roomSearchCmd.Flags().String("name", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("name")
	roomSearchCmd.Flags().String("roomName", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("roomName")
	roomSearchCmd.Flags().String("query", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("query")
	roomSearchCmd.Flags().String("limit", "", "页大小 (可选，不填默认 100，超过 100 按 100 处理)")
	roomSearchCmd.Flags().String("page-size", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("page-size")
	roomSearchCmd.Flags().String("size", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("size")
	roomSearchCmd.Flags().String("count", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("count")
	roomSearchCmd.Flags().String("page", "", "分页起始位置 (可选，不填默认 0)")
	roomSearchCmd.Flags().String("page-index", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("page-index")
	roomSearchCmd.Flags().String("page-num", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("page-num")
	roomSearchCmd.Flags().String("page-no", "", "")
	_ = roomSearchCmd.Flags().MarkHidden("page-no")

	// AddRoom flags
	roomAddCmd.Flags().String("event", "", "日程 ID (必填)")
	roomAddCmd.Flags().String("id", "", "")
	_ = roomAddCmd.Flags().MarkHidden("id")
	roomAddCmd.Flags().String("event-id", "", "")
	_ = roomAddCmd.Flags().MarkHidden("event-id")
	roomAddCmd.Flags().String("eventId", "", "")
	_ = roomAddCmd.Flags().MarkHidden("eventId")
	roomAddCmd.Flags().String("rooms", "", "会议室 ID 列表 (必填)")
	roomAddCmd.Flags().String("room-ids", "", "")
	_ = roomAddCmd.Flags().MarkHidden("room-ids")
	roomAddCmd.Flags().String("roomIds", "", "")
	_ = roomAddCmd.Flags().MarkHidden("roomIds")
	roomAddCmd.Flags().String("calendar-id", "", "日历 ID (可选，默认 primary 主日历；指定其他日历本时填写，可通过 book list 获取)，注意：订阅日历下的日程不支持添加会议室")
	roomAddCmd.Flags().String("calendarId", "", "")
	_ = roomAddCmd.Flags().MarkHidden("calendarId")
	roomAddCmd.Flags().String("calendar", "", "")
	_ = roomAddCmd.Flags().MarkHidden("calendar")

	// DeleteRoom flags
	roomDeleteCmd.Flags().String("event", "", "日程 ID (必填)")
	roomDeleteCmd.Flags().String("id", "", "")
	_ = roomDeleteCmd.Flags().MarkHidden("id")
	roomDeleteCmd.Flags().String("event-id", "", "")
	_ = roomDeleteCmd.Flags().MarkHidden("event-id")
	roomDeleteCmd.Flags().String("eventId", "", "")
	_ = roomDeleteCmd.Flags().MarkHidden("eventId")
	roomDeleteCmd.Flags().String("rooms", "", "会议室 ID 列表 (必填)")
	roomDeleteCmd.Flags().String("room-ids", "", "")
	_ = roomDeleteCmd.Flags().MarkHidden("room-ids")
	roomDeleteCmd.Flags().String("roomIds", "", "")
	_ = roomDeleteCmd.Flags().MarkHidden("roomIds")
	roomDeleteCmd.Flags().String("calendar-id", "", "日历 ID (可选，默认 primary 主日历；指定其他日历本时填写，可通过 book list 获取)。注意：订阅日历下的日程无会议室")
	roomDeleteCmd.Flags().String("calendarId", "", "")
	_ = roomDeleteCmd.Flags().MarkHidden("calendarId")
	roomDeleteCmd.Flags().String("calendar", "", "")
	_ = roomDeleteCmd.Flags().MarkHidden("calendar")

	// ListRoomGroups flags
	roomListGroupsCmd.Flags().String("limit", "", "页大小 (可选，不填默认 100，超过 100 按 100 处理)")
	roomListGroupsCmd.Flags().String("page-size", "", "")
	_ = roomListGroupsCmd.Flags().MarkHidden("page-size")
	roomListGroupsCmd.Flags().String("size", "", "")
	_ = roomListGroupsCmd.Flags().MarkHidden("size")
	roomListGroupsCmd.Flags().String("count", "", "")
	_ = roomListGroupsCmd.Flags().MarkHidden("count")
	roomListGroupsCmd.Flags().String("page", "", "分页起始位置 (可选，不填默认 0)")
	roomListGroupsCmd.Flags().String("page-index", "", "")
	_ = roomListGroupsCmd.Flags().MarkHidden("page-index")
	roomListGroupsCmd.Flags().String("page-num", "", "")
	_ = roomListGroupsCmd.Flags().MarkHidden("page-num")
	roomListGroupsCmd.Flags().String("page-no", "", "")
	_ = roomListGroupsCmd.Flags().MarkHidden("page-no")

	roomCmd.AddCommand(roomSearchCmd, roomAddCmd, roomDeleteCmd, roomListGroupsCmd)

	// busy

	// SearchBusy flags
	busySearchCmd.Flags().String("users", "", "用户 ID 列表，逗号分隔 (与 --rooms 至少指定其一)")
	busySearchCmd.Flags().String("rooms", "", "会议室 ID 列表，逗号分隔 (与 --users 至少指定其一，用于查询会议室闲忙)")
	busySearchCmd.Flags().String("user-ids", "", "")
	_ = busySearchCmd.Flags().MarkHidden("user-ids")
	busySearchCmd.Flags().String("userIds", "", "")
	_ = busySearchCmd.Flags().MarkHidden("userIds")
	busySearchCmd.Flags().String("attendees", "", "")
	_ = busySearchCmd.Flags().MarkHidden("attendees")
	busySearchCmd.Flags().String("attendee-ids", "", "")
	_ = busySearchCmd.Flags().MarkHidden("attendee-ids")
	busySearchCmd.Flags().String("attendeeIds", "", "")
	_ = busySearchCmd.Flags().MarkHidden("attendeeIds")
	busySearchCmd.Flags().String("attendee", "", "")
	_ = busySearchCmd.Flags().MarkHidden("attendee")
	busySearchCmd.Flags().String("user", "", "")
	_ = busySearchCmd.Flags().MarkHidden("user")
	busySearchCmd.Flags().String("participants", "", "")
	_ = busySearchCmd.Flags().MarkHidden("participants")
	busySearchCmd.Flags().String("participant-ids", "", "")
	_ = busySearchCmd.Flags().MarkHidden("participant-ids")
	busySearchCmd.Flags().String("participantIds", "", "")
	_ = busySearchCmd.Flags().MarkHidden("participantIds")
	busySearchCmd.Flags().String("members", "", "")
	_ = busySearchCmd.Flags().MarkHidden("members")
	busySearchCmd.Flags().String("room-ids", "", "")
	_ = busySearchCmd.Flags().MarkHidden("room-ids")
	busySearchCmd.Flags().String("roomIds", "", "")
	_ = busySearchCmd.Flags().MarkHidden("roomIds")
	busySearchCmd.Flags().String("start", "", "开始时间 ISO-8601 (必填，例如 2026-03-10T14:00:00+08:00)")
	busySearchCmd.Flags().String("end", "", "结束时间 ISO-8601 (必填，例如 2026-03-10T18:00:00+08:00)")
	busySearchCmd.Flags().String("time-min", "", "")
	_ = busySearchCmd.Flags().MarkHidden("time-min")
	busySearchCmd.Flags().String("time-max", "", "")
	_ = busySearchCmd.Flags().MarkHidden("time-max")
	busySearchCmd.Flags().String("min-time", "", "")
	_ = busySearchCmd.Flags().MarkHidden("min-time")
	busySearchCmd.Flags().String("max-time", "", "")
	_ = busySearchCmd.Flags().MarkHidden("max-time")
	busySearchCmd.Flags().String("start-time", "", "")
	_ = busySearchCmd.Flags().MarkHidden("start-time")
	busySearchCmd.Flags().String("end-time", "", "")
	_ = busySearchCmd.Flags().MarkHidden("end-time")
	busySearchCmd.Flags().String("startTime", "", "")
	_ = busySearchCmd.Flags().MarkHidden("startTime")
	busySearchCmd.Flags().String("endTime", "", "")
	_ = busySearchCmd.Flags().MarkHidden("endTime")
	busySearchCmd.Flags().String("start_time", "", "")
	_ = busySearchCmd.Flags().MarkHidden("start_time")
	busySearchCmd.Flags().String("end_time", "", "")
	_ = busySearchCmd.Flags().MarkHidden("end_time")
	busySearchCmd.Flags().String("start-date", "", "")
	_ = busySearchCmd.Flags().MarkHidden("start-date")
	busySearchCmd.Flags().String("end-date", "", "")
	_ = busySearchCmd.Flags().MarkHidden("end-date")
	busySearchCmd.Flags().String("startDate", "", "")
	_ = busySearchCmd.Flags().MarkHidden("startDate")
	busySearchCmd.Flags().String("endDate", "", "")
	_ = busySearchCmd.Flags().MarkHidden("endDate")

	busyCmd.AddCommand(busySearchCmd)

	// attachment

	// AddAttachment flags
	attachmentAddCmd.Flags().String("event", "", "日程 ID (必填)")
	attachmentAddCmd.Flags().String("id", "", "")
	_ = attachmentAddCmd.Flags().MarkHidden("id")
	attachmentAddCmd.Flags().String("event-id", "", "")
	_ = attachmentAddCmd.Flags().MarkHidden("event-id")
	attachmentAddCmd.Flags().String("eventId", "", "")
	_ = attachmentAddCmd.Flags().MarkHidden("eventId")
	attachmentAddCmd.Flags().String("files", "", "附件列表，格式 <fileId>:<name>，多项逗号分隔 (必填)")
	attachmentAddCmd.Flags().String("calendar-id", "", "日历 ID (可选，默认 primary 主日历；指定其他日历本时填写，可通过 book list 获取)。注意：订阅日历下的日程不支持添加附件")
	attachmentAddCmd.Flags().String("calendarId", "", "")
	_ = attachmentAddCmd.Flags().MarkHidden("calendarId")
	attachmentAddCmd.Flags().String("calendar", "", "")
	_ = attachmentAddCmd.Flags().MarkHidden("calendar")

	attachmentCmd.AddCommand(attachmentAddCmd)

	// book
	// GetBook flags
	bookGetCmd.Flags().String("id", "", "日历 ID (必填，主日历固定为 primary)")
	bookGetCmd.Flags().String("calendar-id", "", "")
	_ = bookGetCmd.Flags().MarkHidden("calendar-id")
	bookGetCmd.Flags().String("calendarId", "", "")
	_ = bookGetCmd.Flags().MarkHidden("calendarId")

	// SearchBook flags
	bookSearchCmd.Flags().String("query", "", "按日历本名称模糊检索 (必填)")
	bookSearchCmd.Flags().String("keyword", "", "")
	_ = bookSearchCmd.Flags().MarkHidden("keyword")
	bookSearchCmd.Flags().String("keywords", "", "")
	_ = bookSearchCmd.Flags().MarkHidden("keywords")
	bookSearchCmd.Flags().String("search", "", "")
	_ = bookSearchCmd.Flags().MarkHidden("search")

	// UpdateBook flags
	bookUpdateCmd.Flags().String("id", "", "日历 ID (必填)")
	bookUpdateCmd.Flags().String("calendar-id", "", "")
	_ = bookUpdateCmd.Flags().MarkHidden("calendar-id")
	bookUpdateCmd.Flags().String("calendarId", "", "")
	_ = bookUpdateCmd.Flags().MarkHidden("calendarId")
	bookUpdateCmd.Flags().String("summary", "", "日历标题")
	bookUpdateCmd.Flags().String("title", "", "")
	_ = bookUpdateCmd.Flags().MarkHidden("title")
	bookUpdateCmd.Flags().String("desc", "", "日历描述")
	bookUpdateCmd.Flags().String("description", "", "")
	_ = bookUpdateCmd.Flags().MarkHidden("description")

	bookCmd.AddCommand(bookListCmd, bookGetCmd, bookSearchCmd, bookUpdateCmd)

	// acl
	// AddAcl flags
	aclAddCmd.Flags().String("user", "", "授予权限的目标用户 ID (必填)")
	aclAddCmd.Flags().String("user-id", "", "")
	_ = aclAddCmd.Flags().MarkHidden("user-id")
	aclAddCmd.Flags().String("userId", "", "")
	_ = aclAddCmd.Flags().MarkHidden("userId")
	aclAddCmd.Flags().String("privilege", "", "授予的日历权限 (必填): free_busy_reader|title_reader|reader|writer")
	aclAddCmd.Flags().Bool("no-notification", false, "不向被授权用户发送提醒 (默认发送)")

	// DeleteAcl flags
	aclDeleteCmd.Flags().String("acl-id", "", "已授予权限的 ID (必填，可通过 acl list 查询)")
	aclDeleteCmd.Flags().String("aclId", "", "")
	_ = aclDeleteCmd.Flags().MarkHidden("aclId")
	aclDeleteCmd.Flags().String("id", "", "")
	_ = aclDeleteCmd.Flags().MarkHidden("id")

	aclCmd.AddCommand(aclListCmd, aclAddCmd, aclDeleteCmd)

	root.AddCommand(eventCmd, participantCmd, roomCmd, busyCmd, attachmentCmd, bookCmd, aclCmd)

	// Install the unknown-verb fallback on every group command. This covers
	// arbitrary typos like `dws calendar room query --min-duration 30` that
	// the per-verb calendarInfoHintSubCmd registrations below can't anticipate.
	for _, g := range []*cobra.Command{root, eventCmd, participantCmd, roomCmd, busyCmd, attachmentCmd, bookCmd, aclCmd} {
		installUnknownVerbFallback(g)
	}
	// Hint subcommands must swallow any extra flags/args the caller passes,
	// otherwise `dws calendar list` prints the nice "ambiguous command" hint
	// but `dws calendar list --start ...` fails earlier with cobra's
	// `unknown flag: --start`. Disabling flag parsing keeps the message
	// consistent regardless of what the user typed after the wrong verb.
	root.AddCommand(
		calendarInfoHintSubCmd("list", "use: 'dws calendar event list' for list events  (or 'dws calendar book list' for list calendars)"),
		calendarInfoHintSubCmd("today", "use: 'dws calendar event list' (defaults to today's schedule)"),
		calendarInfoHintSubCmd("get", "did you mean: 'dws calendar event get'"),
		calendarInfoHintSubCmd("create", "did you mean: 'dws calendar event create'"),
		calendarInfoHintSubCmd("update", "did you mean: 'dws calendar event update'"),
		calendarInfoHintSubCmd("delete", "did you mean: 'dws calendar event delete'"),
		calendarInfoHintSubCmd("suggest", "did you mean: 'dws calendar event suggest'"),
		calendarInfoHintSubCmd("respond", "did you mean: 'dws calendar event respond'"),
		calendarInfoHintSubCmd("add", "use: 'dws calendar attendee add' for attendees (or 'dws calendar room add' for add rooms to event, 'dws calendar attachment add' for add attachments to event)"),
		calendarInfoHintSubCmd("search", "use: 'dws calendar room search' for rooms (or 'dws calendar busy search' for user/room busy status)"),
		calendarInfoHintSubCmd("list-groups", "did you mean: 'dws calendar room list-groups'"),
	)

	return root
}

// meetingRoomDisabledHint is the user-facing hint surfaced when the upstream
// MCP returns errorCode 400056 ("所选组织不支持预定钉钉会议室") on any
// meeting-room related calendar call. Kept calendar-local on purpose so the
// shared business-error mapper in errors.go stays product-agnostic.
const meetingRoomDisabledHint = "当前企业使用的企业自建会议室系统，无法使用dws会议室相关能力（如 dws calendar room search / list-groups ）。请前往客户端中手动完成预订"

// isMeetingRoomDisabledError reports whether the given message is the upstream
// "meeting room not enabled" business error (errorCode 400056).
func isMeetingRoomDisabledError(msg string) bool {
	return strings.Contains(msg, "所选组织不支持预定钉钉会议室") ||
		strings.Contains(msg, `"errorCode": "400056"`) ||
		strings.Contains(msg, `"errorCode":"400056"`)
}

// withMeetingRoomDisabledHint replaces the Suggestion on a meeting-room
// business error with a calendar-specific actionable hint. Non-matching
// errors and non-CLIError types are returned unchanged.
func withMeetingRoomDisabledHint(err error) error {
	if err == nil {
		return nil
	}
	cliErr, ok := err.(*CLIError)
	if !ok {
		return err
	}
	if !isMeetingRoomDisabledError(cliErr.Message) {
		return err
	}
	cliErr.Suggestion = meetingRoomDisabledHint
	return cliErr
}

// callMeetingRoomMCPTool wraps callMCPTool for meeting-room MCP calls whose
// CLI surface is currently `dws calendar room *`. On the upstream
// "meeting room not enabled" business error (errorCode 400056) the generic
// suggestion is replaced with a calendar-specific actionable hint.
func callMeetingRoomMCPTool(toolName string, args map[string]any) error {
	return withMeetingRoomDisabledHint(callMCPTool(toolName, args))
}

// callSearchRoomsByName calls the search_rooms MCP tool (name-only fuzzy search,
// no availability check) and injects a `searchMode` hint so callers know the
// results are not filtered by time availability.
func callSearchRoomsByName(cmd *cobra.Command, toolArgs map[string]any) error {
	raw, err := callMCPToolReturnText(cmd.Context(), "search_rooms", toolArgs)
	if err != nil {
		return withMeetingRoomDisabledHint(err)
	}
	if raw == "" {
		return nil
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		deps.Out.PrintRaw(raw)
		return nil
	}

	// Inject unified hint so callers know this is name-only search (no availability check)
	hint := map[string]any{
		"searchMode": "name-only",
		"hint":       "本次为按名称搜索，未检查会议室可用性。如需查询指定时间段内的可用会议室，请添加 --start/--end 参数。",
	}
	if result, ok := parsed["result"].(map[string]any); ok {
		result["hint"] = hint
	} else {
		parsed["hint"] = hint
	}

	switch deps.Caller.Format() {
	case "json":
		return deps.Out.PrintJSON(parsed)
	case "raw", "table":
		b, err := json.MarshalIndent(parsed, "", "  ")
		if err == nil {
			deps.Out.PrintRaw(string(b))
			return nil
		}
	}
	deps.Out.PrintRaw(raw)
	return nil
}

// callMeetingRoomSearchResult calls query_available_meeting_room and injects a
// `searchRange` hint into the response so callers can see the actual queried
// time window. The room-search CLI silently defaults to now+1min~now+1h when
// --start/--end are omitted (and also auto-corrects a past --start to now+1min),
// which is otherwise invisible to the caller.
func callMeetingRoomSearchResult(cmd *cobra.Command, toolArgs map[string]any, startProvided, endProvided, startCorrected bool) error {
	raw, err := callMCPToolReturnText(cmd.Context(), "query_available_meeting_room", toolArgs)
	if err != nil {
		return withMeetingRoomDisabledHint(err)
	}
	if raw == "" {
		return nil
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		deps.Out.PrintRaw(raw)
		return nil
	}

	// Inject unified hint so callers know this is availability search (time-range based)
	hint := map[string]any{
		"searchMode": "availability",
		"hint":       "本次为按时间段搜索可用会议室，仅返回指定时间范围内可预定的会议室。",
	}
	attachRoomSearchRange(hint, toolArgs, startProvided, endProvided, startCorrected)
	if result, ok := parsed["result"].(map[string]any); ok {
		result["hint"] = hint
	} else {
		parsed["hint"] = hint
	}

	switch deps.Caller.Format() {
	case "json":
		return deps.Out.PrintJSON(parsed)
	case "raw", "table":
		b, err := json.MarshalIndent(parsed, "", "  ")
		if err == nil {
			deps.Out.PrintRaw(string(b))
			return nil
		}
	}
	deps.Out.PrintRaw(raw)
	return nil
}

// attachRoomSearchRange injects the actual queried [startTime, endTime] window
// plus a human-readable hint into the provided hint map, indicating whether
// --start/--end were user-set, defaulted (now+1min ~ now+1h), or auto-corrected
// from a past --start.
func attachRoomSearchRange(hint map[string]any, toolArgs map[string]any, startProvided, endProvided, startCorrected bool) {
	startMs, sOK := toolArgs["startTime"].(int64)
	endMs, eOK := toolArgs["endTime"].(int64)
	if !sOK || !eOK {
		return
	}
	loc := time.Now().Location()

	var rangeHint string
	switch {
	case !startProvided && !endProvided:
		rangeHint = "未显式指定 --start/--end，已默认查询 当前时间+1分钟 起 1 小时内的会议室（CLI 本地时区）。如需其它区间请显式传入 --start 和 --end。"
	case !startProvided:
		rangeHint = "未显式指定 --start，已默认使用 当前时间+1分钟（CLI 本地时区）。"
	case !endProvided:
		rangeHint = "未显式指定 --end，已默认使用 当前时间+1小时（CLI 本地时区）。"
	default:
		rangeHint = "本次查询范围由调用方显式指定。"
	}
	if startCorrected {
		rangeHint += "传入的 --start 早于当前时间，已自动修正为 当前时间+1分钟（API 限制：startTime 必须是未来时间）。"
	}

	hint["searchRange"] = map[string]any{
		"startTime": time.UnixMilli(startMs).In(loc).Format(time.RFC3339),
		"endTime":   time.UnixMilli(endMs).In(loc).Format(time.RFC3339),
		"hint":      rangeHint,
	}
}

func callFilteredBusyStatus(cmd *cobra.Command, toolArgs map[string]any) error {
	raw, err := callMCPToolReturnText(cmd.Context(), "query_busy_status", toolArgs)
	if err != nil {
		return err
	}
	if raw == "" {
		return nil
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		deps.Out.PrintRaw(raw)
		return nil
	}

	// Filter out invalid scheduleItems (a valid item must have a non-null status)
	if results, ok := parsed["result"].([]any); ok {
		for _, entry := range results {
			if m, ok := entry.(map[string]any); ok {
				if items, ok := m["scheduleItems"].([]any); ok {
					filtered := make([]any, 0, len(items))
					for _, item := range items {
						if si, ok := item.(map[string]any); ok {
							if status, exists := si["status"]; exists && status != nil && status != "" {
								filtered = append(filtered, item)
							}
						}
					}
					m["scheduleItems"] = filtered
				}
			}
		}
	}

	switch deps.Caller.Format() {
	case "json":
		return deps.Out.PrintJSON(parsed)
	case "raw", "table":
		b, err := json.MarshalIndent(parsed, "", "  ")
		if err == nil {
			deps.Out.PrintRaw(string(b))
			return nil
		}
	}
	deps.Out.PrintRaw(raw)
	return nil
}

func callSortedCalendarEvents(cmd *cobra.Command, toolName string, toolArgs map[string]any) error {
	if deps.Caller.DryRun() {
		return callMCPTool(toolName, toolArgs)
	}

	raw, err := callMCPToolReturnText(cmd.Context(), toolName, toolArgs)
	if err != nil {
		return err
	}
	if raw == "" {
		return nil
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		deps.Out.PrintRaw(raw)
		return nil
	}

	// Attach searchRange hint so callers know the actual queried time window.
	// `event list` defaults to today's 00:00:00~23:59:59 when --start/--end
	// are omitted, which is easy for callers to miss.
	attachCalendarSearchRange(parsed, cmd, toolArgs)

	result, ok := parsed["result"].(map[string]any)
	if ok {
		if events, ok := result["events"].([]any); ok {
			// Filter out invalid events (a valid event must have an id)
			filtered := make([]any, 0, len(events))
			for _, e := range events {
				if m, ok := e.(map[string]any); ok {
					if id, exists := m["id"]; exists && id != nil && id != "" {
						filtered = append(filtered, e)
					}
				}
			}
			sort.SliceStable(filtered, func(i, j int) bool {
				return calendarEventSortKey(filtered[i]) < calendarEventSortKey(filtered[j])
			})
			result["events"] = filtered
		}
		// Surface pagination hint when nextCursor is present
		if nc, ok := result["nextCursor"].(string); ok && nc != "" {
			result["paginationHint"] = "还有更多日程，使用 --cursor \"" + nc + "\" 获取下一页"
		}
	}

	switch deps.Caller.Format() {
	case "json":
		return deps.Out.PrintJSON(parsed)
	case "raw", "table":
		b, err := json.MarshalIndent(parsed, "", "  ")
		if err == nil {
			deps.Out.PrintRaw(string(b))
			return nil
		}
	}
	deps.Out.PrintRaw(raw)
	return nil
}

// attachCalendarSearchRange injects a `searchRange` object describing the
// time window that was actually sent to MCP, plus a human-readable hint
// indicating whether --start/--end were user-specified or defaulted.
func attachCalendarSearchRange(parsed map[string]any, cmd *cobra.Command, toolArgs map[string]any) {
	startMs, sOK := toolArgs["startTime"].(int64)
	endMs, eOK := toolArgs["endTime"].(int64)
	if !sOK || !eOK {
		return
	}
	loc := time.Now().Location()
	startUserSet := cmd != nil && (cmd.Flags().Changed("start") || cmd.Flags().Changed("time-min") || cmd.Flags().Changed("min-time") || cmd.Flags().Changed("start-time") || cmd.Flags().Changed("startTime") || cmd.Flags().Changed("start_time") || cmd.Flags().Changed("start-date") || cmd.Flags().Changed("startDate"))
	endUserSet := cmd != nil && (cmd.Flags().Changed("end") || cmd.Flags().Changed("time-max") || cmd.Flags().Changed("max-time") || cmd.Flags().Changed("end-time") || cmd.Flags().Changed("endTime") || cmd.Flags().Changed("end_time") || cmd.Flags().Changed("end-date") || cmd.Flags().Changed("endDate"))

	var hint string
	switch {
	case !startUserSet && !endUserSet:
		hint = "未显式指定 --start/--end，已默认查询当天 00:00:00 ~ 23:59:59（CLI 本地时区）。如需其它区间请显式传入 --start 和 --end。"
	case !startUserSet:
		hint = "未显式指定 --start，已默认使用当天 00:00:00（CLI 本地时区）。"
	case !endUserSet:
		hint = "未显式指定 --end，已默认使用当天 23:59:59（CLI 本地时区）。"
	default:
		hint = "本次查询范围由调用方显式指定。"
	}

	searchRange := map[string]any{
		"startTimeISO": time.UnixMilli(startMs).In(loc).Format(time.RFC3339),
		"endTimeISO":   time.UnixMilli(endMs).In(loc).Format(time.RFC3339),
		"hint":         hint,
	}
	if result, ok := parsed["result"].(map[string]any); ok {
		result["searchRange"] = searchRange
		return
	}
	parsed["searchRange"] = searchRange
}

func calendarEventSortKey(event any) int64 {
	m, ok := event.(map[string]any)
	if !ok {
		return 0
	}
	if start, ok := m["start"].(map[string]any); ok {
		if dt, ok := start["dateTime"].(string); ok && dt != "" {
			if t, err := time.Parse(time.RFC3339, dt); err == nil {
				return t.UnixMilli()
			}
		}
	}
	if created, ok := m["created"].(float64); ok {
		return int64(created)
	}
	if updated, ok := m["updated"].(float64); ok {
		return int64(updated)
	}
	return 0
}

// recurrenceFlagNames lists all flattened --recurrence-* CLI flags that together
// compose the single nested `recurrence` MCP parameter ({pattern, range}).
var recurrenceFlagNames = []string{
	"recurrence-type", "recurrence-interval", "recurrence-days-of-week",
	"recurrence-day-of-month", "recurrence-index", "recurrence-first-day-of-week",
	"recurrence-range-type", "recurrence-end-date", "recurrence-count",
}

// calendarRecurrenceParamDecls declares recurrence required/required_when pins
// for create/update event leaves (native_annotation).
func calendarRecurrenceParamDecls() []contract.ParamDecl {
	return []contract.ParamDecl{
		{Name: "recurrence-day-of-month", Property: "recurrence.pattern.dayOfMonth", Required: boolPtr(false), RequiredWhen: "recurrence-type is absoluteMonthly or absoluteYearly"},
		{Name: "recurrence-days-of-week", Property: "recurrence.pattern.daysOfWeek", Required: boolPtr(false), RequiredWhen: "recurrence-type is weekly or relativeMonthly"},
		{Name: "recurrence-index", Property: "recurrence.pattern.index", Required: boolPtr(false), RequiredWhen: "recurrence-type is relativeMonthly"},
		{Name: "recurrence-interval", Property: "recurrence.pattern.interval", Required: boolPtr(false), RequiredWhen: "any recurrence-* flag is provided"},
		{Name: "recurrence-type", Property: "recurrence.pattern.type", Required: boolPtr(false), RequiredWhen: "any recurrence-* flag is provided"},
		{Name: "recurrence-end-date", Property: "recurrence.range.endDate", Required: boolPtr(false), RequiredWhen: "recurrence-range-type is endDate"},
		{Name: "recurrence-count", Property: "recurrence.range.numberOfOccurrences", Required: boolPtr(false), RequiredWhen: "recurrence-range-type is numbered"},
		{Name: "recurrence-range-type", Property: "recurrence.range.type", Required: boolPtr(false), RequiredWhen: "any recurrence-* flag is provided"},
		{Name: "recurrence-first-day-of-week", Property: "recurrence.pattern.firstDayOfWeek"},
	}
}

// recurrenceIncompleteSuggestion is the canonical reminder appended to any
// recurrence validation error. It explains that the CLI flattens a nested
// MCP object and that partial updates are NOT supported by the backend.
const recurrenceIncompleteSuggestion = "CLI 的 --recurrence-* 系列不是彼此独立的参数。\n" +
	"- 创建/修改周期日程时，必须一次性提供**完整**的循环规则（pattern + range），至少包含 --recurrence-type、--recurrence-interval(>0) 与 --recurrence-range-type；\n" +
	"- 修改已有周期日程的任意一个循环字段时，也必须重新提供全部循环字段，否则规则会被覆盖成不完整状态；\n" +
	"- 完整示例：--recurrence-type daily --recurrence-interval 1 --recurrence-range-type numbered --recurrence-count 10"

// buildReminders parses a comma-separated minutes string (e.g. "5,10,15")
// and constructs the MCP `reminders` array: [{"minutes":5},{"minutes":10},{"minutes":15}].
func buildReminders(raw string) []map[string]any {
	parts := parseCSVValues(raw)
	result := make([]map[string]any, 0, len(parts))
	for _, p := range parts {
		minutes, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || minutes <= 0 {
			continue
		}
		result = append(result, map[string]any{"minutes": minutes})
	}
	return result
}

// buildRecurrence constructs the MCP `recurrence` nested object from the
// flattened --recurrence-* flags. When ANY recurrence flag is set, the FULL
// structure must be provided — the CLI refuses to send a partial recurrence
// because MCP does not merge partial values on update.
//
// Returns (nil, nil) when no recurrence flag is set at all.
func buildRecurrence(cmd *cobra.Command) (map[string]any, error) {
	anySet := false
	for _, f := range recurrenceFlagNames {
		if cmd.Flags().Changed(f) {
			anySet = true
			break
		}
	}
	if !anySet {
		return nil, nil
	}

	patternType, _ := cmd.Flags().GetString("recurrence-type")
	rangeType, _ := cmd.Flags().GetString("recurrence-range-type")
	interval, _ := cmd.Flags().GetInt("recurrence-interval")
	if patternType == "" || rangeType == "" || interval <= 0 {
		return nil, &CLIError{
			Code:       CodeMissingParam,
			Message:    "recurrence 结构不完整：--recurrence-type、--recurrence-interval(>0) 与 --recurrence-range-type 必须同时提供",
			Suggestion: recurrenceIncompleteSuggestion,
		}
	}

	validPatternTypes := map[string]bool{
		"daily": true, "weekly": true, "absoluteMonthly": true,
		"relativeMonthly": true, "absoluteYearly": true,
	}
	if !validPatternTypes[patternType] {
		return nil, &CLIError{
			Code:       CodeMCPToolError,
			Message:    "invalid --recurrence-type: " + patternType,
			Suggestion: "可选值: daily | weekly | absoluteMonthly | relativeMonthly | absoluteYearly",
		}
	}
	validRangeTypes := map[string]bool{"noEnd": true, "endDate": true, "numbered": true}
	if !validRangeTypes[rangeType] {
		return nil, &CLIError{
			Code:       CodeMCPToolError,
			Message:    "invalid --recurrence-range-type: " + rangeType,
			Suggestion: "可选值: noEnd(永不结束) | endDate(指定结束时间) | numbered(指定次数)",
		}
	}

	pattern := map[string]any{"type": patternType, "interval": interval}
	daysOfWeek, _ := cmd.Flags().GetString("recurrence-days-of-week")
	if daysOfWeek != "" {
		pattern["daysOfWeek"] = daysOfWeek
	}
	dayOfMonth, _ := cmd.Flags().GetInt("recurrence-day-of-month")
	if dayOfMonth > 0 {
		pattern["dayOfMonth"] = dayOfMonth
	}
	indexVal, _ := cmd.Flags().GetString("recurrence-index")
	if indexVal != "" {
		pattern["index"] = indexVal
	}
	if v, _ := cmd.Flags().GetString("recurrence-first-day-of-week"); v != "" {
		pattern["firstDayOfWeek"] = v
	}

	// Pattern-type-specific required fields
	switch patternType {
	case "weekly", "relativeMonthly":
		if daysOfWeek == "" {
			return nil, &CLIError{
				Code:       CodeMissingParam,
				Message:    "recurrence 不完整：--recurrence-type=" + patternType + " 时必须提供 --recurrence-days-of-week",
				Suggestion: "示例: --recurrence-days-of-week monday,wednesday,friday\n" + recurrenceIncompleteSuggestion,
			}
		}
	}
	switch patternType {
	case "absoluteMonthly", "absoluteYearly":
		if dayOfMonth <= 0 {
			return nil, &CLIError{
				Code:       CodeMissingParam,
				Message:    "recurrence 不完整：--recurrence-type=" + patternType + " 时必须提供 --recurrence-day-of-month",
				Suggestion: "示例: --recurrence-day-of-month 15\n" + recurrenceIncompleteSuggestion,
			}
		}
	}
	if patternType == "relativeMonthly" && indexVal == "" {
		return nil, &CLIError{
			Code:       CodeMissingParam,
			Message:    "recurrence 不完整：--recurrence-type=relativeMonthly 时必须提供 --recurrence-index",
			Suggestion: "可选值: first | second | third | fourth | last\n" + recurrenceIncompleteSuggestion,
		}
	}

	r := map[string]any{"type": rangeType}
	switch rangeType {
	case "endDate":
		v, _ := cmd.Flags().GetString("recurrence-end-date")
		if v == "" {
			return nil, &CLIError{
				Code:       CodeMissingParam,
				Message:    "recurrence 不完整：--recurrence-range-type=endDate 时必须提供 --recurrence-end-date",
				Suggestion: "示例: --recurrence-end-date 2026-12-31T23:59:59+08:00\n" + recurrenceIncompleteSuggestion,
			}
		}
		ms, err := parseISOTimeToMillis("recurrence-end-date", v)
		if err != nil {
			return nil, err
		}
		r["endDate"] = ms
	case "numbered":
		n, _ := cmd.Flags().GetInt("recurrence-count")
		if n <= 0 {
			return nil, &CLIError{
				Code:       CodeMissingParam,
				Message:    "recurrence 不完整：--recurrence-range-type=numbered 时必须提供 --recurrence-count (>0)",
				Suggestion: "示例: --recurrence-count 10\n" + recurrenceIncompleteSuggestion,
			}
		}
		r["numberOfOccurrences"] = n
	case "noEnd":
		// no additional fields required
	}

	return map[string]any{"pattern": pattern, "range": r}, nil
}
