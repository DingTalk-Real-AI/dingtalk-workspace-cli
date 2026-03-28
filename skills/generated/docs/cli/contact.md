# Canonical Product: contact

Generated from shared Tool IR. Do not edit by hand.

- Display name: 钉钉通讯录
- Description: 钉钉通讯录MCP支持搜索人员/部门、查询成员详情及部门结构，快速获取组织架构信息。
- Server key: `e1e13a2fc7ab1f1b`
- Endpoint: `https://mcp-gw.dingtalk.com/server/db4b26cb38ea6a8739ad55d1997fa1da608cd36b33a6cf0f77884f70c49382fe`
- Protocol: `2025-03-26`
- Degraded: `false`

## Tools

- `user get-self`
  - Path: `contact.get_current_user_profile`
  - CLI route: `dws contact user get-self`
  - Description: 获取当前登录用户的基本信息（如姓名、工号、手机号）、当前组织信息（corpId、组织名称）、直属主管信息、所属部门列表（含部门 ID 与名称）以及角色信息（如管理员类型、自定义角色标签等）。返回内容受组织隐私与权限策略控制：若某些字段（如主管、手机号）被设为不可见，则可能被过滤或省略。
  - Flags: none
  - Schema: `skills/generated/docs/schema/contact/get_current_user_profile.json`
- `dept list-members`
  - Path: `contact.get_dept_members_by_deptId`
  - CLI route: `dws contact dept list-members`
  - Description: 获取指定部门下的所有成员，返回每位成员的用户 ID（userId）和显示名称（如真实姓名或昵称）。结果受组织可见性控制：若调用者无权查看某成员（例如该成员所在子部门被隐藏，或其个人信息设为私密），则该成员不会出现在返回列表中。适用于需要展示部门人员列表、选择协作成员等场景，仅支持调用者有权限访问的部门。
  - Flags: `--ids`
  - Schema: `skills/generated/docs/schema/contact/get_dept_members_by_deptId.json`
- `get_sub_depts_by_dept_id`
  - Path: `contact.get_sub_depts_by_dept_id`
  - CLI route: `dws contact get_sub_depts_by_dept_id`
  - Description: 根据指定的部门 ID，获取其直接子部门列表，返回每个子部门的部门 ID、名称。结果受组织架构可见性控制：仅返回调用者有权限查看的子部门；若父部门不可见或无子部门，则返回空列表。
  - Flags: `--deptId`
  - Schema: `skills/generated/docs/schema/contact/get_sub_depts_by_dept_id.json`
- `user get`
  - Path: `contact.get_user_info_by_user_ids`
  - CLI route: `dws contact user get`
  - Description: 获取指定用户 ID 列表对应的员工详细信息，包括人员基本信息（ID、名称、主管名称、主管userId等）、所属角色信息、所在部门信息。返回结果受组织可见性规则限制：若调用者无权查看某员工（如部门隐藏、手机号设为私密等），则相应字段可能被过滤或不返回该员工。适用于需要批量获取同事信息的场景，如组织架构展示、审批人选择等。仅返回调用者权限范围内的有效数据。
  - Flags: `--ids`
  - Schema: `skills/generated/docs/schema/contact/get_user_info_by_user_ids.json`
- `search_contact_by_key_word`
  - Path: `contact.search_contact_by_key_word`
  - CLI route: `dws contact search_contact_by_key_word`
  - Description: 根据关键词搜索好友和同事
  - Flags: `--keyword`
  - Schema: `skills/generated/docs/schema/contact/search_contact_by_key_word.json`
- `dept search`
  - Path: `contact.search_dept_by_keyword`
  - CLI route: `dws contact dept search`
  - Description: 根据关键词模糊搜索部门，返回匹配的部门列表，包含每个部门的 ID、名称。搜索范围限于调用者有权限查看的组织架构；若关键词无匹配结果或部门因可见性设置被隐藏，则相应部门不会出现在结果中。
  - Flags: `--keyword`
  - Schema: `skills/generated/docs/schema/contact/search_dept_by_keyword.json`
- `user search`
  - Path: `contact.search_user_by_key_word`
  - CLI route: `dws contact user search`
  - Description: 搜索组织内成员，并返回成员的userId。如果需要查询详情，需要调用另外一个工具
  - Flags: `--keyword`
  - Schema: `skills/generated/docs/schema/contact/search_user_by_key_word.json`
- `user search-mobile`
  - Path: `contact.search_user_by_mobile`
  - CLI route: `dws contact user search-mobile`
  - Description: 通过手机号搜索获取用户名称和userId。
  - Flags: `--mobile`
  - Schema: `skills/generated/docs/schema/contact/search_user_by_mobile.json`
