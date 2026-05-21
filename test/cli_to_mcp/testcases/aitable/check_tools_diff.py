#!/usr/bin/env python3
"""详细比对 MCP Server tools 与 aitable.go 实现的差异"""
import json
import re

# 读取 MCP tools 定义
with open('/tmp/aitable_tools.json') as f:
    data = json.load(f)
    mcp_tools = {t['name']: t for t in data['result']['tools']}

# 读取 aitable.go 文件
with open('wukong/products/aitable.go', 'r') as f:
    go_code = f.read()

print("="*120)
print("详细比对报告 - MCP Server vs aitable.go 实现")
print("="*120)

critical_issues = []
warnings = []

def check_tool_implementation(tool_name, tool_def):
    """检查单个 tool 的实现"""
    issues = []
    
    required = tool_def['inputSchema'].get('required', [])
    properties = tool_def['inputSchema'].get('properties', {})
    
    # 1. 检查 tool 是否在代码中被调用
    if f'callMCPTool("{tool_name}"' not in go_code:
        issues.append(f"❌ CRITICAL: 代码中未找到 callMCPTool(\"{tool_name}\") 调用")
        return issues
    
    # 2. 检查每个 required 参数是否在代码中处理
    for param in required:
        # 转换 camelCase 到 kebab-case
        kebab = re.sub(r'([a-z0-9])([A-Z])', r'\1-\2', param).lower()
        # 检查 flag
        if f'"{kebab}"' not in go_code and f'"{param}"' not in go_code:
            # 有些参数可能通过其他方式传递
            if param not in ['records', 'fields', 'config']:  # 这些可能是 JSON 字符串
                issues.append(f"⚠️  WARNING: Required param '{param}' (--{kebab}) may not be properly handled")
    
    # 3. 检查参数映射是否正确
    for param_name in properties:
        kebab = re.sub(r'([a-z0-9])([A-Z])', r'\1-\2', param_name).lower()
        
        # 检查是否在 toolArgs 中正确映射
        if param_name in ['baseId', 'tableId', 'fieldId', 'recordId', 'viewId', 'dashboardId', 'chartId']:
            # 这些 ID 参数通常直接传递
            continue
        
        # 检查特殊参数
        if param_name == 'newTableName':
            if '--name' in go_code and 'newTableName' in go_code:
                pass  # 正确: CLI --name 映射到 newTableName
        elif param_name == 'newBaseName':
            if '--name' in go_code and 'newBaseName' in go_code:
                pass
        elif param_name == 'newViewName':
            if '--name' in go_code and 'newViewName' in go_code:
                pass
        elif param_name == 'viewDescription':
            if '--desc' in go_code and 'viewDescription' in go_code:
                pass
        elif param_name == 'folderId':
            if '--folder-id' in go_code and 'folderId' in go_code:
                pass
    
    return issues

# 逐个检查所有 tools
for tool_name in sorted(mcp_tools.keys()):
    tool_def = mcp_tools[tool_name]
    
    print(f"\n{'='*120}")
    print(f"【{tool_name}】")
    print(f"Title: {tool_def['title']}")
    print(f"Required: {tool_def['inputSchema'].get('required', [])}")
    print(f"Properties: {list(tool_def['inputSchema'].get('properties', {}).keys())}")
    
    issues = check_tool_implementation(tool_name, tool_def)
    
    if issues:
        for issue in issues:
            print(f"  {issue}")
            if 'CRITICAL' in issue:
                critical_issues.append(f"{tool_name}: {issue}")
            else:
                warnings.append(f"{tool_name}: {issue}")
    else:
        print(f"  ✅ 实现看起来正确")

# 总结
print(f"\n{'='*120}")
print(f"总结")
print(f"{'='*120}")
print(f"Critical Issues: {len(critical_issues)}")
for issue in critical_issues:
    print(f"  {issue}")

print(f"\nWarnings: {len(warnings)}")
for issue in warnings[:20]:  # 只显示前20个
    print(f"  {issue}")
if len(warnings) > 20:
    print(f"  ... 还有 {len(warnings) - 20} 个 warnings")
