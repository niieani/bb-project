package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"bb-project/internal/domain"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
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

	RepoPath        string
	RepoName        string
	Branch          string
	Upstream        string
	OriginURL       string
	PreferredRemote string
	Operation       domain.Operation
	GitHubOwner     string
	RemoteProtocol  string
	Action          string
	Risk            fixRiskSnapshot

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

	Applying        bool
	ApplySpinner    spinner.Model
	ApplyPhase      string
	ApplyEvents     <-chan tea.Msg
	ApplyPlan       []fixActionPlanEntry
	ApplyStepStatus map[string]fixWizardApplyStepStatus
	ApplyDetail     string
}

type fixSummaryResult struct {
	RepoName string
	RepoPath string
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

type fixWizardApplyStepStatus int

const (
	fixWizardApplyStepPending fixWizardApplyStepStatus = iota
	fixWizardApplyStepRunning
	fixWizardApplyStepDone
	fixWizardApplyStepFailed
	fixWizardApplyStepSkipped
)

const (
	fixWizardApplyPhasePreparing  = "Preparing operation"
	fixWizardApplyPhaseExecuting  = "Applying repository changes"
	fixWizardApplyPhaseRechecking = "Rechecking repository status"
)

type fixWizardApplyProgressMsg struct {
	Event fixApplyStepEvent
}

type fixWizardApplyCompletedMsg struct {
	Updated fixRepoState
	Err     error
}

type fixWizardApplyChannelClosedMsg struct{}

func isRiskyFixAction(action string) bool {
	spec, ok := fixActionSpecFor(action)
	return ok && spec.Risky
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
	originURL := ""
	preferredRemote := ""
	operation := domain.OperationNone
	for _, repo := range m.repos {
		if repo.Record.Path == decision.RepoPath {
			repoName = repo.Record.Name
			repoRisk = repo.Risk
			branch = strings.TrimSpace(repo.Record.Branch)
			upstream = strings.TrimSpace(repo.Record.Upstream)
			originURL = strings.TrimSpace(repo.Record.OriginURL)
			operation = repo.Record.OperationInProgress
			if repo.Meta != nil {
				preferredRemote = strings.TrimSpace(repo.Meta.PreferredRemote)
			}
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

	showGitignoreToggle := m.shouldShowGitignoreToggle(decision.Action, repoRisk)
	applySpinner := spinner.New(spinner.WithSpinner(spinner.Dot))
	applySpinner.Style = lipgloss.NewStyle().Foreground(accentColor)

	m.wizard.RepoPath = decision.RepoPath
	m.wizard.RepoName = repoName
	m.wizard.Branch = branch
	m.wizard.Upstream = upstream
	m.wizard.OriginURL = originURL
	m.wizard.PreferredRemote = preferredRemote
	m.wizard.Operation = operation
	m.wizard.Action = decision.Action
	m.wizard.Risk = repoRisk
	m.wizard.EnableCommitMessage = decision.Action == FixActionStageCommitPush
	m.wizard.CommitMessage = commitInput
	m.wizard.CommitFocused = false
	m.wizard.EnableProjectName = decision.Action == FixActionCreateProject
	m.wizard.ProjectName = projectNameInput
	m.wizard.ShowGitignoreToggle = showGitignoreToggle
	m.wizard.GenerateGitignore = showGitignoreToggle
	githubOwner, defaultVis, remoteProtocol := m.detectWizardDefaults()
	m.wizard.GitHubOwner = githubOwner
	m.wizard.RemoteProtocol = remoteProtocol
	m.wizard.DefaultVis = defaultVis
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
	m.wizard.Applying = false
	m.wizard.ApplySpinner = applySpinner
	m.wizard.ApplyPhase = ""
	m.wizard.ApplyEvents = nil
	m.wizard.ApplyPlan = nil
	m.wizard.ApplyStepStatus = nil
	m.wizard.ApplyDetail = ""
	m.syncWizardViewport()
}

func (m *fixTUIModel) completeWizard() {
	m.viewMode = fixViewSummary
	m.status = ""
}

func (m *fixTUIModel) appendSummaryResult(action string, status string, detail string) {
	m.summaryResults = append(m.summaryResults, fixSummaryResult{
		RepoName: m.wizard.RepoName,
		RepoPath: m.wizard.RepoPath,
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

func (m *fixTUIModel) applyWizardCurrent() tea.Cmd {
	if m.wizard.Applying {
		return nil
	}

	opts := fixApplyOptions{Interactive: true}
	if m.wizard.EnableProjectName {
		opts.CreateProjectName = sanitizeGitHubRepositoryNameInput(m.wizard.ProjectName.Value())
	}
	if m.wizard.EnableCommitMessage {
		raw := strings.TrimSpace(m.wizard.CommitMessage.Value())
		if raw == "" || raw == DefaultFixCommitMessage {
			raw = "auto"
		}
		opts.CommitMessage = raw
	}
	if m.wizard.ShowGitignoreToggle && m.wizard.GenerateGitignore {
		patterns := m.wizardGitignoreTogglePatterns()
		if len(patterns) > 0 {
			opts.GenerateGitignore = true
			opts.GitignorePatterns = append([]string(nil), patterns...)
		}
	}
	if m.wizard.Action == FixActionCreateProject &&
		(m.wizard.Visibility == domain.VisibilityPrivate || m.wizard.Visibility == domain.VisibilityPublic) {
		opts.CreateProjectVisibility = m.wizard.Visibility
	}

	if err := m.validateWizardInputs(opts); err != nil {
		m.errText = err.Error()
		return nil
	}
	m.errText = ""

	if m.app == nil {
		m.appendSummaryResult(m.wizard.Action, "failed", "internal: app is not configured")
		m.advanceWizard()
		return nil
	}

	entries := fixActionExecutionPlanFor(m.wizard.Action, m.wizardActionPlanContext())
	m.wizard.ApplyPlan = append([]fixActionPlanEntry(nil), entries...)
	m.wizard.ApplyStepStatus = make(map[string]fixWizardApplyStepStatus, len(entries))
	for _, entry := range entries {
		m.wizard.ApplyStepStatus[fixActionPlanEntryKey(entry)] = fixWizardApplyStepPending
	}
	m.wizard.ApplyDetail = m.wizardApplySuccessDetail(opts)
	m.wizard.Applying = true
	m.wizard.ApplyPhase = fixWizardApplyPhasePreparing
	m.status = fmt.Sprintf("applying %s for %s", fixActionLabel(m.wizard.Action), m.wizard.RepoName)

	includeCatalogs := append([]string(nil), m.includeCatalogs...)
	repoPath := m.wizard.RepoPath
	action := m.wizard.Action
	app := m.app
	progress := make(chan tea.Msg, max(4, len(entries)+2))
	m.wizard.ApplyEvents = progress

	go func() {
		updated, err := app.applyFixActionWithObserver(includeCatalogs, repoPath, action, opts, func(event fixApplyStepEvent) {
			progress <- fixWizardApplyProgressMsg{Event: event}
		})
		progress <- fixWizardApplyCompletedMsg{Updated: updated, Err: err}
		close(progress)
	}()

	return tea.Batch(m.wizard.ApplySpinner.Tick, waitWizardApplyMsg(progress))
}

func waitWizardApplyMsg(events <-chan tea.Msg) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return fixWizardApplyChannelClosedMsg{}
		}
		return msg
	}
}

func (m *fixTUIModel) wizardApplySuccessDetail(opts fixApplyOptions) string {
	if !opts.GenerateGitignore {
		return ""
	}
	if m.wizard.Risk.MissingRootGitignore {
		return "generated root .gitignore"
	}
	return "appended noisy patterns to root .gitignore"
}

func fixActionPlanEntryKey(entry fixActionPlanEntry) string {
	if id := strings.TrimSpace(entry.ID); id != "" {
		return id
	}
	return strings.TrimSpace(entry.Summary)
}

func (m *fixTUIModel) wizardStepStatusFor(entry fixActionPlanEntry) fixWizardApplyStepStatus {
	if len(m.wizard.ApplyStepStatus) == 0 {
		return fixWizardApplyStepPending
	}
	status, ok := m.wizard.ApplyStepStatus[fixActionPlanEntryKey(entry)]
	if !ok {
		return fixWizardApplyStepPending
	}
	return status
}

func (m *fixTUIModel) setWizardStepStatus(entry fixActionPlanEntry, status fixWizardApplyStepStatus) {
	key := fixActionPlanEntryKey(entry)
	if key == "" {
		return
	}
	if m.wizard.ApplyStepStatus == nil {
		m.wizard.ApplyStepStatus = map[string]fixWizardApplyStepStatus{}
	}
	m.wizard.ApplyStepStatus[key] = status
}

func (m *fixTUIModel) wizardApplyPlanEntries() []fixActionPlanEntry {
	if len(m.wizard.ApplyPlan) > 0 {
		return append([]fixActionPlanEntry(nil), m.wizard.ApplyPlan...)
	}
	return fixActionExecutionPlanFor(m.wizard.Action, m.wizardActionPlanContext())
}

func (m *fixTUIModel) handleWizardApplyProgress(msg fixWizardApplyProgressMsg) tea.Cmd {
	switch msg.Event.Status {
	case fixApplyStepRunning:
		m.setWizardStepStatus(msg.Event.Entry, fixWizardApplyStepRunning)
		m.wizard.ApplyPhase = m.wizardApplyPhaseForEntry(msg.Event.Entry)
	case fixApplyStepDone:
		m.setWizardStepStatus(msg.Event.Entry, fixWizardApplyStepDone)
	case fixApplyStepFailed:
		m.setWizardStepStatus(msg.Event.Entry, fixWizardApplyStepFailed)
		if msg.Event.Err != nil {
			m.errText = msg.Event.Err.Error()
		}
	case fixApplyStepSkipped:
		m.setWizardStepStatus(msg.Event.Entry, fixWizardApplyStepSkipped)
	}
	if m.wizard.ApplyEvents != nil {
		return waitWizardApplyMsg(m.wizard.ApplyEvents)
	}
	return nil
}

func (m *fixTUIModel) handleWizardApplyCompleted(msg fixWizardApplyCompletedMsg) tea.Cmd {
	m.wizard.Applying = false
	m.wizard.ApplyPhase = ""
	m.wizard.ApplyEvents = nil
	if msg.Err != nil {
		if m.errText == "" {
			m.errText = msg.Err.Error()
		}
		m.status = fmt.Sprintf("apply failed for %s; review, retry, skip, or cancel", m.wizard.RepoName)
		return nil
	}

	m.errText = ""
	m.updateRepoAfterWizardApply(msg.Updated)
	m.appendSummaryResult(m.wizard.Action, "applied", m.wizard.ApplyDetail)
	m.advanceWizard()
	return nil
}

func (m *fixTUIModel) updateRepoAfterWizardApply(updated fixRepoState) {
	path := strings.TrimSpace(updated.Record.Path)
	if path == "" {
		return
	}

	replaced := false
	for i := range m.repos {
		if m.repos[i].Record.Path != path {
			continue
		}
		m.repos[i] = updated
		replaced = true
		break
	}
	if !replaced {
		m.repos = append(m.repos, updated)
	}
	m.rebuildList(path)
}

func (m *fixTUIModel) detectWizardDefaults() (owner string, visibility domain.Visibility, remoteProtocol string) {
	visibility = domain.VisibilityPrivate
	remoteProtocol = "ssh"
	if m.app == nil {
		return "", visibility, remoteProtocol
	}
	cfg, _, err := m.app.loadContext()
	if err != nil {
		return "", visibility, remoteProtocol
	}
	owner = strings.TrimSpace(cfg.GitHub.Owner)
	if strings.EqualFold(strings.TrimSpace(cfg.GitHub.RemoteProtocol), "https") {
		remoteProtocol = "https"
	}
	if strings.EqualFold(strings.TrimSpace(cfg.GitHub.DefaultVisibility), string(domain.VisibilityPublic)) {
		visibility = domain.VisibilityPublic
	}
	return owner, visibility, remoteProtocol
}

func (m *fixTUIModel) validateWizardInputs(opts fixApplyOptions) error {
	if err := validateFixApplyOptions(m.wizard.Action, opts); err != nil {
		return err
	}

	if m.wizard.EnableProjectName {
		name := strings.TrimSpace(opts.CreateProjectName)
		if name == "" {
			name = strings.TrimSpace(m.wizard.RepoName)
		}
		if err := validateGitHubRepositoryName(name); err != nil {
			return fmt.Errorf("invalid repository name on GitHub: %w", err)
		}
	}
	return nil
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

func (m *fixTUIModel) shouldShowGitignoreToggle(action string, risk fixRiskSnapshot) bool {
	if action != FixActionStageCommitPush {
		return false
	}
	if len(risk.NoisyChangedPaths) == 0 {
		return false
	}
	if risk.MissingRootGitignore {
		return len(risk.SuggestedGitignorePatterns) > 0
	}
	return len(risk.MissingGitignorePatterns) > 0
}

func (m *fixTUIModel) wizardGitignoreTogglePatterns() []string {
	if m.wizard.Risk.MissingRootGitignore {
		return append([]string(nil), m.wizard.Risk.SuggestedGitignorePatterns...)
	}
	return append([]string(nil), m.wizard.Risk.MissingGitignorePatterns...)
}

func (m *fixTUIModel) wizardGitignoreToggleTitle() string {
	if m.wizard.Risk.MissingRootGitignore {
		return "Generate .gitignore before commit"
	}
	return "Append to .gitignore before commit"
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
	if key.Matches(msg, m.keys.Help) {
		m.help.ShowAll = !m.help.ShowAll
		m.syncWizardViewport()
		return m, nil
	}
	if m.wizard.Applying {
		return m, nil
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
		m.errText = ""
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
		m.wizard.ProjectName.SetValue(sanitizeGitHubRepositoryNameInput(m.wizard.ProjectName.Value()))
		m.errText = ""
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
			return m, m.applyWizardCurrent()
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
		switch m.wizard.FocusArea {
		case fixWizardFocusVisibility:
			m.shiftWizardVisibility(-1)
			return m, nil
		case fixWizardFocusActions:
			m.wizard.ActionFocus--
			if m.wizard.ActionFocus < fixWizardActionCancel {
				m.wizard.ActionFocus = fixWizardActionApply
			}
			return m, nil
		default:
			return m, nil
		}
	}
	if msg.String() == "right" {
		switch m.wizard.FocusArea {
		case fixWizardFocusVisibility:
			m.shiftWizardVisibility(+1)
			return m, nil
		case fixWizardFocusActions:
			m.wizard.ActionFocus++
			if m.wizard.ActionFocus > fixWizardActionApply {
				m.wizard.ActionFocus = fixWizardActionCancel
			}
			return m, nil
		default:
			return m, nil
		}
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
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}
	if key.Matches(msg, m.keys.Help) {
		m.help.ShowAll = !m.help.ShowAll
		return m, nil
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
	actions := m.clampSingleLine(renderWizardActionButtons(m.wizard.ActionFocus), m.wizardBodyLineWidth())
	if m.wizard.Applying {
		actions = hintStyle.Render(m.clampSingleLine(m.wizardApplyingStatusLine(), m.wizardBodyLineWidth()))
	}
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
	return b.String()
}

func (m *fixTUIModel) wizardApplyingStatusLine() string {
	phase := strings.TrimSpace(m.wizard.ApplyPhase)
	if phase == "" {
		phase = fixWizardApplyPhaseExecuting
	}
	return fmt.Sprintf(
		"%s %s... controls are locked until execution completes.",
		m.wizard.ApplySpinner.View(),
		phase,
	)
}

func (m *fixTUIModel) wizardApplyPhaseForEntry(entry fixActionPlanEntry) string {
	if strings.TrimSpace(entry.ID) == fixActionPlanRevalidateStateID {
		return fixWizardApplyPhaseRechecking
	}
	return fixWizardApplyPhaseExecuting
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
			m.wizardGitignoreToggleTitle(),
			"",
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
	sections := make([]string, 0, 4)
	if changedFilesBlock := m.renderWizardChangedFilesBlock(); changedFilesBlock != "" {
		sections = append(sections, changedFilesBlock)
	}
	if len(m.wizard.Risk.SecretLikeChangedPaths) > 0 {
		sections = append(sections, renderFieldBlock(
			false,
			"Secret-like changes detected",
			"Review carefully before any push/commit operation.",
			strings.Join(m.wizard.Risk.SecretLikeChangedPaths, "\n"),
			"",
		))
	}
	if len(m.wizard.Risk.NoisyChangedPaths) > 0 {
		detail := "Noisy paths were detected in uncommitted changes."
		if m.wizard.ShowGitignoreToggle && m.wizard.Risk.MissingRootGitignore {
			detail += " If you continue, bb can generate a root .gitignore before commit (see toggle below)."
		} else if m.wizard.ShowGitignoreToggle {
			detail += " If you continue, bb can append missing noisy-path patterns to root .gitignore (see toggle below)."
		} else if m.wizard.Risk.MissingRootGitignore {
			detail += " This step will not generate .gitignore."
		} else {
			detail += " Root .gitignore already includes entries for these paths."
		}
		title := "Noisy uncommitted paths"
		if m.wizard.Risk.MissingRootGitignore {
			title = "Missing root .gitignore"
		}
		sections = append(sections, renderFieldBlock(
			false,
			title,
			detail,
			strings.Join(m.wizard.Risk.NoisyChangedPaths, "\n"),
			"",
		))
	}
	if applyPlanBlock := m.renderWizardApplyPlan(); applyPlanBlock != "" {
		sections = append(sections, applyPlanBlock)
	}
	if len(sections) > 0 {
		b.WriteString("\n\n")
		b.WriteString(strings.Join(sections, "\n\n"))
	}
	return b.String()
}

func (m *fixTUIModel) wizardActionPlanContext() fixActionPlanContext {
	ctx := fixActionPlanContext{
		Operation:               m.wizard.Operation,
		Branch:                  strings.TrimSpace(m.wizard.Branch),
		Upstream:                strings.TrimSpace(m.wizard.Upstream),
		OriginURL:               strings.TrimSpace(m.wizard.OriginURL),
		PreferredRemote:         strings.TrimSpace(m.wizard.PreferredRemote),
		GitHubOwner:             strings.TrimSpace(m.wizard.GitHubOwner),
		RemoteProtocol:          strings.TrimSpace(m.wizard.RemoteProtocol),
		RepoName:                strings.TrimSpace(m.wizard.RepoName),
		CreateProjectVisibility: m.wizard.Visibility,
		MissingRootGitignore:    m.wizard.Risk.MissingRootGitignore,
	}
	if m.wizard.EnableProjectName {
		ctx.CreateProjectName = sanitizeGitHubRepositoryNameInput(m.wizard.ProjectName.Value())
	}
	if m.wizard.EnableCommitMessage {
		raw := strings.TrimSpace(m.wizard.CommitMessage.Value())
		if raw == "" || raw == DefaultFixCommitMessage {
			raw = "auto"
		}
		ctx.CommitMessage = raw
	}
	if m.wizard.ShowGitignoreToggle && m.wizard.GenerateGitignore {
		ctx.GenerateGitignore = true
		ctx.GitignorePatterns = append([]string(nil), m.wizardGitignoreTogglePatterns()...)
	}
	return ctx
}

func (m *fixTUIModel) renderWizardApplyPlan() string {
	entries := m.wizardApplyPlanEntries()
	lines := make([]string, 0, len(entries)+1)
	lines = append(lines, "Applying this fix will execute these steps in order:")
	for _, entry := range entries {
		summary := strings.TrimSpace(entry.Summary)
		if summary == "" {
			continue
		}
		label := summary
		if entry.Command {
			label = renderWizardCommandLine(summary)
		}
		lines = append(lines, m.renderWizardPlanMarker(m.wizardStepStatusFor(entry))+" "+label)
	}

	if len(lines) <= 1 {
		return ""
	}

	return renderFieldBlock(
		false,
		"Review before applying",
		"These commands and side effects run when you choose Apply.",
		strings.Join(lines, "\n"),
		"",
	)
}

func (m *fixTUIModel) renderWizardPlanMarker(status fixWizardApplyStepStatus) string {
	switch status {
	case fixWizardApplyStepRunning:
		return lipgloss.NewStyle().Foreground(accentColor).Bold(true).Render(m.wizard.ApplySpinner.View())
	case fixWizardApplyStepDone:
		return lipgloss.NewStyle().Foreground(successColor).Bold(true).Render("✓")
	case fixWizardApplyStepFailed:
		return lipgloss.NewStyle().Foreground(errorFgColor).Bold(true).Render("✗")
	case fixWizardApplyStepSkipped:
		return lipgloss.NewStyle().Foreground(mutedTextColor).Bold(true).Render("◦")
	default:
		return lipgloss.NewStyle().Foreground(accentColor).Bold(true).Render("•")
	}
}

func (m *fixTUIModel) wizardChangedFilesFieldMeta() (string, string) {
	switch m.wizard.Action {
	case FixActionStageCommitPush:
		return "Uncommitted changed files", "These uncommitted files will be staged and committed by this fix."
	default:
		return "Uncommitted changed files", "Shown for review only. This fix does not stage or commit uncommitted files."
	}
}

func (m *fixTUIModel) renderWizardChangedFilesBlock() string {
	if len(m.wizard.Risk.ChangedFiles) == 0 {
		return ""
	}
	title, description := m.wizardChangedFilesFieldMeta()
	return renderFieldBlock(false, title, description, m.renderChangedFilesList(), "")
}

func renderWizardCommandLine(command string) string {
	return lipgloss.NewStyle().
		Foreground(textColor).
		Background(lipgloss.AdaptiveColor{Light: "#ECF3FF", Dark: "#1C2738"}).
		Padding(0, 1).
		Render(command)
}

func (m *fixTUIModel) syncWizardViewport() {
	content := m.viewWizardContextContent()
	controls := m.viewWizardStaticControls()
	actions := m.clampSingleLine(renderWizardActionButtons(m.wizard.ActionFocus), m.wizardBodyLineWidth())
	// Non-scrollable rows:
	// - top indicator row
	// - bottom indicator row
	// - one spacer line before actions (or controls/actions block)
	// - action row
	staticLines := lipgloss.Height(actions) + 3
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
	return lipgloss.Height(helpPanel.Render(m.footerHelpView(helpPanel)))
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
	b.WriteString(labelStyle.Render("Fix outcomes and current syncability after revalidation."))
	b.WriteString("\n\n")
	b.WriteString(renderFieldBlock(false, "Session totals", "", m.renderSummaryTotals(), ""))
	b.WriteString("\n\n")
	if len(m.summaryResults) == 0 {
		b.WriteString(renderFieldBlock(false, "Actions", "", "No fixes were applied.", ""))
	} else {
		for i, item := range m.summaryResults {
			if i > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(renderFieldBlock(
				false,
				item.RepoName,
				strings.TrimSpace(item.RepoPath),
				m.renderSummaryResultValue(item),
				m.renderSummaryResultError(item),
			))
		}
	}
	b.WriteString("\n\n")
	b.WriteString(buttonPrimaryStyle.Render("Done"))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Enter or esc to return to repository list."))
	return b.String()
}

func (m *fixTUIModel) renderSummaryTotals() string {
	applied, skipped, failed := m.summaryResultCounts()
	lines := []string{
		fmt.Sprintf("%s Applied: %d", lipgloss.NewStyle().Foreground(successColor).Bold(true).Render("✓"), applied),
		fmt.Sprintf("%s Skipped: %d", lipgloss.NewStyle().Foreground(mutedTextColor).Bold(true).Render("◦"), skipped),
		fmt.Sprintf("%s Failed: %d", lipgloss.NewStyle().Foreground(errorFgColor).Bold(true).Render("✗"), failed),
	}
	return strings.Join(lines, "\n")
}

func (m *fixTUIModel) renderSummaryResultValue(item fixSummaryResult) string {
	lines := []string{
		fmt.Sprintf("%s %s: %s", renderSummaryResultMarker(item.Status), item.Action, item.Status),
	}
	if outcome := m.summaryOutcomeLine(item); outcome != "" {
		lines = append(lines, outcome)
	}
	if detail := strings.TrimSpace(item.Detail); detail != "" && item.Status != "failed" {
		lines = append(lines, "Detail: "+detail)
	}
	return strings.Join(lines, "\n")
}

func (m *fixTUIModel) renderSummaryResultError(item fixSummaryResult) string {
	if item.Status != "failed" {
		return ""
	}
	return strings.TrimSpace(item.Detail)
}

func renderSummaryResultMarker(status string) string {
	switch strings.TrimSpace(status) {
	case "applied":
		return lipgloss.NewStyle().Foreground(successColor).Bold(true).Render("✓")
	case "failed":
		return lipgloss.NewStyle().Foreground(errorFgColor).Bold(true).Render("✗")
	case "skipped":
		return lipgloss.NewStyle().Foreground(mutedTextColor).Bold(true).Render("◦")
	default:
		return lipgloss.NewStyle().Foreground(accentColor).Bold(true).Render("•")
	}
}

func (m *fixTUIModel) summaryOutcomeLine(item fixSummaryResult) string {
	switch strings.TrimSpace(item.Status) {
	case "applied":
		repo, ok := m.summaryRepoState(item.RepoPath)
		if !ok {
			return "Result: applied; revalidated state unavailable."
		}
		if repo.Record.Syncable {
			return "Result: syncable now."
		}
		return "Result: still unsyncable; more fixes needed."
	case "skipped":
		return "Result: skipped; no repository changes."
	case "failed":
		repo, ok := m.summaryRepoState(item.RepoPath)
		if !ok {
			return "Result: action failed before completion."
		}
		if repo.Record.Syncable {
			return "Result: action failed, but repository is syncable."
		}
		return "Result: action failed; repository still needs fixes."
	default:
		return ""
	}
}

func (m *fixTUIModel) summaryRepoState(repoPath string) (fixRepoState, bool) {
	target := strings.TrimSpace(repoPath)
	if target == "" {
		return fixRepoState{}, false
	}
	for _, repo := range m.repos {
		if strings.TrimSpace(repo.Record.Path) == target {
			return repo, true
		}
	}
	return fixRepoState{}, false
}

func (m *fixTUIModel) summaryResultCounts() (applied int, skipped int, failed int) {
	for _, item := range m.summaryResults {
		switch strings.TrimSpace(item.Status) {
		case "applied":
			applied++
		case "skipped":
			skipped++
		case "failed":
			failed++
		}
	}
	return applied, skipped, failed
}

func renderWizardActionButtons(focus int) string {
	cancelStyle := buttonDangerStyle
	skipStyle := buttonStyle
	applyStyle := buttonPrimaryStyle
	cancelFocused := focus == fixWizardActionCancel
	skipFocused := focus == fixWizardActionSkip
	applyFocused := focus == fixWizardActionApply

	if cancelFocused {
		cancelStyle = buttonDangerFocusStyle
	}
	if skipFocused {
		skipStyle = buttonFocusStyle
	}
	if applyFocused {
		applyStyle = buttonPrimaryFocusStyle
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		cancelStyle.Render(renderButtonLabel("Cancel", cancelFocused)),
		skipStyle.Render(renderButtonLabel("Skip", skipFocused)),
		applyStyle.Render(renderButtonLabel("Apply", applyFocused)),
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
	const maxRows = 10
	limit := len(m.wizard.Risk.ChangedFiles)
	if limit > maxRows {
		limit = maxRows
	}

	blockWidth := m.wizardInnerWidth()
	if blockWidth <= 0 {
		blockWidth = 88
	}
	autoIgnoreBadge := renderBadge("AUTO-IGNORE", badgeToneWarning)
	suggestedPatterns := map[string]struct{}{}
	if m.wizard.ShowGitignoreToggle && m.wizard.GenerateGitignore {
		for _, pattern := range m.wizardGitignoreTogglePatterns() {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" {
				continue
			}
			suggestedPatterns[pattern] = struct{}{}
		}
	}
	autoIgnoreSlotWidth := 0
	if len(suggestedPatterns) > 0 {
		autoIgnoreSlotWidth = ansi.StringWidth(" " + autoIgnoreBadge)
	}

	pathBudget := blockWidth - 24 - autoIgnoreSlotWidth
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
		autoIgnoreSlot := ""
		if autoIgnoreSlotWidth > 0 {
			autoIgnoreSlot = strings.Repeat(" ", autoIgnoreSlotWidth)
			if shouldRenderAutoIgnoreBadge(file.Path, suggestedPatterns) {
				autoIgnoreSlot = " " + autoIgnoreBadge
			}
		}
		row := bulletStyle.Render("•") + " " + path + strings.Repeat(" ", pathPad) + autoIgnoreSlot + strings.Repeat(" ", 2) + stats
		lines = append(lines, row)
	}
	if len(m.wizard.Risk.ChangedFiles) > maxRows {
		lines = append(lines, hintStyle.Render(fmt.Sprintf("... showing first %d of %d changed files", maxRows, len(m.wizard.Risk.ChangedFiles))))
	}
	return strings.Join(lines, "\n")
}

func shouldRenderAutoIgnoreBadge(path string, suggestedPatterns map[string]struct{}) bool {
	if len(suggestedPatterns) == 0 {
		return false
	}
	pattern := noisyPatternForPath(path)
	if pattern == "" {
		return false
	}
	_, ok := suggestedPatterns[pattern]
	return ok
}
