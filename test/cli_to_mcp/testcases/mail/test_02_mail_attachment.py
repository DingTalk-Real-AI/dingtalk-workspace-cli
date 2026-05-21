"""
test_mail_attachment.py — 邮件附件发送测试

环境变量: DINGTALK_MAIL_EMAIL

Commands tested:
  1. dws mail message send --attachment  (带附件发送，编排: create_draft → create_upload_session → PUT → send_draft)

Note:
  端到端附件上传测试需要 MCP Server 实现 create_upload_session 和 send_draft tool。
  在后端就绪前，部分用例标记 @pytest.mark.skip。
"""

import json
import os
import tempfile
import time

import pytest


@pytest.fixture(scope="session")
def email_addr():
    return os.environ.get("DINGTALK_MAIL_EMAIL", "1i2-hgmfj0xkbl@dingtalk.com")


@pytest.fixture(scope="session")
def tmp_attachment():
    """创建一个临时附件文件用于测试。"""
    with tempfile.NamedTemporaryFile(
        mode="w", suffix=".txt", prefix="cli_test_attach_", delete=False
    ) as f:
        f.write("这是 CLI 自动化测试附件内容。\n" * 10)
        path = f.name
    yield path
    try:
        os.unlink(path)
    except OSError:
        pass


class TestMailAttachmentParamValidation:
    """附件参数校验测试（不依赖后端 MCP tool，可立即运行）。"""

    def test_attachment_flag_exists(self, dws):
        """--attachment flag 应被 CLI 识别（不报 unknown flag 错误）。"""
        result = dws.run_raw(
            "mail", "message", "send", "--help",
        )
        help_text = result.stdout + result.stderr
        assert "--attachment" in help_text, (
            f"--attachment flag 未出现在 help 输出中:\n{help_text[:500]}"
        )

    def test_send_attachment_missing_file(self, dws, email_addr):
        """指定不存在的附件文件应报错。"""
        result = dws.run_raw(
            "mail", "message", "send",
            "--from", email_addr,
            "--to", email_addr,
            "--subject", "附件测试",
            "--body", "测试",
            "--attachment", "/tmp/nonexistent_file_12345.pdf",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "cannot" in combined, (
            f"指定不存在的附件应报错, returncode={result.returncode}, "
            f"stdout={result.stdout[:300]}, stderr={result.stderr[:300]}"
        )

    def test_send_attachment_directory(self, dws, email_addr):
        """指定目录作为附件应报错。"""
        result = dws.run_raw(
            "mail", "message", "send",
            "--from", email_addr,
            "--to", email_addr,
            "--subject", "附件测试",
            "--body", "测试",
            "--attachment", "/tmp",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "directory" in combined, (
            f"指定目录作为附件应报错, returncode={result.returncode}, "
            f"stdout={result.stdout[:300]}, stderr={result.stderr[:300]}"
        )

    def test_send_without_attachment_still_works(self, dws, email_addr):
        """不带附件的发送应保持原有行为正常工作。"""
        data = dws.run(
            "mail", "message", "send",
            "--from", email_addr,
            "--to", email_addr,
            "--subject", f"无附件测试_{int(time.time())}",
            "--body", "无附件，验证原有逻辑不受影响。",
        )
        assert isinstance(data, dict) and len(data) > 0, (
            f"无附件发送结果应为非空字典: {data}"
        )


class TestMailAttachmentSend:
    """端到端附件发送测试。

    需要 MCP Server 实现 create_upload_session 和 send_draft tool。
    后端就绪前用 pytest.mark.skip 标记。
    """

    def test_send_single_attachment(self, dws, email_addr, tmp_attachment):
        """发送单个附件。"""
        data = dws.run(
            "mail", "message", "send",
            "--from", email_addr,
            "--to", email_addr,
            "--subject", f"单附件测试_{int(time.time())}",
            "--body", "请查收附件。",
            "--attachment", tmp_attachment,
        )
        assert isinstance(data, dict) and len(data) > 0, (
            f"附件发送结果应为非空字典: {data}"
        )

    def test_send_multiple_attachments(self, dws, email_addr, tmp_attachment):
        """发送多个附件。"""
        # 创建第二个临时文件
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".csv", prefix="cli_test_attach2_", delete=False
        ) as f2:
            f2.write("col1,col2\nval1,val2\n")
            path2 = f2.name

        try:
            data = dws.run(
                "mail", "message", "send",
                "--from", email_addr,
                "--to", email_addr,
                "--subject", f"多附件测试_{int(time.time())}",
                "--body", "请查收多个附件。",
                "--attachment", tmp_attachment,
                "--attachment", path2,
            )
            assert isinstance(data, dict) and len(data) > 0, (
                f"多附件发送结果应为非空字典: {data}"
            )
        finally:
            try:
                os.unlink(path2)
            except OSError:
                pass

    def test_send_attachment_with_cc(self, dws, email_addr, tmp_attachment):
        """发送带抄送和附件的邮件。"""
        data = dws.run(
            "mail", "message", "send",
            "--from", email_addr,
            "--to", email_addr,
            "--cc", email_addr,
            "--subject", f"抄送+附件测试_{int(time.time())}",
            "--body", "带抄送和附件。",
            "--attachment", tmp_attachment,
        )
        assert isinstance(data, dict) and len(data) > 0, (
            f"附件+抄送发送结果应为非空字典: {data}"
        )

    def test_send_attachment_invalid_from(self, dws, email_addr, tmp_attachment):
        """无效发件人 + 附件应报错。"""
        result = dws.run_raw(
            "mail", "message", "send",
            "--from", "invalid@nowhere.test",
            "--to", email_addr,
            "--subject", "X",
            "--body", "X",
            "--attachment", tmp_attachment,
        )
        combined = (result.stdout + result.stderr).lower()
        assert (
            result.returncode != 0
            or "error" in combined
        ), (
            f"无效发件人+附件应报错, returncode={result.returncode}"
        )


class TestMailSendInlineAttachment:
    """内联附件（--inline-attachment）发送测试。

    内联附件用于在邮件正文中嵌入图片，CLI 自动生成 contentId（格式：inline-{文件名}-{序号}@alimail.com）。
    端到端测试需要后端 MCP Server 就绪，标记 @pytest.mark.skip。
    """

    def test_inline_attachment_flag_exists(self, dws):
        """--inline-attachment flag 应被 CLI 识别（出现在 help 输出中）。"""
        result = dws.run_raw("mail", "message", "send", "--help")
        help_text = result.stdout + result.stderr
        assert "--inline-attachment" in help_text, (
            f"--inline-attachment flag 未出现在 help 输出中:\n{help_text[:500]}"
        )

    def test_inline_attachment_missing_file(self, dws, email_addr):
        """指定不存在的内联附件文件应报错。"""
        result = dws.run_raw(
            "mail", "message", "send",
            "--from", email_addr,
            "--to", email_addr,
            "--subject", "内联附件测试",
            "--body", "图片如下",
            "--inline-attachment", "/tmp/nonexistent_image_99999.png",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "cannot" in combined, (
            f"指定不存在的内联附件应报错, returncode={result.returncode}, "
            f"stdout={result.stdout[:300]}, stderr={result.stderr[:300]}"
        )

    def test_send_single_inline_attachment(self, dws, email_addr):
        """发送带单个内联附件的邮件（正文嵌入图片）。"""
        # 创建一个 10×10 红色像素的 PNG（肉眼可见）
        import struct, zlib
        def make_png(width=10, height=10):
            sig = b'\x89PNG\r\n\x1a\n'
            def chunk(t, d):
                return struct.pack('>I', len(d)) + t + d + struct.pack('>I', zlib.crc32(t + d) & 0xffffffff)
            ihdr = chunk(b'IHDR', struct.pack('>IIBBBBB', width, height, 8, 2, 0, 0, 0))
            # 每行：过滤字节 0x00 + width 个 RGB 像素（红色 0xFF0000）
            row = b'\x00' + b'\xff\x00\x00' * width
            raw = row * height
            idat = chunk(b'IDAT', zlib.compress(raw))
            iend = chunk(b'IEND', b'')
            return sig + ihdr + idat + iend

        with tempfile.NamedTemporaryFile(suffix=".png", prefix="inline_test_", delete=False) as f:
            f.write(make_png())
            png_path = f.name

        try:
            png_name = os.path.basename(png_path)  # 使用实际文件名构造占位符
            data = dws.run(
                "mail", "message", "send",
                "--from", email_addr,
                "--to", email_addr,
                "--subject", f"内联图片测试_{int(time.time())}",
                "--body", f"图片如下：[inline:{png_name}]",
                "--inline-attachment", png_path,
            )
            assert isinstance(data, dict) and len(data) > 0, (
                f"内联附件发送结果应为非空字典: {data}"
            )
        finally:
            try:
                os.unlink(png_path)
            except OSError:
                pass

    def test_send_mixed_attachment_and_inline(self, dws, email_addr, tmp_attachment):
        """同时发送普通附件和内联附件。"""
        import struct, zlib
        def make_png(width=10, height=10):
            sig = b'\x89PNG\r\n\x1a\n'
            def chunk(t, d):
                return struct.pack('>I', len(d)) + t + d + struct.pack('>I', zlib.crc32(t + d) & 0xffffffff)
            ihdr = chunk(b'IHDR', struct.pack('>IIBBBBB', width, height, 8, 2, 0, 0, 0))
            row = b'\x00' + b'\x00\xff\x00' * width  # 绿色像素，与单图测试红色区分
            raw = row * height
            idat = chunk(b'IDAT', zlib.compress(raw))
            iend = chunk(b'IEND', b'')
            return sig + ihdr + idat + iend

        with tempfile.NamedTemporaryFile(suffix=".png", prefix="inline_mixed_", delete=False) as f:
            f.write(make_png())
            png_path = f.name

        try:
            png_name = os.path.basename(png_path)  # 使用实际文件名构造占位符
            data = dws.run(
                "mail", "message", "send",
                "--from", email_addr,
                "--to", email_addr,
                "--subject", f"混合附件测试_{int(time.time())}",
                "--body", f"见附件，图片如下：[inline:{png_name}]",
                "--attachment", tmp_attachment,
                "--inline-attachment", png_path,
            )
            assert isinstance(data, dict) and len(data) > 0, (
                f"混合附件发送结果应为非空字典: {data}"
            )
        finally:
            try:
                os.unlink(png_path)
            except OSError:
                pass
