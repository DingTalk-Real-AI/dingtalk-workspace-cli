# 5分钟搭好团队知识库：DWS 让 IT 服务台自己开口答疑，你值得拥有

> 💡 **省流版摘要：**
> 别再用鼠标一个个建文档了，真的没必要！本文带你用 DWS（钉钉命令行工具），5 分钟搭好一个团队知识库：建空间、搭目录、批量导入 FAQ、配权限，全程命令行一步到位。最后再花 30 秒把它挂到钉钉机器人上——同事在群里 @ 一下，知识库自己开口答疑。知识管理员从"搬运工"升级为"甩手掌柜"，就是这么简单。
>
> （温馨提示：本文内容可以直接丢给智能体，让智能体一次性逐步安装和使用）

## 一、痛点吐槽：管知识库有多累？

身为企业 IT / 知识管理员，你肯定经历过这些"九九八十一难"……

- **点击马拉松**：建空间点 5 下、建文件夹点 3 下、建一篇文档再点 4 下。一个季度下来，鼠标点击次数比写的字还多。
- **搬运工噩梦**：几百篇历史 FAQ 散落在本地 Word / Markdown 里，要搬进钉钉知识库？复制粘贴到天荒地老，格式还经常翻车。
- **权限苦差**：新同事入职要加权限、转岗要改角色、离职要移除。逐个空间点进去操作，漏一个就是安全隐患。
- **知识沉睡**：库是建好了，可同事还是习惯私聊问你"VPN 又连不上了怎么办"。知识库躺着吃灰，你继续当人肉客服。😭

今天，DWS（DingTalk Workspace CLI）闪亮登场！🌟

你不需要写一行代码，只要在终端敲几行命令，知识库的"建、搬、管、用"全链路一次搞定。更香的是：搭好的知识库可以直接挂到钉钉机器人上，让知识自己开口答疑。

## 二、DWS 是个啥？知识库的"遥控指挥中心"

DWS 是钉钉能力的原子化封装，把复杂的 OpenAPI 打包成简单指令。管知识库这件事，主要靠它的"三驾马车"：

| 命令族 | 能干什么 |
|---|---|
| 🗂️ `dws wiki` | 知识库空间、目录节点、成员权限的全生命周期管理 |
| 📄 `dws doc` | 文档内容读写、本地文件批量导入、模板套用 |
| 📁 `dws drive` | 钉盘文件上传下载、全局搜索、归档备份 |

你可以把它想象成知识库的"遥控指挥中心" 🎮——既能你手动按（终端敲命令，比点界面快 10 倍），也能让 AI 帮你按（Claude Code、Qoder 等智能体直接听懂并调用）。

**适合谁用：**

- **企业 IT / 知识管理员**：批量建库、批量导入、批量管权限，脚本化解放双手
- **开发者 / AI 玩家**：把知识库挂到机器人上，打造 24 小时答疑小能手
- **重度钉钉用户 / 效率党**：一条命令搜全库，比在界面里翻目录快得多

## 三、搭好的知识库能帮你做什么？

| 场景 | 玩法 |
|---|---|
| 🛟 IT 服务台 FAQ 库 | VPN、邮箱、打印机常见问题集中沉淀，机器人自动答疑 |
| 📜 制度流程库 | 报销、请假、采购制度批量导入，全文秒搜 |
| 🎓 新人上岗手册 | 按部门建目录，入职即授权限，自助通关 |
| 🤖 知识库 + 机器人 | `--knowledge-source wiki:<spaceId>` 一挂，群里 @ 它就答 |

所想即所得，拒绝画饼，直接上菜！🍽️

## 四、5分钟倒计时，搭好你的团队知识库

> 以下命令全部经过真实环境跑通验证，放心照抄。

### 0. 安装并登录 DWS（约 1 分钟）

把以下指令复制给你的智能体（Claude Code、Qoder、Codex 等）执行，或手动在终端跑：

macOS / Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install.sh | sh
```

Windows（PowerShell）：

```powershell
irm https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install.ps1 | iex
```

登录（提示授权请扫码）：

```bash
dws auth login
```

【截图位：dws auth status 显示 token_valid: true】

### 1. 建一个知识库空间（10 秒）

```bash
dws wiki space create --name "IT服务台知识库" --desc "IT 常见问题与制度流程"
```

返回里的 `workspaceId` 就是空间的身份证号，后面每步都要用它。

【截图位：返回 workspaceId 与 spaceUrl】

### 2. 搭目录结构（20 秒）

知识库的结构 = 文件夹节点 + 文档节点。先建分类文件夹：

```bash
dws wiki node create --workspace <workspaceId> --name "常见问题FAQ" --type folder
dws wiki node create --workspace <workspaceId> --name "制度流程" --type folder
```

在文件夹下建一篇空文档（不加 `--folder` 就建在根目录）：

```bash
dws wiki node create --workspace <workspaceId> --name "VPN连接失败排查指南" --folder <文件夹nodeId>
```

### 3. 批量导入历史文档（1 分钟，重头戏！）

几百篇本地 FAQ 不用复制粘贴，`doc import` 直接整批灌进知识库，Word、Excel、Markdown、txt 通吃：

```bash
# 单篇导入到指定文件夹
dws doc import --file ./vpn-faq.md --workspace <workspaceId> --folder <文件夹nodeId> --name "VPN连接失败排查指南"

# 批量导入整个目录（bash 一把梭）
for f in ./faq/*.md; do
  dws doc import --file "$f" --workspace <workspaceId> --folder <文件夹nodeId>
done
```

已有在线文档想补内容？Markdown 直接写入：

```bash
dws doc update --node <文档nodeId> --content-file ./补充内容.md --mode append
```

【截图位：终端批量导入的滚动输出 + 知识库里齐刷刷的文档列表】

### 4. 配权限：把人拉进来（30 秒）

```bash
# 先用通讯录查到同事的 userId
dws contact user search --query "张三"

# 加为编辑者（--users 支持逗号分隔批量加）
dws wiki member add --workspace <workspaceId> --users <userId1>,<userId2> --role EDITOR

# 随时盘点成员
dws wiki member list --workspace <workspaceId>
```

### 5. 验收：搜一下，秒级命中（10 秒）

```bash
# 库内全文搜索
dws wiki node search --workspace <workspaceId> --query "VPN"

# 全局搜知识库空间
dws wiki space search --query "IT服务台"
```

【截图位：搜索结果命中文档标题】

### 6. 封神一步：挂到机器人上，知识自己开口答疑（30 秒）

如果你已经按《5分钟抱走你的嘴替机器人》建好了钉钉机器人，只需加一个参数：

```bash
dws dev connect --channel claudecode \
  --robot-client-id <你的机器人ID> --robot-client-secret <你的机器人密钥> \
  --knowledge-source wiki:<workspaceId>
```

机器人会自动从知识库拉取知识并缓存。同事在群里 @ 它问"VPN 连不上怎么办"，它直接引用你刚导入的排查指南回答——你，终于不用当复读机了。😎

## 五、进阶使用技巧

### 知识库管理速查表

| 操作 | 命令 |
|---|---|
| 列出我的个人空间 | `dws wiki space list --type myWikiSpace` |
| 列出组织知识库 | `dws wiki space list --type orgWikiSpace` |
| 浏览库内节点树 | `dws wiki node list --workspace <ID> [--folder <nodeId>]` |
| 移动 / 复制节点 | `dws wiki node move` / `dws wiki node copy` |
| 改成员角色 | `dws wiki member update --users <UID> --role VIEWER` |
| 移除成员 | `dws wiki member remove --users <UID>` |
| 删除整个空间 | `dws wiki space delete --workspace <ID>`（进回收站，可恢复） |

### 老手避坑指南 ⛳

- 建在线表格用 `--type axls`，**`asheet` 服务端不支持**，别踩坑。
- `member list` 只返回姓名和角色、**不返回 userId**；要串联 `update` / `remove`，先用 `dws contact user search --query "<姓名>"` 反查。
- 搜索关键词的 flag 是 `--query`，`--keyword` 是遗留别名，新脚本请用 `--query`。
- 所有命令加 `--format json`，配合 `--jq` 过滤字段，写脚本时稳得一批。
- 破坏性操作（删空间、覆盖写文档）前加 `--dry-run` 先预览，确认无误再执行。

### 让机器人答得更好

| 开关 | 作用 |
|---|---|
| `--knowledge-source wiki:<spaceId>` | 从钉钉知识库拉知识作为答疑来源（本文主角） |
| `--knowledge-dir <目录>` | 挂本地 .md/.txt 知识目录，可与上面并存 |
| `--allowed-groups / --allowed-users` | 白名单，只让指定群或人触发 |
| `--daemon` | 后台常驻，关掉终端也不断线 |

## 六、更多 DWS 官方信息

- 钉钉 CLI 官网：https://open.dingtalk.com/dingtalk-cli
- 开源仓库：https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli
- 上一篇姊妹篇：《5分钟抱走你的嘴替机器人：启动钉钉DWS，你值得拥有》

## 七、欢迎加入交流群

【截图位：DWS 交流群二维码】

遇到问题来群里喊一声，官方同学在线答疑。下一篇想看什么？批量备份知识库？给知识库做权限审计？留言区点菜！🍻
