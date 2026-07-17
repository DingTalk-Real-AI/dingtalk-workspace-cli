// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
)

func newProfileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "组织 profile 管理",
		Long: `管理本机已登录的钉钉账号 profile。

每个 profile 由 corpId + userId 唯一确定，同一组织可保存多个账号。业务命令可通过
全局 --profile 临时指定组织或账号，profile switch/use 才会持久修改默认账号。`,
		Example: `  dws profile list
  dws profile switch
  dws profile switch <corpId>
  dws profile switch <corpId>:<userId>
  dws profile switch "<corpName>:<userName>"
  dws profile switch -
  dws --profile <corpId>:<userId> contact user get-self`,
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newProfileListCommand(), newProfileSwitchCommand(), newProfileUseCommand())
	return cmd
}

func newProfileListCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "列出全部已登录账号 profile",
		Long:    "列出本机全部已登录账号。状态和到期时间直接读取各身份 Token，列表本身不会刷新 Token。",
		Example: `  dws profile list
  dws profile list --format json`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configDir := defaultConfigDir()
			if err := profileEnsureProfilesMigration(configDir); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to migrate profiles: %v", err))
			}
			cfg, err := profileLoadProfiles(configDir)
			if err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to load profiles: %v", err))
			}
			format, _ := cmd.Root().PersistentFlags().GetString("format")
			if strings.EqualFold(strings.TrimSpace(format), "json") {
				return writeProfileListJSON(cmd.OutOrStdout(), configDir, cfg)
			}
			writeProfileListTable(cmd.OutOrStdout(), configDir, cfg)
			return nil
		},
	}
}

func newProfileUseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use [profile-selector|-]",
		Short: "切换当前账号 profile（兼容 profile switch）",
		Long:  "兼容命令，语义等同于 dws profile switch。选择器支持组织 ID/名称、账号 ID/名称组合或本地 profile 名；- 切回上一个账号。",
		Example: `  dws profile use <corpId>
  dws profile use --name "钉钉"
  dws profile use -`,
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfileSwitchCommand(cmd, args)
		},
	}
	addProfileSwitchSelectorFlags(cmd)
	return cmd
}

func newProfileSwitchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "switch [profile-selector|-]",
		Short: "切换当前账号 profile",
		Long: `切换默认账号 profile，并记录 previousProfile 以支持 dws profile switch - 快速切回。

选择器支持 corpId:userId、corpId:userName、corpName:userId、corpName:userName，
也兼容单独的 corpId、唯一 corpName 和本地 profile 名。组织或账号名称重名时会报错，
要求改用稳定的 corpId:userId。不带参数时交互选择；单次执行请使用全局 --profile。`,
		Example: `  dws profile switch
  dws profile switch <corpId>
  dws profile switch <corpId>:<userId>
  dws profile switch "<corpName>:<userName>"
  dws profile switch --corpId <corpId>
  dws profile switch --name "钉钉"
  dws profile switch -
  dws --profile <corpId>:<userId> contact user get-self`,
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfileSwitchCommand(cmd, args)
		},
	}
	addProfileSwitchSelectorFlags(cmd)
	return cmd
}

func addProfileSwitchSelectorFlags(cmd *cobra.Command) {
	cmd.Flags().String("corpId", "", "按 corpId 直接切换组织 profile")
	cmd.Flags().String("corp-id", "", "按 corpId 直接切换组织 profile")
	cmd.Flags().String("corpid", "", "按 corpId 直接切换组织 profile")
	cmd.Flags().String("corp", "", "按 corpId 直接切换组织 profile")
	cmd.Flags().String("name", "", "按组织名或 profile 名直接切换组织 profile")
	_ = cmd.Flags().MarkHidden("corp-id")
	_ = cmd.Flags().MarkHidden("corpid")
	_ = cmd.Flags().MarkHidden("corp")
}

var (
	profileSwitchSelector            = selectProfileSwitchProfile
	profileSwitchInteractiveTerminal = isInteractiveTerminal
	profileSwitchTUIRunner           = runProfileSwitchTUI
	profileEnsureProfilesMigration   = authpkg.EnsureProfilesMigration
	profileLoadProfiles              = authpkg.LoadProfiles
	profileLoadTokenData             = authpkg.LoadTokenDataForProfile
	profileUsePrevious               = authpkg.UsePreviousProfile
	profileSetCurrent                = authpkg.SetCurrentProfile
	profileRunTeaProgram             = (*tea.Program).Run
)

const (
	profileSwitchVisibleOptions = 5
	profileSwitchCellPadding    = 1
	profileSwitchOrgWidth       = 34
	profileSwitchStatusWidth    = 10
)

var profileSwitchRenderer = newProfileSwitchRenderer()

func newProfileSwitchRenderer() *lipgloss.Renderer {
	renderer := lipgloss.NewRenderer(io.Discard)
	renderer.SetColorProfile(termenv.TrueColor)
	renderer.SetHasDarkBackground(true)
	return renderer
}

func runProfileSwitchCommand(cmd *cobra.Command, args []string) error {
	configDir := defaultConfigDir()
	selector, err := profileSwitchSelectorFromCommand(cmd, args)
	if err != nil {
		return err
	}
	usedTUI := false
	if selector == "" {
		selector, err = profileSwitchSelector(cmd, configDir)
		if err != nil {
			return err
		}
		usedTUI = true
	}
	return switchProfileAndWrite(cmd, configDir, selector, usedTUI)
}

func profileSwitchSelectorFromCommand(cmd *cobra.Command, args []string) (string, error) {
	selectors := make([]string, 0, 2)
	if len(args) > 0 {
		selectors = append(selectors, strings.TrimSpace(args[0]))
	}
	for _, name := range []string{"corpId", "corp-id", "corpid", "corp", "name"} {
		value, changed := changedStringFlag(cmd, name)
		if !changed {
			continue
		}
		if value == "" {
			return "", apperrors.NewValidation(fmt.Sprintf("--%s 不能为空", name))
		}
		selectors = append(selectors, value)
	}
	if len(selectors) == 0 {
		return "", nil
	}
	selector := selectors[0]
	for _, candidate := range selectors[1:] {
		if candidate != selector {
			return "", apperrors.NewValidation("只能指定一个组织选择器，请使用位置参数或 --corpId/--name 其中一种")
		}
	}
	return selector, nil
}

func changedStringFlag(cmd *cobra.Command, name string) (string, bool) {
	if cmd == nil || cmd.Flags() == nil {
		return "", false
	}
	flag := cmd.Flags().Lookup(name)
	if flag == nil || !flag.Changed {
		return "", false
	}
	return strings.TrimSpace(flag.Value.String()), true
}

func switchProfileAndWrite(cmd *cobra.Command, configDir, selector string, usedTUI bool) error {
	var (
		profile *authpkg.Profile
		err     error
	)
	if strings.TrimSpace(selector) == "-" {
		profile, err = profileUsePrevious(configDir)
	} else {
		profile, err = profileSetCurrent(configDir, selector)
	}
	if err != nil {
		return apperrors.NewValidation(err.Error())
	}
	ResetRuntimeTokenCache()
	clearCompatCache()
	format, _ := cmd.Root().PersistentFlags().GetString("format")
	if strings.EqualFold(strings.TrimSpace(format), "json") && !(usedTUI && authLoginAllowsInteractiveDefault(cmd, format)) {
		cfg, loadErr := profileLoadProfiles(configDir)
		if loadErr != nil {
			return apperrors.NewInternal(fmt.Sprintf("failed to load profiles: %v", loadErr))
		}
		return writeProfileUseJSON(cmd.OutOrStdout(), profile, cfg)
	}
	fmt.Fprintln(cmd.OutOrStdout(), profileUseMessage(profile))
	return nil
}

func selectProfileSwitchProfile(cmd *cobra.Command, configDir string) (string, error) {
	if !profileSwitchInteractiveTerminal() {
		return "", apperrors.NewValidation("profile selector required in non-interactive mode; use dws profile switch <corpId|corpId:userId|corpName:userName>")
	}
	if err := profileEnsureProfilesMigration(configDir); err != nil {
		return "", apperrors.NewInternal(fmt.Sprintf("failed to migrate profiles: %v", err))
	}
	cfg, err := profileLoadProfiles(configDir)
	if err != nil {
		return "", apperrors.NewInternal(fmt.Sprintf("failed to load profiles: %v", err))
	}
	if cfg == nil || len(cfg.Profiles) == 0 {
		return "", apperrors.NewValidation("未找到已登录 profile，请先运行 dws auth login")
	}
	choice := strings.TrimSpace(cfg.CurrentProfile)
	if choice == "" {
		choice = authpkg.ProfileSelector(cfg.Profiles[0])
	}
	return profileSwitchTUIRunner(cmd, cfg, choice)
}

func runProfileSwitchTUI(cmd *cobra.Command, cfg *authpkg.ProfilesConfig, selectedCorpID string) (string, error) {
	model := newProfileSwitchTUIModel(cfg, selectedCorpID)
	program := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithInput(cmd.InOrStdin()),
		tea.WithOutput(cmd.ErrOrStderr()),
		tea.WithContext(cmd.Context()),
	)
	finalModel, err := profileRunTeaProgram(program)
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			return "", apperrors.NewValidation("组织选择中止: user aborted")
		}
		return "", apperrors.NewInternal(fmt.Sprintf("failed to run profile selector: %v", err))
	}
	final, ok := finalModel.(profileSwitchTUIModel)
	if !ok || final.aborted || !final.submitted {
		return "", apperrors.NewValidation("组织选择中止: user aborted")
	}
	return final.selectedCorpID(), nil
}

type profileSwitchTUIModel struct {
	cfg       *authpkg.ProfilesConfig
	profiles  []authpkg.Profile
	selected  int
	offset    int
	submitted bool
	aborted   bool
}

func newProfileSwitchTUIModel(cfg *authpkg.ProfilesConfig, selectedCorpID string) profileSwitchTUIModel {
	model := profileSwitchTUIModel{cfg: cfg}
	if cfg != nil {
		model.profiles = profileSwitchSortedProfiles(cfg.Profiles)
	}
	model.selected = profileSwitchProfileIndex(model.profiles, selectedCorpID, cfg)
	if model.selected < 0 {
		model.selected = 0
	}
	model.ensureSelectedVisible()
	return model
}

func profileSwitchSortedProfiles(profiles []authpkg.Profile) []authpkg.Profile {
	return append([]authpkg.Profile(nil), profiles...)
}

func (m profileSwitchTUIModel) Init() tea.Cmd {
	return nil
}

func (m profileSwitchTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.aborted = true
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
				m.ensureSelectedVisible()
			}
		case "down", "j":
			if m.selected < len(m.profiles)-1 {
				m.selected++
				m.ensureSelectedVisible()
			}
		case "enter":
			m.submitted = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m profileSwitchTUIModel) View() string {
	var b strings.Builder
	title := profileSwitchTitleStyle().Render("选择要切换的组织")
	hint := profileSwitchMutedStyle().Render("全部已登录 profile，↑↓ 选择，Enter 确认")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(hint)
	b.WriteString("\n\n")
	b.WriteString(m.tableView())
	b.WriteString("\n")
	b.WriteString(profileSwitchMutedStyle().Render("↑/k up • ↓/j down • enter submit • esc cancel"))
	return b.String()
}

func (m profileSwitchTUIModel) tableView() string {
	rows := []string{
		profileSwitchBorder("┌", "┬", "┐"),
		profileSwitchStyledTableLine("组织名", "本地状态", profileSwitchHeaderStyle()),
		profileSwitchBorder("├", "┼", "┤"),
	}
	for i := 0; i < profileSwitchVisibleOptions; i++ {
		idx := m.offset + i
		if idx >= 0 && idx < len(m.profiles) {
			rows = append(rows, m.profileRow(idx))
			continue
		}
		rows = append(rows, profileSwitchStyledTableLine("", "", profileSwitchNormalRowStyle()))
	}
	rows = append(rows, profileSwitchBorder("└", "┴", "┘"))
	return strings.Join(rows, "\n")
}

func (m profileSwitchTUIModel) profileRow(idx int) string {
	profile := m.profiles[idx]
	org, status := profileSwitchProfileCells(profile, m.cfg)
	style := profileSwitchNormalRowStyle()
	if idx == m.selected {
		org = "› " + org
		style = profileSwitchSelectedRowStyle()
	} else {
		org = "  " + org
	}
	return profileSwitchStyledTableLine(org, status, style)
}

func (m *profileSwitchTUIModel) ensureSelectedVisible() {
	if len(m.profiles) == 0 {
		m.selected = 0
		m.offset = 0
		return
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.profiles) {
		m.selected = len(m.profiles) - 1
	}
	if m.selected < m.offset {
		m.offset = m.selected
	}
	if m.selected >= m.offset+profileSwitchVisibleOptions {
		m.offset = m.selected - profileSwitchVisibleOptions + 1
	}
	maxOffset := len(m.profiles) - profileSwitchVisibleOptions
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m profileSwitchTUIModel) selectedCorpID() string {
	if m.selected < 0 || m.selected >= len(m.profiles) {
		return ""
	}
	return authpkg.ProfileSelector(m.profiles[m.selected])
}

func profileSwitchProfileIndex(profiles []authpkg.Profile, selector string, cfg *authpkg.ProfilesConfig) int {
	selector = strings.TrimSpace(selector)
	if corpID, userID, exact := authpkg.ParseIdentitySelector(selector); exact {
		for i, p := range profiles {
			if strings.TrimSpace(p.CorpID) == corpID && strings.TrimSpace(p.UserID) == userID {
				return i
			}
		}
		return -1
	}
	fallback := -1
	for i, p := range profiles {
		if strings.TrimSpace(p.CorpID) == selector {
			if fallback < 0 {
				fallback = i
			}
			if profileIsOrgCurrent(p, cfg) {
				return i
			}
		}
	}
	return fallback
}

func profileSwitchOptionLabel(p authpkg.Profile, cfg *authpkg.ProfilesConfig) string {
	org, status := profileSwitchProfileCells(p, cfg)
	if status == "" {
		return org
	}
	return strings.Join([]string{org, status}, " | ")
}

func profileSwitchProfileCells(p authpkg.Profile, cfg *authpkg.ProfilesConfig) (string, string) {
	orgName := profileOrgName(p)
	if cfg != nil {
		sameCorp := 0
		for _, candidate := range cfg.Profiles {
			if candidate.CorpID == p.CorpID {
				sameCorp++
			}
		}
		if sameCorp > 1 {
			user := strings.TrimSpace(p.UserName)
			if user == "" {
				user = strings.TrimSpace(p.UserID)
			}
			if user != "" {
				orgName += " / " + user
			}
		}
	}
	return orgName, profileSwitchProfileStatus(p, cfg)
}

func profileSwitchProfileStatus(p authpkg.Profile, cfg *authpkg.ProfilesConfig) string {
	if cfg != nil && profileSelectorSelectsProfile(cfg.CurrentProfile, p, profileIsOrgCurrent(p, cfg), profileCountForCorp(cfg, p.CorpID) <= 1) {
		return "当前组织"
	}
	return ""
}

func profileSwitchBorder(left, sep, right string) string {
	segments := []string{
		strings.Repeat("─", profileSwitchCellWidth(profileSwitchOrgWidth)),
		strings.Repeat("─", profileSwitchCellWidth(profileSwitchStatusWidth)),
	}
	return profileSwitchBorderStyle().Render(left + strings.Join(segments, sep) + right)
}

func profileSwitchTableLine(org, status string) string {
	cells := []string{
		profileSwitchTableCell(org, profileSwitchOrgWidth),
		profileSwitchTableCell(status, profileSwitchStatusWidth),
	}
	return "│" + strings.Join(cells, "│") + "│"
}

func profileSwitchStyledTableLine(org, status string, style lipgloss.Style) string {
	cells := []string{
		style.Render(profileSwitchTableCell(org, profileSwitchOrgWidth)),
		style.Render(profileSwitchTableCell(status, profileSwitchStatusWidth)),
	}
	return profileSwitchTableSeparator() + strings.Join(cells, profileSwitchTableSeparator()) + profileSwitchTableSeparator()
}

func profileSwitchTableSeparator() string {
	return profileSwitchBorderStyle().Render("│")
}

func profileSwitchTableCell(value string, width int) string {
	clipped := clipProfileDisplayCell(strings.TrimSpace(value), width)
	padding := strings.Repeat(" ", profileSwitchCellPadding)
	return padding + padProfileDisplayCell(clipped, width) + padding
}

func padProfileDisplayCell(value string, width int) string {
	padding := width - lipgloss.Width(value)
	if padding < 0 {
		padding = 0
	}
	return value + strings.Repeat(" ", padding)
}

func profileSwitchCellWidth(contentWidth int) int {
	return contentWidth + profileSwitchCellPadding*2
}

func profileSwitchSelectedRowStyle() lipgloss.Style {
	return lipgloss.NewStyle().Renderer(profileSwitchRenderer).Foreground(lipgloss.Color("#69B1FF")).Bold(true)
}

func profileSwitchNormalRowStyle() lipgloss.Style {
	return lipgloss.NewStyle().Renderer(profileSwitchRenderer).Foreground(lipgloss.Color("#FFFFFF"))
}

func profileSwitchHeaderStyle() lipgloss.Style {
	return profileSwitchMutedStyle().Bold(true)
}

func profileSwitchBorderStyle() lipgloss.Style {
	return lipgloss.NewStyle().Renderer(profileSwitchRenderer).Foreground(lipgloss.Color("#2F3B52"))
}

func profileSwitchTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Renderer(profileSwitchRenderer).Foreground(lipgloss.Color("#69B1FF")).Bold(true)
}

func profileSwitchMutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Renderer(profileSwitchRenderer).Foreground(lipgloss.Color("#8A96A8"))
}

type profileListResponse struct {
	Success         bool          `json:"success"`
	PrimaryProfile  string        `json:"primaryProfile,omitempty"`
	CurrentProfile  string        `json:"currentProfile,omitempty"`
	PreviousProfile string        `json:"previousProfile,omitempty"`
	Profiles        []profileView `json:"profiles"`
}

type profileUseResponse struct {
	Success bool        `json:"success"`
	Profile profileView `json:"profile"`
}

type profileView struct {
	Profile           string   `json:"profile"`
	CorpID            string   `json:"corpId"`
	CorpName          string   `json:"corpName"`
	UserID            string   `json:"userId,omitempty"`
	UserName          string   `json:"userName,omitempty"`
	ClientID          string   `json:"clientId,omitempty"`
	Status            string   `json:"status,omitempty"`
	AuthorizedDomains []string `json:"authorizedDomains,omitempty"`
	ExpiresAt         string   `json:"expiresAt,omitempty"`
	RefreshExpAt      string   `json:"refreshExpAt,omitempty"`
	LastLoginAt       string   `json:"lastLoginAt,omitempty"`
	LastUsedAt        string   `json:"lastUsedAt,omitempty"`
	IsPrimary         bool     `json:"isPrimary"`
	IsCurrent         bool     `json:"isCurrent"`
	IsOrgCurrent      bool     `json:"isOrgCurrent"`
}

func writeProfileListJSON(w io.Writer, configDir string, cfg *authpkg.ProfilesConfig) error {
	resp := profileListResponse{
		Success:         true,
		PrimaryProfile:  cfg.PrimaryProfile,
		CurrentProfile:  cfg.CurrentProfile,
		PreviousProfile: cfg.PreviousProfile,
		Profiles:        profileViews(configDir, cfg),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

func writeProfileUseJSON(w io.Writer, profile *authpkg.Profile, cfg *authpkg.ProfilesConfig) error {
	resp := profileUseResponse{Success: true}
	if profile != nil {
		primaryProfile := ""
		currentProfile := ""
		if cfg != nil {
			primaryProfile = cfg.PrimaryProfile
			currentProfile = cfg.CurrentProfile
		}
		resp.Profile = profileViewFromProfile(
			*profile,
			cfg,
			primaryProfile,
			currentProfile,
			profileCountForCorp(cfg, profile.CorpID) <= 1,
			nil,
		)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

func writeProfileListTable(w io.Writer, configDir string, cfg *authpkg.ProfilesConfig) {
	if cfg == nil || len(cfg.Profiles) == 0 {
		fmt.Fprintln(w, "未找到已登录 profile")
		return
	}
	fmt.Fprintf(w, "%-3s %-28s %-34s %-10s %s\n", "CUR", "ORG_NAME", "CORP_ID", "STATUS", "USER")
	for _, p := range cfg.Profiles {
		view := profileViewFromProfile(
			p,
			cfg,
			cfg.PrimaryProfile,
			cfg.CurrentProfile,
			profileCountForCorp(cfg, p.CorpID) == 1,
			loadProfileTokenState(configDir, p),
		)
		current := ""
		if view.IsCurrent {
			current = "*"
		}
		user := p.UserName
		if user == "" {
			user = p.UserID
		}
		fmt.Fprintf(
			w,
			"%-3s %-28s %-34s %-10s %s\n",
			current,
			clipProfileCell(profileOrgName(p), 28),
			clipProfileCell(p.CorpID, 34),
			view.Status,
			user,
		)
	}
}

func profileUseMessage(profile *authpkg.Profile) string {
	if profile == nil {
		return "[OK] 当前 profile 已切换"
	}
	corpID := strings.TrimSpace(profile.CorpID)
	orgName := strings.TrimSpace(profile.CorpName)
	if orgName == "" {
		orgName = profileOrgName(*profile)
	}
	return fmt.Sprintf("[OK] 当前组织: %s (%s)", orgName, corpID)
}

func profileOrgName(p authpkg.Profile) string {
	if v := strings.TrimSpace(p.CorpName); v != "" {
		return v
	}
	if v := strings.TrimSpace(p.Name); v != "" {
		return v
	}
	return strings.TrimSpace(p.CorpID)
}

type profileTokenState struct {
	Status       string
	ExpiresAt    string
	RefreshExpAt string
}

func profileViews(configDir string, cfg *authpkg.ProfilesConfig) []profileView {
	if cfg == nil {
		return nil
	}
	views := make([]profileView, 0, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		views = append(views, profileViewFromProfile(
			p,
			cfg,
			cfg.PrimaryProfile,
			cfg.CurrentProfile,
			profileCountForCorp(cfg, p.CorpID) == 1,
			loadProfileTokenState(configDir, p),
		))
	}
	return views
}

func profileViewFromProfile(
	p authpkg.Profile,
	cfg *authpkg.ProfilesConfig,
	primaryProfile, currentProfile string,
	onlyAccountInOrg bool,
	tokenState *profileTokenState,
) profileView {
	isOrgCurrent := profileIsOrgCurrent(p, cfg)
	view := profileView{
		Profile:           authpkg.ProfileSelector(p),
		CorpID:            p.CorpID,
		CorpName:          profileOrgName(p),
		UserID:            p.UserID,
		UserName:          p.UserName,
		ClientID:          p.ClientID,
		Status:            p.Status,
		AuthorizedDomains: p.AuthorizedDomains,
		ExpiresAt:         p.ExpiresAt,
		RefreshExpAt:      p.RefreshExpAt,
		LastLoginAt:       p.LastLoginAt,
		LastUsedAt:        p.LastUsedAt,
		IsPrimary:         profileSelectorSelectsProfile(primaryProfile, p, isOrgCurrent, onlyAccountInOrg),
		IsCurrent:         profileSelectorSelectsProfile(currentProfile, p, isOrgCurrent, onlyAccountInOrg),
		IsOrgCurrent:      isOrgCurrent,
	}
	if tokenState != nil {
		view.Status = tokenState.Status
		view.ExpiresAt = tokenState.ExpiresAt
		view.RefreshExpAt = tokenState.RefreshExpAt
	}
	return view
}

func loadProfileTokenState(configDir string, profile authpkg.Profile) *profileTokenState {
	data, err := profileLoadTokenData(configDir, authpkg.ProfileSelector(profile))
	if errors.Is(err, authpkg.ErrTokenDataNotFound) || (err == nil && data == nil) {
		return &profileTokenState{Status: authpkg.ProfileStatusRevoked}
	}
	if err != nil {
		return &profileTokenState{Status: authpkg.ProfileStatusUnavailable}
	}
	status := authpkg.ProfileStatusExpired
	if data.IsAccessTokenValid() {
		status = authpkg.ProfileStatusActive
	}
	return &profileTokenState{
		Status:       status,
		ExpiresAt:    profileTokenTime(data.ExpiresAt),
		RefreshExpAt: profileTokenTime(data.RefreshExpAt),
	}
}

func profileTokenTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func profileSelectorSelectsProfile(selector string, profile authpkg.Profile, isOrgCurrent, onlyAccountInOrg bool) bool {
	selector = strings.TrimSpace(selector)
	if corpID, userID, exact := authpkg.ParseIdentitySelector(selector); exact {
		return corpID == strings.TrimSpace(profile.CorpID) && userID == strings.TrimSpace(profile.UserID)
	}
	return selector == strings.TrimSpace(profile.CorpID) && (isOrgCurrent || onlyAccountInOrg)
}

func profileCountForCorp(cfg *authpkg.ProfilesConfig, corpID string) int {
	if cfg == nil {
		return 0
	}
	count := 0
	for _, profile := range cfg.Profiles {
		if strings.TrimSpace(profile.CorpID) == strings.TrimSpace(corpID) {
			count++
		}
	}
	return count
}

func profileIsOrgCurrent(profile authpkg.Profile, cfg *authpkg.ProfilesConfig) bool {
	if cfg == nil {
		return false
	}
	selector := strings.TrimSpace(cfg.OrgCurrentProfiles[strings.TrimSpace(profile.CorpID)])
	if corpID, userID, exact := authpkg.ParseIdentitySelector(selector); exact {
		return corpID == strings.TrimSpace(profile.CorpID) && userID == strings.TrimSpace(profile.UserID)
	}
	return profileCountForCorp(cfg, profile.CorpID) == 1
}

func clipProfileCell(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

func clipProfileDisplayCell(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= limit {
		return value
	}
	if limit <= 3 {
		var b strings.Builder
		for _, r := range value {
			rw := lipgloss.Width(string(r))
			if lipgloss.Width(b.String())+rw > limit {
				break
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	target := limit - 3
	var b strings.Builder
	width := 0
	for _, r := range value {
		rw := lipgloss.Width(string(r))
		if width+rw > target {
			break
		}
		b.WriteRune(r)
		width += rw
	}
	return b.String() + "..."
}
