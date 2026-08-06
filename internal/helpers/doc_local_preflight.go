package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// attachDocLocalPreflight wraps every doc leaf before its RunE reaches MCP.
// Only facts provable from local argv/filesystem state belong here; resource
// existence, permissions and business state remain server-owned.
func attachDocLocalPreflight(root *cobra.Command) {
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			walk(child)
		}
		if cmd.RunE == nil {
			return
		}
		original := cmd.RunE
		cmd.RunE = func(c *cobra.Command, args []string) error {
			if err := validateDocLocalArgs(c); err != nil {
				return err
			}
			return original(c, args)
		}
	}
	walk(root)
}

func docLocalError(cmd *cobra.Command, code, message, suggestion string) error {
	return &CLIError{Code: code, Message: message, Suggestion: suggestion, Operation: strings.TrimPrefix(cmd.CommandPath(), "dws ")}
}

func docRequire(cmd *cobra.Command, example string, names ...string) error {
	missing := make([]string, 0, len(names))
	for _, name := range names {
		flag := cmd.Flags().Lookup(name)
		if flag == nil || strings.TrimSpace(flag.Value.String()) == "" {
			missing = append(missing, "--"+name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return docLocalError(cmd, CodeMissingParam, "缺少必填参数: "+strings.Join(missing, ", "), "示例: "+example)
}

func docRequireNode(cmd *cobra.Command, example string) error {
	if flagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id") != "" {
		return nil
	}
	return docLocalError(cmd, CodeMissingParam, "缺少目标文档 --node（可传文档 URL 或 nodeId）", "先用 dws drive search 或 dws wiki node search 获取 nodeId。示例: "+example)
}

func docValidateLocalFile(cmd *cobra.Command, flagName, example string) error {
	path, _ := cmd.Flags().GetString(flagName)
	if strings.TrimSpace(path) == "" {
		return docRequire(cmd, example, flagName)
	}
	info, err := os.Stat(path)
	if err != nil {
		return docLocalError(cmd, CodeFileNotFound, fmt.Sprintf("本地文件 %q 不可读取", path), "检查路径后重试。示例: "+example)
	}
	if info.IsDir() {
		return docLocalError(cmd, CodeInvalidPath, fmt.Sprintf("%q 是目录，不是文件", path), "请传入具体文件路径。示例: "+example)
	}
	return nil
}

func docValidateWhere(cmd *cobra.Command, example string) error {
	where, _ := cmd.Flags().GetString("where")
	ref, _ := cmd.Flags().GetString("ref-block")
	if where != "" && where != "before" && where != "after" {
		return docLocalError(cmd, CodeInvalidParam, fmt.Sprintf("--where %q 无效，仅支持 before 或 after", where), "示例: "+example)
	}
	if where != "" && ref == "" {
		return docLocalError(cmd, CodeMissingParam, "使用 --where 时必须同时提供 --ref-block", "先用 dws doc block list --node <DOC_ID> 获取 blockId。示例: "+example)
	}
	return nil
}

func docValidateEnum(cmd *cobra.Command, name string, allowed []string, example string) error {
	value, _ := cmd.Flags().GetString(name)
	if value == "" {
		return nil
	}
	for _, candidate := range allowed {
		if strings.EqualFold(value, candidate) {
			return nil
		}
	}
	return docLocalError(cmd, CodeInvalidParam, fmt.Sprintf("--%s %q 无效，仅支持 %s", name, value, strings.Join(allowed, "、")), "示例: "+example)
}

func docValidateLimit(cmd *cobra.Command, max int, example string) error {
	for _, name := range []string{"limit", "page-size", "max-results"} {
		if flag := cmd.Flags().Lookup(name); flag != nil && cmd.Flags().Changed(name) {
			value, err := cmd.Flags().GetInt(name)
			if err == nil && (value < 1 || value > max) {
				return docLocalError(cmd, CodeInvalidParam, fmt.Sprintf("--%s 必须在 1 到 %d 之间", name, max), "示例: "+example)
			}
		}
	}
	return nil
}

func docValidateBlockContent(cmd *cobra.Command, example string) error {
	changed := make([]string, 0, 3)
	for _, name := range []string{"text", "heading", "element"} {
		if flag := cmd.Flags().Lookup(name); flag != nil && cmd.Flags().Changed(name) && strings.TrimSpace(flag.Value.String()) != "" {
			changed = append(changed, "--"+name)
		}
	}
	if len(changed) == 0 {
		return docLocalError(cmd, CodeMissingParam, "必须提供一种块内容：--text、--heading 或 --element", "示例: "+example)
	}
	if len(changed) > 1 {
		return docLocalError(cmd, CodeInvalidParam, "块内容参数不能同时使用: "+strings.Join(changed, ", "), "三选一。示例: "+example)
	}
	format, _ := cmd.Flags().GetString("content-format")
	if format == "jsonml" && changed[0] != "--element" {
		return docLocalError(cmd, CodeInvalidParam, "--content-format jsonml 必须通过 --element 提供 JSONML 节点", "示例: "+example)
	}
	return nil
}

func docValidatePermissionUsers(cmd *cobra.Command, example string) error {
	raw := flagOrFallback(cmd, "users", "user")
	users := parseCommentMentionIds(raw)
	if len(users) == 0 {
		return docLocalError(cmd, CodeMissingParam, "缺少至少一个用户 ID：--users", "示例: "+example)
	}
	if len(users) > 30 {
		return docLocalError(cmd, CodeInvalidParam, fmt.Sprintf("单次最多处理 30 个用户，当前为 %d 个", len(users)), "请拆分为多次调用")
	}
	return nil
}

func validateDocLocalArgs(cmd *cobra.Command) error {
	path := strings.TrimPrefix(cmd.CommandPath(), "dws ")
	switch path {
	case "doc info":
		return docRequireNode(cmd, "dws "+path+" --node <DOC_ID> --format json")
	case "doc search":
		if err := docValidateLimit(cmd, 30, "dws doc search --query \"周报\" --limit 10"); err != nil {
			return err
		}
		createdFrom, _ := cmd.Flags().GetInt64("created-from")
		createdTo, _ := cmd.Flags().GetInt64("created-to")
		visitedFrom, _ := cmd.Flags().GetInt64("visited-from")
		visitedTo, _ := cmd.Flags().GetInt64("visited-to")
		if createdFrom < 0 || createdTo < 0 || visitedFrom < 0 || visitedTo < 0 {
			return docLocalError(cmd, CodeInvalidParam, "时间过滤值必须是非负毫秒时间戳", "示例: dws doc search --created-from 1700000000000 --created-to 1710000000000")
		}
		if cmd.Flags().Changed("created-from") && cmd.Flags().Changed("created-to") && createdFrom > createdTo {
			return docLocalError(cmd, CodeInvalidParam, "--created-from 不能晚于 --created-to", "请交换起止时间")
		}
		if cmd.Flags().Changed("visited-from") && cmd.Flags().Changed("visited-to") && visitedFrom > visitedTo {
			return docLocalError(cmd, CodeInvalidParam, "--visited-from 不能晚于 --visited-to", "请交换起止时间")
		}
	case "doc list":
		return docValidateLimit(cmd, 50, "dws doc list --workspace <WORKSPACE_ID> --limit 50")
	case "doc read":
		if err := docRequireNode(cmd, "dws doc read --node <DOC_ID> --format json"); err != nil {
			return err
		}
		if depth, _ := cmd.Flags().GetInt("max-depth"); depth < 0 {
			return docLocalError(cmd, CodeInvalidParam, "--max-depth 不能为负数", "示例: dws doc read --node <DOC_ID> --content-format jsonml --scope outline --max-depth 3")
		}
		if scopeValue, _ := cmd.Flags().GetString("scope"); scopeValue != "" {
			valid := false
			for _, candidate := range []string{"outline", "range", "section", "tags"} {
				if scopeValue == candidate {
					valid = true
				}
			}
			if !valid {
				return docLocalError(cmd, CodeInvalidParam, fmt.Sprintf("invalid --scope %q: must be one of outline|range|section|tags", scopeValue), "示例: dws doc read --node <DOC_ID> --content-format jsonml --scope outline")
			}
		}
		format, _ := cmd.Flags().GetString("content-format")
		scope, _ := cmd.Flags().GetString("scope")
		tags, _ := cmd.Flags().GetString("tags")
		startBlockID, _ := cmd.Flags().GetString("start-block-id")
		if (scope == "range" || scope == "section") && strings.TrimSpace(startBlockID) == "" {
			return docLocalError(cmd, CodeMissingParam, "--scope "+scope+" 必须提供 --start-block-id", "先用 dws doc block list --node <DOC_ID> --format json 获取真实 blockId")
		}
		if cmd.Flags().Changed("output") && format != "jsonml" {
			return docLocalError(cmd, CodeInvalidParam, "--output 仅支持 --content-format jsonml", "Markdown 内容会直接显示在终端；如需保存为文件，请执行: dws doc read --node <DOC_ID> --content-format markdown --format raw > body.md")
		}
		if (scope != "" || tags != "") && format != "jsonml" {
			return docLocalError(cmd, CodeInvalidParam, "--scope/--tags requires --content-format jsonml", "示例: dws doc read --node <DOC_ID> --content-format jsonml --scope tags --tags h1,h2")
		}
		if scope == "tags" && strings.TrimSpace(tags) == "" {
			return docLocalError(cmd, CodeMissingParam, "--tags is required when --scope=tags", "示例: dws doc read --node <DOC_ID> --content-format jsonml --scope tags --tags h1,h2")
		}
		if scope != "tags" && tags != "" {
			if scope == "" {
				return docLocalError(cmd, CodeInvalidParam, "--tags requires --scope tags", "移除 --tags 或改用 --scope tags")
			}
			return docLocalError(cmd, CodeInvalidParam, "--tags only works with --scope tags", "移除 --tags 或改用 --scope tags")
		}
	case "doc create":
		if strings.TrimSpace(flagOrFallback(cmd, "name", "title")) == "" {
			return docLocalError(cmd, CodeMissingParam, "缺少文档名称 --name", "示例: dws doc create --name \"项目周报\" --format json")
		}
		if cmd.Flags().Changed("content") && cmd.Flags().Changed("content-file") {
			return docLocalError(cmd, CodeInvalidParam, "--content 与 --content-file 不能同时使用", "短文本用 --content；长文本或表格用 --content-file")
		}
		if cmd.Flags().Changed("content-file") {
			return docValidateLocalFile(cmd, "content-file", "dws doc create --name \"周报\" --content-file ./weekly.md")
		}
	case "doc update":
		if err := docRequireNode(cmd, "dws doc update --node <DOC_ID> --content \"追加内容\" --mode append"); err != nil {
			return err
		}
		if cmd.Flags().Changed("content") && cmd.Flags().Changed("content-file") {
			return docLocalError(cmd, CodeInvalidParam, "--content 与 --content-file 不能同时使用", "二选一；长内容优先 --content-file")
		}
		if cmd.Flags().Changed("content-file") {
			if err := docValidateLocalFile(cmd, "content-file", "dws doc update --node <DOC_ID> --content-file ./body.md --mode append"); err != nil {
				return err
			}
		}
		mode, _ := cmd.Flags().GetString("mode")
		if mode != "append" && mode != "overwrite" {
			return docLocalError(cmd, CodeInvalidParam, "--mode 必须是 append 或 overwrite", "追加优先使用: dws doc update --node <DOC_ID> --content \"内容\" --mode append")
		}
		if idx, _ := cmd.Flags().GetInt("index"); cmd.Flags().Changed("index") && (mode != "append" || idx < 0) {
			return docLocalError(cmd, CodeInvalidParam, "--index 仅支持 mode=append 且必须大于等于 0", "示例: dws doc update --node <DOC_ID> --content \"内容\" --mode append --index 0")
		}
		content := flagOrFallback(cmd, "content", "markdown")
		if strings.TrimSpace(content) == "" && !cmd.Flags().Changed("content-file") {
			return docLocalError(cmd, CodeMissingParam, "必须通过 --content 或 --content-file 提供非空内容", "示例: dws doc update --node <DOC_ID> --content \"追加内容\" --mode append")
		}
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		yes, _ := cmd.Flags().GetBool("yes")
		if dryRun && mode != "overwrite" {
			return docLocalError(cmd, CodeInvalidParam, "--dry-run 仅用于预览 overwrite 覆盖写入", "append 本身不覆盖全文，请移除 --dry-run")
		}
		if yes && mode != "overwrite" {
			return docLocalError(cmd, CodeInvalidParam, "--yes 仅用于确认 overwrite 覆盖写入", "append 无需 --yes，请移除该参数")
		}
		if mode == "overwrite" && !dryRun && !yes {
			return docLocalError(cmd, CodeInvalidParam, "--mode overwrite 必须加 --yes 确认，或使用 --dry-run 预览", "示例: dws doc update --node <DOC_ID> --content-file ./body.md --mode overwrite --dry-run")
		}
	case "doc block list":
		if err := docRequireNode(cmd, "dws doc block list --node <DOC_ID> --format json"); err != nil {
			return err
		}
		start, _ := cmd.Flags().GetInt("start-index")
		end, _ := cmd.Flags().GetInt("end-index")
		if start < 0 || end < 0 || (cmd.Flags().Changed("end-index") && end < start) {
			return docLocalError(cmd, CodeInvalidParam, "块索引必须非负，且 --end-index 不能小于 --start-index", "示例: dws doc block list --node <DOC_ID> --start-index 0 --end-index 5")
		}
	case "doc block insert":
		if err := docRequireNode(cmd, "dws doc block insert --node <DOC_ID> --text \"内容\""); err != nil {
			return err
		}
		if err := docValidateWhere(cmd, "dws doc block insert --node <DOC_ID> --text \"内容\" --ref-block <BLOCK_ID> --where after"); err != nil {
			return err
		}
		if level, _ := cmd.Flags().GetInt("level"); level < 1 || level > 6 {
			return docLocalError(cmd, CodeInvalidParam, "--level 必须在 1 到 6 之间", "示例: dws doc block insert --node <DOC_ID> --heading \"标题\" --level 2")
		}
		if idx, _ := cmd.Flags().GetInt("index"); cmd.Flags().Changed("index") && idx < 0 {
			return docLocalError(cmd, CodeInvalidParam, "--index 不能为负数", "示例: dws doc block insert --node <DOC_ID> --text \"内容\" --index 0")
		}
		return docValidateBlockContent(cmd, "dws doc block insert --node <DOC_ID> --text \"内容\"")
	case "doc block update":
		if err := docRequireNode(cmd, "dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --text \"新内容\""); err != nil {
			return err
		}
		if err := docRequire(cmd, "dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --text \"新内容\"", "block-id"); err != nil {
			return err
		}
		if level, _ := cmd.Flags().GetInt("level"); level < 1 || level > 6 {
			return docLocalError(cmd, CodeInvalidParam, "--level 必须在 1 到 6 之间", "示例: dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --heading \"标题\" --level 2")
		}
		return docValidateBlockContent(cmd, "dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --text \"新内容\"")
	case "doc block delete":
		if err := docRequireNode(cmd, "dws doc block delete --node <DOC_ID> --block-id <BLOCK_ID> --yes"); err != nil {
			return err
		}
		return docRequire(cmd, "dws doc block delete --node <DOC_ID> --block-id <BLOCK_ID> --yes", "block-id")
	case "doc media download":
		if err := docRequireNode(cmd, "dws doc media download --node <DOC_ID> --resource-id <RESOURCE_ID>"); err != nil {
			return err
		}
		return docRequire(cmd, "dws doc media download --node <DOC_ID> --resource-id <RESOURCE_ID>", "resource-id")
	case "doc media insert":
		if err := docRequireNode(cmd, "dws doc media insert --node <DOC_ID> --file ./report.pdf"); err != nil {
			return err
		}
		if err := docValidateWhere(cmd, "dws doc media insert --node <DOC_ID> --file ./report.pdf --ref-block <BLOCK_ID> --where after"); err != nil {
			return err
		}
		return docValidateLocalFile(cmd, "file", "dws doc media insert --node <DOC_ID> --file ./report.pdf")
	case "doc comment list":
		if err := docRequireNode(cmd, "dws doc comment list --node <DOC_ID> --format json"); err != nil {
			return err
		}
		if err := docValidateLimit(cmd, 50, "dws doc comment list --node <DOC_ID> --limit 50"); err != nil {
			return err
		}
		if err := docValidateEnum(cmd, "type", []string{"global", "inline"}, "dws doc comment list --node <DOC_ID> --type inline"); err != nil {
			return err
		}
		return docValidateEnum(cmd, "resolve-status", []string{"resolved", "unresolved"}, "dws doc comment list --node <DOC_ID> --resolve-status unresolved")
	case "doc comment create":
		if err := docRequireNode(cmd, "dws doc comment create --node <DOC_ID> --content \"评论\""); err != nil {
			return err
		}
		return docRequire(cmd, "dws doc comment create --node <DOC_ID> --content \"评论\"", "content")
	case "doc comment reply", "doc comment update":
		if err := docRequireNode(cmd, "dws "+path+" --node <DOC_ID> --comment-key <COMMENT_KEY> --content \"内容\""); err != nil {
			return err
		}
		if err := docRequire(cmd, "dws "+path+" --node <DOC_ID> --comment-key <COMMENT_KEY> --content \"内容\"", "comment-key", "content"); err != nil {
			return err
		}
		if path == "doc comment reply" {
			emoji, _ := cmd.Flags().GetBool("emoji")
			groups, groupErr := commentGroupMentionIDs(cmd)
			if groupErr != nil {
				return groupErr
			}
			if emoji && len(groups) > 0 {
				return docLocalError(cmd, CodeInvalidParam, "--emoji cannot be used with --mentioned-open-conversation-id: emoji replies do not support group mentions", "表情回复请移除群 @；需要 @群时使用普通文字回复")
			}
		}
	case "doc comment delete":
		if err := docRequireNode(cmd, "dws doc comment delete --node <DOC_ID> --comment-key <COMMENT_KEY> --yes"); err != nil {
			return err
		}
		return docRequire(cmd, "dws doc comment delete --node <DOC_ID> --comment-key <COMMENT_KEY> --yes", "comment-key")
	case "doc comment create-inline":
		if err := docRequireNode(cmd, "dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --start 0 --end 5 --content \"评论\""); err != nil {
			return err
		}
		if err := docRequire(cmd, "dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --start 0 --end 5 --content \"评论\"", "block-id", "content"); err != nil {
			return err
		}
		start, _ := cmd.Flags().GetInt("start")
		end, _ := cmd.Flags().GetInt("end")
		if !cmd.Flags().Changed("start") || !cmd.Flags().Changed("end") || start < 0 || end <= start {
			return docLocalError(cmd, CodeInvalidParam, "划词范围必须显式提供 --start/--end，且满足 0 <= start < end", "先读取块文本确认字符偏移。示例: dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --start 0 --end 5 --content \"评论\"")
		}
	case "doc export":
		if err := docRequireNode(cmd, "dws doc export --node <DOC_ID> --export-format pdf --output ./report.pdf"); err != nil {
			return err
		}
		if err := docRequire(cmd, "dws doc export --node <DOC_ID> --export-format pdf --output ./report.pdf", "output"); err != nil {
			return err
		}
		return docValidateEnum(cmd, "export-format", []string{"docx", "markdown", "md", "pdf"}, "dws doc export --node <DOC_ID> --export-format pdf --output ./report.pdf")
	case "doc export get":
		return docRequire(cmd, "dws doc export get --job-id <JOB_ID>", "job-id")
	case "doc import":
		if err := docValidateLocalFile(cmd, "file", "dws doc import --file ./report.docx --workspace <WORKSPACE_ID>"); err != nil {
			return err
		}
		if !deps.Caller.DryRun() && flagOrFallback(cmd, "folder", "folder-id") == "" && flagOrFallback(cmd, "workspace", "workspace-id") == "" {
			return docLocalError(cmd, CodeMissingParam, "导入文档必须提供 --folder 或 --workspace 作为目标位置", "先用 dws wiki space list --type myWikiSpace --format json 获取 workspaceId")
		}
	case "doc import get":
		return docRequire(cmd, "dws doc import get --task-id <TASK_ID>", "task-id")
	case "doc version save", "doc version list", "doc version revert":
		if err := docRequireNode(cmd, "dws "+path+" --node <DOC_ID> --format json"); err != nil {
			return err
		}
		if path == "doc version revert" && (!cmd.Flags().Changed("version")) {
			return docLocalError(cmd, CodeMissingParam, "缺少目标版本 --version", "先用 dws doc version list --node <DOC_ID> 获取真实版本号")
		}
		if path == "doc version revert" {
			version, _ := cmd.Flags().GetInt("version")
			if version < 1 {
				return docLocalError(cmd, CodeInvalidParam, "--version 必须是大于 0 的真实版本号", "先用 dws doc version list --node <DOC_ID> 获取版本号")
			}
		}
		if path == "doc version list" {
			return docValidateLimit(cmd, 50, "dws doc version list --node <DOC_ID> --limit 10")
		}
	case "doc permission add", "doc permission update":
		if err := docRequireNode(cmd, "dws "+path+" --node <DOC_ID> --users uid1 --role EDITOR"); err != nil {
			return err
		}
		if err := docValidatePermissionUsers(cmd, "dws "+path+" --node <DOC_ID> --users uid1 --role EDITOR"); err != nil {
			return err
		}
		if err := docRequire(cmd, "dws "+path+" --node <DOC_ID> --users uid1 --role EDITOR", "role"); err != nil {
			return err
		}
		return docValidateEnum(cmd, "role", []string{"MANAGER", "EDITOR", "DOWNLOADER", "READER"}, "dws "+path+" --node <DOC_ID> --users uid1 --role EDITOR")
	case "doc permission remove":
		if err := docRequireNode(cmd, "dws doc permission remove --node <DOC_ID> --users uid1"); err != nil {
			return err
		}
		return docValidatePermissionUsers(cmd, "dws doc permission remove --node <DOC_ID> --users uid1")
	case "doc permission list":
		if err := docRequireNode(cmd, "dws doc permission list --node <DOC_ID> --limit 30"); err != nil {
			return err
		}
		if err := docValidateLimit(cmd, 200, "dws doc permission list --node <DOC_ID> --limit 30"); err != nil {
			return err
		}
		if roles, _ := cmd.Flags().GetString("filter-role"); roles != "" {
			for _, role := range parseRoleList(roles) {
				valid := false
				for _, candidate := range []string{"OWNER", "MANAGER", "EDITOR", "DOWNLOADER", "READER"} {
					if role == candidate {
						valid = true
					}
				}
				if !valid {
					return docLocalError(cmd, CodeInvalidParam, "--filter-role 包含非法角色 "+role, "仅支持 OWNER、MANAGER、EDITOR、DOWNLOADER、READER")
				}
			}
		}
	case "doc template list":
		if err := docValidateEnum(cmd, "source", []string{"MY", "PUBLIC"}, "dws doc template list --source MY"); err != nil {
			return err
		}
		return docValidateLimit(cmd, 50, "dws doc template list --limit 20")
	case "doc template search":
		if flagOrFallback(cmd, "query", "keyword", "name") == "" {
			return docLocalError(cmd, CodeMissingParam, "缺少非空模板关键词 --query", "示例: dws doc template search --query \"周报\" --format json")
		}
		if err := docValidateEnum(cmd, "source", []string{"MY", "PUBLIC"}, "dws doc template search --query \"周报\" --source PUBLIC"); err != nil {
			return err
		}
		return docValidateLimit(cmd, 50, "dws doc template search --query \"周报\" --limit 20")
	case "doc template apply":
		if flagOrFallback(cmd, "template-id", "template", "tpl-id") == "" {
			return docLocalError(cmd, CodeMissingParam, "缺少模板 ID --template-id", "先用 dws doc template search --query \"关键词\" 获取真实 templateId")
		}
	}
	return nil
}

func docLocalFileExtension(path string) string { return strings.ToLower(filepath.Ext(path)) }
