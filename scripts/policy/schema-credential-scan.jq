# 仅允许 dev.connect 的已声明 argv 参数名；字符串值、默认值与未知元数据不豁免。
walk(
  if type == "object" and .canonical_path == "dev.connect" and (.parameters | type) == "object" then
    .parameters["robot-client-secret"] as $parameter |
    if (.parameters | has("robot-credential-parameter") | not)
       and ($parameter | type) == "object"
       and $parameter.type == "string"
       and $parameter.property == "robotClientSecret"
       and ($parameter.required | type) == "boolean"
       and ($parameter | keys - ["description", "field_provenance", "property", "required", "type"] | length) == 0
       and (($parameter.field_provenance // {}) | keys - ["description", "property", "required", "required_when", "type"] | length) == 0
    then .parameters |= with_entries(if .key == "robot-client-secret" then .key = "robot-credential-parameter" else . end)
    else . end
  else . end
)
