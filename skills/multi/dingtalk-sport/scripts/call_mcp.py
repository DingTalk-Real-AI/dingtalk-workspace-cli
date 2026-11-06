#!/usr/bin/env python3
"""
钉钉 MCP 工具调用脚本 — 通过 StreamableHTTP 协议直接调用 MCP Server

用法：
    # 列出 MCP Server 的所有工具
    python3 call_mcp.py list <mcp_server_url_or_env_var>

    # 调用指定工具
    python3 call_mcp.py call <mcp_server_url_or_env_var> <tool_name> [--params '{"key": "value"}']

URL 支持两种格式：
    1. 直接 URL：   "https://mcp-gw.dingtalk.com/server/xxx?key=yyy"
    2. 环境变量名： "$MCP_BOCHA_URL"（从系统环境或 .env 文件读取）

示例：
    python3 call_mcp.py list "$MCP_BOCHA_URL"
    python3 call_mcp.py call "$MCP_BOCHA_URL" web_search --params '{"query": "今天天气"}'
    python3 call_mcp.py call "https://mcp-gw.dingtalk.com/server/xxx?key=yyy" web_search --params '{"query": "今天天气"}'

MCP Server URL 获取方式：
    1. 访问 https://mcp.dingtalk.com/#/detail?mcpId={mcpId}&detailType=marketMcpDetail
    2. 登录钉钉账号
    3. 点击页面右侧获取 StreamableHTTP URL
    4. 写入技能目录下的 .env 文件，格式：MCP_BOCHA_URL=https://mcp-gw.dingtalk.com/server/xxx?key=yyy
"""

import argparse
import json
import os
import sys
import uuid
from pathlib import Path
from urllib.request import Request, urlopen
from urllib.error import HTTPError, URLError


def load_env_file():
    """从技能目录及父目录查找并加载 .env 文件。优先加载离调用脚本最近的 .env。"""
    search_dirs = [
        Path(__file__).parent.parent,  # 技能根目录（scripts 的上一级）
        Path(__file__).parent,         # scripts 目录
        Path.cwd(),                    # 当前工作目录
    ]
    for directory in search_dirs:
        env_file = directory / ".env"
        if env_file.exists():
            with open(env_file, encoding="utf-8") as f:
                for line in f:
                    line = line.strip()
                    if not line or line.startswith("#") or "=" not in line:
                        continue
                    key, _, value = line.partition("=")
                    key = key.strip()
                    value = value.strip().strip('"').strip("'")
                    if key and key not in os.environ:
                        os.environ[key] = value
            return str(env_file)
    return None


def resolve_url(url_or_var):
    """解析 URL：支持直接 URL 或 $ENV_VAR 格式的环境变量引用。"""
    if url_or_var.startswith("$"):
        var_name = url_or_var[1:]
        value = os.environ.get(var_name)
        if not value:
            print(f"❌ 环境变量未设置：{var_name}", file=sys.stderr)
            print(f"   请在技能目录的 .env 文件中添加：{var_name}=<your_url>", file=sys.stderr)
            print(f"   获取 URL：访问 https://mcp.dingtalk.com/ 登录后复制 StreamableHTTP URL", file=sys.stderr)
            sys.exit(1)
        return value
    return url_or_var


def make_jsonrpc_request(method, params=None):
    """构造 JSON-RPC 2.0 请求体。"""
    request_body = {
        "jsonrpc": "2.0",
        "id": str(uuid.uuid4()),
        "method": method,
    }
    if params is not None:
        request_body["params"] = params
    return request_body


def send_mcp_request(server_url, method, params=None):
    """向 MCP Server 发送 JSON-RPC 请求并返回结果。"""
    body = make_jsonrpc_request(method, params)
    data = json.dumps(body).encode("utf-8")

    request = Request(
        server_url,
        data=data,
        headers={
            "Content-Type": "application/json",
            "Accept": "application/json, text/event-stream",
        },
        method="POST",
    )

    try:
        with urlopen(request, timeout=30) as response:
            content_type = response.headers.get("Content-Type", "")
            raw = response.read().decode("utf-8")

            # StreamableHTTP 可能返回 SSE 格式或直接 JSON
            if "text/event-stream" in content_type:
                return parse_sse_response(raw)
            else:
                result = json.loads(raw)
                if "error" in result:
                    print(f"❌ MCP 错误：{result['error']}", file=sys.stderr)
                    return None
                return result.get("result")
    except HTTPError as error:
        error_body = error.read().decode("utf-8", errors="replace")
        print(f"❌ HTTP {error.code} 错误：{error_body}", file=sys.stderr)
        return None
    except URLError as error:
        print(f"❌ 网络错误：{error.reason}", file=sys.stderr)
        return None
    except json.JSONDecodeError as error:
        print(f"❌ JSON 解析失败：{error}", file=sys.stderr)
        return None


def parse_sse_response(raw_text):
    """解析 SSE（Server-Sent Events）格式的响应。"""
    for line in raw_text.strip().split("\n"):
        line = line.strip()
        if line.startswith("data:"):
            data_str = line[5:].strip()
            if not data_str:
                continue
            try:
                data = json.loads(data_str)
                if "result" in data:
                    return data["result"]
                if "error" in data:
                    print(f"❌ MCP 错误：{data['error']}", file=sys.stderr)
                    return None
            except json.JSONDecodeError:
                continue
    print("❌ 未能从 SSE 响应中解析到结果", file=sys.stderr)
    return None


def list_tools(server_url):
    """列出 MCP Server 的所有可用工具。"""
    print(f"🔍 正在获取工具列表...")
    print(f"   URL: {server_url[:80]}...")
    print()

    result = send_mcp_request(server_url, "tools/list")
    if result is None:
        return False

    tools = result.get("tools", [])
    if not tools:
        print("⚠️  该 MCP Server 没有可用的工具")
        return True

    print(f"📋 共 {len(tools)} 个工具：")
    print()
    for tool in tools:
        name = tool.get("name", "未知")
        description = tool.get("description", "无描述")
        print(f"  🔧 {name}")
        # 截断过长的描述
        if len(description) > 120:
            description = description[:117] + "..."
        print(f"     {description}")

        # 显示参数
        input_schema = tool.get("inputSchema", {})
        properties = input_schema.get("properties", {})
        required = input_schema.get("required", [])
        if properties:
            param_parts = []
            for param_name, param_info in properties.items():
                param_type = param_info.get("type", "any")
                is_required = "必填" if param_name in required else "可选"
                param_parts.append(f"{param_name}({param_type}, {is_required})")
            print(f"     参数: {', '.join(param_parts)}")
        print()

    return True


def call_tool(server_url, tool_name, params=None):
    """调用 MCP Server 的指定工具。"""
    print(f"🚀 正在调用工具: {tool_name}")
    if params:
        print(f"   参数: {json.dumps(params, ensure_ascii=False)}")
    print()

    call_params = {
        "name": tool_name,
        "arguments": params or {},
    }

    result = send_mcp_request(server_url, "tools/call", call_params)
    if result is None:
        return False

    # 解析工具返回的内容
    content_items = result.get("content", [])
    if not content_items:
        print("⚠️  工具返回了空结果")
        return True

    print("✅ 调用结果：")
    print()
    for item in content_items:
        item_type = item.get("type", "text")
        if item_type == "text":
            text = item.get("text", "")
            # 尝试格式化 JSON
            try:
                parsed = json.loads(text)
                print(json.dumps(parsed, ensure_ascii=False, indent=2))
            except (json.JSONDecodeError, TypeError):
                print(text)
        elif item_type == "image":
            print(f"[图片: {item.get('mimeType', 'unknown')}]")
        elif item_type == "resource":
            print(f"[资源: {item.get('uri', 'unknown')}]")
        else:
            print(f"[{item_type}]: {json.dumps(item, ensure_ascii=False)}")

    is_error = result.get("isError", False)
    if is_error:
        print("\n⚠️  工具执行报告了错误（见上方内容）")

    return not is_error


def main():
    # 启动时加载 .env 文件
    env_file = load_env_file()

    parser = argparse.ArgumentParser(
        description="钉钉 MCP 工具调用脚本",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例：
  python3 call_mcp.py list "$MCP_BOCHA_URL"
  python3 call_mcp.py call "$MCP_BOCHA_URL" web_search --params '{"query": "天气"}'
  python3 call_mcp.py call "https://mcp-gw.dingtalk.com/server/xxx?key=yyy" web_search --params '{"query": "天气"}'

URL 支持：
  直接 URL 或 $ENV_VAR 格式（从 .env 文件或系统环境变量读取）

MCP Server URL 获取方式：
  访问 https://mcp.dingtalk.com/#/detail?mcpId={mcpId}&detailType=marketMcpDetail
  登录后获取 StreamableHTTP URL，写入技能目录的 .env 文件
        """,
    )
    subparsers = parser.add_subparsers(dest="action", help="操作类型")

    # list 子命令
    list_parser = subparsers.add_parser("list", help="列出 MCP Server 的所有工具")
    list_parser.add_argument("url", help="MCP Server URL 或 $ENV_VAR 名")

    # call 子命令
    call_parser = subparsers.add_parser("call", help="调用 MCP Server 的指定工具")
    call_parser.add_argument("url", help="MCP Server URL 或 $ENV_VAR 名")
    call_parser.add_argument("tool", help="要调用的工具名称")
    call_parser.add_argument(
        "--params",
        default="{}",
        help='工具参数（JSON 格式），如 \'{"query": "天气"}\'',
    )

    args = parser.parse_args()

    if not args.action:
        parser.print_help()
        sys.exit(1)

    resolved_url = resolve_url(args.url)

    if args.action == "list":
        success = list_tools(resolved_url)
    elif args.action == "call":
        try:
            params = json.loads(args.params)
        except json.JSONDecodeError as error:
            print(f"❌ 参数 JSON 格式错误：{error}", file=sys.stderr)
            sys.exit(1)
        success = call_tool(resolved_url, args.tool, params)
    else:
        parser.print_help()
        sys.exit(1)

    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()
