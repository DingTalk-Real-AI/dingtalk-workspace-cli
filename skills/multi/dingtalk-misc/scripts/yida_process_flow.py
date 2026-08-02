#!/usr/bin/env python3
"""Shared Yida process-flow schema builder.

The schema consumed by integration automation and design process update is the
same shape:

    {"props": {"processCode": "TPROC-xxx", ...}, "nodes": [...]}

This module accepts either that ready-to-save shape or the flatter node objects
described in references/yida-process-node.md, then normalizes them into
the save payload used by `dws yida design process update --content`.
"""
from __future__ import annotations

import copy
from typing import Any


FLOW_PROP_KEYS = ("allowWithdraw", "allowCollaboration", "allowTemporaryStorage")
FLOW_TOP_LEVEL_KEYS = ("flowConfig", "formulaRules", "approvalSummary", "nodeI18nKeyMap")

NODE_CORE_KEYS = {
    "type",
    "componentName",
    "nodeType",
    "tool",
    "toolName",
    "kind",
    "nodeId",
    "name",
    "description",
    "title",
    "approvalType",
    "nextId",
    "prevId",
    "props",
    "childNodes",
}

TYPE_ALIASES = {
    "create_trigger_node": "trigger",
    "trigger_node": "trigger",
    "trigger": "trigger",
    "create_timer_trigger_node": "trigger",
    "timer_trigger_node": "trigger",
    "timerTrigger": "trigger",
    "create_data_create_node": "dataCreate",
    "data_create_node": "dataCreate",
    "AddDataNode": "dataCreate",
    "addData": "dataCreate",
    "dataCreate": "dataCreate",
    "create_data_update_node": "dataUpdate",
    "data_update_node": "dataUpdate",
    "UpdateDataNode": "dataUpdate",
    "updateData": "dataUpdate",
    "dataUpdate": "dataUpdate",
    "create_data_retrieve_node": "dataRetrieve",
    "data_retrieve_node": "dataRetrieve",
    "GetSingleDataNode": "dataRetrieve",
    "GetBatchDataNode": "dataRetrieve",
    "getSingleData": "dataRetrieve",
    "getBatchData": "dataRetrieve",
    "dataRetrieve": "dataRetrieve",
    "DeleteDataNode": "dataDelete",
    "deleteData": "dataDelete",
    "dataDelete": "dataDelete",
    "create_connector_node": "connector",
    "connector_node": "connector",
    "ConnectorNode": "connector",
    "connector": "connector",
    "create_send_message_node": "sendMessage",
    "send_message_node": "sendMessage",
    "SendMessageNode": "sendMessage",
    "sendMessage": "sendMessage",
    "create_send_email_node": "sendEmail",
    "send_email_node": "sendEmail",
    "SendEmailNode": "sendEmail",
    "sendEmail": "sendEmail",
    "create_send_card_node": "sendCard",
    "send_card_node": "sendCard",
    "SendCardNode": "sendCard",
    "CardNode": "sendCard",
    "sendCard": "sendCard",
    "create_update_card_node": "updateCard",
    "update_card_node": "updateCard",
    "UpdateCardNode": "updateCard",
    "CardUpdateNode": "updateCard",
    "updateCard": "updateCard",
    "create_route_node": "route",
    "route_node": "route",
    "ConditionContainer": "route",
    "route": "route",
    "ConditionNode": "condition",
    "ParallelNode": "condition",
    "condition": "condition",
    "create_foreach_node": "forEach",
    "foreach_node": "forEach",
    "CycleContainer": "forEach",
    "forEach": "forEach",
    "foreach": "forEach",
    "create_initiate_approval_node": "initiateApproval",
    "initiate_approval_node": "initiateApproval",
    "InitiateApprovalNode": "initiateApproval",
    "subProcess": "initiateApproval",
    "initiateApproval": "initiateApproval",
    "create_approver_node": "approver",
    "approver_node": "approver",
    "ApprovalNode": "approval",
    "MultiApprovalNode": "approval",
    "approver": "approver",
    "create_executor_node": "executor",
    "executor_node": "executor",
    "OperatorNode": "operator",
    "executor": "executor",
    "create_carbon_copy_node": "carbonCopy",
    "carbon_copy_node": "carbonCopy",
    "CarbonNode": "carbon",
    "carbonCopy": "carbonCopy",
    "GroovyNode": "CodeExecutor",
    "groovy": "CodeExecutor",
    "JavaScriptNode": "CodeExecutor",
    "javascript": "CodeExecutor",
    "AINode": "AIExecutor",
    "ai": "AIExecutor",
    "create_finish_node": "finish",
    "finish_node": "finish",
    "EndNode": "finish",
    "finish": "finish",
}

DEFAULT_NODE_NAMES = {
    "apply": "发起",
    "approval": "审批人",
    "operator": "执行人",
    "carbon": "抄送人",
    "trigger": "表单事件触发",
    "dataCreate": "新增数据",
    "dataUpdate": "更新数据",
    "dataRetrieve": "获取数据",
    "dataDelete": "删除数据",
    "connector": "连接器",
    "sendMessage": "消息通知",
    "sendEmail": "发送邮件",
    "sendCard": "发送卡片",
    "updateCard": "更新卡片",
    "route": "ConditionContainer",
    "condition": "条件",
    "forEach": "循环容器",
    "foreach": "循环容器",
    "initiateApproval": "发起审批",
    "CodeExecutor": "脚本",
    "AIExecutor": "AI 节点",
    "approver": "审批人",
    "executor": "执行人",
    "carbonCopy": "抄送人",
    "finish": "结束",
}

VIEW_COMPONENT_BY_TYPE = {
    "apply": "ApplyNode",
    "approval": "ApprovalNode",
    "operator": "OperatorNode",
    "carbon": "CarbonNode",
    "connector": "ConnectorNode",
    "dataCreate": "AddDataNode",
    "dataUpdate": "UpdateDataNode",
    "dataRetrieve": "GetSingleDataNode",
    "dataDelete": "DeleteDataNode",
    "sendMessage": "SendMessageNode",
    "sendEmail": "SendEmailNode",
    "sendCard": "CardNode",
    "updateCard": "CardUpdateNode",
    "route": "ConditionContainer",
    "condition": "ConditionNode",
    "initiateApproval": "InitiateApprovalNode",
    "approver": "ApprovalNode",
    "executor": "OperatorNode",
    "carbonCopy": "CarbonNode",
    "forEach": "CycleContainer",
    "foreach": "CycleContainer",
    "AIExecutor": "AINode",
    "CodeExecutor": "JavaScriptNode",
    "finish": "EndNode",
}

VIEW_COMPONENT_RULE_KEY = {
    "ConnectorNode": "connectorRules",
    "AddDataNode": "addDataRules",
    "UpdateDataNode": "updateDataRules",
    "DeleteDataNode": "deleteData",
    "GetSingleDataNode": "getData",
    "GetBatchDataNode": "getData",
    "SendMessageNode": "sendMessageRules",
    "SendEmailNode": "sendEmailRules",
    "CardNode": "cardRules",
    "CardUpdateNode": "cardUpdateRules",
    "InitiateApprovalNode": "initiateApprovalRules",
    "CycleContainer": "cycleContainerRules",
    "AINode": "workFlowRules",
    "GroovyNode": "groovy",
    "JavaScriptNode": "JavaScript",
}

TRIGGER_INPUT_KEYS = {
    "formUuid",
    "formEventType",
    "formEventField",
    "triggerFormEventRecursively",
    "conditions",
    "startDate",
    "repeat",
    "end",
}

UNRESOLVED_CARD_BIZ_ID_VALUES = {
    "__masterdata_form_inst_id",
}


class FlowSchemaError(ValueError):
    """Raised when a flow spec cannot be normalized safely."""


def i18n_name(value: Any, default: str) -> dict[str, str]:
    if isinstance(value, dict):
        result = copy.deepcopy(value)
        result.setdefault("zh_CN", result.get("name") or default)
        result.setdefault("en_US", "")
        result.setdefault("type", "i18n")
        return result
    text = str(value or default)
    return {"zh_CN": text, "en_US": "", "type": "i18n"}


def normalize_next_id(value: Any) -> list[str]:
    if value is None:
        return []
    if isinstance(value, str):
        return [value] if value else []
    if isinstance(value, list):
        return [str(item) for item in value if item not in (None, "")]
    raise FlowSchemaError(f"nextId must be string or list, got {type(value).__name__}")


def normalize_node_type(node: dict[str, Any]) -> str:
    raw = (
        node.get("componentName")
        or node.get("nodeType")
        or node.get("tool")
        or node.get("toolName")
        or node.get("kind")
        or node.get("type")
    )
    if not raw:
        raise FlowSchemaError(f"node {node.get('nodeId') or '<unknown>'} missing type")
    raw = str(raw)
    return TYPE_ALIASES.get(raw, raw)


def _normalize_trigger_props(props: dict[str, Any]) -> dict[str, Any]:
    result = copy.deepcopy(props)
    inputs = result.get("inputs")
    if inputs is None:
        inputs = {}
    if not isinstance(inputs, dict):
        raise FlowSchemaError("trigger props.inputs must be object")

    for key in list(TRIGGER_INPUT_KEYS):
        if key in result:
            inputs[key] = result.pop(key)

    if "triggerType" not in result:
        result["triggerType"] = "TimerEvent" if ("repeat" in inputs or "startDate" in inputs) else "FormEvent"
    result["inputs"] = inputs
    return result


def _script_outputs_schema(outputs: Any) -> list[dict[str, Any]]:
    if not isinstance(outputs, list):
        return []
    result = []
    for item in outputs:
        if not isinstance(item, dict):
            continue
        result.append({
            "description": item.get("desc") or item.get("description") or "",
            "name": item.get("name"),
            "type": item.get("type"),
            "valueType": item.get("valueType"),
        })
    return result


def _normalize_process_payload(node_type: str, props: dict[str, Any]) -> tuple[str, dict[str, Any]]:
    result = copy.deepcopy(props)

    if node_type in {"approval", "approver", "operator", "executor"} and isinstance(result.get("approver"), dict):
        approver = result["approver"]
        if approver.get("approvalType") and not result.get("approvalType"):
            result["approvalType"] = copy.deepcopy(approver["approvalType"])
        if approver.get("approvals") is not None and "approvals" not in result:
            result["approvals"] = copy.deepcopy(approver.get("approvals"))
        if isinstance(approver.get("approverRules"), dict) and "approverRules" not in result:
            result["approverRules"] = copy.deepcopy(approver["approverRules"])

    if node_type == "dataRetrieve" and isinstance(result.get("getData"), dict):
        return node_type, copy.deepcopy(result["getData"])
    if node_type == "dataUpdate" and isinstance(result.get("updateDataRules"), dict):
        return node_type, copy.deepcopy(result["updateDataRules"])
    if node_type == "dataDelete" and isinstance(result.get("deleteData"), dict):
        return node_type, copy.deepcopy(result["deleteData"])
    if node_type == "sendMessage" and isinstance(result.get("sendMessageRules"), dict):
        payload = copy.deepcopy(result["sendMessageRules"])
        payload.pop("description", None)
        return node_type, payload
    if node_type == "sendEmail" and isinstance(result.get("sendEmailRules"), dict):
        return node_type, copy.deepcopy(result["sendEmailRules"])
    if node_type == "sendCard":
        if isinstance(result.get("sendCardRules"), dict):
            return node_type, copy.deepcopy(result["sendCardRules"])
        if isinstance(result.get("cardRules"), dict):
            return node_type, copy.deepcopy(result["cardRules"])
    if node_type == "updateCard":
        if isinstance(result.get("updateCardRules"), dict):
            return node_type, copy.deepcopy(result["updateCardRules"])
        if isinstance(result.get("cardUpdateRules"), dict):
            return node_type, copy.deepcopy(result["cardUpdateRules"])
    if node_type == "initiateApproval" and isinstance(result.get("initiateApprovalRules"), dict):
        return node_type, copy.deepcopy(result["initiateApprovalRules"])
    if node_type == "forEach":
        if isinstance(result.get("cycleContainerRules"), dict):
            return "foreach", copy.deepcopy(result["cycleContainerRules"])
        if isinstance(result.get("cycleRules"), dict):
            return "foreach", copy.deepcopy(result["cycleRules"])
        if isinstance(result.get("foreachRules"), dict):
            return "foreach", copy.deepcopy(result["foreachRules"])
    if node_type == "AIExecutor" and isinstance(result.get("workFlowRules"), dict):
        workflow = result["workFlowRules"]
        return node_type, {
            "type": "aiFlow",
            "action": {"flowId": workflow.get("flowId")},
            "outputs": copy.deepcopy(workflow.get("outputs") or []),
            "yidaFieldIdList": copy.deepcopy(workflow.get("yidaFieldIdList") or []),
        }
    if node_type == "CodeExecutor":
        script = result.get("JavaScript") if isinstance(result.get("JavaScript"), dict) else None
        script_type = "JavaScript"
        if script is None and isinstance(result.get("groovy"), dict):
            script = result.get("groovy")
            script_type = "groovy"
        if script is not None:
            return node_type, {
                "inputs": copy.deepcopy(script.get("inputs")),
                "action": copy.deepcopy(script.get("action")),
                "scriptType": script.get("scriptType") or script_type,
                "outputsSchema": _script_outputs_schema(script.get("outputs")),
            }

    return node_type, result


def _contains_originator_keyword(value: Any) -> bool:
    text = _i18n_text(value, "").lower()
    return any(keyword in text for keyword in ("申请人", "发起人", "applicant", "originator"))


def _actioner_targets_originator(value: Any) -> bool:
    if isinstance(value, str):
        return value.lower() in {"originator", "$originator", "applicant", "$applicant"} or value in {"申请人", "发起人"}
    if not isinstance(value, dict):
        return False

    rule_type = str(value.get("type") or value.get("approvalType") or "").lower()
    if rule_type in {"originator", "ext_target_approval_originator"}:
        return True

    for key in ("value", "id", "userId", "fieldId", "fieldName", "label", "name", "description"):
        if _actioner_targets_originator(value.get(key)):
            return True
    return False


def _is_originator_operator_node(node_type: str, node: dict[str, Any], props: dict[str, Any]) -> bool:
    if node_type not in {"operator", "executor"}:
        return False
    if _actioner_targets_originator(props.get("actionerRule")):
        return True
    if _actioner_targets_originator(props.get("executor")) or _actioner_targets_originator(props.get("approver")):
        return True
    if _contains_originator_keyword(node.get("name")) or _contains_originator_keyword(node.get("description")):
        return True
    if _contains_originator_keyword(props.get("name")) or _contains_originator_keyword(props.get("description")):
        return True
    return False


def _operator_action_to_approval_action(action: Any) -> dict[str, Any] | None:
    if isinstance(action, dict):
        normalized = copy.deepcopy(action)
        raw_action = str(normalized.get("action") or "").lower()
        if raw_action in {"submit", "confirm"}:
            normalized["action"] = "agree"
        normalized.setdefault("hidden", False)
        normalized.setdefault("name", i18n_name(normalized.get("name") or normalized.get("text"), "确认"))
        normalized.setdefault("text", copy.deepcopy(normalized["name"]))
        normalized.setdefault("alias", copy.deepcopy(normalized["name"]))
        return normalized

    raw = str(action or "").lower()
    if raw in {"submit", "confirm", "agree"}:
        return {
            "hidden": False,
            "name": {"en_US": "Confirm", "zh_CN": "确认", "type": "i18n"},
            "text": {"en_US": "Confirm", "zh_CN": "确认", "type": "i18n"},
            "action": "agree",
            "alias": {"en_US": "Confirm", "zh_CN": "确认", "type": "i18n"},
        }
    if raw in {"transfer", "forward"}:
        return {
            "hidden": True,
            "name": {"en_US": "Forward", "zh_CN": "转交", "type": "i18n"},
            "text": {"en_US": "Forward", "zh_CN": "转交", "type": "i18n"},
            "action": "forward",
            "alias": {"en_US": "Forward", "zh_CN": "转交", "type": "i18n"},
        }
    return None


def _originator_approval_props_from_operator(props: dict[str, Any]) -> dict[str, Any]:
    result = copy.deepcopy(props)
    result.pop("actionerRule", None)
    result.pop("finishRule", None)
    result.pop("executor", None)
    result.pop("approver", None)

    result["approvals"] = [["originator"]]
    approver_rules = copy.deepcopy(result.get("approverRules") if isinstance(result.get("approverRules"), dict) else {})
    approver_rules.update({
        "type": "ext_target_approval_originator",
        "mode": "ApprovalNode_rules_only",
        "approverList": [{"type": "ext_target_approval"}],
        "multiApproverType": approver_rules.get("multiApproverType", "all"),
        "conditionalMode": approver_rules.get("conditionalMode", "conditional"),
        "approvals": [["originator"]],
        "description": approver_rules.get("description") or "发起人本人",
    })
    result["approverRules"] = approver_rules

    raw_actions = result.get("actions")
    converted_actions = [
        converted
        for converted in (_operator_action_to_approval_action(item) for item in raw_actions or [])
        if converted is not None
    ]
    if not converted_actions:
        converted_actions = [
            _operator_action_to_approval_action("submit"),
            _operator_action_to_approval_action("transfer"),
        ]
    result["actions"] = [item for item in converted_actions if item is not None]
    result.setdefault("appendActions", [])
    result.setdefault("openDigitalSign", False)
    result.setdefault("noActionersType", "stopProcess")
    return result


def normalize_node(node: dict[str, Any]) -> dict[str, Any]:
    if not isinstance(node, dict):
        raise FlowSchemaError(f"node must be object, got {type(node).__name__}")

    node_type = normalize_node_type(node)
    node_id = node.get("nodeId")
    if not isinstance(node_id, str) or not node_id:
        raise FlowSchemaError(f"{node_type} node missing nodeId")

    props = copy.deepcopy(node.get("props") or {})
    if not isinstance(props, dict):
        raise FlowSchemaError(f"node {node_id} props must be object")

    for key, value in node.items():
        if key == "type" and (
            node.get("componentName")
            or node.get("nodeType")
            or node.get("tool")
            or node.get("toolName")
            or node.get("kind")
        ):
            props[key] = copy.deepcopy(value)
            continue
        if key not in NODE_CORE_KEYS:
            props[key] = copy.deepcopy(value)

    if node_type == "trigger":
        props = _normalize_trigger_props(props)
    node_type, props = _normalize_process_payload(node_type, props)
    approval_type = copy.deepcopy(node.get("approvalType")) if "approvalType" in node else copy.deepcopy(props.get("approvalType"))
    if _is_originator_operator_node(node_type, node, props):
        node_type = "approval"
        approval_type = "ext_target_approval_originator"
        props = _originator_approval_props_from_operator(props)

    child_nodes = node.get("childNodes") or []
    if not isinstance(child_nodes, list):
        raise FlowSchemaError(f"node {node_id} childNodes must be list")

    normalized = {
        "name": i18n_name(node.get("name"), DEFAULT_NODE_NAMES.get(node_type, node_type)),
        "type": node_type,
        "nodeId": node_id,
        "nextId": normalize_next_id(node.get("nextId")),
        "props": props,
        "childNodes": [normalize_node(child) for child in child_nodes],
    }
    if "prevId" in node:
        normalized["prevId"] = node["prevId"]
    if "description" in node:
        normalized["description"] = copy.deepcopy(node.get("description"))
    if "title" in node:
        normalized["title"] = copy.deepcopy(node.get("title"))
    if approval_type is not None:
        normalized["approvalType"] = approval_type
    return normalized


def _walk_nodes(nodes: list[dict[str, Any]]) -> list[dict[str, Any]]:
    result: list[dict[str, Any]] = []
    for node in nodes:
        result.append(node)
        result.extend(_walk_nodes(node.get("childNodes") or []))
    return result


def _node_label(node: dict[str, Any]) -> str:
    return f"{node.get('nodeId') or '<unknown>'}({ _i18n_text(node.get('name'), str(node.get('type') or 'node')) })"


def _non_empty_assignments(value: Any) -> bool:
    return isinstance(value, list) and any(isinstance(item, dict) for item in value)


def _rule_list_has_entries(value: Any) -> bool:
    if not isinstance(value, dict):
        return False
    rules = value.get("rules")
    if isinstance(rules, list):
        return any(isinstance(item, dict) for item in rules)
    if isinstance(rules, dict):
        return _rule_list_has_entries(rules)
    return False


def _validate_assignment_columns(node: dict[str, Any], assignments: Any) -> None:
    if not isinstance(assignments, list):
        return
    for index, assignment in enumerate(assignments, start=1):
        if not isinstance(assignment, dict):
            raise FlowSchemaError(f"{_node_label(node)} assignment #{index} must be object")
        if not assignment.get("column"):
            raise FlowSchemaError(f"{_node_label(node)} assignment #{index} missing column")
        if not assignment.get("valueType"):
            raise FlowSchemaError(f"{_node_label(node)} assignment #{index} missing valueType")


def _validate_designer_runtime_node(node: dict[str, Any]) -> None:
    node_type = str(node.get("type") or "")
    props = node.get("props")
    if not isinstance(props, dict):
        return

    if node_type == "dataCreate":
        if not props.get("formUuid"):
            raise FlowSchemaError(f"{_node_label(node)} AddDataNode missing target formUuid")
        assignments = props.get("assignments")
        has_rules = _rule_list_has_entries(props.get("rules"))
        add_data_rules = props.get("addDataRules") if isinstance(props.get("addDataRules"), dict) else {}
        has_add_rules = _rule_list_has_entries(add_data_rules.get("rules"))
        if not (_non_empty_assignments(assignments) or has_rules or has_add_rules):
            raise FlowSchemaError(f"{_node_label(node)} AddDataNode missing field assignment rules")
        _validate_assignment_columns(node, assignments)

    if node_type == "dataUpdate":
        if not props.get("sourceId"):
            raise FlowSchemaError(f"{_node_label(node)} UpdateDataNode missing sourceId")
        assignments = props.get("assignments")
        update_rules = props.get("updateDataRules") if isinstance(props.get("updateDataRules"), dict) else {}
        has_update_rules = _rule_list_has_entries(update_rules.get("rules"))
        if not (_non_empty_assignments(assignments) or has_update_rules):
            raise FlowSchemaError(f"{_node_label(node)} UpdateDataNode missing field assignment rules")
        _validate_assignment_columns(node, assignments)

    if node_type == "dataRetrieve":
        if not props.get("sourceId"):
            raise FlowSchemaError(f"{_node_label(node)} GetDataNode missing sourceId")

    if node_type == "sendCard":
        if not props.get("cardAppId"):
            raise FlowSchemaError(f"{_node_label(node)} CardNode missing cardAppId")
        if not props.get("cardPageCode"):
            raise FlowSchemaError(f"{_node_label(node)} CardNode missing cardPageCode")
        if props.get("sendType") == "update":
            biz_id = props.get("bizId") if isinstance(props.get("bizId"), dict) else {}
            value = str(biz_id.get("value") or "")
            if not value:
                raise FlowSchemaError(f"{_node_label(node)} CardNode sendType=update missing bizId.value")
            if value in UNRESOLVED_CARD_BIZ_ID_VALUES:
                raise FlowSchemaError(
                    f"{_node_label(node)} CardNode bizId.value={value!r} is not designer-visible; "
                    "choose a real variable from the designer dropdown, such as a current form fieldId"
                )


def validate_flow_schema(flow: dict[str, Any]) -> None:
    if not isinstance(flow, dict):
        raise FlowSchemaError("flow schema must be object")
    props = flow.get("props")
    nodes = flow.get("nodes")
    if not isinstance(props, dict):
        raise FlowSchemaError("flow schema missing props object")
    if not props.get("processCode"):
        raise FlowSchemaError("flow schema missing props.processCode")
    if not isinstance(nodes, list) or not nodes:
        raise FlowSchemaError("flow schema nodes must be non-empty array")

    seen: set[str] = set()
    for node in _walk_nodes(nodes):
        node_id = node.get("nodeId")
        if node_id in seen:
            raise FlowSchemaError(f"duplicate nodeId: {node_id}")
        seen.add(node_id)
        _validate_designer_runtime_node(node)


def build_automation_flow(
    process_code: str,
    nodes: list[dict[str, Any]],
    *,
    allow_withdraw: bool = True,
    allow_collaboration: bool = True,
    allow_temporary_storage: bool = True,
) -> dict[str, Any]:
    if not process_code:
        raise FlowSchemaError("processCode is required")
    if not isinstance(nodes, list) or not nodes:
        raise FlowSchemaError("nodes must be non-empty array")

    flow = {
        "props": {
            "processCode": process_code,
            "allowWithdraw": allow_withdraw,
            "allowCollaboration": allow_collaboration,
            "allowTemporaryStorage": allow_temporary_storage,
        },
        "nodes": [normalize_node(node) for node in nodes],
    }
    validate_flow_schema(flow)
    return flow


def build_flow_schema(spec: Any, *, process_code: str | None = None) -> dict[str, Any]:
    if isinstance(spec, list):
        if not process_code:
            raise FlowSchemaError("processCode is required when spec is a node array")
        return build_automation_flow(process_code, spec)

    if not isinstance(spec, dict):
        raise FlowSchemaError("flow spec must be object or node array")

    if "props" in spec and "nodes" in spec:
        props = copy.deepcopy(spec["props"])
        if not isinstance(props, dict):
            raise FlowSchemaError("props must be object")
        if process_code:
            props["processCode"] = process_code
        if "bindingForm" in spec and not props.get("bindingForm"):
            props["bindingForm"] = copy.deepcopy(spec["bindingForm"])
        nodes = spec["nodes"]
        if not isinstance(nodes, list):
            raise FlowSchemaError("nodes must be array")
        flow = {"props": props, "nodes": [normalize_node(node) for node in nodes]}
        for key in FLOW_PROP_KEYS:
            flow["props"].setdefault(key, True)
        for key in FLOW_TOP_LEVEL_KEYS:
            if key in spec:
                flow[key] = copy.deepcopy(spec[key])
        validate_flow_schema(flow)
        return flow

    nodes = spec.get("nodes")
    if not isinstance(nodes, list):
        raise FlowSchemaError("flow spec missing nodes array")

    code = process_code or spec.get("processCode")
    flow = build_automation_flow(
        str(code or ""),
        nodes,
        allow_withdraw=bool(spec.get("allowWithdraw", True)),
        allow_collaboration=bool(spec.get("allowCollaboration", True)),
        allow_temporary_storage=bool(spec.get("allowTemporaryStorage", True)),
    )
    if "bindingForm" in spec:
        flow["props"]["bindingForm"] = copy.deepcopy(spec["bindingForm"])
    return flow


def _i18n_text(value: Any, default: str = "") -> str:
    if isinstance(value, dict):
        return str(value.get("zh_CN") or value.get("en_US") or value.get("value") or default)
    return str(value or default)


def _default_component_props() -> dict[str, Any]:
    return {
        "defaultDataSource": {},
        "relateAppType": "",
        "relateOrderEnable": False,
        "relateOrderConfig": [],
    }


def _rule_for_assignment(assignment: dict[str, Any], fields_by_id: dict[str, dict[str, Any]]) -> dict[str, Any]:
    column = str(assignment.get("column") or "")
    field = copy.deepcopy(fields_by_id.get(column) or {})
    rule = {
        "ruleId": f"rule-{column}" if column else "rule-unknown",
        "name": column,
        "label": field.get("label") or column,
        "componentName": field.get("componentName"),
        "componentOption": field.get("componentOption", "[]"),
        "componentProps": copy.deepcopy(field.get("componentProps") or _default_component_props()),
        "required": bool(field.get("required", False)),
        "valueType": assignment.get("valueType", "literal"),
        "value": assignment.get("value"),
    }
    for key in ("__display", "__source"):
        if key in assignment:
            rule[key] = copy.deepcopy(assignment[key])
    return rule


def _component_option_map(child_list: list[dict[str, Any]]) -> dict[str, Any]:
    return {
        str(field.get("fieldId") or field.get("name") or ""): copy.deepcopy(field.get("componentOption", "[]"))
        for field in child_list
        if field.get("fieldId") or field.get("name")
    }


def _rules_for_assignments(
    assignments: list[dict[str, Any]],
    child_list: list[dict[str, Any]],
    existing_rules: Any = None,
) -> dict[str, Any]:
    rules = copy.deepcopy(existing_rules or {})
    component_option_map = _component_option_map(child_list)
    if assignments and not rules.get("rules"):
        fields_by_id = {str(field.get("fieldId") or field.get("name")): field for field in child_list}
        rules = {
            "ruleId": "rule-root",
            "childList": copy.deepcopy(child_list),
            "componentOptionMap": copy.deepcopy(component_option_map),
            "rules": [_rule_for_assignment(item, fields_by_id) for item in assignments],
        }
    else:
        rules.setdefault("ruleId", "rule-root")
        rules.setdefault("childList", copy.deepcopy(child_list))
        rules.setdefault("componentOptionMap", copy.deepcopy(component_option_map))
        rules.setdefault("rules", [])
    return rules


def _approval_option_lookup(props: dict[str, Any]) -> dict[str, dict[str, Any]]:
    result: dict[str, dict[str, Any]] = {}
    for key in ("approvalUsers", "approvalOptions", "approverOptions"):
        options = props.get(key)
        if not isinstance(options, list):
            continue
        for item in options:
            if not isinstance(item, dict):
                continue
            user_id = item.get("id") or item.get("userId") or item.get("value")
            if isinstance(user_id, str) and user_id:
                normalized = copy.deepcopy(item)
                normalized.setdefault("id", user_id)
                normalized.setdefault("label", item.get("name") or item.get("userName") or user_id)
                normalized.setdefault("type", "employee")
                normalized.setdefault("roleType", "DINGTALK")
                result[user_id] = normalized
    return result


def _approval_value_for_view(value: Any, options_by_id: dict[str, dict[str, Any]]) -> Any:
    if isinstance(value, dict):
        item = copy.deepcopy(value)
        user_id = item.get("id") or item.get("userId") or item.get("value")
        if isinstance(user_id, str) and user_id:
            item.setdefault("id", user_id)
        item.setdefault("label", item.get("name") or item.get("userName") or item.get("id") or "")
        item.setdefault("type", "employee")
        item.setdefault("roleType", "DINGTALK")
        return item
    if isinstance(value, str):
        if value in options_by_id:
            return copy.deepcopy(options_by_id[value])
        return {
            "id": value,
            "label": value,
            "type": "employee",
            "roleType": "DINGTALK",
        }
    return value


def _approver_rules_for_view(node: dict[str, Any], props: dict[str, Any]) -> dict[str, Any]:
    raw_rules = props.get("approverRules") if isinstance(props.get("approverRules"), dict) else {}
    result = copy.deepcopy(raw_rules)
    result.setdefault("type", node.get("approvalType") or props.get("approvalType") or "ext_target_approval")
    result.setdefault("mode", "ApprovalNode_rules_only")
    if result.get("type") == "ext_target_approval_originator":
        approver_list = result.get("approverList")
        if isinstance(approver_list, list) and approver_list:
            normalized_list = []
            for item in approver_list:
                if isinstance(item, dict):
                    normalized = copy.deepcopy(item)
                    if normalized.get("type") == "ext_target_approval_originator":
                        normalized["type"] = "ext_target_approval"
                    normalized_list.append(normalized)
                else:
                    normalized_list.append(item)
            result["approverList"] = normalized_list
        else:
            result["approverList"] = [{"type": "ext_target_approval"}]
        result.setdefault("description", "发起人本人")
    else:
        result.setdefault("approverList", [{"type": result["type"]}])
    result.setdefault("multiApproverType", "all")
    result.setdefault("conditionalMode", props.get("conditionalMode", "conditional"))

    approvals = result.get("approvals", props.get("approvals", []))
    if isinstance(approvals, list):
        options_by_id = _approval_option_lookup(props)
        result["approvals"] = [_approval_value_for_view(item, options_by_id) for item in approvals]
        labels = [
            str(item.get("label") or item.get("id") or "")
            for item in result["approvals"]
            if isinstance(item, dict)
        ]
        if labels and not result.get("description"):
            result["description"] = "、".join(label for label in labels if label)
    return result


def _operator_actions_for_view(props: dict[str, Any]) -> dict[str, Any]:
    normal_actions = []
    for action in props.get("actions") or []:
        if isinstance(action, dict):
            normalized = copy.deepcopy(action)
            normalized.setdefault("hidden", False)
            normalized.setdefault("name", i18n_name(normalized.get("name") or normalized.get("text"), "提交"))
            normalized.setdefault("text", copy.deepcopy(normalized["name"]))
            normalized.setdefault("alias", copy.deepcopy(normalized["name"]))
            normal_actions.append(normalized)
            continue

        raw = str(action or "").lower()
        if raw in {"submit", "agree", "confirm"}:
            normal_actions.append({
                "hidden": False,
                "name": {"en_US": "Submit", "zh_CN": "提交", "type": "i18n"},
                "text": {"en_US": "Submit", "zh_CN": "提交", "type": "i18n"},
                "action": "submit",
                "alias": {"en_US": "Submit", "zh_CN": "提交", "type": "i18n"},
            })
        elif raw in {"transfer", "forward"}:
            normal_actions.append({
                "hidden": True,
                "name": {"en_US": "Forward", "zh_CN": "转交", "type": "i18n"},
                "text": {"en_US": "Forward", "zh_CN": "转交", "type": "i18n"},
                "action": "forward",
                "alias": {"en_US": "Forward", "zh_CN": "转交", "type": "i18n"},
            })

    if not normal_actions:
        normal_actions.append({
            "hidden": False,
            "name": {"en_US": "Submit", "zh_CN": "提交", "type": "i18n"},
            "text": {"en_US": "Submit", "zh_CN": "提交", "type": "i18n"},
            "action": "submit",
            "alias": {"en_US": "Submit", "zh_CN": "提交", "type": "i18n"},
        })

    return {
        "normalActions": normal_actions,
        "appendActions": copy.deepcopy(props.get("appendActions") or []),
    }


def _operator_rules_for_view(node: dict[str, Any], props: dict[str, Any]) -> dict[str, Any]:
    actioner_rule = props.get("actionerRule") if isinstance(props.get("actionerRule"), dict) else {}
    if actioner_rule and not props.get("approvals") and not props.get("approverRules"):
        rule_type = str(actioner_rule.get("type") or "")
        value = str(actioner_rule.get("value") or "")
        if rule_type == "static" and value:
            props = copy.deepcopy(props)
            props["approvals"] = [item.strip() for item in value.split(",") if item.strip()]
            props.setdefault("approvalType", "ext_target_approval")
    return _approver_rules_for_view(node, props)


def _message_recipient_description(props: dict[str, Any]) -> str:
    parts: list[str] = []

    for item in props.get("toUsers") or []:
        if not isinstance(item, dict):
            continue
        label = item.get("userName") or item.get("name") or item.get("label") or item.get("userId")
        if label:
            parts.append(str(label))

    for item in props.get("toRoles") or []:
        if not isinstance(item, dict):
            continue
        label = item.get("roleName") or item.get("name") or item.get("label") or item.get("roleCode")
        if label:
            parts.append(str(label))

    field_labels = {
        "PROCESS_PARTICIPANTS": "流程参与人",
        "PROCESS_APPROVERS": "流程审批人",
        "PROCESS_CARBONS": "流程抄送人",
        "FORM_INST_MODIFIER": "表单修改人",
    }
    for item in props.get("userFields") or []:
        label = field_labels.get(str(item), str(item))
        if label:
            parts.append(label)

    for key, label in (
        ("toVars", "变量接收人"),
        ("toGroups", "群组"),
        ("toGroupVars", "变量群组"),
        ("robotUrls", "机器人"),
    ):
        values = props.get(key) or []
        if values:
            parts.append(label)

    seen: set[str] = set()
    result: list[str] = []
    for part in parts:
        if part not in seen:
            seen.add(part)
            result.append(part)
    return "、".join(result)


def _send_message_rules_for_view(node: dict[str, Any], props: dict[str, Any]) -> tuple[dict[str, Any], str]:
    result = copy.deepcopy(props)
    description = result.get("description")
    if isinstance(description, dict):
        description = _i18n_text(description)
    if not description and node.get("description") not in (None, ""):
        description = _i18n_text(node.get("description"))
    if not description:
        description = _message_recipient_description(result)
    if description:
        result["description"] = description
    return result, str(description or "")


def _condition_description(condition_group: Any) -> str:
    if not isinstance(condition_group, dict):
        return ""
    rules = condition_group.get("rules")
    if not isinstance(rules, list):
        return ""
    parts = []
    for rule in rules:
        if not isinstance(rule, dict):
            continue
        name = str(rule.get("name") or rule.get("id") or "")
        op = str(rule.get("op") or rule.get("opCode") or "")
        value = rule.get("ruleValue", rule.get("value", ""))
        if isinstance(value, list):
            value_text = "、".join(str(item.get("label") or item.get("name") or item.get("id") or item) if isinstance(item, dict) else str(item) for item in value)
        else:
            value_text = str(value)
        if name and op:
            parts.append(f"{name}{op}{value_text}")
    return "，".join(parts)


def _condition_rules_for_view(props: dict[str, Any]) -> dict[str, Any]:
    raw_conditions = props.get("conditions") if isinstance(props.get("conditions"), dict) else {}
    result = copy.deepcopy(raw_conditions)
    result.setdefault("condition", "AND")
    result.setdefault("conditionCode", "&&")
    result.setdefault("rules", [])
    result["calculate"] = props.get("calculate") or raw_conditions.get("calculate") or "condition"
    result["conditions"] = copy.deepcopy(raw_conditions)
    result["isDefault"] = bool(props.get("isDefault", raw_conditions.get("isDefault", False)))
    if "priority" in props:
        result["priority"] = props["priority"]
    elif "priority" in raw_conditions:
        result["priority"] = raw_conditions["priority"]
    description = props.get("description") or raw_conditions.get("description") or _condition_description(raw_conditions)
    if not description and result.get("calculate") == "all" and not result.get("rules"):
        description = "全部数据"
    if description:
        result["description"] = description
    return result


def _build_add_data_rules(
    node_props: dict[str, Any],
    *,
    fields: list[dict[str, Any]] | None = None,
    form_title: str = "",
) -> dict[str, Any]:
    raw_add_rules = node_props.get("addDataRules") if isinstance(node_props.get("addDataRules"), dict) else {}
    child_list = copy.deepcopy(fields or raw_add_rules.get("inputs", {}).get("childList") or [])
    component_option_map = _component_option_map(child_list)

    assignments = copy.deepcopy(node_props.get("assignments") or raw_add_rules.get("assignments") or [])
    rules = _rules_for_assignments(assignments, child_list, raw_add_rules.get("rules") or node_props.get("rules"))

    inputs = copy.deepcopy(raw_add_rules.get("inputs") or node_props.get("inputs") or {})
    inputs.setdefault("childList", copy.deepcopy(child_list))
    inputs.setdefault("componentOptionMap", component_option_map)

    description = raw_add_rules.get("description")
    if isinstance(description, dict):
        description = _i18n_text(description)
    if not description and form_title:
        description = f"在 [{form_title}] 中新增数据"
    if not description:
        description = node_props.get("description")
        if isinstance(description, dict):
            description = _i18n_text(description)

    return {
        "formUuid": node_props.get("formUuid", raw_add_rules.get("formUuid", "")),
        "appType": node_props.get("appType", raw_add_rules.get("appType", "")),
        "subFormUuid": node_props.get("subFormUuid", raw_add_rules.get("subFormUuid", "")),
        "insertType": (
            node_props.get("insertType")
            or raw_add_rules.get("insertType")
            or node_props.get("originalType")
            or raw_add_rules.get("originalType")
            or "form"
        ),
        "originalType": node_props.get("originalType", raw_add_rules.get("originalType", "form")),
        "type": node_props.get("type", raw_add_rules.get("type", "single")),
        "sourceId": node_props.get("sourceId", raw_add_rules.get("sourceId", "")),
        "assignments": copy.deepcopy(raw_add_rules.get("assignments") or []),
        "inputs": inputs,
        "rules": rules,
        "description": description or None,
    }


def _normalize_none_operation(value: Any) -> str:
    raw = str(value or "").strip()
    lowered = raw.lower()
    if lowered in {"add", "新增一条数据"} or raw == "ADD":
        return "add"
    if lowered in {"ignored", "ignore", "skip", "跳过当前节点"} or raw == "IGNORED":
        return "ignored"
    return lowered or "ignored"


def _condition_rule_text(rule: Any) -> str:
    if not isinstance(rule, dict):
        return ""
    name = str(rule.get("name") or rule.get("label") or rule.get("id") or rule.get("column") or "")
    op = str(rule.get("op") or rule.get("opCode") or "")
    value = rule.get("ruleValue") if rule.get("ruleValue") not in (None, "") else rule.get("value")
    value_text = str(value or "")
    if name and op and value_text:
        return f"{name}{op}{value_text}"
    if name and op:
        return f"{name}{op}"
    return name or value_text


def _condition_description_for_get_data(condition: Any) -> str:
    if not isinstance(condition, dict):
        return ""
    rules = condition.get("rules")
    if not isinstance(rules, list):
        return ""
    parts = [_condition_rule_text(rule) for rule in rules]
    return "，".join(part for part in parts if part)


def _build_get_data_rules(
    node_props: dict[str, Any],
    *,
    form_title: str = "",
) -> tuple[dict[str, Any], str]:
    raw_get_data = node_props.get("getData") if isinstance(node_props.get("getData"), dict) else {}
    get_data = copy.deepcopy(raw_get_data or node_props)
    get_data.setdefault("type", node_props.get("type") or "single")
    get_data.setdefault("sourceId", node_props.get("sourceId") or "")
    get_data.setdefault("appType", node_props.get("appType") or "")
    get_data.setdefault("originalType", node_props.get("originalType") or "form")
    get_data.setdefault("subSourceId", node_props.get("subSourceId") or "")
    get_data.setdefault("filterType", node_props.get("filterType") or "condition")
    get_data.setdefault("quantity", str(node_props.get("quantity") or "1"))
    get_data.setdefault("assignments", copy.deepcopy(node_props.get("assignments") or []))
    get_data.setdefault("sort", copy.deepcopy(node_props.get("sort") or {"type": "none", "column": ""}))

    description = get_data.get("description") or node_props.get("description")
    if isinstance(description, dict):
        description = _i18n_text(description)
    if not description:
        condition_text = _condition_description_for_get_data(get_data.get("condition"))
        if condition_text:
            description = condition_text
        elif form_title:
            quantity = get_data.get("quantity") or "1"
            description = f"在 [{form_title}] 中查询 {quantity} 条数据"

    if description:
        get_data["description"] = description
        condition = get_data.get("condition")
        if isinstance(condition, dict):
            condition.setdefault("description", description)
            condition.setdefault("conditions", copy.deepcopy(condition))
    return get_data, str(description or "")


def _find_data_source_form(
    node_props: dict[str, Any],
    *,
    nodes_by_id: dict[str, dict[str, Any]] | None = None,
) -> tuple[str, str]:
    app_type = str(node_props.get("appType") or "")
    direct_form = node_props.get("formUuid") or node_props.get("sourceFormUuid")
    if isinstance(direct_form, str) and direct_form:
        return app_type, direct_form

    source_id = str(node_props.get("sourceId") or "")
    if source_id.startswith("FORM-"):
        return app_type, source_id

    source_node = (nodes_by_id or {}).get(source_id)
    if isinstance(source_node, dict):
        props = source_node.get("props") if isinstance(source_node.get("props"), dict) else {}
        source_app = str(props.get("appType") or app_type)
        source_form = props.get("formUuid") or props.get("sourceId")
        if isinstance(source_form, str) and source_form.startswith("FORM-"):
            return source_app, source_form
    return app_type, ""


def _build_update_data_rules(
    node_props: dict[str, Any],
    *,
    fields: list[dict[str, Any]] | None = None,
    form_title: str = "",
) -> tuple[dict[str, Any], str]:
    raw_update_rules = node_props.get("updateDataRules") if isinstance(node_props.get("updateDataRules"), dict) else {}
    rules = copy.deepcopy(raw_update_rules or node_props)
    child_list = copy.deepcopy(fields or raw_update_rules.get("inputs", {}).get("childList") or [])
    component_option_map = _component_option_map(child_list)
    assignments = copy.deepcopy(node_props.get("assignments") or raw_update_rules.get("assignments") or [])

    rules.setdefault("type", node_props.get("type") or "node")
    rules.setdefault("sourceId", node_props.get("sourceId") or "")
    rules.setdefault("subSourceId", node_props.get("subSourceId") or "")
    rules.setdefault("condition", copy.deepcopy(node_props.get("condition") or {}))
    rules.setdefault("subCondition", copy.deepcopy(node_props.get("subCondition") or {}))
    rules["assignments"] = assignments
    display_rules = _rules_for_assignments(assignments, child_list, raw_update_rules.get("rules") or node_props.get("rules"))
    rules["rules"] = display_rules
    rules["dataRules"] = copy.deepcopy(
        raw_update_rules.get("dataRules")
        or node_props.get("dataRules")
        or display_rules
    )
    inputs = copy.deepcopy(raw_update_rules.get("inputs") or node_props.get("inputs") or {})
    inputs.setdefault("childList", copy.deepcopy(child_list))
    inputs.setdefault("componentOptionMap", component_option_map)
    rules["inputs"] = inputs
    rules.setdefault("rulesFilter", copy.deepcopy(node_props.get("rulesFilter") or []))
    rules.setdefault("tableRulesFilter", copy.deepcopy(node_props.get("tableRulesFilter") or []))
    rules["noneOperation"] = _normalize_none_operation(
        raw_update_rules.get("noneOperation") or node_props.get("noneOperation")
    )

    description = rules.get("description") or node_props.get("description")
    if isinstance(description, dict):
        description = _i18n_text(description)
    if not description and form_title:
        description = f"更新 [{form_title}] 中的数据"
    if description:
        rules["description"] = description
    return rules, str(description or "")


def _view_component_name_for_node(node: dict[str, Any]) -> str:
    node_type = str(node.get("type") or "")
    props = node.get("props") if isinstance(node.get("props"), dict) else {}
    if node_type == "dataRetrieve":
        get_data = props.get("getData") if isinstance(props.get("getData"), dict) else props
        retrieve_type = str(get_data.get("type") or "").lower()
        return "GetBatchDataNode" if retrieve_type in {"batch", "multiple", "multi"} else "GetSingleDataNode"
    if node_type == "CodeExecutor":
        if "groovy" in props or str(props.get("scriptType") or "").lower() == "groovy":
            return "GroovyNode"
        return "JavaScriptNode"
    return VIEW_COMPONENT_BY_TYPE.get(node_type, str(node.get("componentName") or node_type))


def _view_props_for_node(
    node: dict[str, Any],
    *,
    data_create_metadata: dict[tuple[str, str], dict[str, Any]] | None = None,
    nodes_by_id: dict[str, dict[str, Any]] | None = None,
) -> dict[str, Any]:
    node_type = str(node.get("type") or "")
    component_name = _view_component_name_for_node(node)
    props = copy.deepcopy(node.get("props") or {})
    name = copy.deepcopy(node.get("name") or i18n_name(None, DEFAULT_NODE_NAMES.get(node_type, node_type)))
    if node.get("description") not in (None, ""):
        props.setdefault("description", copy.deepcopy(node["description"]))

    view_props: dict[str, Any] = {
        "nodeName": component_name,
        "name": name,
    }
    if node.get("description") not in (None, ""):
        view_props["description"] = copy.deepcopy(node["description"])

    if component_name == "AddDataNode":
        target_key = (str(props.get("appType") or ""), str(props.get("formUuid") or ""))
        metadata = (data_create_metadata or {}).get(target_key) or {}
        add_data_rules = _build_add_data_rules(
            props,
            fields=metadata.get("fields") or [],
            form_title=str(metadata.get("title") or ""),
        )
        view_props["addDataRules"] = add_data_rules
    elif component_name in {"GetSingleDataNode", "GetBatchDataNode"}:
        target_app, target_form = _find_data_source_form(props, nodes_by_id=nodes_by_id)
        metadata = (data_create_metadata or {}).get((target_app, target_form)) or {}
        get_data, description = _build_get_data_rules(
            props,
            form_title=str(metadata.get("title") or ""),
        )
        view_props["getData"] = get_data
        if description:
            view_props.setdefault("description", description)
    elif component_name == "UpdateDataNode":
        target_app, target_form = _find_data_source_form(props, nodes_by_id=nodes_by_id)
        metadata = (data_create_metadata or {}).get((target_app, target_form)) or {}
        update_data_rules, description = _build_update_data_rules(
            props,
            fields=metadata.get("fields") or [],
            form_title=str(metadata.get("title") or ""),
        )
        view_props["updateDataRules"] = update_data_rules
        if description:
            view_props.setdefault("description", description)
    elif component_name == "ApprovalNode":
        view_props["approverRules"] = _approver_rules_for_view(node, props)
        if props.get("actions") or props.get("appendActions"):
            view_props["actions"] = {
                "normalActions": copy.deepcopy(props.get("actions") or []),
                "appendActions": copy.deepcopy(props.get("appendActions") or []),
            }
    elif component_name == "OperatorNode":
        view_props["approverRules"] = _operator_rules_for_view(node, props)
        view_props["actions"] = _operator_actions_for_view(props)
    elif component_name == "SendMessageNode":
        send_message_rules, description = _send_message_rules_for_view(node, props)
        view_props["sendMessageRules"] = send_message_rules
        if description:
            view_props.setdefault("description", description)
    elif node_type == "condition":
        for key, value in props.items():
            view_props.setdefault(key, copy.deepcopy(value))
        view_props["conditions"] = _condition_rules_for_view(props)
    elif node_type == "route":
        for key, value in props.items():
            view_props.setdefault(key, copy.deepcopy(value))
    else:
        rule_key = VIEW_COMPONENT_RULE_KEY.get(component_name)
        if rule_key:
            view_props[rule_key] = copy.deepcopy(props.get(rule_key) or props)

    view_props["title"] = copy.deepcopy(node.get("title") or name)
    return view_props


def _view_child_for_node(
    node: dict[str, Any],
    *,
    data_create_metadata: dict[tuple[str, str], dict[str, Any]] | None = None,
    nodes_by_id: dict[str, dict[str, Any]] | None = None,
) -> dict[str, Any]:
    child = {
        "componentName": _view_component_name_for_node(node),
        "id": node.get("nodeId"),
        "props": _view_props_for_node(node, data_create_metadata=data_create_metadata, nodes_by_id=nodes_by_id),
    }
    nested = node.get("childNodes") or []
    if isinstance(nested, list) and nested:
        child["children"] = [
            _view_child_for_node(item, data_create_metadata=data_create_metadata, nodes_by_id=nodes_by_id)
            for item in nested
        ]
    return child


def build_process_view_schema(
    flow: dict[str, Any],
    *,
    data_create_metadata: dict[tuple[str, str], dict[str, Any]] | None = None,
) -> dict[str, Any]:
    """Build the designer viewJson that matches a normalized process schema.

    The backend processJson can execute dataCreate nodes with plain
    ``props.assignments``. The Yida designer, however, renders and re-saves
    AddDataNode from ``props.addDataRules.rules.rules`` plus target field
    metadata. This helper keeps both surfaces aligned.
    """
    validate_flow_schema(flow)
    nodes_by_id = {str(node.get("nodeId")): node for node in _walk_nodes(flow.get("nodes") or [])}
    children = []
    for node in flow.get("nodes") or []:
        children.append(_view_child_for_node(node, data_create_metadata=data_create_metadata, nodes_by_id=nodes_by_id))

    return {
        "schema": {
            "componentName": "CanvasEngine",
            "id": "canvas",
            "props": {},
            "children": children,
        },
        "bindingForm": flow.get("props", {}).get("bindingForm"),
        "formulaRules": flow.get("formulaRules", []),
        "globalSetting": {
            "enableSignature": False,
            "stopAssociationRulesIfFailed": flow.get("props", {}).get("stopAssociationRulesIfFailed", False),
            "nodeMerge": False,
            "originatorMerge": False,
            "allNodeMerge": False,
            "behaviorList": [],
            "needOpenDigitalSignNodes": [],
            "approvalSummary": flow.get("approvalSummary", []),
            "noRecordRecall": flow.get("props", {}).get("noRecordRecall", False),
            "untimedRule": flow.get("props", {}).get("untimedRule", []),
        },
    }


def _sync_view_top_level_defaults(view: dict[str, Any], flow: dict[str, Any]) -> None:
    props = flow.get("props") if isinstance(flow.get("props"), dict) else {}
    binding_form = props.get("bindingForm")
    if binding_form:
        view["bindingForm"] = binding_form
    view.setdefault("formulaRules", copy.deepcopy(flow.get("formulaRules") or []))
    view.setdefault("flowConfig", copy.deepcopy(flow.get("flowConfig") or {}))

    global_setting = view.setdefault("globalSetting", {})
    if not isinstance(global_setting, dict):
        global_setting = {}
        view["globalSetting"] = global_setting
    global_setting.setdefault("enableSignature", False)
    global_setting.setdefault("stopAssociationRulesIfFailed", props.get("stopAssociationRulesIfFailed", False))
    global_setting.setdefault("nodeMerge", False)
    global_setting.setdefault("originatorMerge", False)
    global_setting.setdefault("allNodeMerge", False)
    global_setting.setdefault("behaviorList", [])
    global_setting.setdefault("needOpenDigitalSignNodes", [])
    global_setting.setdefault("approvalSummary", flow.get("approvalSummary", []))
    global_setting.setdefault("noRecordRecall", props.get("noRecordRecall", False))
    global_setting.setdefault("untimedRule", props.get("untimedRule", []))


def _iter_view_children(children: list[dict[str, Any]]):
    for child in children:
        yield child
        nested = child.get("children") or []
        if isinstance(nested, list):
            yield from _iter_view_children(nested)


def merge_process_view_schema(
    base_view: dict[str, Any],
    flow: dict[str, Any],
    *,
    data_create_metadata: dict[tuple[str, str], dict[str, Any]] | None = None,
) -> dict[str, Any]:
    """Patch a designer viewJson with normalized flow-node configuration.

    Prefer this over building a view from scratch when a source process version
    already has a valid designer skeleton. It preserves designer-specific node
    ids and props, while refreshing advanced component settings such as
    AddDataNode's UI-visible addDataRules.
    """
    validate_flow_schema(flow)
    view = copy.deepcopy(base_view)
    schema = view.setdefault("schema", {})
    children = schema.setdefault("children", [])
    nodes_by_id = {str(node.get("nodeId")): node for node in _walk_nodes(flow.get("nodes") or [])}
    seen: set[str] = set()

    def sync_children(items: list[dict[str, Any]]) -> list[dict[str, Any]]:
        synced: list[dict[str, Any]] = []
        for child in items:
            node_id = str(child.get("id") or "")
            node = nodes_by_id.get(node_id)
            if node:
                fresh_child = _view_child_for_node(
                    node,
                    data_create_metadata=data_create_metadata,
                    nodes_by_id=nodes_by_id,
                )
                component_name = fresh_child["componentName"]
                if (
                    child.get("componentName") == component_name
                    and str(node.get("type") or "") in {"apply", "finish"}
                ):
                    seen.add(node_id)
                    synced.append(child)
                    continue
                child["componentName"] = component_name
                child["props"] = fresh_child["props"]
                if "children" in fresh_child:
                    child["children"] = fresh_child["children"]
                else:
                    child.pop("children", None)
                seen.add(node_id)
                synced.append(child)
                continue
            nested = child.get("children")
            if isinstance(nested, list):
                child["children"] = sync_children(nested)
            synced.append(child)
        return synced

    schema["children"] = sync_children(children)
    children = schema.setdefault("children", [])

    for child in _iter_view_children(children):
        node_id = str(child.get("id") or "")
        node = nodes_by_id.get(node_id)
        if not node:
            continue
        seen.add(node_id)

    existing_top_level = {str(child.get("id") or ""): child for child in children if child.get("id")}
    ordered_children: list[dict[str, Any]] = []
    for child in children:
        node_id = str(child.get("id") or "")
        if child.get("componentName") == "ApplyNode" and node_id not in nodes_by_id:
            ordered_children.append(child)

    for node in flow.get("nodes") or []:
        node_id = str(node.get("nodeId") or "")
        if str(node.get("type") or "") == "apply" and not existing_top_level.get(node_id):
            continue
        child = existing_top_level.get(node_id)
        if not child:
            child = _view_child_for_node(
                node,
                data_create_metadata=data_create_metadata,
                nodes_by_id=nodes_by_id,
            )
        ordered_children.append(child)

    if ordered_children:
        schema["children"] = ordered_children
    _sync_view_top_level_defaults(view, flow)
    return view
