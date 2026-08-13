#!/usr/bin/env python3
"""
上传附件到钉钉 AI 表格 attachment 字段

完整流程（内部自动执行 3 步）:
  1. dws aitable attachment upload → 获取 uploadUrl + fileToken
  2. HTTP PUT 上传文件到 OSS
  3. 返回 fileToken，可直接用于 record create/update

用法:
    python upload_attachment.py <baseId> <filePath>

输出 (JSON):
    { "fileToken": "ft_xxx", "fileName": "report.pdf", "size": 204800 }

然后在 record create/update 中使用:
    dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
      --records '[{"cells":{"fldAttachId":[{"fileToken":"ft_xxx"}]}}]' --format json
"""

import argparse
import sys
import json
import subprocess
import os
import mimetypes
import re
import ssl
from pathlib import Path
from typing import Optional, Dict, Any
from urllib.request import Request, urlopen
from urllib.error import HTTPError, URLError
from urllib.parse import urlparse

RESOURCE_ID_PATTERN = re.compile(r'^[A-Za-z0-9_-]{6,128}$')
MAX_FILE_SIZE = 100 * 1024 * 1024  # 100MB


def validate_resource_id(resource_id: str) -> bool:
    return bool(resource_id and RESOURCE_ID_PATTERN.match(resource_id.strip()))


def detect_mime_type(file_path: Path) -> str:
    """根据文件扩展名推断 MIME type。"""
    mime_type, _ = mimetypes.guess_type(str(file_path))
    return mime_type or 'application/octet-stream'


def run_dws(args: list, dws_bin: str = 'dws') -> Optional[Dict[str, Any]]:
    """调用 dws 命令并返回解析后的 JSON 结果。"""
    cmd = [dws_bin] + args
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
        if result.returncode != 0:
            print(f"错误：dws 命令失败: {result.stderr.strip()}", file=sys.stderr)
            return None
        try:
            payload = json.loads(result.stdout)
            if not isinstance(payload, dict):
                print('错误：dws 响应 JSON 不是对象', file=sys.stderr)
                return None
            if payload.get('status') != 'success':
                detail = payload.get('summary') or payload.get('error') or payload
                print(f"错误：dws 业务失败: {detail}", file=sys.stderr)
                return None
            return payload
        except json.JSONDecodeError:
            print(f"错误：无法解析 dws 响应: {result.stdout[:300]}", file=sys.stderr)
            return None
    except subprocess.TimeoutExpired:
        print('错误：dws 命令超时（60 秒）', file=sys.stderr)
        return None
    except FileNotFoundError:
        print('错误：未找到 dws 命令，请确认已安装并在 PATH 中', file=sys.stderr)
        return None


def upload_to_oss(upload_url: str, file_path: Path, mime_type: str) -> bool:
    """通过 HTTP PUT 上传文件到 OSS。"""
    parsed = urlparse(upload_url)
    if parsed.scheme not in {'http', 'https'} or not parsed.hostname:
        print('错误：uploadUrl 必须是有效的 HTTP(S) URL', file=sys.stderr)
        return False
    if not parsed.hostname.endswith('.oss.aliyuncs.com'):
        print('错误：uploadUrl 不是阿里云 OSS 地址', file=sys.stderr)
        return False
    upload_url = parsed._replace(scheme='https').geturl()
    file_data = file_path.read_bytes()
    req = Request(upload_url, data=file_data, method='PUT')
    req.add_header('Content-Type', mime_type)

    try:
        try:
            import certifi
            context = ssl.create_default_context(cafile=certifi.where())
        except ImportError:
            context = ssl.create_default_context()
        with urlopen(req, timeout=120, context=context) as resp:
            if resp.status == 200:
                return True
            print(f"错误：OSS 上传失败，HTTP {resp.status}", file=sys.stderr)
            return False
    except HTTPError as e:
        print(f"错误：OSS 上传 HTTP 错误 {e.code}: {e.reason}", file=sys.stderr)
        return False
    except URLError as e:
        print(f"错误：OSS 上传网络错误: {e.reason}", file=sys.stderr)
        return False


def upload_attachment(
    base_id: str, file_path_str: str, dws_bin: str = 'dws'
) -> Optional[Dict[str, Any]]:
    """
    执行完整的附件上传流程:
      1. prepare_attachment_upload → uploadUrl + fileToken
      2. PUT 文件到 OSS
      3. 返回 fileToken 信息
    """
    # 验证文件
    file_path = Path(file_path_str).resolve()
    if not file_path.exists():
        print(f"错误：文件不存在: {file_path}", file=sys.stderr)
        return None
    if not file_path.is_file():
        print(f"错误：不是文件: {file_path}", file=sys.stderr)
        return None

    file_size = file_path.stat().st_size
    if file_size <= 0:
        print("错误：文件为空", file=sys.stderr)
        return None
    if file_size > MAX_FILE_SIZE:
        print(f"错误：文件过大 ({file_size:,} 字节，限制 {MAX_FILE_SIZE:,} 字节)", file=sys.stderr)
        return None

    file_name = file_path.name
    mime_type = detect_mime_type(file_path)

    # 步骤 1: prepare_attachment_upload
    print(f"步骤 1/3: 准备上传 {file_name} ({file_size:,} 字节, {mime_type})...", file=sys.stderr)
    dws_args = [
        'aitable', 'attachment', 'upload',
        '--base-id', base_id,
        '--file-name', file_name,
        '--size', str(file_size),
        '--mime-type', mime_type,
        '--format', 'json',
    ]
    result = run_dws(dws_args, dws_bin)
    if not result:
        return None

    status = result.get('status', '')
    if status != 'success':
        error = result.get('error', {})
        print(f"错误：准备上传失败: {error.get('message', json.dumps(error, ensure_ascii=False))}", file=sys.stderr)
        return None

    data = result.get('data', {})
    upload_url = data.get('uploadUrl', '')
    file_token = data.get('fileToken', '')

    if not upload_url or not file_token:
        print(f"错误：返回数据缺少 uploadUrl 或 fileToken: {json.dumps(data, ensure_ascii=False)}", file=sys.stderr)
        return None

    # 步骤 2: PUT 文件到 OSS
    print(f"步骤 2/3: 上传文件到 OSS...", file=sys.stderr)
    if not upload_to_oss(upload_url, file_path, mime_type):
        return None

    # 步骤 3: 返回 fileToken
    print(f"步骤 3/3: 上传完成！", file=sys.stderr)
    output = {
        "fileToken": file_token,
        "fileName": file_name,
        "size": file_size,
        "mimeType": mime_type,
    }

    return output


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('base_id')
    parser.add_argument('file_path')
    parser.add_argument('--dws', default='dws', help='dws 可执行文件路径')
    args = parser.parse_args()

    base_id = args.base_id
    file_path = args.file_path

    if not validate_resource_id(base_id):
        print('错误：无效的 baseId 格式', file=sys.stderr)
        parser.error('无效的 baseId 格式')

    result = upload_attachment(base_id, file_path, args.dws)
    if result is None:
        sys.exit(1)

    # 正常输出到 stdout（JSON 格式，方便解析）
    print(json.dumps(result, ensure_ascii=False, indent=2))
    sys.exit(0)


if __name__ == '__main__':
    main()
