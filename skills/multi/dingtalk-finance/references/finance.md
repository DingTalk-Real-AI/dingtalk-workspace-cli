# 智能财务 (finance) 命令参考

> 覆盖 `dws finance` 全部 13 个命令组：`receipt / invoice / bank / voucher / customer / account / journal / supplier / category / payment / company / digital-invoice / process`。
> MCP 工具名用于 AI 直接调用底层能力；CLI flag 用于命令行调用。两者等价，参数名见每条命令后注释。

## 命令总览

| 命令组 | 用途 | 主要子命令 |
|--------|------|------------|
| `receipt` | 付款单 / 收款单 | `create`、`create-collection` |
| `invoice` | 发票上传 / 开具 / 查询 | `upload`、`issue`、`issue-result`、`recommend-category`、`list-application`、`add-record` |
| `bank` | 银行交易明细 | `create`、`query` |
| `voucher` | 会计凭证 | `entries`、`generate` |
| `customer` | 客户管理 | `list`、`get`、`save` |
| `account` | 企业账户 | `list` |
| `journal` | 现金日报 | `daily`、`detail-url` |
| `supplier` | 供应商 | `search` |
| `category` | 收支类别 | `search` |
| `payment` | 支付 / 付款 | `create`、`list`、`account-list`、`cashier-url`、`account-url`、`payer-list` |
| `company` | 主体管理 | `search`、`save`、`update` |
| `digital-invoice` | 数电发票全流程 | `do-login-status`、`login-page`、`do-login`、`account`、`sms-code`、`goods-code`、`face-qr`、`face-status`、`title`、`issue`、`file`、`skill-version`、`send-email[-saas]`、`batch-draw[-saas]`、`batch-draw-query[-saas]` |
| `process` | 审批单 | `form-data`、`list` |

---

## 付款单 / 收款单 (receipt)

### 创建付款单
```
Usage:
  dws finance receipt create [flags]
Example:
  dws finance receipt create --amount 1000.00
  dws finance receipt create --amount 5000 --category-code C001 --supplier-code S001
  dws finance receipt create --amount 3000 --tax 300 \
    --invoices '[{"invoiceNo":"12345","invoiceCode":"ABC"}]'
Flags:
      --amount string          金额 (必填)
      --category-code string   收支类别编码 (可选)
      --supplier-code string   供应商编码 (可选)
      --tax string             税额 (可选)
      --invoices string        发票列表 JSON 数组 (可选)
```
MCP 工具: `skill_generate_receipt`；参数: amount, categoryCode, supplierCode, taxationMoney, invoiceInfoList。

### 基于银行交易明细创建收款单
```
Usage:
  dws finance receipt create-collection [flags]
Example:
  dws finance receipt create-collection --amount 5000 --title "货款收入" --detail-id DET001
  dws finance receipt create-collection --amount 10000 --title "服务费" --detail-id DET002 \
    --customer-code CUST01 --account-code ACCT01 \
    --record-time "2025-07-01 10:00:00"
Flags:
      --amount string          收款金额 (必填)
      --title string           单据标题 (必填)
      --detail-id string       银行交易明细 ID (必填)
      --record-time string     记账时间 yyyy-MM-dd HH:mm:ss (可选)
      --customer-code string   客户编码 (可选)
      --account-code string    企业账户编码 (可选)
```
MCP 工具: `create_collection_receipt`；参数: amount, title, bankTradeDetailId, recordTime, customerCode, enterpriseAccountCode。

---

## 发票 (invoice)

### 上传发票
```
Usage:
  dws finance invoice upload [flags]
Example:
  dws finance invoice upload --url https://example.com/invoice.pdf --name "采购发票.pdf" --type pdf
  dws finance invoice upload --url https://example.com/inv.jpg --name "餐饮发票.jpg" --type jpg
Flags:
      --url string   发票文件链接 (必填)
      --name string  发票文件名称 (必填)
      --type string  发票文件类型: pdf/jpg/png 等 (必填)
```
MCP 工具: `skill_upload_invoice`；参数: invoiceUrl, fileName, fileType。上传后系统自动 OCR 识别、真伪查验、查重、合规检查，返回发票详情。

### 开具发票（旧版入口）
```
Usage:
  dws finance invoice issue [flags]
Example:
  dws finance invoice issue --purchaser "某某公司" --taxnum 91110000 --invoice-type 9 \
    --products '[{"productName":"咨询服务","quantity":"1","unit":"100","amountIncludeTax":"100","taxSign":"1"}]'
Flags:
      --purchaser string      购方抬头 (可选)
      --taxnum string         购方税号 (可选)
      --invoice-type string   发票类型: 8=数电专票, 9=数电普票 (默认 9)
      --products string       商品信息 JSON 数组 (可选)
```
MCP 工具: `skill_issue_invoice`；参数: purchaser, taxnum, invoiceType (数字), products。
> 推荐使用 `digital-invoice issue` 走数电流程；此命令保留向前兼容。

### 查询开票结果
```
Usage:
  dws finance invoice issue-result [flags]
Example:
  dws finance invoice issue-result --order-id ORD123456
Flags:
      --order-id string   订单编号 (必填)
```
MCP 工具: `skill_query_invoice_issue_result`；参数: orderId。

### AI 推荐发票收支类别
```
Usage:
  dws finance invoice recommend-category [flags]
Example:
  dws finance invoice recommend-category \
    --items '[{"requestId":"req1","companyIndexId":"idx1"}]'
Flags:
      --items string   发票列表 JSON 数组，元素含 requestId 和 companyIndexId (必填)
```
MCP 工具: `recommend_category_form_invoice`；参数: invoiceList。

### 查询开票申请列表
```
Usage:
  dws finance invoice list-application [flags]
Example:
  dws finance invoice list-application --page-no 1 --page-size 20
  dws finance invoice list-application --page-no 1 --page-size 20 \
    --start-time "2025-01-01 00:00:00" --end-time "2025-07-01 23:59:59"
Flags:
      --page-no float      页码 (默认 1)
      --page-size float    分页大小 (默认 20)
      --start-time string  筛选开始时间 (可选)
      --end-time string    筛选结束时间 (可选)
```
MCP 工具: `list_invoice_application`；参数: pageNo, pageSize, startTime, endTime。

### 添加发票到审批单
```
Usage:
  dws finance invoice add-record [flags]
Example:
  dws finance invoice add-record --business-id BIZ123456 \
    --invoice-pdf-url https://example.com/invoice.pdf
Flags:
      --business-id string       审批编号 (必填)
      --invoice-pdf-url string   发票 PDF 文件 URL (必填)
```
MCP 工具: `add_invoice_record_to_receipt`；参数: businessId, invoicePdfUrl。将发票 PDF 添加到审批单评论区。

---

## 银行交易明细 (bank)

### 录入银行交易明细
```
Usage:
  dws finance bank create [flags]
Example:
  dws finance bank create \
    --trade-time "2025-07-01 10:00:00" --amount 50000 --in-out-flag C \
    --my-name "我方公司" --my-account 622001234 \
    --other-name "供应商公司" --other-account 622009876 \
    --my-bank "招商银行"
Flags:
      --trade-time string       交易时间 yyyy-MM-dd HH:mm:ss (必填)
      --amount string           交易金额 (必填)
      --in-out-flag string      收入支出标识: C=收入, D=支出 (必填)
      --my-name string          当前账户户名 (必填)
      --my-account string       当前账户账号 (必填)
      --other-name string       对方账户户名 (必填)
      --other-account string    对方账户账号 (必填)
      --my-bank string          当前账户银行名称 (必填)
      --trade-no string         交易流水号 (可选)
      --balance string          余额 (可选)
      --my-account-id string    智能财务企业账户 ID (可选)
      --usage string            用途 (可选)
      --remark string           备注 (可选)
      --other-bank string       对方账户银行名称 (可选)
      --other-branch string     对方账户支行名称 (可选)
      --skip-check-repeat bool  跳过重复项校验 (默认 false)
```
MCP 工具: `create_bank_trade_detail`；参数: gmtTradeStr, tradeAmount, inOutFlag, myAccountName, myAccountNo, otherAccountName, otherAccountNo, myBankName, tradeNo, balance, myAccountId, usage, remark, otherBankName, otherBranchName, skipCheckRepeat。

### 查询银行交易明细
```
Usage:
  dws finance bank query [flags]
Example:
  dws finance bank query --detail-id 123456
Flags:
      --detail-id string   交易明细 ID (必填)
```
MCP 工具: `query_bank_trade_detail`；参数: detailId。

---

## 会计凭证 (voucher)

### 根据审批单生成会计分录
```
Usage:
  dws finance voucher entries [flags]
Example:
  dws finance voucher entries --instance-id INST123456
Flags:
      --instance-id string   审批单实例 ID (必填)
```
MCP 工具: `get_voucher_entries_approval`；参数: instanceId。返回借贷分录列表，每条含科目名称、科目代码、辅助核算代码、金额。

### 根据审批单据号生成会计凭证
```
Usage:
  dws finance voucher generate [flags]
Example:
  dws finance voucher generate --biz-id BIZ123456
Flags:
      --biz-id string   审批单据号 (必填)
```
MCP 工具: `get_voucher_by_approval_no`；参数: bizId。

---

## 客户管理 (customer)

### 查询客户列表
```
Usage:
  dws finance customer list [flags]
Example:
  dws finance customer list --page-size 20 --page-index 1
  dws finance customer list --query "阿里"
Flags:
      --query string       客户名称关键字 (可选)
      --page-size float    分页大小 (默认 20)
      --page-index float   分页页码 (默认 1)
```
MCP 工具: `list_customer`；参数: pageSize, pageIndex, keyword（对应 --page-size/--page-index/--query）。
> `--keyword` 为 `--query` 的别名（隐藏），两者等价。

### 精确查询客户详情
```
Usage:
  dws finance customer get [flags]
Example:
  dws finance customer get --name "阿里巴巴" --corp-id ding12345
Flags:
      --name string      客户名称 (必填，精确匹配)
      --corp-id string   组织 ID (必填)
```
MCP 工具: `query_customer_info`；参数: customerName, corpId。

### 保存（创建）客户档案
```
Usage:
  dws finance customer save [flags]
Example:
  dws finance customer save \
    --customer-name "阿里巴巴（中国）有限公司" \
    --purchaser-name "阿里巴巴（中国）有限公司" \
    --tax-no 91330100799655772X
Flags:
      --customer-name string    客户名称，用于档案展示 (必填)
      --purchaser-name string   发票抬头，开票时填入发票 (必填)
      --tax-no string           税号 (必填)
```
MCP 工具: `save_customer`；参数: customerName, purchaserName, taxNo。

---

## 企业账户 (account)

### 查询企业账户列表
```
Usage:
  dws finance account list [flags]
Example:
  dws finance account list --page-size 20 --page-index 1
  dws finance account list --query "招商"
  dws finance account list --account-no 622001234
Flags:
      --query string        账户名称关键字 (可选)
      --account-no string   账号筛选 (可选)
      --page-size float     分页大小 (默认 20)
      --page-index float    分页页码 (默认 1)
```
MCP 工具: `list_enterprise_account`；参数: pageSize, pageIndex, keyword, accountNo。

---

## 现金日报 (journal)

### 查询指定日期现金日报
```
Usage:
  dws finance journal daily [flags]
Example:
  dws finance journal daily --date 2025-07-01
Flags:
      --date string   统计日期 yyyy-MM-dd (必填)
```
MCP 工具: `query_daily_cashflow_journal`；参数: date。返回当日期初余额、收入合计、支出合计、期末余额、按账户分组汇总。

### 获取现金日报明细页面链接
```
Usage:
  dws finance journal detail-url [flags]
Example:
  dws finance journal detail-url
```
MCP 工具: `get_daily_cashflow_detail_url`。返回可在浏览器打开的钉钉财务端页面，供用户查看完整交易明细。

---

## 供应商 (supplier)

### 搜索供应商
```
Usage:
  dws finance supplier search [flags]
Example:
  dws finance supplier search --query "华为"
  dws finance supplier search --query "顺丰" --page-size 50 --page-index 1
Flags:
      --query string       搜索关键词 (可选)
      --page-size float    页大小 (默认 20)
      --page-index float   页号 (默认 1)
```
MCP 工具: `search_supplier`；参数: keyword, pageSize, pageIndex。返回字段含 `code`（或 `supplierCode`），可用于 `receipt create --supplier-code`。

---

## 收支类别 (category)

### 搜索收支类别
```
Usage:
  dws finance category search [flags]
Example:
  dws finance category search --type expense --query "差旅"
  dws finance category search --type income --query "销售"
Flags:
      --type string         收支类别类型: income=收入, expense=支出 (必填)
      --query string        搜索关键词 (可选)
      --page-size string    页大小 (可选)
      --page-index string   页号 (可选)
```
MCP 工具: `search_category`；参数: type, keyword, pageSize, pageIndex。返回字段含 `code`（或 `categoryCode`），可用于 `receipt create --category-code`。

---

## 支付 / 付款 (payment)

### 发起付款
```
Usage:
  dws finance payment create [flags]
Example:
  dws finance payment create --amount 10000 \
    --payee-account-no 622001234 --payee-account-type corporate \
    --payee-account-name "收款方公司" --payee-bank-name "招商银行"
Flags:
      --amount string              付款金额 (必填)
      --payee-account-no string    收款账户卡号 (必填)
      --payee-account-type string  收款账户类型 (必填)
      --payee-account-name string  收款账户户名 (必填)
      --payee-bank-name string     收款银行名称 (可选)
      --payee-branch-name string   收款账户支行名称 (可选)
      --remark string              备注 (可选)
```
MCP 工具: `create_payment`。
> **CAUTION:** 真实资金操作 — 执行前必须向用户确认金额与收款账户信息。

### 查询付款单列表
```
Usage:
  dws finance payment list [flags]
Example:
  dws finance payment list --page-no 1 --page-size 20
  dws finance payment list --payee-account-no 622001234
Flags:
      --page-no float            页码 (默认 1)
      --page-size float          分页大小 (默认 20)
      --payee-account-no string  收款账户筛选 (可选)
```

### 查询付款账户列表
```
Usage:
  dws finance payment account-list [flags]
Example:
  dws finance payment account-list --page-no 1 --page-size 20
  dws finance payment account-list --query "招商"
Flags:
      --page-no float     页码 (默认 1)
      --page-size float   分页大小 (默认 20)
      --query string      搜索关键字 (可选)
```

### 获取付款收银台链接
```
Usage:
  dws finance payment cashier-url [flags]
Example:
  dws finance payment cashier-url --instance-id INST001
  dws finance payment cashier-url --instance-ids INST001,INST002
Flags:
      --instance-id string    单据 ID（单笔付款）(二选一)
      --instance-ids string   单据 ID 列表（合并多笔，逗号分隔）(二选一)
```
> `--instance-id` 与 `--instance-ids` 二选一；前者单笔付款，后者批量合并付款。

### 获取账户管理页 / 付款人列表
```
dws finance payment account-url     # 返回账户管理页面链接
dws finance payment payer-list      # 查询可选付款人列表
```

---

## 主体 (company)

### 搜索主体
```
Usage:
  dws finance company search [flags]
Example:
  dws finance company search
  dws finance company search --name "阿里"
Flags:
      --name string   主体名称关键词（模糊搜索，为空返回全部）(可选)
```
MCP 工具: `search_company`；参数: corpName。

### 新增主体
```
Usage:
  dws finance company save [flags]
Example:
  dws finance company save --name "XX 科技有限公司" --tax-no 91330100ABCDEFGHIJ
Flags:
      --name string     主体名称 (必填)
      --tax-no string   税号 (必填)
```
MCP 工具: `save_company`；参数: corpName, taxNo。

### 修改主体信息
```
Usage:
  dws finance company update [flags]
Example:
  dws finance company update --code COMP001 --name "新公司名称" --tax-no 91110000123456789X
Flags:
      --code string     主体编号 (必填)
      --name string     主体名称 (必填)
      --tax-no string   税号 (必填)
```
MCP 工具: `update_company`；参数: code, corpName, taxNo。

---

## 数电发票 (digital-invoice)

> 数电发票是税务局主推的新型发票，需先完成"数电登录认证"获得开票资质。
> 典型流程：`do-login-status` → `do-login`（或 `login-page`）→ `sms-code`（验证码登录时）→ `face-qr` + `face-status`（人脸核验）→ `issue` → `file`。

### 查询登录状态
```
Usage:
  dws finance digital-invoice do-login-status [flags]
Example:
  dws finance digital-invoice do-login-status --company-code COMP001
Flags:
      --company-code string   主体编码 (必填)
```
MCP 工具: `query_invoice_login_status`；参数: companyCode。

### 获取登录页面参数
```
Usage:
  dws finance digital-invoice login-page [flags]
Example:
  dws finance digital-invoice login-page --company-code COMP001 \
    --company-name "XX 公司" --tax-no 91110000123456789X
Flags:
      --company-code string   主体编码 (必填)
      --company-name string   主体名称 (必填)
      --tax-no string         主体税号 (必填)
```
MCP 工具: `query_invoice_login_page`。

### 数电登录认证
```
Usage:
  dws finance digital-invoice do-login [flags]
Example:
  dws finance digital-invoice do-login --company-code COMP001 \
    --login-account 13800000000 --taxpayer-user-id 110101199001011234 \
    --login-id 01 --login-pwd "password" \
    --taxpayer-user "张三" --taxpayer-user-phone 13800000000 --serial-no SN001
Flags:
      --company-code string          主体编码 (必填)
      --login-account string         登录账号 (必填)
      --taxpayer-user-id string      办税人员身份证件号 (必填)
      --login-id string              登录身份 (必填)
      --login-pwd string             登录密码 (必填)
      --taxpayer-user string         办税人员姓名 (必填)
      --taxpayer-user-phone string   办税人员手机号 (必填)
      --serial-no string             流水号 (必填)
```
MCP 工具: `do_invoice_login`。支持账号密码 / 手机验证码两种方式，验证码方式需配合 `sms-code` 上传验证码完成。

### 查询登录所需账号信息
```
dws finance digital-invoice account --company-code COMP001
```
MCP 工具: `query_invoice_login_account_info`；参数: companyCode。

### 上传短信验证码
```
Usage:
  dws finance digital-invoice sms-code [flags]
Example:
  dws finance digital-invoice sms-code --company-code COMP001 \
    --serial-no SN001 --sms-code 123456 --phone 13800000000
Flags:
      --company-code string   主体编码 (必填)
      --serial-no string      流水号 (必填)
      --sms-code string       手机验证码 (必填)
      --phone string          办税人手机号 (必填)
```
MCP 工具: `invoice_login_sms_code`。

### 查询商品编码
```
dws finance digital-invoice goods-code --company-code COMP001 --good-name "咨询服务"
```
MCP 工具: `query_invoice_goods_code`。返回可用于开票的 `revenueCode`（商品和服务税收分类编码）。

### 生成人脸识别二维码
```
Usage:
  dws finance digital-invoice face-qr [flags]
Example:
  dws finance digital-invoice face-qr --company-code COMP001 --id-auth-type 0
Flags:
      --company-code string    主体编码 (必填)
      --id-auth-type string    身份认证人脸识别类型: 0=税务App, 1=个税App (必填)
```
MCP 工具: `query_invoice_face_qr`。扫码后用户在税务 App 完成活体认证。

### 查询人脸识别结果
```
dws finance digital-invoice face-status --company-code COMP001
```
MCP 工具: `query_invoice_face_status`。

### 搜索发票抬头
```
Usage:
  dws finance digital-invoice title [flags]
Example:
  dws finance digital-invoice title --company-code COMP001 --name "阿里"
Flags:
      --company-code string   主体编码 (必填)
      --name string           购方公司名称关键词 (必填)
```
MCP 工具: `query_invoice_title`。返回购方信息供开票时 `customerCode` 使用。

### 开具数电发票
```
Usage:
  dws finance digital-invoice issue [flags]
Example:
  dws finance digital-invoice issue --company-code COMP001 --serial-no SN001 \
    --invoice-type-code 026 --customer-code CUST001 \
    --total-exclude-tax 1000 --total-tax-amount 130 --total-include-tax 1130 \
    --details '[{"amount":"1000","taxAmount":"130","taxRate":"0.13"}]'
Flags:
      --company-code string        主体编码 (必填)
      --serial-no string           流水号 (必填)
      --invoice-type-code string   发票类型代码 (必填)
      --customer-code string       购方编码 (必填)
      --total-exclude-tax string   不含税总金额 (必填)
      --total-tax-amount string    税额合计 (必填)
      --total-include-tax string   含税总金额 (必填)
      --details string             明细列表 JSON 数组 (必填)
```
MCP 工具: `invoice_do_invoice`。

### 查询发票文件下载地址
```
Usage:
  dws finance digital-invoice file [flags]
Example:
  dws finance digital-invoice file --serial-no SN001 \
    --drew-date "2025-07-01" --invoice-no 25110000000000000001
Flags:
      --serial-no string    流水号 (必填)
      --drew-date string    开票日期 (必填)
      --invoice-no string   发票号码 (必填)
```
MCP 工具: `query_invoice_file`。返回 PDF/OFD 下载链接，可供用户下载或作为 `invoice add-record` 的入参。

### 查询开票能力版本
```
dws finance digital-invoice skill-version
```
MCP 工具: `query_invoice_skill_version`。返回当前企业开通的数电发票能力版本（轻量化 / SaaS），用于决定用 `batch-draw` 还是 `batch-draw-saas`。

### 发送发票邮件（轻量化版）
```
Usage:
  dws finance digital-invoice send-email [flags]
Example:
  dws finance digital-invoice send-email --company-code COMP001 \
    --items '[{"email":"a@b.com","items":[{"serialNo":"SN001","invoiceNo":"25110000000000000001"}]}]'
Flags:
      --company-code string   主体编码 (必填)
      --items string          发送邮件列表 JSON 数组，每项含 email 和 items (必填)
```
MCP 工具: `invoice_send_email`。

### 发送发票邮件（SaaS 版）
```
dws finance digital-invoice send-email-saas --company-code COMP001 --items '[...]'
```
MCP 工具: `invoice_send_email_saas`。

### 批量开票（轻量化版）
```
Usage:
  dws finance digital-invoice batch-draw [flags]
Example:
  dws finance digital-invoice batch-draw --company-code COMP001 \
    --items '[{"invoiceTypeCode":"026","customerCode":"CUST001","details":[{"amountIncludeTax":"1000","taxRate":"0.13","revenueCode":"3040201","itemTitle":"咨询服务"}]}]'
Flags:
      --company-code string   主体编码 (必填)
      --items string          批量开票项列表 JSON 数组 (必填)
```
MCP 工具: `invoice_batch_draw`。适用于无发票权益的组织。

### 查询批量开票进度（轻量化版）
```
dws finance digital-invoice batch-draw-query --company-code COMP001 --batch-no BATCH001
```
MCP 工具: `invoice_batch_draw_query`。

### 批量开票（SaaS 版）
```
Usage:
  dws finance digital-invoice batch-draw-saas [flags]
Example:
  dws finance digital-invoice batch-draw-saas --company-code COMP001 --batch-no BATCH001 \
    --orders '[{"orderId":"ORD001","invoiceType":"9","customerCode":"CUST001","products":[{"productName":"咨询服务","amountWithTax":"1130","revenueCode":"3040201"}]}]'
Flags:
      --company-code string   主体编码 (必填)
      --batch-no string       批次号 (必填)
      --orders string         订单账单列表 JSON 数组 (必填)
```
MCP 工具: `invoice_batch_draw_saas`。适用于有发票权益的组织。

### 查询批量开票进度（SaaS 版）
```
dws finance digital-invoice batch-draw-query-saas --batch-no BATCH001
```
MCP 工具: `invoice_batch_draw_query_saas`。

---

## 审批单 (process)

### 根据审批编号查询审批表单信息
```
Usage:
  dws finance process form-data [flags]
Example:
  dws finance process form-data --business-id BIZ123456 --format json
Flags:
      --business-id string   审批编号 (必填)
```
MCP 工具: `query_process_form_data`；参数: businessId。

### 根据审批模版名查询审批单列表
```
Usage:
  dws finance process list [flags]
Example:
  dws finance process list --form-name "付款审批" \
    --start-time "2025-01-01" --end-time "2025-07-01" \
    --page-no 1 --page-size 20 --format json
Flags:
      --form-name string    审批模版名称 (必填)
      --start-time string   开始时间 (必填)
      --end-time string     结束时间 (必填)
      --page-no float       页码 (默认 1)
      --page-size float     分页大小 (默认 20)
```
MCP 工具: `list_process_by_form_name`；参数: formName, startTime, endTime, pageNo, pageSize。

---

## 典型调用链

### 1. 标准报销流程（命令行）
```
supplier search  →  取 code
category search  →  取 code
receipt create --amount --supplier-code --category-code --tax --invoices
```
对应 Python 封装：`scripts/finance_expense_flow.py`。

### 2. 收款入账流程
```
bank create      →  拿到银行交易明细 (detail-id)
receipt create-collection --amount --title --detail-id [--customer-code --account-code]
voucher entries --instance-id            # 查询生成的借贷分录
```

### 3. 数电开票流程
```
do-login-status --company-code           # 登录态存在则跳过 2-4
login-page                               # 获取登录页面参数
do-login / sms-code                      # 完成登录认证
face-qr + face-status                    # 首次开票或失效时做人脸核验
title --name                             # 定位 customerCode
goods-code --good-name                   # 定位 revenueCode
issue / batch-draw[-saas]                # 开票
file --serial-no --drew-date --invoice-no  # 获取 PDF/OFD 链接
send-email[-saas]                        # 可选：邮件发送
```

### 4. 审批单开票（从审批单出发）
```
process list --form-name --start-time --end-time
  → 拿到 businessId
invoice add-record --business-id --invoice-pdf-url
  → 把发票 PDF 挂到审批单评论区
voucher generate --biz-id                 # 审批单据号生成凭证
```

### 5. 现金日报查看
```
journal daily --date           # 结构化数据
journal detail-url             # 明细页面链接（浏览器打开）
```
对应 Python 封装：`scripts/finance_daily_cashflow.py`。

---

## 字段传递规则

| 来源命令 | 字段 | 传递给 |
|----------|------|--------|
| `supplier search` | `code` / `supplierCode` | `receipt create --supplier-code` |
| `category search` | `code` / `categoryCode` | `receipt create --category-code` |
| `customer list` / `customer get` | `customerCode` | `receipt create-collection --customer-code`、`digital-invoice issue --customer-code` |
| `account list` | `accountCode` | `receipt create-collection --account-code` |
| `bank create` | `detailId` | `receipt create-collection --detail-id` |
| `company search` | `code` | `digital-invoice *` 的 `--company-code`、`company update --code` |
| `digital-invoice title` | `customerCode` | `digital-invoice issue --customer-code` |
| `digital-invoice goods-code` | `revenueCode` | `digital-invoice batch-draw` / `batch-draw-saas` 的 details 中 |
| `digital-invoice issue` | `serialNo`、`invoiceNo`、`drewDate` | `digital-invoice file` |
| `process list` | `businessId` | `process form-data`、`invoice add-record --business-id` |
| `voucher entries` | `instanceId` | `voucher generate --biz-id`（审批单据号） |

---

## 风险与注意事项

- `payment create` 触发真实资金转账，属于不可逆操作，**执行前必须向用户显式确认**金额与收款账户。
- `receipt create` 本身只生成单据，不直接发起资金流转；资金动作仍需走 `payment` 或审批流。
- `digital-invoice issue` 一旦成功即产生真实税务记录，作废需联系税局专门流程，CLI 不覆盖作废能力。
- `bank create` 默认校验重复项，重复明细会报错；确认业务允许重复再加 `--skip-check-repeat`。
- 跨产品协作：
  - 审批单状态流转（同意 / 拒绝 / 撤销）→ 切到 `dingtalk-oa`
  - 把发票 / 凭证文件落钉盘 → 切到 `dingtalk-drive`
  - 把开票汇总 / 现金日报写文档 → 切到 `dingtalk-doc`
  - 发票邮件发送仅限数电开票后触发；一般邮件任务 → 切到 `dingtalk-mail`

---

## 与 MCP 工具映射速查

| MCP 工具名 | CLI 命令 |
|-----------|----------|
| `skill_generate_receipt` | `receipt create` |
| `create_collection_receipt` | `receipt create-collection` |
| `skill_upload_invoice` | `invoice upload` |
| `skill_issue_invoice` | `invoice issue` |
| `skill_query_invoice_issue_result` | `invoice issue-result` |
| `recommend_category_form_invoice` | `invoice recommend-category` |
| `list_invoice_application` | `invoice list-application` |
| `add_invoice_record_to_receipt` | `invoice add-record` |
| `create_bank_trade_detail` | `bank create` |
| `query_bank_trade_detail` | `bank query` |
| `get_voucher_entries_approval` | `voucher entries` |
| `get_voucher_by_approval_no` | `voucher generate` |
| `list_customer` | `customer list` |
| `query_customer_info` | `customer get` |
| `save_customer` | `customer save` |
| `list_enterprise_account` | `account list` |
| `query_daily_cashflow_journal` | `journal daily` |
| `get_daily_cashflow_detail_url` | `journal detail-url` |
| `search_supplier` | `supplier search` |
| `search_category` | `category search` |
| `create_payment` | `payment create` |
| `search_company` / `save_company` / `update_company` | `company search/save/update` |
| `query_invoice_login_status` | `digital-invoice do-login-status` |
| `query_invoice_login_page` | `digital-invoice login-page` |
| `do_invoice_login` | `digital-invoice do-login` |
| `query_invoice_login_account_info` | `digital-invoice account` |
| `invoice_login_sms_code` | `digital-invoice sms-code` |
| `query_invoice_goods_code` | `digital-invoice goods-code` |
| `query_invoice_face_qr` / `query_invoice_face_status` | `digital-invoice face-qr/face-status` |
| `query_invoice_title` | `digital-invoice title` |
| `invoice_do_invoice` | `digital-invoice issue` |
| `query_invoice_file` | `digital-invoice file` |
| `query_invoice_skill_version` | `digital-invoice skill-version` |
| `invoice_send_email` / `invoice_send_email_saas` | `digital-invoice send-email[-saas]` |
| `invoice_batch_draw` / `invoice_batch_draw_saas` | `digital-invoice batch-draw[-saas]` |
| `invoice_batch_draw_query` / `invoice_batch_draw_query_saas` | `digital-invoice batch-draw-query[-saas]` |
| `query_process_form_data` | `process form-data` |
| `list_process_by_form_name` | `process list` |
