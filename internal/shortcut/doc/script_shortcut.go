// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0
package doc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	ast "github.com/yuin/goldmark/ast"
	gmtext "github.com/yuin/goldmark/text"
)

var Script = shortcut.Shortcut{
	Service: "doc", Command: "+script", Product: productDoc,
	Description:   "初始化本地文档草稿，或检查Markdown/JSONML结构和字数",
	Intent:        "需要先在本地建立草稿、查看结构统计并检查字数和必需块时使用；不会上传或修改在线文档。在线文档可用--node读取后检查。XML不属于本入口支持的格式。",
	Risk:          shortcut.RiskWrite,
	Safety:        contract.SafetySpec{Effect: "write", Risk: "low", Confirmation: "not_required", Idempotency: "unknown"},
	OutputRollout: output.RolloutUnifiedActive,
	Contract:      scriptContract(),
	Flags: []shortcut.Flag{
		{Name: "command", Type: shortcut.FlagString, Required: true, Enum: []string{"init-draft", "parse"}, Desc: "本地动作；init-draft创建独立目录，parse只检查内容；parse恰好提供content或node；init-draft不接受二者；字数上下界非负且顺序正确"},
		{Name: "content", Type: shortcut.FlagString, Desc: docContentInputDescription + "；parse恰好提供content或node；init-draft不接受二者；字数上下界非负且顺序正确"},
		{Name: "node", Type: shortcut.FlagString, Desc: "parse可读取的文档ID/URL；与content互斥；parse恰好提供content或node；init-draft不接受二者；字数上下界非负且顺序正确"},
		{Name: "doc-format", Type: shortcut.FlagString, Default: "markdown", Enum: []string{"markdown", "jsonml"}, Desc: "草稿或解析内容格式；不支持DocxXML"},
		{Name: "min-words", Type: shortcut.FlagInt, Desc: "最少字数，0不限制；汉字逐字、其他字母数字连续词计数；parse恰好提供content或node；init-draft不接受二者；字数上下界非负且顺序正确"},
		{Name: "max-words", Type: shortcut.FlagInt, Desc: "最多字数，0不限制；parse恰好提供content或node；init-draft不接受二者；字数上下界非负且顺序正确"},
		{Name: "required-blocks", Type: shortcut.FlagStringSlice, Desc: "要求至少出现一次的块类型，如heading,paragraph,table（Markdown）或h1,p,table（JSONML）"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"command", "content", "node", "min-words", "max-words"}, Description: "parse恰好提供content或node；init-draft不接受二者；字数上下界非负且顺序正确"}},
	Validate:    validateDocScript, Execute: executeDocScript,
}

func scriptContract() corecmd.ContractDecl {
	d := docContract("+script", "初始化本地草稿或检查文档结构", "本地准备Markdown/JSONML草稿、检查字数与必需块；没有上传写入", []string{`dws doc +script --command init-draft`, `dws doc +script --command parse --content @draft.md --required-blocks heading`})
	d.DryRun = &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewPlan, RemoteReads: false}
	d.Result = &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","properties":{"preview_kind":{"type":"string","description":"dry-run的计划类型plan"},"command":{"type":"string","description":"本地操作"},"executed":{"type":"boolean","description":"是否执行实际动作"},"workspace":{"type":"string","description":"新建的相对草稿目录"},"draft_path":{"type":"string","description":"可编辑的相对草稿路径"},"profile":{"type":"object","description":"字数和块结构统计"},"assessment":{"type":"string","description":"结构检查状态passed或failed"}}}`)}
	return d
}
func validateDocScript(rt *shortcut.RuntimeContext) error {
	if rt.Int("min-words") < 0 || rt.Int("max-words") < 0 || (rt.Int("max-words") > 0 && rt.Int("min-words") > rt.Int("max-words")) {
		return apperrors.NewValidation("字数范围无效")
	}
	if rt.Str("command") == "init-draft" {
		if rt.Changed("content") || rt.Changed("node") {
			return apperrors.NewValidation("init-draft不接受content/node")
		}
		return nil
	}
	if rt.Changed("node") && strings.TrimSpace(rt.Str("node")) == "" {
		return apperrors.NewValidation("node不能为空")
	}
	if rt.Changed("node") == rt.Changed("content") {
		return apperrors.NewValidation("parse必须且只能提供content或node")
	}
	return nil
}
func executeDocScript(rt *shortcut.RuntimeContext) error {
	if rt.DryRun() {
		return rt.Output(map[string]any{"command": rt.Str("command"), "executed": false, "preview_kind": "plan"})
	}
	if rt.Str("command") == "init-draft" {
		cwd, err := docGetwd()
		if err != nil {
			return err
		}
		dir, err := docMkdirTemp(cwd, "doc-draft-")
		if err != nil {
			return err
		}
		suffix := ".md"
		initial := ""
		if rt.Str("doc-format") == "jsonml" {
			suffix = ".json"
			initial = `["root",{}]`
		}
		draft := filepath.Join(dir, "draft"+suffix)
		if err := os.WriteFile(draft, []byte(initial), 0600); err != nil {
			_ = os.RemoveAll(dir)
			return err
		}
		return rt.Output(map[string]any{"command": "init-draft", "executed": true, "workspace": filepath.Base(dir), "draft_path": filepath.ToSlash(filepath.Join(filepath.Base(dir), "draft"+suffix))})
	}
	content := ""
	var err error
	if rt.Changed("node") {
		nodeID := strings.TrimSpace(rt.Str("node"))
		if strings.Contains(nodeID, "://") {
			info, err := rt.CallMCPData(productDoc, "get_document_info", map[string]any{"nodeId": nodeID})
			if err != nil {
				return err
			}
			nodeID = nestedString(info, "nodeId")
			if nodeID == "" {
				return apperrors.NewAPI("文档链接解析未返回准确nodeId")
			}
		}
		data, e := rt.CallMCPData(productDoc, "get_document_content", map[string]any{"nodeId": nodeID, "format": rt.Str("doc-format")})
		if e != nil {
			return e
		}
		if actual := nestedString(data, "nodeId"); actual != "" && actual != nodeID {
			return apperrors.NewAPI("文档读取返回的身份与请求不一致")
		}
		content = nestedString(data, rt.Str("doc-format"))
		if content == "" {
			return apperrors.NewAPI("文档读取缺少所需内容字段")
		}
	} else {
		content, err = readShortcutContent(rt, "content")
		if err != nil {
			return err
		}
	}
	if len(content) > docMarkdownVerifyMax {
		return apperrors.NewValidation("草稿检查上限2MiB")
	}
	profile, err := docScriptProfile(content, rt.Str("doc-format"))
	if err != nil {
		return err
	}
	count := profile["word_count"].(int)
	types := profile["blocks"].(map[string]int)
	missing := []string{}
	for _, kind := range stringSliceNonEmpty(rt.StrSlice("required-blocks")) {
		if types[kind] == 0 {
			missing = append(missing, kind)
		}
	}
	if (rt.Int("min-words") > 0 && count < rt.Int("min-words")) || (rt.Int("max-words") > 0 && count > rt.Int("max-words")) || len(missing) > 0 {
		return apperrors.NewValidation("文档结构检查未通过", apperrors.WithDetails(map[string]any{"assessment": "failed", "profile": profile, "missingBlocks": missing}))
	}
	return rt.Output(map[string]any{"command": "parse", "executed": true, "assessment": "passed", "profile": profile})
}
func docScriptProfile(content, format string) (map[string]any, error) {
	types := map[string]int{}
	var text strings.Builder
	if format == "jsonml" {
		var tree any
		if err := json.Unmarshal([]byte(content), &tree); err != nil {
			return nil, apperrors.NewValidation("JSONML解析失败")
		}
		if _, err := validateJSONMLElement(content); err != nil {
			return nil, err
		}
		var structureErr error
		var walk func(any)
		walk = func(v any) {
			switch v := v.(type) {
			case string:
				text.WriteString(v)
			case []any:
				if len(v) < 2 {
					structureErr = apperrors.NewValidation("JSONML元素缺少标签或属性对象")
					return
				}
				if tag, ok := v[0].(string); !ok || strings.TrimSpace(tag) == "" {
					structureErr = apperrors.NewValidation("JSONML标签必须是非空字符串")
					return
				}
				if _, ok := v[1].(map[string]any); !ok {
					structureErr = apperrors.NewValidation("JSONML属性必须是对象")
					return
				}
				tag, _ := v[0].(string)
				if tag != "root" && tag != "fragment" && tag != "span" && tag != "a" && tag != "b" && tag != "i" && tag != "strong" && tag != "em" {
					types[tag]++
				}
				for _, child := range v[2:] {
					walk(child)
				}
				if tag != "span" && tag != "a" && tag != "b" && tag != "i" && tag != "strong" && tag != "em" {
					text.WriteByte(' ')
				}
			default:
				structureErr = apperrors.NewValidation("JSONML子元素必须是文本或元素数组")
			}
		}
		walk(tree)
		if structureErr != nil {
			return nil, structureErr
		}
	} else {
		data := []byte(content)
		tree := docMarkdown.Parser().Parse(gmtext.NewReader(data))
		_ = ast.Walk(tree, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
			if !enter {
				if n.Type() == ast.TypeBlock {
					text.WriteByte(' ')
				}
				return ast.WalkContinue, nil
			}
			if n.Type() == ast.TypeBlock && n.Kind() != ast.KindDocument {
				types[strings.ToLower(n.Kind().String())]++
			}
			if t, ok := n.(*ast.Text); ok {
				text.Write(t.Segment.Value(data))
				if t.SoftLineBreak() || t.HardLineBreak() {
					text.WriteByte(' ')
				}
			}
			switch block := n.(type) {
			case *ast.CodeBlock:
				text.Write(block.Lines().Value(data))
			case *ast.FencedCodeBlock:
				text.Write(block.Lines().Value(data))
			}
			return ast.WalkContinue, nil
		})
	}
	blockCount := 0
	for _, n := range types {
		blockCount += n
	}
	wordCount := 0
	inWord := false
	for _, c := range text.String() {
		if unicode.Is(unicode.Han, c) {
			wordCount++
			inWord = false
		} else if unicode.IsLetter(c) || unicode.IsDigit(c) {
			if !inWord {
				wordCount++
			}
			inWord = true
		} else {
			inWord = false
		}
	}
	return map[string]any{"word_count": wordCount, "char_count": len([]rune(text.String())), "block_count": blockCount, "blocks": types}, nil
}
func init() { registerDocShortcuts(Script) }
