# 相关产品

## 相关产品

- [contact](../../dingtalk-contact/references/contact.md) — 搜索同事/好友，获取 userId 用于 --user、send-by-bot --users、send-by-bot --at-user-ids、list-by-sender --sender-user-id；获取 openDingTalkId 用于 message send 的 --at-open-dingtalk-ids、--open-dingtalk-id、send-by-bot --open-dingtalk-ids、send-by-bot --at-open-dingtalk-ids、list-by-sender 的 --sender-open-dingtalk-id
- [drive](../../dingtalk-drive/references/drive.md) — 云盘文件管理；chat 纯文件发送优先用 `chat message send --msg-type file --file-path`

### text translate (文本翻译)

#### 翻译文本内容 — 将指定文本翻译成目标语言
```
Usage:
  dws chat text translate [flags]
Example:
  dws chat text translate --query "你好世界" --to en_US
  dws chat text translate --query "Hello World" --to zh_CN
  dws chat text translate --query "Bonjour" --to ja_JP
Flags:
      --query string   待翻译的文本内容 (必填)
      --to string      目标语言代码 (必填，默认 en_US)
```

支持的目标语言代码:
en_US, zh_CN, zh_TW, zh_HK, ja_JP, ko_KR, vi_VN, th_TH,
id_ID, ms_MY, es_419, fr_FR, pt_BR, tr_TR, ru_RU, de_DE,
hi_IN, hu_HU, pl_PL, sv_SE, fi_FI, cs_CZ, ar_SA, tl_PH,
he_IL, nl_NL, lo_LA, it_IT

注意:
- `--to` 参数必须使用上述支持的语言代码，大小写敏感
- 默认目标语言为 en_US（美式英语）
- 返回结果为翻译后的纯文本内容
