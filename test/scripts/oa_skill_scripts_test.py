#!/usr/bin/env python3
"""Regression tests for deterministic OA create preflight projection."""

import importlib.util
import json
import sys
import unittest
from pathlib import Path


sys.dont_write_bytecode = True

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "skills" / "multi" / "dingtalk-misc" / "scripts" / "oa_create_preflight.py"


def load_script():
    spec = importlib.util.spec_from_file_location("oa_create_preflight", SCRIPT)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load {SCRIPT}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


PREFLIGHT = load_script()


class OAFormProjectionTest(unittest.TestCase):
    def test_projects_items_without_exposing_raw_template_metadata(self):
        schema = {
            "title": "日常报销",
            "largeUnusedMetadata": "x" * 20_000,
            "items": [
                {
                    "children": [
                        {
                            "componentName": "TextField",
                            "props": {
                                "label": "系统金额",
                                "id": "hidden",
                                "hideInDesigner": True,
                            },
                        },
                        {
                            "componentName": "DDSelectField",
                            "props": {
                                "label": "费用类型",
                                "id": "type",
                                "required": True,
                                "options": [
                                    {"key": "one", "value": "交通费"},
                                    {"key": "two", "value": "住宿费"},
                                ],
                            },
                        },
                        {
                            "componentName": "TableField",
                            "props": {"label": "报销明细", "id": "table"},
                            "children": [
                                {
                                    "componentName": "MoneyField",
                                    "props": {"label": "金额", "required": True},
                                },
                                {
                                    "componentName": "CascadeField",
                                    "props": {"label": "费用类型", "required": True},
                                },
                                {
                                    "componentName": "TextNote",
                                    "props": {"label": "提示"},
                                },
                            ],
                        },
                        {
                            "componentName": "DDHolidayField",
                            "props": {"label": "请假时间", "required": True},
                        },
                    ]
                }
            ],
        }
        payload = {
            "success": True,
            "result": {"content": json.dumps(schema, ensure_ascii=False)},
        }

        projected = PREFLIGHT.project_form_schema(payload, "PROC-1")

        self.assertEqual("PROC-1", projected["processCode"])
        self.assertEqual(["费用类型", "报销明细", "请假时间"], [
            field["name"] for field in projected["fields"]
        ])
        self.assertEqual(["交通费", "住宿费"], projected["fields"][0]["options"])
        self.assertEqual("table_rows_json", projected["fields"][1]["valueKind"])
        self.assertEqual("金额", projected["fields"][1]["children"][0]["name"])
        self.assertEqual("decimal_string", projected["fields"][1]["children"][0]["valueKind"])
        self.assertEqual(
            ["CascadeField", "DDHolidayField"],
            [blocker["componentName"] for blocker in projected["blockers"]],
        )
        self.assertTrue(projected["needsComponentReference"])
        encoded = json.dumps(projected, ensure_ascii=False)
        self.assertNotIn("largeUnusedMetadata", encoded)
        self.assertNotIn("系统金额", encoded)
        self.assertNotIn("提示", encoded)
        self.assertLess(len(encoded), 2_500)

    def test_date_range_uses_first_label_and_preserves_labels(self):
        payload = {
            "result": {
                "content": {
                    "items": [
                        {
                            "componentName": "DDDateRangeField",
                            "props": {
                                "label": ["开始时间", "结束时间"],
                                "format": "yyyy-MM-dd HH:mm",
                            },
                        }
                    ]
                }
            }
        }
        field = PREFLIGHT.project_form_schema(payload)["fields"][0]
        self.assertEqual("开始时间", field["name"])
        self.assertEqual(["开始时间", "结束时间"], field["labels"])
        self.assertEqual("date_range_json_array", field["valueKind"])

    def test_template_agnostic_vehicle_and_item_forms(self):
        templates = {
            "用车申请": [
                {
                    "componentName": "DDDateRangeField",
                    "props": {"label": ["用车日期", "返回日期"]},
                },
                {
                    "componentName": "TableField",
                    "props": {"label": "车辆明细"},
                    "children": [
                        {
                            "componentName": "NumberField",
                            "props": {"label": "数量（辆）"},
                        }
                    ],
                },
            ],
            "物品领用": [
                {
                    "componentName": "TextField",
                    "props": {"label": "物品用途"},
                },
                {
                    "componentName": "TableField",
                    "props": {"label": "物品明细"},
                    "children": [
                        {
                            "componentName": "TextField",
                            "props": {"label": "物品名称"},
                        },
                        {
                            "componentName": "NumberField",
                            "props": {"label": "数量"},
                        },
                    ],
                },
            ],
        }

        for title, items in templates.items():
            with self.subTest(title=title):
                payload = {
                    "result": {
                        "content": json.dumps({"title": title, "items": items})
                    }
                }
                projected = PREFLIGHT.project_form_schema(payload, "PROC-generic")
                self.assertEqual(title, projected["title"])
                self.assertEqual([], projected["blockers"])
                self.assertFalse(projected["needsComponentReference"])
                self.assertNotIn(title, SCRIPT.read_text(encoding="utf-8"))


class OAForecastProjectionTest(unittest.TestCase):
    def test_projects_standard_target_select_roles_without_node_reference(self):
        payload = {
            "success": True,
            "result": {
                "forecastSuccess": True,
                "processCode": "PROC-1",
                "userId": "self-user",
                "staticWorkflow": True,
                "workflowActivityRuleVOs": [
                    {
                        "activityName": "审批人",
                        "activityType": "target_select",
                        "targetSelect": True,
                        "largeUnusedMetadata": "x" * 10_000,
                        "workflowActor": {
                            "actorKey": "manual-approver",
                            "actorType": "approver",
                            "required": True,
                            "allowedMulti": False,
                        },
                    },
                    {
                        "activityName": "抄送人",
                        "activityType": "target_select",
                        "targetSelect": True,
                        "workflowActor": {
                            "actorKey": "manual-notifier",
                            "actorType": "notifier",
                            "required": False,
                            "allowedMulti": True,
                        },
                    },
                ],
            },
        }

        projected = PREFLIGHT.project_forecast(payload)

        self.assertFalse(projected["needsNodeReference"])
        self.assertEqual(
            ["approver", "notifier"],
            [item["actorType"] for item in projected["targetSelections"]],
        )
        self.assertNotIn("largeUnusedMetadata", json.dumps(projected))

    def test_preserves_error_envelope(self):
        payload = {"error": {"reason": "business_error", "message": "系统错误"}}
        self.assertIs(payload, PREFLIGHT.project_forecast(payload))
        self.assertIs(payload, PREFLIGHT.project_form_schema(payload))


if __name__ == "__main__":
    unittest.main()
