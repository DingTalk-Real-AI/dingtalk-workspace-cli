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

	mailboxCmd.AddCommand(mailboxListCmd)

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
			if sizeVal == "" {
				sizeVal = "20"
			}
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
			if folderId == "" {
				folderId = "2" // 默认收件箱
			}
			query := fmt.Sprintf("folderId:%s", folderId)
			sizeVal := flagOrFallback(cmd, "limit", "size", "page-size")
			if sizeVal == "" {
				sizeVal = "20"
			}
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
			if !cmd.Flags().Changed("yes") {
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
			if !cmd.Flags().Changed("yes") {
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

	messageVerifyCmd.Flags().String("email", "", "邮件所属邮箱地址 (必填)")
	messageVerifyCmd.Flags().String("internet-message-id", "", "邮件的 internetMessageId (必填)，取自发送类命令返回值")

	messageCmd.AddCommand(messageListCmd, messageSearchCmd, messageGetCmd, messageSendCmd,
		messageReplyCmd, messageReplyAllCmd, messageForwardCmd,
		messageBatchMoveCmd, messageBatchDeleteCmd, messageBatchModifyCmd, messageVerifyCmd)

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
			err := callMCPTool("update_user_message_template", toolArgs)
			if cliErr, ok := err.(*CLIError); ok && strings.Contains(cliErr.Message, "Invalid parameter") {
				cliErr.Suggestion = "邮箱服务端仅支持更新草稿模板 (创建时带 --is-draft)；非草稿模板不可修改，请先 dws mail template delete 后用 --is-draft 重建"
			}
			return err
		},
	}

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

	// TODO: auto-reply update 暂时注释，待服务端修复后恢复
	// autoReplyUpdateCmd := &cobra.Command{
	// 	Use:   "update",
	// 	Short: "更新/设置用户的自动回复配置",
	// 	Long: `更新或设置用户的邮件自动回复配置。所有参数均为必填。
	//
	// 建议工作流：先通过 auto-reply get 获取当前配置，再传入需要修改的字段值。
	//
	// 时间格式示例：2026/06/25 16:30:00 +0800
	//
	// 参数说明（全部必填）：
	//   --enabled    是否启用自动回复 (true/false)
	//   --startTime  自动回复开始时间
	//   --endTime    自动回复结束时间
	//   --scope      回复范围: "contact"(仅联系人) 或 "all"(所有人)
	//   --content    自动回复内容
	//
	// 错误说明：
	//   domain.notFound  该用户的邮箱不是由钉钉邮箱托管，无法完成操作`,
	// 	Example: `  dws mail auto-reply update --email user@company.com --enabled true --startTime "2026/07/01 09:00:00 +0800" --endTime "2026/07/07 18:00:00 +0800" --scope all --content "出差中，请稍后联系"
	//   dws mail auto-reply update --email user@company.com --enabled false --startTime "" --endTime "" --scope all --content ""`,
	// 	RunE: func(cmd *cobra.Command, args []string) error {
	// 		if err := validateRequiredFlags(cmd, "email", "enabled", "startTime", "endTime", "scope", "content"); err != nil {
	// 			return err
	// 		}
	// 		toolArgs := map[string]any{
	// 			"email":     mustGetFlag(cmd, "email"),
	// 			"enabled":   mustGetFlag(cmd, "enabled") == "true",
	// 			"startTime": mustGetFlag(cmd, "startTime"),
	// 			"endTime":   mustGetFlag(cmd, "endTime"),
	// 			"scope":     mustGetFlag(cmd, "scope"),
	// 			"content":   mustGetFlag(cmd, "content"),
	// 		}
	// 		return callMCPTool("update_auto_reply", toolArgs)
	// 	},
	// }

	autoReplyGetCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	// autoReplyUpdateCmd.Flags().String("email", "", "用户的邮箱地址 (必填)")
	// autoReplyUpdateCmd.Flags().String("enabled", "", "是否启用自动回复: true/false (必填)")
	// autoReplyUpdateCmd.Flags().String("startTime", "", "自动回复开始时间 (必填)，格式: 2026/06/25 16:30:00 +0800")
	// autoReplyUpdateCmd.Flags().String("endTime", "", "自动回复结束时间 (必填)，格式: 2026/06/25 16:30:00 +0800")
	// autoReplyUpdateCmd.Flags().String("scope", "", "回复范围: contact(仅联系人)/all(所有人) (必填)")
	// autoReplyUpdateCmd.Flags().String("content", "", "自动回复内容 (必填)")

	autoReplyCmd.AddCommand(autoReplyGetCmd) // , autoReplyUpdateCmd)

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

	root.AddCommand(mailboxCmd, messageCmd, draftCmd, threadCmd, folderCmd, tagCmd, userCmd, attachmentCmd, templateCmd, contactCmd, autoReplyCmd, ruleCmd)

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
		if err := httpPutMailAttachment(ctx, accountType, uploadURL, f.path, f.size); err != nil {
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
		if err := httpPutMailAttachment(ctx, accountType, uploadURL, f.path, f.size); err != nil {
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

	client := &http.Client{Timeout: 5 * time.Minute}
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

	client := &http.Client{Timeout: 5 * time.Minute}
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
	if err := httpGetMailAttachment(ctx, accountType, downloadURL, destPath); err != nil {
		return fmt.Errorf("下载附件失败: %w", err)
	}

	deps.Out.PrintInfo(fmt.Sprintf("附件已保存到: %s", destPath))
	return nil
}
