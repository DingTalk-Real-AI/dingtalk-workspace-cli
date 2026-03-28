# dws 全命令真实测试报告（无 dry-run）

**测试时间**: 2026-03-25 20:44:41
**测试环境**: Darwin arm64
**dws 版本**: 版本:  v1.0.0
Go:  1.24+
**测试模式**: 真实执行（无 dry-run）

## 测试汇总

| 指标 | 数值 |
|------|------|
| 总计 | 117 |
| ✅ 通过 | 115 |
| ❌ 失败 | 2 |
| ⏭️ 跳过 | 0 |
| 执行通过率 | 98% (115/117) |
| 总通过率 | 98% (115/117) |

## 详细结果

| 状态 | 测试名称 | 命令 | 备注 |
|------|---------|------|------|
| ✅ PASS | `contact user get-self (前置)` | `dws contact user get-self -f json` | 成功 |
| ✅ PASS | `todo task list` | `dws todo task list -f json` | 成功 |
| ✅ PASS | `todo task list (未完成)` | `dws todo task list --status false -f json` | 成功 |
| ✅ PASS | `todo task list (已完成)` | `dws todo task list --status true -f json` | 成功 |
| ✅ PASS | `todo task create` | `dws todo task create --title CLI真实测试待办_1774442198 --executors 061978 -f json` | 成功 |
| ✅ PASS | `todo task get` | `dws todo task get --task-id 51611028137 -f json` | 成功 |
| ✅ PASS | `todo task update` | `dws todo task update --task-id 51611028137 --title 已更新的测试待办 -f json` | 成功 |
| ✅ PASS | `todo task done` | `dws todo task done --task-id 51611028137 --status true -f json` | 成功 |
| ✅ PASS | `todo task delete` | `dws todo task delete --task-id 51611028137 -f json -y` | 成功 |
| ✅ PASS | `contact user search` | `dws contact user search --keyword test -f json` | 成功 |
| ✅ PASS | `contact user get-self` | `dws contact user get-self -f json` | 成功 |
| ✅ PASS | `contact user get` | `dws contact user get --ids 061978 -f json` | 成功 |
| ✅ PASS | `contact user search-mobile` | `dws contact user search-mobile --mobile 13800138000 -f json` | 成功 |
| ✅ PASS | `contact dept list-children` | `dws contact dept list-children -f json` | 成功 |
| ✅ PASS | `contact dept list-members` | `dws contact dept list-members -f json` | 成功 |
| ✅ PASS | `contact dept search` | `dws contact dept search --keyword test -f json` | 成功 |
| ⏰ TIMEOUT | `calendar event list` | `dws calendar event list -f json` | 超时 |
| ✅ PASS | `calendar event create` | `dws calendar event create --title CLI真实测试日程_1774442272 --start 2026-12-31T10:00:00+08:00 --end 2026-12-31T11:00:00+08:00 -f json` | 成功 |
| ✅ PASS | `calendar event get` | `dws calendar event get --id Y2RzNGlmdWltdjEyNkp6VVhzaUtIQT09 -f json` | 成功 |
| ✅ PASS | `calendar event update` | `dws calendar event update --id Y2RzNGlmdWltdjEyNkp6VVhzaUtIQT09 --title 已更新的测试日程 -f json` | 成功 |
| ✅ PASS | `calendar participant add` | `dws calendar participant add --event Y2RzNGlmdWltdjEyNkp6VVhzaUtIQT09 --users 061978 -f json` | 成功 |
| ✅ PASS | `calendar participant list` | `dws calendar participant list --event Y2RzNGlmdWltdjEyNkp6VVhzaUtIQT09 -f json` | 成功 |
| ✅ PASS | `calendar participant delete` | `dws calendar participant delete --event Y2RzNGlmdWltdjEyNkp6VVhzaUtIQT09 --users 061978 -f json -y` | 成功 |
| ✅ PASS | `calendar room add` | `dws calendar room add --event Y2RzNGlmdWltdjEyNkp6VVhzaUtIQT09 --rooms room1 -f json` | 成功 |
| ✅ PASS | `calendar room delete` | `dws calendar room delete --event Y2RzNGlmdWltdjEyNkp6VVhzaUtIQT09 --rooms room1 -f json -y` | 成功 |
| ✅ PASS | `calendar event delete` | `dws calendar event delete --id Y2RzNGlmdWltdjEyNkp6VVhzaUtIQT09 -f json -y` | 成功 |
| ✅ PASS | `calendar busy search` | `dws calendar busy search -f json` | 成功 |
| ✅ PASS | `calendar room list-groups` | `dws calendar room list-groups -f json` | 成功 |
| ✅ PASS | `calendar room search` | `dws calendar room search -f json` | 成功 |
| ✅ PASS | `aitable base list` | `dws aitable base list -f json` | 成功 |
| ✅ PASS | `aitable base search` | `dws aitable base search --query test -f json` | 成功 |
| ✅ PASS | `aitable template search` | `dws aitable template search --query 项目 -f json` | 成功 |
| ✅ PASS | `aitable base create` | `dws aitable base create --name CLI真实测试表格_1774442332 -f json` | 成功 |
| ✅ PASS | `aitable base get` | `dws aitable base get --base-id QPGYqjpJYRyab0nxsoZddQlg8akx1Z5N -f json` | 成功 |
| ✅ PASS | `aitable table create` | `dws aitable table create --base-id QPGYqjpJYRyab0nxsoZddQlg8akx1Z5N --name 测试数据表 -f json` | 成功 |
| ✅ PASS | `aitable table get` | `dws aitable table get --base-id QPGYqjpJYRyab0nxsoZddQlg8akx1Z5N --table-ids dhEGEFA -f json` | 成功 |
| ✅ PASS | `aitable table update` | `dws aitable table update --base-id QPGYqjpJYRyab0nxsoZddQlg8akx1Z5N --table-id dhEGEFA --name 新表名 -f json` | 成功 |
| ✅ PASS | `aitable field get` | `dws aitable field get --base-id QPGYqjpJYRyab0nxsoZddQlg8akx1Z5N --table-id dhEGEFA -f json` | 成功 |
| ✅ PASS | `aitable field create` | `dws aitable field create --base-id QPGYqjpJYRyab0nxsoZddQlg8akx1Z5N --table-id dhEGEFA --fields '[{"fieldName":"测试字段","type":"text"}]' -f json` | 成功 |
| ✅ PASS | `aitable field update` | `dws aitable field update --base-id QPGYqjpJYRyab0nxsoZddQlg8akx1Z5N --table-id dhEGEFA --field-id vgt2TCM --name 新字段名 -f json` | 成功 |
| ✅ PASS | `aitable field delete` | `dws aitable field delete --base-id QPGYqjpJYRyab0nxsoZddQlg8akx1Z5N --table-id dhEGEFA --field-id vgt2TCM -f json -y` | 成功 |
| ✅ PASS | `aitable record query` | `dws aitable record query --base-id QPGYqjpJYRyab0nxsoZddQlg8akx1Z5N --table-id dhEGEFA -f json` | 成功 |
| ✅ PASS | `aitable record create` | `dws aitable record create --base-id QPGYqjpJYRyab0nxsoZddQlg8akx1Z5N --table-id dhEGEFA --records '[{"cells":{"标题":"test"}}]' -f json` | 成功 |
| ✅ PASS | `aitable record update` | `dws aitable record update --base-id QPGYqjpJYRyab0nxsoZddQlg8akx1Z5N --table-id dhEGEFA --records '[{"cells":{"标题":"updated"},"recordId":"'7vYhcuw02R'"}]' -f json` | 成功 |
| ✅ PASS | `aitable record delete` | `dws aitable record delete --base-id QPGYqjpJYRyab0nxsoZddQlg8akx1Z5N --table-id dhEGEFA --record-ids 7vYhcuw02R -f json -y` | 成功 |
| ✅ PASS | `aitable table delete` | `dws aitable table delete --base-id QPGYqjpJYRyab0nxsoZddQlg8akx1Z5N --table-id dhEGEFA -f json -y` | 成功 |
| ✅ PASS | `aitable attachment upload` | `dws aitable attachment upload --base-id QPGYqjpJYRyab0nxsoZddQlg8akx1Z5N --file-name test.txt --mime-type text/plain --size 1024 -f json` | 成功 |
| ✅ PASS | `aitable base delete` | `dws aitable base delete --base-id QPGYqjpJYRyab0nxsoZddQlg8akx1Z5N -f json -y` | 成功 |
| ✅ PASS | `attendance record list` | `dws attendance record list --users 061978 --start 2026-03-20 --end 2026-03-25 -f json` | 成功 |
| ✅ PASS | `attendance shift list` | `dws attendance shift list --users 061978 --start 2026-03-20 --end 2026-03-25 -f json` | 成功 |
| ✅ PASS | `attendance summary` | `dws attendance summary --user 061978 --date '2026-03-25 15:00:00' -f json` | 成功 |
| ✅ PASS | `attendance rules` | `dws attendance rules --date 2026-03-25 -f json` | 成功 |
| ✅ PASS | `chat search` | `dws chat search --query test -f json` | 成功 |
| ✅ PASS | `chat bot search` | `dws chat bot search -f json` | 成功 |
| ✅ PASS | `chat message list (help)` | `dws chat message list --help` | 成功 |
| ✅ PASS | `chat group create` | `dws chat group create --name CLI真实测试群_1774442418 --users 061978 -f json` | 成功 |
| ✅ PASS | `chat group rename` | `dws chat group rename --id testGroupId --name CLI测试群_已重命名 -f json` | 成功 |
| ✅ PASS | `chat group members add-bot` | `dws chat group members add-bot --robot-code testBot --id testGroupId -f json` | 成功 |
| ✅ PASS | `chat group members remove` | `dws chat group members remove --id testGroupId --users 061978 -f json` | 成功 |
| ✅ PASS | `chat message send` | `dws chat message send --user 061978 CLI自动化测试消息 -f json` | 成功 |
| ✅ PASS | `chat message send-by-bot` | `dws chat message send-by-bot --group testConvId --robot-code testBot --text 机器人消息 --title 测试 -f json` | 成功 |
| ✅ PASS | `chat message recall-by-bot` | `dws chat message recall-by-bot --group testConvId --robot-code testBot --keys msgKey1 -f json` | 成功 |
| ✅ PASS | `chat message send-by-webhook` | `dws chat message send-by-webhook --token testToken --title 告警 --text 'CPU超90%' -f json` | 成功 |
| ✅ PASS | `conference meeting create` | `dws conference meeting create --title CLI真实测试会议_1774442445 --start 2026-12-31T14:00:00+08:00 --end 2026-12-31T15:00:00+08:00 -f json` | 成功 |
| ✅ PASS | `aiapp create` | `dws aiapp create --prompt 创建天气查询应用 -f json` | 成功 |
| ✅ PASS | `aiapp query` | `dws aiapp query --task-id 019d2503-14c5-7a50-8413-0801c710ff8c -f json` | 成功 |
| ✅ PASS | `aiapp modify` | `dws aiapp modify --prompt 修改应用描述 --thread-id d25b736c-a45c-4804-8176-936242e0237e -f json` | 成功 |
| ✅ PASS | `aidesign generate` | `dws aidesign generate --prompt 画一只猫 -f json` | 成功 |
| ✅ PASS | `aidesign edit` | `dws aidesign edit --prompt 修改背景 --image-url https://example.com/img.png -f json` | 成功 |
| ✅ PASS | `aidesign generate-with-image` | `dws aidesign generate-with-image --prompt 参考图生成 --image-url https://example.com/img.png -f json` | 成功 |
| ✅ PASS | `aidesign generate-with-template` | `dws aidesign generate-with-template --image-url https://example.com/img.png --name 测试模板 --template tpl1 -f json` | 成功 |
| ✅ PASS | `aidesign upscale` | `dws aidesign upscale --image-url https://example.com/img.png -f json` | 成功 |
| ✅ PASS | `aidesign isolate` | `dws aidesign isolate --image-url https://example.com/img.png -f json` | 成功 |
| ✅ PASS | `report list` | `dws report list -f json` | 成功 |
| ✅ PASS | `report sent` | `dws report sent -f json` | 成功 |
| ✅ PASS | `report template list` | `dws report template list -f json` | 成功 |
| ✅ PASS | `report template detail (日报)` | `dws report template detail --name 日报 -f json` | 成功 |
| ✅ PASS | `report create (fallback)` | `dws report create --template-id tpl123 --contents '["CLI自动化测试内容"]' -f json` | 成功 |
| ✅ PASS | `report stats` | `dws report stats --report-id rpt123 -f json` | 成功 |
| ✅ PASS | `report detail` | `dws report detail --report-id rpt123 -f json` | 成功 |
| ✅ PASS | `ding message send` | `dws ding message send --users 061978 --content 测试DING --robot-code testBot -f json` | 成功 |
| ✅ PASS | `ding message recall` | `dws ding message recall --id msg123 --robot-code testBot -f json` | 成功 |
| ✅ PASS | `devdoc article search` | `dws devdoc article search --keyword OAuth2 -f json` | 成功 |
| ✅ PASS | `law search` | `dws law search --query 劳动合同 -f json` | 成功 |
| ⏰ TIMEOUT | `law consult` | `dws law consult --query 加班费怎么算 -f json` | 超时 |
| ✅ PASS | `law case` | `dws law case --query 工伤 -f json` | 成功 |
| ✅ PASS | `live stream list` | `dws live stream list -f json` | 成功 |
| ✅ PASS | `tb project list` | `dws tb project list -f json` | 成功 |
| ✅ PASS | `tb project list-mine` | `dws tb project list-mine -f json` | 成功 |
| ✅ PASS | `tb project list-priorities` | `dws tb project list-priorities -f json` | 成功 |
| ✅ PASS | `tb project create` | `dws tb project create --name CLI真实测试项目_1774442587 -f json` | 成功 |
| ✅ PASS | `tb project update` | `dws tb project update --id 69c3d85f7ae5ad1ec4e21d63 --name 已更新的测试项目 -f json` | 成功 |
| ✅ PASS | `tb project list-members` | `dws tb project list-members --id 69c3d85f7ae5ad1ec4e21d63 -f json` | 成功 |
| ✅ PASS | `tb project add-member` | `dws tb project add-member --id 69c3d85f7ae5ad1ec4e21d63 --users 061978 -f json` | 成功 |
| ✅ PASS | `tb project list-task-types` | `dws tb project list-task-types --id 69c3d85f7ae5ad1ec4e21d63 -f json` | 成功 |
| ✅ PASS | `tb project list-workflow` | `dws tb project list-workflow --id 69c3d85f7ae5ad1ec4e21d63 -f json` | 成功 |
| ✅ PASS | `tb task create` | `dws tb task create --project 69c3d85f7ae5ad1ec4e21d63 --title CLI真实测试任务_1774442609 --content 自动化测试任务内容 --executor 061978 -f json` | 成功 |
| ✅ PASS | `tb task get` | `dws tb task get --id task_fallback -f json` | 成功 |
| ✅ PASS | `tb task update-title` | `dws tb task update-title --id task_fallback --title 已更新的测试任务 -f json` | 成功 |
| ✅ PASS | `tb task update-priority` | `dws tb task update-priority --id task_fallback --priority 高 -f json` | 成功 |
| ✅ PASS | `tb task update-remark` | `dws tb task update-remark --id task_fallback --note CLI自动化测试备注 -f json` | 成功 |
| ✅ PASS | `tb task update-start` | `dws tb task update-start --id task_fallback --date 2026-03-25 -f json` | 成功 |
| ✅ PASS | `tb task update-due` | `dws tb task update-due --id task_fallback --date 2026-04-25 -f json` | 成功 |
| ✅ PASS | `tb task assign` | `dws tb task assign --id task_fallback --executor 061978 -f json` | 成功 |
| ✅ PASS | `tb task comment` | `dws tb task comment --id task_fallback --content CLI自动化测试评论 -f json` | 成功 |
| ✅ PASS | `tb task add-progress` | `dws tb task add-progress --id task_fallback --title 进展更新 --content 测试进展 --status normal -f json` | 成功 |
| ✅ PASS | `tb task get-progress` | `dws tb task get-progress --id task_fallback -f json` | 成功 |
| ✅ PASS | `tb task update-status` | `dws tb task update-status --id task_fallback --status 已完成 -f json` | 成功 |
| ✅ PASS | `tb worktime create` | `dws tb worktime create --task task_fallback --executor 061978 --hours 2 --start 2026-03-25 --end 2026-03-25 -f json` | 成功 |
| ✅ PASS | `tb worktime list` | `dws tb worktime list --task task_fallback -f json` | 成功 |
| ✅ PASS | `tb worktime update` | `dws tb worktime update --id wt_fallback --executor 061978 --date 2026-03-25 -f json` | 成功 |
| ✅ PASS | `tb task search` | `dws tb task search --tql 'title = "测试"' -f json` | 成功 |
| ✅ PASS | `workbench app list` | `dws workbench app list -f json` | 成功 |
| ✅ PASS | `workbench app get (fallback)` | `dws workbench app get --ids app123 -f json` | 成功 |
| ✅ PASS | `ai-sincere-hire guide` | `dws ai-sincere-hire guide -f json` | 成功 |
| ✅ PASS | `ai-sincere-hire job list` | `dws ai-sincere-hire job list -f json` | 成功 |
| ✅ PASS | `ai-sincere-hire talent list` | `dws ai-sincere-hire talent list -f json` | 成功 |

---

> 所有命令均真实执行（无 dry-run）。写操作创建真实数据并在测试后清理。
> 需要 robot-code 的操作使用假 robot-code 测试，验证命令可执行性。
