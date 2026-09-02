#!/usr/bin/env python3
"""Compact read-only OA form-schema and forecast-process responses."""

import argparse
import json
import sys
from typing import Any, Dict, Iterable, List, Optional


VALUE_KINDS = {
    "TextField": "text",
    "TextareaField": "text",
    "NumberField": "number_string",
    "DDSelectField": "option_text",
    "DDMultiSelectField": "option_text_json_array_string",
    "DDDateField": "date_yyyy_mm_dd",
    "DDDateRangeField": "date_range_json_array_string",
    "PhoneField": "phone_string",
    "IdCardField": "id_card_string",
    "MoneyField": "decimal_string",
    "InnerContactField": "user_id_or_json_array",
    "DepartmentField": "dept_id_or_json_array",
    "AddressField": "address_json_array_string",
    "DDPhotoField": "url_json_array_string",
    "DDAttachment": "attachment_json_array_string",
    "StarRatingField": "number_string",
    "RelateField": "process_instance_id",
    "TableField": "table_rows_json_string",
}

AUTOMATIC_COMPONENTS = {
    "TextNote": "display_only",
    "CalculateField": "computed",
    "SeqNumberField": "generated",
}

CLIENT_ONLY_COMPONENTS = {
    "SignatureField",
    "OcrTextField",
    "OcrIdCardField",
    "InvoiceField",
    "RecipientAccountField",
}


def _truthy(value: Any) -> bool:
    if isinstance(value, str):
        return value.strip().lower() in {"1", "true", "yes"}
    return bool(value)


def _label(props: Dict[str, Any]) -> tuple[Optional[str], Optional[List[str]]]:
    raw = props.get("label")
    if isinstance(raw, list):
        labels = [str(item) for item in raw if str(item).strip()]
        return (labels[0] if labels else None), (labels or None)
    if raw is None or not str(raw).strip():
        return None, None
    return str(raw), None


def _option_values(raw: Any) -> Optional[List[str]]:
    if not isinstance(raw, list):
        return None
    values: List[str] = []
    for item in raw:
        if isinstance(item, dict):
            value = item.get("value")
        else:
            value = item
        if value is not None:
            values.append(str(value))
    return values or None


def _holiday_options(raw: Any) -> Optional[List[Dict[str, str]]]:
    if not isinstance(raw, list):
        return None
    options = []
    for item in raw:
        if not isinstance(item, dict):
            continue
        option = {
            key: str(item[key])
            for key in (
                "name",
                "value",
                "leaveCode",
                "unit",
                "displayUnit",
                "bizType",
            )
            if item.get(key) is not None
        }
        if option:
            options.append(option)
    return options or None


def _support(
    component_name: str, props: Dict[str, Any]
) -> tuple[str, Optional[str]]:
    if component_name in VALUE_KINDS:
        return "supported", VALUE_KINDS[component_name]
    if component_name in AUTOMATIC_COMPONENTS:
        return "automatic", AUTOMATIC_COMPONENTS[component_name]
    if component_name == "DDHolidayField":
        return "supported", "holiday_suite_request"
    if component_name == "DDBizSuite" and props.get("bizType") == "attendance.supply":
        return "supported", "supply_suite_request"
    if component_name in CLIENT_ONLY_COMPONENTS:
        return "client_only", None
    return "unknown", None


def _is_hidden(props: Dict[str, Any]) -> bool:
    return any(
        _truthy(props.get(key))
        for key in (
            "hideInDesigner",
            "hidden",
            "hide",
            "invisible",
            "disabled",
            "readOnly",
        )
    )


def _project_component(component: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    component_name = component.get("componentName")
    props = component.get("props") or {}
    if not isinstance(component_name, str) or not isinstance(props, dict):
        return None

    required = _truthy(props.get("required"))
    support, value_kind = _support(component_name, props)
    hidden = _is_hidden(props)
    if hidden or support == "automatic":
        return None

    name, labels = _label(props)
    missing_label = not name
    if missing_label:
        if component_name == "DDBizSuite":
            name = str(
                props.get("bizType")
                or props.get("bizAlias")
                or props.get("id")
                or "DDBizSuite"
            )
        else:
            name = str(props.get("id") or component_name)

    projected: Dict[str, Any] = {
        "name": name,
        "componentName": component_name,
        "required": required,
        "support": support,
    }
    if missing_label:
        projected["missingLabel"] = True
    for key, value in (
        ("id", props.get("id")),
        ("labels", labels),
        ("valueKind", value_kind),
        (
            "options",
            _holiday_options(props.get("options"))
            if component_name == "DDHolidayField"
            else _option_values(props.get("options")),
        ),
        ("unit", props.get("unit")),
        ("format", props.get("format")),
        ("bizType", props.get("bizType")),
        ("bizAlias", props.get("bizAlias")),
        ("attendTypeLabel", props.get("attendTypeLabel")),
        ("multiple", props.get("multiple")),
        ("choice", props.get("choice")),
        ("needDetail", props.get("needDetail")),
    ):
        if value is not None and value != [] and value != "":
            projected[key] = value

    if component_name in {"TableField", "DDBizSuite"}:
        children = []
        for child in component.get("children") or []:
            if isinstance(child, dict):
                item = _project_component(child)
                if item is not None:
                    children.append(item)
        projected["children"] = children
    return projected


def _walk_components(items: Iterable[Any]) -> Iterable[Dict[str, Any]]:
    for item in items:
        if not isinstance(item, dict):
            continue
        projected = _project_component(item)
        if projected is not None:
            yield projected
        if item.get("componentName") not in {"TableField", "DDBizSuite"}:
            children = item.get("children") or []
            if isinstance(children, list):
                yield from _walk_components(children)


def _flatten_fields(fields: Iterable[Dict[str, Any]]) -> Iterable[Dict[str, Any]]:
    for field in fields:
        yield field
        children = field.get("children") or []
        if isinstance(children, list):
            yield from _flatten_fields(children)


def _decode_schema_content(content: Any) -> Dict[str, Any]:
    if isinstance(content, str):
        content = json.loads(content)
    if not isinstance(content, dict):
        raise ValueError("form-schema result.content must be a JSON object")
    return content


def project_form_schema(payload: Dict[str, Any], process_code: str = "") -> Dict[str, Any]:
    if "error" in payload:
        return payload
    result = payload.get("result")
    if not isinstance(result, dict):
        raise ValueError("form-schema response is missing result")
    schema = _decode_schema_content(result.get("content"))
    items = schema.get("items") or []
    if not isinstance(items, list):
        raise ValueError("form-schema content.items must be an array")

    fields = list(_walk_components(items))
    all_fields = list(_flatten_fields(fields))
    blockers = []
    for field in all_fields:
        if not (
            (
                field["required"]
                and (
                    field["support"] != "supported"
                    or field.get("missingLabel")
                )
            )
            or (
                field["componentName"] == "DDBizSuite"
                and field["support"] == "unknown"
            )
        ):
            continue
        blocker = {
            "name": field["name"],
            "componentName": field["componentName"],
            "support": field["support"],
        }
        if field.get("missingLabel"):
            blocker["missingLabel"] = True
        blockers.append(blocker)
    optional_unavailable = [
        {
            "name": field["name"],
            "componentName": field["componentName"],
            "support": field["support"],
        }
        for field in all_fields
        if (
            not field["required"]
            and field["support"] != "supported"
            and not (
                field["componentName"] == "DDBizSuite"
                and field["support"] == "unknown"
            )
        )
    ]
    return {
        "success": payload.get("success", True),
        "processCode": process_code or result.get("processCode"),
        "title": schema.get("title"),
        "fields": fields,
        "blockers": blockers,
        "optionalUnavailable": optional_unavailable,
        "needsComponentReference": any(
            field["support"] == "unknown" or field.get("missingLabel")
            for field in all_fields
        ),
        "fieldCount": len(all_fields),
    }


def project_forecast(payload: Dict[str, Any]) -> Dict[str, Any]:
    if "error" in payload:
        return payload
    result = payload.get("result")
    if not isinstance(result, dict):
        raise ValueError("forecast response is missing result")

    if result.get("forecastSuccess") is not True:
        diagnostic_keys = (
            "dingOpenErrcode",
            "errorCode",
            "errorMsg",
            "errorMessage",
            "message",
            "msg",
            "hint",
            "failedReason",
            "failureReason",
            "failureMessage",
            "requestId",
        )
        server = {}
        for scope, source in (("response", payload), ("result", result)):
            details = {
                key: source[key]
                for key in diagnostic_keys
                if key in source and source[key] not in (None, "", [], {})
            }
            if details:
                server[scope] = details
        error = {
            "reason": "forecast_failed",
            "message": "流程预测未成功；不得继续创建审批单",
        }
        if server:
            error["server"] = server
        return {
            "success": False,
            "forecastSuccess": result.get("forecastSuccess"),
            "processCode": result.get("processCode"),
            "error": error,
        }

    nodes = []
    selections = []
    unusual_actor = False
    for raw_node in result.get("workflowActivityRuleVOs") or []:
        if not isinstance(raw_node, dict):
            continue
        actor = raw_node.get("workflowActor") or {}
        if not isinstance(actor, dict):
            actor = {}
        actor_type = actor.get("actorType")
        actioners = []
        for raw_actioner in raw_node.get("activityActioners") or []:
            if isinstance(raw_actioner, dict):
                actioners.append(
                    {
                        key: raw_actioner.get(key)
                        for key in ("name", "emplId")
                        if raw_actioner.get(key) is not None
                    }
                )
        node = {
            "name": raw_node.get("activityName"),
            "activityType": raw_node.get("activityType"),
            "targetSelect": bool(raw_node.get("targetSelect")),
            "actorType": actor_type,
            "actorKey": actor.get("actorKey"),
            "required": bool(actor.get("required")),
            "allowedMulti": bool(actor.get("allowedMulti")),
            "actioners": actioners,
        }
        nodes.append({key: value for key, value in node.items() if value not in (None, [])})
        if node["targetSelect"]:
            selections.append(
                {
                    "actorKey": actor.get("actorKey"),
                    "actorType": actor_type,
                    "required": node["required"],
                    "allowedMulti": node["allowedMulti"],
                }
            )
        if actor_type not in (None, "approver", "notifier", "bizHandler"):
            unusual_actor = True

    return {
        "success": payload.get("success", True),
        "forecastSuccess": result.get("forecastSuccess"),
        "processCode": result.get("processCode"),
        "userId": result.get("userId"),
        "staticWorkflow": result.get("staticWorkflow"),
        "nodes": nodes,
        "targetSelections": selections,
        "needsNodeReference": unusual_actor,
    }


def _read_payload() -> Dict[str, Any]:
    payload = json.load(sys.stdin)
    if not isinstance(payload, dict):
        raise ValueError("input must be one JSON object")
    return payload


def main(argv: Optional[List[str]] = None) -> int:
    parser = argparse.ArgumentParser(
        description="Project OA preflight responses without exposing raw template metadata"
    )
    subparsers = parser.add_subparsers(dest="mode", required=True)
    form_parser = subparsers.add_parser("form-schema")
    form_parser.add_argument("--process-code", default="")
    subparsers.add_parser("forecast")
    args = parser.parse_args(argv)

    try:
        payload = _read_payload()
        if args.mode == "form-schema":
            projected = project_form_schema(payload, args.process_code)
        else:
            projected = project_forecast(payload)
        print(json.dumps(projected, ensure_ascii=False, separators=(",", ":")))
        return 1 if "error" in projected else 0
    except (json.JSONDecodeError, ValueError) as exc:
        print(
            json.dumps(
                {"error": {"reason": "projection_error", "message": str(exc)}},
                ensure_ascii=False,
                separators=(",", ":"),
            )
        )
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
