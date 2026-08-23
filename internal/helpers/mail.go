package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// ──────────────────────────────────────────────────────────
// dws mail — 邮箱
// ──────────────────────────────────────────────────────────

var mailRuleAllowedOperations = map[string]map[string]bool{
	"from": {
		"include": true, "exclude": true, "oneof": true, "noneof": true,
	},
	"to": {
		"include": true, "exclude": true, "oneof": true, "noneof": true,
	},
	"subject": {
		"include": true, "exclude": true,
	},
	"attachment": {
		"exist": true,
	},
	"x-aliyun-size": {
		"greater": true, "less": true,
	},
}

var (
	mailHTTPClient    = func() *http.Client { return &http.Client{Timeout: 5 * time.Minute} }
	mailPutAttachment = httpPutMailAttachment
	mailGetAttachment = httpGetMailAttachment
)

func parseMailRuleConditions(raw string) ([]any, error) {
	var conditions []any
	if err := json.Unmarshal([]byte(raw), &conditions); err != nil {
		return nil, fmt.Errorf("--conditions JSON 格式错误: %w", err)
	}
	if err := validateMailRuleConditions(conditions); err != nil {
		return nil, err
	}
	return conditions, nil
}

func validateMailRuleConditions(conditions []any) error {
	for i, raw := range conditions {
		condition, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("--conditions[%d] 必须是对象", i)
		}
		object, _ := condition["object"].(string)
		object = strings.TrimSpace(object)
		allowed, ok := mailRuleAllowedOperations[object]
		if !ok {
			return fmt.Errorf("--conditions[%d].object 不支持 %q；支持: from/to/subject/attachment/x-aliyun-size", i, object)
		}
		orItems, ok := condition["or"].([]any)
		if !ok {
			return fmt.Errorf("--conditions[%d].or 必须是数组", i)
		}
		for j, orRaw := range orItems {
			orItem, ok := orRaw.(map[string]any)
			if !ok {
				return fmt.Errorf("--conditions[%d].or[%d] 必须是对象", i, j)
			}
			andItems, ok := orItem["and"].([]any)
			if !ok {
				return fmt.Errorf("--conditions[%d].or[%d].and 必须是数组", i, j)
			}
			for k, exprRaw := range andItems {
				expr, ok := exprRaw.(map[string]any)
				if !ok {
					return fmt.Errorf("--conditions[%d].or[%d].and[%d] 必须是对象", i, j, k)
				}
				op, _ := expr["operation"].(string)
				op = strings.TrimSpace(op)
				if !allowed[op] {
					return fmt.Errorf("--conditions[%d].or[%d].and[%d].operation=%q 与 object=%q 不匹配；请按 --help 中 object 与 operation 合法组合填写", i, j, k, op, object)
				}
			}
		}
	}
	return nil
}

func newMailCommand() *cobra.Command {
	// Product-level Agent routing Decl (migrated from selection/mail.json
	// products.mail). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "mail",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "管理邮箱、邮件、草稿、附件、文件夹、联系人与邮件模板",
			UseWhen: []string{
				"查收、搜索、阅读、回复、发送或整理邮件",
			},
			AvoidWhen: []string{
				"即时消息用 chat；开放平台应用配置用 dev",
				"公开 catalog 不含 thread trash / sent-message recall 等硬 --yes 命令时不要臆造路径",
			},
		},
	})
	root := &cobra.Command{
		Use:   "mail",
		Short: "邮箱 / 邮件收发",
		Long:  `管理钉钉企业邮箱：查询邮箱地址、搜索邮件、查看邮件、发送邮件、获取会话（thread）、列举文件夹、列举标签。`,
		RunE:  groupRunE,
	}

	mailboxCmd := &cobra.Command{Use: "mailbox", Short: "邮箱地址管理", RunE: groupRunE}

	mailboxListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询可用邮箱地址",
		Long: `查询当前用户绑定的所有邮箱地址。

返回字段：
  emailAccounts  邮箱列表，每条包含邮箱地址(email)、账号类型(type)、所属企业(orgName)

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail mailbox list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("list_user_mailboxes", nil)
		},
	}
	DeclareLeafMetadata(mailboxListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "list_user_mailboxes",
				CanonicalPath:  "mail.list_user_mailboxes",
				CLIPath:        "mail mailbox list",
				PrimaryCLIPath: "mail mailbox list",
			},
			Description: "列出当前登录用户自己的可用邮箱",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "list_user_mailboxes"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "列出当前登录用户自己的可用邮箱",
				UseWhen:      []string{"需要选择当前用户企业邮箱或个人邮箱时"},
				AvoidWhen:    []string{"查找他人的邮箱地址不能使用此命令"},
				Examples:     []string{"dws mail mailbox list"},
			},
		},
	})

	mailboxProfileCmd := &cobra.Command{
		Use:   "profile",
		Short: "获取用户邮箱信息",
		Long: `根据邮箱地址获取用户的邮箱详细信息，包含容量、别名等。

返回字段：
  email          邮箱地址
  emailAliases   邮件地址别名列表
  name           用户名
  nickname       用户昵称
  displayName    用户显示名
  mboxSize       邮箱容量（字节）
  mboxSizeUsed   已使用的邮箱容量（字节）
  createdTime    创建时间
  modifiedTime   修改时间

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail mailbox profile --email user@company.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email"); err != nil {
				return err
			}
			return callMCPTool("get_mailbox_profile", map[string]any{
				"email": mustGetFlag(cmd, "email"),
			})
		},
	}

	mailboxProfileCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")

	mailboxSharedWithMeCmd := &cobra.Command{
		Use:   "shared-with-me",
		Short: "查询共享给我的邮箱",
		Long: `查询他人共享给当前用户的邮箱账号列表，包含共享关系类型。

共享关系（relationships）取值：
  LOGIN           登录（可登录该共享邮箱）
  SEND_AS         代发（以该邮箱身份发送邮件）
  SEND_ON_BEHALF  代表发送（代表该邮箱发送邮件）

返回字段：
  total    可访问的共享账号总数
  targets  可访问的共享账号列表`,
		Example: `  dws mail mailbox shared-with-me
  dws mail mailbox shared-with-me --limit 20 --offset 0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if cmd.Flags().Changed("limit") {
				limit, _ := cmd.Flags().GetInt("limit")
				toolArgs["limit"] = limit
			}
			if cmd.Flags().Changed("offset") {
				offset, _ := cmd.Flags().GetInt("offset")
				toolArgs["offset"] = offset
			}
			if len(toolArgs) == 0 {
				return callMCPTool("list_shared_with_me", nil)
			}
			return callMCPTool("list_shared_with_me", toolArgs)
		},
	}
	DeclareLeafMetadata(mailboxSharedWithMeCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "list_shared_with_me",
				CanonicalPath:  "mail.list_shared_with_me",
				CLIPath:        "mail mailbox shared-with-me",
				PrimaryCLIPath: "mail mailbox shared-with-me",
			},
			Description: "查询共享给我的邮箱",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "list_shared_with_me"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询他人共享给当前用户的邮箱",
				UseWhen:      []string{"需要列出登录/代发/代表发送权限的共享邮箱时"},
				AvoidWhen:    []string{"列出自己邮箱用 mail mailbox list"},
				Examples:     []string{"dws mail mailbox shared-with-me"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "limit", Property: "limit", InterfaceType: "integer"},
				{Name: "offset", Property: "offset", InterfaceType: "integer"},
			},
		},
	})
	mailboxSharedWithMeCmd.Flags().Int("limit", 0, "返回数量上限 (可选)")
	mailboxSharedWithMeCmd.Flags().Int("offset", 0, "偏移量 (可选)")

	mailboxCmd.AddCommand(mailboxListCmd, mailboxProfileCmd, mailboxSharedWithMeCmd)

	messageCmd := &cobra.Command{Use: "message", Short: "邮件管理", RunE: groupRunE}

	messageSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "搜索邮件 (KQL 语法)",
		Long: `使用类 KQL 查询表达式搜索邮件，仅返回邮件 ID 及元信息（不含正文）。

支持的查询字段：
  date, size, tag, folderId, isRead, hasAttachments,
  subject, attachname, body, from, to

常用文件夹 ID：1=已发送, 2=收件箱, 3=垃圾邮件, 5=草稿, 6=已删除

字段说明：
  date        ISO8601 日期时间，支持 > < >= <= 比较运算符
                示例: date>2025-06-01T00:00:00Z
  size        邮件大小（字节），支持 > < >= <= 比较运算符
                示例: size>1024
  folderId    文件夹 ID（整数），必须用数字，不能用文件夹名称
                示例: folderId:2
  isRead      是否已读，布尔值 true/false
                示例: isRead:false
  hasAttachments  是否有附件，布尔值 true/false
                示例: hasAttachments:true
  subject     邮件主题，含空格须加双引号
                示例: subject:周报  subject:"项目 进展"
  body        邮件正文，含空格须加双引号
                示例: body:会议纪要  body:"Q1 总结"
  attachname  附件文件名，含空格须加双引号
                示例: attachname:report.pdf
  from        发件人，支持纯邮件地址、纯名称、"名称<邮件地址>" 格式，含空格须加双引号
                示例: from:alice@company.com  from:"张 三"  from:"alice<a@b.com>"
  to          收件人，支持纯邮件地址、纯名称、"名称<邮件地址>" 格式，含空格须加双引号
                示例: to:bob@company.com  to:"李 四"  to:"alice<a@b.com>"

示例查询：
  date>2025-01-01T00:00:00Z AND (NOT folderId:3) AND (NOT folderId:6)
  (from:"alice") OR (to:"alice<a@b.com>" AND folderId:1)
  subject:"周报" AND hasAttachments:true

返回字段：
  messages    邮件列表，每条包含邮件 ID 及元信息（不含正文）
  total       符合条件的总邮件数
  nextCursor  下一页游标，传入 --cursor 翻页；值为 "$" 表示已到达列表尾部

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail message search --email user@company.com --query "subject:\"周报\"" --limit 20  # 查询邮箱: dws mail mailbox list
  dws mail message search --email user@company.com --query "from:alice AND date>2025-06-01T00:00:00Z" --limit 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "query", "keyword"); err != nil {
				return err
			}
			sizeVal := flagOrFallback(cmd, "limit", "size", "page-size")
			toolArgs := map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"query": flagOrFallback(cmd, "query", "keyword"),
				"size":  sizeVal,
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			return callMCPTool("search_emails", toolArgs)
		},
	}
	DeclareLeafMetadata(messageSearchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "search_emails",
				CanonicalPath:  "mail.search_emails",
				CLIPath:        "mail message search",
				PrimaryCLIPath: "mail message search",
			},
			Description: "用 KQL 搜索邮件，返回 ID 与元信息而非完整正文",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "search_emails"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "用 KQL 搜索邮件，返回 ID 与元信息而非完整正文",
				UseWhen:      []string{"按主题/发件人/时间等条件搜索邮件并提取 messageId"},
				AvoidWhen: []string{
					"已知 messageId 直接看正文时用 mail message get",
					"只列某文件夹邮件且无需复杂条件时优先 mail message list",
				},
				Examples: []string{"dws mail message search --email user@company.com --query \"subject:周报\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "limit", Property: "size"},
			},
		},
	})

	messageListCmd := &cobra.Command{
		Use:   "list",
		Short: "列出文件夹中的邮件",
		Long: `列出指定文件夹中的邮件列表（默认为收件箱）。

底层通过 KQL 查询 folderId 实现，返回邮件 ID 及元信息（不含正文）。

常用文件夹 ID：1=已发送, 2=收件箱, 3=垃圾邮件, 5=草稿, 6=已删除

返回字段：
  messages    邮件列表，每条包含邮件 ID 及元信息（不含正文）
  total       符合条件的总邮件数
  nextCursor  下一页游标，传入 --cursor 翻页；值为 "$" 表示已到达列表尾部

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail message list --email user@company.com  # 默认列出收件箱邮件
  dws mail message list --email user@company.com --folder-id 1  # 列出已发送邮件
  dws mail message list --email user@company.com --folder-id 2 --limit 50
  dws mail message list --email user@company.com --cursor <nextCursor>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email"); err != nil {
				return err
			}
			folderId := flagOrFallback(cmd, "folder-id", "folder")
			query := fmt.Sprintf("folderId:%s", folderId)
			sizeVal := flagOrFallback(cmd, "limit", "size", "page-size")
			toolArgs := map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"query": query,
				"size":  sizeVal,
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			return callMCPTool("search_emails", toolArgs)
		},
	}
	DeclareLeafMetadata(messageListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "list_emails",
				CanonicalPath:  "mail.list_emails",
				CLIPath:        "mail message list",
				PrimaryCLIPath: "mail message list",
			},
			Description: "列出收件箱或指定文件夹中的邮件（无需手写 KQL）",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "CLI wrapper converts the reviewed folder selection into a KQL query and maps pagination before calling the pinned search_emails RPC; the local transform plus RPC call is not a one-to-one interface contract",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "列出收件箱或指定文件夹中的邮件（无需手写 KQL）",
				UseWhen:      []string{"浏览某文件夹邮件列表，且没有复杂搜索条件"},
				AvoidWhen:    []string{"需要主题/发件人/日期等组合条件时用 mail message search"},
				Examples: []string{
					"dws mail message list --email user@company.com",
					"dws mail message list --email user@company.com --folder-id 1 --limit 50",
				},
			},
			// Composite wrapper maps --limit onto search_emails.size (same RPC as
			// mail message search). Declare here so clearing hints does not fall
			// back to flag_name_inference → "limit".
			Parameters: []contract.ParamDecl{
				{Name: "limit", Property: "size"},
			},
		},
	})

	messageGetCmd := &cobra.Command{
		Use:   "get",
		Short: "查看邮件完整内容",
		Long: `根据邮件 ID 获取邮件完整内容，包含正文。

返回字段：
  message  邮件完整信息，包含主题、发件人、收件人、正文、附件等

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail message get --email user@company.com --id <messageId>  # 查询邮箱: dws mail mailbox list; 查询邮件 ID: dws mail message search`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id"); err != nil {
				return err
			}
			return callMCPTool("get_email_by_message_id", map[string]any{
				"email":     mustGetFlag(cmd, "email"),
				"messageId": mustGetFlag(cmd, "id"),
			})
		},
	}
	DeclareLeafMetadata(messageGetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "get_email_by_message_id",
				CanonicalPath:  "mail.get_email_by_message_id",
				CLIPath:        "mail message get",
				PrimaryCLIPath: "mail message get",
			},
			Description: "获取指定邮件的完整正文和元数据",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "get_email_by_message_id"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取指定邮件的完整正文和元数据",
				UseWhen:      []string{"搜索或列表已返回 messageId，需要打开邮件内容时"},
				AvoidWhen:    []string{"只需筛选邮件列表时使用 message list 或 search"},
				Examples:     []string{"dws mail message get --email user@company.com --id <messageId>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "messageId"},
			},
		},
	})

	messageSendCmd := &cobra.Command{
		Use:   "send",
		Short: "发送邮件",
		Long: `发送一封邮件到指定收件人，支持添加普通附件和内联附件（如图片）。

当指定 --attachment 或 --inline-attachment 时，自动执行以下流程：
  1. 创建草稿
  2. 为每个普通附件（isInline=false）创建上传会话并上传文件内容
  3. 为每个内联附件（isInline=true，自动生成 contentId）创建上传会话并上传文件内容
  4. 发送草稿

内联附件说明：
  使用 --inline-attachment 指定内联图片（支持 jpg/jpeg/png/gif/webp/bmp/svg），CLI 自动生成 contentId（格式：inline-{文件名}-{序号}@alimail.com）
  在 --content 中使用占位符 [inline:文件名] 引用内联图片，CLI 自动将正文转为 HTML 并注入 <img> 标签
  若 content 中无对应占位符，内联图片会自动追加到正文末尾
  注意：仅支持图片类型作为内联附件，视频、音频、PDF 等文件请改用 --attachment

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail message send --from user@company.com \
    --to colleague@company.com --subject "周报" --content "本周..."  # 查询邮箱: dws mail mailbox list
  dws mail message send --from user@company.com --to colleague@company.com \
    --subject "周报" --content "见附件" --attachment ./report.pdf
  dws mail message send --from user@company.com --to colleague@company.com \
    --subject "周报" --content "见附件" --attachment ./a.pdf --attachment ./b.xlsx
  dws mail message send --from user@company.com --to colleague@company.com \
    --subject "图表周报" --content "图表如下：[inline:chart.png]" --inline-attachment ./chart.png
  dws mail message send --from user@company.com --to colleague@company.com \
    --subject "带图文档" --content "见附件，图表：[inline:img.png]" --attachment ./doc.pdf --inline-attachment ./img.png`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "from", "sender"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "to", "subject"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "content", "body"); err != nil {
				return err
			}

			// 检查是否有附件（普通附件或内联附件）
			attachments, _ := cmd.Flags().GetStringArray("attachment")
			inlineAttachments, _ := cmd.Flags().GetStringArray("inline-attachment")
			if len(attachments) > 0 || len(inlineAttachments) > 0 {
				return runMailSendWithAttachment(cmd, attachments, inlineAttachments)
			}

			// 无附件：原逻辑
			toolArgs := map[string]any{
				"from":         flagOrFallback(cmd, "from", "sender"),
				"toRecipients": parseRecipients(mustGetFlag(cmd, "to")),
				"subject":      mustGetFlag(cmd, "subject"),
				"body":         flagOrFallback(cmd, "content", "body"),
			}
			if v, _ := cmd.Flags().GetString("cc"); v != "" {
				toolArgs["ccRecipients"] = parseRecipients(v)
			}
			return callMCPTool("send_email", toolArgs)
		},
	}
	DeclareLeafMetadata(messageSendCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "send_email",
				CanonicalPath:  "mail.send_email",
				CLIPath:        "mail message send",
				PrimaryCLIPath: "mail message send",
			},
			Description: "以指定邮箱地址发送一封邮件",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "send_email"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "以指定邮箱地址发送一封邮件",
				UseWhen:      []string{"用户明确要发邮件，且已确认发件人邮箱、收件人、主题与正文"},
				AvoidWhen: []string{
					"只需创建草稿不发送时用 mail draft create",
					"回复/转发已有邮件时用 mail message reply / forward",
				},
				Examples: []string{"dws mail message send --from user@company.com --to colleague@company.com --subject \"周报\" --content \"本周完成...\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cc", Property: "ccRecipients"},
				{Name: "content", Property: "body"},
				{Name: "to", Property: "toRecipients"},
			},
		},
	})

	folderCmd := &cobra.Command{Use: "folder", Short: "邮件文件夹管理", RunE: groupRunE}

	folderListCmd := &cobra.Command{
		Use:   "list",
		Short: "列举邮件文件夹",
		Long: `列出指定邮箱的顶层文件夹或指定父文件夹下的所有子文件夹。
不传 --folder 则返回顶层文件夹，传入则返回该文件夹的子文件夹列表。

返回字段（folders 数组）：
  id                文件夹唯一标识
  displayName       文件夹显示名称
  parentFolderId    父文件夹 ID
  childFolderCount  子文件夹数量
  totalItemCount    邮件总数
  unreadItemCount   未读邮件数量

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail folder list --email user@company.com
  dws mail folder list --email user@company.com --folder <folderId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"email": mustGetFlag(cmd, "email"),
			}
			if v := flagOrFallback(cmd, "folder", "folder-id"); v != "" {
				toolArgs["folderId"] = v
			}
			return callMCPTool("list_folders", toolArgs)
		},
	}
	DeclareLeafMetadata(folderListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "list_folders",
				CanonicalPath:  "mail.list_folders",
				CLIPath:        "mail folder list",
				PrimaryCLIPath: "mail folder list",
			},
			Description: "列出指定邮箱的系统和自定义文件夹",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "list_folders"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "列出指定邮箱的系统和自定义文件夹",
				UseWhen:      []string{"需要取得 folderId 或浏览邮箱目录时"},
				AvoidWhen:    []string{"按文件夹列出邮件时使用 mail message list"},
				Examples:     []string{"dws mail folder list --email user@company.com"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "folder", Property: "folderId"},
			},
		},
	})

	folderListCmd.Flags().String("email", "", "邮件所属邮箱地址 (必填)")
	folderListCmd.Flags().String("folder", "", "父文件夹唯一标识，不传则返回顶层文件夹 (可选)")
	folderListCmd.Flags().String("folder-id", "", "--folder 的别名")
	_ = folderListCmd.Flags().MarkHidden("folder-id")

	folderCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建邮件文件夹",
		Long: `在指定邮箱下创建邮件文件夹。

不传 --folder 时创建顶层文件夹；传入 --folder 时在指定父文件夹下创建子文件夹。
注意：--folder 需要填写父文件夹 ID，不是文件夹名称。父文件夹 ID 可通过 dws mail folder list 获取。

返回字段：
  success        是否成功
  result.folder  创建成功后的文件夹信息

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail folder create --email user@company.com --name "项目资料"
  dws mail folder create --email user@company.com --name "子文件夹" --folder <folderId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "name"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"name":  mustGetFlag(cmd, "name"),
			}
			if v := flagOrFallback(cmd, "folder", "parent-id", "parent-node-id", "parent-folder-id"); v != "" {
				toolArgs["folder"] = v
			}
			return callMCPTool("create_mail_folder", toolArgs)
		},
	}
	DeclareLeafMetadata(folderCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "create_mail_folder",
				CanonicalPath:  "mail.create_mail_folder",
				CLIPath:        "mail folder create",
				PrimaryCLIPath: "mail folder create",
			},
			Description: "创建自定义邮件文件夹",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "create_mail_folder"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建自定义邮件文件夹",
				UseWhen:      []string{"需要在指定邮箱中新建归档文件夹时"},
				AvoidWhen:    []string{"移动邮件到已有文件夹时使用 mail message batch-move"},
				Examples:     []string{"dws mail folder create --email user@company.com --name \"项目资料\""},
			},
		},
	})

	folderDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除邮件文件夹",
		Long: `删除指定邮箱下的邮件文件夹。

注意：--id 需要填写要删除的文件夹 ID，不是文件夹名称。文件夹 ID 可通过 dws mail folder list 获取。

返回字段：
  success  是否成功
  result   删除结果，成功时为空对象

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail folder delete --email user@company.com --id <folderId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id"); err != nil {
				return err
			}
			return callMCPTool("delete_mail_folder", map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"id":    mustGetFlag(cmd, "id"),
			})
		},
	}
	DeclareLeafMetadata(folderDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "delete_mail_folder",
				CanonicalPath:  "mail.delete_mail_folder",
				CLIPath:        "mail folder delete",
				PrimaryCLIPath: "mail folder delete",
			},
			Description: "删除指定邮箱下的邮件文件夹",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "delete_mail_folder"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "删除指定邮箱下的邮件文件夹",
				UseWhen:      []string{"已通过 folder list 拿到文件夹 ID，用户明确要删除该文件夹"},
				AvoidWhen: []string{
					"不要把文件夹名称当作 ID",
					"只重命名时用 mail folder update",
				},
				Examples: []string{"dws mail folder delete --email user@company.com --id <folderId>"},
			},
		},
	})

	folderUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新邮件文件夹",
		Long: `更新指定邮箱下邮件文件夹的名称。

注意：--id 需要填写要更新的文件夹 ID，不是文件夹名称。文件夹 ID 可通过 dws mail folder list 获取。

返回字段：
  success  是否成功
  result   更新结果，成功时为空对象

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail folder update --email user@company.com --id <folderId> --name "新文件夹名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id", "name"); err != nil {
				return err
			}
			return callMCPTool("update_mail_folder", map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"id":    mustGetFlag(cmd, "id"),
				"name":  mustGetFlag(cmd, "name"),
			})
		},
	}
	DeclareLeafMetadata(folderUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "update_mail_folder",
				CanonicalPath:  "mail.update_mail_folder",
				CLIPath:        "mail folder update",
				PrimaryCLIPath: "mail folder update",
			},
			Description: "重命名指定邮件文件夹",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "update_mail_folder"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "重命名指定邮件文件夹",
				UseWhen:      []string{"已知 folderId 并需要修改文件夹名称时"},
				AvoidWhen:    []string{"创建或删除文件夹使用对应命令"},
				Examples:     []string{"dws mail folder update --email user@company.com --id <folderId> --name \"新文件夹名\""},
			},
		},
	})

	folderCreateCmd.Flags().String("email", "", "邮件所属邮箱地址 (必填)")
	folderCreateCmd.Flags().String("name", "", "新建邮件文件夹名称 (必填)")
	folderCreateCmd.Flags().String("folder", "", "父文件夹 ID，不传则创建顶层文件夹 (可选)")
	folderCreateCmd.Flags().String("parent-id", "", "--folder 的别名")
	_ = folderCreateCmd.Flags().MarkHidden("parent-id")
	folderCreateCmd.Flags().String("parent-node-id", "", "--folder 的别名")
	_ = folderCreateCmd.Flags().MarkHidden("parent-node-id")
	folderCreateCmd.Flags().String("parent-folder-id", "", "--folder 的别名")
	_ = folderCreateCmd.Flags().MarkHidden("parent-folder-id")

	folderDeleteCmd.Flags().String("email", "", "邮件所属邮箱地址 (必填)")
	folderDeleteCmd.Flags().String("id", "", "要删除的邮件文件夹 ID (必填)")

	folderUpdateCmd.Flags().String("email", "", "邮件所属邮箱地址 (必填)")
	folderUpdateCmd.Flags().String("id", "", "要更新的邮件文件夹 ID (必填)")
	folderUpdateCmd.Flags().String("name", "", "更新后的邮件文件夹名称 (必填)")

	folderCmd.AddCommand(folderListCmd, folderCreateCmd, folderDeleteCmd, folderUpdateCmd)

	tagCmd := &cobra.Command{Use: "tag", Short: "邮件标签管理", RunE: groupRunE}

	tagListCmd := &cobra.Command{
		Use:   "list",
		Short: "列举邮件标签",
		Long: `列出指定邮箱下的所有邮件标签，返回标签的 ID 和元信息。

返回字段（tags 数组）：
  id                标签唯一标识
  name              标签显示名称
  parentId          父标签 ID
  totalItemCount    标签下邮件总数
  unreadItemCount   标签下未读邮件数量

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail tag list --email user@company.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email"); err != nil {
				return err
			}
			return callMCPTool("list_tags", map[string]any{
				"email": mustGetFlag(cmd, "email"),
			})
		},
	}
	DeclareLeafMetadata(tagListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "list_tags",
				CanonicalPath:  "mail.list_tags",
				CLIPath:        "mail tag list",
				PrimaryCLIPath: "mail tag list",
			},
			Description: "列出指定邮箱可用的邮件标签",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "list_tags"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "列出指定邮箱可用的邮件标签",
				UseWhen:      []string{"需要取得标签 ID 或查看现有标签时"},
				AvoidWhen:    []string{"邮件文件夹目录使用 mail folder list"},
				Examples:     []string{"dws mail tag list --email user@company.com"},
			},
		},
	})

	tagCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建邮件标签",
		Long: `在指定邮箱下创建邮件标签。

不传 --parent-id 时创建顶层标签；传入 --parent-id 时在指定父标签下创建子标签。
注意：--parent-id 需要填写父标签 ID，不是标签名称。父标签 ID 可通过 dws mail tag list 获取。

返回字段：
  success     是否成功
  result.tag  创建成功后的标签信息

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail tag create --email user@company.com --name "项目资料"
  dws mail tag create --email user@company.com --name "子标签" --parent-id <tagId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "name"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"name":  mustGetFlag(cmd, "name"),
			}
			if v := mustGetFlag(cmd, "parent-id"); v != "" {
				toolArgs["parentId"] = v
			}
			return callMCPTool("create_mail_tag", toolArgs)
		},
	}

	tagDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除邮件标签",
		Long: `删除指定邮箱下的邮件标签。

注意：--id 需要填写要删除的标签 ID，不是标签名称。标签 ID 可通过 dws mail tag list 获取。
只能删除用户自定义标签，系统标签不能删除。

返回字段：
  success  是否成功
  result   删除结果，成功时为空对象

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail tag delete --email user@company.com --id <tagId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id"); err != nil {
				return err
			}
			return callMCPTool("delete_mail_tag", map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"id":    mustGetFlag(cmd, "id"),
			})
		},
	}

	tagUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新邮件标签",
		Long: `更新指定邮箱下邮件标签的名称。

注意：--id 需要填写要更新的标签 ID，不是标签名称。标签 ID 可通过 dws mail tag list 获取。
只能更新用户自定义标签，系统标签不能更新。

返回字段：
  success  是否成功
  result   更新结果，成功时为空对象

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail tag update --email user@company.com --id <tagId> --name "新标签名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id", "name"); err != nil {
				return err
			}
			return callMCPTool("update_mail_tag", map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"id":    mustGetFlag(cmd, "id"),
				"name":  mustGetFlag(cmd, "name"),
			})
		},
	}

	tagListCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")

	tagCreateCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	tagCreateCmd.Flags().String("name", "", "新建邮件标签名称 (必填)")
	tagCreateCmd.Flags().String("parent-id", "", "父标签 ID，不传则创建顶层标签 (可选)")

	tagDeleteCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	tagDeleteCmd.Flags().String("id", "", "要删除的邮件标签 ID (必填)")

	tagUpdateCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	tagUpdateCmd.Flags().String("id", "", "要更新的邮件标签 ID (必填)")
	tagUpdateCmd.Flags().String("name", "", "更新后的邮件标签名称 (必填)")

	tagCmd.AddCommand(tagListCmd, tagCreateCmd, tagDeleteCmd, tagUpdateCmd)

	threadCmd := &cobra.Command{Use: "thread", Short: "邮件会话管理", RunE: groupRunE}

	threadListCmd := &cobra.Command{
		Use:   "list",
		Short: "列出邮件会话",
		Long: `列出指定邮箱文件夹下的邮件会话（thread / conversation）。

注意：--folder 需要填写文件夹 ID，不是文件夹名称。文件夹 ID 可通过 dws mail folder list 获取。
--limit 最大 100。首次请求不传 --cursor；翻页时使用上一次返回的 nextCursor。

返回字段（conversations 数组）：
  id                    会话唯一标识
  subject               会话主题
  summary               会话摘要信息
  lastModifiedDateTime  会话最后修改时间
  messageCount          会话邮件数量
  tags                  会话标签 ID 列表
  senders               会话发件人列表（email / name）
  isRead                会话是否已读
  priority              会话重要性
  flag                  会话标识
  hasAttachments        会话是否包含附件
  nextCursor            下一页游标
  hasMore               是否还有更多会话

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail thread list --email user@company.com --folder <folderId> --limit 10  # 查询文件夹: dws mail folder list
  dws mail thread list --email user@company.com --folder 104 --limit 20 --cursor <nextCursor>
  dws mail thread list --email user@company.com --folder 104 --limit 20 --start 2024-01-01T00:00:00Z --end 2024-12-31T23:59:59Z --ascending`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "folder"); err != nil {
				return err
			}
			limit, err := validateMailboxThreadLimit(cmd)
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"email":    mustGetFlag(cmd, "email"),
				"folderId": flagOrFallback(cmd, "folder", "folder-id"),
				"size":     limit,
			}
			if v := mustGetFlag(cmd, "cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			if v := flagOrFallback(cmd, "start", "start-time"); v != "" {
				toolArgs["startTime"] = v
			}
			if v := flagOrFallback(cmd, "end", "end-time"); v != "" {
				toolArgs["endTime"] = v
			}
			if cmd.Flags().Changed("ascending") {
				ascending, _ := cmd.Flags().GetBool("ascending")
				toolArgs["isAscending"] = ascending
			}
			return callMCPTool("list_mailbox_threads", toolArgs)
		},
	}

	threadGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取会话详情",
		Long: `根据会话 ID 获取会话（thread）详情。

返回字段：
  id                    会话唯一标识
  subject               会话主题
  summary               会话摘要信息
  lastModifiedDateTime  会话最后修改时间
  messageCount          会话邮件数量
  tags                  会话 tag 信息
  senders               会话发件人列表（email / name）
  isRead                会话是否已读（全部已读/未读）
  priority              会话重要性，取会话内邮件最高优先级（PRY_HIGH / PRY_NORMAL）
  flag                  会话标识，取会话内最近邮件的标识（FLAG_NONE / FLAG_REPLY / FLAG_FORWARD）
  hasAttachments        会话是否包含附件（不含 inline 资源）

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail thread get --email user@company.com --id <conversationId>  # 查询邮箱: dws mail mailbox list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id"); err != nil {
				return err
			}
			return callMCPTool("get_thread", map[string]any{
				"email":          mustGetFlag(cmd, "email"),
				"conversationId": mustGetFlag(cmd, "id"),
			})
		},
	}
	DeclareLeafMetadata(threadGetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "get_thread",
				CanonicalPath:  "mail.get_thread",
				CLIPath:        "mail thread get",
				PrimaryCLIPath: "mail thread get",
			},
			Description: "获取指定邮件会话的详情",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "get_thread"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取指定邮件会话的详情",
				UseWhen:      []string{"已知 conversationId 并需要查看同一会话中的邮件上下文时"},
				AvoidWhen:    []string{"只有单封 messageId 时使用 mail message get"},
				Examples:     []string{"dws mail thread get --email user@company.com --id <conversationId>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "conversationId"},
			},
		},
	})

	threadUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "修改邮件会话状态",
		Long: `修改单个邮件会话的状态或标签，通过 --action 指定操作类型。支持标记已读、标记未读、添加标签、移除标签。

支持的操作类型（--action）：
  markRead     标记会话为已读
  markUnread   标记会话为未读
  addTags      给会话增加标签，标签 ID 列表通过 --tag-ids 传入
  removeTags   从会话移除标签，标签 ID 列表通过 --tag-ids 传入

注意：--id 需要填写会话 ID，不是邮件 ID。会话 ID 可通过 dws mail thread list 获取。

返回字段：
  success  是否成功
  result   更新结果，成功时为空对象

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail thread update --email user@company.com --id <conversationId> --action markRead
  dws mail thread update --email user@company.com --id <conversationId> --action markUnread
  dws mail thread update --email user@company.com --id <conversationId> --action addTags --tag-ids 1,2
  dws mail thread update --email user@company.com --id <conversationId> --action removeTags --tag-ids 11`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id", "action"); err != nil {
				return err
			}
			action := mustGetFlag(cmd, "action")
			if err := validateMailboxThreadAction(cmd, action); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"email":  mustGetFlag(cmd, "email"),
				"id":     mustGetFlag(cmd, "id"),
				"action": action,
			}
			if v := flagOrFallback(cmd, "tag-ids", "tags"); v != "" {
				toolArgs["tagIds"] = parseRecipients(v)
			}
			return callMCPTool("update_mailbox_thread", toolArgs)
		},
	}

	threadBatchUpdateCmd := &cobra.Command{
		Use:   "batch-update",
		Short: "批量修改邮件会话状态",
		Long: `批量修改邮件会话的状态或标签，通过 --action 指定操作类型。单次最多 100 个会话。支持标记已读、标记未读、添加标签、移除标签。

支持的操作类型（--action）：
  markRead     标记会话为已读
  markUnread   标记会话为未读
  addTags      给会话增加标签，标签 ID 列表通过 --tag-ids 传入
  removeTags   从会话移除标签，标签 ID 列表通过 --tag-ids 传入

注意：--ids 需要填写会话 ID 列表，不是邮件 ID 列表，最多 100 个。会话 ID 可通过 dws mail thread list 获取。

返回字段：
  success  是否成功
  result   批量更新结果，成功时为空对象

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail thread batch-update --email user@company.com --ids <conversationId1>,<conversationId2> --action markRead
  dws mail thread batch-update --email user@company.com --ids <conversationId1>,<conversationId2> --action markUnread
  dws mail thread batch-update --email user@company.com --ids <conversationId1>,<conversationId2> --action addTags --tag-ids 1,2
  dws mail thread batch-update --email user@company.com --ids <conversationId1>,<conversationId2> --action removeTags --tag-ids 11`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "ids", "action"); err != nil {
				return err
			}
			action := mustGetFlag(cmd, "action")
			if err := validateMailboxThreadAction(cmd, action); err != nil {
				return err
			}
			ids := parseRecipients(mustGetFlag(cmd, "ids"))
			if len(ids) > 100 {
				return fmt.Errorf("--ids 最多支持 100 个会话 ID，收到: %d", len(ids))
			}
			toolArgs := map[string]any{
				"email":  mustGetFlag(cmd, "email"),
				"ids":    ids,
				"action": action,
			}
			if v := flagOrFallback(cmd, "tag-ids", "tags"); v != "" {
				toolArgs["tagIds"] = parseRecipients(v)
			}
			return callMCPTool("batch_update_mailbox_threads", toolArgs)
		},
	}

	threadTrashCmd := &cobra.Command{
		Use:   "trash",
		Short: "[危险] 删除邮件会话",
		Long: `[危险] 删除指定邮件会话，将会话移入已删除文件夹。此操作不可撤销，请谨慎执行。

默认需要 --yes 确认才能执行。传入 --yes 跳过确认提示。

注意：--id 需要填写会话 ID，不是邮件 ID。会话 ID 可通过 dws mail thread list 获取。

返回字段：
  success  是否成功
  result   删除结果，成功时为空对象

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail thread trash --email user@company.com --id <conversationId> --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id"); err != nil {
				return err
			}
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes && !commandDryRun(cmd) {
				return fmt.Errorf("此操作将删除会话且不可撤销，请添加 --yes 确认执行")
			}
			return callMCPTool("trash_mailbox_thread", map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"id":    mustGetFlag(cmd, "id"),
			})
		},
	}

	threadBatchTrashCmd := &cobra.Command{
		Use:   "batch-trash",
		Short: "[危险] 批量删除邮件会话",
		Long: `[危险] 批量删除指定邮件会话，将会话移入已删除文件夹。此操作不可撤销，请谨慎执行。

默认需要 --yes 确认才能执行。传入 --yes 跳过确认提示。

注意：--ids 需要填写会话 ID 列表，不是邮件 ID 列表，最多 100 个。会话 ID 可通过 dws mail thread list 获取。

返回字段：
  success  是否成功
  result   批量删除结果，成功时为空对象

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail thread batch-trash --email user@company.com --ids <conversationId1>,<conversationId2> --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "ids"); err != nil {
				return err
			}
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes && !commandDryRun(cmd) {
				return fmt.Errorf("此操作将批量删除会话且不可撤销，请添加 --yes 确认执行")
			}
			ids := parseRecipients(mustGetFlag(cmd, "ids"))
			if len(ids) > 100 {
				return fmt.Errorf("--ids 最多支持 100 个会话 ID，收到: %d", len(ids))
			}
			return callMCPTool("batch_trash_mailbox_threads", map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"ids":   ids,
			})
		},
	}

	threadListCmd.Flags().String("email", "", "会话所属邮箱地址 (必填)")
	threadListCmd.Flags().String("folder", "", "邮件文件夹 ID，不是文件夹名称 (必填)")
	threadListCmd.Flags().String("folder-id", "", "--folder 的别名")
	_ = threadListCmd.Flags().MarkHidden("folder-id")
	threadListCmd.Flags().Int("limit", 0, "本次列出的会话数，最大 100 (必填)")
	threadListCmd.Flags().String("cursor", "", "分页游标，首次请求可不传 (可选)")
	threadListCmd.Flags().String("start", "", "开始 UTC 时间字符串，如 2024-01-01T00:00:00Z (可选)")
	threadListCmd.Flags().String("start-time", "", "--start 的别名")
	_ = threadListCmd.Flags().MarkHidden("start-time")
	threadListCmd.Flags().String("end", "", "结束 UTC 时间字符串，如 2024-12-31T23:59:59Z (可选)")
	threadListCmd.Flags().String("end-time", "", "--end 的别名")
	_ = threadListCmd.Flags().MarkHidden("end-time")
	threadListCmd.Flags().Bool("ascending", false, "是否按时间升序；不传由服务端默认排序 (可选)")

	threadGetCmd.Flags().String("email", "", "会话所属邮箱地址 (必填)")
	threadGetCmd.Flags().String("id", "", "会话唯一标识 conversationId (必填)")

	threadUpdateCmd.Flags().String("email", "", "会话所属邮箱地址 (必填)")
	threadUpdateCmd.Flags().String("id", "", "会话唯一标识 conversationId (必填)")
	threadUpdateCmd.Flags().String("action", "", "操作类型：markRead、markUnread、addTags、removeTags (必填)")
	threadUpdateCmd.Flags().String("tag-ids", "", "标签 ID 列表，多个用英文逗号分隔；addTags/removeTags 时必填 (可选)")
	threadUpdateCmd.Flags().String("tags", "", "--tag-ids 的别名")
	_ = threadUpdateCmd.Flags().MarkHidden("tags")

	threadBatchUpdateCmd.Flags().String("email", "", "会话所属邮箱地址 (必填)")
	threadBatchUpdateCmd.Flags().String("ids", "", "会话 ID 列表，多个用英文逗号分隔，最多 100 个 (必填)")
	threadBatchUpdateCmd.Flags().String("action", "", "操作类型：markRead、markUnread、addTags、removeTags (必填)")
	threadBatchUpdateCmd.Flags().String("tag-ids", "", "标签 ID 列表，多个用英文逗号分隔；addTags/removeTags 时必填 (可选)")
	threadBatchUpdateCmd.Flags().String("tags", "", "--tag-ids 的别名")
	_ = threadBatchUpdateCmd.Flags().MarkHidden("tags")

	threadTrashCmd.Flags().String("email", "", "会话所属邮箱地址 (必填)")
	threadTrashCmd.Flags().String("id", "", "要删除的会话 ID (必填)")
	threadTrashCmd.Flags().Bool("yes", false, "确认执行此危险操作 (必填)")

	threadBatchTrashCmd.Flags().String("email", "", "会话所属邮箱地址 (必填)")
	threadBatchTrashCmd.Flags().String("ids", "", "要删除的会话 ID 列表，多个用英文逗号分隔，最多 100 个 (必填)")
	threadBatchTrashCmd.Flags().Bool("yes", false, "确认执行此危险操作 (必填)")

	threadCmd.AddCommand(threadListCmd, threadGetCmd, threadUpdateCmd, threadBatchUpdateCmd, threadTrashCmd, threadBatchTrashCmd)

	messageVerifyCmd := &cobra.Command{
		Use:   "verify",
		Short: "查询邮件发送状态",
		Long: `根据 internetMessageId 查询邮件的发送状态。

internetMessageId 来源：message send / draft send / message reply / message reply-all / message forward 等发送类命令的返回值。

返回字段：
  message     邮件完整信息
  sendStatus  发送状态，取值如下：
                none             未发送
                posting          投递中
                partial_success  部分成功
                success          发送成功
                failed           发送失败
                unknown          未知

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail message verify --email user@company.com --internet-message-id <internetMessageId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "internet-message-id"); err != nil {
				return err
			}
			return callMCPTool("get_email_by_internet_message_id", map[string]any{
				"email":             mustGetFlag(cmd, "email"),
				"internetMessageId": mustGetFlag(cmd, "internet-message-id"),
			})
		},
	}

	messageReplyCmd := &cobra.Command{
		Use:   "reply",
		Short: "回复邮件",
		Long: `回复指定邮件（仅回复发件人）。

返回字段：
  messageId  新生成的回复邮件 ID

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail message reply --from user@company.com --id <messageId> --subject "Re: 周报" --content "已收到，谢谢！"  # 查询邮件 ID: dws mail message search`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "from", "sender"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}

			attachments, _ := cmd.Flags().GetStringArray("attachment")
			inlineAttachments, _ := cmd.Flags().GetStringArray("inline-attachment")

			toolArgs := map[string]any{
				"from":      flagOrFallback(cmd, "from", "sender"),
				"messageId": mustGetFlag(cmd, "id"),
			}
			body := ""
			if v, _ := cmd.Flags().GetString("to"); v != "" {
				toolArgs["to"] = v
			}
			if v, _ := cmd.Flags().GetString("subject"); v != "" {
				toolArgs["subject"] = v
			}
			if v := flagOrFallback(cmd, "content", "body"); v != "" {
				body = v
				toolArgs["body"] = v
			}
			messageId, err := runMailDraftWithAttachment("create_reply_draft", toolArgs, "", body, attachments, inlineAttachments)
			if err != nil {
				return err
			}
			return callMCPTool("send_draft", map[string]any{
				"email":           flagOrFallback(cmd, "from", "sender"),
				"messageId":       messageId,
				"saveToSentItems": true,
			})
		},
	}
	DeclareLeafMetadata(messageReplyCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "create_reply_draft",
				CanonicalPath:  "mail.create_reply_draft",
				CLIPath:        "mail message reply",
				PrimaryCLIPath: "mail message reply",
			},
			Description: "创建仅回复原发件人的邮件草稿",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "create_reply_draft"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建仅回复原发件人的邮件草稿",
				UseWhen:      []string{"已知原邮件 ID 并要回复发件人时"},
				AvoidWhen:    []string{"需要回复全部参与者时使用 mail message reply-all"},
				Examples:     []string{"dws mail message reply --from user@company.com --id <messageId> --subject \"Re: 周报\" --content \"已收到\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "content", Property: "body"},
				{Name: "id", Property: "messageId"},
			},
		},
	})

	messageReplyAllCmd := &cobra.Command{
		Use:   "reply-all",
		Short: "回复所有人",
		Long: `回复邮件给发件人及所有原始收件人。

返回字段：
  messageId  新生成的回复邮件 ID

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail message reply-all --from user@company.com --id <messageId> --subject "Re: 周报" --content "感谢大家的参与！"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "from", "sender"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}

			attachments, _ := cmd.Flags().GetStringArray("attachment")
			inlineAttachments, _ := cmd.Flags().GetStringArray("inline-attachment")

			toolArgs := map[string]any{
				"from":      flagOrFallback(cmd, "from", "sender"),
				"messageId": mustGetFlag(cmd, "id"),
			}
			body := ""
			if v, _ := cmd.Flags().GetString("to"); v != "" {
				toolArgs["toRecipients"] = parseRecipients(v)
			}
			if v, _ := cmd.Flags().GetString("subject"); v != "" {
				toolArgs["subject"] = v
			}
			if v := flagOrFallback(cmd, "content", "body"); v != "" {
				body = v
				toolArgs["body"] = v
			}
			messageId, err := runMailDraftWithAttachment("create_replyall_draft", toolArgs, "", body, attachments, inlineAttachments)
			if err != nil {
				return err
			}
			return callMCPTool("send_draft", map[string]any{
				"email":           flagOrFallback(cmd, "from", "sender"),
				"messageId":       messageId,
				"saveToSentItems": true,
			})
		},
	}
	DeclareLeafMetadata(messageReplyAllCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "create_replyall_draft",
				CanonicalPath:  "mail.create_replyall_draft",
				CLIPath:        "mail message reply-all",
				PrimaryCLIPath: "mail message reply-all",
			},
			Description: "创建回复原邮件全部参与者的草稿",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "create_replyall_draft"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建回复原邮件全部参与者的草稿",
				UseWhen:      []string{"已知原邮件 ID 且回复需要覆盖所有相关收件人时"},
				AvoidWhen:    []string{"只回复原发件人时使用 mail message reply"},
				Examples:     []string{"dws mail message reply-all --from user@company.com --id <messageId> --subject \"Re: 周报\" --content \"感谢大家\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "content", Property: "body"},
				{Name: "id", Property: "messageId"},
				{Name: "to", Property: "toRecipients"},
			},
		},
	})

	messageForwardCmd := &cobra.Command{
		Use:   "forward",
		Short: "转发邮件",
		Long: `将指定邮件转发给其他收件人。

返回字段：
  messageId  新生成的转发邮件 ID

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail message forward --from user@company.com --to colleague@company.com --id <messageId> --subject "Fwd: 周报"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "from", "sender"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}

			attachments, _ := cmd.Flags().GetStringArray("attachment")
			inlineAttachments, _ := cmd.Flags().GetStringArray("inline-attachment")

			toolArgs := map[string]any{
				"from":      flagOrFallback(cmd, "from", "sender"),
				"messageId": mustGetFlag(cmd, "id"),
			}
			body := ""
			if v, _ := cmd.Flags().GetString("to"); v != "" {
				toolArgs["toRecipients"] = parseRecipients(v)
			}
			if v, _ := cmd.Flags().GetString("subject"); v != "" {
				toolArgs["subject"] = v
			}
			if v := flagOrFallback(cmd, "content", "body"); v != "" {
				body = v
				toolArgs["body"] = v
			}
			messageId, err := runMailDraftWithAttachment("create_forward_draft", toolArgs, "", body, attachments, inlineAttachments)
			if err != nil {
				return err
			}
			return callMCPTool("send_draft", map[string]any{
				"email":           flagOrFallback(cmd, "from", "sender"),
				"messageId":       messageId,
				"saveToSentItems": true,
			})
		},
	}
	DeclareLeafMetadata(messageForwardCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "create_forward_draft",
				CanonicalPath:  "mail.create_forward_draft",
				CLIPath:        "mail message forward",
				PrimaryCLIPath: "mail message forward",
			},
			Description: "基于已有邮件创建转发草稿",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "create_forward_draft"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "基于已有邮件创建转发草稿",
				UseWhen:      []string{"已知原邮件 ID，需要编辑收件人或附言后再发送时"},
				AvoidWhen:    []string{"直接发送全新邮件使用 mail message send"},
				Examples:     []string{"dws mail message forward --from user@company.com --to colleague@company.com --id <messageId> --subject \"Fwd: 周报\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "content", Property: "body"},
				{Name: "id", Property: "messageId"},
				{Name: "to", Property: "toRecipients"},
			},
		},
	})

	messageBatchMoveCmd := &cobra.Command{
		Use:   "batch-move",
		Short: "批量移动邮件到指定文件夹",
		Long: `将多封邮件批量移动到目标文件夹。

常用文件夹 ID：1=已发送, 2=收件箱, 3=垃圾邮件, 5=草稿, 6=已删除

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail message batch-move --email user@company.com --ids <id1>,<id2> --folder 6`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "ids", "folder"); err != nil {
				return err
			}
			return callMCPTool("batch_move_message", map[string]any{
				"email":               mustGetFlag(cmd, "email"),
				"ids":                 parseRecipients(mustGetFlag(cmd, "ids")),
				"destinationFolderId": mustGetFlag(cmd, "folder"),
			})
		},
	}
	DeclareLeafMetadata(messageBatchMoveCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "batch_move_message",
				CanonicalPath:  "mail.batch_move_message",
				CLIPath:        "mail message batch-move",
				PrimaryCLIPath: "mail message batch-move",
			},
			Description: "把多封邮件批量移动到指定文件夹",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "batch_move_message"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "把多封邮件批量移动到指定文件夹",
				UseWhen:      []string{"已知邮件 ID 列表和目标 folderId 时"},
				AvoidWhen:    []string{"永久删除邮件时使用 mail message batch-delete"},
				Examples:     []string{"dws mail message batch-move --email user@company.com --ids <id1>,<id2> --folder 6"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "folder", Property: "destinationFolderId"},
			},
		},
	})

	messageBatchDeleteCmd := &cobra.Command{
		Use:   "batch-delete",
		Short: "批量删除邮件",
		Long: `批量删除指定邮件（移入已删除文件夹或永久删除）。

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail message batch-delete --email user@company.com --ids <id1>,<id2>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "ids"); err != nil {
				return err
			}
			return callMCPTool("batch_delete_message", map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"ids":   parseRecipients(mustGetFlag(cmd, "ids")),
			})
		},
	}
	DeclareLeafMetadata(messageBatchDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "batch_delete_message",
				CanonicalPath:  "mail.batch_delete_message",
				CLIPath:        "mail message batch-delete",
				PrimaryCLIPath: "mail message batch-delete",
			},
			Description: "批量删除邮件（可移入已删除或按类型删除）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "batch_delete_message"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "批量删除邮件（可移入已删除或按类型删除）",
				UseWhen:      []string{"已知多个 messageId，用户明确要求删除这些邮件"},
				AvoidWhen: []string{
					"只需换文件夹时用 mail message batch-move",
					"messageId 未确认时先 search/get",
				},
				Examples: []string{"dws mail message batch-delete --email user@company.com --ids <id1>,<id2>"},
			},
		},
	})

	messageBatchModifyCmd := &cobra.Command{
		Use:   "batch-update",
		Short: "批量修改邮件状态（标记已读/未读/添加标签/移除标签）",
		Long: `批量修改邮件的已读状态或标签，通过 --action 指定操作类型。

支持的操作类型（--action）：
  markRead     标记邮件为已读
  markUnread   标记邮件为未读
  addTags      给邮件增加标签，标签 ID 列表通过 --tags 传入
  removeTags   从邮件移除标签，标签 ID 列表通过 --tags 传入

常用标签 ID：
  1   跟进事项（小红旗）
  2   完成事项（绿色小勾）
  11  重要

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail message batch-update --email user@company.com --ids <id1>,<id2> --action markRead
  dws mail message batch-update --email user@company.com --ids <id1>,<id2> --action addTags --tags 1,2
  dws mail message batch-update --email user@company.com --ids <id1>,<id2> --action removeTags --tags 11`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "ids", "action"); err != nil {
				return err
			}
			action := mustGetFlag(cmd, "action")
			validActions := map[string]bool{
				"markRead": true, "markUnread": true,
				"addTags": true, "removeTags": true,
			}
			if !validActions[action] {
				return fmt.Errorf("--action 必须为 markRead、markUnread、addTags 或 removeTags，收到: %s", action)
			}
			if action == "addTags" || action == "removeTags" {
				if err := validateRequiredFlags(cmd, "tags"); err != nil {
					return err
				}
			}
			toolArgs := map[string]any{
				"email":  mustGetFlag(cmd, "email"),
				"ids":    parseRecipients(mustGetFlag(cmd, "ids")),
				"action": action,
			}
			if v := mustGetFlag(cmd, "tags"); v != "" {
				toolArgs["tags"] = parseRecipients(v)
			}
			return callMCPTool("batch_update_message", toolArgs)
		},
	}

	messageBatchGetCmd := &cobra.Command{
		Use:   "batch-get",
		Short: "批量获取邮件详情",
		Long: `根据邮件 ID 列表批量获取邮件完整内容，包含正文。

限制说明：
  - 单次最多 20 个邮件 ID
  - CLI 会逐个调用 get_email_by_message_id 并返回聚合结果

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail message batch-get --email user@company.com --ids <id1>,<id2>
  dws mail message batch-get --email user@company.com --ids <id1>,<id2>,<id3>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "ids"); err != nil {
				return err
			}
			email := mustGetFlag(cmd, "email")
			ids := parseRecipients(mustGetFlag(cmd, "ids"))
			if len(ids) == 0 {
				return fmt.Errorf("--ids 至少需要 1 个邮件 ID")
			}
			if len(ids) > 20 {
				return fmt.Errorf("--ids 单次最多支持 20 个邮件 ID，当前: %d", len(ids))
			}
			if deps.Caller.DryRun() {
				fmt.Printf("[DRY-RUN] Preview only, not executed:\n")
				fmt.Printf("Operation:    batch-get (loop get_email_by_message_id x%d)\n", len(ids))
				fmt.Printf("Arguments:\n")
				fmt.Printf("  email:      %s\n", email)
				fmt.Printf("  ids:        %v\n", ids)
				return nil
			}
			items := make([]map[string]any, 0, len(ids))
			for _, id := range ids {
				text, err := callMCPToolReturnText(context.Background(), "get_email_by_message_id", map[string]any{
					"email":     email,
					"messageId": id,
				})
				if err != nil {
					return fmt.Errorf("获取邮件详情失败 id=%s: %w", id, err)
				}
				var payload any
				if err := json.Unmarshal([]byte(text), &payload); err != nil {
					payload = text
				}
				items = append(items, map[string]any{
					"id":     id,
					"result": payload,
				})
			}
			return deps.Out.PrintJSON(map[string]any{
				"email":    email,
				"messages": items,
			})
		},
	}

	draftCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建草稿",
		Long: `创建一封邮件草稿，保存到草稿箱（folderId:5）。支持添加普通附件和内联图片。

指定 --attachment 或 --inline-attachment 时，CLI 自动完成草稿创建与附件上传，草稿保留在草稿箱（不发送）。

返回字段：
  messageId  新建草稿的邮件 ID

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail draft create --from user@company.com --to colleague@company.com --subject "草稿标题" --content "草稿正文"
  dws mail draft create --from user@company.com --subject "带附件草稿" --content "见附件" --attachment ./report.pdf
  dws mail draft create --from user@company.com --subject "带图片草稿" --content "图表：[inline:chart.png]" --inline-attachment ./chart.png`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "from", "sender"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "subject"); err != nil {
				return err
			}

			attachments, _ := cmd.Flags().GetStringArray("attachment")
			inlineAttachments, _ := cmd.Flags().GetStringArray("inline-attachment")

			from := flagOrFallback(cmd, "from", "sender")
			body := flagOrFallback(cmd, "content", "body")

			toolArgs := map[string]any{
				"from":    from,
				"subject": mustGetFlag(cmd, "subject"),
			}
			if v, _ := cmd.Flags().GetString("to"); v != "" {
				toolArgs["toRecipients"] = parseRecipients(v)
			}
			if v, _ := cmd.Flags().GetString("cc"); v != "" {
				toolArgs["ccRecipients"] = parseRecipients(v)
			}
			if body != "" {
				toolArgs["body"] = body
			}

			if len(attachments) > 0 || len(inlineAttachments) > 0 {
				msgId, err := runMailDraftWithAttachment("create_draft", toolArgs, "", body, attachments, inlineAttachments)
				if err != nil {
					return err
				}
				return callMCPTool("get_email_by_message_id", map[string]any{
					"email":     from,
					"messageId": msgId,
				})
			}
			return callMCPTool("create_draft", toolArgs)
		},
	}
	DeclareLeafMetadata(draftCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "create_draft",
				CanonicalPath:  "mail.create_draft",
				CLIPath:        "mail draft create",
				PrimaryCLIPath: "mail draft create",
			},
			Description: "创建邮件草稿但不发送",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "create_draft"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建邮件草稿但不发送",
				UseWhen:      []string{"需要先保存收件人、主题、正文或附件供后续编辑时"},
				AvoidWhen:    []string{"用户要求立即发信时使用 mail message send"},
				Examples:     []string{"dws mail draft create --from user@company.com --to colleague@company.com --subject \"草稿标题\" --content \"草稿正文\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cc", Property: "ccRecipients"},
				{Name: "content", Property: "body"},
				{Name: "to", Property: "toRecipients"},
			},
		},
	})

	draftUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新草稿",
		Long: `更新草稿箱中已有草稿的内容，支持添加普通附件和内联图片。

指定 --attachment 或 --inline-attachment 时，CLI 自动完成草稿更新与附件上传，草稿保留在草稿箱（不发送）。

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail draft update --from user@company.com --id <messageId> --subject "新标题" --content "新正文"
  dws mail draft update --from user@company.com --id <messageId> --content "见附件" --attachment ./report.pdf
  dws mail draft update --from user@company.com --id <messageId> --content "图表：[inline:chart.png]" --inline-attachment ./chart.png`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "from", "sender"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}

			attachments, _ := cmd.Flags().GetStringArray("attachment")
			inlineAttachments, _ := cmd.Flags().GetStringArray("inline-attachment")

			from := flagOrFallback(cmd, "from", "sender")
			messageId := mustGetFlag(cmd, "id")
			body := flagOrFallback(cmd, "content", "body")

			toolArgs := map[string]any{
				"from": from,
				"id":   messageId,
			}
			if v, _ := cmd.Flags().GetString("to"); v != "" {
				toolArgs["toRecipients"] = parseRecipients(v)
			}
			if v, _ := cmd.Flags().GetString("cc"); v != "" {
				toolArgs["ccRecipients"] = parseRecipients(v)
			}
			if v, _ := cmd.Flags().GetString("subject"); v != "" {
				toolArgs["subject"] = v
			}
			if body != "" {
				toolArgs["body"] = body
			}

			if len(attachments) > 0 || len(inlineAttachments) > 0 {
				msgId, err := runMailDraftWithAttachment("update_draft", toolArgs, messageId, body, attachments, inlineAttachments)
				if err != nil {
					return err
				}
				return callMCPTool("get_email_by_message_id", map[string]any{
					"email":     from,
					"messageId": msgId,
				})
			}
			return callMCPTool("update_draft", toolArgs)
		},
	}
	DeclareLeafMetadata(draftUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "update_draft",
				CanonicalPath:  "mail.update_draft",
				CLIPath:        "mail draft update",
				PrimaryCLIPath: "mail draft update",
			},
			Description: "更新已有邮件草稿的内容",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "update_draft"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新已有邮件草稿的内容",
				UseWhen:      []string{"已知草稿 messageId 并需要修改主题、正文或收件人时"},
				AvoidWhen:    []string{"已发送邮件不能用草稿更新"},
				Examples:     []string{"dws mail draft update --from user@company.com --id <messageId> --subject \"新标题\" --content \"新正文\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cc", Property: "ccRecipients"},
				{Name: "content", Property: "body"},
				{Name: "to", Property: "toRecipients"},
			},
		},
	})

	attachmentCmd := &cobra.Command{Use: "attachment", Short: "邮件附件管理", RunE: groupRunE}

	attachmentListCmd := &cobra.Command{
		Use:   "list",
		Short: "列举邮件附件",
		Long: `列出指定邮件的所有附件信息。

返回字段（attachments 数组）：
  id            附件唯一标识
  name          附件文件名
  contentType   附件 MIME 类型
  size          附件大小（字节）

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail attachment list --email user@company.com --id <messageId>  # 查询邮件 ID: dws mail message search`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id"); err != nil {
				return err
			}
			return callMCPTool("list_mail_attachments", map[string]any{
				"email":     mustGetFlag(cmd, "email"),
				"messageId": mustGetFlag(cmd, "id"),
			})
		},
	}
	DeclareLeafMetadata(attachmentListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "list_mail_attachments",
				CanonicalPath:  "mail.list_mail_attachments",
				CLIPath:        "mail attachment list",
				PrimaryCLIPath: "mail attachment list",
			},
			Description: "列出指定邮件的全部附件元数据",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "list_mail_attachments"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "列出指定邮件的全部附件元数据",
				UseWhen:      []string{"已知邮件 ID 并需要取得附件 ID、名称或大小时"},
				AvoidWhen:    []string{"实际保存附件时使用 mail attachment download"},
				Examples:     []string{"dws mail attachment list --email user@company.com --id <messageId>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "messageId"},
			},
		},
	})

	attachmentListCmd.Flags().String("email", "", "用户邮箱地址 (必填)")
	attachmentListCmd.Flags().String("id", "", "邮件唯一标识 messageId (必填)")
	attachmentCmd.AddCommand(attachmentListCmd)

	attachmentDownloadCmd := &cobra.Command{
		Use:   "download",
		Short: "下载邮件附件到本地",
		Long: `下载指定邮件的某个附件到本地文件。

流程说明：
  1. 调用 create_download_session 获取下载链接（stream id）
  2. 通过 HTTP GET 下载附件内容并保存到本地

参数说明：
  --email          用户邮箱地址（必填）
  --message-id     邮件唯一标识（必填）
  --attachment-id  附件唯一标识，取自 attachment list 的 id 字段（必填）
  --name           保存到本地的文件名（必填，取自 attachment list 的 name 字段）
  --output         保存目录，默认为当前目录

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  # 先列出附件获取 id 和 name
  dws mail attachment list --email user@company.com --id <messageId>
  # 再下载指定附件
  dws mail attachment download --email user@company.com --message-id <messageId> --attachment-id <attachmentId> --name report.pdf
  dws mail attachment download --email user@company.com --message-id <messageId> --attachment-id <attachmentId> --name img.png --output /tmp`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "message-id", "attachment-id", "name"); err != nil {
				return err
			}
			return runMailAttachmentDownload(cmd)
		},
	}
	DeclareLeafMetadata(attachmentDownloadCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "create_download_session",
				CanonicalPath:  "mail.create_download_session",
				CLIPath:        "mail attachment download",
				PrimaryCLIPath: "mail attachment download",
			},
			Description: "把指定邮件附件下载到本地文件",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "create_download_session"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "把指定邮件附件下载到本地文件",
				UseWhen:      []string{"已知邮件 ID、附件 ID 和文件名，需要保存单个附件时"},
				AvoidWhen:    []string{"只需列出附件元数据时使用 mail attachment list"},
				Examples:     []string{"dws mail attachment download --email user@company.com --message-id <messageId> --attachment-id <attachmentId> --name report.pdf --output ."},
			},
		},
	})

	attachmentDownloadCmd.Flags().String("email", "", "用户邮箱地址 (必填)")
	attachmentDownloadCmd.Flags().String("message-id", "", "邮件唯一标识 messageId (必填)")
	attachmentDownloadCmd.Flags().String("attachment-id", "", "附件唯一标识，取自 attachment list 的 id 字段 (必填)")
	attachmentDownloadCmd.Flags().String("name", "", "保存到本地的文件名，取自 attachment list 的 name 字段 (必填)")
	attachmentDownloadCmd.Flags().String("output", ".", "保存目录，默认为当前目录")
	attachmentCmd.AddCommand(attachmentDownloadCmd)

	messageListCmd.Flags().String("email", "", "邮件所属邮箱地址 (必填)")
	messageListCmd.Flags().String("folder-id", "2", "文件夹 ID（1=已发送, 2=收件箱, 3=垃圾邮件, 5=草稿, 6=已删除），默认为收件箱")
	messageListCmd.Flags().String("folder", "", "--folder-id 的别名")
	_ = messageListCmd.Flags().MarkHidden("folder")
	messageListCmd.Flags().String("limit", "20", "每页返回数量(最大限制 100, 默认 20)")
	messageListCmd.Flags().String("size", "", "--limit 的别名")
	_ = messageListCmd.Flags().MarkHidden("size")
	messageListCmd.Flags().String("page-size", "", "--limit 的别名")
	_ = messageListCmd.Flags().MarkHidden("page-size")
	messageListCmd.Flags().String("cursor", "", "邮件的起始偏移标识, 其值取自响应中的nextCursor字段")

	messageSearchCmd.Flags().String("email", "", "搜索目标邮箱地址 (必填)")
	messageSearchCmd.Flags().String("query", "", "KQL 查询表达式 (必填), 其中 date 格式必须遵循 ISO8601 规范")
	messageSearchCmd.Flags().String("keyword", "", "--query alias")
	_ = messageSearchCmd.Flags().MarkHidden("keyword")
	messageSearchCmd.Flags().String("limit", "20", "每页返回数量(最大限制 100, 默认 20)")
	messageSearchCmd.Flags().String("size", "", "--limit 的别名")
	_ = messageSearchCmd.Flags().MarkHidden("size")
	messageSearchCmd.Flags().String("page-size", "", "--limit 的别名")
	_ = messageSearchCmd.Flags().MarkHidden("page-size")
	messageSearchCmd.Flags().String("cursor", "", "邮件的起始偏移标识, 其值取自响应中的nextCursor字段。\"\"表示从头开始")

	messageGetCmd.Flags().String("email", "", "邮件所属邮箱地址 (必填)")
	messageGetCmd.Flags().String("id", "", "邮件 ID (必填)")

	messageSendCmd.Flags().String("from", "", "发件人邮箱 (必填)")
	messageSendCmd.Flags().String("sender", "", "--from 的别名")
	_ = messageSendCmd.Flags().MarkHidden("sender")
	messageSendCmd.Flags().String("to", "", "收件人列表 (必填)")
	messageSendCmd.Flags().String("subject", "", "邮件标题 (必填)")
	messageSendCmd.Flags().String("content", "", "邮件正文 (必填)")
	messageSendCmd.Flags().String("body", "", "--content 的别名")
	_ = messageSendCmd.Flags().MarkHidden("body")
	messageSendCmd.Flags().String("cc", "", "抄送人列表")
	messageSendCmd.Flags().StringArray("attachment", nil, "附件文件路径，可多次指定 (可选)")
	messageSendCmd.Flags().StringArray("inline-attachment", nil, "内联附件文件路径（如图片），可多次指定，cid 自动生成 (可选)")

	messageReplyCmd.Flags().String("from", "", "发件人邮箱 (必填)")
	messageReplyCmd.Flags().String("sender", "", "--from 的别名")
	_ = messageReplyCmd.Flags().MarkHidden("sender")
	messageReplyCmd.Flags().String("to", "", "收件人列表")
	messageReplyCmd.Flags().String("id", "", "要回复的邮件 ID (必填)")
	messageReplyCmd.Flags().String("subject", "", "回复邮件标题")
	messageReplyCmd.Flags().String("content", "", "回复正文")
	messageReplyCmd.Flags().String("body", "", "--content 的别名")
	_ = messageReplyCmd.Flags().MarkHidden("body")
	messageReplyCmd.Flags().StringArray("attachment", nil, "附件文件路径，可多次指定 (可选)")
	messageReplyCmd.Flags().StringArray("inline-attachment", nil, "内联附件文件路径（如图片），可多次指定，cid 自动生成 (可选)")

	messageReplyAllCmd.Flags().String("from", "", "发件人邮箱 (必填)")
	messageReplyAllCmd.Flags().String("sender", "", "--from 的别名")
	_ = messageReplyAllCmd.Flags().MarkHidden("sender")
	messageReplyAllCmd.Flags().String("to", "", "收件人列表")
	messageReplyAllCmd.Flags().String("id", "", "要回复的邮件 ID (必填)")
	messageReplyAllCmd.Flags().String("subject", "", "回复邮件标题")
	messageReplyAllCmd.Flags().String("content", "", "回复正文")
	messageReplyAllCmd.Flags().String("body", "", "--content 的别名")
	_ = messageReplyAllCmd.Flags().MarkHidden("body")
	messageReplyAllCmd.Flags().StringArray("attachment", nil, "附件文件路径，可多次指定 (可选)")
	messageReplyAllCmd.Flags().StringArray("inline-attachment", nil, "内联附件文件路径（如图片），可多次指定，cid 自动生成 (可选)")

	messageForwardCmd.Flags().String("from", "", "发件人邮箱 (必填)")
	messageForwardCmd.Flags().String("sender", "", "--from 的别名")
	_ = messageForwardCmd.Flags().MarkHidden("sender")
	messageForwardCmd.Flags().String("to", "", "转发收件人列表")
	messageForwardCmd.Flags().String("id", "", "要转发的邮件 ID (必填)")
	messageForwardCmd.Flags().String("subject", "", "转发邮件标题")
	messageForwardCmd.Flags().String("content", "", "转发附言")
	messageForwardCmd.Flags().String("body", "", "--content 的别名")
	_ = messageForwardCmd.Flags().MarkHidden("body")
	messageForwardCmd.Flags().StringArray("attachment", nil, "附件文件路径，可多次指定 (可选)")
	messageForwardCmd.Flags().StringArray("inline-attachment", nil, "内联附件文件路径（如图片），可多次指定，cid 自动生成 (可选)")

	messageBatchMoveCmd.Flags().String("email", "", "邮件所属邮箱地址 (必填)")
	messageBatchMoveCmd.Flags().String("ids", "", "要移动的邮件 ID 列表，逗号分隔 (必填)")
	messageBatchMoveCmd.Flags().String("folder", "", "目标文件夹 ID (必填)")

	messageBatchDeleteCmd.Flags().String("email", "", "邮件所属邮箱地址 (必填)")
	messageBatchDeleteCmd.Flags().String("ids", "", "要删除的邮件 ID 列表，逗号分隔 (必填)")

	messageBatchModifyCmd.Flags().String("email", "", "邮件所属邮箱地址 (必填)")
	messageBatchModifyCmd.Flags().String("ids", "", "要修改的邮件 ID 列表，逗号分隔 (必填)")
	messageBatchModifyCmd.Flags().String("action", "", "操作类型: markRead/markUnread/addTags/removeTags (必填)")
	messageBatchModifyCmd.Flags().String("tags", "", "标签 ID 列表，逗号分隔 (action 为 addTags/removeTags 时必填)")

	messageBatchGetCmd.Flags().String("email", "", "邮件所属邮箱地址 (必填)")
	messageBatchGetCmd.Flags().String("ids", "", "要获取的邮件 ID 列表，逗号分隔，最多 20 个 (必填)")

	messageVerifyCmd.Flags().String("email", "", "邮件所属邮箱地址 (必填)")
	messageVerifyCmd.Flags().String("internet-message-id", "", "邮件的 internetMessageId (必填)，取自发送类命令返回值")

	messageExportCmd := &cobra.Command{
		Use:   "export",
		Short: "导出/备份邮件（EML格式）",
		Long: `导出指定邮件为 EML 格式文件并保存到本地。

不指定 --filename 时，默认以邮件主题作为文件名。
文件保存在当前工作目录下，扩展名为 .eml。
默认不覆盖同名文件，使用 --overwrite 强制覆盖。

注意：目前仅支持 100KB 以内的邮件导出。

编排流程：
  1. 调用 get_email_by_message_id 获取邮件主题（用作默认文件名）
  2. 调用 export_message_mime 获取 EML 内容
  3. 将 EML 内容原子写入本地文件`,
		Example: `  dws mail message export --email user@company.com --id <messageId>
  dws mail message export --email user@company.com --id <messageId> --filename my-mail`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id"); err != nil {
				return err
			}
			email := mustGetFlag(cmd, "email")
			messageID := mustGetFlag(cmd, "id")
			filename := mustGetFlag(cmd, "filename")
			overwrite, _ := cmd.Flags().GetBool("overwrite")

			if deps.Caller.DryRun() {
				// Human plan summary (no "[DRY-RUN]" tag): Schema dry-run
				// evidence classifies "操作:" + audited DryRun() as plan.
				deps.Out.PrintKeyValue("操作", "导出邮件为 EML 文件")
				deps.Out.PrintKeyValue("email", email)
				deps.Out.PrintKeyValue("messageId", messageID)
				if filename != "" {
					deps.Out.PrintKeyValue("filename", filename)
				}
				deps.Out.PrintKeyValue("overwrite", fmt.Sprintf("%v", overwrite))
				deps.Out.PrintKeyValue("编排", "get_email_by_message_id → export_message_mime → 写入本地 .eml 文件")
				return nil
			}

			ctx := cmd.Context()
			if filename == "" {
				msgText, err := callMCPToolReturnText(ctx, "get_email_by_message_id", map[string]any{
					"email":     email,
					"messageId": messageID,
				})
				if err != nil {
					return fmt.Errorf("获取邮件信息失败: %w", err)
				}
				var msgData map[string]any
				if err := json.Unmarshal([]byte(msgText), &msgData); err == nil {
					data := msgData
					if result, ok := data["result"].(map[string]any); ok {
						data = result
					}
					if msg, ok := data["message"].(map[string]any); ok {
						data = msg
					}
					if subj, ok := data["subject"].(string); ok && subj != "" {
						filename = subj
					}
				}
				if filename == "" {
					filename = messageID
				}
			}
			filename = sanitizeMailFilename(filename)

			exportText, err := callMCPToolReturnText(ctx, "export_message_mime", map[string]any{
				"email": email,
				"id":    messageID,
			})
			if err != nil {
				return fmt.Errorf("导出邮件失败: %w", err)
			}
			var exportData map[string]any
			if err := json.Unmarshal([]byte(exportText), &exportData); err != nil {
				return fmt.Errorf("解析导出结果失败: %w", err)
			}
			if result, ok := exportData["result"].(map[string]any); ok {
				exportData = result
			}
			emlContent, _ := exportData["emlContent"].(string)
			if emlContent == "" {
				return fmt.Errorf("导出结果为空: %s", exportText)
			}
			destPath := filename + ".eml"
			if err := atomicWriteFile(destPath, []byte(emlContent), 0600, overwrite); err != nil {
				if os.IsExist(err) {
					return fmt.Errorf("文件 %s 已存在，使用 --overwrite 覆盖", destPath)
				}
				return fmt.Errorf("保存文件失败: %w", err)
			}
			deps.Out.PrintInfo(fmt.Sprintf("邮件已导出到: %s", destPath))
			return nil
		},
	}
	DeclareLeafMetadata(messageExportCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "export_message_mime",
				CanonicalPath:  "mail.export_message_mime",
				CLIPath:        "mail message export",
				PrimaryCLIPath: "mail message export",
			},
			Description: "导出/备份邮件为本地 EML 文件",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Orchestrates get_email_by_message_id + export_message_mime + local file write.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "导出邮件为本地 EML 备份",
				UseWhen:      []string{"需要把单封邮件备份为本地 .eml 文件时"},
				AvoidWhen:    []string{"仅查看正文用 mail message get；分享到 IM 用 mail message share-to-chat"},
				Examples:     []string{"dws mail message export --email user@company.com --id <messageId>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "email", Property: "email", Required: boolPtr(true)},
				{Name: "id", Property: "id", Required: boolPtr(true)},
				{Name: "filename", Property: "filename"},
				{Name: "overwrite", Property: "overwrite", InterfaceType: "boolean"},
			},
		},
	})
	messageExportCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	messageExportCmd.Flags().String("id", "", "邮件ID (必填)")
	messageExportCmd.Flags().String("filename", "", "导出文件名（不含扩展名），默认使用邮件主题")
	messageExportCmd.Flags().Bool("overwrite", false, "是否覆盖同名文件，默认 false")

	messageShareToChatCmd := &cobra.Command{
		Use:   "share-to-chat",
		Short: "[危险] 分享邮件至IM聊天",
		Long: `将指定邮件分享到钉钉单聊。

参数说明：
  --users 目标用户UID列表，逗号分隔（规范名），兼容 --uids
  --yes   确认执行此危险操作 (必填)

默认需要 --yes 确认才能执行；--dry-run 仅预览分享计划，不发起真实请求。
服务端可能返回风险提示（riskMessage）和 sign；在 --yes 已通过的前提下，
将展示风险提示并自动携带 sign 重新请求。`,
		Example: `  dws mail message share-to-chat --email user@company.com --id <messageId> --users uid1,uid2 --yes
  dws mail message share-to-chat --email user@company.com --id <messageId> --users uid1 --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id"); err != nil {
				return err
			}
			mcpArgs := map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"id":    mustGetFlag(cmd, "id"),
			}
			if users := flagOrFallback(cmd, "users", "uids"); users != "" {
				mcpArgs["uids"] = parseRecipients(users)
			}
			yes, _ := cmd.Flags().GetBool("yes")
			if deps.Caller.DryRun() {
				// Human plan summary (no "[DRY-RUN]" tag): Schema dry-run
				// evidence classifies "操作:" + audited DryRun() as plan.
				deps.Out.PrintKeyValue("操作", "分享邮件至 IM 聊天")
				deps.Out.PrintKeyValue("email", mustGetFlag(cmd, "email"))
				deps.Out.PrintKeyValue("messageId", mustGetFlag(cmd, "id"))
				if users := flagOrFallback(cmd, "users", "uids"); users != "" {
					deps.Out.PrintKeyValue("users", users)
				}
				deps.Out.PrintKeyValue("yes", fmt.Sprintf("%v", yes))
				deps.Out.PrintKeyValue("说明", "仅预览分享计划，不发起真实分享请求")
				return nil
			}
			if !commandBoolFlag(cmd, "yes") {
				return apperrors.NewValidation(
					"分享邮件至 IM 为高风险操作；获得用户确认后加 --yes 执行",
					apperrors.WithReason("confirmation_required"),
					apperrors.WithHint("先确认目标用户与邮件内容；用户明确同意后以相同参数追加 --yes"),
					apperrors.WithActions("确认目标用户与邮件", "获得用户确认后使用 --yes 执行"),
				)
			}
			ctx := cmd.Context()
			firstText, err := callMCPToolReturnText(ctx, "share_message_to_chat", mcpArgs)
			if err != nil {
				return fmt.Errorf("分享邮件失败: %w", err)
			}
			var firstResult map[string]any
			if err := json.Unmarshal([]byte(firstText), &firstResult); err != nil {
				return fmt.Errorf("解析分享结果失败: %w", err)
			}
			if result, ok := firstResult["result"].(map[string]any); ok {
				firstResult = result
			}
			if sign, ok := firstResult["sign"].(string); ok && sign != "" {
				if riskMsg, _ := firstResult["riskMessage"].(string); riskMsg != "" {
					deps.Out.PrintInfo(fmt.Sprintf("[风险提示] %s", riskMsg))
				}
				mcpArgs["sign"] = sign
				return callMCPTool("share_message_to_chat", mcpArgs)
			}
			// firstResult is already a parsed object (possibly unwrapped from result).
			return deps.Out.PrintJSON(firstResult)
		},
	}
	DeclareLeafMetadata(messageShareToChatCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "share_message_to_chat",
				CanonicalPath:  "mail.share_message_to_chat",
				CLIPath:        "mail message share-to-chat",
				PrimaryCLIPath: "mail message share-to-chat",
			},
			Description: "分享邮件至 IM 单聊",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "share_message_to_chat"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "把邮件分享到钉钉单聊",
				UseWhen:      []string{"需要将指定邮件分享给钉钉用户（单聊）时"},
				AvoidWhen:    []string{"仅导出本地备份用 mail message export；群聊发消息用 chat message send"},
				Examples:     []string{"dws mail message share-to-chat --email user@company.com --id <messageId> --users uid1"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "email", Property: "email", Required: boolPtr(true)},
				{Name: "id", Property: "id", Required: boolPtr(true)},
				{Name: "users", Property: "uids"},
				{Name: "yes", Property: "yes", InterfaceType: "boolean"},
			},
		},
	})
	messageShareToChatCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	messageShareToChatCmd.Flags().String("id", "", "邮件ID (必填)")
	messageShareToChatCmd.Flags().String("users", "", "目标用户UID列表，逗号分隔")
	messageShareToChatCmd.Flags().String("uids", "", "--users 的别名")
	_ = messageShareToChatCmd.Flags().MarkHidden("uids")
	messageShareToChatCmd.Flags().Bool("yes", false, "确认执行此危险操作 (必填)")

	messageCmd.AddCommand(messageListCmd, messageSearchCmd, messageGetCmd, messageSendCmd,
		messageReplyCmd, messageReplyAllCmd, messageForwardCmd,
		messageBatchMoveCmd, messageBatchDeleteCmd, messageBatchModifyCmd, messageBatchGetCmd, messageVerifyCmd, messageExportCmd, messageShareToChatCmd)

	sentMessageCmd := &cobra.Command{Use: "sent-message", Short: "已发送邮件管理", RunE: groupRunE}

	sentMessageRecallCmd := &cobra.Command{
		Use:   "recall",
		Short: "[危险] 撤回已发送的邮件",
		Long: `撤回已发送的邮件。仅支持撤回同组织内未读邮件。

返回字段：
  id        撤回任务 ID（可用于 recall-detail 查询进度）
  success   接口调用是否成功
  errorCode 错误码（仅失败时存在）
  errorMsg  错误信息（仅失败时存在）

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail sent-message recall --email user@company.com --id <mailId> --subject "邮件主题" --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id", "subject"); err != nil {
				return err
			}
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes && !deps.Caller.DryRun() {
				return fmt.Errorf("此操作为危险操作，需要传入 --yes 确认执行")
			}
			return callMCPTool("recall_sent_message", map[string]any{
				"email":   mustGetFlag(cmd, "email"),
				"id":      mustGetFlag(cmd, "id"),
				"subject": mustGetFlag(cmd, "subject"),
			})
		},
	}

	sentMessageRecallDetailCmd := &cobra.Command{
		Use:   "recall-detail",
		Short: "查询邮件撤回进度",
		Long: `根据撤回任务 ID 查询邮件撤回的详细进度。

撤回任务 ID 来源：sent-message recall 命令返回值中的 id 字段。

返回字段：
  id             撤回任务 ID
  status         任务状态: UNINITED/SUBMITTED/RUNNING/FINISHED/CANCELED/FAILED
  createdTime    创建时间
  updatedTime    更新时间
  totalCount     总邮件数
  succeededCount 成功撤回数
  failedCount    撤回失败数
  details        每封邮件的撤回结果

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail sent-message recall-detail --email user@company.com --id <recallTaskId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id"); err != nil {
				return err
			}
			return callMCPTool("get_recall_detail", map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"id":    mustGetFlag(cmd, "id"),
			})
		},
	}

	sentMessageRecallCmd.Flags().String("email", "", "发件人邮箱地址 (必填)")
	sentMessageRecallCmd.Flags().String("id", "", "要撤回的邮件 ID (必填)")
	sentMessageRecallCmd.Flags().String("subject", "", "邮件主题 (必填)")
	sentMessageRecallCmd.Flags().Bool("yes", false, "跳过确认提示，直接执行")

	sentMessageRecallDetailCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	sentMessageRecallDetailCmd.Flags().String("id", "", "撤回任务 ID (必填)，由 recall 命令返回")

	sentMessageCmd.AddCommand(sentMessageRecallCmd, sentMessageRecallDetailCmd)

	draftCreateCmd.Flags().String("from", "", "发件人邮箱 (必填)")
	draftCreateCmd.Flags().String("sender", "", "--from 的别名")
	_ = draftCreateCmd.Flags().MarkHidden("sender")
	draftCreateCmd.Flags().String("to", "", "收件人列表")
	draftCreateCmd.Flags().String("cc", "", "抄送人列表")
	draftCreateCmd.Flags().String("subject", "", "邮件标题 (必填)")
	draftCreateCmd.Flags().String("content", "", "邮件正文")
	draftCreateCmd.Flags().String("body", "", "--content 的别名")
	_ = draftCreateCmd.Flags().MarkHidden("body")
	draftCreateCmd.Flags().StringArray("attachment", nil, "附件文件路径，可多次指定 (可选)")
	draftCreateCmd.Flags().StringArray("inline-attachment", nil, "内联附件文件路径（如图片），可多次指定，cid 自动生成 (可选)")

	draftUpdateCmd.Flags().String("from", "", "发件人邮箱 (必填)")
	draftUpdateCmd.Flags().String("sender", "", "--from 的别名")
	_ = draftUpdateCmd.Flags().MarkHidden("sender")
	draftUpdateCmd.Flags().String("id", "", "草稿邮件 ID (必填)")
	draftUpdateCmd.Flags().String("to", "", "收件人列表")
	draftUpdateCmd.Flags().String("cc", "", "抄送人列表")
	draftUpdateCmd.Flags().String("subject", "", "邮件标题")
	draftUpdateCmd.Flags().String("content", "", "邮件正文")
	draftUpdateCmd.Flags().String("body", "", "--content 的别名")
	_ = draftUpdateCmd.Flags().MarkHidden("body")
	draftUpdateCmd.Flags().StringArray("attachment", nil, "附件文件路径，可多次指定 (可选)")
	draftUpdateCmd.Flags().StringArray("inline-attachment", nil, "内联附件文件路径（如图片），可多次指定，cid 自动生成 (可选)")

	draftSendCmd := &cobra.Command{
		Use:   "send",
		Short: "发送草稿",
		Long: `将草稿箱中已有的草稿发送出去。

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail draft send --from user@company.com --id <messageId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "from", "sender"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}
			return callMCPTool("send_draft", map[string]any{
				"email":           flagOrFallback(cmd, "from", "sender"),
				"messageId":       mustGetFlag(cmd, "id"),
				"saveToSentItems": true,
			})
		},
	}
	DeclareLeafMetadata(draftSendCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "send_draft",
				CanonicalPath:  "mail.send_draft",
				CLIPath:        "mail draft send",
				PrimaryCLIPath: "mail draft send",
			},
			Description: "发送草稿箱中已有的邮件草稿",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "send_draft"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "发送草稿箱中已有的邮件草稿",
				UseWhen:      []string{"已知草稿 messageId 且用户要求发送该草稿时"},
				AvoidWhen:    []string{"创建或编辑草稿分别使用 draft create 和 draft update"},
				Examples:     []string{"dws mail draft send --from user@company.com --id <messageId>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "from", Property: "email"},
				{Name: "id", Property: "messageId"},
			},
		},
	})

	draftSendCmd.Flags().String("from", "", "发件人邮箱 (必填)，别名: --sender")
	draftSendCmd.Flags().String("sender", "", "--from 的别名")
	_ = draftSendCmd.Flags().MarkHidden("sender")
	draftSendCmd.Flags().String("id", "", "草稿邮件 ID (必填)")

	draftCmd := &cobra.Command{Use: "draft", Short: "草稿管理", RunE: groupRunE}
	draftCmd.AddCommand(draftCreateCmd, draftUpdateCmd, draftSendCmd)

	userCmd := &cobra.Command{Use: "user", Short: "邮箱用户管理", RunE: groupRunE}

	userSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "搜索邮箱用户",
		Long: `按关键词或工号搜索邮箱用户，返回匹配的用户列表及分页游标。

注意：仅企业邮箱（非 @dingtalk.com 个人邮箱）可使用此功能；
使用个人邮箱（如 xxx@dingtalk.com）调用将因无权限而报错。

搜索方式（二选一）：
  --keyword      按姓名/关键词搜索（当未提供 --employee-no 时为必填）
  --employee-no  按工号精确搜索；提供工号时 keyword 不再必填

返回字段：
  users       匹配的用户列表，每条包含用户 ID、邮箱地址、姓名、昵称、工号、职位、工作地
  nextCursor  下一页游标，传入 --cursor 翻页
  hasMore     是否还有更多数据

user 对象字段：
  id            用户 ID
  email         展示使用的邮件地址
  name          用户名（人名）
  nickname      用户昵称（或者花名）
  employeeNo    工号
  jobTitle      职位
  workLocation  工作地

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail user search --keyword "张三"
  dws mail user search --email user@company.com --keyword "alice"
  dws mail user search --email user@company.com --keyword "alice" --limit 10
  dws mail user search --email user@company.com --keyword "alice" --cursor <nextCursor>
  dws mail user search --email user@company.com --employee-no "E123456"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			keyword := mustGetFlag(cmd, "keyword")
			employeeNo := mustGetFlag(cmd, "employee-no")
			if keyword == "" && employeeNo == "" {
				return fmt.Errorf("--keyword 与 --employee-no 至少需要提供一个")
			}
			toolArgs := map[string]any{}
			if keyword != "" {
				toolArgs["keyword"] = keyword
			}
			if employeeNo != "" {
				toolArgs["employeeNo"] = employeeNo
			}
			if v, _ := cmd.Flags().GetString("email"); v != "" {
				toolArgs["email"] = v
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			if v := flagOrFallback(cmd, "limit", "size"); v != "" {
				toolArgs["size"] = v
			}
			return callMCPTool("search_mail_users", toolArgs)
		},
	}
	DeclareLeafMetadata(userSearchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "search_mail_users",
				CanonicalPath:  "mail.search_mail_users",
				CLIPath:        "mail user search",
				PrimaryCLIPath: "mail user search",
			},
			Description: "按姓名、工号或关键词搜索企业邮箱用户",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "search_mail_users"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "按姓名、工号或关键词搜索企业邮箱用户",
				UseWhen:      []string{"需要查找他人的企业邮箱地址时"},
				AvoidWhen:    []string{"列出当前用户自己的邮箱使用 mail mailbox list，搜索邮件内容使用 message search"},
				Examples:     []string{"dws mail user search --email user@company.com --keyword \"张三\" --limit 20"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "employee-no", Required: boolPtr(false)},
				{Name: "keyword", Required: boolPtr(false)},
				{Name: "limit", Property: "size"},
			},
		},
	})

	userSearchCmd.Flags().String("email", "", "搜索目标邮箱地址 (可选)")
	userSearchCmd.Flags().String("keyword", "", "搜索关键词（未提供 --employee-no 时为必填）")
	userSearchCmd.Flags().String("employee-no", "", "按工号搜索用户；提供此参数时 keyword 不再必填")
	userSearchCmd.Flags().String("cursor", "", "分页游标，取自响应中的 nextCursor 字段")
	userSearchCmd.Flags().String("limit", "", "每页返回数量")
	userSearchCmd.Flags().String("size", "", "--limit 的别名")
	_ = userSearchCmd.Flags().MarkHidden("size")
	userCmd.AddCommand(userSearchCmd)

	// ── template 子命令组 ──────────────────────────────────

	templateCmd := &cobra.Command{Use: "template", Short: "邮件模板管理", RunE: groupRunE}

	templateCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建邮件模板",
		Long: `创建一个新的邮件模板。

返回字段：
  模板创建结果

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail template create --email user@company.com --from user@company.com --name "周报模板" --subject "周报" --content "本周工作总结..."
  dws mail template create --email user@company.com --from user@company.com --name "通知模板" --subject "通知" --content "..." --to a@x.com,b@x.com --cc c@x.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "subject", "name"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "content", "body"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"email":   mustGetFlag(cmd, "email"),
				"subject": mustGetFlag(cmd, "subject"),
				"body":    flagOrFallback(cmd, "content", "body"),
				"name":    mustGetFlag(cmd, "name"),
			}
			if v, _ := cmd.Flags().GetString("from"); v != "" {
				toolArgs["from"] = v
			}
			if v, _ := cmd.Flags().GetString("to"); v != "" {
				toolArgs["toRecipients"] = parseRecipients(v)
			}
			if v, _ := cmd.Flags().GetString("cc"); v != "" {
				toolArgs["ccRecipients"] = parseRecipients(v)
			}
			isDraft, _ := cmd.Flags().GetBool("is-draft")
			toolArgs["isDraft"] = isDraft
			return callMCPTool("create_user_message_template", toolArgs)
		},
	}
	DeclareLeafMetadata(templateCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "create_user_message_template",
				CanonicalPath:  "mail.create_user_message_template",
				CLIPath:        "mail template create",
				PrimaryCLIPath: "mail template create",
			},
			Description: "创建可复用的邮件模板",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "create_user_message_template"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建可复用的邮件模板",
				UseWhen:      []string{"需要保存固定主题、正文或收件人供重复使用时"},
				AvoidWhen:    []string{"只创建一次性草稿时使用 mail draft create"},
				Examples:     []string{"dws mail template create --email user@company.com --from user@company.com --name \"周报模板\" --subject \"周报\" --content \"本周工作总结\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cc", Property: "ccRecipients"},
				{Name: "content", Property: "body"},
				{Name: "to", Property: "toRecipients"},
			},
		},
	})

	templateCreateCmd.Flags().String("email", "", "用户邮箱地址 (必填)")
	templateCreateCmd.Flags().String("from", "", "模板发件人邮箱 (可选)")
	templateCreateCmd.Flags().String("subject", "", "模板邮件标题 (必填)")
	templateCreateCmd.Flags().String("content", "", "模板邮件正文 (必填)")
	templateCreateCmd.Flags().String("body", "", "--content 的别名")
	_ = templateCreateCmd.Flags().MarkHidden("body")
	templateCreateCmd.Flags().String("name", "", "模板名称 (必填)")
	templateCreateCmd.Flags().String("to", "", "模板收件人列表，逗号分隔 (可选)")
	templateCreateCmd.Flags().String("cc", "", "模板抄送人列表，逗号分隔 (可选)")
	templateCreateCmd.Flags().Bool("is-draft", false, "是否为草稿模板 (可选，默认 false；仅草稿模板后续可 template update)")

	templateListCmd := &cobra.Command{
		Use:   "list",
		Short: "列举邮件模板",
		Long: `列出指定邮箱的所有邮件模板。

返回字段：
  模板列表及分页信息

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail template list --email user@company.com --limit 20
  dws mail template list --email user@company.com --limit 20 --cursor <nextCursor>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "limit", "size"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "email"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"size":  flagOrFallback(cmd, "limit", "size"),
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			return callMCPTool("list_user_message_templates", toolArgs)
		},
	}
	DeclareLeafMetadata(templateListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "list_user_message_templates",
				CanonicalPath:  "mail.list_user_message_templates",
				CLIPath:        "mail template list",
				PrimaryCLIPath: "mail template list",
			},
			Description: "分页列出当前邮箱的邮件模板",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "list_user_message_templates"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "分页列出当前邮箱的邮件模板",
				UseWhen:      []string{"需要浏览模板或取得 templateId 时"},
				AvoidWhen:    []string{"查看单个模板详情时使用 mail template get"},
				Examples:     []string{"dws mail template list --email user@company.com --limit 20"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "limit", Property: "size"},
			},
		},
	})

	templateListCmd.Flags().String("email", "", "用户邮箱地址 (必填)")
	templateListCmd.Flags().String("cursor", "", "分页游标，取自响应中的 nextCursor 字段 (可选)")
	templateListCmd.Flags().String("limit", "", "每页返回数量 (必填)")
	templateListCmd.Flags().String("size", "", "--limit 的别名")
	_ = templateListCmd.Flags().MarkHidden("size")

	templateGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取邮件模板详情",
		Long: `根据模板 ID 获取邮件模板的完整信息。

返回字段：
  模板完整信息

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail template get --email user@company.com --id <templateId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id"); err != nil {
				return err
			}
			return callMCPTool("get_user_message_template", map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"id":    mustGetFlag(cmd, "id"),
			})
		},
	}
	DeclareLeafMetadata(templateGetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "get_user_message_template",
				CanonicalPath:  "mail.get_user_message_template",
				CLIPath:        "mail template get",
				PrimaryCLIPath: "mail template get",
			},
			Description: "获取指定邮件模板的完整内容",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "get_user_message_template"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取指定邮件模板的完整内容",
				UseWhen:      []string{"已知 templateId 并需要查看主题、正文或收件人时"},
				AvoidWhen:    []string{"需要浏览所有模板时使用 mail template list"},
				Examples:     []string{"dws mail template get --email user@company.com --id <templateId>"},
			},
		},
	})

	templateGetCmd.Flags().String("email", "", "用户邮箱地址 (必填)")
	templateGetCmd.Flags().String("id", "", "模板唯一标识 (必填)")

	templateUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新邮件模板",
		Long: `更新已有邮件模板的内容。仅传入需要更新的字段即可。

注意: 邮箱服务端仅支持更新草稿模板 (创建时带 --is-draft)；
非草稿模板不可修改 (服务端返回 Invalid parameter)，只能删除后重建。

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail template update --email user@company.com --id <templateId> --subject "新标题" --content "新正文"
  dws mail template update --email user@company.com --id <templateId> --name "新模板名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"id":    mustGetFlag(cmd, "id"),
			}
			if v, _ := cmd.Flags().GetString("from"); v != "" {
				toolArgs["from"] = v
			}
			if v, _ := cmd.Flags().GetString("subject"); v != "" {
				toolArgs["subject"] = v
			}
			if v := flagOrFallback(cmd, "content", "body"); v != "" {
				toolArgs["body"] = v
			}
			if v, _ := cmd.Flags().GetString("name"); v != "" {
				toolArgs["name"] = v
			}
			if v, _ := cmd.Flags().GetString("to"); v != "" {
				toolArgs["toRecipients"] = parseRecipients(v)
			}
			if v, _ := cmd.Flags().GetString("cc"); v != "" {
				toolArgs["ccRecipients"] = parseRecipients(v)
			}
			if !hasNonEmptyMailTemplateUpdate(toolArgs) {
				return fmt.Errorf("至少需要指定一个更新字段：--from / --subject / --content（或 --body）/ --name / --to / --cc")
			}
			err := callMCPTool("update_user_message_template", toolArgs)
			if cliErr, ok := err.(*CLIError); ok && strings.Contains(cliErr.Message, "Invalid parameter") {
				cliErr.Suggestion = "邮箱服务端仅支持更新草稿模板 (创建时带 --is-draft)；非草稿模板不可修改，请先 dws mail template delete 后用 --is-draft 重建"
			}
			return err
		},
	}
	DeclareLeafMetadata(templateUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "update_user_message_template",
				CanonicalPath:  "mail.update_user_message_template",
				CLIPath:        "mail template update",
				PrimaryCLIPath: "mail template update",
			},
			Description: "更新已有邮件模板的内容",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "update_user_message_template"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新已有邮件模板的内容",
				UseWhen:      []string{"已知 templateId 并需要修改名称、主题、正文或收件人时"},
				AvoidWhen:    []string{"创建新模板时使用 mail template create"},
				Examples:     []string{"dws mail template update --email user@company.com --id <templateId> --subject \"新标题\" --content \"新正文\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "content", Property: "body", Required: boolPtr(false)},
				{Name: "name", Required: boolPtr(false)},
				{Name: "subject", Required: boolPtr(false)},
				{Name: "cc", Property: "ccRecipients"},
				{Name: "to", Property: "toRecipients"},
			},
		},
	})

	templateUpdateCmd.Flags().String("email", "", "用户邮箱地址 (必填)")
	templateUpdateCmd.Flags().String("id", "", "模板唯一标识 (必填)")
	templateUpdateCmd.Flags().String("from", "", "模板发件人邮箱 (可选)")
	templateUpdateCmd.Flags().String("subject", "", "模板邮件标题 (可选)")
	templateUpdateCmd.Flags().String("content", "", "模板邮件正文 (可选)")
	templateUpdateCmd.Flags().String("body", "", "--content 的别名")
	_ = templateUpdateCmd.Flags().MarkHidden("body")
	templateUpdateCmd.Flags().String("name", "", "模板名称 (可选)")
	templateUpdateCmd.Flags().String("to", "", "模板收件人列表，逗号分隔 (可选)")
	templateUpdateCmd.Flags().String("cc", "", "模板抄送人列表，逗号分隔 (可选)")

	templateDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除邮件模板",
		Long: `根据模板 ID 删除指定邮件模板。

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail template delete --email user@company.com --id <templateId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id"); err != nil {
				return err
			}
			return callMCPTool("delete_user_message_template", map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"id":    mustGetFlag(cmd, "id"),
			})
		},
	}
	DeclareLeafMetadata(templateDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "delete_user_message_template",
				CanonicalPath:  "mail.delete_user_message_template",
				CLIPath:        "mail template delete",
				PrimaryCLIPath: "mail template delete",
			},
			Description: "删除指定邮件模板",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "delete_user_message_template"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "删除指定邮件模板",
				UseWhen:      []string{"已知 templateId 且明确不再需要该模板时"},
				AvoidWhen:    []string{"只修改模板内容时使用 mail template update"},
				Examples:     []string{"dws mail template delete --email user@company.com --id <templateId>"},
			},
		},
	})

	templateDeleteCmd.Flags().String("email", "", "用户邮箱地址 (必填)")
	templateDeleteCmd.Flags().String("id", "", "模板唯一标识 (必填)")

	templateCmd.AddCommand(templateCreateCmd, templateListCmd, templateGetCmd, templateUpdateCmd, templateDeleteCmd)

	// ── contact 子命令组 ──────────────────────────────────

	contactCmd := &cobra.Command{Use: "contact", Short: "邮件联系人管理", RunE: groupRunE}

	contactCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建邮件联系人",
		Long: `创建一个新的邮件联系人。

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail contact create --email user@company.com --contact-email colleague@company.com --display-name "张三"
  dws mail contact create --email user@company.com --contact-email colleague@company.com --first-name "三" --last-name "张"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "contact-email"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"email":        mustGetFlag(cmd, "email"),
				"contactEmail": mustGetFlag(cmd, "contact-email"),
			}
			if v, _ := cmd.Flags().GetString("first-name"); v != "" {
				toolArgs["firstName"] = v
			}
			if v, _ := cmd.Flags().GetString("middle-name"); v != "" {
				toolArgs["middleName"] = v
			}
			if v, _ := cmd.Flags().GetString("last-name"); v != "" {
				toolArgs["lastName"] = v
			}
			if v, _ := cmd.Flags().GetString("display-name"); v != "" {
				toolArgs["displayName"] = v
			}
			return callMCPTool("create_user_mail_contact", toolArgs)
		},
	}
	DeclareLeafMetadata(contactCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "create_user_mail_contact",
				CanonicalPath:  "mail.create_user_mail_contact",
				CLIPath:        "mail contact create",
				PrimaryCLIPath: "mail contact create",
			},
			Description: "创建个人邮件联系人",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "create_user_mail_contact"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建个人邮件联系人",
				UseWhen:      []string{"需要把一个邮件地址保存到自己的联系人列表时"},
				AvoidWhen:    []string{"按姓名搜索企业邮箱用户时使用 mail user search"},
				Examples:     []string{"dws mail contact create --email user@company.com --contact-email colleague@company.com --display-name \"张三\""},
			},
		},
	})

	contactCreateCmd.Flags().String("email", "", "用户邮箱地址 (必填)")
	contactCreateCmd.Flags().String("contact-email", "", "联系人邮箱地址 (必填)")
	contactCreateCmd.Flags().String("first-name", "", "联系人名 (可选)")
	contactCreateCmd.Flags().String("middle-name", "", "联系人中间名 (可选)")
	contactCreateCmd.Flags().String("last-name", "", "联系人姓 (可选)")
	contactCreateCmd.Flags().String("display-name", "", "联系人显示名称 (可选)")

	contactListCmd := &cobra.Command{
		Use:   "list",
		Short: "列举邮件联系人",
		Long: `列出指定邮箱的所有邮件联系人。

返回字段：
  联系人列表及分页信息

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail contact list --email user@company.com --limit 20
  dws mail contact list --email user@company.com --limit 20 --cursor <nextCursor>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "limit", "size"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "email"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"size":  flagOrFallback(cmd, "limit", "size"),
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			return callMCPTool("list_user_mail_contacts", toolArgs)
		},
	}
	DeclareLeafMetadata(contactListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "list_user_mail_contacts",
				CanonicalPath:  "mail.list_user_mail_contacts",
				CLIPath:        "mail contact list",
				PrimaryCLIPath: "mail contact list",
			},
			Description: "分页列出个人保存的邮件联系人",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "list_user_mail_contacts"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "分页列出个人保存的邮件联系人",
				UseWhen:      []string{"需要浏览自己的联系人或取得 contactId 时"},
				AvoidWhen:    []string{"搜索企业通讯录用户时使用 mail user search"},
				Examples:     []string{"dws mail contact list --email user@company.com --limit 20"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "limit", Property: "size"},
			},
		},
	})

	contactListCmd.Flags().String("email", "", "用户邮箱地址 (必填)")
	contactListCmd.Flags().String("cursor", "", "分页游标，取自响应中的 nextCursor 字段 (可选)")
	contactListCmd.Flags().String("limit", "", "每页返回数量 (必填)")
	contactListCmd.Flags().String("size", "", "--limit 的别名")
	_ = contactListCmd.Flags().MarkHidden("size")

	contactUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新邮件联系人",
		Long: `更新已有邮件联系人的信息。仅传入需要更新的字段即可。

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail contact update --email user@company.com --contact-id <contactId> --display-name "李四"
  dws mail contact update --email user@company.com --contact-id <contactId> --contact-email new@company.com --first-name "四" --last-name "李"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "contact-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"email":     mustGetFlag(cmd, "email"),
				"contactId": mustGetFlag(cmd, "contact-id"),
			}
			if v, _ := cmd.Flags().GetString("contact-email"); v != "" {
				toolArgs["contactEmail"] = v
			}
			if v, _ := cmd.Flags().GetString("first-name"); v != "" {
				toolArgs["firstName"] = v
			}
			if v, _ := cmd.Flags().GetString("middle-name"); v != "" {
				toolArgs["middleName"] = v
			}
			if v, _ := cmd.Flags().GetString("last-name"); v != "" {
				toolArgs["lastName"] = v
			}
			if v, _ := cmd.Flags().GetString("display-name"); v != "" {
				toolArgs["displayName"] = v
			}
			return callMCPTool("update_user_mail_contact", toolArgs)
		},
	}
	DeclareLeafMetadata(contactUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "update_user_mail_contact",
				CanonicalPath:  "mail.update_user_mail_contact",
				CLIPath:        "mail contact update",
				PrimaryCLIPath: "mail contact update",
			},
			Description: "更新个人邮件联系人的资料",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "update_user_mail_contact"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新个人邮件联系人的资料",
				UseWhen:      []string{"已知 contactId 并需要改显示名或邮箱信息时"},
				AvoidWhen:    []string{"企业通讯录资料不能通过此命令修改"},
				Examples:     []string{"dws mail contact update --email user@company.com --contact-id <contactId> --display-name \"李四\""},
			},
		},
	})

	contactUpdateCmd.Flags().String("email", "", "用户邮箱地址 (必填)")
	contactUpdateCmd.Flags().String("contact-id", "", "联系人唯一标识 (必填)")
	contactUpdateCmd.Flags().String("contact-email", "", "联系人邮箱地址 (可选)")
	contactUpdateCmd.Flags().String("first-name", "", "联系人名 (可选)")
	contactUpdateCmd.Flags().String("middle-name", "", "联系人中间名 (可选)")
	contactUpdateCmd.Flags().String("last-name", "", "联系人姓 (可选)")
	contactUpdateCmd.Flags().String("display-name", "", "联系人显示名称 (可选)")

	contactBatchDeleteCmd := &cobra.Command{
		Use:   "batch-delete",
		Short: "批量删除邮件联系人",
		Long: `批量删除指定的邮件联系人。

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail contact batch-delete --email user@company.com --contact-ids <id1>,<id2>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "contact-ids"); err != nil {
				return err
			}
			return callMCPTool("batch_delete_user_mail_contacts", map[string]any{
				"email":      mustGetFlag(cmd, "email"),
				"contactIds": parseRecipients(mustGetFlag(cmd, "contact-ids")),
			})
		},
	}
	DeclareLeafMetadata(contactBatchDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "batch_delete_user_mail_contacts",
				CanonicalPath:  "mail.batch_delete_user_mail_contacts",
				CLIPath:        "mail contact batch-delete",
				PrimaryCLIPath: "mail contact batch-delete",
			},
			Description: "批量删除个人邮件联系人",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "batch_delete_user_mail_contacts"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "批量删除个人邮件联系人",
				UseWhen:      []string{"需要从自己的联系人列表移除多个已知 contactId 时"},
				AvoidWhen:    []string{"删除邮件或查询企业通讯录用户时不要使用"},
				Examples:     []string{"dws mail contact batch-delete --email user@company.com --contact-ids <id1>,<id2>"},
			},
		},
	})

	contactBatchDeleteCmd.Flags().String("email", "", "用户邮箱地址 (必填)")
	contactBatchDeleteCmd.Flags().String("contact-ids", "", "要删除的联系人 ID 列表，逗号分隔 (必填)")

	contactCmd.AddCommand(contactCreateCmd, contactListCmd, contactUpdateCmd, contactBatchDeleteCmd)

	// ── auto-reply 自动回复 ──────────────────────────────
	autoReplyCmd := &cobra.Command{Use: "auto-reply", Short: "邮件自动回复管理", RunE: groupRunE}

	autoReplyGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取用户的自动回复配置",
		Long: `获取当前用户的邮件自动回复配置，包括是否启用、生效时间、回复范围和回复内容。

返回字段：
  enabled    是否启用自动回复 (true=启用, false=禁用)
  startTime  自动回复开始时间
  endTime    自动回复结束时间
  scope      回复范围: "contact"(仅联系人) 或 "all"(所有人)
  content    自动回复内容

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail auto-reply get --email user@company.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email"); err != nil {
				return err
			}
			return callMCPTool("get_auto_reply", map[string]any{
				"email": mustGetFlag(cmd, "email"),
			})
		},
	}

	autoReplyUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新/设置用户的自动回复配置",
		Long: `更新或设置用户的邮件自动回复配置。所有参数均为必填。

建议工作流：先通过 auto-reply get 获取当前配置，再传入需要修改的字段值。

时间格式示例：2026/06/25 16:00:00 +0800

参数说明（全部必填）：
  --enabled  是否启用自动回复 (true/false)
  --start    自动回复开始时间
  --end      自动回复结束时间
  --scope    回复范围: "contact"(仅联系人) 或 "all"(所有人)
  --content  自动回复内容

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail auto-reply update --email user@company.com --enabled true --start "2026/07/01 09:00:00 +0800" --end "2026/07/07 18:00:00 +0800" --scope all --content "出差中，请稍后联系"
  dws mail auto-reply update --email user@company.com --enabled false --start "2026/07/01 09:00:00 +0800" --end "2026/07/07 18:00:00 +0800" --scope all --content "已关闭自动回复"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "enabled", "start", "end", "scope", "content"); err != nil {
				return err
			}
			enabledRaw := strings.ToLower(strings.TrimSpace(mustGetFlag(cmd, "enabled")))
			if enabledRaw != "true" && enabledRaw != "false" {
				return fmt.Errorf("--enabled 必须为 true 或 false")
			}
			toolArgs := map[string]any{
				"email":     mustGetFlag(cmd, "email"),
				"enabled":   enabledRaw == "true",
				"startTime": mustGetFlag(cmd, "start"),
				"endTime":   mustGetFlag(cmd, "end"),
				"scope":     mustGetFlag(cmd, "scope"),
				"content":   mustGetFlag(cmd, "content"),
			}
			return callMCPTool("update_auto_reply", toolArgs)
		},
	}

	autoReplyGetCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	autoReplyUpdateCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	autoReplyUpdateCmd.Flags().String("enabled", "", "是否启用自动回复: true/false (必填)")
	autoReplyUpdateCmd.Flags().String("start", "", "自动回复开始时间，格式: YYYY/MM/DD HH:MM:SS +ZZZZ (必填)")
	autoReplyUpdateCmd.Flags().String("end", "", "自动回复结束时间，格式: YYYY/MM/DD HH:MM:SS +ZZZZ (必填)")
	autoReplyUpdateCmd.Flags().String("scope", "", "回复范围: contact(仅联系人)/all(所有人) (必填)")
	autoReplyUpdateCmd.Flags().String("content", "", "自动回复内容 (必填)")

	autoReplyCmd.AddCommand(autoReplyGetCmd, autoReplyUpdateCmd)

	// ── rule 收信规则 ────────────────────────────────────
	ruleCmd := &cobra.Command{Use: "rule", Short: "收信规则管理", RunE: groupRunE}

	ruleListCmd := &cobra.Command{
		Use:   "list",
		Short: "列出个人收信规则",
		Long: `列出当前用户的所有收信规则，包括规则名称、启用状态、条件、动作和排序。

返回字段：
  total    规则总数
  rules    规则列表，每条包含 id, name, enabled, conditions, actions, order

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail rule list --email user@company.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email"); err != nil {
				return err
			}
			return callMCPTool("list_mail_rules", map[string]any{
				"email": mustGetFlag(cmd, "email"),
			})
		},
	}

	ruleCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建个人收信规则",
		Long: `创建一条新的收信规则。支持设置规则名称、启用状态、匹配条件和执行动作。

--conditions 和 --actions 为 JSON 数组字符串，示例：
  --conditions '[{"object":"from","or":[{"and":[{"operation":"oneof","keyword":"a@test.com","ignoreCase":true}]},{"and":[{"operation":"oneof","keyword":"b@test.com","ignoreCase":true}]}]}]'
  --actions '[{"action":"ActFlagMail2","parameters":["asread"]}]'

条件逻辑：
  conditions 数组中多个条件之间为 AND(且) 关系
  同一 object 下 or 数组中多个表达式之间为 OR(或) 关系
  同一 and 数组中多个子条件之间为 AND(且) 关系
  同一 object 匹配多个值时，在 or 数组中放多个 and 项(每个 and 对应一个值)，表示满足任一即可

object 与 operation 合法组合：
  from/to → include(包含), exclude(不包含), oneof(是联系人之一), noneof(不是联系人之一)
  subject → include(包含), exclude(不包含)
  attachment → exist(是否存在附件): keyword="1"有附件, keyword="0"无附件
  x-aliyun-size → greater(大于), less(小于): 单位为字节(Bytes)，如 1KB=1024, 1MB=1048576，可组合表示范围区间

动作类型(action)：
  ActSavetoFolder(移动到文件夹) → parameters为目标文件夹ID，需先 dws mail folder list 获取
  ActFlagMail(标记标签) → parameters为标签ID列表(逗号分隔)，需先 dws mail tag list 获取
  ActFlagMail2(标记已读) → parameters为"asread"（服务端仅支持标记已读，不支持标记未读）
  ActReply(自动回复) → parameters为回复内容文本

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail rule create --email user@company.com --name "VIP邮件标记" --enabled true \
    --conditions '[{"object":"from","or":[{"and":[{"operation":"oneof","keyword":"a@test.com","ignoreCase":true}]},{"and":[{"operation":"oneof","keyword":"b@test.com","ignoreCase":true}]}]}]' \
    --actions '[{"action":"ActFlagMail2","parameters":["asread"]}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "name", "enabled", "actions"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"email":   mustGetFlag(cmd, "email"),
				"name":    mustGetFlag(cmd, "name"),
				"enabled": mustGetFlag(cmd, "enabled") == "true",
			}
			if v, _ := cmd.Flags().GetString("conditions"); v != "" {
				conditions, err := parseMailRuleConditions(v)
				if err != nil {
					return err
				}
				toolArgs["conditions"] = conditions
			}
			var actions []any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "actions")), &actions); err != nil {
				return fmt.Errorf("--actions JSON 格式错误: %w", err)
			}
			toolArgs["actions"] = actions
			return callMCPTool("create_mail_rule", toolArgs)
		},
	}

	ruleUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新个人收信规则",
		Long: `更新已有的收信规则。除 --conditions 外所有参数均为必填。

建议工作流：先通过 rule list 获取当前规则的完整配置，再传入需要修改的字段值。

--conditions 为空或不传表示命中所有邮件（无条件匹配）。
--actions 为 JSON 数组字符串，格式同 create 命令。

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail rule update --email user@company.com --id <ruleId> --name "新规则名" --enabled true \
    --actions '[{"action":"ActSavetoFolder","parameters":["6"]}]'
  dws mail rule update --email user@company.com --id <ruleId> --name "全量归档" --enabled false \
    --conditions '[{"object":"subject","or":[{"and":[{"operation":"include","keyword":"报告","ignoreCase":true}]}]}]' \
    --actions '[{"action":"ActSavetoFolder","parameters":["6"]}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id", "name", "enabled", "actions"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"email":   mustGetFlag(cmd, "email"),
				"id":      mustGetFlag(cmd, "id"),
				"name":    mustGetFlag(cmd, "name"),
				"enabled": mustGetFlag(cmd, "enabled") == "true",
			}
			if v, _ := cmd.Flags().GetString("conditions"); v != "" {
				conditions, err := parseMailRuleConditions(v)
				if err != nil {
					return err
				}
				toolArgs["conditions"] = conditions
			}
			var actions []any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "actions")), &actions); err != nil {
				return fmt.Errorf("--actions JSON 格式错误: %w", err)
			}
			toolArgs["actions"] = actions
			return callMCPTool("update_mail_rule", toolArgs)
		},
	}

	ruleDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除个人收信规则",
		Long: `删除指定的收信规则。

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail rule delete --email user@company.com --id <ruleId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id"); err != nil {
				return err
			}
			return callMCPTool("delete_mail_rule", map[string]any{
				"email": mustGetFlag(cmd, "email"),
				"id":    mustGetFlag(cmd, "id"),
			})
		},
	}

	ruleAdjustCmd := &cobra.Command{
		Use:   "adjust",
		Short: "调整收信规则排序",
		Long: `调整指定收信规则的排序位置，向上(up)或向下(down)移动。

错误说明：
  domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
		Example: `  dws mail rule adjust --email user@company.com --id <ruleId> --direction up
  dws mail rule adjust --email user@company.com --id <ruleId> --direction down`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "id", "direction"); err != nil {
				return err
			}
			return callMCPTool("adjust_mail_rule", map[string]any{
				"email":     mustGetFlag(cmd, "email"),
				"id":        mustGetFlag(cmd, "id"),
				"direction": mustGetFlag(cmd, "direction"),
			})
		},
	}

	ruleListCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	ruleCreateCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	ruleCreateCmd.Flags().String("name", "", "规则名称 (必填)")
	ruleCreateCmd.Flags().String("enabled", "", "是否启用: true/false (必填)")
	ruleCreateCmd.Flags().String("conditions", "", "规则条件 JSON 数组 (可选)")
	ruleCreateCmd.Flags().String("actions", "", "规则动作 JSON 数组 (必填)")
	ruleUpdateCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	ruleUpdateCmd.Flags().String("id", "", "规则 ID (必填)")
	ruleUpdateCmd.Flags().String("name", "", "规则名称 (必填)")
	ruleUpdateCmd.Flags().String("enabled", "", "是否启用: true/false (必填)")
	ruleUpdateCmd.Flags().String("conditions", "", "规则条件 JSON 数组 (可选，为空表示命中所有邮件)")
	ruleUpdateCmd.Flags().String("actions", "", "规则动作 JSON 数组 (必填)")
	ruleDeleteCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	ruleDeleteCmd.Flags().String("id", "", "规则 ID (必填)")
	ruleAdjustCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	ruleAdjustCmd.Flags().String("id", "", "规则 ID (必填)")
	ruleAdjustCmd.Flags().String("direction", "", "调整方向: up/down (必填)")

	ruleCmd.AddCommand(ruleListCmd, ruleCreateCmd, ruleUpdateCmd, ruleDeleteCmd, ruleAdjustCmd)

	// ── allow-list 个人收信白名单 ────────────────────────────────
	allowListCmd := &cobra.Command{Use: "allow-list", Short: "个人收信白名单管理", RunE: groupRunE}

	allowListListCmd := &cobra.Command{
		Use:   "list",
		Short: "列出个人收信白名单",
		Long: `列出当前用户的个人收信白名单地址列表。

返回字段：
  entries   白名单地址列表
  success   接口调用是否成功
  errorCode 错误码（仅失败时存在）
  errorMsg  错误信息（仅失败时存在）`,
		Example: `  dws mail allow-list list --email user@company.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email"); err != nil {
				return err
			}
			return callMCPTool("list_mailbox_allowlist", map[string]any{
				"email": mustGetFlag(cmd, "email"),
			})
		},
	}
	allowListAddCmd := &cobra.Command{
		Use:   "add",
		Short: "添加个人收信白名单",
		Long: `向个人收信白名单中添加邮件地址或域名。

条目格式：
  - 邮件地址：123@domain.com
  - 域名：@domain.com（域名前需加 @ 符号）`,
		Example: `  dws mail allow-list add --email user@company.com --entries a@b.com,@spam.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "entries"); err != nil {
				return err
			}
			return callMCPTool("add_mailbox_allowlist", map[string]any{
				"email":   mustGetFlag(cmd, "email"),
				"entries": parseRecipients(mustGetFlag(cmd, "entries")),
			})
		},
	}
	allowListRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "移除个人收信白名单",
		Long: `从个人收信白名单中移除邮件地址或域名。

条目格式：
  - 邮件地址：123@domain.com
  - 域名：@domain.com（域名前需加 @ 符号）`,
		Example: `  dws mail allow-list remove --email user@company.com --entries a@b.com,@spam.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "entries"); err != nil {
				return err
			}
			return callMCPTool("remove_mailbox_allowlist", map[string]any{
				"email":   mustGetFlag(cmd, "email"),
				"entries": parseRecipients(mustGetFlag(cmd, "entries")),
			})
		},
	}
	allowListListCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	allowListAddCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	allowListAddCmd.Flags().String("entries", "", "逗号分隔的地址列表，支持邮件地址(如123@domain.com)或域名(如@domain.com)")
	allowListRemoveCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	allowListRemoveCmd.Flags().String("entries", "", "逗号分隔的地址列表，支持邮件地址(如123@domain.com)或域名(如@domain.com)")
	allowListCmd.AddCommand(allowListListCmd, allowListAddCmd, allowListRemoveCmd)

	// ── block-list 个人收信黑名单 ────────────────────────────────
	blockListCmd := &cobra.Command{Use: "block-list", Short: "个人收信黑名单管理", RunE: groupRunE}

	blockListListCmd := &cobra.Command{
		Use:   "list",
		Short: "列出个人收信黑名单",
		Long: `列出当前用户的个人收信黑名单地址列表。

返回字段：
  entries   黑名单地址列表
  success   接口调用是否成功
  errorCode 错误码（仅失败时存在）
  errorMsg  错误信息（仅失败时存在）`,
		Example: `  dws mail block-list list --email user@company.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email"); err != nil {
				return err
			}
			return callMCPTool("list_mailbox_blocklist", map[string]any{
				"email": mustGetFlag(cmd, "email"),
			})
		},
	}
	blockListAddCmd := &cobra.Command{
		Use:   "add",
		Short: "添加个人收信黑名单",
		Long: `向个人收信黑名单中添加邮件地址或域名。

条目格式：
  - 邮件地址：123@domain.com
  - 域名：@domain.com（域名前需加 @ 符号）`,
		Example: `  dws mail block-list add --email user@company.com --entries spam@bad.com,@junk.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "entries"); err != nil {
				return err
			}
			return callMCPTool("add_mailbox_blocklist", map[string]any{
				"email":   mustGetFlag(cmd, "email"),
				"entries": parseRecipients(mustGetFlag(cmd, "entries")),
			})
		},
	}
	blockListRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "移除个人收信黑名单",
		Long: `从个人收信黑名单中移除邮件地址或域名。

条目格式：
  - 邮件地址：123@domain.com
  - 域名：@domain.com（域名前需加 @ 符号）`,
		Example: `  dws mail block-list remove --email user@company.com --entries spam@bad.com,@junk.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "entries"); err != nil {
				return err
			}
			return callMCPTool("remove_mailbox_blocklist", map[string]any{
				"email":   mustGetFlag(cmd, "email"),
				"entries": parseRecipients(mustGetFlag(cmd, "entries")),
			})
		},
	}
	blockListListCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	blockListAddCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	blockListAddCmd.Flags().String("entries", "", "逗号分隔的地址列表，支持邮件地址(如123@domain.com)或域名(如@domain.com)")
	blockListRemoveCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	blockListRemoveCmd.Flags().String("entries", "", "逗号分隔的地址列表，支持邮件地址(如123@domain.com)或域名(如@domain.com)")
	blockListCmd.AddCommand(blockListListCmd, blockListAddCmd, blockListRemoveCmd)

	calendarCmd := &cobra.Command{Use: "calendar", Short: "邮箱日历管理", RunE: groupRunE}
	calendarListCmd := &cobra.Command{
		Use:   "list",
		Short: "列出用户可访问的日历列表",
		Long: `列出用户可访问的日历列表，包括用户自己创建以及接受共享后生成的日历。
返回的 id 可作为 calendar-event list 的 --id / --folder-id 使用。`,
		Example: `  dws mail calendar list --email user@company.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email"); err != nil {
				return err
			}
			return callMCPTool("list_mailbox_calendars", map[string]any{
				"email": mustGetFlag(cmd, "email"),
			})
		},
	}
	DeclareLeafMetadata(calendarListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "list_mailbox_calendars",
				CanonicalPath:  "mail.list_mailbox_calendars",
				CLIPath:        "mail calendar list",
				PrimaryCLIPath: "mail calendar list",
			},
			Description: "列出邮箱日历文件夹",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "list_mailbox_calendars"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "列出邮箱可访问的日历文件夹",
				UseWhen:      []string{"查询邮箱日历文件夹 id，以便继续查日程时"},
				AvoidWhen:    []string{"钉钉主日历日程请用 dws calendar event list"},
				Examples:     []string{"dws mail calendar list --email user@company.com"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "email", Property: "email", Required: boolPtr(true)},
			},
		},
	})
	calendarListCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	calendarCmd.AddCommand(calendarListCmd)

	calendarEventCmd := &cobra.Command{Use: "calendar-event", Short: "邮箱日历日程管理", RunE: groupRunE}
	calendarEventListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询指定日历时间范围内的日程",
		Long:  `查询指定邮箱日历文件夹在 UTC 时间区间 [startTime, endTime) 内出现的日程，支持 cursor 分页。循环日程会展开为该时间范围内的单次日程。`,
		Example: `  dws mail calendar-event list --email user@company.com --id <calendarFolderId> --start "2026-07-01T00:00:00Z" --end "2026-07-31T23:59:59Z"
  dws mail calendar-event list --email user@company.com --id <calendarFolderId> --start "2026-07-01T00:00:00Z" --end "2026-07-31T23:59:59Z" --cursor <cursor>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "email", "start", "end"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "id", "folder-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"email":     mustGetFlag(cmd, "email"),
				"id":        flagOrFallback(cmd, "id", "folder-id"),
				"startTime": flagOrFallback(cmd, "start", "start-time"),
				"endTime":   flagOrFallback(cmd, "end", "end-time"),
			}
			if cursor := mustGetFlag(cmd, "cursor"); cursor != "" {
				toolArgs["cursor"] = cursor
			}
			return callMCPTool("list_mailbox_calendar_events", toolArgs)
		},
	}
	DeclareLeafMetadata(calendarEventListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "mail",
				Name:           "list_mailbox_calendar_events",
				CanonicalPath:  "mail.list_mailbox_calendar_events",
				CLIPath:        "mail calendar-event list",
				PrimaryCLIPath: "mail calendar-event list",
			},
			Description: "查询邮箱日历文件夹时间范围内的日程",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "mail", RPCName: "list_mailbox_calendar_events"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询邮箱日历日程列表",
				UseWhen:      []string{"已知邮箱日历文件夹 id，需要按 UTC 时间窗列出日程时"},
				AvoidWhen:    []string{"钉钉主日历请用 dws calendar event list；未知文件夹 id 时先 mail calendar list"},
				Examples:     []string{"dws mail calendar-event list --email user@company.com --id <calendarFolderId> --start \"2026-07-01T00:00:00Z\" --end \"2026-07-31T23:59:59Z\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "email", Property: "email", Required: boolPtr(true)},
				{Name: "id", Property: "id", Required: boolPtr(true)},
				{Name: "start", Property: "startTime", Required: boolPtr(true)},
				{Name: "end", Property: "endTime", Required: boolPtr(true)},
				{Name: "cursor", Property: "cursor"},
			},
		},
	})
	calendarEventListCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	calendarEventListCmd.Flags().String("id", "", "日历文件夹ID (必填)")
	calendarEventListCmd.Flags().String("folder-id", "", "--id 的别名")
	_ = calendarEventListCmd.Flags().MarkHidden("folder-id")
	calendarEventListCmd.Flags().String("start", "", "视图开始UTC时间 (必填)")
	calendarEventListCmd.Flags().String("start-time", "", "--start 的别名")
	_ = calendarEventListCmd.Flags().MarkHidden("start-time")
	calendarEventListCmd.Flags().String("end", "", "视图结束UTC时间 (必填)")
	calendarEventListCmd.Flags().String("end-time", "", "--end 的别名")
	_ = calendarEventListCmd.Flags().MarkHidden("end-time")
	calendarEventListCmd.Flags().String("cursor", "", "分页光标 (可选)")
	calendarEventCmd.AddCommand(calendarEventListCmd)

	root.AddCommand(mailboxCmd, messageCmd, sentMessageCmd, draftCmd, threadCmd, folderCmd, tagCmd, userCmd, attachmentCmd, templateCmd, contactCmd, autoReplyCmd, ruleCmd, allowListCmd, blockListCmd, calendarCmd, calendarEventCmd)

	return root
}

// ──────────────────────────────────────────────────────────
// 邮件附件上传编排
// ──────────────────────────────────────────────────────────

// inlineAttachInfo 保存内联附件的路径、文件名、大小及自动生成的 contentId（cid）。
type inlineAttachInfo struct {
	path      string
	name      string
	size      int64
	contentId string
}

// generateContentId 生成标准格式的内联附件 contentId。
// 格式：inline-{文件名（不含扩展名）}-{序号}@alimail.com
// 文件名中的空格替换为 "-"，字母统一转小写，去掉扩展名，确保 cid local-part 中无多余的 "."。
func generateContentId(filename string, index int) string {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	nameWithoutExt := strings.TrimSuffix(base, ext)
	name := strings.ToLower(strings.ReplaceAll(nameWithoutExt, " ", "-"))
	return fmt.Sprintf("inline-%s-%d@alimail.com", name, index)
}

// supportedInlineExts 列出允许作为内联附件的图片文件扩展名。
// 内联附件仅支持图片类型，视频、音频、PDF 等请改用 --attachment。
var supportedInlineExts = map[string]bool{
	"jpg": true, "jpeg": true, "png": true, "gif": true,
	"webp": true, "bmp": true, "svg": true,
}

// validateInlineAttachmentType 校验文件是否为支持内联的图片类型。
// 非图片文件不支持内联，应改用 --attachment。
func validateInlineAttachmentType(filePath string) error {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filePath), "."))
	if !supportedInlineExts[ext] {
		return fmt.Errorf(
			"不支持将 %q 作为内联附件（扩展名 .%s 不是图片类型）\n"+
				"支持的内联图片类型：jpg/jpeg/png/gif/webp/bmp/svg\n"+
				"如需发送此类文件，请改用 --attachment",
			filepath.Base(filePath), ext,
		)
	}
	return nil
}

// inlineHtmlTag 为内联图片附件生成 HTML img 标签。
// 内联附件仅支持图片类型，统一输出 <img src="cid:..." alt="...">。
func inlineHtmlTag(cid, filename string) string {
	return fmt.Sprintf(`<img src="cid:%s" alt="%s">`, cid, filename)
}

// injectInlineCids 将 body 文本中的 [inline:文件名] 占位符替换为对应类型的 HTML 标签，
// 并将换行符转换为 <br>，包裹为完整 HTML 文档。
// 若 body 中没有某个内联附件对应的占位符，则将该文件追加到正文末尾。
// 不同文件类型生成不同标签（详见 inlineHtmlTag）。
func injectInlineCids(body string, inlineFiles []inlineAttachInfo) string {
	htmlBody := strings.ReplaceAll(body, "\n", "<br>")
	for _, f := range inlineFiles {
		placeholder := fmt.Sprintf("[inline:%s]", f.name)
		tag := inlineHtmlTag(f.contentId, f.name)
		htmlBody = strings.ReplaceAll(htmlBody, placeholder, tag)
	}
	// 未被占位符引用的内联附件追加到正文末尾
	for _, f := range inlineFiles {
		placeholder := fmt.Sprintf("[inline:%s]", f.name)
		if !strings.Contains(body, placeholder) {
			htmlBody += "<br>" + inlineHtmlTag(f.contentId, f.name)
		}
	}
	return "<html><body>" + htmlBody + "</body></html>"
}

// runMailDraftWithAttachment 执行草稿编排：upsert 草稿并上传附件，返回 messageId。
//
// upsert 语义：messageId 为空时调用 draftTool 创建新草稿；messageId 非空时调用
// update_draft 更新已有草稿（draftTool 参数此时忽略）。
//
// 调用方负责决定后续操作（发送或仅保留草稿）。
func runMailDraftWithAttachment(draftTool string, draftArgs map[string]any, messageId string, body string, attachments []string, inlineAttachments []string) (string, error) {
	// 预校验普通附件文件
	type attachInfo struct {
		path string
		name string
		size int64
	}
	files := make([]attachInfo, 0, len(attachments))
	for _, fp := range attachments {
		fi, err := os.Stat(fp)
		if err != nil {
			return "", fmt.Errorf("cannot read attachment %s: %w", fp, err)
		}
		if fi.IsDir() {
			return "", fmt.Errorf("%s is a directory, not a file", fp)
		}
		if fi.Size() <= 0 {
			return "", fmt.Errorf("attachment %s is empty", fp)
		}
		files = append(files, attachInfo{
			path: fp,
			name: filepath.Base(fp),
			size: fi.Size(),
		})
	}

	// 预校验内联附件文件，并生成 contentId
	inlineFiles := make([]inlineAttachInfo, 0, len(inlineAttachments))
	for i, fp := range inlineAttachments {
		fi, err := os.Stat(fp)
		if err != nil {
			return "", fmt.Errorf("cannot read inline attachment %s: %w", fp, err)
		}
		if fi.IsDir() {
			return "", fmt.Errorf("%s is a directory, not a file", fp)
		}
		if fi.Size() <= 0 {
			return "", fmt.Errorf("inline attachment %s is empty", fp)
		}
		if err := validateInlineAttachmentType(fp); err != nil {
			return "", err
		}
		inlineFiles = append(inlineFiles, inlineAttachInfo{
			path:      fp,
			name:      filepath.Base(fp),
			size:      fi.Size(),
			contentId: generateContentId(fp, i+1),
		})
	}

	from, _ := draftArgs["from"].(string)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Step 0: 查询邮箱类型（个人/企业），用于确定附件上传域名
	mailboxText, err := callMCPToolReturnText(ctx, "list_user_mailboxes", nil)
	if err != nil {
		return "", fmt.Errorf("查询邮箱列表失败: %w", err)
	}
	accountType := parseMailAccountType(mailboxText, from)

	// Step 1: 若有内联附件，将 body 转为 HTML 并注入 cid 引用
	if len(inlineFiles) > 0 {
		draftArgs["body"] = injectInlineCids(body, inlineFiles)
	}

	// Step 2: upsert 草稿
	var draftText string
	if messageId == "" {
		// create
		draftText, err = callMCPToolReturnText(ctx, draftTool, draftArgs)
		if err != nil {
			return "", fmt.Errorf("创建草稿失败: %w", err)
		}
		messageId, err = parseMailDraftId(draftText)
		if err != nil {
			return "", err
		}
	} else {
		// update
		if _, err = callMCPToolReturnText(ctx, "update_draft", draftArgs); err != nil {
			return "", fmt.Errorf("更新草稿失败: %w", err)
		}
	}

	// Step 3: 上传普通附件（isInline=false）
	for _, f := range files {
		sessionArgs := map[string]any{
			"email":     from,
			"messageId": messageId,
			"name":      f.name,
			"isInline":  false,
		}
		sessionText, err := callMCPToolReturnText(ctx, "create_upload_session", sessionArgs)
		if err != nil {
			return "", fmt.Errorf("创建附件 %s 上传会话失败: %w", f.name, err)
		}
		uploadURL, err := parseMailUploadSession(sessionText)
		if err != nil {
			return "", fmt.Errorf("解析附件 %s 上传信息失败: %w", f.name, err)
		}
		if err := mailPutAttachment(ctx, accountType, uploadURL, f.path, f.size); err != nil {
			return "", fmt.Errorf("上传附件 %s 失败: %w", f.name, err)
		}
	}

	// Step 4: 上传内联附件（isInline=true，传入 contentId）
	for _, f := range inlineFiles {
		sessionArgs := map[string]any{
			"email":     from,
			"messageId": messageId,
			"name":      f.name,
			"isInline":  true,
			"contentId": f.contentId,
		}
		sessionText, err := callMCPToolReturnText(ctx, "create_upload_session", sessionArgs)
		if err != nil {
			return "", fmt.Errorf("创建内联附件 %s 上传会话失败: %w", f.name, err)
		}
		uploadURL, err := parseMailUploadSession(sessionText)
		if err != nil {
			return "", fmt.Errorf("解析内联附件 %s 上传信息失败: %w", f.name, err)
		}
		if err := mailPutAttachment(ctx, accountType, uploadURL, f.path, f.size); err != nil {
			return "", fmt.Errorf("上传内联附件 %s 失败: %w", f.name, err)
		}
	}

	return messageId, nil
}

// runMailSendWithAttachment 在有附件时执行编排流程：创建草稿、上传附件、发送草稿。
func runMailSendWithAttachment(cmd *cobra.Command, attachments []string, inlineAttachments []string) error {
	from := flagOrFallback(cmd, "from", "sender")
	subject := mustGetFlag(cmd, "subject")
	body := flagOrFallback(cmd, "content", "body")
	toRecipients := parseRecipients(mustGetFlag(cmd, "to"))

	draftArgs := map[string]any{
		"from":         from,
		"subject":      subject,
		"body":         body,
		"toRecipients": toRecipients,
	}
	if v, _ := cmd.Flags().GetString("cc"); v != "" {
		draftArgs["ccRecipients"] = parseRecipients(v)
	}

	messageId, err := runMailDraftWithAttachment("create_draft", draftArgs, "", body, attachments, inlineAttachments)
	if err != nil {
		return err
	}
	return callMCPTool("send_draft", map[string]any{
		"email":           from,
		"messageId":       messageId,
		"saveToSentItems": true,
	})
}

// parseMailDraftId 从 create_draft MCP tool 响应中提取 messageId。
// 支持两种响应格式：
//   - {"result":{"message":{"id":"xxx",...}}}  （实际返回格式）
//   - {"result":{"messageId":"xxx"}}           （兼容格式）
func parseMailDraftId(text string) (string, error) {
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return "", fmt.Errorf("failed to parse draft response: %w", err)
	}
	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}
	// 优先从 message.id 提取（实际返回格式）
	if msg, ok := data["message"].(map[string]any); ok {
		if id, ok := msg["id"].(string); ok && id != "" {
			return id, nil
		}
	}
	// 兼容 messageId 字段
	if id, ok := data["messageId"].(string); ok && id != "" {
		return id, nil
	}
	return "", fmt.Errorf("create_draft response missing messageId: %s", text)
}

// parseMailUploadSession 从 create_upload_session MCP tool 响应中提取上传 URL。
// 读取 uploadUrl 字段（含完整 URL）。
// 统一返回 rawURL，由调用方（httpPutMailAttachment）负责在缺少 host 时补全。
func parseMailUploadSession(text string) (string, error) {
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return "", fmt.Errorf("failed to parse upload session response: %w", err)
	}
	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}
	rawURL, _ := data["uploadUrl"].(string)
	if rawURL == "" {
		return "", fmt.Errorf("create_upload_session response missing uploadUrl: %s", text)
	}
	return rawURL, nil
}

// parseMailAccountType 从 list_user_mailboxes 响应中查找指定邮箱的账号类型。
// 返回 "PERSONAL" 或 "ENTERPRISE"（默认为企业邮箱）。
func parseMailAccountType(text string, email string) string {
	var data map[string]any
	if json.Unmarshal([]byte(text), &data) != nil {
		return "ENTERPRISE"
	}
	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}
	accounts, _ := data["emailAccounts"].([]any)
	emailLower := strings.ToLower(email)
	for _, item := range accounts {
		acc, ok := item.(map[string]any)
		if !ok {
			continue
		}
		accEmail, _ := acc["email"].(string)
		if strings.ToLower(accEmail) == emailLower {
			if t, ok := acc["type"].(string); ok {
				return t
			}
		}
	}
	return "ENTERPRISE"
}

// mailUploadBaseURL 根据账号类型判断使用个人邮箱还是企业邮箱的上传域名。
//   - PERSONAL（个人邮箱）: https://alimail-personal.aliyuncs.com
//   - 其他（企业邮箱）:     https://alimail-cn.aliyuncs.com
func mailUploadBaseURL(accountType string) string {
	if strings.ToUpper(accountType) == "PERSONAL" {
		return "https://alimail-personal.aliyuncs.com"
	}
	return "https://alimail-cn.aliyuncs.com"
}

// httpPutMailAttachment 通过 HTTP POST 上传附件文件内容到邮件上传链接。
// uploadURL 可以是完整 URL（含 host），也可以是相对路径（/v2/stream/{id}）。
// 若 uploadURL 不含 host，则根据 accountType 自动补全：
//   - PERSONAL（个人邮箱）: https://alimail-personal.aliyuncs.com
//   - 其他（企业邮箱）:     https://alimail-cn.aliyuncs.com
func httpPutMailAttachment(ctx context.Context, accountType string, uploadURL string, filePath string, fileSize int64) error {
	var fullURL string
	if strings.HasPrefix(uploadURL, "http://") || strings.HasPrefix(uploadURL, "https://") {
		fullURL = uploadURL
	} else {
		fullURL = mailUploadBaseURL(accountType) + uploadURL
	}

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, file)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}
	req.ContentLength = fileSize
	req.Header.Set("Content-Type", "application/octet-stream")

	client := mailHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("attachment upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("attachment upload failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func parseRecipients(raw string) []string {
	parts := strings.Split(raw, ",")
	recipients := make([]string, 0, len(parts))
	for _, p := range parts {
		addr := strings.TrimSpace(p)
		if addr != "" {
			recipients = append(recipients, addr)
		}
	}
	return recipients
}

// hasNonEmptyMailTemplateUpdate validates the actual RPC payload rather than
// flag Changed state. That keeps the CLI runtime contract aligned with the
// Schema require_one_of rule and rejects blank recipient lists as no-ops.
func hasNonEmptyMailTemplateUpdate(toolArgs map[string]any) bool {
	for _, name := range []string{"from", "subject", "body", "name"} {
		if value, ok := toolArgs[name].(string); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	for _, name := range []string{"toRecipients", "ccRecipients"} {
		if value, ok := toolArgs[name].([]string); ok && len(value) > 0 {
			return true
		}
	}
	return false
}

func validateMailboxThreadLimit(cmd *cobra.Command) (int, error) {
	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return 0, err
	}
	if limit <= 0 {
		return 0, fmt.Errorf("missing required flag --limit")
	}
	if limit > 100 {
		return 0, fmt.Errorf("--limit 必须为 1 到 100，收到: %d", limit)
	}
	return limit, nil
}

func validateMailboxThreadAction(cmd *cobra.Command, action string) error {
	validActions := map[string]bool{
		"markRead":   true,
		"markUnread": true,
		"addTags":    true,
		"removeTags": true,
	}
	if !validActions[action] {
		return fmt.Errorf("--action 必须为 markRead、markUnread、addTags 或 removeTags，收到: %s", action)
	}
	if action == "addTags" || action == "removeTags" {
		if flagOrFallback(cmd, "tag-ids", "tags") == "" {
			return fmt.Errorf("missing required flag --tag-ids")
		}
	}
	return nil
}

// ──────────────────────────────────────────────────────────
// 邮件附件下载编排
// ──────────────────────────────────────────────────────────

// parseMailDownloadSession 从 create_download_session MCP tool 响应中提取下载 URL。
// 读取 downloadUrl 字段（含完整 URL）。
// 统一返回 rawURL，由调用方（httpGetMailAttachment）负责在缺少 host 时补全。
func parseMailDownloadSession(text string) (string, error) {
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return "", fmt.Errorf("failed to parse download session response: %w", err)
	}
	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}
	rawURL, _ := data["downloadUrl"].(string)
	if rawURL == "" {
		return "", fmt.Errorf("create_download_session response missing downloadUrl: %s", text)
	}
	return rawURL, nil
}

// httpGetMailAttachment 通过 HTTP GET 下载附件内容并保存到本地文件。
// downloadURL 可以是完整 URL（含 host），也可以是相对路径（/v2/stream/{id}）。
// 若 downloadURL 不含 host，则根据 accountType 自动补全：
//   - PERSONAL（个人邮箱）: https://alimail-personal.aliyuncs.com
//   - 其他（企业邮箱）:     https://alimail-cn.aliyuncs.com
func httpGetMailAttachment(ctx context.Context, accountType string, downloadURL string, destPath string) error {
	var fullURL string
	if strings.HasPrefix(downloadURL, "http://") || strings.HasPrefix(downloadURL, "https://") {
		fullURL = downloadURL
	} else {
		fullURL = mailUploadBaseURL(accountType) + downloadURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}

	client := mailHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("attachment download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("attachment download failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create local file %s: %w", destPath, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to write attachment to %s: %w", destPath, err)
	}
	return nil
}

// runMailAttachmentDownload 执行附件下载编排流程：
//  1. 查询邮箱类型 → 确定下载域名
//  2. 调用 create_download_session → 获取 stream id
//  3. HTTP GET 下载附件内容 → 保存到本地
func runMailAttachmentDownload(cmd *cobra.Command) error {
	email := mustGetFlag(cmd, "email")
	messageId := mustGetFlag(cmd, "message-id")
	attachmentId := mustGetFlag(cmd, "attachment-id")
	name := mustGetFlag(cmd, "name")
	outputDir, _ := cmd.Flags().GetString("output")

	destPath := filepath.Join(outputDir, name)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Step 1: 查询邮箱类型（个人/企业），用于确定附件下载域名
	mailboxText, err := callMCPToolReturnText(ctx, "list_user_mailboxes", nil)
	if err != nil {
		return fmt.Errorf("查询邮箱列表失败: %w", err)
	}
	accountType := parseMailAccountType(mailboxText, email)

	// Step 2: 创建下载会话，获取 stream id
	sessionArgs := map[string]any{
		"email":        email,
		"messageId":    messageId,
		"attachmentId": attachmentId,
	}
	sessionText, err := callMCPToolReturnText(ctx, "create_download_session", sessionArgs)
	if err != nil {
		return fmt.Errorf("创建下载会话失败: %w", err)
	}
	downloadURL, err := parseMailDownloadSession(sessionText)
	if err != nil {
		return fmt.Errorf("解析下载会话信息失败: %w", err)
	}

	// Step 3: HTTP GET 下载附件内容并保存到本地
	if err := mailGetAttachment(ctx, accountType, downloadURL, destPath); err != nil {
		return fmt.Errorf("下载附件失败: %w", err)
	}

	deps.Out.PrintInfo(fmt.Sprintf("附件已保存到: %s", destPath))
	return nil
}

func sanitizeMailFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "\x00", "")
	if name == "" {
		return "mail"
	}
	return name
}

// mailAtomicLink is the no-clobber commit for atomicWriteFile (test-injectable).
var mailAtomicLink = os.Link

// atomicWriteFile 原子写入文件：先写同目录临时文件，成功后提交到目标路径。
// overwrite=false 使用 link(2) 实现存在即失败；overwrite=true 使用 rename 覆盖。
func atomicWriteFile(path string, data []byte, perm os.FileMode, overwrite bool) error {
	dir := filepath.Dir(path)
	tmp, err := atomicCreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = tmp.Close()
			_ = atomicRemove(tmpName)
		}
	}()
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("设置文件权限失败: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("写入数据失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("同步磁盘失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}
	if overwrite {
		if err := atomicRename(tmpName, path); err != nil {
			return fmt.Errorf("重命名文件失败: %w", err)
		}
		success = true
		return nil
	}
	if err := mailAtomicLink(tmpName, path); err != nil {
		return err
	}
	_ = atomicRemove(tmpName)
	success = true
	return nil
}
