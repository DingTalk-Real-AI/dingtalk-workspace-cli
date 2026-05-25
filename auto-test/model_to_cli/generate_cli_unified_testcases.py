#!/usr/bin/env python3
"""
CLI服务测试用例生成器 v3.0

从 skills/references/products/*.md 动态解析命令格式，生成测试用例。
避免硬编码参数名/参数值导致测试集与文档不一致的问题。

解析逻辑：
  - 命令路径：从 Usage: 行提取 verb tokens（直到遇到 <positional> 或 [flags]）
  - 位置参数：从 Usage: 行提取 <xxx> 形式的占位符
  - Flags：从 Flags: 段提取，(必填) 标注判断是否必填，无类型声明视为布尔 flag
  - 示例值：从 Example: 行提取每个 flag 对应的值

输出：testcases/<product>/<command_key>_testcases.json
"""

import json
import re
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional

# ── 路径配置 ──────────────────────────────────────────────────────────────────
SCRIPT_DIR = Path(__file__).parent
# auto-test/model_to_cli/ → auto-test/ → <repo-root>
REFERENCES_DIR = SCRIPT_DIR.parent.parent / "skills" / "references" / "products"
OUTPUT_DIR = SCRIPT_DIR / "testcases"

# secretId 占位符：这些 ID 必须在 user_intent 中以自然语言形式出现
SECRET_PLACEHOLDER_PATTERN = re.compile(
    r"<("
    r"ROBOT_CODE|CONV_ID|AGENT_ID|WEBHOOK_TOKEN|"
    r"DOC_ID|DOC_UUID|DENTRY_UUID|ROOT_UUID|"
    r"TASK_ID|USER_ID(?:_\d+)?"
    r")>"
)



# ── 解析工具函数 ───────────────────────────────────────────────────────────────

def _parse_flags_section(flags_text: str) -> List[Dict[str, Any]]:
    """
    解析 Flags: 段，提取每个 flag 的元数据。

    输入格式（每行）：
          --base string   AI 表格 Base ID (必填)
          --available     仅查空闲会议室        ← 布尔 flag，无类型
          --group string  群聊 openconversation_id（群聊时必填）  ← 条件必填

    返回：[{"name": "base", "type": "string", "required": True, "condition": None, "desc": "..."}]
    条件必填时 condition 为条件文本（如 "群聊"），用于互斥分组。
    """
    flags = []
    for line in flags_text.split("\n"):
        m = re.match(r"\s+(?:-\w,\s+)?--([a-z][a-z0-9-]*)(?:\s+(\S+))?\s+(.*)", line)
        if not m:
            continue
        name = m.group(1)
        flag_type = m.group(2) or "bool"   # 无类型声明 → 布尔开关
        desc = m.group(3).strip()

        # 检测条件必填："（群聊时必填）"、"（单聊时必填）" 等
        cond_match = re.search(r'[（(](\S+?)时必填[）)]', desc)
        if cond_match:
            required = True
            condition = cond_match.group(1)  # e.g. "群聊", "单聊"
        else:
            required = "必填" in desc
            condition = None

        flags.append({"name": name, "type": flag_type, "required": required,
                      "condition": condition, "desc": desc})
    return flags


def _parse_example_values(example_line: str) -> Dict[str, str]:
    """
    从 Example 行提取每个 flag 的示例值。

      --flag1 <VAL>           → {"flag1": "<VAL>"}
      --flag2 "quoted value"  → {"flag2": "quoted value"}
      --flag3 unquoted        → {"flag3": "unquoted"}
      --bool-flag             → {"bool-flag": ""}

    跳过 --format（全局 flag，不纳入测试评分）。
    跳过 --yes（危险确认 flag，全局通用）。
    """
    values: Dict[str, str] = {}
    # 切分 token，保留带引号的整体和 <占位符>
    tokens = re.findall(r'"[^"]*"|\'[^\']*\'|<[^>]+>|\S+', example_line)

    i = 0
    while i < len(tokens):
        tok = tokens[i]
        if not tok.startswith("--"):
            i += 1
            continue
        flag_name = tok[2:]
        if flag_name in ("format", "yes"):
            # format 后面有值要跳过，yes 是布尔 flag 没有值
            if flag_name == "format" and i + 1 < len(tokens) and not tokens[i + 1].startswith("--"):
                i += 2
            else:
                i += 1
            continue
        # 下一个 token 如果不以 -- 开头，视为值
        if i + 1 < len(tokens) and not tokens[i + 1].startswith("--"):
            raw_val = tokens[i + 1]
            values[flag_name] = raw_val.strip("\"'")
            i += 2
        else:
            values[flag_name] = ""   # 布尔 flag
            i += 1
    return values


def _parse_usage_positional_args(usage_line: str) -> List[str]:
    """
    从 Usage 行提取位置参数名（<xxx> 形式）。

    dws aitable sheet rename <tableId> <new-name> [flags]
    → ["tableId", "new-name"]
    """
    return re.findall(r"<([^>]+)>", usage_line)


def _extract_preceding_h3(content: str, block_start: int) -> str:
    """反向查找距代码块最近的 ### 标题文本。"""
    preceding = content[:block_start]
    titles = re.findall(r"###\s+(.+)", preceding)
    return titles[-1].strip() if titles else ""


def parse_product_md(product: str, md_path: Path) -> List[Dict]:
    """
    解析产品 .md 文件，返回结构化命令列表。

    每个命令字典包含：
      command_path    : 动词路径，如 "aitable field list"
      title           : 来自 ### 标题的描述
      positional_args : 位置参数名列表，如 ["tableId", "new-name"]
      flags           : flag 元数据列表
      example_values  : {flag_name: example_value}（来自 Example 行）
      example_line    : 完整示例命令（去掉 "dws " 前缀）
    """
    content = md_path.read_text(encoding="utf-8")
    commands = []

    for block_match in re.finditer(r"```\n(.*?)\n```", content, re.DOTALL):
        block = block_match.group(1)
        if not block.strip().startswith("Usage:"):
            continue

        # ── Usage 行 ──
        usage_m = re.search(r"Usage:\s*\n\s*dws\s+(.+)", block)
        if not usage_m:
            continue
        usage_line = usage_m.group(1).strip()

        # verb tokens：遇到 <xxx> 或 [xxx] 停止
        verb_tokens: List[str] = []
        for token in usage_line.split():
            if token.startswith("<") or token.startswith("["):
                break
            verb_tokens.append(token)
        if not verb_tokens:
            continue
        command_path = " ".join(verb_tokens)

        positional_args = _parse_usage_positional_args(usage_line)

        # ── Example 行 ──
        example_m = re.search(r"Example:\s*\n(.*?)(?=Flags:|$)", block, re.DOTALL)
        example_line = ""
        example_values: Dict[str, str] = {}
        if example_m:
            example_raw = example_m.group(1).strip().replace("\\\n", " ")
            for line in example_raw.split("\n"):
                line = line.strip()
                if line.startswith("dws "):
                    example_line = line[4:]   # 去掉 "dws "
                    example_values = _parse_example_values(line)
                    break

        # ── Flags 段 ──
        flags_m = re.search(r"Flags:\s*\n(.*?)$", block, re.DOTALL)
        flags: List[Dict] = []
        if flags_m:
            flags = _parse_flags_section(flags_m.group(1))

        # 段落标题
        title = _extract_preceding_h3(content, block_match.start())

        commands.append({
            "product": product,
            "command_path": command_path,
            "title": title or command_path,
            "positional_args": positional_args,
            "flags": flags,
            "example_values": example_values,
            "example_line": example_line,
        })

    return commands


# ── 测试用例构建 ───────────────────────────────────────────────────────────────

def _build_cliflat_array(
    cmd: Dict,
    include_required: bool = True,
    include_optional: bool = False,
    max_required: Optional[int] = None,
) -> List[str]:
    """
    构建 expected_cliflat_array。

    格式：[verb_token..., "--flag", "value", ...]
    位置参数使用文档中的占位符格式（<name>），对 param_score 无影响。

    参数：
      include_required : 是否包含必填 flag
      include_optional : 是否包含可选 flag
      max_required     : 最多包含几个必填 flag（None = 全部）

    互斥处理：
      条件必填 flag（如 "群聊时必填" / "单聊时必填"）属于不同 condition 分组，
      只选第一个遇到的 condition 分组，跳过其余互斥分组的 flag。
    """
    parts = list(cmd["command_path"].split())

    # 位置参数（保持命令完整性，不参与 param_score 评分）
    for arg in cmd["positional_args"]:
        val = cmd["example_values"].get(arg, f"<{arg.upper()}>")
        parts.append(val if val else f"<{arg.upper()}>")

    # 互斥分组：收集所有 condition 值，锁定第一个遇到的 condition
    chosen_condition: Optional[str] = None
    has_conditional = any(f.get("condition") for f in cmd["flags"])
    if has_conditional:
        # 取第一个条件必填 flag 的 condition 作为选中分组
        for flag in cmd["flags"]:
            if flag.get("condition"):
                chosen_condition = flag["condition"]
                break

    required_count = 0
    for flag in cmd["flags"]:
        is_required = flag["required"]
        condition = flag.get("condition")

        # 互斥跳过：如果是条件必填 flag 且不属于选中的 condition 分组，跳过
        if condition and condition != chosen_condition:
            continue

        if is_required and not include_required:
            continue
        if not is_required and not include_optional:
            continue
        if is_required and max_required is not None and required_count >= max_required:
            continue

        name = flag["name"]
        # 优先使用 Example 中的示例值，否则生成标准占位符
        value = cmd["example_values"].get(name)
        if value is None:
            value = f"<{name.upper().replace('-', '_')}>"

        if flag["type"] == "bool":
            parts.append(f"--{name}")
        else:
            parts.append(f"--{name}")
            parts.append(value)

        if is_required:
            required_count += 1

    return parts


def _build_user_intent(
    cmd: Dict,
    base_intent: str,
    cliflat_array: List[str],
    include_values: bool = True,
    include_all_placeholders: bool = False,
) -> str:
    """
    构建包含具体参数信息的 user_intent，使意图与期望输出一致。

    从 cliflat_array 中提取 flag 的值：
    - 具体值（非占位符）始终纳入。
    - 占位符：include_all_placeholders=True 时全部纳入；否则仅纳入 secretId 占位符。
    """
    if not include_values:
        return base_intent

    param_descs = []
    i = 0
    while i < len(cliflat_array):
        tok = cliflat_array[i]
        if tok.startswith("--"):
            flag_name = tok[2:]
            if i + 1 < len(cliflat_array) and not cliflat_array[i + 1].startswith("--"):
                value = cliflat_array[i + 1]
                is_placeholder = value.startswith("<") and value.endswith(">")
                if not is_placeholder:
                    param_descs.append(f"{flag_name} 为 {value}")
                elif include_all_placeholders or SECRET_PLACEHOLDER_PATTERN.fullmatch(value):
                    param_descs.append(f"{flag_name} 为 {value}")
                i += 2
            else:
                i += 1
        else:
            i += 1

    if param_descs:
        return f"{base_intent}，{', '.join(param_descs)}"
    return base_intent


def _cliflat_to_cli_command(cliflat_array: List[str]) -> str:
    """从 cliflat_array 构建 dws 命令行字符串，确保与 array 一致。"""
    parts = []
    for part in cliflat_array:
        # 包含空格或特殊字符的值加引号
        if " " in part or part.startswith("{") or part.startswith("["):
            parts.append(f'"{part}"')
        else:
            parts.append(part)
    cmd = "dws " + " ".join(parts)
    if "--format" not in cmd:
        cmd += " --format json"
    return cmd


def generate_cases_for_command(cmd: Dict) -> List[Dict]:
    """Generate basic test case: all required flags with example values."""
    product = cmd["product"]
    cmd_key = cmd["command_path"].replace(" ", "_").replace("-", "_")

    base = {
        "product": product,
        "product_id": product,
        "command_path": cmd["command_path"],
    }

    array_001 = _build_cliflat_array(cmd, include_required=True, include_optional=False)

    return [
        {
            **base,
            "test_id": f"{product}_{cmd_key}_001",
            "user_intent": _build_user_intent(
                cmd, f"{cmd['title']}", array_001, include_values=True
            ),
            "category": "basic",
            "difficulty": "easy",
            "tags": ["basic", product],
            "expected_cli_command": _cliflat_to_cli_command(array_001),
            "expected_cliflat_array": array_001,
        },
    ]


# ── 输出 ───────────────────────────────────────────────────────────────────────

def save_command_testcases(product: str, cmd: Dict, cases: List[Dict]) -> None:
    """保存单个命令的测试用例文件。"""
    cmd_key = cmd["command_path"].replace(" ", "_").replace("-", "_")
    product_dir = OUTPUT_DIR / product
    product_dir.mkdir(parents=True, exist_ok=True)

    payload = {
        "command_path": cmd["command_path"],
        "description": cmd["title"],
        "product": product,
        "product_id": product,
        "case_count": len(cases),
        "flags_required": [f["name"] for f in cmd["flags"] if f["required"]],
        "flags_optional": [f["name"] for f in cmd["flags"] if not f["required"]],
        "positional_args": cmd["positional_args"],
        "generated_at": datetime.now().isoformat(),
        "test_cases": cases,
    }
    filepath = product_dir / f"{cmd_key}_testcases.json"
    filepath.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")


def save_product_index(product: str, commands: List[Dict], all_cases: List[Dict]) -> None:
    """保存产品级索引文件。"""
    product_dir = OUTPUT_DIR / product
    product_dir.mkdir(parents=True, exist_ok=True)

    diff_dist: Dict[str, int] = {}
    cat_dist: Dict[str, int] = {}
    for case in all_cases:
        d = case.get("difficulty", "easy")
        c = case.get("category", "basic")
        diff_dist[d] = diff_dist.get(d, 0) + 1
        cat_dist[c] = cat_dist.get(c, 0) + 1

    index = {
        "product_id": product,
        "product_name": product,
        "priority": "P2",
        "total_commands": len(commands),
        "total_test_cases": len(all_cases),
        "generated_at": datetime.now().isoformat(),
        "reference_source": f"skills/references/products/{product}.md",
        "difficulty_distribution": diff_dist,
        "category_distribution": cat_dist,
        "commands": [
            {
                "command_path": cmd["command_path"],
                "title": cmd["title"],
                "flags_required": [f["name"] for f in cmd["flags"] if f["required"]],
                "flags_optional": [f["name"] for f in cmd["flags"] if not f["required"]],
                "positional_args": cmd["positional_args"],
                "file": f"{cmd['command_path'].replace(' ', '_').replace('-', '_')}_testcases.json",
            }
            for cmd in commands
        ],
    }
    (product_dir / "index.json").write_text(
        json.dumps(index, ensure_ascii=False, indent=2), encoding="utf-8"
    )


def save_global_index() -> None:
    """扫描 OUTPUT_DIR 下所有产品子目录，合并构建全局索引文件。"""
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    products = {}
    total_cases = 0
    for sub in sorted(OUTPUT_DIR.iterdir()):
        if not sub.is_dir():
            continue
        product_index_path = sub / "index.json"
        if not product_index_path.exists():
            continue
        try:
            pi = json.loads(product_index_path.read_text(encoding="utf-8"))
        except Exception:
            continue
        product_id = pi.get("product_id", sub.name)
        cmd_count = pi.get("total_commands", 0)
        case_count = pi.get("total_test_cases", 0)
        products[product_id] = {
            "name": product_id,
            "command_count": cmd_count,
            "case_count": case_count,
            "path": f"{product_id}/",
        }
        total_cases += case_count

    index = {
        "name": "CLI服务测试用例集 v3.0",
        "description": "参数名与示例值均从 skills 最新文档动态解析，无硬编码",
        "total_products": len(products),
        "total_test_cases": total_cases,
        "generated_at": datetime.now().isoformat(),
        "version": "3.0.0",
        "products": products,
    }
    (OUTPUT_DIR / "index.json").write_text(
        json.dumps(index, ensure_ascii=False, indent=2), encoding="utf-8"
    )


# ── 主流程 ─────────────────────────────────────────────────────────────────────

def generate_all() -> None:
    print("=" * 70)
    print("CLI服务测试用例生成器 v3.0")
    print(f"参考目录: {REFERENCES_DIR}")
    print(f"输出目录: {OUTPUT_DIR}")
    print("=" * 70)

    if not REFERENCES_DIR.exists():
        print(f"\n❌ 参考目录不存在: {REFERENCES_DIR}")
        return

    total_cases = 0
    stats: List[Dict] = []

    # 自动扫描 references/products/*.md，不再依赖 PRODUCTS_CONFIG 枚举
    for md_path in sorted(REFERENCES_DIR.glob("*.md")):
        product = md_path.stem  # e.g. "aitable", "aiapp", "minutes"
        print(f"\n📦 {product}")

        # 已生成过则跳过
        if (OUTPUT_DIR / product).exists():
            print(f"   ⏭️  已存在，跳过")
            continue

        # 检测"暂未上线"标记，跳过不生成测试集
        md_content_raw = md_path.read_text(encoding="utf-8")
        if "暂未上线" in md_content_raw:
            print(f"   ⏭️  暂未上线，跳过")
            continue

        commands = parse_product_md(product, md_path)
        if not commands:
            print(f"   ⚠️  未解析到命令，跳过")
            continue

        print(f"   {len(commands)} 个命令")
        all_cases: List[Dict] = []

        for cmd in commands:
            cases = generate_cases_for_command(cmd)
            save_command_testcases(product, cmd, cases)
            all_cases.extend(cases)

            req = [f["name"] for f in cmd["flags"] if f["required"]]
            opt = [f["name"] for f in cmd["flags"] if not f["required"]]
            pos = cmd["positional_args"]
            req_str = f"必填:[{','.join(req)}]" if req else "无必填"
            opt_str = f" 可选:[{','.join(opt)}]" if opt else ""
            pos_str = f" 位置:{pos}" if pos else ""
            print(f"   ✓  {cmd['command_path']:42s} {req_str}{opt_str}{pos_str}")

        save_product_index(product, commands, all_cases)
        total_cases += len(all_cases)
        stats.append({
            "product": product,
            "name": product,
            "commands": len(commands),
            "cases": len(all_cases),
        })

    save_global_index()

    print("\n" + "=" * 70)
    print("生成完成")
    print("-" * 70)
    for s in stats:
        print(f"  {s['name']:12s} ({s['product']:12s}): "
              f"{s['commands']:2d} 命令  {s['cases']:4d} 用例")
    print("-" * 70)
    print(f"  合计: {len(stats)} 个产品，{total_cases} 条用例")
    print("=" * 70)


if __name__ == "__main__":
    generate_all()