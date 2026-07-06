package helpers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws finance — 智能财务 (17 tools)
// ──────────────────────────────────────────────────────────

func newFinanceCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "finance",
		Short: "财务 / 发票 / 凭证 / 银行",
		Long: `管理钉钉智能财务：付款单、收款单、发票、银行交易明细、会计凭证、
客户管理、企业账户、现金日报、供应商搜索、收支类别搜索。`,
		RunE: groupRunE,
	}

	// ── receipt (付款单/收款单) ──────────────────────────────────

	receiptCmd := &cobra.Command{Use: "receipt", Short: "付款单/收款单管理", RunE: groupRunE}

	receiptCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建付款单",
		Long: `在智能财务系统中创建付款单，支持指定金额、收支类别、供应商等信息。
可选附带发票列表，系统会自动关联。`,
		Example: `  dws finance receipt create --amount 1000.00
  dws finance receipt create --amount 5000 --category-code C001 --supplier-code S001
  dws finance receipt create --amount 3000 --tax 300 --invoices '[{"invoiceNo":"12345","invoiceCode":"ABC"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "amount"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"amount": mustGetFlag(cmd, "amount"),
			}
			if v := mustGetFlag(cmd, "category-code"); v != "" {
				toolArgs["categoryCode"] = v
			}
			if v := mustGetFlag(cmd, "supplier-code"); v != "" {
				toolArgs["supplierCode"] = v
			}
			if v := mustGetFlag(cmd, "tax"); v != "" {
				toolArgs["taxationMoney"] = v
			}
			if v := mustGetFlag(cmd, "invoices"); v != "" {
				var invoices []map[string]any
				if err := json.Unmarshal([]byte(v), &invoices); err != nil {
					return fmt.Errorf("--invoices JSON parse failed: %w", err)
				}
				toolArgs["invoiceInfoList"] = invoices
			}
			return callMCPTool("skill_generate_receipt", toolArgs)
		},
	}

	receiptCollectionCmd := &cobra.Command{
		Use:   "create-collection",
		Short: "基于银行交易明细创建收款单",
		Long: `基于银行交易明细创建收款单据，支持指定金额、企业账户、客户等信息。
适用于基于银行明细快速创建收款单的场景。`,
		Example: `  dws finance receipt create-collection --amount 5000 --title "货款收入" --detail-id DET001
  dws finance receipt create-collection --amount 10000 --title "服务费" --detail-id DET002 \
    --customer-code CUST01 --account-code ACCT01 --record-time "2025-07-01 10:00:00"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "amount", "title", "detail-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"amount":            mustGetFlag(cmd, "amount"),
				"title":             mustGetFlag(cmd, "title"),
				"bankTradeDetailId": mustGetFlag(cmd, "detail-id"),
			}
			if v := mustGetFlag(cmd, "record-time"); v != "" {
				toolArgs["recordTime"] = v
			}
			if v := mustGetFlag(cmd, "customer-code"); v != "" {
				toolArgs["customerCode"] = v
			}
			if v := mustGetFlag(cmd, "account-code"); v != "" {
				toolArgs["enterpriseAccountCode"] = v
			}
			return callMCPTool("create_collection_receipt", toolArgs)
		},
	}

	// ── invoice (发票) ──────────────────────────────────────────

	invoiceCmd := &cobra.Command{Use: "invoice", Short: "发票管理", RunE: groupRunE}

	invoiceUploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "上传发票",
		Long: `将发票文件上传到智能财务系统，系统自动进行 OCR 识别、真伪查验、查重检测和合规检查。
返回完整的发票详细信息。`,
		Example: `  dws finance invoice upload --url https://example.com/invoice.pdf --name "采购发票.pdf" --type pdf
  dws finance invoice upload --url https://example.com/inv.jpg --name "餐饮发票.jpg" --type jpg`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "url", "name", "type"); err != nil {
				return err
			}
			return callMCPTool("skill_upload_invoice", map[string]any{
				"invoiceUrl": mustGetFlag(cmd, "url"),
				"fileName":   mustGetFlag(cmd, "name"),
				"fileType":   mustGetFlag(cmd, "type"),
			})
		},
	}

	invoiceIssueCmd := &cobra.Command{
		Use:   "issue",
		Short: "开具发票",
		Long: `向购方开具发票。支持数电专票(类型8)和数电普票(类型9)。
商品信息通过 --products JSON 数组传入。`,
		Example: `  dws finance invoice issue --purchaser "某某公司" --taxnum 91110000 --invoice-type 9 \
    --products '[{"productName":"咨询服务","quantity":"1","unit":"100","amountIncludeTax":"100","taxSign":"1"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v := mustGetFlag(cmd, "purchaser"); v != "" {
				toolArgs["purchaser"] = v
			}
			if v := mustGetFlag(cmd, "taxnum"); v != "" {
				toolArgs["taxnum"] = v
			}
			if v := mustGetFlag(cmd, "invoice-type"); v != "" {
				toolArgs["invoiceType"] = parseInvoiceType(v)
			}
			if v := mustGetFlag(cmd, "products"); v != "" {
				var products []map[string]any
				if err := json.Unmarshal([]byte(v), &products); err != nil {
					return fmt.Errorf("--products JSON parse failed: %w", err)
				}
				toolArgs["products"] = products
			}
			return callMCPTool("skill_issue_invoice", toolArgs)
		},
	}

	invoiceIssueResultCmd := &cobra.Command{
		Use:     "issue-result",
		Short:   "查询开票结果",
		Example: `  dws finance invoice issue-result --order-id ORD123456`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "order-id"); err != nil {
				return err
			}
			return callMCPToolUnescaped("skill_query_invoice_issue_result", map[string]any{
				"orderId": mustGetFlag(cmd, "order-id"),
			})
		},
	}

	invoiceRecommendCategoryCmd := &cobra.Command{
		Use:   "recommend-category",
		Short: "AI 推荐发票收支类别",
		Long: `根据已识别的发票，由 AI 自动推荐匹配的收支类别，省去手动搜索选择的步骤。
输入为 JSON 数组，每个元素包含 requestId 和 companyIndexId。`,
		Example: `  dws finance invoice recommend-category --items '[{"requestId":"req1","companyIndexId":"idx1"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "items"); err != nil {
				return err
			}
			raw := mustGetFlag(cmd, "items")
			var items []map[string]any
			if err := json.Unmarshal([]byte(raw), &items); err != nil {
				return fmt.Errorf("--items JSON parse failed: %w", err)
			}
			return callMCPTool("recommend_category_form_invoice", map[string]any{
				"invoiceList": items,
			})
		},
	}

	// ── bank (银行交易明细) ─────────────────────────────────────

	bankCmd := &cobra.Command{Use: "bank", Short: "银行交易明细", RunE: groupRunE}

	bankCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "录入银行交易明细",
		Long: `将银行交易明细上传到智能财务系统。

--in-out-flag: 收入支出标识
  C = 收入
  D = 支出`,
		Example: `  dws finance bank create \
    --trade-time "2025-07-01 10:00:00" --amount 50000 --in-out-flag C \
    --my-name "我方公司" --my-account 622001234 \
    --other-name "供应商公司" --other-account 622009876 \
    --my-bank "招商银行"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "trade-time", "amount", "in-out-flag", "my-name", "my-account", "other-name", "other-account", "my-bank"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"gmtTradeStr":      mustGetFlag(cmd, "trade-time"),
				"tradeAmount":      mustGetFlag(cmd, "amount"),
				"inOutFlag":        mustGetFlag(cmd, "in-out-flag"),
				"myAccountName":    mustGetFlag(cmd, "my-name"),
				"myAccountNo":      mustGetFlag(cmd, "my-account"),
				"otherAccountName": mustGetFlag(cmd, "other-name"),
				"otherAccountNo":   mustGetFlag(cmd, "other-account"),
				"myBankName":       mustGetFlag(cmd, "my-bank"),
			}
			if v := mustGetFlag(cmd, "trade-no"); v != "" {
				toolArgs["tradeNo"] = v
			}
			if v := mustGetFlag(cmd, "balance"); v != "" {
				toolArgs["balance"] = v
			}
			if v := mustGetFlag(cmd, "my-account-id"); v != "" {
				toolArgs["myAccountId"] = v
			}
			if v := mustGetFlag(cmd, "usage"); v != "" {
				toolArgs["usage"] = v
			}
			if v := mustGetFlag(cmd, "remark"); v != "" {
				toolArgs["remark"] = v
			}
			if v := mustGetFlag(cmd, "other-bank"); v != "" {
				toolArgs["otherBankName"] = v
			}
			if v := mustGetFlag(cmd, "other-branch"); v != "" {
				toolArgs["otherBranchName"] = v
			}
			if v, _ := cmd.Flags().GetBool("skip-check-repeat"); v {
				toolArgs["skipCheckRepeat"] = true
			}
			return callMCPTool("create_bank_trade_detail", toolArgs)
		},
	}

	bankQueryCmd := &cobra.Command{
		Use:     "query",
		Short:   "查询银行交易明细",
		Example: `  dws finance bank query --detail-id 123456`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "detail-id"); err != nil {
				return err
			}
			return callMCPTool("query_bank_trade_detail", map[string]any{
				"detailId": mustGetFlag(cmd, "detail-id"),
			})
		},
	}

	bankListCmd := &cobra.Command{
		Use:   "list",
		Short: "分页查询银行交易明细",
		Long:  `按企业账户和交易时间范围分页查询银行交易明细，适用于查询账户某段时间内交易流水的场景。`,
		Example: `  dws finance bank list --account-id ACCT001 --page-no 1 --page-size 20 \
    --trade-start "2025-07-01 00:00:00" --trade-end "2025-07-31 23:59:59"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "account-id", "page-no", "page-size", "trade-start", "trade-end"); err != nil {
				return err
			}
			return callMCPTool("page_query_bank_trade_detail", map[string]any{
				"accountId":     mustGetFlag(cmd, "account-id"),
				"pageNo":        mustGetFlag(cmd, "page-no"),
				"pageSize":      mustGetFlag(cmd, "page-size"),
				"gmtTradeStart": mustGetFlag(cmd, "trade-start"),
				"gmtTradeEnd":   mustGetFlag(cmd, "trade-end"),
			})
		},
	}

	// ── voucher (会计凭证) ──────────────────────────────────────

	voucherCmd := &cobra.Command{Use: "voucher", Short: "会计凭证", RunE: groupRunE}

	voucherEntriesCmd := &cobra.Command{
		Use:   "entries",
		Short: "根据审批单生成会计分录",
		Long: `根据钉钉审批单实例，自动生成标准会计分录。
返回借贷会计分录列表，每条包含科目名称、科目代码、辅助核算代码列表及金额。`,
		Example: `  dws finance voucher entries --instance-id INST123456`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instance-id"); err != nil {
				return err
			}
			return callMCPTool("get_voucher_entries_approval", map[string]any{
				"instanceId": mustGetFlag(cmd, "instance-id"),
			})
		},
	}

	voucherGenerateCmd := &cobra.Command{
		Use:     "generate",
		Short:   "根据审批单据号生成会计凭证",
		Example: `  dws finance voucher generate --biz-id BIZ123456`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "biz-id"); err != nil {
				return err
			}
			return callMCPTool("get_voucher_by_approval_no", map[string]any{
				"bizId": mustGetFlag(cmd, "biz-id"),
			})
		},
	}

	// ── customer (客户管理) ─────────────────────────────────────

	customerCmd := &cobra.Command{Use: "customer", Short: "客户管理", RunE: groupRunE}

	customerListCmd := &cobra.Command{
		Use:   "list",
		Short: "分页查询客户列表",
		Long:  `分页查询智能财务系统维护的客户列表，支持根据客户名称模糊匹配。`,
		Example: `  dws finance customer list --page-size 20 --page-index 1
  dws finance customer list --query "科技" --page-size 10 --page-index 1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pageSize, _ := cmd.Flags().GetFloat64("page-size")
			pageIndex, _ := cmd.Flags().GetFloat64("page-index")
			toolArgs := map[string]any{
				"pageSize":  pageSize,
				"pageIndex": pageIndex,
			}
			if v := flagOrFallback(cmd, "query", "keyword"); v != "" {
				toolArgs["keyWord"] = v
			}
			return callMCPTool("page_query_customer_list", toolArgs)
		},
	}

	customerGetCmd := &cobra.Command{
		Use:     "get",
		Short:   "根据名称精确查询客户",
		Example: `  dws finance customer get --name "某某科技有限公司" --corp-id CORP001`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "name", "corp-id"); err != nil {
				return err
			}
			return callMCPTool("query_customer_by_name", map[string]any{
				"CustomerQuery": map[string]any{
					"corpId": mustGetFlag(cmd, "corp-id"),
					"name":   mustGetFlag(cmd, "name"),
				},
			})
		},
	}

	// ── account (企业账户) ──────────────────────────────────────

	accountCmd := &cobra.Command{Use: "account", Short: "企业账户管理", RunE: groupRunE}

	accountListCmd := &cobra.Command{
		Use:   "list",
		Short: "分页查询企业账户列表",
		Long:  `分页查询智能财务系统中维护的企业账户列表，支持根据账户名称模糊匹配或按账号筛选。`,
		Example: `  dws finance account list --page-size 20 --page-index 1
  dws finance account list --query "招商" --page-size 10 --page-index 1
  dws finance account list --account-no 622001234 --page-size 10 --page-index 1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pageSize, _ := cmd.Flags().GetFloat64("page-size")
			pageIndex, _ := cmd.Flags().GetFloat64("page-index")
			toolArgs := map[string]any{
				"pageSize":  pageSize,
				"pageIndex": pageIndex,
			}
			if v := flagOrFallback(cmd, "query", "keyword"); v != "" {
				toolArgs["keyword"] = v
			}
			if v := mustGetFlag(cmd, "account-no"); v != "" {
				toolArgs["accountNo"] = v
			}
			return callMCPTool("page_query_enterprise_account", toolArgs)
		},
	}

	// ── journal (现金日报) ──────────────────────────────────────

	journalCmd := &cobra.Command{Use: "journal", Short: "现金日报", RunE: groupRunE}

	journalDailyCmd := &cobra.Command{
		Use:   "daily",
		Short: "按日查询现金日报",
		Long: `查询指定企业在某一天的现金日报汇总数据。
汇总当日所有银行账户的收支流水，计算各账户的收入、支出及余额。`,
		Example: `  dws finance journal daily --date 2025-07-01`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "date"); err != nil {
				return err
			}
			return callMCPTool("get_fund_journal_by_day", map[string]any{
				"statDate": mustGetFlag(cmd, "date"),
			})
		},
	}

	journalDetailURLCmd := &cobra.Command{
		Use:     "detail-url",
		Short:   "获取现金日报明细链接",
		Long:    `获取现金日报的明细页面链接，用于跳转查看详细的现金日报。`,
		Example: `  dws finance journal detail-url`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("get_fund_journal_detail_url", nil)
		},
	}

	// ── supplier (供应商) ───────────────────────────────────────

	supplierCmd := &cobra.Command{Use: "supplier", Short: "供应商管理", RunE: groupRunE}

	supplierSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "模糊搜索供应商",
		Long: `按关键词模糊搜索当前用户可见的供应商列表，可用于创建付款单时选取供应商编码。
搜索范围：供应商名称、自定义编码、联系人（三者任一匹配均返回）。
query 为空时全量分页返回。`,
		Example: `  dws finance supplier search --query "华为"
  dws finance supplier search --query "科技" --page-size 10 --page-index 1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v := flagOrFallback(cmd, "query", "keyword"); v != "" {
				toolArgs["keyword"] = v
			}
			if pageSize, _ := cmd.Flags().GetFloat64("page-size"); pageSize > 0 {
				toolArgs["pageSize"] = pageSize
			}
			if pageIndex, _ := cmd.Flags().GetFloat64("page-index"); pageIndex > 0 {
				toolArgs["pageIndex"] = pageIndex
			}
			return callMCPTool("search_supplier", toolArgs)
		},
	}

	// ── category (收支类别) ─────────────────────────────────────

	categoryCmd := &cobra.Command{Use: "category", Short: "收支类别管理", RunE: groupRunE}

	categorySearchCmd := &cobra.Command{
		Use:   "search",
		Short: "搜索收支类别",
		Long: `按关键词模糊搜索当前用户可见的收支类别，可用于创建收款单/付款单时选取类别编码。

--type: 收支类别类型
  income  = 收入类别
  expense = 支出类别`,
		Example: `  dws finance category search --type expense
  dws finance category search --type income --query "服务费"
  dws finance category search --type expense --query "差旅" --page-size 10 --page-index 1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "type"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"categoryType": mustGetFlag(cmd, "type"),
			}
			if v := flagOrFallback(cmd, "query", "keyword"); v != "" {
				toolArgs["keyword"] = v
			}
			if v := mustGetFlag(cmd, "page-size"); v != "" {
				toolArgs["pageSize"] = v
			}
			if v := mustGetFlag(cmd, "page-index"); v != "" {
				toolArgs["pageIndex"] = v
			}
			return callMCPTool("search_category", toolArgs)
		},
	}

	// ── company (主体管理) ──────────────────────────────────────

	financeCompanyCmd := &cobra.Command{Use: "company", Short: "主体管理", RunE: groupRunE}

	financeCompanySearchCmd := &cobra.Command{
		Use:   "search",
		Short: "模糊搜索主体",
		Long: `通过名称模糊搜索主体列表。
corpName 为空时返回该组织下所有生效主体；仅返回生效（未停用）的主体。`,
		Example: `  dws finance company search --name "钉钉"
  dws finance company search`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v := mustGetFlag(cmd, "name"); v != "" {
				toolArgs["corpName"] = v
			}
			return callMCPTool("search_company", toolArgs)
		},
	}

	financeCompanySaveCmd := &cobra.Command{
		Use:     "save",
		Short:   "保存主体",
		Long:    `保存主体信息，包括企业名称和税号。`,
		Example: `  dws finance company save --name "某某科技有限公司" --tax-no 91110000123456789X`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "name", "tax-no"); err != nil {
				return err
			}
			return callMCPTool("save_company", map[string]any{
				"corpName": mustGetFlag(cmd, "name"),
				"taxNo":    mustGetFlag(cmd, "tax-no"),
			})
		},
	}

	financeCompanyUpdateCmd := &cobra.Command{
		Use:     "update",
		Short:   "修改主体信息",
		Long:    `修改主体信息（名称、税号），通过 --code 定位要修改的主体。`,
		Example: `  dws finance company update --code COMP001 --name "新公司名称" --tax-no 91110000123456789X`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "code", "name", "tax-no"); err != nil {
				return err
			}
			return callMCPTool("update_company", map[string]any{
				"code":     mustGetFlag(cmd, "code"),
				"corpName": mustGetFlag(cmd, "name"),
				"taxNo":    mustGetFlag(cmd, "tax-no"),
			})
		},
	}

	// ── digital-invoice (数电发票) ──────────────────────────────

	var financeDigitalInvoiceCmd = &cobra.Command{Use: "digital-invoice", Short: "数电发票管理", RunE: groupRunE}

	var financeDigitalInvoiceLoginStatusCmd = &cobra.Command{
		Use:   "do-login-status",
		Short: "查询数电登录状态",
		Long: `查询当前用户是否已完成数电登录认证。
    未登录时需引导用户先进行登录认证（调用 do-login 命令）。`,
		Example: `  dws finance digital-invoice do-login-status --company-code COMP001`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code"); err != nil {
				return err
			}
			return callMCPTool("query_invoice_login_status", map[string]any{
				"companyCode": mustGetFlag(cmd, "company-code"),
			})
		},
	}

	var financeDigitalInvoiceLoginCmd = &cobra.Command{
		Use:   "do-login",
		Short: "数电登录认证",
		Long: `数电登录认证，支持账号密码登录和手机验证码登录两种方式。
    手机验证码登录需配合 sms-code 命令上传验证码完成认证。`,
		Example: `  dws finance digital-invoice do-login --company-code COMP001 \
        --login-account acc123 --taxpayer-user-id 110101199001011234 \
        --login-id ID001 --login-pwd "password" \
        --taxpayer-user "张三" --taxpayer-user-phone 13800138000 --serial-no SN001`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code", "login-account", "taxpayer-user-id", "login-id", "login-pwd", "taxpayer-user", "taxpayer-user-phone", "serial-no"); err != nil {
				return err
			}
			return callMCPTool("login_digital_invoice", map[string]any{
				"companyCode":       mustGetFlag(cmd, "company-code"),
				"loginAccount":      mustGetFlag(cmd, "login-account"),
				"taxpayerUserID":    mustGetFlag(cmd, "taxpayer-user-id"),
				"loginID":           mustGetFlag(cmd, "login-id"),
				"loginPwd":          mustGetFlag(cmd, "login-pwd"),
				"taxpayerUser":      mustGetFlag(cmd, "taxpayer-user"),
				"taxpayerUserPhone": mustGetFlag(cmd, "taxpayer-user-phone"),
				"serialNo":          mustGetFlag(cmd, "serial-no"),
			})
		},
	}

	var financeDigitalInvoiceAccountCmd = &cobra.Command{
		Use:   "account",
		Short: "查询数电账号信息",
		Long: `查询当前用户已绑定的数电账号详情，包括税号、纳税人名称、登录方式等。
    常用于开票前确认账号状态。`,
		Example: `  dws finance digital-invoice account --company-code COMP001`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code"); err != nil {
				return err
			}
			return callMCPTool("query_digital_account", map[string]any{
				"companyCode": mustGetFlag(cmd, "company-code"),
			})
		},
	}

	var financeDigitalInvoiceSmsCodeCmd = &cobra.Command{
		Use:   "sms-code",
		Short: "上传数电登录短信验证码",
		Long: `数电手机验证码登录时，用户收到短信后将验证码提交，完成登录认证。
    需在 login 登录流程内配合使用。`,
		Example: `  dws finance digital-invoice sms-code --company-code COMP001 --serial-no SN001 --sms-code 123456 --phone 13800138000`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code", "serial-no", "sms-code", "phone"); err != nil {
				return err
			}
			return callMCPTool("upload_invoice_sms_code", map[string]any{
				"companyCode":       mustGetFlag(cmd, "company-code"),
				"serialNo":          mustGetFlag(cmd, "serial-no"),
				"smsCode":           mustGetFlag(cmd, "sms-code"),
				"taxpayerUserPhone": mustGetFlag(cmd, "phone"),
			})
		},
	}

	var financeDigitalInvoiceGoodsCodeCmd = &cobra.Command{
		Use:   "goods-code",
		Short: "商品智能赋码",
		Long: `开票时填写商品明细，通过商品名称智能匹配对应的税收分类编码和适用税率，
    避免手动查找编码。返回结果为候选列表，由用户最终选择应用。`,
		Example: `  dws finance digital-invoice goods-code --company-code COMP001 --good-name "办公编程软件"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code", "good-name"); err != nil {
				return err
			}
			return callMCPTool("intelligent_goods_code", map[string]any{
				"companyCode": mustGetFlag(cmd, "company-code"),
				"goodName":    mustGetFlag(cmd, "good-name"),
			})
		},
	}

	var financeDigitalInvoiceFaceQrCmd = &cobra.Command{
		Use:   "face-qr",
		Short: "获取人脸识别二维码",
		Long: `数电开票前需对办税人员进行人脸识别认证，获取二维码展示给用户扫码。
    二维码带有有效期，过期后需重新获取；认证结果通过 face-status 命令轮询。

    --id-auth-type: 身份认证人脸识别类型
      0 = 税务App
      1 = 个税App`,
		Example: `  dws finance digital-invoice face-qr --company-code COMP001 --id-auth-type 0
      dws finance digital-invoice face-qr --company-code COMP001 --id-auth-type 1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code", "id-auth-type"); err != nil {
				return err
			}
			return callMCPTool("get_face_qr_code", map[string]any{
				"companyCode": mustGetFlag(cmd, "company-code"),
				"idAuthType":  mustGetFlag(cmd, "id-auth-type"),
			})
		},
	}

	var financeDigitalInvoiceFaceStatusCmd = &cobra.Command{
		Use:   "face-status",
		Short: "获取人脸认证状态",
		Long: `用户扫码完成人脸识别后，轮询该接口确认认证状态，判断是否可进行开票。
    返回字段 faceSwiping="1" 表示认证成功，"0" 表示未认证。`,
		Example: `  dws finance digital-invoice face-status --company-code COMP001`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code"); err != nil {
				return err
			}
			return callMCPTool("get_face_auth_status", map[string]any{
				"companyCode": mustGetFlag(cmd, "company-code"),
			})
		},
	}

	var financeDigitalInvoiceTitleCmd = &cobra.Command{
		Use:   "title",
		Short: "智能抬头",
		Long: `开票时输入购方名称，智能匹配购方纳税人信息（包括税号、开户行、地址等），
    减少手动输入。返回结果为候选列表，由用户选择应用于开票表单。`,
		Example: `  dws finance digital-invoice title --company-code COMP001 --name "钉钉科技"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code", "name"); err != nil {
				return err
			}
			return callMCPTool("intelligent_title", map[string]any{
				"companyCode": mustGetFlag(cmd, "company-code"),
				"name":        mustGetFlag(cmd, "name"),
			})
		},
	}

	var financeDigitalInvoiceIssueCmd = &cobra.Command{
		Use:   "issue",
		Short: "开具数电发票",
		Long: `用户已完成数电登录认证后，开具数电发票，支持上传购方、明细、金额等完整开票信息。
    销方税号和名称由系统通过 --company-code 自动带入，无需手动传入。
    开票成功后返回发票号、PDF/OFD/XML 等多种格式的文件 URL。

    --details 为 JSON 数组，每个元素包含 amount、taxAmount、taxRate（必填）及 itemTitle、revenueCode（可选）。`,
		Example: `  dws finance digital-invoice issue \
        --company-code COMP001 --serial-no SN001 --invoice-type-code 026 \
        --customer-code CUST001 \
        --total-exclude-tax 1000 --total-tax-amount 130 --total-include-tax 1130 \
        --details '[{"amount":"1000","taxAmount":"130","taxRate":"0.13","itemTitle":"咨询服务"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code", "serial-no", "invoice-type-code", "customer-code", "total-exclude-tax", "total-tax-amount", "total-include-tax", "details"); err != nil {
				return err
			}
			rawDetails := mustGetFlag(cmd, "details")
			var details []map[string]any
			if err := json.Unmarshal([]byte(rawDetails), &details); err != nil {
				return fmt.Errorf("--details JSON parse failed: %w", err)
			}
			return callMCPTool("issue_digital_invoice", map[string]any{
				"companyCode":     mustGetFlag(cmd, "company-code"),
				"serialNo":        mustGetFlag(cmd, "serial-no"),
				"invoiceTypeCode": mustGetFlag(cmd, "invoice-type-code"),
				"customerCode":    mustGetFlag(cmd, "customer-code"),
				"totalExcludeTax": mustGetFlag(cmd, "total-exclude-tax"),
				"totalTaxAmount":  mustGetFlag(cmd, "total-tax-amount"),
				"totalIncludeTax": mustGetFlag(cmd, "total-include-tax"),
				"details":         details,
			})
		},
	}

	var financeDigitalInvoiceFileCmd = &cobra.Command{
		Use:   "file",
		Short: "获取发票版式文件",
		Long: `开票完成后，获取该发票的 PDF、OFD、XML 格式文件地址及二维码 URL，
    供用户下载或预览。`,
		Example: `  dws finance digital-invoice file --serial-no SN001 --drew-date 2025-07-01 --invoice-no 24110000000001234567`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "serial-no", "drew-date", "invoice-no"); err != nil {
				return err
			}
			return callMCPTool("get_invoice_file", map[string]any{
				"serialNo":  mustGetFlag(cmd, "serial-no"),
				"drewDate":  mustGetFlag(cmd, "drew-date"),
				"invoiceNo": mustGetFlag(cmd, "invoice-no"),
			})
		},
	}

	var financeDigitalInvoiceSkillVersionCmd = &cobra.Command{
		Use:   "skill-version",
		Short: "查询开票 Skill 版本",
		Long: `查询当前组织应使用的开票 Skill 版本，用于 AI 在发起开票流程前判断调用哪套开票接口。

    版本说明：
      V1 = SaaS 开票版本
      V2 = 轻量化开票版本

    判断逻辑：组织未命中灰度名单返回 V1；命中灰度且在白名单内返回 V2；
    否则判断组织是否有发票权益，有权益返回 V1，无权益返回 V2。`,
		Example: `  dws finance digital-invoice skill-version`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("query_invoice_skill_version", nil)
		},
	}

	var financeDigitalInvoiceLoginPageCmd = &cobra.Command{
		Use:   "login-page",
		Short: "获取数电发票登录页面链接",
		Long: `获取数电发票登录认证页面的链接，用于引导用户跳转完成数电登录认证。
    未完成登录认证时，可通过此接口获取登录页面地址展示给用户。`,
		Example: `  dws finance digital-invoice login-page --company-code COMP001 --company-name "钉钉科技" --tax-no "913485740000000000"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code"); err != nil {
				return err
			}
			return callMCPTool("get_invoice_login_page", map[string]any{
				"companyCode": mustGetFlag(cmd, "company-code"),
				"companyName": mustGetFlag(cmd, "company-name"),
				"taxNo":       mustGetFlag(cmd, "tax-no"),
			})
		},
	}

	var financeDigitalInvoiceSendEmailCmd = &cobra.Command{
		Use:   "send-email",
		Short: "发送发票邮件（轻量化版）",
		Long: `将已开具的发票通过邮件方式发送给指定收件人。
    系统根据 skill-version 返回值自动路由，V2 使用本命令（轻量化版本）。

    --items 为 JSON 数组，每个元素包含 email（邮箱地址）和 items（发票列表，每项含 invoiceNo 和 drewDate）。`,
		Example: `  dws finance digital-invoice send-email --company-code COMP001 \
        --items '[{"email":"test@example.com","items":[{"invoiceNo":"12345","drewDate":"2025-07-01"}]}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code", "items"); err != nil {
				return err
			}
			var items []map[string]any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "items")), &items); err != nil {
				return fmt.Errorf("--items JSON parse failed: %w", err)
			}
			return callMCPTool("send_invoice_to_email", map[string]any{
				"companyCode": mustGetFlag(cmd, "company-code"),
				"items":       items,
			})
		},
	}

	var financeDigitalInvoiceSendEmailSaasCmd = &cobra.Command{
		Use:   "send-email-saas",
		Short: "发送发票邮件（SaaS版）",
		Long: `将已开具的发票通过邮件方式发送给指定收件人（SaaS 版本，适用于有发票权益的组织）。
      --items 为 JSON 数组，每个元素包含 email（邮箱地址）和 items（发票列表，每项含 invoiceNo 和 drewDate）。`,
		Example: `  dws finance digital-invoice send-email-saas --company-code COMP001 \
          --items '[{"email":"test@example.com","items":[{"invoiceNo":"12345","drewDate":"2025-07-01"}]}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code", "items"); err != nil {
				return err
			}
			var items []map[string]any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "items")), &items); err != nil {
				return fmt.Errorf("--items JSON parse failed: %w", err)
			}
			return callMCPTool("send_invoice_to_email_saas", map[string]any{
				"companyCode": mustGetFlag(cmd, "company-code"),
				"items":       items,
			})
		},
	}

	var financeDigitalInvoiceBatchDrawCmd = &cobra.Command{
		Use:   "batch-draw",
		Short: "批量开票（轻量化版）",
		Long: `一次性开具多张发票，提高开票效率（轻量化版本，适用于无发票权益的组织）。

    --items 为 JSON 数组，每个元素包含 invoiceTypeCode、customerCode 和 details（明细列表）。
    details 中每个明细包含：amountIncludeTax（含税金额）、taxRate（税率）、revenueCode（税收分类编码）、itemTitle（商品名称）、spec（规格）、unit（单位）、quantity（数量）、unitPrice（单价）。`,
		Example: `  dws finance digital-invoice batch-draw --company-code COMP001 \
        --items '[{"invoiceTypeCode":"026","customerCode":"CUST001","details":[{"amountIncludeTax":"1000","taxRate":"0.13","revenueCode":"3040201","itemTitle":"咨询服务","spec":"标准版","unit":"次","quantity":"1","unitPrice":"1000"}]}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code", "items"); err != nil {
				return err
			}
			var items []map[string]any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "items")), &items); err != nil {
				return fmt.Errorf("--items JSON parse failed: %w", err)
			}
			return callMCPTool("invoice_batch_draw", map[string]any{
				"companyCode": mustGetFlag(cmd, "company-code"),
				"items":       items,
			})
		},
	}

	var financeDigitalInvoiceBatchDrawSaasCmd = &cobra.Command{
		Use:   "batch-draw-saas",
		Short: "批量开票（SaaS版）",
		Long: `一次性开具多张发票，提高开票效率（SaaS 版本，适用于有发票权益的组织）。
    --orders 为 JSON 数组，每个元素包含 orderId、invoiceType、customerCode 和 products（产品列表）。
    products 中每个产品包含：productName（产品名称）、amountWithTax（含税金额）、spec（规格）、unit（单位）、quantity（数量）、unitPrice（单价）。`,
		Example: `  dws finance digital-invoice batch-draw-saas --company-code COMP001 --batch-no BATCH001 \
        --orders '[{"orderId":"ORD001","invoiceType":"9","customerCode":"CUST001","products":[{"productName":"咨询服务","amountWithTax":"1130","spec":"标准版","unit":"次","quantity":"1","unitPrice":"1130"}]}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code", "batch-no", "orders"); err != nil {
				return err
			}
			var orders []map[string]any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "orders")), &orders); err != nil {
				return fmt.Errorf("--orders JSON parse failed: %w", err)
			}
			return callMCPTool("invoice_batch_draw_saas", map[string]any{
				"companyCode":   mustGetFlag(cmd, "company-code"),
				"batchNo":       mustGetFlag(cmd, "batch-no"),
				"orderBillList": orders,
			})
		},
	}

	var financeDigitalInvoiceBatchDrawQueryCmd = &cobra.Command{
		Use:     "batch-draw-query",
		Short:   "批量开票查询（轻量化版）",
		Long:    `查询批量开票任务的执行状态和结果（轻量化版本，适用于无发票权益的组织）。`,
		Example: `  dws finance digital-invoice batch-draw-query --company-code COMP001 --batch-no BATCH001`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code", "batch-no"); err != nil {
				return err
			}
			return callMCPToolUnescaped("invoice_batch_draw_query", map[string]any{
				"companyCode": mustGetFlag(cmd, "company-code"),
				"batchNo":     mustGetFlag(cmd, "batch-no"),
			})
		},
	}

	var financeDigitalInvoiceBatchDrawQuerySaasCmd = &cobra.Command{
		Use:     "batch-draw-query-saas",
		Short:   "批量开票查询（SaaS版）",
		Long:    `查询批量开票任务的执行状态和结果（SaaS 版本，适用于有发票权益的组织）。`,
		Example: `  dws finance digital-invoice batch-draw-query-saas --company-code COMP001 --batch-no BATCH001`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code", "batch-no"); err != nil {
				return err
			}
			return callMCPToolUnescaped("invoice_batch_draw_query_saas", map[string]any{
				"companyCode": mustGetFlag(cmd, "company-code"),
				"batchNo":     mustGetFlag(cmd, "batch-no"),
			})
		},
	}

	var financeDigitalInvoiceGetTableCmd = &cobra.Command{
		Use:   "get-table",
		Short: "获取发票表格配置",
		Long: `根据表格类型从Diamond配置中获取对应的schema，并组合传入的动态数据，用于前端渲染发票相关表格。
    仅返回表格配置，不触发实际业务操作；返回的JSON包含schema结构和传入的data数据。`,
		Example: `  dws finance digital-invoice get-table --type "invoice_list" --data '{"key":"value"}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "type"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"type": mustGetFlag(cmd, "type"),
			}
			if v := mustGetFlag(cmd, "data"); v != "" {
				var data map[string]any
				if err := json.Unmarshal([]byte(v), &data); err != nil {
					return fmt.Errorf("--data JSON parse failed: %w", err)
				}
				toolArgs["data"] = data
			}
			return callMCPTool("get_invoice_table", toolArgs)
		},
	}

	var financeDigitalInvoiceImportGoodsCmd = &cobra.Command{
		Use:   "import-goods",
		Short: "导入商品",
		Long: `批量导入商品信息到系统中，用于开票时选择商品明细。
    支持导入多个商品，每个商品包含商品名称、税收分类编码等必填字段。`,
		Example: `  dws finance digital-invoice import-goods --company-code COMP001 --items '[{"goodsName":"办公用品","revenueCode":"3040201","unit":"个","taxRate":"0.13","unitPrice":"100","specifications":"标准版"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code", "items"); err != nil {
				return err
			}
			var items []map[string]any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "items")), &items); err != nil {
				return fmt.Errorf("--items JSON parse failed: %w", err)
			}
			return callMCPTool("import_goods", map[string]any{
				"companyCode": mustGetFlag(cmd, "company-code"),
				"items":       items,
			})
		},
	}

	var financeDigitalInvoiceSearchGoodsCmd = &cobra.Command{
		Use:   "search-goods",
		Short: "搜索商品",
		Long: `按关键词搜索已导入的商品列表，可用于开票时选取商品信息。
    支持分页查询，可按商品名称、编码等进行搜索。`,
		Example: `  dws finance digital-invoice search-goods --company-code COMP001 --goods-name "办公用品"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"companyCode": mustGetFlag(cmd, "company-code"),
			}
			if v := flagOrFallback(cmd, "goods-name"); v != "" {
				toolArgs["goodsName"] = v
			}
			return callMCPTool("search_goods", toolArgs)
		},
	}

	// ── gather (自定义经营报表采集) ─────────────────────────────

	gatherCmd := &cobra.Command{Use: "gather", Short: "自定义经营报表数据采集", RunE: groupRunE}

	gatherSaveRuleCmd := &cobra.Command{
		Use:   "save-rule",
		Short: "保存采集规则",
		Long: `保存自定义经营报表的数据采集规则，针对某个具体的单据模版保存该模版的数据采集规则。
适用于 Agent 中采集自定义经营报表数据的场景。`,
		Example: `  dws finance gather save-rule --process-code PROC001 --rules '{"field":"amount","op":"sum"}'
  dws finance gather save-rule --process-code PROC001 --rules '...' --table-field-id FLD001`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "process-code", "rules"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"processCode": mustGetFlag(cmd, "process-code"),
				"rules":       mustGetFlag(cmd, "rules"),
			}
			if v := mustGetFlag(cmd, "table-field-id"); v != "" {
				toolArgs["basedOnTableFieldId"] = v
			}
			return callMCPTool("save_gather_rule", toolArgs)
		},
	}

	gatherQueryRuleCmd := &cobra.Command{
		Use:   "query-rule",
		Short: "查询采集规则",
		Long:  `根据单据模版 code 或名称查询自定义经营报表数据采集规则。`,
		Example: `  dws finance gather query-rule --process-code PROC001
  dws finance gather query-rule --process-name "付款审批"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v := mustGetFlag(cmd, "process-code"); v != "" {
				toolArgs["processCode"] = v
			}
			if v := mustGetFlag(cmd, "process-name"); v != "" {
				toolArgs["processName"] = v
			}
			return callMCPTool("query_gather_rule", toolArgs)
		},
	}

	gatherTryExecuteCmd := &cobra.Command{
		Use:   "try-execute",
		Short: "尝试执行数据采集（单条验证）",
		Long: `尝试执行自定义报表数据采集，采集单个单据的数据。
适用于验证采集规则是否正确的场景。`,
		Example: `  dws finance gather try-execute --process-code PROC001 --business-id BIZ001
  dws finance gather try-execute --process-code PROC001 --instance-id INST001`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v := mustGetFlag(cmd, "business-id"); v != "" {
				toolArgs["businessId"] = v
			}
			if v := mustGetFlag(cmd, "instance-id"); v != "" {
				toolArgs["instanceId"] = v
			}
			if v := mustGetFlag(cmd, "process-code"); v != "" {
				toolArgs["processCode"] = v
			}
			return callMCPTool("try_execute_gather", toolArgs)
		},
	}

	gatherExecuteCmd := &cobra.Command{
		Use:   "execute",
		Short: "执行数据采集（批量）",
		Long: `实际执行自定义经营报表数据采集，支持传入多个单据进行批量采集。

--instances 为 JSON 数组，每个元素包含 businessId、instanceId、processCode。`,
		Example: `  dws finance gather execute --instances '[{"businessId":"BIZ001","instanceId":"INST001","processCode":"PROC001"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "instances"); err != nil {
				return err
			}
			var instances []map[string]any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "instances")), &instances); err != nil {
				return fmt.Errorf("--instances JSON parse failed: %w", err)
			}
			return callMCPTool("execute_gather", map[string]any{
				"instanceList": instances,
			})
		},
	}

	gatherExecuteGeneralCmd := &cobra.Command{
		Use:   "execute-general",
		Short: "执行数据采集（通用场景）",
		Long: `执行自定义经营报表数据采集（通用场景），入参即为自定义经营报表数据本身，无需解析。
适用于 Agent 采集自定义经营报表数据的通用场景。

--data-list 为 JSON 数组，每项必填 instanceId；常用字段包括：
  title 标题 / amount 记账金额 / result 单据结果 / source 数据来源 / status 单据状态
  company 主体 / product 商品 / project 项目 / category 费用类型 / customer 客户 / supplier 供应商
  detailId 明细 ID / businessId 审批编号 / extension 拓展信息 / instanceUrl 流程实例跳转链接
  recordTime 记账时间 / recordYear 记账年份 / recordMonth 记账月份 / accountType 记账类型
  processCode 模版 code / processName 模版名称 / principalName 归属人名称 / departmentName 归属部门名称 / enterpriseAccount 账户`,
		Example: `  dws finance gather execute-general \
    --data-list '[{"instanceId":"INST001","businessId":"BIZ001","title":"差旅报销","amount":"1000","processCode":"PROC001","recordTime":"2025-07-01 10:00:00"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "data-list"); err != nil {
				return err
			}
			var dataList []map[string]any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "data-list")), &dataList); err != nil {
				return fmt.Errorf("--data-list JSON parse failed: %w", err)
			}
			return callMCPTool("execute_gather_general", map[string]any{
				"dataList": dataList,
			})
		},
	}

	// ── process (审批) ──────────────────────────────────────────

	processCmd := &cobra.Command{Use: "process", Short: "审批单管理", RunE: groupRunE}

	processFormDataCmd := &cobra.Command{
		Use:     "form-data",
		Short:   "根据审批编号查询审批表单信息",
		Long:    `根据审批编号查询审批表单信息，返回审批单的完整表单详情。`,
		Example: `  dws finance process form-data --business-id BIZ123456`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "business-id"); err != nil {
				return err
			}
			return callMCPTool("query_process_form_data", map[string]any{
				"businessId": mustGetFlag(cmd, "business-id"),
			})
		},
	}

	processListCmd := &cobra.Command{
		Use:   "list",
		Short: "根据审批模版名查询审批单列表",
		Long: `根据审批模版名查询审批单列表，支持按时间范围筛选，分页返回结果。
适用于查询某类审批单的场景。

--status: 审批单状态列表 JSON 数组，为空时查询所有状态
  可选值: COMPLETED, RUNNING, NEW 等
--order-by: 排序字段，为空时按创建时间排序
  可选值: finishTime 等`,
		Example: `  dws finance process list --form-name "付款审批" --start-time "2025-01-01 00:00:00" --end-time "2025-07-01 23:59:59" --page-no 1 --page-size 10
  dws finance process list --form-name "付款审批" --start-time "2025-01-01 00:00:00" --end-time "2025-07-01 23:59:59" --status '["COMPLETED","RUNNING"]' --order-by finishTime`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "form-name", "start-time", "end-time"); err != nil {
				return err
			}
			pageNo, _ := cmd.Flags().GetFloat64("page-no")
			pageSize, _ := cmd.Flags().GetFloat64("page-size")
			toolArgs := map[string]any{
				"formName":  mustGetFlag(cmd, "form-name"),
				"pageNo":    pageNo,
				"pageSize":  pageSize,
				"startTime": mustGetFlag(cmd, "start-time"),
				"endTime":   mustGetFlag(cmd, "end-time"),
			}
			if v := mustGetFlag(cmd, "status"); v != "" {
				var statusList []string
				if err := json.Unmarshal([]byte(v), &statusList); err != nil {
					return fmt.Errorf("--status JSON parse failed: %w", err)
				}
				toolArgs["status"] = statusList
			}
			if v := mustGetFlag(cmd, "order-by"); v != "" {
				toolArgs["orderBy"] = v
			}
			return callMCPTool("list_process_by_form_name", toolArgs)
		},
	}

	// ── invoice application (开票申请) ─────────────────────────

	invoiceListApplicationCmd := &cobra.Command{
		Use:   "list-application",
		Short: "查询开票申请列表",
		Long:  `查询开票审批单列表，支持按时间范围筛选，分页返回结果。`,
		Example: `  dws finance invoice list-application --page-no 1 --page-size 10
  dws finance invoice list-application --page-no 1 --page-size 10 --start-time "2025-01-01 00:00:00" --end-time "2025-07-01 23:59:59"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pageNo, _ := cmd.Flags().GetFloat64("page-no")
			pageSize, _ := cmd.Flags().GetFloat64("page-size")
			toolArgs := map[string]any{
				"pageNo":   pageNo,
				"pageSize": pageSize,
			}
			if v := mustGetFlag(cmd, "start-time"); v != "" {
				toolArgs["startTime"] = v
			}
			if v := mustGetFlag(cmd, "end-time"); v != "" {
				toolArgs["endTime"] = v
			}
			return callMCPTool("list_invoice_application", toolArgs)
		},
	}

	invoiceAddRecordCmd := &cobra.Command{
		Use:     "add-record",
		Short:   "添加发票到审批单",
		Long:    `将发票 PDF 文件添加到审批单评论区，适用于保存发票到审批单的场景。`,
		Example: `  dws finance invoice add-record --business-id BIZ123456 --invoice-pdf-url https://example.com/invoice.pdf`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "business-id", "invoice-pdf-url"); err != nil {
				return err
			}
			return callMCPTool("add_invoice_record_to_receipt", map[string]any{
				"businessId":    mustGetFlag(cmd, "business-id"),
				"invoicePdfUrl": mustGetFlag(cmd, "invoice-pdf-url"),
			})
		},
	}

	invoiceListCmd := &cobra.Command{
		Use:   "list",
		Short: "分页查询发票列表",
		Long:  `分页查询智能财务系统内的发票列表，支持按发票类型、财务类型、发票认证状态、关键字等筛选。适用于 Agent 查询发票列表的场景。`,
		Example: `  dws finance invoice list --company-code COMP001 --page-no 1 --page-size 20 \
    --start-time "2025-07-01 00:00:00" --end-time "2025-07-31 23:59:59"
  dws finance invoice list --company-code COMP001 --page-no 1 --page-size 20 \
    --start-time "2025-07-01 00:00:00" --end-time "2025-07-31 23:59:59" \
    --invoice-type 8 --finance-type 1 --verify-status 1 --query "服务费"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "company-code", "start-time", "end-time"); err != nil {
				return err
			}
			pageNo, _ := cmd.Flags().GetFloat64("page-no")
			pageSize, _ := cmd.Flags().GetFloat64("page-size")
			toolArgs := map[string]any{
				"companyCode": mustGetFlag(cmd, "company-code"),
				"pageNumber":  pageNo,
				"pageSize":    pageSize,
				"startTime":   mustGetFlag(cmd, "start-time"),
				"endTime":     mustGetFlag(cmd, "end-time"),
			}
			if v := mustGetFlag(cmd, "finance-type"); v != "" {
				toolArgs["financeType"] = v
			}
			if v := mustGetFlag(cmd, "invoice-type"); v != "" {
				toolArgs["invoiceType"] = v
			}
			if v := mustGetFlag(cmd, "verify-status"); v != "" {
				toolArgs["verifyStatus"] = v
			}
			if v := flagOrFallback(cmd, "query", "keyword"); v != "" {
				toolArgs["fuzzyQueryKey"] = v
			}
			if v := mustGetFlag(cmd, "invoice-types"); v != "" {
				var types []string
				if err := json.Unmarshal([]byte(v), &types); err != nil {
					return fmt.Errorf("--invoice-types JSON parse failed: %w", err)
				}
				toolArgs["invoiceTypeList"] = types
			}
			return callMCPTool("page_query_invoice", toolArgs)
		},
	}

	// ── customer save (新建客户) ────────────────────────────────

	var financeCustomerSaveCmd = &cobra.Command{
		Use:   "save",
		Short: "新建客户",
		Long: `开票时购方客户不存在时，先新建客户档案，包括客户名称、发票抬头和税号。
    新建成功后可通过返回的客户编码（customerCode）进行开票。
    注意：客户名称用于档案展示，发票抬头（--purchaser-name）是开票时实际填写到发票中的抬头，两者可不同。`,
		Example: `  dws finance customer save --customer-name "某某科技有限公司" --purchaser-name "某某科技有限公司" --tax-no 91110000123456789X`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "customer-name", "purchaser-name"); err != nil {
				return err
			}
			return callMCPTool("save_customer", map[string]any{
				"customerName":   mustGetFlag(cmd, "customer-name"),
				"purchaserName":  mustGetFlag(cmd, "purchaser-name"),
				"purchaserTaxNo": mustGetFlag(cmd, "tax-no"),
			})
		},
	}

	// ── payment (支付/付款) ────────────────────────────────────

	paymentCmd := &cobra.Command{Use: "payment", Short: "支付 / 付款管理", RunE: groupRunE}

	financePaymentCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建待付款审批单",
		Long: `创建带付款人节点的付款审批单，用于支付，可指定付款金额、收款账户、备注。

    --payee-account-type: 收款账户类型，如 BANK_CARD（银行卡）等。`,
		Example: `  dws finance payment create --amount 5000 --payee-account-no 622001234 \
        --payee-account-type BANK_CARD --payee-account-name "某某公司"
      dws finance payment create --amount 1000 --payee-account-no 622001234 \
        --payee-account-type BANK_CARD --payee-account-name "某某公司" \
        --payee-bank-name "招商银行" --payee-branch-name "上海分行" --remark "货款"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "amount", "payee-account-no", "payee-account-type", "payee-account-name"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"amount":           mustGetFlag(cmd, "amount"),
				"payeeAccountNo":   mustGetFlag(cmd, "payee-account-no"),
				"payeeAccountType": mustGetFlag(cmd, "payee-account-type"),
				"payeeAccountName": mustGetFlag(cmd, "payee-account-name"),
			}
			if v := mustGetFlag(cmd, "payee-bank-name"); v != "" {
				toolArgs["payeeBankName"] = v
			}
			if v := mustGetFlag(cmd, "payee-branch-name"); v != "" {
				toolArgs["payeeBranchName"] = v
			}
			if v := mustGetFlag(cmd, "remark"); v != "" {
				toolArgs["remark"] = v
			}
			return callMCPTool("create_payment_order", toolArgs)
		},
	}

	financePaymentListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询待付款列表",
		Long:  `查询待付款列表，支持根据收款账号筛选。`,
		Example: `  dws finance payment list --page-no 1 --page-size 20
      dws finance payment list --page-no 1 --page-size 20 --payee-account-no 622001234`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pageNo, _ := cmd.Flags().GetFloat64("page-no")
			pageSize, _ := cmd.Flags().GetFloat64("page-size")
			toolArgs := map[string]any{
				"pageNo":   pageNo,
				"pageSize": pageSize,
			}
			if v := mustGetFlag(cmd, "payee-account-no"); v != "" {
				toolArgs["payeeAccountNo"] = v
			}
			return callMCPTool("query_wait_orders", toolArgs)
		},
	}

	financePaymentAccountListCmd := &cobra.Command{
		Use:   "account-list",
		Short: "查询收款账户列表",
		Long:  `分页查询收款账户列表，支持按照关键字搜索。`,
		Example: `  dws finance payment account-list --page-no 1 --page-size 20
      dws finance payment account-list --page-no 1 --page-size 20 --query "招商"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pageNo, _ := cmd.Flags().GetFloat64("page-no")
			pageSize, _ := cmd.Flags().GetFloat64("page-size")
			toolArgs := map[string]any{
				"pageNo":   pageNo,
				"pageSize": pageSize,
			}
			if v := mustGetFlag(cmd, "query"); v != "" {
				toolArgs["searchKey"] = v
			}
			return callMCPTool("list_receiptor_account", toolArgs)
		},
	}

	financePaymentCashierURLCmd := &cobra.Command{
		Use:   "cashier-url",
		Short: "查询支付收银台链接",
		Long: `查询支付收银台链接，支持单笔付款或合并多笔审批单付款。
    --instance-id 和 --instance-ids 二选一传入，合并付款时使用 --instance-ids。`,
		Example: `  dws finance payment cashier-url --instance-id INST001
      dws finance payment cashier-url --instance-ids "INST001,INST002,INST003"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v := mustGetFlag(cmd, "instance-id"); v != "" {
				toolArgs["instanceId"] = v
			}
			if v := mustGetFlag(cmd, "instance-ids"); v != "" {
				toolArgs["instanceIdList"] = strings.Split(v, ",")
			}
			return callMCPTool("query_cashier_url", toolArgs)
		},
	}

	financePaymentAccountURLCmd := &cobra.Command{
		Use:     "account-url",
		Short:   "获取收款账户管理页面链接",
		Long:    `获取收款账户管理页面链接，用于跳转到收款账户管理页面。`,
		Example: `  dws finance payment account-url`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("query_receiptor_management_url", nil)
		},
	}

	financePaymentPayerListCmd := &cobra.Command{
		Use:   "payer-list",
		Short: "查询付款账户列表",
		Long: `查询付款账户列表，包括智能财务中签约的付款账户和企业支付中绑定的付款账户。
    无需任何参数，直接返回当前企业可用的付款账户。`,
		Example: `  dws finance payment payer-list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("query_payer_account_list", nil)
		},
	}

	// ── receipt flags & tree ──
	receiptCreateCmd.Flags().String("amount", "", "金额 (必填)")
	receiptCreateCmd.Flags().String("category-code", "", "收支类别编码")
	receiptCreateCmd.Flags().String("supplier-code", "", "供应商编码")
	receiptCreateCmd.Flags().String("tax", "", "税额")
	receiptCreateCmd.Flags().String("invoices", "", "发票列表 JSON 数组")

	receiptCollectionCmd.Flags().String("amount", "", "收款金额 (必填)")
	receiptCollectionCmd.Flags().String("title", "", "单据标题 (必填)")
	receiptCollectionCmd.Flags().String("detail-id", "", "银行交易明细 ID (必填)")
	receiptCollectionCmd.Flags().String("record-time", "", "记账时间 yyyy-MM-dd HH:mm:ss")
	receiptCollectionCmd.Flags().String("customer-code", "", "客户编码")
	receiptCollectionCmd.Flags().String("account-code", "", "企业账户编码")

	receiptCmd.AddCommand(receiptCreateCmd, receiptCollectionCmd)

	// ── invoice ──
	invoiceUploadCmd.Flags().String("url", "", "发票文件链接 (必填)")
	invoiceUploadCmd.Flags().String("name", "", "发票文件名称 (必填)")
	invoiceUploadCmd.Flags().String("type", "", "发票文件类型，如 pdf/jpg/png (必填)")

	invoiceIssueCmd.Flags().String("purchaser", "", "购方抬头")
	invoiceIssueCmd.Flags().String("taxnum", "", "购方税号")
	invoiceIssueCmd.Flags().String("invoice-type", "9", "发票类型: 8=数电专票, 9=数电普票 (默认 9)")
	invoiceIssueCmd.Flags().String("products", "", "商品信息 JSON 数组")

	invoiceIssueResultCmd.Flags().String("order-id", "", "订单编号 (必填)")

	invoiceRecommendCategoryCmd.Flags().String("items", "", "发票列表 JSON 数组，含 requestId 和 companyIndexId (必填)")

	invoiceListApplicationCmd.Flags().Float64("page-no", 1, "页码 (默认 1)")
	invoiceListApplicationCmd.Flags().Float64("page-size", 20, "分页大小 (默认 20)")
	invoiceListApplicationCmd.Flags().String("start-time", "", "筛选开始时间")
	invoiceListApplicationCmd.Flags().String("end-time", "", "筛选结束时间")

	invoiceAddRecordCmd.Flags().String("business-id", "", "审批编号 (必填)")
	invoiceAddRecordCmd.Flags().String("invoice-pdf-url", "", "发票 PDF 文件 URL (必填)")

	invoiceListCmd.Flags().String("company-code", "", "企业主体 code (必填)")
	invoiceListCmd.Flags().Float64("page-no", 1, "页码 (默认 1)")
	invoiceListCmd.Flags().Float64("page-size", 20, "分页大小 (默认 20)")
	invoiceListCmd.Flags().String("start-time", "", "查询开始时间 yyyy-MM-dd HH:mm:ss (必填)")
	invoiceListCmd.Flags().String("end-time", "", "查询结束时间 yyyy-MM-dd HH:mm:ss (必填)")
	invoiceListCmd.Flags().String("finance-type", "", "财务类型")
	invoiceListCmd.Flags().String("invoice-type", "", "发票类型")
	invoiceListCmd.Flags().String("verify-status", "", "发票认证状态")
	invoiceListCmd.Flags().String("query", "", "查询关键字")
	invoiceListCmd.Flags().String("keyword", "", "查询关键字 (--query 的别名)")
	_ = invoiceListCmd.Flags().MarkHidden("keyword")
	invoiceListCmd.Flags().String("invoice-types", "", "发票类型列表 JSON 数组，与 --invoice-type 二选一")

	invoiceCmd.AddCommand(
		invoiceUploadCmd, invoiceIssueCmd,
		invoiceIssueResultCmd, invoiceRecommendCategoryCmd,
		invoiceListApplicationCmd, invoiceAddRecordCmd,
		invoiceListCmd,
	)

	// ── bank ──
	bankCreateCmd.Flags().String("trade-time", "", "交易时间，如 2025-07-01 10:00:00 (必填)")
	bankCreateCmd.Flags().String("amount", "", "交易金额 (必填)")
	bankCreateCmd.Flags().String("in-out-flag", "", "收入支出标识: in/out (必填)")
	bankCreateCmd.Flags().String("my-name", "", "当前账户户名 (必填)")
	bankCreateCmd.Flags().String("my-account", "", "当前账户账号 (必填)")
	bankCreateCmd.Flags().String("other-name", "", "对方账户户名 (必填)")
	bankCreateCmd.Flags().String("other-account", "", "对方账户账号 (必填)")
	bankCreateCmd.Flags().String("my-bank", "", "当前账户银行名称 (必填)")
	bankCreateCmd.Flags().String("trade-no", "", "交易流水号")
	bankCreateCmd.Flags().String("balance", "", "余额")
	bankCreateCmd.Flags().String("my-account-id", "", "智能财务企业账户 ID")
	bankCreateCmd.Flags().String("usage", "", "用途")
	bankCreateCmd.Flags().String("remark", "", "备注")
	bankCreateCmd.Flags().String("other-bank", "", "对方账户银行名称")
	bankCreateCmd.Flags().String("other-branch", "", "对方账户支行名称")
	bankCreateCmd.Flags().Bool("skip-check-repeat", false, "是否跳过重复项校验")

	bankQueryCmd.Flags().String("detail-id", "", "交易明细 ID (必填)")

	bankListCmd.Flags().String("account-id", "", "企业账户 ID (必填)")
	bankListCmd.Flags().String("page-no", "", "页码 (必填)")
	bankListCmd.Flags().String("page-size", "", "分页大小 (必填)")
	bankListCmd.Flags().String("trade-start", "", "交易时间起始范围，yyyy-MM-dd HH:mm:ss (必填)")
	bankListCmd.Flags().String("trade-end", "", "交易时间结束范围，yyyy-MM-dd HH:mm:ss (必填)")

	bankCmd.AddCommand(bankCreateCmd, bankQueryCmd, bankListCmd)

	// ── voucher ──
	voucherEntriesCmd.Flags().String("instance-id", "", "审批单实例 ID (必填)")
	voucherGenerateCmd.Flags().String("biz-id", "", "审批单据号 (必填)")
	voucherCmd.AddCommand(voucherEntriesCmd, voucherGenerateCmd)

	// ── customer ──
	customerListCmd.Flags().Float64("page-size", 20, "分页大小 (默认 20)")
	customerListCmd.Flags().Float64("page-index", 1, "分页页码 (默认 1)")
	customerListCmd.Flags().String("query", "", "客户名称关键字")
	customerListCmd.Flags().String("keyword", "", "客户名称关键字 (--query 的别名)")
	_ = customerListCmd.Flags().MarkHidden("keyword")

	customerGetCmd.Flags().String("name", "", "客户名称 (必填，精确匹配)")
	customerGetCmd.Flags().String("corp-id", "", "组织 ID (必填)")

	customerCmd.AddCommand(customerListCmd, customerGetCmd)

	// ── account ──
	accountListCmd.Flags().Float64("page-size", 20, "分页大小 (默认 20)")
	accountListCmd.Flags().Float64("page-index", 1, "分页页码 (默认 1)")
	accountListCmd.Flags().String("query", "", "账户名称关键字")
	accountListCmd.Flags().String("keyword", "", "账户名称关键字 (--query 的别名)")
	_ = accountListCmd.Flags().MarkHidden("keyword")
	accountListCmd.Flags().String("account-no", "", "账号筛选")

	accountCmd.AddCommand(accountListCmd)

	// ── journal ──
	journalDailyCmd.Flags().String("date", "", "统计日期 yyyy-MM-dd (必填)")

	journalCmd.AddCommand(journalDailyCmd, journalDetailURLCmd)

	// ── supplier ──
	supplierSearchCmd.Flags().String("query", "", "搜索关键词")
	supplierSearchCmd.Flags().String("keyword", "", "搜索关键词 (--query 的别名)")
	_ = supplierSearchCmd.Flags().MarkHidden("keyword")
	supplierSearchCmd.Flags().Float64("page-size", 20, "页大小 (默认 20)")
	supplierSearchCmd.Flags().Float64("page-index", 1, "页号 (默认 1)")

	supplierCmd.AddCommand(supplierSearchCmd)

	// ── category ──
	categorySearchCmd.Flags().String("type", "", "收支类别类型: income/expense (必填)")
	categorySearchCmd.Flags().String("query", "", "搜索关键词")
	categorySearchCmd.Flags().String("keyword", "", "搜索关键词 (--query 的别名)")
	_ = categorySearchCmd.Flags().MarkHidden("keyword")
	categorySearchCmd.Flags().String("page-size", "", "页大小")
	categorySearchCmd.Flags().String("page-index", "", "页号")

	categoryCmd.AddCommand(categorySearchCmd)

	// ── payment ──
	financePaymentCreateCmd.Flags().String("amount", "", "付款金额 (必填)")
	financePaymentCreateCmd.Flags().String("payee-account-no", "", "收款账户卡号 (必填)")
	financePaymentCreateCmd.Flags().String("payee-account-type", "", "收款账户类型 (必填)")
	financePaymentCreateCmd.Flags().String("payee-account-name", "", "收款账户户名 (必填)")
	financePaymentCreateCmd.Flags().String("payee-bank-name", "", "收款银行名称")
	financePaymentCreateCmd.Flags().String("payee-branch-name", "", "收款账户支行名称")
	financePaymentCreateCmd.Flags().String("remark", "", "备注")

	// ── company ──
	financeCompanySearchCmd.Flags().String("name", "", "主体名称关键词（模糊搜索，为空返回全部）")
	financeCompanySaveCmd.Flags().String("name", "", "主体名称 (必填)")
	financeCompanySaveCmd.Flags().String("tax-no", "", "税号 (必填)")
	financeCompanyUpdateCmd.Flags().String("code", "", "主体编号 (必填)")
	financeCompanyUpdateCmd.Flags().String("name", "", "主体名称 (必填)")
	financeCompanyUpdateCmd.Flags().String("tax-no", "", "税号 (必填)")
	financeCompanyCmd.AddCommand(financeCompanySearchCmd, financeCompanySaveCmd, financeCompanyUpdateCmd)

	// ── digital-invoice ──
	financeDigitalInvoiceLoginStatusCmd.Flags().String("company-code", "", "主体编码 (必填)")

	financeDigitalInvoiceLoginPageCmd.Flags().String("company-code", "", "主体编码 (必填)")
	financeDigitalInvoiceLoginPageCmd.Flags().String("company-name", "", "主体名称 (必填)")
	financeDigitalInvoiceLoginPageCmd.Flags().String("tax-no", "", "主体税号 (必填)")

	financeDigitalInvoiceLoginCmd.Flags().String("company-code", "", "主体编码 (必填)")
	financeDigitalInvoiceLoginCmd.Flags().String("login-account", "", "登录账号 (必填)")
	financeDigitalInvoiceLoginCmd.Flags().String("taxpayer-user-id", "", "办税人员身份证件号 (必填)")
	financeDigitalInvoiceLoginCmd.Flags().String("login-id", "", "登录身份 (必填)")
	financeDigitalInvoiceLoginCmd.Flags().String("login-pwd", "", "登录密码 (必填)")
	financeDigitalInvoiceLoginCmd.Flags().String("taxpayer-user", "", "办税人员姓名 (必填)")
	financeDigitalInvoiceLoginCmd.Flags().String("taxpayer-user-phone", "", "办税人员手机号 (必填)")
	financeDigitalInvoiceLoginCmd.Flags().String("serial-no", "", "流水号 (必填)")

	financeDigitalInvoiceAccountCmd.Flags().String("company-code", "", "主体编码 (必填)")

	financeDigitalInvoiceSmsCodeCmd.Flags().String("company-code", "", "主体编码 (必填)")
	financeDigitalInvoiceSmsCodeCmd.Flags().String("serial-no", "", "流水号 (必填)")
	financeDigitalInvoiceSmsCodeCmd.Flags().String("sms-code", "", "手机验证码 (必填)")
	financeDigitalInvoiceSmsCodeCmd.Flags().String("phone", "", "办税人手机号 (必填)")

	financeDigitalInvoiceGoodsCodeCmd.Flags().String("company-code", "", "主体编码 (必填)")
	financeDigitalInvoiceGoodsCodeCmd.Flags().String("good-name", "", "商品名称 (必填)")

	financeDigitalInvoiceFaceQrCmd.Flags().String("company-code", "", "主体编码 (必填)")
	financeDigitalInvoiceFaceQrCmd.Flags().String("id-auth-type", "", "身份认证人脸识别类型: 0=税务App, 1=个税App (必填)")

	financeDigitalInvoiceFaceStatusCmd.Flags().String("company-code", "", "主体编码 (必填)")

	financeDigitalInvoiceTitleCmd.Flags().String("company-code", "", "主体编码 (必填)")
	financeDigitalInvoiceTitleCmd.Flags().String("name", "", "购方公司名称关键词 (必填)")

	financeDigitalInvoiceIssueCmd.Flags().String("company-code", "", "主体编码 (必填)")
	financeDigitalInvoiceIssueCmd.Flags().String("serial-no", "", "流水号 (必填)")
	financeDigitalInvoiceIssueCmd.Flags().String("invoice-type-code", "", "发票类型代码 (必填)")
	financeDigitalInvoiceIssueCmd.Flags().String("customer-code", "", "购方编码 (必填)")
	financeDigitalInvoiceIssueCmd.Flags().String("total-exclude-tax", "", "不含税总金额 (必填)")
	financeDigitalInvoiceIssueCmd.Flags().String("total-tax-amount", "", "税额合计 (必填)")
	financeDigitalInvoiceIssueCmd.Flags().String("total-include-tax", "", "含税总金额 (必填)")
	financeDigitalInvoiceIssueCmd.Flags().String("details", "", "明细列表 JSON 数组，每项含 amount/taxAmount/taxRate (必填)")

	financeDigitalInvoiceFileCmd.Flags().String("serial-no", "", "流水号 (必填)")
	financeDigitalInvoiceFileCmd.Flags().String("drew-date", "", "开票日期 (必填)")
	financeDigitalInvoiceFileCmd.Flags().String("invoice-no", "", "发票号码 (必填)")

	// ── send-email ──
	financeDigitalInvoiceSendEmailCmd.Flags().String("company-code", "", "主体编码 (必填)")
	financeDigitalInvoiceSendEmailCmd.Flags().String("items", "", "发送邮件列表 JSON 数组，每项含 email 和 items (必填)")

	financeDigitalInvoiceSendEmailSaasCmd.Flags().String("company-code", "", "主体编码 (必填)")
	financeDigitalInvoiceSendEmailSaasCmd.Flags().String("items", "", "发送邮件列表 JSON 数组，每项含 email 和 items (必填)")

	// ── batch-draw ──
	financeDigitalInvoiceBatchDrawCmd.Flags().String("company-code", "", "主体编码 (必填)")
	financeDigitalInvoiceBatchDrawCmd.Flags().String("items", "", "批量开票项列表 JSON 数组 (必填)")

	// ── batch-draw-query ──
	financeDigitalInvoiceBatchDrawQueryCmd.Flags().String("company-code", "", "主体编码 (必填)")
	financeDigitalInvoiceBatchDrawQueryCmd.Flags().String("batch-no", "", "批次号 (必填)")

	financeDigitalInvoiceBatchDrawSaasCmd.Flags().String("company-code", "", "主体编码 (必填)")
	financeDigitalInvoiceBatchDrawSaasCmd.Flags().String("batch-no", "", "批次号 (必填)")
	financeDigitalInvoiceBatchDrawSaasCmd.Flags().String("orders", "", "订单账单列表 JSON 数组 (必填)")

	financeDigitalInvoiceBatchDrawQuerySaasCmd.Flags().String("company-code", "", "主体编码 (必填)")
	financeDigitalInvoiceBatchDrawQuerySaasCmd.Flags().String("batch-no", "", "批次号 (必填)")

	financeDigitalInvoiceGetTableCmd.Flags().String("type", "", "表格类型 (必填)")
	financeDigitalInvoiceGetTableCmd.Flags().String("data", "", "动态数据 JSON 对象")

	financeDigitalInvoiceImportGoodsCmd.Flags().String("company-code", "", "主体编码 (必填)")
	financeDigitalInvoiceImportGoodsCmd.Flags().String("items", "", "商品项目列表 JSON 数组 (必填)")

	financeDigitalInvoiceSearchGoodsCmd.Flags().String("company-code", "", "主体编码 (必填)")
	financeDigitalInvoiceSearchGoodsCmd.Flags().String("goods-name", "", "搜索关键词")

	financeDigitalInvoiceCmd.AddCommand(
		financeDigitalInvoiceLoginStatusCmd,
		financeDigitalInvoiceLoginPageCmd,
		financeDigitalInvoiceLoginCmd,
		financeDigitalInvoiceAccountCmd,
		financeDigitalInvoiceSmsCodeCmd,
		financeDigitalInvoiceGoodsCodeCmd,
		financeDigitalInvoiceFaceQrCmd,
		financeDigitalInvoiceFaceStatusCmd,
		financeDigitalInvoiceTitleCmd,
		financeDigitalInvoiceIssueCmd,
		financeDigitalInvoiceFileCmd,
		financeDigitalInvoiceSkillVersionCmd,
		financeDigitalInvoiceSendEmailCmd,
		financeDigitalInvoiceSendEmailSaasCmd,
		financeDigitalInvoiceBatchDrawCmd,
		financeDigitalInvoiceBatchDrawSaasCmd,
		financeDigitalInvoiceBatchDrawQueryCmd,
		financeDigitalInvoiceBatchDrawQuerySaasCmd,
		financeDigitalInvoiceGetTableCmd,
		financeDigitalInvoiceImportGoodsCmd,
		financeDigitalInvoiceSearchGoodsCmd,
	)

	// ── customer save ──
	financeCustomerSaveCmd.Flags().String("customer-name", "", "客户名称，用于档案展示 (必填)")
	financeCustomerSaveCmd.Flags().String("purchaser-name", "", "发票抬头，开票时填写到发票中 (必填)")
	financeCustomerSaveCmd.Flags().String("tax-no", "", "税号")
	customerCmd.AddCommand(financeCustomerSaveCmd)

	financePaymentListCmd.Flags().Float64("page-no", 1, "页码 (默认 1)")
	financePaymentListCmd.Flags().Float64("page-size", 20, "分页大小 (默认 20)")
	financePaymentListCmd.Flags().String("payee-account-no", "", "收款账户筛选")

	financePaymentAccountListCmd.Flags().Float64("page-no", 1, "页码 (默认 1)")
	financePaymentAccountListCmd.Flags().Float64("page-size", 20, "分页大小 (默认 20)")
	financePaymentAccountListCmd.Flags().String("query", "", "搜索关键字")

	financePaymentCashierURLCmd.Flags().String("instance-id", "", "单据 ID（单笔付款）")
	financePaymentCashierURLCmd.Flags().String("instance-ids", "", "单据 ID 列表（合并多笔付款，逗号分隔）")

	paymentCmd.AddCommand(
		financePaymentCreateCmd,
		financePaymentListCmd,
		financePaymentAccountListCmd,
		financePaymentCashierURLCmd,
		financePaymentAccountURLCmd,
		financePaymentPayerListCmd,
	)

	// ── process ──
	processFormDataCmd.Flags().String("business-id", "", "审批编号 (必填)")

	processListCmd.Flags().String("form-name", "", "审批模版名称 (必填)")
	processListCmd.Flags().Float64("page-no", 1, "页码 (默认 1)")
	processListCmd.Flags().Float64("page-size", 20, "分页大小 (默认 20)")
	processListCmd.Flags().String("start-time", "", "开始时间 (必填)")
	processListCmd.Flags().String("end-time", "", "结束时间 (必填)")
	processListCmd.Flags().String("status", "", `审批单状态列表 JSON 数组，如 '["COMPLETED","RUNNING"]'`)
	processListCmd.Flags().String("order-by", "", "排序字段，如 finishTime")

	processCmd.AddCommand(processFormDataCmd, processListCmd)

	// ── gather ──
	gatherSaveRuleCmd.Flags().String("process-code", "", "单据模版 code (必填)")
	gatherSaveRuleCmd.Flags().String("rules", "", "采集规则 (必填)")
	gatherSaveRuleCmd.Flags().String("table-field-id", "", "模版基准明细/表格 ID")

	gatherQueryRuleCmd.Flags().String("process-code", "", "单据模版 code")
	gatherQueryRuleCmd.Flags().String("process-name", "", "单据模版名称")

	gatherTryExecuteCmd.Flags().String("process-code", "", "单据模版 code")
	gatherTryExecuteCmd.Flags().String("business-id", "", "审批编号")
	gatherTryExecuteCmd.Flags().String("instance-id", "", "单据实例 ID")

	gatherExecuteCmd.Flags().String("instances", "", "单据列表 JSON 数组，每项含 businessId/instanceId/processCode (必填)")

	gatherExecuteGeneralCmd.Flags().String("data-list", "", "自定义报表数据列表 JSON 数组，每项必填 instanceId (必填)")

	gatherCmd.AddCommand(gatherSaveRuleCmd, gatherQueryRuleCmd, gatherTryExecuteCmd, gatherExecuteCmd, gatherExecuteGeneralCmd)

	root.AddCommand(
		receiptCmd, invoiceCmd, bankCmd, voucherCmd,
		customerCmd, accountCmd, journalCmd,
		supplierCmd, categoryCmd, paymentCmd,
		financeCompanyCmd, financeDigitalInvoiceCmd,
		processCmd, gatherCmd,
	)
	return root
}

// parseInvoiceType converts "专票"/"普票" or "8"/"9" to float64.
func parseInvoiceType(raw string) float64 {
	s := strings.TrimSpace(raw)
	switch s {
	case "8", "专票":
		return 8
	case "9", "普票":
		return 9
	default:
		return 9
	}
}
