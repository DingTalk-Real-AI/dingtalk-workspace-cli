"""
test_05_block_insert.py — 块元素插入测试 (doc block insert)

Commands tested:
  1. dws doc block insert --element (blockquote)
  2. dws doc block insert --element (columns)
  3. dws doc block insert --element (table)
  4. dws doc block insert --text (快捷段落)
  5. dws doc block insert --heading (快捷标题)

实际返回格式 (2026-05):
  doc block insert: {"success": true, ...}
"""

import json
import time
import pytest


class TestBlockInsertBlockquote:
    """dws doc block insert — 引用块 (blockquote)"""

    def test_insert_blockquote(self, dws, test_doc_node_id):
        """插入引用块应成功。"""
        element = json.dumps({
            "blockType": "blockquote",
            "blockquote": {"text": "这是一条引用内容"},
        }, ensure_ascii=False)
        data = dws.run(
            "doc", "block", "insert",
            "--node", test_doc_node_id,
            "--element", element,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_insert_blockquote_empty_text(self, dws, test_doc_node_id):
        """插入空文本引用块应成功。"""
        element = json.dumps({
            "blockType": "blockquote",
            "blockquote": {"text": ""},
        }, ensure_ascii=False)
        data = dws.run(
            "doc", "block", "insert",
            "--node", test_doc_node_id,
            "--element", element,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_insert_blockquote_invalid_node(self, dws):
        """无效 nodeId 插入引用块应报错。"""
        element = json.dumps({
            "blockType": "blockquote",
            "blockquote": {"text": "不该成功"},
        }, ensure_ascii=False)
        result = dws.run_raw(
            "doc", "block", "insert",
            "--node", "INVALID_NODE_99999",
            "--element", element,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestBlockInsertColumns:
    """dws doc block insert — 分栏块 (columns)"""

    def test_insert_two_columns(self, dws, test_doc_node_id):
        """插入两栏分栏块应成功。"""
        element = json.dumps({
            "blockType": "columns",
            "columns": {"size": 2},
            "children": [
                {"blockType": "paragraph", "paragraph": {"text": "左栏"}},
                {"blockType": "paragraph", "paragraph": {"text": "右栏"}},
            ],
        }, ensure_ascii=False)
        data = dws.run(
            "doc", "block", "insert",
            "--node", test_doc_node_id,
            "--element", element,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_insert_three_columns(self, dws, test_doc_node_id):
        """插入三栏分栏块应成功。"""
        element = json.dumps({
            "blockType": "columns",
            "columns": {"size": 3},
            "children": [
                {"blockType": "paragraph", "paragraph": {"text": "第一栏"}},
                {"blockType": "paragraph", "paragraph": {"text": "第二栏"}},
                {"blockType": "paragraph", "paragraph": {"text": "第三栏"}},
            ],
        }, ensure_ascii=False)
        data = dws.run(
            "doc", "block", "insert",
            "--node", test_doc_node_id,
            "--element", element,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_insert_columns_invalid_node(self, dws):
        """无效 nodeId 插入分栏块应报错。"""
        element = json.dumps({
            "blockType": "columns",
            "columns": {"size": 2},
            "children": [
                {"blockType": "paragraph", "paragraph": {"text": "左"}},
                {"blockType": "paragraph", "paragraph": {"text": "右"}},
            ],
        }, ensure_ascii=False)
        result = dws.run_raw(
            "doc", "block", "insert",
            "--node", "INVALID_NODE_99999",
            "--element", element,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestBlockInsertTable:
    """dws doc block insert — 表格块 (table)"""

    def test_insert_table_basic(self, dws, test_doc_node_id):
        """插入 2 行 3 列表格应成功。"""
        element = json.dumps({
            "blockType": "table",
            "table": {
                "rolSize": 2,
                "colSize": 3,
                "cells": [
                    ["姓名", "部门", "工号"],
                    ["张三", "工程部", "E001"],
                ],
            },
        }, ensure_ascii=False)
        data = dws.run(
            "doc", "block", "insert",
            "--node", test_doc_node_id,
            "--element", element,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_insert_table_single_row(self, dws, test_doc_node_id):
        """插入只有表头的单行表格应成功。"""
        element = json.dumps({
            "blockType": "table",
            "table": {
                "rolSize": 1,
                "colSize": 2,
                "cells": [["列A", "列B"]],
            },
        }, ensure_ascii=False)
        data = dws.run(
            "doc", "block", "insert",
            "--node", test_doc_node_id,
            "--element", element,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_insert_table_invalid_node(self, dws):
        """无效 nodeId 插入表格应报错。"""
        element = json.dumps({
            "blockType": "table",
            "table": {
                "rolSize": 1,
                "colSize": 1,
                "cells": [["test"]],
            },
        }, ensure_ascii=False)
        result = dws.run_raw(
            "doc", "block", "insert",
            "--node", "INVALID_NODE_99999",
            "--element", element,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestBlockInsertShortcuts:
    """dws doc block insert — 快捷方式 (--text / --heading)"""

    def test_insert_text_shortcut(self, dws, test_doc_node_id):
        """--text 快捷插入段落应成功。"""
        data = dws.run(
            "doc", "block", "insert",
            "--node", test_doc_node_id,
            "--text", "快捷插入的段落文字",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_insert_heading_shortcut(self, dws, test_doc_node_id):
        """--heading + --level 快捷插入标题应成功。"""
        data = dws.run(
            "doc", "block", "insert",
            "--node", test_doc_node_id,
            "--heading", "测试二级标题",
            "--level", "2",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_insert_missing_node(self, dws):
        """缺少 --node 应报错。"""
        result = dws.run_raw(
            "doc", "block", "insert",
            "--text", "没有指定文档",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        )


class TestBlockInsertOrderedList:
    """dws doc block insert — 有序列表 (orderedList)"""

    def test_insert_ordered_list_single_item(self, dws, test_doc_node_id):
        """插入有序列表单个 item 应成功。"""
        element = json.dumps({
            "blockType": "orderedList",
            "orderedList": {"list": {"listId": "test-ol-1"}},
            "children": [{"text": "第一项"}],
        }, ensure_ascii=False)
        data = dws.run(
            "doc", "block", "insert",
            "--node", test_doc_node_id,
            "--element", element,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_insert_ordered_list_same_list_id(self, dws, test_doc_node_id):
        """同一 listId 连续插入多个 item 应属于同一列表。"""
        list_id = f"test-ol-{int(time.time())}"
        for text in ["有序第一项", "有序第二项", "有序第三项"]:
            element = json.dumps({
                "blockType": "orderedList",
                "orderedList": {"list": {"listId": list_id}},
                "children": [{"text": text}],
            }, ensure_ascii=False)
            data = dws.run(
                "doc", "block", "insert",
                "--node", test_doc_node_id,
                "--element", element,
            )
            assert data.get("success") is True, f"插入 '{text}' 失败: {data}"

    def test_insert_ordered_list_invalid_node(self, dws):
        """无效 nodeId 插入有序列表应报错。"""
        element = json.dumps({
            "blockType": "orderedList",
            "orderedList": {"list": {"listId": "test-ol-err"}},
            "children": [{"text": "不该成功"}],
        }, ensure_ascii=False)
        result = dws.run_raw(
            "doc", "block", "insert",
            "--node", "INVALID_NODE_99999",
            "--element", element,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestBlockInsertUnorderedList:
    """dws doc block insert — 无序列表 (unorderedList)"""

    def test_insert_unordered_list_single_item(self, dws, test_doc_node_id):
        """插入无序列表单个 item 应成功。"""
        element = json.dumps({
            "blockType": "unorderedList",
            "unorderedList": {"list": {"listId": "test-ul-1"}},
            "children": [{"text": "第一项"}],
        }, ensure_ascii=False)
        data = dws.run(
            "doc", "block", "insert",
            "--node", test_doc_node_id,
            "--element", element,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_insert_unordered_list_same_list_id(self, dws, test_doc_node_id):
        """同一 listId 连续插入多个 item 应属于同一列表。"""
        list_id = f"test-ul-{int(time.time())}"
        for text in ["无序第一项", "无序第二项", "无序第三项"]:
            element = json.dumps({
                "blockType": "unorderedList",
                "unorderedList": {"list": {"listId": list_id}},
                "children": [{"text": text}],
            }, ensure_ascii=False)
            data = dws.run(
                "doc", "block", "insert",
                "--node", test_doc_node_id,
                "--element", element,
            )
            assert data.get("success") is True, f"插入 '{text}' 失败: {data}"

    def test_insert_unordered_list_invalid_node(self, dws):
        """无效 nodeId 插入无序列表应报错。"""
        element = json.dumps({
            "blockType": "unorderedList",
            "unorderedList": {"list": {"listId": "test-ul-err"}},
            "children": [{"text": "不该成功"}],
        }, ensure_ascii=False)
        result = dws.run_raw(
            "doc", "block", "insert",
            "--node", "INVALID_NODE_99999",
            "--element", element,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
