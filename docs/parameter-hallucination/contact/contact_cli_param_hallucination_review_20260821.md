# Contact CLI 参数幻觉补充复核（2026-08-21）

独立候选已按线上 `main` `11934eed057267d97e7442ddd420c711ee1802dc` 重建为
66 concepts / 367 overrides / 728 fixtures，fresh generate 与嵌入式 PreParse 通过。

25 个命令产生有效差异：28 alias、116 block、7 ambiguous。只保留姓名查询、单用户 ID、部门 JSON、
等可复用且可证明的中央角色；昵称和主管用户 ID 是局部 flag 角色，已下沉到精确命令 override。
同时移除无生成效果的 phone/mobile-number 猜测、`avatar-file` 等伪别名。
完整手机号反查与姓名语义搜索仍按 Contact/AISearch 产品边界执行，不由参数名兜底互相代替。
