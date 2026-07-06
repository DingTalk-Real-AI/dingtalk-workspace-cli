package helpers

import (
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws credit — 芝麻企业信用
// ──────────────────────────────────────────────────────────

func newCreditCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "credit",
		Short: "企业信用查询 (芝麻企业信用)",
		Long:  `芝麻企业信用查询：企业工商信息、风险信息、知识产权、股权、招投标、联系方式。`,
		RunE:  groupRunE,
	}

	// ── 企业搜索 (所有服务共享) ──────────────────────────────

	creditSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "企业名称搜索",
		Long:  `输入企业名称关键词，模糊搜索，返回命中的企业统代信息。`,
		Example: `  # 企业名称，由用户提供
  dws credit search --name "阿里巴巴"
  dws credit search --name "腾讯" --size 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "name"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"company_name": mustGetFlag(cmd, "name"),
			}
			addCreditPagination(cmd, toolArgs)
			return callMCPToolOnServer("credit-ep", "ep_info_search_query", toolArgs)
		},
	}

	// ── 查企业 (credit-ep) ─────────────────────────────────

	creditInfoCmd := &cobra.Command{
		Use:     "info",
		Short:   "企业基本工商信息",
		Long:    `获取企业基本工商登记信息，包括统代、注册号、成立日期、注册地址、公司类型、经营范围等。`,
		Example: `  dws credit info --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-ep", "ep_dossier_basicinfo_query"),
	}

	creditMemberCmd := &cobra.Command{
		Use:     "member",
		Short:   "企业成员信息",
		Long:    `获取企业工商登记的成员信息，包括职务、姓名等。`,
		Example: `  dws credit member --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-ep", "ep_dossier_member_query"),
	}

	creditChangeCmd := &cobra.Command{
		Use:     "change",
		Short:   "工商变更信息",
		Long:    `获取企业工商变更信息，包括变更类型、变更前后信息、变更日期等。`,
		Example: `  dws credit change --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-ep", "ep_dossier_reginfochange_query"),
	}

	creditAnnualCmd := &cobra.Command{
		Use:     "annual",
		Short:   "工商年报",
		Long:    `获取企业公开披露的工商年报信息。`,
		Example: `  dws credit annual --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-ep", "ep_dossier_annualreport_query"),
	}

	creditLicenseCmd := &cobra.Command{
		Use:     "license",
		Short:   "行政许可信息",
		Long:    `获取企业的行政许可信息，包括许可类型、许可机构、许可日期等。`,
		Example: `  dws credit license --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-ep", "ep_dossier_license_query"),
	}

	creditCertificateCmd := &cobra.Command{
		Use:     "cert-info",
		Short:   "资质证照信息",
		Long:    `获取企业的资质证照信息，包括证照类型、发证机构、发证日期等。`,
		Example: `  dws credit cert-info --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-ep", "ep_dossier_certificate_query"),
	}

	creditBranchCmd := &cobra.Command{
		Use:     "branch",
		Short:   "分支机构信息",
		Long:    `获取企业的分支机构信息，包括分支机构名称、成立日期等。`,
		Example: `  dws credit branch --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-ep", "ep_dossier_branch_query"),
	}

	// ── 查风险 (credit-risk) ────────────────────────────────

	creditRiskCmd := &cobra.Command{
		Use:   "risk",
		Short: "企业风险信息",
		Long:  `查询企业风险信息：裁判文书、被执行、失信、涉诉、行政处罚、欠税等。`,
		RunE:  groupRunE,
	}

	creditRiskVerdictCmd := &cobra.Command{
		Use:     "verdict",
		Short:   "裁判文书",
		Long:    `获取企业的裁判文书信息，包括案由、案号、案件类型、文书类型、审理法院等。`,
		Example: `  dws credit risk verdict --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-risk", "ep_dossier_verdict_query"),
	}

	creditRiskExecuteCmd := &cobra.Command{
		Use:     "execute",
		Short:   "被执行信息",
		Long:    `获取企业的被执行信息，包括被执行金额、日期、审理法院、案号等。`,
		Example: `  dws credit risk execute --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-risk", "ep_dossier_execute_query"),
	}

	creditRiskDishonestCmd := &cobra.Command{
		Use:     "dishonest",
		Short:   "失信被执行",
		Long:    `获取企业的失信被执行信息，包括审理法院、案号、被执行日期等。`,
		Example: `  dws credit risk dishonest --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-risk", "ep_dossier_dishonest_query"),
	}

	creditRiskLitigationCmd := &cobra.Command{
		Use:     "litigation",
		Short:   "涉诉公告",
		Long:    `获取企业的涉诉公告信息，包括公告类型、公告内容、公告日期、审理法院等。`,
		Example: `  dws credit risk litigation --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-risk", "ep_dossier_litigation_query"),
	}

	creditRiskFinalcaseCmd := &cobra.Command{
		Use:     "finalcase",
		Short:   "终本案件",
		Long:    `获取企业的终本案件信息，包括案号、终本原因、日期等。`,
		Example: `  dws credit risk finalcase --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-risk", "ep_dossier_finalcase_query"),
	}

	creditRiskConsumCmd := &cobra.Command{
		Use:     "consum",
		Short:   "限制高消费",
		Long:    `获取企业的限制高消费信息，包括审理法院、案号、被执行日期等。`,
		Example: `  dws credit risk consum --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-risk", "ep_dossier_consum_query"),
	}

	creditRiskCourtCmd := &cobra.Command{
		Use:     "court",
		Short:   "开庭公告",
		Long:    `获取企业的开庭公告信息，包括案由、审理法院、开庭日期等。`,
		Example: `  dws credit risk court --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-risk", "ep_dossier_courtnotice_query"),
	}

	creditRiskAssistCmd := &cobra.Command{
		Use:     "assist",
		Short:   "司法协助",
		Long:    `获取企业的司法协助信息，包括案号、执行日期、执行内容等。`,
		Example: `  dws credit risk assist --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-risk", "ep_dossier_legalassist_query"),
	}

	creditRiskPenaltyCmd := &cobra.Command{
		Use:     "penalty",
		Short:   "行政处罚",
		Long:    `获取企业的行政处罚信息，包括处罚机构、处罚内容、处罚日期等。`,
		Example: `  dws credit risk penalty --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-risk", "ep_dossier_adminpenalty_query"),
	}

	creditRiskOwetaxCmd := &cobra.Command{
		Use:     "owetax",
		Short:   "催缴欠税",
		Long:    `获取企业的催缴欠税信息，包括欠税日期、欠税金额等。`,
		Example: `  dws credit risk owetax --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-risk", "ep_dossier_owetax_query"),
	}

	creditRiskTaxviolationCmd := &cobra.Command{
		Use:     "taxviolation",
		Short:   "重大税收违法",
		Long:    `获取企业的重大税收违法信息，包括处罚日期、处罚内容等。`,
		Example: `  dws credit risk taxviolation --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-risk", "ep_dossier_taxviolation_query"),
	}

	creditRiskPledgeCmd := &cobra.Command{
		Use:     "pledge",
		Short:   "股权出质",
		Long:    `获取企业在公示系统公示的股权出质信息，包括出质日期、出质比例、出质方等。`,
		Example: `  dws credit risk pledge --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-risk", "ep_dossier_equitypledge_query"),
	}

	// ── 查知识产权 (credit-ip) ──────────────────────────────

	creditIPCmd := &cobra.Command{
		Use:   "ip",
		Short: "知识产权信息",
		Long:  `查询企业知识产权：商标、专利、著作权、ICP备案。`,
		RunE:  groupRunE,
	}

	creditIPTrademarkCmd := &cobra.Command{
		Use:     "trademark",
		Short:   "商标信息",
		Long:    `获取企业的商标信息。`,
		Example: `  dws credit ip trademark --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-ip", "ep_dossier_trademark_query"),
	}

	creditIPPatentCmd := &cobra.Command{
		Use:     "patent",
		Short:   "专利信息",
		Long:    `获取企业的专利信息。`,
		Example: `  dws credit ip patent --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-ip", "ep_dossier_patent_query"),
	}

	creditIPCopyrightCmd := &cobra.Command{
		Use:     "copyright",
		Short:   "著作权信息",
		Long:    `获取企业的著作权信息。`,
		Example: `  dws credit ip copyright --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-ip", "ep_dossier_copyright_query"),
	}

	creditIPICPCmd := &cobra.Command{
		Use:     "icp",
		Short:   "ICP备案信息",
		Long:    `获取企业的ICP备案信息。`,
		Example: `  dws credit ip icp --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-ip", "ep_dossier_icpregistration_query"),
	}

	// ── 查股权 (credit-equity) ──────────────────────────────

	creditEquityCmd := &cobra.Command{
		Use:   "equity",
		Short: "股权信息",
		Long:  `查询企业股权信息：股东信息、对外投资。`,
		RunE:  groupRunE,
	}

	creditEquityShareholderCmd := &cobra.Command{
		Use:     "shareholder",
		Short:   "股东信息",
		Long:    `获取企业工商登记的股东信息，包括股东名称、类型、出资比例、出资金额等。`,
		Example: `  dws credit equity shareholder --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-equity", "ep_dossier_shareholder_query"),
	}

	creditEquityInvestCmd := &cobra.Command{
		Use:     "invest",
		Short:   "对外投资",
		Long:    `获取企业的对外投资信息，包括投资企业名称、投资比例、投资日期等。`,
		Example: `  dws credit equity invest --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-equity", "ep_dossier_invest_query"),
	}

	// ── 查招标 (credit-bid) ─────────────────────────────────

	creditBiddingCmd := &cobra.Command{
		Use:     "bidding",
		Short:   "招投标信息",
		Long:    `获取企业的招投标信息。`,
		Example: `  dws credit bidding --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE:    creditCertRunner("credit-bid", "ep_dossier_bidding_query"),
	}

	// ── 查联系方式 (credit-contact) ─────────────────────────

	creditContactCmd := &cobra.Command{
		Use:     "kp",
		Short:   "KP联系人信息",
		Long:    `获取企业高可信的KP联系人信息，包括联系方式、置信度等。`,
		Example: `  dws credit kp --cert "91330100MA2CK6BX6X"  # 统一社会信用代码，由用户提供`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "cert"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"ep_cert_no": mustGetFlag(cmd, "cert"),
			}
			// kp 查询无分页
			return callMCPToolOnServer("credit-contact", "ep_contactinfo_ext_query", toolArgs)
		},
	}

	// search uses --name instead of --cert
	creditSearchCmd.Flags().String("name", "", "企业名称关键词 (必填)")
	creditSearchCmd.Flags().Int("page", 0, "分页起始位置 (>=1)")
	creditSearchCmd.Flags().Int("size", 0, "每页返回条数 (10-50)")

	// 查企业 subcommands
	epCmds := []*cobra.Command{
		creditInfoCmd, creditMemberCmd, creditChangeCmd,
		creditAnnualCmd, creditLicenseCmd, creditCertificateCmd, creditBranchCmd,
	}
	for _, c := range epCmds {
		addCreditCertFlags(c)
	}

	// 查风险 subcommands
	riskCmds := []*cobra.Command{
		creditRiskVerdictCmd, creditRiskExecuteCmd, creditRiskDishonestCmd,
		creditRiskLitigationCmd, creditRiskFinalcaseCmd, creditRiskConsumCmd,
		creditRiskCourtCmd, creditRiskAssistCmd, creditRiskPenaltyCmd,
		creditRiskOwetaxCmd, creditRiskTaxviolationCmd, creditRiskPledgeCmd,
	}
	for _, c := range riskCmds {
		addCreditCertFlags(c)
	}
	creditRiskCmd.AddCommand(riskCmds...)

	// 查知识产权 subcommands
	ipCmds := []*cobra.Command{
		creditIPTrademarkCmd, creditIPPatentCmd, creditIPCopyrightCmd, creditIPICPCmd,
	}
	for _, c := range ipCmds {
		addCreditCertFlags(c)
	}
	creditIPCmd.AddCommand(ipCmds...)

	// 查股权 subcommands
	equityCmds := []*cobra.Command{creditEquityShareholderCmd, creditEquityInvestCmd}
	for _, c := range equityCmds {
		addCreditCertFlags(c)
	}
	creditEquityCmd.AddCommand(equityCmds...)

	// 查招标
	addCreditCertFlags(creditBiddingCmd)

	// 查联系方式 (no pagination)
	creditContactCmd.Flags().String("cert", "", "企业统一社会信用代码/注册号/企业名 (必填)")

	root.AddCommand(
		creditSearchCmd,
		// 查企业
		creditInfoCmd, creditMemberCmd, creditChangeCmd,
		creditAnnualCmd, creditLicenseCmd, creditCertificateCmd, creditBranchCmd,
		// 子命令组
		creditRiskCmd,
		creditIPCmd,
		creditEquityCmd,
		// 独立命令
		creditBiddingCmd,
		creditContactCmd,
	)

	return root
}

// creditCertRunner returns a RunE func for standard cert+pagination queries.
func creditCertRunner(serverID, toolName string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := validateRequiredFlags(cmd, "cert"); err != nil {
			return err
		}
		toolArgs := map[string]any{
			"ep_cert_no": mustGetFlag(cmd, "cert"),
		}
		addCreditPagination(cmd, toolArgs)
		return callMCPToolOnServer(serverID, toolName, toolArgs)
	}
}

// addCreditPagination reads --page and --size flags and adds them to toolArgs.
func addCreditPagination(cmd *cobra.Command, toolArgs map[string]any) {
	if v, _ := cmd.Flags().GetInt("page"); v > 0 {
		toolArgs["page_index"] = v
	}
	if v, _ := cmd.Flags().GetInt("size"); v > 0 {
		toolArgs["page_size"] = v
	}
}

// addCreditCertFlags adds --cert, --page, --size to a command.
func addCreditCertFlags(cmd *cobra.Command) {
	cmd.Flags().String("cert", "", "企业注册号或统一社会信用代码 (必填)")
	cmd.Flags().Int("page", 0, "分页起始位置")
	cmd.Flags().Int("size", 0, "每页返回条数")
}
