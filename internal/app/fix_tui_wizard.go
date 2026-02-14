package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"bb-project/internal/domain"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type fixViewMode int

const (
	fixViewList fixViewMode = iota
	fixViewWizard
	fixViewSummary
)

type fixWizardDecision struct {
	RepoPath string
	Action   string
}

type fixWizardState struct {
	Queue []fixWizardDecision
	Index int

	RepoPath string
	RepoName string
	Branch   string
	Upstream string
	Action   string
	Risk     fixRiskSnapshot

	EnableCommitMessage bool
	CommitMessage       textinput.Model
	CommitFocused       bool

	EnableProjectName bool
	ProjectName       textinput.Model

	ShowGitignoreToggle bool
	GenerateGitignore   bool

	Visibility  domain.Visibility
	DefaultVis  domain.Visibility
	FocusArea   fixWizardFocusArea
	ActionFocus int

	BodyViewport viewport.Model
}

type fixSummaryResult struct {
	RepoName string
	Action   string
	Status   string
	Detail   string
}

const (
	fixWizardActionCancel = iota
	fixWizardActionSkip
	fixWizardActionApply
)

type fixWizardFocusArea int

const (
	fixWizardFocusActions fixWizardFocusArea = iota
	fixWizardFocusCommit
	fixWizardFocusProjectName
	fixWizardFocusGitignore
	fixWizardFocusVisibility
)

func isRiskyFixAction(action string) bool {
	switch action {
	case FixActionAbortOperation, FixActionPush, FixActionSetUpstreamPush, FixActionStageCommitPush, FixActionCreateProject, FixActionForkAndRetarget:
		return true
	default:
		return false
	}
}

func (m *fixTUIModel) startWizardQueue(queue []fixWizardDecision) {
	if len(queue) == 0 {
		return
	}
	m.viewMode = fixViewWizard
	m.wizard = fixWizardState{
		Queue: append([]fixWizardDecision(nil), queue...),
		Index: 0,
	}
	m.status = fmt.Sprintf("reviewing %d risky fix(es)", len(queue))
	m.errText = ""
	m.loadWizardCurrent()
}

func (m *fixTUIModel) loadWizardCurrent() {
	if m.wizard.Index < 0 || m.wizard.Index >= len(m.wizard.Queue) {
		m.completeWizard()
		return
	}
	decision := m.wizard.Queue[m.wizard.Index]
	repoName := filepath.Base(decision.RepoPath)
	repoRisk := fixRiskSnapshot{}
	branch := ""
	upstream := ""
	for _, repo := range m.repos {
		if repo.Record.Path == decision.RepoPath {
			repoName = repo.Record.Name
			repoRisk = repo.Risk
			branch = strings.TrimSpace(repo.Record.Branch)
			upstream = strings.TrimSpace(repo.Record.Upstream)
			break
		}
	}
	if m.app != nil {
		if refreshedRisk, err := collectFixRiskSnapshot(decision.RepoPath, m.app.Git); err == nil {
			repoRisk = refreshedRisk
		}
	}

	commitInput := textinput.New()
	commitInput.Placeholder = DefaultFixCommitMessage
	commitInput.SetValue("")
	commitInput.Blur()

	projectNamePlaceholder := strings.TrimSpace(repoName)
	if projectNamePlaceholder == "" {
		projectNamePlaceholder = filepath.Base(decision.RepoPath)
	}
	projectNameInput := textinput.New()
	projectNameInput.Placeholder = projectNamePlaceholder
	projectNameInput.SetValue("")
	projectNameInput.Blur()

	showGitignoreToggle := (decision.Action == FixActionStageCommitPush || decision.Action == FixActionCreateProject) &&
		repoRisk.MissingRootGitignore &&
		len(repoRisk.SuggestedGitignorePatterns) > 0

	m.wizard.RepoPath = decision.RepoPath
	m.wizard.RepoName = repoName
	m.wizard.Branch = branch
	m.wizard.Upstream = upstream
	m.wizard.Action = decision.Action
	m.wizard.Risk = repoRisk
	m.wizard.EnableCommitMessage = decision.Action == FixActionStageCommitPush
	m.wizard.CommitMessage = commitInput
	m.wizard.CommitFocused = false
	m.wizard.EnableProjectName = decision.Action == FixActionCreateProject
	m.wizard.ProjectName = projectNameInput
	m.wizard.ShowGitignoreToggle = showGitignoreToggle
	m.wizard.GenerateGitignore = showGitignoreToggle
	m.wizard.DefaultVis = m.detectDefaultVisibility()
	m.wizard.Visibility = m.wizard.DefaultVis
	m.wizard.FocusArea = fixWizardFocusActions
	if m.wizard.EnableCommitMessage {
		m.wizard.FocusArea = fixWizardFocusCommit
	}
	if m.wizard.EnableProjectName {
		m.wizard.FocusArea = fixWizardFocusProjectName
	}
	m.syncWizardFieldFocus()
	m.wizard.ActionFocus = fixWizardActionCancel
	m.syncWizardViewport()
}

func (m *fixTUIModel) completeWizard() {
	m.viewMode = fixViewSummary
	applied := 0
	skipped := 0
	failed := 0
	for _, item := range m.summaryResults {
		switch item.Status {
		case "applied":
			applied++
		case "skipped":
			skipped++
		case "failed":
			failed++
		}
	}
	m.status = fmt.Sprintf("applied %d, skipped %d, failed %d", applied, skipped, failed)
}

func (m *fixTUIModel) appendSummaryResult(action string, status string, detail string) {
	m.summaryResults = append(m.summaryResults, fixSummaryResult{
		RepoName: m.wizard.RepoName,
		Action:   fixActionLabel(action),
		Status:   status,
		Detail:   detail,
	})
}

func (m *fixTUIModel) advanceWizard() {
	m.wizard.Index++
	if m.wizard.Index >= len(m.wizard.Queue) {
		m.completeWizard()
		return
	}
	m.loadWizardCurrent()
}

func (m *fixTUIModel) skipWizardCurrent() {
	m.appendSummaryResult(m.wizard.Action, "skipped", "skipped by user")
	m.advanceWizard()
}

func (m *fixTUIModel) applyWizardCurrent() {
	if m.app == nil {
		m.appendSummaryResult(m.wizard.Action, "failed", "internal: app is not configured")
		m.advanceWizard()
		return
	}
	opts := fixApplyOptions{Interactive: true}
	if m.wizard.EnableProjectName {
		opts.CreateProjectName = strings.TrimSpace(m.wizard.ProjectName.Value())
	}
	if m.wizard.EnableCommitMessage {
		raw := strings.TrimSpace(m.wizard.CommitMessage.Value())
		if raw == "" || raw == DefaultFixCommitMessage {
			raw = "auto"
		}
		opts.CommitMessage = raw
	}
	if m.wizard.ShowGitignoreToggle && m.wizard.GenerateGitignore {
		opts.GenerateGitignore = true
		opts.GitignorePatterns = append([]string(nil), m.wizard.Risk.SuggestedGitignorePatterns...)
	}
	if m.wizard.Action == FixActionCreateProject &&
		(m.wizard.Visibility == domain.VisibilityPrivate || m.wizard.Visibility == domain.VisibilityPublic) {
		opts.CreateProjectVisibility = m.wizard.Visibility
	}

	if _, err := m.app.applyFixAction(m.includeCatalogs, m.wizard.RepoPath, m.wizard.Action, opts); err != nil {
		m.appendSummaryResult(m.wizard.Action, "failed", err.Error())
	} else {
		detail := ""
		if opts.GenerateGitignore {
			detail = "generated root .gitignore"
		}
		m.appendSummaryResult(m.wizard.Action, "applied", detail)
	}
	if err := m.refreshRepos(scanRefreshAlways); err != nil {
		m.errText = err.Error()
	}
	m.advanceWizard()
}

func (m *fixTUIModel) detectDefaultVisibility() domain.Visibility {
	if m.app == nil {
		return domain.VisibilityPrivate
	}
	cfg, _, err := m.app.loadContext()
	if err != nil {
		return domain.VisibilityPrivate
	}
	if strings.EqualFold(strings.TrimSpace(cfg.GitHub.DefaultVisibility), string(domain.VisibilityPublic)) {
		return domain.VisibilityPublic
	}
	return domain.VisibilityPrivate
}

func (m *fixTUIModel) syncWizardFieldFocus() {
	m.wizard.CommitFocused = false
	m.wizard.CommitMessage.Blur()
	m.wizard.ProjectName.Blur()
	switch m.wizard.FocusArea {
	case fixWizardFocusCommit:
		m.wizard.CommitFocused = true
		m.wizard.CommitMessage.Focus()
	case fixWizardFocusProjectName:
		m.wizard.ProjectName.Focus()
	}
}

func (m *fixTUIModel) wizardFocusOrder() []fixWizardFocusArea {
	order := make([]fixWizardFocusArea, 0, 5)
	if m.wizard.EnableCommitMessage {
		order = append(order, fixWizardFocusCommit)
	}
	if m.wizard.EnableProjectName {
		order = append(order, fixWizardFocusProjectName)
	}
	if m.wizard.ShowGitignoreToggle {
		order = append(order, fixWizardFocusGitignore)
	}
	if m.wizard.Action == FixActionCreateProject {
		order = append(order, fixWizardFocusVisibility)
	}
	order = append(order, fixWizardFocusActions)
	return order
}

func (m *fixTUIModel) wizardMoveFocus(delta int, wrap bool) bool {
	order := m.wizardFocusOrder()
	if len(order) == 0 || delta == 0 {
		return false
	}
	index := -1
	for i, area := range order {
		if area == m.wizard.FocusArea {
			index = i
			break
		}
	}
	if index < 0 {
		index = len(order) - 1
	}
	next := index + delta
	if wrap {
		next = (next%len(order) + len(order)) % len(order)
	} else if next < 0 || next >= len(order) {
		return false
	}
	if next == index {
		return false
	}
	m.wizard.FocusArea = order[next]
	m.syncWizardFieldFocus()
	return true
}

func (m *fixTUIModel) shiftWizardVisibility(delta int) {
	options := []string{"default", "private", "public"}
	current := "default"
	switch m.wizard.Visibility {
	case domain.VisibilityPrivate:
		current = "private"
	case domain.VisibilityPublic:
		current = "public"
	}
	next := shiftEnumValue(current, options, delta)
	switch next {
	case "private":
		m.wizard.Visibility = domain.VisibilityPrivate
	case "public":
		m.wizard.Visibility = domain.VisibilityPublic
	default:
		m.wizard.Visibility = domain.VisibilityUnknown
	}
}

func (m *fixTUIModel) scrollWizardDown(lines int) bool {
	before := m.wizard.BodyViewport.YOffset
	m.wizard.BodyViewport.ScrollDown(lines)
	return m.wizard.BodyViewport.YOffset > before
}

func (m *fixTUIModel) scrollWizardUp(lines int) bool {
	before := m.wizard.BodyViewport.YOffset
	m.wizard.BodyViewport.ScrollUp(lines)
	return m.wizard.BodyViewport.YOffset < before
}

func (m *fixTUIModel) updateWizard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch msg.String() {
	case "tab":
		m.wizardMoveFocus(1, true)
		return m, nil
	case "shift+tab":
		m.wizardMoveFocus(-1, true)
		return m, nil
	}
	if m.wizard.FocusArea == fixWizardFocusCommit {
		if key.Matches(msg, m.keys.Cancel) {
			m.viewMode = fixViewList
			m.status = "cancelled remaining risky confirmations"
			return m, nil
		}
		switch msg.String() {
		case "enter":
			m.wizardMoveFocus(1, false)
			return m, nil
		case "down":
			if m.wizardMoveFocus(1, false) {
				return m, nil
			}
			m.scrollWizardDown(1)
			return m, nil
		case "up":
			if m.wizardMoveFocus(-1, false) {
				return m, nil
			}
			m.scrollWizardUp(1)
			return m, nil
		}
		var cmd tea.Cmd
		m.wizard.CommitMessage, cmd = m.wizard.CommitMessage.Update(msg)
		return m, cmd
	}
	if m.wizard.FocusArea == fixWizardFocusProjectName {
		if key.Matches(msg, m.keys.Cancel) {
			m.viewMode = fixViewList
			m.status = "cancelled remaining risky confirmations"
			return m, nil
		}
		switch msg.String() {
		case "enter":
			m.wizardMoveFocus(1, false)
			return m, nil
		case "down":
			if m.wizardMoveFocus(1, false) {
				return m, nil
			}
			m.scrollWizardDown(1)
			return m, nil
		case "up":
			if m.wizardMoveFocus(-1, false) {
				return m, nil
			}
			m.scrollWizardUp(1)
			return m, nil
		}
		var cmd tea.Cmd
		m.wizard.ProjectName, cmd = m.wizard.ProjectName.Update(msg)
		return m, cmd
	}
	if m.wizard.FocusArea == fixWizardFocusGitignore {
		if key.Matches(msg, m.keys.Cancel) {
			m.viewMode = fixViewList
			m.status = "cancelled remaining risky confirmations"
			return m, nil
		}
		switch msg.String() {
		case " ":
			m.wizard.GenerateGitignore = !m.wizard.GenerateGitignore
			return m, nil
		case "enter":
			m.wizardMoveFocus(1, false)
			return m, nil
		case "down":
			if m.wizardMoveFocus(1, false) {
				return m, nil
			}
			m.scrollWizardDown(1)
			return m, nil
		case "up":
			if m.wizardMoveFocus(-1, false) {
				return m, nil
			}
			m.scrollWizardUp(1)
			return m, nil
		}
	}

	if key.Matches(msg, m.keys.Cancel) {
		m.viewMode = fixViewList
		m.status = "cancelled remaining risky confirmations"
		return m, nil
	}
	if key.Matches(msg, m.keys.Skip) {
		m.skipWizardCurrent()
		return m, nil
	}
	if key.Matches(msg, m.keys.Apply) {
		switch m.wizard.ActionFocus {
		case fixWizardActionCancel:
			m.viewMode = fixViewList
			m.status = "cancelled remaining risky confirmations"
		case fixWizardActionSkip:
			m.skipWizardCurrent()
		case fixWizardActionApply:
			m.applyWizardCurrent()
		default:
			m.viewMode = fixViewList
			m.status = "cancelled remaining risky confirmations"
		}
		return m, nil
	}
	switch msg.String() {
	case "up":
		if m.wizardMoveFocus(-1, false) {
			return m, nil
		}
		m.scrollWizardUp(1)
		return m, nil
	case "down":
		if m.wizardMoveFocus(1, false) {
			return m, nil
		}
		m.scrollWizardDown(1)
		return m, nil
	case "pgdown":
		m.scrollWizardDown(max(1, m.wizard.BodyViewport.Height-1))
		return m, nil
	case "pgup":
		m.scrollWizardUp(max(1, m.wizard.BodyViewport.Height-1))
		return m, nil
	}
	if msg.String() == "left" {
		if m.wizard.FocusArea == fixWizardFocusVisibility {
			m.shiftWizardVisibility(-1)
			return m, nil
		}
		m.wizard.ActionFocus--
		if m.wizard.ActionFocus < fixWizardActionCancel {
			m.wizard.ActionFocus = fixWizardActionApply
		}
		return m, nil
	}
	if msg.String() == "right" {
		if m.wizard.FocusArea == fixWizardFocusVisibility {
			m.shiftWizardVisibility(+1)
			return m, nil
		}
		m.wizard.ActionFocus++
		if m.wizard.ActionFocus > fixWizardActionApply {
			m.wizard.ActionFocus = fixWizardActionCancel
		}
		return m, nil
	}
	if msg.String() == " " && m.wizard.FocusArea == fixWizardFocusGitignore {
		m.wizard.GenerateGitignore = !m.wizard.GenerateGitignore
		return m, nil
	}
	return m, nil
}

func (m *fixTUIModel) updateSummary(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if key.Matches(msg, m.keys.Apply) || key.Matches(msg, m.keys.Cancel) || key.Matches(msg, m.keys.Skip) {
		m.viewMode = fixViewList
		return m, nil
	}
	return m, nil
}

func (m *fixTUIModel) viewWizardContent() string {
	m.syncWizardViewport()

	controls := m.viewWizardStaticControls()
	hint := m.wizardFooterHint()
	actions := m.clampSingleLine(renderWizardActionButtons(m.wizard.ActionFocus), m.wizardBodyLineWidth())
	topIndicator := m.wizardScrollIndicatorTop()
	bottomIndicator := m.wizardScrollIndicatorBottom()

	var b strings.Builder
	b.WriteString(topIndicator)
	b.WriteString("\n")
	b.WriteString(m.wizard.BodyViewport.View())
	b.WriteString("\n")
	b.WriteString(bottomIndicator)
	if controls != "" {
		b.WriteString("\n\n")
		b.WriteString(controls)
	}
	b.WriteString("\n\n")
	b.WriteString(actions)
	b.WriteString("\n")
	b.WriteString(hint)
	return b.String()
}

func (m *fixTUIModel) wizardHasContextOverflow() bool {
	return !(m.wizard.BodyViewport.AtTop() && m.wizard.BodyViewport.AtBottom())
}

func (m *fixTUIModel) wizardScrollIndicatorTop() string {
	if !m.wizardHasContextOverflow() || m.wizard.BodyViewport.AtTop() {
		return ""
	}
	const text = "↑ More context above (scroll up)"
	return hintStyle.Render(m.clampSingleLine(text, m.wizardBodyLineWidth()))
}

func (m *fixTUIModel) wizardScrollIndicatorBottom() string {
	if !m.wizardHasContextOverflow() || m.wizard.BodyViewport.AtBottom() {
		return ""
	}
	const text = "↓ More context below (scroll down)"
	return hintStyle.Render(m.clampSingleLine(text, m.wizardBodyLineWidth()))
}

func (m *fixTUIModel) viewWizardStaticControls() string {
	controls := make([]string, 0, 4)
	if m.wizard.EnableCommitMessage {
		controls = append(controls, renderFieldBlock(
			m.wizard.FocusArea == fixWizardFocusCommit,
			"Commit message",
			"Leave empty to auto-generate.",
			renderInputContainer(m.wizard.CommitMessage.View(), m.wizard.FocusArea == fixWizardFocusCommit),
			"",
		))
	}
	if m.wizard.EnableProjectName {
		controls = append(controls, renderFieldBlock(
			m.wizard.FocusArea == fixWizardFocusProjectName,
			"Repository name on GitHub",
			"Leave empty to use the current folder/repo name.",
			renderInputContainer(m.wizard.ProjectName.View(), m.wizard.FocusArea == fixWizardFocusProjectName),
			"",
		))
	}
	if m.wizard.ShowGitignoreToggle {
		controls = append(controls, renderToggleField(
			m.wizard.FocusArea == fixWizardFocusGitignore,
			"Generate .gitignore before commit",
			"Only when root .gitignore is missing.",
			m.wizard.GenerateGitignore,
		))
	}
	if m.wizard.Action == FixActionCreateProject {
		controls = append(controls, renderFieldBlock(
			m.wizard.FocusArea == fixWizardFocusVisibility,
			"Project visibility",
			"Left/right changes this field when focused.",
			renderCreateVisibilityLine(m.wizard.Visibility, m.wizard.DefaultVis),
			"",
		))
	}
	return strings.Join(controls, "\n\n")
}

func (m *fixTUIModel) viewWizardTopLine() string {
	progress := hintStyle.Copy().Bold(true).Render(fmt.Sprintf("[%d/%d]", m.wizard.Index+1, len(m.wizard.Queue)))
	left := lipgloss.JoinHorizontal(
		lipgloss.Top,
		titleBadgeStyle.Render("bb"),
		" "+headerStyle.Render("fix"),
		"  ",
		labelStyle.Render("Confirm Risky Fix"),
	)
	line := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", progress)
	if m.wizard.RepoName != "" {
		context := hintStyle.Render(fmt.Sprintf("(%s · %s)", m.wizard.RepoName, fixActionLabel(m.wizard.Action)))
		line = lipgloss.JoinHorizontal(lipgloss.Top, line, " ", context)
	}
	return m.clampSingleLine(line, m.pageLineWidth())
}

func (m *fixTUIModel) wizardFooterHint() string {
	const text = "Tab/Shift+Tab: focus  Up/Down: scroll or move focus at edge  Left/Right: change enum/buttons  Space: toggle  Enter: activate  Esc: cancel"
	return hintStyle.Render(m.clampSingleLine(text, m.wizardBodyLineWidth()))
}

func (m *fixTUIModel) pageLineWidth() int {
	if m.width <= 0 {
		return 0
	}
	width := m.width - fixPageStyle.GetHorizontalFrameSize()
	if width < 1 {
		return 1
	}
	return width
}

func (m *fixTUIModel) wizardBodyLineWidth() int {
	width := m.wizardInnerWidth()
	if width > 0 {
		return width
	}
	if content := m.viewContentWidth(); content > 8 {
		return content - 8
	}
	return 0
}

func (m *fixTUIModel) clampSingleLine(text string, width int) string {
	line := strings.ReplaceAll(text, "\n", " ")
	if width <= 0 {
		return line
	}
	return ansi.Truncate(line, width, "…")
}

func (m *fixTUIModel) viewWizardContextContent() string {
	var b strings.Builder

	targetBranch := m.wizard.Branch
	if targetBranch == "" {
		targetBranch = "(unknown)"
	}
	targetLabel := targetBranch
	if m.wizard.Upstream != "" {
		targetLabel += " -> " + m.wizard.Upstream
	}
	context := strings.Join([]string{
		fmt.Sprintf("Repository: %s", m.wizard.RepoName),
		fmt.Sprintf("Path: %s", m.wizard.RepoPath),
		fmt.Sprintf("Action: %s", fixActionLabel(m.wizard.Action)),
		fmt.Sprintf("Target branch: %s", targetLabel),
	}, "\n")
	b.WriteString(fieldStyle.Render(context))
	b.WriteString("\n\n")
	b.WriteString(renderFieldBlock(false, "Changed files", "Files that will be included by this fix.", m.renderChangedFilesList(), ""))

	if len(m.wizard.Risk.SecretLikeChangedPaths) > 0 {
		b.WriteString("\n\n")
		b.WriteString(renderFieldBlock(
			false,
			"Secret-like changes detected",
			"Review carefully before any push/commit operation.",
			strings.Join(m.wizard.Risk.SecretLikeChangedPaths, "\n"),
			"",
		))
	}
	if m.wizard.Risk.MissingRootGitignore && len(m.wizard.Risk.NoisyChangedPaths) > 0 {
		detail := "Noisy paths were detected in uncommitted changes."
		if m.wizard.ShowGitignoreToggle {
			detail += " If you continue, bb can generate a root .gitignore before commit (see toggle below)."
		} else {
			detail += " This step will not generate .gitignore."
		}
		b.WriteString("\n\n")
		b.WriteString(renderFieldBlock(
			false,
			"Missing root .gitignore",
			detail,
			strings.Join(m.wizard.Risk.NoisyChangedPaths, "\n"),
			"",
		))
	}
	return b.String()
}

func (m *fixTUIModel) syncWizardViewport() {
	content := m.viewWizardContextContent()
	controls := m.viewWizardStaticControls()
	actions := m.clampSingleLine(renderWizardActionButtons(m.wizard.ActionFocus), m.wizardBodyLineWidth())
	hint := m.wizardFooterHint()
	// Non-scrollable rows:
	// - top indicator row
	// - bottom indicator row
	// - one spacer line before actions (or controls/actions block)
	// - action row + hint row
	staticLines := lipgloss.Height(actions) + lipgloss.Height(hint) + 3
	if controls != "" {
		// With controls shown, the single spacer before actions becomes:
		// one spacer before controls + one spacer before actions.
		staticLines += lipgloss.Height(controls) + 1
	}
	width, height := m.wizardViewportSize(lipgloss.Height(content), longestANSIWidth(content), staticLines)
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if m.wizard.BodyViewport.Width <= 0 || m.wizard.BodyViewport.Height <= 0 {
		m.wizard.BodyViewport = viewport.New(width, height)
	}
	offset := m.wizard.BodyViewport.YOffset
	m.wizard.BodyViewport.Width = width
	m.wizard.BodyViewport.Height = height
	m.wizard.BodyViewport.SetContent(content)
	m.wizard.BodyViewport.SetYOffset(offset)
}

func (m *fixTUIModel) wizardViewportSize(contentHeight int, contentWidth int, staticLines int) (int, int) {
	width := m.wizardInnerWidth()
	if width <= 0 {
		if contentWidth < 88 {
			contentWidth = 88
		}
		width = contentWidth
	}
	if contentHeight < 1 {
		contentHeight = 1
	}
	if m.height <= 0 {
		return width, contentHeight
	}

	available := m.height - fixPageStyle.GetVerticalFrameSize()
	available -= m.wizardHelpHeight()
	available -= 1 // separator line between body and help

	nonPanel := 1 // single top wizard header line
	if m.status != "" {
		nonPanel++
	}
	if m.errText != "" {
		nonPanel++
	}
	available -= nonPanel
	if available < panelStyle.GetVerticalFrameSize()+1 {
		available = panelStyle.GetVerticalFrameSize() + 1
	}

	panelInner := available - panelStyle.GetVerticalFrameSize()
	if staticLines < 1 {
		staticLines = 1
	}
	height := panelInner - staticLines
	if height < 1 {
		height = 1
	}
	return width, height
}

func (m *fixTUIModel) wizardHelpHeight() int {
	helpPanel := helpPanelStyle
	if w := m.viewContentWidth(); w > 0 {
		helpPanel = helpPanel.Width(w)
	}
	return lipgloss.Height(helpPanel.Render(m.help.View(m.keys)))
}

func longestANSIWidth(s string) int {
	maxWidth := 0
	for _, line := range strings.Split(s, "\n") {
		if width := ansi.StringWidth(line); width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth
}

func (m *fixTUIModel) viewSummaryContent() string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Fix Summary"))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Results from this apply session."))
	b.WriteString("\n\n")
	if len(m.summaryResults) == 0 {
		b.WriteString(fieldStyle.Render("No fixes were applied."))
	} else {
		for i, item := range m.summaryResults {
			if i > 0 {
				b.WriteString("\n")
			}
			value := fmt.Sprintf("%s: %s", item.Action, item.Status)
			if strings.TrimSpace(item.Detail) != "" {
				value += " (" + item.Detail + ")"
			}
			b.WriteString(renderFieldBlock(false, item.RepoName, "", value, ""))
		}
	}
	b.WriteString("\n\n")
	b.WriteString(buttonPrimaryStyle.Render("Done"))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Enter or esc to return to repository list."))
	return b.String()
}

func renderWizardActionButtons(focus int) string {
	cancelStyle := buttonDangerStyle
	skipStyle := buttonStyle
	applyStyle := buttonPrimaryStyle

	if focus == fixWizardActionCancel {
		cancelStyle = buttonDangerFocusStyle
	}
	if focus == fixWizardActionSkip {
		skipStyle = buttonFocusStyle
	}
	if focus == fixWizardActionApply {
		applyStyle = buttonPrimaryFocusStyle
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		cancelStyle.Render("Cancel"),
		skipStyle.Render("Skip"),
		applyStyle.Render("Apply"),
	)
}

func renderCreateVisibilityLine(current domain.Visibility, defaultVis domain.Visibility) string {
	makeLabel := func(vis domain.Visibility) string {
		name := string(vis)
		if vis == defaultVis {
			return name + " (default)"
		}
		return name
	}
	options := []struct {
		Visibility domain.Visibility
		Label      string
	}{
		{Visibility: domain.VisibilityPrivate, Label: makeLabel(domain.VisibilityPrivate)},
		{Visibility: domain.VisibilityPublic, Label: makeLabel(domain.VisibilityPublic)},
	}
	if current != domain.VisibilityPublic {
		current = domain.VisibilityPrivate
	}

	parts := make([]string, 0, len(options))
	for _, option := range options {
		style := enumOptionStyle
		if option.Visibility == current {
			style = enumOptionActiveStyle
		}
		parts = append(parts, style.Render(option.Label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m *fixTUIModel) wizardInnerWidth() int {
	width := m.viewContentWidth()
	if width <= 0 {
		return 0
	}
	inner := width - 8 // panel border + horizontal padding
	if inner < 40 {
		return 0
	}
	return inner
}

func (m *fixTUIModel) renderChangedFilesList() string {
	if len(m.wizard.Risk.ChangedFiles) == 0 {
		return hintStyle.Render("No uncommitted changes detected.")
	}

	const maxRows = 10
	limit := len(m.wizard.Risk.ChangedFiles)
	if limit > maxRows {
		limit = maxRows
	}

	blockWidth := m.wizardInnerWidth()
	if blockWidth <= 0 {
		blockWidth = 88
	}
	pathBudget := blockWidth - 24
	if pathBudget < 24 {
		pathBudget = 24
	}

	addedStyle := lipgloss.NewStyle().Foreground(successColor).Bold(true)
	deletedStyle := lipgloss.NewStyle().Foreground(errorFgColor).Bold(true)
	bulletStyle := lipgloss.NewStyle().Foreground(accentColor).Bold(true)

	lines := make([]string, 0, limit+1)
	for _, file := range m.wizard.Risk.ChangedFiles[:limit] {
		stats := lipgloss.JoinHorizontal(
			lipgloss.Top,
			addedStyle.Render(fmt.Sprintf("+%d", file.Added)),
			" ",
			deletedStyle.Render(fmt.Sprintf("-%d", file.Deleted)),
		)
		path := ansi.Truncate(file.Path, pathBudget, "…")
		pathPad := pathBudget - ansi.StringWidth(path)
		if pathPad < 0 {
			pathPad = 0
		}
		row := bulletStyle.Render("•") + " " + path + strings.Repeat(" ", pathPad+2) + stats
		lines = append(lines, row)
	}
	if len(m.wizard.Risk.ChangedFiles) > maxRows {
		lines = append(lines, hintStyle.Render(fmt.Sprintf("... showing first %d of %d changed files", maxRows, len(m.wizard.Risk.ChangedFiles))))
	}
	return strings.Join(lines, "\n")
}
