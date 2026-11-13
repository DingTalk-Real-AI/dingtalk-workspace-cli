// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
//
// Mono↔multi skill content QA (G1–G5). Spec: docs/skill-mono-multi-qa.md
// Contract: skills/content-qa/mono-multi-coverage.yaml

package unit_test

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type monoMultiQAContract struct {
	SchemaVersion int `yaml:"schema_version"`
	Coverage      []struct {
		Mono       string   `yaml:"mono"`
		MultiSkill string   `yaml:"multi_skill"`
		MultiRefs  []string `yaml:"multi_refs"`
	} `yaml:"coverage"`
	OmitCoverage []struct {
		Mono        string `yaml:"mono"`
		Disposition string `yaml:"disposition"`
		Via         string `yaml:"via"`
		Reason      string `yaml:"reason"`
	} `yaml:"omit_coverage"`
	GlobalProtocols []struct {
		ID        string `yaml:"id"`
		MultiPath string `yaml:"multi_path"`
	} `yaml:"global_protocols"`
	OmitGlobal []struct {
		ID            string `yaml:"id"`
		Disposition   string `yaml:"disposition"`
		Reason        string `yaml:"reason"`
		ExpectedMulti string `yaml:"expected_multi"`
	} `yaml:"omit_global"`
	PairedFiles []struct {
		Mono              string                      `yaml:"mono"`
		Multi             string                      `yaml:"multi"`
		Mode              string                      `yaml:"mode"`
		LinkSubstitutions []monoMultiLinkSubstitution `yaml:"link_substitutions"`
	} `yaml:"paired_files"`
	PairedTrees []struct {
		Mono  string `yaml:"mono"`
		Multi string `yaml:"multi"`
		Mode  string `yaml:"mode"`
	} `yaml:"paired_trees"`
	OrphanScriptsAllowlist []struct {
		Path        string `yaml:"path"`
		Disposition string `yaml:"disposition"`
		Reason      string `yaml:"reason"`
	} `yaml:"orphan_scripts_allowlist"`
	RetiredScripts []struct {
		Path   string `yaml:"path"`
		Reason string `yaml:"reason"`
	} `yaml:"retired_scripts"`
	SkillsWithoutReferencesAllowlist []string `yaml:"skills_without_references_allowlist"`
}

type monoMultiLinkSubstitution struct {
	Mono  string `yaml:"mono"`
	Multi string `yaml:"multi"`
}

type markdownLink struct {
	Line   int
	Target string
}

type markdownLinkIssue struct {
	Link     markdownLink
	Resolved string
	Err      error
}

var inlineMarkdownLinkPattern = regexp.MustCompile(`!?\[[^\]\n]*\]\(\s*(<[^>\n]+>|[^)\s\n]+)(?:\s+[^)\n]+)?\)`)

func loadMonoMultiQAContract(t *testing.T) (string, monoMultiQAContract) {
	t.Helper()
	root := repoRoot(t)
	path := filepath.Join(root, "skills", "content-qa", "mono-multi-coverage.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var c monoMultiQAContract
	if err := yaml.Unmarshal(data, &c); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	if c.SchemaVersion != 1 {
		t.Fatalf("unsupported schema_version %d", c.SchemaVersion)
	}
	return root, c
}

// Keep per-target final delivery and body links explicit on every AI Table Skill entry.
func TestMonoMultiSkillContentAITableHostDeliveryBoundary(t *testing.T) {
	root := repoRoot(t)
	for _, tt := range []struct {
		path    string
		heading string
	}{
		{"skills/mono/SKILL.md", "## 严格要求 (MUST DO)"},
		{"skills/multi/dingtalk-aitable/SKILL.md", "## 过程展示建议"},
		{"skills/mono/references/products/aitable.md", "## 过程展示建议"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, tt.path))
			if err != nil {
				t.Fatal(err)
			}
			_, section, ok := strings.Cut(string(data), tt.heading+"\n")
			if !ok {
				t.Fatalf("main Skill is missing %q", tt.heading)
			}
			section, _, _ = strings.Cut(section, "\n## ")
			for _, required := range []string{
				"当前有效、角色匹配入口",
				"逐一展示具名完整链接",
				"不合并为 Base",
				"不以父级",
				"仅有父级入口时明确说明，不冒充目标直达",
				"同对象同入口",
				"可去重",
				"URL 及参数",
				"逐字符原样保留",
				"身份匹配的 `links` 即使 `relation=parent` 仍是当前对象入口",
				"只有链接身份确属父级时才称父级入口",
				"从本条回执直接复制完整 URL",
				"不套用其他对象的 URL 替换 ID",
				"不手抄 JSON 充作源数据",
				"仅当存在明确识别为 JSON 格式的原始业务回执文件时",
				"CSV、XLS、XLSX 等非 JSON 导入附件不是业务回执，不作为 JSON 解析",
				"直接使用当前命令的结构化回执核对并展示链接",
				"不因非 JSON 附件阻断成功结果展示",
				"不重新键入、解码再编码或统一大小写",
				"同批链接各自保留编码",
				"实际待发送链接与本条回执逐字符比对",
				"一处不同就修正",
				"只声称原样保留不算核对",
				"`%3D` 不改为 `%3d`",
				"上传签名 URL、token 等凭据脱敏",
				"不编造名称",
				"名称须对应链接对象",
				"最终目标",
				"清点",
				"逐目标完成",
				"核对正文链接",
				"最终产物",
				"普通链接不替代登记",
				"过程已展示链接或宿主已呈现卡片，最终 Markdown 正文仍须保留最终目标的有效具名完整链接",
				"（`[对象名称](完整URL)`）",
				"过程去重不免除此要求",
				"缺少适用入口时说明缺口",
				"同对象同入口在过程内可去重",
				"业务核验、登记、展示",
				"成功登记不重复",
				"未知登记不自动重发",
				"缺链仅按工具明确规则及本次真实标识回退",
				"说明缺口",
				"不拼接",
				"不为补链接",
				"重放业务写入",
				"填写页",
				"交付给填写者且填写入口未就绪时，只说明缺口，不展示或登记管理页链接",
				"即使注明非填写入口，也不将管理页列为可交付内容",
				"只有用户明确要求管理操作或接收者为管理者时，才交付当前可用的管理页",
				"分享未知",
				"开启分享或扩大权限",
				"回执或展示请求不证明登记、渲染或面板已打开",
			} {
				if !strings.Contains(section, required) {
					t.Errorf("%s must publish %q in %s without loading references", tt.path, required, tt.heading)
				}
			}
			if strings.Index(section, "最终目标") >= strings.Index(section, "逐一展示具名完整链接") {
				t.Error("closure guidance must precede process display")
			}
			for _, forbidden := range []string{
				"[名称](完整URL)",
				"正文可不重复",
				"不重复卡片 URL",
				"同对象同入口在本任务内可去重",
				"无有效入口时说明缺口，不拼接",
				"当输入含有原始回执文件或请求附件时",
				"无有效入口时必须说明缺口，不拼接",
				"Base 缺链回退",
				"Base 列表／搜索缺链回退",
				"https://alidocs.dingtalk.com/i/nodes/{baseId}",
			} {
				if strings.Contains(section, forbidden) {
					t.Errorf("%s contains forbidden guidance %q", tt.path, forbidden)
				}
			}
			if strings.Contains(section, "show_link") || strings.Contains(section, "open_in_qwenwork") {
				t.Error("main Skill must leave host tool names to the host")
			}
			if tt.path == "skills/mono/SKILL.md" {
				for _, required := range []string{
					"规定工具与成功条件",
					"- **AI 表格链接保真必经步骤：**",
					"只读及明确不展示要求按宿主适用范围处理",
				} {
					if !strings.Contains(section, required) {
						t.Errorf("Mono MUST DO is missing %q", required)
					}
				}
				if !strings.Contains(string(data), "不预加载完整产品参考文件") {
					t.Error("Mono must retain progressive reference loading")
				}
			}
		})
	}
}

// TestMonoMultiSkillContentAITableBootstrapDeliveryBoundary keeps explicit table targets on the bootstrap route.
func TestMonoMultiSkillContentAITableBootstrapDeliveryBoundary(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "skills/multi/dingtalk-aitable/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "| 新建 Base 与整套表字段 |") {
			continue
		}
		for _, required := range []string{
			"aitable +base-bootstrap", "明确要求交付的表逐表核对登记及正文链接",
			"使用各表自身入口", "Base 不默认追加",
		} {
			if !strings.Contains(line, required) {
				t.Errorf("bootstrap route must publish %q", required)
			}
		}
		return
	}
	t.Fatal("AI Table bootstrap Golden Route is missing")
}

// Compare only the shared presentation section; G4 does not pair these entire files.
func TestMonoMultiSkillContentAITableProcessSectionMatches(t *testing.T) {
	root := repoRoot(t)
	var sections []string
	for _, path := range []string{
		"skills/multi/dingtalk-aitable/SKILL.md",
		"skills/mono/references/products/aitable.md",
	} {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		_, section, ok := strings.Cut(string(data), "## 过程展示建议\n")
		if !ok {
			t.Fatalf("%s: process section missing", path)
		}
		section, _, _ = strings.Cut(section, "\n## ")
		sections = append(sections, strings.TrimSpace(section))
		for _, required := range []string{
			"成功回执、`relation=target` 和过程对象不自动成为最终产物",
			"某目标恢复中不遗漏其他可交付目标",
			"用户要求完整汇总时保留各项完整链接及原状态",
			"内部自动对象不自动纳入",
			"用户要求的对象不因自动创建而排除",
			"过程展示与最终交付分别完成",
			"当前对象 `url`、身份匹配的 `links` 或已确认可用的 `shareFormUrl`",
			"`target/parent` 仅表调用关系",
			"独立读回优先，显式 null/空值不由旧回执覆盖",
			"编码和顺序",
			"不截断、改写或重建",
			"仅在无法区分时附 ID",
			"填写者只用已确认可用的填写页",
			"未分享或关闭时可向管理者交付可用管理页",
			"管理页，非填写入口",
			"`objectType=field`、`objectId=当前 fieldId`",
			"无自身链接时说明缺口，不展示错配链接，保留核验状态",
			"保留 pending/unknown/partial/STOP",
			"链接不证明成功、核验、权限、分享或流程启用",
			"失败、工具不可用或状态未知时说明缺口",
			"仅按宿主查询／安全恢复契约处理",
			"展示去重不裁剪业务回执及 `links`",
			"只使用可信完整预发 URL，前缀不证明环境映射",
			"缺少时保留业务结果并说明未完成，不拼接、换域或改交生产链接",
		} {
			if !strings.Contains(section, required) {
				t.Errorf("%s: missing process boundary %q", path, required)
			}
		}
	}
	if sections[0] != sections[1] {
		t.Error("AI Table process presentation differs between Mono and Multi")
	}
}

func TestMonoMultiSkillContentAITableReceiptStatusBoundary(t *testing.T) {
	root := repoRoot(t)
	for _, tt := range []struct {
		path    string
		heading string
	}{
		{"skills/mono/SKILL.md", "## 严格要求 (MUST DO)"},
		{"skills/multi/dingtalk-aitable/SKILL.md", "## 过程展示建议"},
		{"skills/mono/references/products/aitable.md", "## 过程展示建议"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, tt.path))
			if err != nil {
				t.Fatal(err)
			}
			_, section, ok := strings.Cut(string(data), tt.heading+"\n")
			if !ok {
				t.Fatalf("missing %q", tt.heading)
			}
			section, _, _ = strings.Cut(section, "\n## ")
			for _, required := range []string{
				"明确的合成演练或回执转述",
				"准备性 Schema 查询不将其转为契约评审",
				"按给定回执的字段语义转述",
				"不以本 Agent 是否亲自执行或独立读回改写该字段",
				"纯 JSON 也保持指定字段语义",
				"转述不等于真实实跑或独立核验",
				"真实写入仍须按结果契约读回",
				"缺失或 pending/unknown 的状态不得擅自提升为 verified",
			} {
				if !strings.Contains(section, required) {
					t.Errorf("%s must publish receipt provenance boundary %q in %s", tt.path, required, tt.heading)
				}
			}
		})
	}
}

func TestMonoMultiSkillContentG1Shape(t *testing.T) {
	root, c := loadMonoMultiQAContract(t)
	multiRoot := filepath.Join(root, "skills", "multi")
	entries, err := os.ReadDir(multiRoot)
	if err != nil {
		t.Fatal(err)
	}
	foundShared := false
	noRefs := map[string]bool{}
	for _, name := range c.SkillsWithoutReferencesAllowlist {
		noRefs[name] = true
	}
	for _, e := range entries {
		if !e.IsDir() {
			t.Errorf("G1: unexpected non-directory %s under skills/multi", e.Name())
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "dingtalk-") {
			t.Errorf("G1: invalid skill directory name %q (want dingtalk-*)", name)
		}
		if name == "dingtalk-shared" {
			foundShared = true
		}
		skillMD := filepath.Join(multiRoot, name, "SKILL.md")
		if _, err := os.Stat(skillMD); err != nil {
			t.Errorf("G1: missing SKILL.md for %s: %v", name, err)
		}
		refs := filepath.Join(multiRoot, name, "references")
		if _, err := os.Stat(refs); err != nil && !noRefs[name] {
			t.Errorf("G1: missing references/ for %s (add dir or skills_without_references_allowlist)", name)
		}
		if noRefs[name] {
			if _, err := os.Stat(refs); err == nil {
				t.Errorf("G1: %s is on skills_without_references_allowlist but references/ exists; remove allowlist entry", name)
			}
		}
	}
	if !foundShared {
		t.Error("G1: skills/multi must contain dingtalk-shared")
	}
}

func TestMonoMultiSkillContentG2Frontmatter(t *testing.T) {
	root, _ := loadMonoMultiQAContract(t)
	multiRoot := filepath.Join(root, "skills", "multi")
	entries, err := os.ReadDir(multiRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		path := filepath.Join(multiRoot, name, "SKILL.md")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		fm, err := parseSkillFrontmatter(body)
		if err != nil {
			t.Errorf("G2: %s: %v", name, err)
			continue
		}
		if fm.Name != name {
			t.Errorf("G2: %s: frontmatter name %q != directory", name, fm.Name)
		}
		if strings.TrimSpace(fm.Description) == "" {
			t.Errorf("G2: %s: empty description", name)
		}
		category := fm.Metadata.Category
		if category == "" {
			t.Errorf("G2: %s: missing metadata.category", name)
		} else if category != "product" && category != "shared" {
			t.Errorf("G2: %s: metadata.category %q want product|shared", name, category)
		}
		wantCat := "product"
		if name == "dingtalk-shared" {
			wantCat = "shared"
		}
		if category != "" && category != wantCat {
			t.Errorf("G2: %s: metadata.category %q want %q", name, category, wantCat)
		}
		bins := fm.Metadata.Requires.Bins
		hasDWS := false
		for _, b := range bins {
			if b == "dws" {
				hasDWS = true
				break
			}
		}
		if !hasDWS {
			t.Errorf("G2: %s: metadata.requires.bins must include dws", name)
		}
	}
}

func TestMonoMultiSkillContentG3Coverage(t *testing.T) {
	root, c := loadMonoMultiQAContract(t)
	products := filepath.Join(root, "skills", "mono", "references", "products")
	stems, err := monoProductStems(products)
	if err != nil {
		t.Fatal(err)
	}
	covered := map[string]bool{}
	for _, row := range c.Coverage {
		if row.Mono == "" || row.MultiSkill == "" {
			t.Fatalf("G3: coverage row missing mono/multi_skill: %+v", row)
		}
		if covered[row.Mono] {
			t.Errorf("G3: duplicate coverage for mono %q", row.Mono)
		}
		covered[row.Mono] = true
		skillDir := filepath.Join(root, "skills", "multi", row.MultiSkill)
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
			t.Errorf("G3: coverage %s → %s missing SKILL.md: %v", row.Mono, row.MultiSkill, err)
		}
		for _, rel := range row.MultiRefs {
			p := filepath.Join(skillDir, filepath.FromSlash(rel))
			if _, err := os.Stat(p); err != nil {
				t.Errorf("G3: coverage %s → %s missing %s: %v", row.Mono, row.MultiSkill, rel, err)
			}
		}
	}
	omitted := map[string]bool{}
	for _, row := range c.OmitCoverage {
		if row.Mono == "" || strings.TrimSpace(row.Reason) == "" || strings.TrimSpace(row.Disposition) == "" {
			t.Fatalf("G3: omit_coverage requires mono, disposition, reason: %+v", row)
		}
		omitted[row.Mono] = true
	}
	for stem := range stems {
		if covered[stem] || omitted[stem] {
			continue
		}
		t.Errorf("G3: mono product stem %q not in coverage or omit_coverage", stem)
	}
	for stem := range covered {
		if !stems[stem] {
			t.Errorf("G3: coverage mono %q has no matching skills/mono/references/products entry", stem)
		}
	}
	for stem := range omitted {
		if !stems[stem] {
			t.Errorf("G3: omit_coverage mono %q has no matching products entry", stem)
		}
	}
}

func TestMonoMultiSkillContentG4Drift(t *testing.T) {
	root, c := loadMonoMultiQAContract(t)
	multiRoot := filepath.Join(root, "skills", "multi")

	for _, g := range c.GlobalProtocols {
		p := filepath.Join(multiRoot, filepath.FromSlash(g.MultiPath))
		if _, err := os.Stat(p); err != nil {
			t.Errorf("G4: global protocol %s missing at %s: %v", g.ID, g.MultiPath, err)
		}
	}
	for _, o := range c.OmitGlobal {
		if strings.TrimSpace(o.ID) == "" || strings.TrimSpace(o.Disposition) == "" || strings.TrimSpace(o.Reason) == "" {
			t.Fatalf("G4: omit_global requires id, disposition, reason: %+v", o)
		}
	}

	for _, pair := range c.PairedFiles {
		monoPath := filepath.Join(root, "skills", "mono", filepath.FromSlash(pair.Mono))
		multiPath := filepath.Join(multiRoot, filepath.FromSlash(pair.Multi))
		monoData, err := os.ReadFile(monoPath)
		if err != nil {
			t.Errorf("G4: paired mono %s: %v", pair.Mono, err)
			continue
		}
		multiData, err := os.ReadFile(multiPath)
		if err != nil {
			t.Errorf("G4: paired multi %s: %v", pair.Multi, err)
			continue
		}
		mode := pair.Mode
		if mode == "" {
			mode = "identical"
		}
		if err := comparePairedFileContent(monoData, multiData, mode, pair.LinkSubstitutions); err != nil {
			t.Errorf("G4: paired file mono %s vs multi %s: %v", pair.Mono, pair.Multi, err)
		}
	}

	for _, pair := range c.PairedTrees {
		if pair.Mono == "" || pair.Multi == "" {
			t.Fatalf("G4: paired tree requires mono and multi paths: %+v", pair)
		}
		mode := pair.Mode
		if mode == "" {
			mode = "identical"
		}
		if mode != "identical" {
			t.Errorf("G4: unknown paired tree mode %q", mode)
			continue
		}

		monoDir := filepath.Join(root, "skills", "mono", filepath.FromSlash(pair.Mono))
		multiDir := filepath.Join(multiRoot, filepath.FromSlash(pair.Multi))
		monoFiles, err := readRelativeFileTree(monoDir)
		if err != nil {
			t.Errorf("G4: paired mono tree %s: %v", pair.Mono, err)
			continue
		}
		multiFiles, err := readRelativeFileTree(multiDir)
		if err != nil {
			t.Errorf("G4: paired multi tree %s: %v", pair.Multi, err)
			continue
		}
		for rel, monoData := range monoFiles {
			multiData, ok := multiFiles[rel]
			if !ok {
				t.Errorf("G4: paired multi tree %s missing %s from mono %s", pair.Multi, rel, pair.Mono)
				continue
			}
			if !bytes.Equal(monoData, multiData) {
				t.Errorf("G4: paired tree file differs: mono %s/%s vs multi %s/%s", pair.Mono, rel, pair.Multi, rel)
			}
		}
		for rel := range multiFiles {
			if _, ok := monoFiles[rel]; !ok {
				t.Errorf("G4: paired mono tree %s missing %s from multi %s", pair.Mono, rel, pair.Multi)
			}
		}
	}

	allow := map[string]bool{}
	for _, row := range c.OrphanScriptsAllowlist {
		if row.Path == "" || row.Disposition == "" || row.Reason == "" {
			t.Fatalf("G4: orphan allowlist requires path, disposition, reason: %+v", row)
		}
		allow[filepath.ToSlash(row.Path)] = true
	}
	var orphans []string
	entries, err := os.ReadDir(multiRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skill := e.Name()
		scriptsDir := filepath.Join(multiRoot, skill, "scripts")
		if st, err := os.Stat(scriptsDir); err != nil || !st.IsDir() {
			continue
		}
		blob := skillMarkdownBlob(t, filepath.Join(multiRoot, skill))
		_ = filepath.WalkDir(scriptsDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return walkErr
			}
			rel, err := filepath.Rel(multiRoot, path)
			if err != nil {
				return err
			}
			relSlash := filepath.ToSlash(rel)
			base := filepath.Base(path)
			if (strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")) && strings.HasSuffix(base, ".py") {
				return nil
			}
			if strings.Contains(blob, base) || strings.Contains(blob, relSlash) {
				return nil
			}
			if allow[relSlash] {
				return nil
			}
			orphans = append(orphans, relSlash)
			return nil
		})
	}
	for _, o := range orphans {
		t.Errorf("G4: orphan script %s (reference from skill markdown or add orphan_scripts_allowlist)", o)
	}
	for path := range allow {
		full := filepath.Join(multiRoot, filepath.FromSlash(path))
		if _, err := os.Stat(full); err != nil {
			t.Errorf("G4: orphan allowlist path missing on disk: %s", path)
		}
	}

	retired := map[string]bool{}
	for _, row := range c.RetiredScripts {
		path := filepath.ToSlash(filepath.Clean(row.Path))
		if row.Path == "" || strings.TrimSpace(row.Reason) == "" {
			t.Fatalf("G4: retired_scripts requires path and reason: %+v", row)
		}
		if filepath.IsAbs(row.Path) || path == "." || path == ".." || strings.HasPrefix(path, "../") {
			t.Fatalf("G4: invalid retired script path %q", row.Path)
		}
		if retired[path] {
			t.Fatalf("G4: duplicate retired script path %q", path)
		}
		retired[path] = true
		full := filepath.Join(root, filepath.FromSlash(path))
		if _, err := os.Stat(full); err == nil {
			t.Errorf("G4: retired script was republished: %s", path)
		} else if !os.IsNotExist(err) {
			t.Errorf("G4: inspect retired script %s: %v", path, err)
		}
	}
}

func TestMonoMultiSkillContentG5RelativeLinks(t *testing.T) {
	root, c := loadMonoMultiQAContract(t)
	multiRoot := filepath.Join(root, "skills", "multi")
	sources := map[string]bool{}

	for _, pair := range c.PairedFiles {
		if pair.Mono == "" || pair.Multi == "" {
			t.Fatalf("G5: paired file requires mono and multi paths: %+v", pair)
		}
		sources[filepath.Join(root, "skills", "mono", filepath.FromSlash(pair.Mono))] = true
		sources[filepath.Join(multiRoot, filepath.FromSlash(pair.Multi))] = true
	}
	for _, pair := range c.PairedTrees {
		if pair.Mono == "" || pair.Multi == "" {
			t.Fatalf("G5: paired tree requires mono and multi paths: %+v", pair)
		}
		trees := []string{
			filepath.Join(root, "skills", "mono", filepath.FromSlash(pair.Mono)),
			filepath.Join(multiRoot, filepath.FromSlash(pair.Multi)),
		}
		for _, tree := range trees {
			files, err := readRelativeFileTree(tree)
			if err != nil {
				t.Errorf("G5: read paired Markdown tree %s: %v", tree, err)
				continue
			}
			for rel := range files {
				sources[filepath.Join(tree, filepath.FromSlash(rel))] = true
			}
		}
	}

	paths := make([]string, 0, len(sources))
	for path := range sources {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, source := range paths {
		if !strings.EqualFold(filepath.Ext(source), ".md") {
			continue
		}
		data, err := os.ReadFile(source)
		if err != nil {
			t.Errorf("G5: read %s: %v", source, err)
			continue
		}
		rel, relErr := filepath.Rel(root, source)
		if relErr != nil {
			rel = source
		}
		for _, issue := range unresolvedRelativeMarkdownLinks(root, source, data) {
			resolved := issue.Resolved
			if resolved != "" {
				if resolvedRel, err := filepath.Rel(root, resolved); err == nil {
					resolved = filepath.ToSlash(resolvedRel)
				}
			}
			t.Errorf("G5: %s:%d relative link %q resolves to %q: %v", filepath.ToSlash(rel), issue.Link.Line, issue.Link.Target, resolved, issue.Err)
		}
	}
}

func TestMonoMultiSkillContentLinkNormalization(t *testing.T) {
	rules := []monoMultiLinkSubstitution{
		{Mono: "../url-patterns.md", Multi: "../../dingtalk-shared/references/url-patterns.md"},
		{Mono: "../intent-guide.md", Multi: "sheet-intent-guide.md"},
	}
	mono := []byte("url=../url-patterns.md\nintent=../intent-guide.md\n")
	multi := []byte("url=../../dingtalk-shared/references/url-patterns.md\nintent=sheet-intent-guide.md\n")
	if err := comparePairedFileContent(mono, multi, "link-normalized", rules); err != nil {
		t.Fatalf("valid substitutions rejected: %v", err)
	}
	if err := comparePairedFileContent(mono, append(multi, []byte("drift\n")...), "link-normalized", rules); err == nil {
		t.Fatal("unknown content drift must not be normalized away")
	}
	if err := comparePairedFileContent(mono, multi, "identical", rules); err == nil {
		t.Fatal("identical mode must reject link substitutions")
	}
	if err := comparePairedFileContent(mono, multi, "unknown", nil); err == nil {
		t.Fatal("unknown paired-file mode must fail closed")
	}
}

func TestMonoMultiSkillContentLinkNormalizationRejectsInvalidRules(t *testing.T) {
	tests := []struct {
		name  string
		mono  string
		multi string
		rules []monoMultiLinkSubstitution
	}{
		{name: "no rules", mono: "m1", multi: "x1"},
		{name: "empty mono", mono: "m1", multi: "x1", rules: []monoMultiLinkSubstitution{{Multi: "x1"}}},
		{name: "empty multi", mono: "m1", multi: "x1", rules: []monoMultiLinkSubstitution{{Mono: "m1"}}},
		{name: "same sides", mono: "m1", multi: "m1", rules: []monoMultiLinkSubstitution{{Mono: "m1", Multi: "m1"}}},
		{name: "duplicate mono", mono: "m1", multi: "x1 x2", rules: []monoMultiLinkSubstitution{{Mono: "m1", Multi: "x1"}, {Mono: "m1", Multi: "x2"}}},
		{name: "duplicate multi", mono: "m1 m2", multi: "x1", rules: []monoMultiLinkSubstitution{{Mono: "m1", Multi: "x1"}, {Mono: "m2", Multi: "x1"}}},
		{name: "overlapping mono", mono: "m1 m", multi: "x1 x2", rules: []monoMultiLinkSubstitution{{Mono: "m", Multi: "x1"}, {Mono: "m1", Multi: "x2"}}},
		{name: "missing occurrence", mono: "m1", multi: "x1", rules: []monoMultiLinkSubstitution{{Mono: "m2", Multi: "x1"}}},
		{name: "multiple occurrences", mono: "m1 m1", multi: "x1", rules: []monoMultiLinkSubstitution{{Mono: "m1", Multi: "x1"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := normalizePairedFileLinks([]byte(tt.mono), []byte(tt.multi), tt.rules); err == nil {
				t.Fatal("invalid substitution rules must fail closed")
			}
		})
	}
}

func TestMonoMultiSkillContentRelativeLinkScanner(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	if err := os.MkdirAll(filepath.Join(docs, "sheet"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "topic.md"), []byte("# Topic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := []byte("[topic](./topic.md#section)\n[directory](./sheet/)\n[external](https://example.com/a)\n[email](mailto:test@example.com)\n[anchor](#section)\n`[inline](missing-inline.md)`\n```text\n[fenced](missing-fenced.md)\n```\n[missing](missing.md)\n[escape](../../escape.md)\n")
	issues := unresolvedRelativeMarkdownLinks(root, filepath.Join(docs, "source.md"), body)
	if len(issues) != 2 {
		t.Fatalf("got %d unresolved links, want 2: %+v", len(issues), issues)
	}
	if issues[0].Link.Target != "missing.md" || issues[0].Link.Line != 10 {
		t.Fatalf("unexpected unresolved link: %+v", issues[0])
	}
	if issues[1].Link.Target != "../../escape.md" || issues[1].Link.Line != 11 || !strings.Contains(issues[1].Err.Error(), "escapes repository") {
		t.Fatalf("unexpected escaping link: %+v", issues[1])
	}
}

func TestMonoMultiSkillContentReadRelativeFileTreeFiltersNonMarkdown(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"nested", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"guide.md":          "visible",
		"nested/topic.md":   "nested",
		"notes.txt":         "ignored",
		".DS_Store":         "ignored",
		".draft.md":         "ignored",
		".hidden/topic.md":  "ignored",
		"nested/README.MD":  "accepted",
		"nested/.hidden.md": "ignored",
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := readRelativeFileTree(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"guide.md", "nested/README.MD", "nested/topic.md"}
	gotNames := make([]string, 0, len(got))
	for rel := range got {
		gotNames = append(gotNames, rel)
	}
	sort.Strings(gotNames)
	if strings.Join(gotNames, "\n") != strings.Join(want, "\n") {
		t.Fatalf("visible Markdown tree = %v, want %v", gotNames, want)
	}
}

func comparePairedFileContent(monoData, multiData []byte, mode string, substitutions []monoMultiLinkSubstitution) error {
	switch mode {
	case "identical":
		if len(substitutions) != 0 {
			return fmt.Errorf("identical mode does not allow link_substitutions")
		}
		if !bytes.Equal(monoData, multiData) {
			return fmt.Errorf("contents differ")
		}
		return nil
	case "link-normalized":
		monoNormalized, multiNormalized, err := normalizePairedFileLinks(monoData, multiData, substitutions)
		if err != nil {
			return err
		}
		if !bytes.Equal(monoNormalized, multiNormalized) {
			return fmt.Errorf("contents differ after declared link substitutions")
		}
		return nil
	default:
		return fmt.Errorf("unknown paired mode %q", mode)
	}
}

func normalizePairedFileLinks(monoData, multiData []byte, substitutions []monoMultiLinkSubstitution) ([]byte, []byte, error) {
	if len(substitutions) == 0 {
		return nil, nil, fmt.Errorf("link-normalized mode requires link_substitutions")
	}
	for i, substitution := range substitutions {
		if substitution.Mono == "" || substitution.Multi == "" {
			return nil, nil, fmt.Errorf("link_substitutions[%d] requires non-empty mono and multi values", i)
		}
		if substitution.Mono == substitution.Multi {
			return nil, nil, fmt.Errorf("link_substitutions[%d] must describe a layout difference", i)
		}
		for j := 0; j < i; j++ {
			previous := substitutions[j]
			if strings.Contains(substitution.Mono, previous.Mono) || strings.Contains(previous.Mono, substitution.Mono) {
				return nil, nil, fmt.Errorf("link_substitutions[%d].mono overlaps link_substitutions[%d].mono", i, j)
			}
			if strings.Contains(substitution.Multi, previous.Multi) || strings.Contains(previous.Multi, substitution.Multi) {
				return nil, nil, fmt.Errorf("link_substitutions[%d].multi overlaps link_substitutions[%d].multi", i, j)
			}
		}
		if count := bytes.Count(monoData, []byte(substitution.Mono)); count != 1 {
			return nil, nil, fmt.Errorf("link_substitutions[%d].mono occurs %d times, want exactly 1", i, count)
		}
		if count := bytes.Count(multiData, []byte(substitution.Multi)); count != 1 {
			return nil, nil, fmt.Errorf("link_substitutions[%d].multi occurs %d times, want exactly 1", i, count)
		}
	}

	monoNormalized := bytes.Clone(monoData)
	multiNormalized := bytes.Clone(multiData)
	for i, substitution := range substitutions {
		marker := []byte(fmt.Sprintf("\x00DWS_LINK_SUBSTITUTION_%d\x00", i))
		if bytes.Contains(monoNormalized, marker) || bytes.Contains(multiNormalized, marker) {
			return nil, nil, fmt.Errorf("link_substitutions[%d] normalization marker already exists in content", i)
		}
		monoNormalized = bytes.Replace(monoNormalized, []byte(substitution.Mono), marker, 1)
		multiNormalized = bytes.Replace(multiNormalized, []byte(substitution.Multi), marker, 1)
	}
	return monoNormalized, multiNormalized, nil
}

func unresolvedRelativeMarkdownLinks(repoRoot, source string, body []byte) []markdownLinkIssue {
	var issues []markdownLinkIssue
	for _, link := range extractMarkdownLinks(body) {
		targetPath, check, err := relativeMarkdownTargetPath(link.Target)
		if err != nil {
			issues = append(issues, markdownLinkIssue{Link: link, Err: err})
			continue
		}
		if !check {
			continue
		}
		resolved := filepath.Clean(filepath.Join(filepath.Dir(source), filepath.FromSlash(targetPath)))
		rel, err := filepath.Rel(repoRoot, resolved)
		if err != nil {
			issues = append(issues, markdownLinkIssue{Link: link, Resolved: resolved, Err: err})
			continue
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			issues = append(issues, markdownLinkIssue{Link: link, Resolved: resolved, Err: fmt.Errorf("target escapes repository")})
			continue
		}
		if _, err := os.Stat(resolved); err != nil {
			issues = append(issues, markdownLinkIssue{Link: link, Resolved: resolved, Err: err})
			continue
		}
		realRoot, err := filepath.EvalSymlinks(repoRoot)
		if err != nil {
			issues = append(issues, markdownLinkIssue{Link: link, Resolved: resolved, Err: fmt.Errorf("resolve repository path: %w", err)})
			continue
		}
		realTarget, err := filepath.EvalSymlinks(resolved)
		if err != nil {
			issues = append(issues, markdownLinkIssue{Link: link, Resolved: resolved, Err: fmt.Errorf("resolve target path: %w", err)})
			continue
		}
		realRel, err := filepath.Rel(realRoot, realTarget)
		if err != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(os.PathSeparator)) {
			if err == nil {
				err = fmt.Errorf("target escapes repository through a symbolic link")
			}
			issues = append(issues, markdownLinkIssue{Link: link, Resolved: resolved, Err: err})
		}
	}
	return issues
}

func extractMarkdownLinks(body []byte) []markdownLink {
	var links []markdownLink
	var fence byte
	for i, line := range strings.Split(string(body), "\n") {
		if marker := markdownFenceMarker(line); marker != 0 {
			if fence == 0 {
				fence = marker
			} else if fence == marker {
				fence = 0
			}
			continue
		}
		if fence != 0 {
			continue
		}
		masked := maskInlineMarkdownCode(line)
		for _, match := range inlineMarkdownLinkPattern.FindAllStringSubmatch(masked, -1) {
			target := match[1]
			if strings.HasPrefix(target, "<") && strings.HasSuffix(target, ">") {
				target = strings.TrimSuffix(strings.TrimPrefix(target, "<"), ">")
			}
			links = append(links, markdownLink{Line: i + 1, Target: target})
		}
	}
	return links
}

func markdownFenceMarker(line string) byte {
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, "```") {
		return '`'
	}
	if strings.HasPrefix(trimmed, "~~~") {
		return '~'
	}
	return 0
}

func maskInlineMarkdownCode(line string) string {
	masked := []byte(line)
	for i := 0; i < len(masked); {
		if masked[i] != '`' {
			i++
			continue
		}
		endOfOpener := i
		for endOfOpener < len(masked) && masked[endOfOpener] == '`' {
			endOfOpener++
		}
		marker := masked[i:endOfOpener]
		closingOffset := bytes.Index(masked[endOfOpener:], marker)
		if closingOffset < 0 {
			i = endOfOpener
			continue
		}
		end := endOfOpener + closingOffset + len(marker)
		for j := i; j < end; j++ {
			masked[j] = ' '
		}
		i = end
	}
	return string(masked)
}

func relativeMarkdownTargetPath(target string) (string, bool, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return "", false, fmt.Errorf("parse target: %w", err)
	}
	if parsed.Scheme != "" || parsed.Host != "" || strings.HasPrefix(target, "//") || strings.HasPrefix(parsed.Path, "/") {
		return "", false, nil
	}
	if parsed.Path == "" {
		return "", false, nil
	}
	path, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", false, fmt.Errorf("decode target path: %w", err)
	}
	return path, true, nil
}

func readRelativeFileTree(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	return files, err
}

type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Metadata    struct {
		Category string `yaml:"category"`
		Requires struct {
			Bins []string `yaml:"bins"`
		} `yaml:"requires"`
	} `yaml:"metadata"`
}

func parseSkillFrontmatter(body []byte) (skillFrontmatter, error) {
	var out skillFrontmatter
	s := string(body)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return out, fmt.Errorf("missing YAML frontmatter opener")
	}
	rest := s[3:]
	if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	} else if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return out, fmt.Errorf("missing YAML frontmatter closer")
	}
	block := rest[:end]
	if err := yaml.Unmarshal([]byte(block), &out); err != nil {
		return out, err
	}
	return out, nil
}

func monoProductStems(productsDir string) (map[string]bool, error) {
	entries, err := os.ReadDir(productsDir)
	if err != nil {
		return nil, err
	}
	stems := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			stems[name] = true
			continue
		}
		if strings.HasSuffix(name, ".md") {
			stems[strings.TrimSuffix(name, ".md")] = true
		}
	}
	return stems, nil
}

func skillMarkdownBlob(t *testing.T, skillDir string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.WalkDir(skillDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
		return nil
	})
	return b.String()
}
