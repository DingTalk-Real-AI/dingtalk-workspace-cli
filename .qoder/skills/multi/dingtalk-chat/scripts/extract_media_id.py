#!/usr/bin/env python3
"""
从 dt_media_upload 返回的 URL 中提取 mediaId。

用法:
    python extract_media_id.py <url>
    python extract_media_id.py "https://down.dingtalk.com/media/lQLPD4JNnliqBq3NBQDNA8Cw_960_1280.png"
    # 输出: @lQLPD4JNnliqBq3NBQDNA8Cw

依赖: 无（纯 Python 标准库）

典型工作流（发送图片/语音/视频消息）:
    # 1. 用 dt_media_upload 上传文件，获得 URL
    # 2. 用本脚本从 URL 提取 mediaId
    python extract_media_id.py "<dt_media_upload 返回的 URL>"
    # 输出: @lQLPxxx（直接用于 --media-id 或 --pic-url 参数）
"""

import argparse
import re
import sys
from urllib.parse import urlparse


def extract_media_id(url: str) -> str:
    """从 dt_media_upload 返回的 URL 中提取 mediaId。

    支持的 URL 格式示例:
      https://down.dingtalk.com/media/lQLPD4JNnliqBq3NBQDNA8Cw_960_1280.png
      https://down.dingtalk.com/media/lQLPD4JNnliqBq3NBQDNA8Cw.png
      https://down.dingtalk.com/media/lQLPD4JNnliqBq3NBQDNA8Cw

    提取规则:
      1. 取 URL 路径中 /media/ 之后的部分
      2. 去除尾部的 _数字_数字.扩展名 后缀（如 _960_1280.png）
      3. 如果没有尺寸后缀，去除尾部的 .扩展名
      4. 加上 @ 前缀

    返回: @lQLPD4JNnliqBq3NBQDNA8Cw
    """
    url = url.strip()

    # 提取路径部分
    parsed = urlparse(url)
    path = parsed.path  # e.g. /media/lQLPxxx_960_1280.png

    # 取 /media/ 之后的部分
    media_prefix = "/media/"
    idx = path.find(media_prefix)
    if idx == -1:
        print(f"错误：URL 中未找到 /media/ 路径: {url}", file=sys.stderr)
        sys.exit(1)
    raw = path[idx + len(media_prefix):]  # e.g. lQLPxxx_960_1280.png

    if not raw:
        print(f"错误：/media/ 后无内容: {url}", file=sys.stderr)
        sys.exit(1)

    # 尝试去除 _数字_数字.扩展名 后缀（如 _960_1280.png）
    match = re.match(r'^(.+?)(_\d+_\d+\.\w+)$', raw)
    if match:
        media_id = match.group(1)
    else:
        # 没有尺寸后缀，尝试去除 .扩展名
        match2 = re.match(r'^(.+)\.\w+$', raw)
        if match2:
            media_id = match2.group(1)
        else:
            # 无后缀，直接使用
            media_id = raw

    return f"@{media_id}"


def main():
    parser = argparse.ArgumentParser(
        description="从 dt_media_upload 返回的 URL 中提取 mediaId"
    )
    parser.add_argument("url", help="dt_media_upload 返回的文件 URL")
    args = parser.parse_args()

    media_id = extract_media_id(args.url)
    print(media_id)


if __name__ == "__main__":
    main()
