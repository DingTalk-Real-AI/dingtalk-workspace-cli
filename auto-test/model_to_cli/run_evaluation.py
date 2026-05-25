#!/usr/bin/env python3
"""
CLI 命令选择评测引擎

只输出失败用例（命令错误 或 参数不正确）到 JSON，供人工/AI 分析。

用法:
  python run_evaluation.py
  python run_evaluation.py --models qwen3-max --cases 50
  python run_evaluation.py --dry-run
"""

# 已知布尔 flag 白名单（无值，不消耗下一个 token）
_BOOL_FLAGS = {"yes", "verbose", "dry-run", "force", "at-all", "available", "thumbnail"}

import argparse
import asyncio
import json
import os
import re as _re
import shlex
import time
import traceback
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional, Any, Tuple


# ============================================================
# 路径配置
# ============================================================
BASE_DIR       = Path(__file__).parent
CLI_DIR        = BASE_DIR.parent.parent  # test/model_to_cli/ → test/ → cli/
TESTCASES_DIR  = BASE_DIR / "testcases"
REFERENCES_DIR = CLI_DIR  / "skills" / "references" / "products"
REPORT_BASE_DIR = BASE_DIR / "evaluation_reports"
ENV_FILE       = BASE_DIR / "env.txt"
_API_KEY_DEBUG_PRINTED = False

WUKONG_PRODUCTS = [
    "aitable", "contact", "calendar", "todo", "doc", "chat", "conference", "minutes",
    "attendance", "mail", "oa", "report", "tb", "workbench", "bot", "ding", "notify",
    "aiapp", "aisearch", "contract", "devdoc", "drive", "live", "recruit",
]

OPEN_PRODUCTS = [
    "aitable", "contact", "calendar", "todo", "doc", "chat", "conference", "minutes",
    "attendance", "mail", "oa", "report", "workbench", "bot", "ding", "aiapp",
    "aisearch", "contract", "devdoc", "drive", "wiki", "live", "recruit",
]

PRODUCT_PROFILES = {
    "wukong": set(WUKONG_PRODUCTS),
    "open": set(OPEN_PRODUCTS),
}


def load_local_env() -> None:
    if not ENV_FILE.exists():
        return
    for line in ENV_FILE.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip().strip('"\'')
        if key and key not in os.environ:
            os.environ[key] = value

# ============================================================
# 模型配置（全部通过 DashScope 统一接入）
# ============================================================
MODELS_CONFIG = {
    "qwen3-max": {
        "model_id": "qwen3-max",
        "description": "通义千问3-max",
        "max_tokens": 512,
        "temperature": 0.1,
        "rpm": 10,
    },
    # "qwen3.5-plus": {
    #     "model_id": "qwen3.5-plus",
    #     "description": "通义千问3.5-plus",
    #     "max_tokens": 512,
    #     "temperature": 0.1,
    #     "rpm": 10,
    # },
}

# ============================================================
# 数据模型
# ============================================================

@dataclass
class TestCase:
    test_id: str
    user_intent: str
    product_id: str
    command_path: str
    expected_cliflat_array: List[str]
    expected_cli_command: str = ""
    expected_behavior: str = ""


@dataclass
class EvaluationResult:
    test_id: str
    user_intent: str
    expected_command: str
    command_selected: str
    command_correct: bool
    param_correct: bool
    raw_response: str
    param_diff: Optional[Dict] = field(default=None)
    error: Optional[str] = None

# ============================================================
# System Prompt 构建
# ============================================================

def build_system_prompt(product_id: str) -> str:
    lines = ["你是一个 AI 助手，能通过 dws CLI 操作钉钉产品。\n"]

    skill_md = REFERENCES_DIR / f"{product_id}.md"
    if not skill_md.exists():
        raise FileNotFoundError(f"产品文档不存在: {skill_md}")

    with open(skill_md, encoding="utf-8") as f:
        lines.append(f.read())

    lines.append("""
用户会给你一个需求。请输出对应的完整 dws CLI 命令。

规则:
- 只输出一条 dws 命令，不要输出任何解释
- 必须以 dws 开头，例如: dws calendar event create --title "会议" --start "..." --end "..."
- 缺失的 ID 类参数用占位符，例如: --id <EVENT_ID>
- 不要编造用户没提到的信息
- 末尾加 --format json
""")

    return "\n\n".join(lines)

# ============================================================
# 测试用例加载
# ============================================================

def load_test_cases(limit: Optional[int] = None, allowed_products: Optional[set] = None) -> List[TestCase]:
    test_cases = []
    if not TESTCASES_DIR.exists():
        print(f"  ⚠️ 测试用例目录不存在: {TESTCASES_DIR}")
        return test_cases

    for product_dir in sorted(TESTCASES_DIR.iterdir()):
        if not product_dir.is_dir():
            continue
        product_id = product_dir.name
        if allowed_products is not None and product_id not in allowed_products:
            continue
        for json_file in sorted(product_dir.glob("*_testcases.json")):
            try:
                with open(json_file, encoding="utf-8") as f:
                    data = json.load(f)
                for case in data.get("test_cases", []):
                    test_cases.append(TestCase(
                        test_id=case.get("test_id", ""),
                        user_intent=case.get("user_intent", ""),
                        product_id=case.get("product_id", product_id),
                        command_path=case.get("command_path", ""),
                        expected_cliflat_array=case.get("expected_cliflat_array", []),
                        expected_cli_command=case.get("expected_cli_command", ""),
                        expected_behavior=case.get("expected_behavior", ""),
                    ))
            except Exception as e:
                print(f"  ⚠️ 加载 {json_file.name} 失败: {e}")

    print(f"  加载完成: {len(test_cases)} 条用例（{TESTCASES_DIR}）")

    if limit and len(test_cases) > limit:
        import random
        random.seed(42)
        test_cases = random.sample(test_cases, limit)

    return test_cases

# ============================================================
# 模型调用
# ============================================================

async def call_model(model_name: str, system_prompt: str, user_intent: str) -> Tuple[str, int]:
    from openai import AsyncOpenAI
    global _API_KEY_DEBUG_PRINTED

    config = MODELS_CONFIG[model_name]
    start_time = time.time()

    load_local_env()
    api_key = os.environ.get("DASHSCOPE_API_KEY") or os.environ.get("CODE_DASHSCOPE_API_KEY", "")

    if not _API_KEY_DEBUG_PRINTED:
        masked = api_key[:8] + "..." if api_key else "NONE"
        print(f"  🔑 api_key={masked}")
        _API_KEY_DEBUG_PRINTED = True

    if not api_key:
        return "ERROR: DASHSCOPE_API_KEY not set", 0

    client = AsyncOpenAI(
        api_key=api_key,
        base_url="https://dashscope.aliyuncs.com/compatible-mode/v1",
    )

    try:
        resp = await client.chat.completions.create(
            model=config["model_id"],
            messages=[
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_intent},
            ],
            temperature=config["temperature"],
            max_tokens=config["max_tokens"],
        )
        text = resp.choices[0].message.content or ""
        return text, int((time.time() - start_time) * 1000)
    except Exception as e:
        return f"ERROR: {e}", int((time.time() - start_time) * 1000)

# ============================================================
# 响应解析
# ============================================================

def _verb_path(path: str) -> str:
    verbs = []
    for token in path.split():
        token_clean = token.strip('"\'<>[]')
        if _re.match(r'^[a-z][a-z0-9-]*$', token_clean):
            verbs.append(token_clean)
        else:
            break
    return " ".join(verbs)


def parse_cli_response(response: str) -> Tuple[Optional[str], Dict[str, Any]]:
    for line in response.strip().split("\n"):
        line = line.strip().strip("`")
        if line.startswith("dws "):
            cmd_path, flags = _extract_command_and_flags(line[4:])
            return _verb_path(cmd_path), flags
    return None, {}


def _extract_command_and_flags(cmd: str) -> Tuple[str, Dict[str, Any]]:
    try:
        parts = shlex.split(cmd)
    except ValueError:
        parts = cmd.split()

    cmd_parts = []
    flag_start = len(parts)
    for i, p in enumerate(parts):
        if p.startswith("-"):
            flag_start = i
            break
        if _re.match(r'^[a-z][a-z0-9-]*$', p):
            cmd_parts.append(p)
        else:
            flag_start = i
            break

    flags: Dict[str, Any] = {}
    i = flag_start
    while i < len(parts):
        p = parts[i]
        if p.startswith("--"):
            # 处理 --key=value 语法
            if "=" in p:
                key, val = p.lstrip("-").split("=", 1)
                flags[key] = val
                i += 1
            else:
                key = p.lstrip("-")
                if key in _BOOL_FLAGS or i + 1 >= len(parts) or parts[i + 1].startswith("--"):
                    flags[key] = True
                    i += 1
                else:
                    flags[key] = parts[i + 1]
                    i += 2
        else:
            i += 1

    return " ".join(cmd_parts), flags

# ============================================================
# 评分
# ============================================================

_SKIP_KEYS = {"format", "yes"}


def check_param_correct(generated: Dict, expected: Dict) -> Tuple[bool, Optional[Dict]]:
    """只对比参数的 key 是否对齐，不比较值。返回 (是否正确, diff详情)。"""
    gen_keys = {k for k in generated if k not in _SKIP_KEYS}
    exp_keys = {k for k in expected if k not in _SKIP_KEYS}

    extra = gen_keys - exp_keys
    missing = exp_keys - gen_keys

    if not extra and not missing:
        return True, None

    diff_detail: Dict[str, Any] = {}
    if extra:
        diff_detail["extra_params"] = sorted(extra)
    if missing:
        diff_detail["missing_params"] = sorted(missing)

    return False, diff_detail


def parse_expected_flags(cliflat_array: List[str]) -> Dict[str, Any]:
    _, flags = _extract_command_and_flags(shlex.join(cliflat_array))
    return flags

# ============================================================
# 评测执行
# ============================================================

async def evaluate_single_case(
    test_case: TestCase,
    model: str,
    system_prompt: str,
    semaphore: asyncio.Semaphore,
) -> EvaluationResult:
    async with semaphore:
        response, _ = await call_model(model, system_prompt, test_case.user_intent)

        if response.startswith("ERROR"):
            return EvaluationResult(
                test_id=test_case.test_id,
                user_intent=test_case.user_intent,
                expected_command=test_case.expected_cli_command,
                command_selected="",
                command_correct=False,
                param_correct=False,
                raw_response=response,
                error=response,
            )

        command_selected, params_generated = parse_cli_response(response)
        expected_verb = _verb_path(test_case.command_path.strip())
        command_correct = (command_selected or "").strip() == expected_verb

        expected_flags = parse_expected_flags(test_case.expected_cliflat_array)

        if test_case.expected_behavior == "ask_user":
            param_correct = command_correct
            param_diff = None
        else:
            param_correct, param_diff = check_param_correct(params_generated, expected_flags)

        return EvaluationResult(
            test_id=test_case.test_id,
            user_intent=test_case.user_intent,
            expected_command=test_case.expected_cli_command,
            command_selected=command_selected or "",
            command_correct=command_correct,
            param_correct=param_correct,
            raw_response=response,
            param_diff=param_diff,
        )


async def evaluate_model(model: str, test_cases: List[TestCase], rpm: int) -> List[EvaluationResult]:
    print(f"\n🔄 评测: {model} (rpm={rpm}, cases={len(test_cases)})")

    semaphore = asyncio.Semaphore(min(rpm, 20))
    cases_by_product: Dict[str, List[TestCase]] = {}
    for tc in test_cases:
        cases_by_product.setdefault(tc.product_id, []).append(tc)

    tasks = []
    for product_id, product_cases in sorted(cases_by_product.items()):
        system_prompt = build_system_prompt(product_id)
        tasks.extend(
            evaluate_single_case(tc, model, system_prompt, semaphore)
            for tc in product_cases
        )

    raw_results = await asyncio.gather(*tasks, return_exceptions=True)

    results = []
    for r in raw_results:
        if isinstance(r, Exception):
            print(f"  ⚠️ 异常: {r}")
        else:
            results.append(r)

    cmd_correct = sum(1 for r in results if r.command_correct)
    param_correct = sum(1 for r in results if r.param_correct)
    print(f"  ✅ {model}: 命令准确率={cmd_correct}/{len(results)} ({cmd_correct/len(results)*100:.1f}%), 参数准确率={param_correct}/{len(results)} ({param_correct/len(results)*100:.1f}%)")

    return results

# ============================================================
# 输出：只输出失败用例
# ============================================================

def generate_failures_json(
    model: str,
    results: List[EvaluationResult],
    timestamp: str,
    output_dir: Path,
) -> Path:
    output_dir.mkdir(parents=True, exist_ok=True)

    total = len(results)
    cmd_correct = sum(1 for r in results if r.command_correct)
    param_correct_count = sum(1 for r in results if r.param_correct)

    # 分两层：命令选错 vs 命令对但参数错
    command_wrong = [r for r in results if not r.command_correct]
    param_wrong = [r for r in results if r.command_correct and not r.param_correct]

    out_file = output_dir / f"{model}_{total}cases_{timestamp}_failures.json"

    data = {
        "model": model,
        "timestamp": timestamp,
        "total_cases": total,
        "command_accuracy": round(cmd_correct / total * 100, 1) if total else 0,
        "param_accuracy": round(param_correct_count / total * 100, 1) if total else 0,
        "command_wrong": {
            "count": len(command_wrong),
            "cases": [
                {
                    "test_id": r.test_id,
                    "user_intent": r.user_intent,
                    "expected_command": r.expected_command,
                    "raw_response": r.raw_response,
                }
                for r in command_wrong
            ],
        },
        "param_wrong": {
            "count": len(param_wrong),
            "cases": [
                {
                    "test_id": r.test_id,
                    "user_intent": r.user_intent,
                    "expected_command": r.expected_command,
                    "raw_response": r.raw_response,
                    "param_diff": r.param_diff,
                }
                for r in param_wrong
            ],
        },
    }

    with open(out_file, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)

    total_failures = len(command_wrong) + len(param_wrong)
    print(f"\n📄 失败用例 ({total_failures}/{total}): 命令错误={len(command_wrong)}, 参数错误={len(param_wrong)} → {out_file}")
    return out_file

# ============================================================
# 主函数
# ============================================================

async def main():
    parser = argparse.ArgumentParser(description="dws CLI 命令评测引擎")
    parser.add_argument("--models", default="all", help="模型列表，逗号分隔，默认 all")
    parser.add_argument("--cases", type=int, default=None, help="限制用例数量")
    parser.add_argument("--output", default=None, help="输出目录")
    parser.add_argument(
        "--edition",
        choices=["wukong", "open"],
        default="open",
        help="产品清单版本：open(开源版，默认) 或 wukong(内部版，仅供参考对比)",
    )
    parser.add_argument("--dry-run", action="store_true", help="只加载用例，不调用 API")
    args = parser.parse_args()

    models = list(MODELS_CONFIG.keys()) if args.models == "all" else [m.strip() for m in args.models.split(",")]
    default_report_subdir = "wukong_report" if args.edition == "wukong" else "open_report"
    output_dir = Path(args.output) if args.output else (REPORT_BASE_DIR / default_report_subdir)
    allowed_products = PRODUCT_PROFILES[args.edition]

    print("=" * 50)
    print("dws CLI 命令评测引擎")
    print(f"模型: {models}  版本: {args.edition}  输出: {output_dir}")
    print("=" * 50)

    print("\n📂 加载测试用例...")
    test_cases = load_test_cases(args.cases, allowed_products=allowed_products)
    if not test_cases:
        print("无用例，退出")
        return

    if args.dry_run:
        print(f"\n⚠️ dry-run: {len(test_cases)} 条用例已加载，退出")
        return

    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")

    for model in models:
        if model not in MODELS_CONFIG:
            print(f"  ⚠️ 未知模型: {model}，跳过")
            continue
        try:
            results = await evaluate_model(model, test_cases, MODELS_CONFIG[model]["rpm"])
            generate_failures_json(model, results, timestamp, output_dir)
        except Exception as e:
            print(f"  ❌ {model} 失败: {e}")
            traceback.print_exc()

    print("\n✅ 评测完成")


if __name__ == "__main__":
    asyncio.run(main())
