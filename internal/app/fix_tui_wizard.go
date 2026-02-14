package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"bb-project/internal/domain"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
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

	ShowGitignoreToggle bool
	GenerateGitignore   bool

	Visibility  domain.Visibility
	ActionFocus int
}

type fixSummaryResult struct {
	RepoName string
	Action   string
	Status   string
	Detail   string
}

const (
	fixWizardActionApply = iota
	fixWizardActionSkip
	fixWizardActionCancel
)

func isRiskyFixAction(action string) bool {
	switch action {
	case FixActionPush, FixActionSetUpstreamPush, FixActionStageCommitPush, FixActionCreateProject:
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
	commitInput.Focus()

	showGitignoreToggle := decision.Action == FixActionStageCommitPush &&
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
	m.wizard.CommitFocused = decision.Action == FixActionStageCommitPush
	m.wizard.ShowGitignoreToggle = showGitignoreToggle
	m.wizard.GenerateGitignore = showGitignoreToggle
	m.wizard.Visibility = domain.VisibilityUnknown
	m.wizard.ActionFocus = fixWizardActionApply
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
			detail = "generated .gitignore before commit"
		}
		m.appendSummaryResult(m.wizard.Action, "applied", detail)
	}
	if err := m.refreshRepos(true); err != nil {
		m.errText = err.Error()
	}
	m.advanceWizard()
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

func (m *fixTUIModel) updateWizard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.wizard.EnableCommitMessage && m.wizard.CommitFocused {
		if key.Matches(msg, m.keys.Cancel) {
			m.viewMode = fixViewList
			m.status = "cancelled remaining risky confirmations"
			return m, nil
		}
		switch msg.String() {
		case "enter", "tab", "down":
			m.wizard.CommitFocused = false
			return m, nil
		}
		var cmd tea.Cmd
		m.wizard.CommitMessage, cmd = m.wizard.CommitMessage.Update(msg)
		return m, cmd
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
		case fixWizardActionApply:
			m.applyWizardCurrent()
		case fixWizardActionSkip:
			m.skipWizardCurrent()
		default:
			m.viewMode = fixViewList
			m.status = "cancelled remaining risky confirmations"
		}
		return m, nil
	}
	switch msg.String() {
	case "up", "tab":
		if m.wizard.EnableCommitMessage {
			m.wizard.CommitFocused = true
			return m, nil
		}
	}
	if key.Matches(msg, m.keys.Left) {
		m.wizard.ActionFocus--
		if m.wizard.ActionFocus < fixWizardActionApply {
			m.wizard.ActionFocus = fixWizardActionCancel
		}
		return m, nil
	}
	if key.Matches(msg, m.keys.Right) {
		m.wizard.ActionFocus++
		if m.wizard.ActionFocus > fixWizardActionCancel {
			m.wizard.ActionFocus = fixWizardActionApply
		}
		return m, nil
	}
	if m.wizard.Action == FixActionCreateProject && !m.wizard.EnableCommitMessage {
		switch msg.String() {
		case "up":
			m.shiftWizardVisibility(-1)
			return m, nil
		case "down":
			m.shiftWizardVisibility(+1)
			return m, nil
		}
	}
	if msg.String() == " " && m.wizard.ShowGitignoreToggle {
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
	var b strings.Builder
	progress := renderStatusPill(fmt.Sprintf("%d/%d", m.wizard.Index+1, len(m.wizard.Queue)))
	headerLeft := labelStyle.Render("Confirm Risky Fix")
	headerRight := progress
	header := headerLeft
	if width := m.wizardInnerWidth(); width > 0 {
		gap := width - lipgloss.Width(headerLeft) - lipgloss.Width(headerRight)
		if gap < 2 {
			gap = 2
		}
		header = lipgloss.JoinHorizontal(lipgloss.Top, headerLeft, strings.Repeat(" ", gap), headerRight)
	}
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Review context before applying this fix."))
	b.WriteString("\n\n")

	targetBranch := m.wizard.Branch
	if targetBranch == "" {
		targetBranch = "(unknown)"
	}
	targetLabel := targetBranch
	if m.wizard.Upstream != "" {
		targetLabel += " -> " + m.wizard.Upstream
	}
	overview := strings.Join([]string{
		fmt.Sprintf("Repository: %s", m.wizard.RepoName),
		fmt.Sprintf("Path: %s", m.wizard.RepoPath),
		fmt.Sprintf("Action: %s", fixActionLabel(m.wizard.Action)),
		fmt.Sprintf("Target branch: %s", targetLabel),
	}, "\n")
	b.WriteString(renderFieldBlock(false, "Overview", "", overview, ""))
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
		b.WriteString("\n\n")
		b.WriteString(renderFieldBlock(
			false,
			"Missing root .gitignore",
			"Noisy paths were detected in uncommitted changes.",
			strings.Join(m.wizard.Risk.NoisyChangedPaths, "\n"),
			"",
		))
	}

	if m.wizard.EnableCommitMessage {
		b.WriteString("\n\n")
		b.WriteString(renderFieldBlock(
			m.wizard.CommitFocused,
			"Commit message",
			"Leave empty to auto-generate. Enter/Tab moves to action buttons.",
			renderInputContainer(m.wizard.CommitMessage.View(), m.wizard.CommitFocused),
			"",
		))
	}
	if m.wizard.ShowGitignoreToggle {
		b.WriteString("\n\n")
		b.WriteString(renderToggleField(
			false,
			"Generate .gitignore before commit",
			"Only when root .gitignore is missing.",
			m.wizard.GenerateGitignore,
		))
	}
	if m.wizard.Action == FixActionCreateProject {
		b.WriteString("\n\n")
		current := "default"
		switch m.wizard.Visibility {
		case domain.VisibilityPrivate:
			current = "private"
		case domain.VisibilityPublic:
			current = "public"
		}
		b.WriteString(renderFieldBlock(
			false,
			"Project visibility",
			"Use up/down to change.",
			renderEnumLine(current, []string{"default", "private", "public"}),
			"",
		))
	}

	b.WriteString("\n\n")
	b.WriteString(renderWizardActionButtons(m.wizard.ActionFocus))
	b.WriteString("\n")
	if m.wizard.EnableCommitMessage && m.wizard.CommitFocused {
		b.WriteString(hintStyle.Render("Typing mode: text input captures all letters. Enter/Tab to buttons. Esc cancels remaining."))
	} else {
		b.WriteString(hintStyle.Render("Left/right: choose button   enter: activate   up/tab: back to input   esc: cancel remaining"))
	}
	return b.String()
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
	applyStyle := buttonPrimaryStyle
	skipStyle := buttonStyle
	cancelStyle := buttonDangerStyle

	if focus == fixWizardActionApply {
		applyStyle = buttonPrimaryFocusStyle
	}
	if focus == fixWizardActionSkip {
		skipStyle = buttonFocusStyle
	}
	if focus == fixWizardActionCancel {
		cancelStyle = buttonDangerFocusStyle
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		applyStyle.Render("Apply"),
		skipStyle.Render("Skip"),
		cancelStyle.Render("Cancel"),
	)
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
