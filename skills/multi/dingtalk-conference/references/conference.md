# 视频会议 (conference) 命令参考

涵盖发起即时会议、预约会议、邀请成员入会、会中控制。

> **运行约束**：需钉钉桌面端运行。所有命令须加 `--format json`。
> **SENSITIVE 约束**：SENSITIVE 标记的命令禁止自动加 `--yes`；须先向用户确认，用户明确同意后方可添加。

## 命令树

```
dws conference
  ├── start                              # 发起即时会议（无时间）
  ├── status                             # 查询当前会议状态
  ├── get-id                             # 获取当前会议ID
  ├── end                                # 结束会议（SENSITIVE）
  ├── leave                              # 离开会议（SENSITIVE）
  ├── meeting reserve                    # 预约会议（有时间）
  ├── member invite                      # 邀请指定人入会（有人名）
  ├── mic [mute|unmute|mute-all]         # 麦克风
  ├── camera [open|close]                # 摄像头
  ├── share [start|stop|panel|annotate]  # 屏幕共享
  ├── record [start|stop|request]        # 云录制
  ├── subtitle [open|close]              # 实时字幕
  ├── transcript [open|close]            # 智能听记
  ├── view [standard|speaker|grid]       # 视图切换
  ├── invite [all|panel]                 # 一键呼叫/打开邀请面板（无具体人名）
  ├── interpretation [open|close]        # 同声传译
  ├── appearance [background-setting|beauty-setting]  # 外观设置
  ├── photo take                         # 全员合影
  ├── interaction panel                  # 表情互动
  └── security menu                      # 安全设置
```

## 意图判断

用户说"开个会/开会" (无时间) → `start`
用户说"预约会议/约个会" (有时间) → `meeting reserve`
用户说"静音/mute" → `mic mute`
用户说"取消静音/unmute" → `mic unmute`
用户说"全员静音" → `mic mute-all`
用户说"开摄像头/打开摄像头" → `camera open`
用户说"关摄像头/关闭摄像头" → `camera close`
用户说"共享屏幕/开始共享" → `share start`
用户说"停止共享" → `share stop`
用户说"共享选择面板" → `share panel`
用户说"标注" → `share annotate`
用户说"开始录制/录制" → `record start`
用户说"停止录制" → `record stop`
用户说"申请录制" → `record request`
用户说"开字幕/打开字幕" → `subtitle open`
用户说"关字幕" → `subtitle close`
用户说"开始听记/打开转写" → `transcript open`
用户说"关闭听记/停止转写" → `transcript close`
用户说"标准视图/画廊视图" → `view standard`
用户说"演讲者视图" → `view speaker`
用户说"宫格视图" → `view grid`
用户说"一键呼叫/呼叫所有人" (无具体人名) → `invite all`
用户说"打开邀请面板" → `invite panel`
用户说"邀请张三入会" (有具体人名) → `member invite`
用户说"同声传译" → `interpretation open/close`
用户说"虚拟背景" → `appearance background-setting`
用户说"美颜" → `appearance beauty-setting`
用户说"合影" → `photo take`
用户说"表情互动" → `interaction panel`
用户说"安全设置" → `security menu`
用户说"结束会议" → `end`
用户说"离开会议" → `leave`
用户说"会议状态/在不在会" → `status`

### ⛔ 易混淆路由

| 用户说 | 正确命令 | 判断依据 | 常见错误 |
|--------|---------|---------|---------|
| "开个会" (无时间) | `start` | 没有明确开始/结束时间 | ✗ 误用 meeting reserve |
| "预约明天下午的会" (有时间) | `meeting reserve` | 有明确开始和结束时间 | ✗ 误用 start |
| "一键呼叫" (无具体人名) | `invite all` | 没有具体人名 | ✗ 误用 member invite |
| "邀请张三入会" (有具体人名) | `member invite` | 有明确的人名 | ✗ 误用 invite all |
| "打开邀请面板" | `invite panel` | 无具体人名，仅打开面板 | ✗ 误用 member invite |

> **关键辨析**：start vs meeting reserve = 是否有时间；invite all vs member invite = 是否有人名；conference vs calendar = 会议控制 vs 日程管理。

## 规则与约束

1. **禁止**编造 conferenceId — 必须从 `start`/`status`/`get-id` 返回中提取。
2. **禁止**自动添加 `--yes` — SENSITIVE 操作（end/leave/record stop/transcript close）须先向用户确认，用户明确同意后方可添加。
3. **禁止**在无明确用户意图时打开摄像头或开始共享屏幕 — 会直接影响用户设备。
4. **禁止**使用不存在的子命令试探 — 合法命令以上方命令树为准。
5. 所有命令必须加 `--format json`。
6. `member invite` 的 `--nicks` 和 `--open-dingtalk-ids` 数量必须一一对应；`openDingTalkId` 需通过 `contact user search` 获取。

---

## 会议生命周期

### start — 发起即时会议
```
Usage:
  dws conference start [flags]
Example:
  dws conference start --title "周会" --format json
Flags:
      --title string    会议标题 (可选，不填则使用默认标题)
      --format string   输出格式，固定 json (必填)
Returns:
  conferenceId   string   会议 ID（可用于 conference member invite 的 --conference-id）
  title          string   会议标题
```

### status — 查询当前会议状态
```
Usage:
  dws conference status [flags]
Example:
  dws conference status --format json
Flags:
      --format string   输出格式，固定 json (必填)
Returns:
  status         string   会议状态（IN_MEETING / NOT_IN_MEETING）
  conferenceId   string   当前会议 ID（仅 IN_MEETING 时返回）
  title          string   会议标题
```

### get-id — 获取当前会议 ID
```
Usage:
  dws conference get-id [flags]
Example:
  dws conference get-id --format json
Flags:
      --format string   输出格式，固定 json (必填)
Returns:
  conferenceId   string   当前会议 ID
```

### end — 全员结束会议 (SENSITIVE)
```
Usage:
  dws conference end [flags]
Example:
  dws conference end --format json
Flags:
      --format string   输出格式，固定 json (必填)
Note: SENSITIVE 操作，须先向用户确认，用户明确同意后方可添加 --yes。
```

### leave — 自己离开会议 (SENSITIVE)
```
Usage:
  dws conference leave [flags]
Example:
  dws conference leave --format json
Flags:
      --format string   输出格式，固定 json (必填)
Note: SENSITIVE 操作，须先向用户确认，用户明确同意后方可添加 --yes。
```

---

## meeting reserve — 预约会议

```
Usage:
  dws conference meeting reserve [flags]
Example:
  dws conference meeting reserve --title "产品评审会" \
    --start 2026-03-11T14:00:00+08:00 --end 2026-03-11T15:00:00+08:00 --format json
Flags:
      --title string    会议标题 (必填)
      --start string    开始时间 ISO-8601 格式 (必填)
      --end string      结束时间 ISO-8601 格式 (必填)
      --format string   输出格式，固定 json (必填)
Returns:
  conferenceId   string   预约会议 ID
  title          string   会议标题
Note: 不会自动关联日历日程。如需同步日历，需额外使用 calendar 命令。
```

---

## member invite — 邀请指定人入会

```
Usage:
  dws conference member invite [flags]
Example:
  dws conference member invite --conference-id "xxx" \
    --nicks "张三,李四" --open-dingtalk-ids "id1,id2" --format json
Flags:
      --conference-id string       会议ID (必填，通过 get-id 或 start 返回值获取)
      --nicks string               被邀请人昵称，逗号分隔 (必填)
      --open-dingtalk-ids string   被邀请人 openDingTalkId，逗号分隔 (必填，通过 contact user search 获取)
      --format string              输出格式，固定 json (必填)
Note: --nicks 和 --open-dingtalk-ids 数量必须一一对应。
```

---

## mic (麦克风控制)

### mute — 自己静音
```
Usage:  dws conference mic mute --format json
```

### unmute — 取消静音
```
Usage:  dws conference mic unmute --format json
```

### mute-all — 全员静音
```
Usage:  dws conference mic mute-all --format json
```

## camera (摄像头控制)

### open — 开启摄像头
```
Usage:  dws conference camera open --format json
```

### close — 关闭摄像头
```
Usage:  dws conference camera close --format json
```

## share (屏幕共享)

### start — 开始桌面共享
```
Usage:  dws conference share start --format json
```

### stop — 停止共享
```
Usage:  dws conference share stop --format json
```

### panel — 打开共享选择面板
```
Usage:  dws conference share panel --format json
```

### annotate — 打开共享标注工具
```
Usage:  dws conference share annotate --format json
```

## record (云录制)

### start — 开启云录制
```
Usage:  dws conference record start --format json
```

### stop — 停止云录制 (SENSITIVE)
```
Usage:  dws conference record stop --format json
Note: SENSITIVE 操作，须先向用户确认，用户明确同意后方可添加 --yes。
```

### request — 向主持人申请录制
```
Usage:  dws conference record request --format json
```

## subtitle (实时字幕)

### open — 开启实时字幕
```
Usage:  dws conference subtitle open --format json
```

### close — 关闭实时字幕
```
Usage:  dws conference subtitle close --format json
```

## transcript (智能听记)

### open — 开启智能听记
```
Usage:  dws conference transcript open --format json
```

### close — 关闭智能听记 (SENSITIVE)
```
Usage:  dws conference transcript close --format json
Note: SENSITIVE 操作，须先向用户确认，用户明确同意后方可添加 --yes。
```

## view (视图切换)

### standard — 切换为标准视图
```
Usage:  dws conference view standard --format json
```

### speaker — 切换为演讲者视图
```
Usage:  dws conference view speaker --format json
```

### grid — 切换为宫格视图
```
Usage:  dws conference view grid --format json
```

## invite (邀请入会 — 无具体人名)

### all — 一键呼叫所有未入会的成员
```
Usage:  dws conference invite all --format json
```

### panel — 打开邀请选人面板
```
Usage:  dws conference invite panel --format json
```


## interpretation (同声传译)

### open — 开启同声传译
```
Usage:  dws conference interpretation open --format json
```

### close — 关闭同声传译
```
Usage:  dws conference interpretation close --format json
```

## appearance (虚拟背景/美颜设置面板)

### background-setting — 打开虚拟背景设置
```
Usage:  dws conference appearance background-setting --format json
```

### beauty-setting — 打开美颜设置
```
Usage:  dws conference appearance beauty-setting --format json
```

## 其他

### photo take — 发起全员合影
```
Usage:  dws conference photo take --format json
```

### interaction panel — 打开表情互动面板
```
Usage:  dws conference interaction panel --format json
```

### security menu — 打开安全设置菜单
```
Usage:  dws conference security menu --format json
```

---

## 核心工作流

```bash
# 1. 发起即时会议 — 提取 conferenceId
dws conference start --title "周会" --format json

# 2. 查询会议状态（确认当前在会中）
dws conference status --format json

# 3. 邀请指定人入会（需先搜联系人获取 openDingTalkId）
dws contact user search --query "张三" --format json
dws conference member invite --conference-id <conferenceId> \
  --nicks "张三" --open-dingtalk-ids <openDingTalkId> --format json

# 4. 一键呼叫所有未入会成员（无具体人名时）
dws conference invite all --format json

# 5. 会中控制
dws conference mic mute --format json
dws conference camera open --format json
dws conference share start --format json
dws conference record start --format json
dws conference subtitle open --format json
dws conference transcript open --format json
dws conference view grid --format json

# 6. 预约会议（有明确时间）
dws conference meeting reserve --title "产品评审会" \
  --start 2026-03-11T14:00:00+08:00 --end 2026-03-11T15:00:00+08:00 --format json

# 7. 结束会议（SENSITIVE：先向用户确认，用户明确同意后再加 --yes 执行）
# 正确流程：1.向用户展示"即将结束本次会议（所有人离会）" → 2.等用户确认 → 3.带 --yes 执行下面命令
dws conference end --yes --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `start` | `conferenceId` | member invite 的 --conference-id |
| `status` | `conferenceId` | member invite 的 --conference-id |
| `get-id` | `conferenceId` | member invite 的 --conference-id |
| `meeting reserve` | `conferenceId` | 后续会议操作标识 |
| `contact user search` | `openDingTalkId` | member invite 的 --open-dingtalk-ids |
| `contact user search` | `name/nick` | member invite 的 --nicks |

## 常见错误

| 错误关键字 | 处理 |
|-----------|------|
| `服务未就绪` / `ECONNREFUSED` | 提示用户启动并登录钉钉 |
| `NOT_IN_MEETING` | 告知用户当前未在会议中，需要先发起或加入会议 |
| `MISSING_PARAM` | 提示用户提供必要参数，不要编造 |
| `PERMISSION_DENIED` | 告知用户需要主持人权限 |

## 注意事项

- 会中控制命令（mic/camera/share/record/subtitle/transcript/view 等）执行后返回操作结果状态，无额外上下文输出
- `meeting reserve` 不会自动关联日历日程，如需同步日历须额外使用 `calendar` 命令
- `status` 可同时确认是否在会中并获取 conferenceId；`get-id` 仅获取 ID，适合已确认在会中的场景

## 相关产品

- `dingtalk-calendar` (`references/calendar.md`) — 日程/会议室/参与者管理（预约会议后如需同步到日历，使用此命令）
- `dingtalk-contact` (`references/contact.md`) — 通讯录查询（获取 openDingTalkId，member invite 的前置步骤）
- `dingtalk-minutes` (`references/minutes.md`) — 会后听记
