#!/usr/bin/env bash
# =============================================================================
# run-mcp-probe.sh — dws CLI parity 验收工具启动脚本
# =============================================================================
#
# 用途：
#   基于 `dws schema` 的自发现能力，验证所有 dws CLI 命令面
#   都能正确产生 MCP tools/call 请求，并且参数映射与 schema 一致。
#
# 工作原理：
#   0. 自动执行 "dws cache refresh" 刷新所有产品的 MCP server 列表
#   1. 调用 `dws schema --json` 获取当前 canonical catalog（产品列表 + MCP endpoints）
#   2. 调用 `dws schema <product> --json` 和 `dws schema <product>.<tool> --json`
#      自发现每个产品/工具的 CLI 路径、flags 和 input_schema
#   3. 启动一个透明 HTTP 代理，拦截所有 MCP tools/call 请求并记录参数
#   4. 将 catalog 中所有产品的 endpoint 替换为代理地址，写入临时 fixture 文件
#   5. 通过 DWS_CATALOG_FIXTURE 环境变量注入 fixture，让 dws 把流量打到代理
#   6. 对每个工具自动生成探测参数，分别通过 flags / aliases /
#      flat --json / protocol-nested --json 执行，
#      校验 CLI 命令面与 MCP 请求参数完全一致
#
# 前置条件：
#   - dws 已安装且在 PATH 中（或通过 --dws 指定路径）
#   - dws 已完成 auth 登录（dws auth login）
#   - Go 1.21+ 已安装
#
# 使用方式：
#   ./scripts/dev/run-mcp-probe.sh [选项]
#
# 选项：
#   --dws PATH        dws 二进制路径（默认：自动查找 PATH 中的 dws）
#   --truth-dws PATH  真相源 dws 二进制路径；启用 help surface parity 对比
#   --servers-json PATH 使用本地 servers.json 作为临时 discovery 服务列表源
#   --filter STRING   只运行 schema path 或 CLI path 包含此字符串的工具（默认：运行全部）
#   --verbose         显示详细输出，包括 dws 的 stdout/stderr
#   --help            显示此帮助信息
#
# 示例：
#   # 运行所有测试用例
#   ./scripts/dev/run-mcp-probe.sh
#
#   # 只运行 aitable 相关的测试用例
#   ./scripts/dev/run-mcp-probe.sh --filter aitable
#
#   # 指定 dws 路径，显示详细输出
#   ./scripts/dev/run-mcp-probe.sh --dws ./bin/dws --verbose
#
#   # 只跑 todo 相关工具
#   ./scripts/dev/run-mcp-probe.sh --filter todo
#
# =============================================================================

set -euo pipefail

# --------------------------------------------------------------------------
# 默认参数
# --------------------------------------------------------------------------
DWS_BINARY=""          # 空表示自动查找
TRUTH_DWS_BINARY=""    # 空表示不启用真相源对比
SERVERS_JSON_PATH=""   # 空表示使用线上 discovery
FILTER=""              # 空表示不过滤
VERBOSE=false
DISCOVERY_BASE_URL=""
DISCOVERY_SERVER_PID=""
DISCOVERY_READY_FILE=""

cleanup() {
  if [[ -n "$DISCOVERY_SERVER_PID" ]]; then
    kill "$DISCOVERY_SERVER_PID" &>/dev/null || true
    wait "$DISCOVERY_SERVER_PID" 2>/dev/null || true
  fi
  if [[ -n "$DISCOVERY_READY_FILE" ]]; then
    rm -f "$DISCOVERY_READY_FILE"
  fi
}

start_local_discovery_server() {
  local servers_json="$1"

  if ! command -v python3 &>/dev/null; then
    echo "错误：使用 --servers-json 需要 python3。" >&2
    exit 1
  fi

  DISCOVERY_READY_FILE="$(mktemp)"
  python3 - "$servers_json" "$DISCOVERY_READY_FILE" <<'PY' &
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit
import sys

payload = Path(sys.argv[1]).read_bytes()
ready_path = Path(sys.argv[2])

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if urlsplit(self.path).path != "/cli/discovery/apis":
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, _format, *args):
        return

server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
ready_path.write_text(f"http://127.0.0.1:{server.server_address[1]}", encoding="utf-8")
server.serve_forever()
PY
  DISCOVERY_SERVER_PID=$!

  for _ in {1..100}; do
    if [[ -s "$DISCOVERY_READY_FILE" ]]; then
      DISCOVERY_BASE_URL="$(<"$DISCOVERY_READY_FILE")"
      return 0
    fi
    if ! kill -0 "$DISCOVERY_SERVER_PID" 2>/dev/null; then
      break
    fi
    sleep 0.1
  done

  echo "错误：启动本地 discovery 服务失败。" >&2
  exit 1
}

trap cleanup EXIT

# --------------------------------------------------------------------------
# 解析命令行参数
# --------------------------------------------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dws)
      DWS_BINARY="$2"
      shift 2
      ;;
    --truth-dws)
      TRUTH_DWS_BINARY="$2"
      shift 2
      ;;
    --servers-json)
      SERVERS_JSON_PATH="$2"
      shift 2
      ;;
    --filter)
      FILTER="$2"
      shift 2
      ;;
    --verbose)
      VERBOSE=true
      shift
      ;;
    --help|-h)
      cat <<'EOF'
dws cli parity checker — 验证 dws CLI 调用产生正确的 MCP tools/call 请求

用途：
  验证 dws CLI 调用能正确产生 MCP tools/call 请求，
  作为 canonical 命令面参数等价性的验收标准。

使用方式：
  ./scripts/dev/run-mcp-probe.sh [选项]

选项：
  --dws PATH        dws 二进制路径（默认：自动查找 PATH 中的 dws）
  --truth-dws PATH  真相源 dws 二进制路径；启用 help surface parity 对比
  --servers-json PATH 使用本地 servers.json 作为临时 discovery 服务列表源
  --filter STRING   只运行 schema path 或 CLI path 包含此字符串的工具（默认：运行全部）
  --verbose         显示详细输出，包括 dws 的 stdout/stderr
  --help            显示此帮助信息

示例：
  # 运行所有测试用例
  ./scripts/dev/run-mcp-probe.sh

  # 只运行 aitable 相关的测试用例
  ./scripts/dev/run-mcp-probe.sh --filter aitable

  # 指定 dws 路径，显示详细输出
  ./scripts/dev/run-mcp-probe.sh --dws ./bin/dws --verbose

前置条件：
  - dws 已安装且在 PATH 中（或通过 --dws 指定路径）
  - dws 已完成 auth 登录（dws auth login）
  - dws 本地有 catalog 缓存（运行过 dws <product> --help 等命令）
  - Go 1.21+ 已安装
EOF
      exit 0
      ;;
    *)
      echo "未知参数: $1" >&2
      echo "使用 --help 查看帮助" >&2
      exit 1
      ;;
  esac
done

# --------------------------------------------------------------------------
# 确定脚本和项目根目录
# --------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# --------------------------------------------------------------------------
# 查找 dws 二进制
# --------------------------------------------------------------------------
if [[ -z "$DWS_BINARY" ]]; then
  if command -v dws &>/dev/null; then
    DWS_BINARY="$(command -v dws)"
  else
    echo "错误：未找到 dws 命令。请安装 dws 或通过 --dws 指定路径。" >&2
    exit 1
  fi
fi

if [[ ! -x "$DWS_BINARY" ]]; then
  echo "错误：dws 路径不可执行: $DWS_BINARY" >&2
  exit 1
fi

if [[ -n "$TRUTH_DWS_BINARY" && ! -x "$TRUTH_DWS_BINARY" ]]; then
  echo "错误：truth dws 路径不可执行: $TRUTH_DWS_BINARY" >&2
  exit 1
fi

if [[ -n "$SERVERS_JSON_PATH" && ! -f "$SERVERS_JSON_PATH" ]]; then
  echo "错误：servers.json 路径不存在: $SERVERS_JSON_PATH" >&2
  exit 1
fi

# --------------------------------------------------------------------------
# 确定测试用例文件路径
# --------------------------------------------------------------------------
# --------------------------------------------------------------------------
# 检查 Go 环境
# --------------------------------------------------------------------------
if ! command -v go &>/dev/null; then
  echo "错误：未找到 go 命令。请安装 Go 1.21+。" >&2
  exit 1
fi

# --------------------------------------------------------------------------
# 构建 go run 参数
# --------------------------------------------------------------------------
ARGS=(
  "--dws" "$DWS_BINARY"
)

if [[ -n "$FILTER" ]]; then
  ARGS+=("--filter" "$FILTER")
fi

if [[ -n "$TRUTH_DWS_BINARY" ]]; then
  ARGS+=("--truth-dws" "$TRUTH_DWS_BINARY")
fi

if [[ "$VERBOSE" == "true" ]]; then
  ARGS+=("--verbose")
fi

if [[ -n "$SERVERS_JSON_PATH" ]]; then
  start_local_discovery_server "$SERVERS_JSON_PATH"
  export DWS_DISCOVERY_BASE_URL="$DISCOVERY_BASE_URL"
fi

# --------------------------------------------------------------------------
# Step 0: 刷新 dws catalog 缓存
#
# "dws schema --json" 需要 MCP server 列表已缓存才能返回 products。
# "dws cache refresh" 会连接 market 服务拉取所有可用 server 的 endpoint，
# 写入 ~/.dws/cache/*/market/servers.json，之后 dws schema 才能正常工作。
# --------------------------------------------------------------------------
echo "=== dws cli parity checker ==="
echo "dws 路径:   $DWS_BINARY"
[[ -n "$TRUTH_DWS_BINARY" ]] && echo "真相源:     $TRUTH_DWS_BINARY"
[[ -n "$SERVERS_JSON_PATH" ]] && echo "服务列表:   $SERVERS_JSON_PATH"
[[ -n "$DISCOVERY_BASE_URL" ]] && echo "discovery:  $DISCOVERY_BASE_URL"
[[ -n "$FILTER" ]] && echo "过滤条件:   $FILTER"
[[ "$VERBOSE" == "true" ]] && echo "详细模式:   开启"
echo ""

echo "[1/2] 刷新 dws catalog 缓存..."
if ! "$DWS_BINARY" cache refresh 2>&1; then
  echo "警告：dws cache refresh 失败，将使用现有缓存继续（如果有的话）" >&2
fi
echo ""

# --------------------------------------------------------------------------
# Step 1: 运行验收工具
# 切换到项目根目录运行，确保相对路径正确
# --------------------------------------------------------------------------
echo "[2/2] 运行 cli parity checker..."
cd "$REPO_ROOT"
go run ./test/mcp-probe/... "${ARGS[@]}"
