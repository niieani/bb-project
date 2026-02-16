package app

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"bb-project/internal/domain"
)

type configStep int

const (
	stepIntro configStep = iota
	stepGitHub
	stepSync
	stepAutomation
	stepFixes
	stepCatalogs
	stepReview
)

type catalogEditorMode int

const (
	catalogEditorAdd catalogEditorMode = iota
	catalogEditorEditRoot
)

type catalogFocusArea int

const (
	catalogFocusTable catalogFocusArea = iota
	catalogFocusButtons
)

const (
	automationFocusScheduler = iota
	automationFocusNotifyEnabled
	automationFocusNotifyDedupe
	automationFocusNotifyThrottle
	automationFocusCount
)

const (
	fixesFocusLumenAutoCommit = iota
)

type configWizardKeyMap struct {
	NextStep       key.Binding
	PrevStep       key.Binding
	NextField      key.Binding
	PrevField      key.Binding
	Toggle         key.Binding
	Advance        key.Binding
	Apply          key.Binding
	Help           key.Binding
	Quit           key.Binding
	Back           key.Binding
	CatalogAdd     key.Binding
	CatalogEdit    key.Binding
	CatalogDelete  key.Binding
	CatalogDefault key.Binding
}

func (k configWizardKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.NextStep, k.Back, k.NextField, k.Toggle, k.Apply, k.Help, k.Quit}
}

func (k configWizardKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.NextStep, k.PrevStep, k.NextField, k.PrevField, k.Back, k.Advance},
		{k.Toggle, k.Apply, k.Help, k.Quit},
	}
}

func defaultConfigWizardKeyMap() configWizardKeyMap {
	return configWizardKeyMap{
		NextStep: key.NewBinding(
			key.WithKeys("right"),
			key.WithHelp("right", "next step/option"),
		),
		PrevStep: key.NewBinding(
			key.WithKeys("left"),
			key.WithHelp("left", "prev step/option"),
		),
		NextField: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("down", "next field"),
		),
		PrevField: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("up", "prev field / focus tabs"),
		),
		Toggle: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "toggle/set default"),
		),
		Advance: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "next step"),
		),
		Apply: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("ctrl+s", "apply"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back/cancel"),
		),
		CatalogAdd: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "add catalog"),
		),
		CatalogEdit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit catalog"),
		),
		CatalogDelete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete catalog"),
		),
		CatalogDefault: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "set default"),
		),
	}
}

type catalogEditor struct {
	mode            catalogEditorMode
	inputs          []textinput.Model
	presetOptions   []string
	presetValue     string
	rootSuggestions []string
	rootSuggestion  int
	focus           int
	row             int
	err             string
	confirmDelete   bool
	repoPathDepth   int
	privatePush     bool
	publicPush      bool
	autoCloneOnSync bool
}

type catalogTableRow struct {
	Name           string
	Root           string
	Preset         string
	Layout         string
	Default        string
	PrivatePush    string
	PublicPush     string
	RemoteOnly     bool
	SuggestedRoots []string
}

type configWizardModel struct {
	input ConfigWizardInput

	originalConfig  domain.ConfigFile
	originalMachine domain.MachineFile
	config          domain.ConfigFile
	machine         domain.MachineFile

	step  configStep
	dirty bool

	width  int
	height int

	help help.Model
	keys configWizardKeyMap

	errorText   string
	confirmQuit bool
	allowQuit   bool
	applied     bool
	focusTabs   bool

	githubOwnerInput textinput.Model
	githubFocus      int

	syncCursor int

	schedulerInterval textinput.Model

	notifyThrottle  textinput.Model
	automationFocus int

	fixesFocus     int
	lumenAvailable bool

	catalogTable table.Model
	catalogRows  []catalogTableRow
	catalogEdit  *catalogEditor
	catalogFocus catalogFocusArea
	catalogBtn   int

	createMissingRoots bool
}

var (
	textColor      = lipgloss.AdaptiveColor{Light: "#1F2328", Dark: "#E6EDF3"}
	mutedTextColor = lipgloss.AdaptiveColor{Light: "#57606A", Dark: "#8B949E"}
	borderColor    = lipgloss.AdaptiveColor{Light: "#D0D7DE", Dark: "#30363D"}
	panelBgColor   = lipgloss.AdaptiveColor{Light: "#F6F8FA", Dark: "#0D1117"}
	accentColor    = lipgloss.AdaptiveColor{Light: "#0969DA", Dark: "#58A6FF"}
	accentBgColor  = lipgloss.AdaptiveColor{Light: "#DDF4FF", Dark: "#1F2937"}
	successColor   = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#3FB950"}
	errorFgColor   = lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"}
	warningColor   = lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#D29922"}

	pageStyle = lipgloss.NewStyle().Padding(1, 2)

	titleBadgeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("31")).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(textColor)
	labelStyle  = lipgloss.NewStyle().Bold(true).Foreground(textColor)
	errorStyle  = lipgloss.NewStyle().Foreground(errorFgColor)
	hintStyle   = lipgloss.NewStyle().Foreground(mutedTextColor)
	warnStyle   = lipgloss.NewStyle().Foreground(warningColor).Bold(true)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Background(panelBgColor).
			Padding(0, 2)

	alertStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(errorFgColor).
			PaddingLeft(1)

	helpPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(0, 1)

	fieldStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(borderColor).
			PaddingLeft(1)

	fieldFocusStyle = fieldStyle.BorderForeground(accentColor)

	inputStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(borderColor).
			Padding(0, 1)

	inputFocusStyle = inputStyle.BorderForeground(accentColor)

	switchOnStyle = lipgloss.NewStyle().
			Foreground(successColor).
			Background(lipgloss.AdaptiveColor{Light: "#EAFBEF", Dark: "#0F2418"}).
			Bold(true).
			Padding(0, 2)

	switchOffStyle = lipgloss.NewStyle().
			Foreground(mutedTextColor).
			Background(lipgloss.AdaptiveColor{Light: "#F6F8FA", Dark: "#161B22"}).
			Padding(0, 2)

	enumOptionStyle = lipgloss.NewStyle().
			Foreground(mutedTextColor).
			Background(lipgloss.AdaptiveColor{Light: "#F6F8FA", Dark: "#161B22"}).
			Padding(0, 2).
			MarginRight(2)

	enumOptionActiveStyle = enumOptionStyle.
				Background(lipgloss.AdaptiveColor{Light: "#DDF4FF", Dark: "#13233A"}).
				Foreground(textColor).
				Bold(true)

	tabActiveBorder = lipgloss.Border{
		Top:         "─",
		Bottom:      " ",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "┘",
		BottomRight: "└",
	}

	tabBorder = lipgloss.Border{
		Top:         "─",
		Bottom:      "─",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "┴",
		BottomRight: "┴",
	}

	tabBaseStyle = lipgloss.NewStyle().
			Border(tabBorder, true).
			BorderForeground(borderColor).
			Foreground(mutedTextColor).
			Padding(0, 1)

	tabCurrentStyle = tabBaseStyle.
			BorderForeground(accentColor).
			Foreground(textColor).
			Bold(true)

	tabFocusedStyle = tabCurrentStyle.
			Border(tabActiveBorder, true).
			Background(accentBgColor)

	tabGapStyle = tabBaseStyle.
			BorderTop(false).
			BorderLeft(false).
			BorderRight(false)

	buttonStyle = lipgloss.NewStyle().
			Foreground(textColor).
			Background(lipgloss.AdaptiveColor{Light: "#F6F8FA", Dark: "#161B22"}).
			Padding(0, 2).
			MarginRight(1)

	buttonPrimaryStyle = buttonStyle.
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("31")).
				Bold(true)

	buttonFocusStyle = buttonStyle.
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("33")).
				Bold(true).
				Underline(true)

	buttonPrimaryFocusStyle = buttonPrimaryStyle.
				Background(lipgloss.Color("33")).
				Bold(true).
				Underline(true)

	buttonDangerStyle = buttonStyle.
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("160")).
				Bold(true)

	buttonDangerFocusStyle = buttonDangerStyle.
				Background(lipgloss.Color("196")).
				Bold(true).
				Underline(true)

	buttonDisabledStyle = buttonStyle.
				Foreground(mutedTextColor).
				Background(lipgloss.AdaptiveColor{Light: "#F6F8FA", Dark: "#161B22"})
)

func runConfigWizardInteractive(input ConfigWizardInput) (ConfigWizardResult, error) {
	model := newConfigWizardModel(input)
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithFilter(configWizardFilter))
	finalModel, err := program.Run()
	if err != nil {
		return ConfigWizardResult{}, err
	}
	m, ok := finalModel.(*configWizardModel)
	if !ok {
		return ConfigWizardResult{}, fmt.Errorf("unexpected config wizard model type %T", finalModel)
	}
	if !m.applied {
		return ConfigWizardResult{Applied: false}, nil
	}
	return ConfigWizardResult{
		Applied:                   true,
		CreateMissingCatalogRoots: m.createMissingRoots,
		Config:                    m.config,
		Machine:                   m.machine,
	}, nil
}

func configWizardFilter(model tea.Model, msg tea.Msg) tea.Msg {
	if _, ok := msg.(tea.QuitMsg); !ok {
		return msg
	}
	m, ok := model.(*configWizardModel)
	if !ok {
		return msg
	}
	if m.dirty && !m.allowQuit && !m.applied {
		return nil
	}
	return msg
}

func newConfigWizardModel(input ConfigWizardInput) *configWizardModel {
	m := &configWizardModel{
		input:              input,
		originalConfig:     input.Config,
		originalMachine:    input.Machine,
		config:             input.Config,
		machine:            input.Machine,
		step:               stepIntro,
		help:               help.New(),
		keys:               defaultConfigWizardKeyMap(),
		catalogFocus:       catalogFocusButtons,
		catalogBtn:         0,
		createMissingRoots: true,
		lumenAvailable:     input.LumenAvailable,
	}
	if m.input.KnownCatalogRoots == nil {
		m.input.KnownCatalogRoots = map[string][]string{}
	}
	if m.machine.Catalogs == nil {
		m.machine.Catalogs = []domain.Catalog{}
	}
	if m.config.StateTransport.Mode != "external" {
		m.config.StateTransport.Mode = "external"
	}
	if m.config.Scheduler.IntervalMinutes <= 0 {
		m.config.Scheduler.IntervalMinutes = 60
	}
	if m.config.Clone.Presets == nil {
		m.config.Clone.Presets = map[string]domain.ClonePreset{}
	}
	if m.config.Clone.CatalogPreset == nil {
		m.config.Clone.CatalogPreset = map[string]string{}
	}
	if strings.TrimSpace(m.config.Link.TargetDir) == "" {
		m.config.Link.TargetDir = "references"
	}
	m.initGitHubInputs()
	m.initSchedulerInput()
	m.initNotifyInput()
	m.initCatalogTable()
	m.focusTabs = true
	m.onStepChanged()
	m.recomputeDirty()
	return m
}

func (m *configWizardModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *configWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.resizeCatalogTable()
		return m, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		case key.Matches(msg, m.keys.Quit):
			if !m.dirty || m.confirmQuit {
				m.allowQuit = true
				return m, tea.Quit
			}
			m.confirmQuit = true
			m.errorText = "Unsaved changes. Press Ctrl+C again to discard and quit."
			return m, nil
		case key.Matches(msg, m.keys.Back):
			if m.catalogEdit != nil {
				m.catalogEdit = nil
				m.errorText = ""
				return m, nil
			}
			m.prevStep()
			return m, nil
		case key.Matches(msg, m.keys.PrevStep):
			if m.catalogEdit == nil && m.focusTabs {
				m.prevStepKeepTabs()
				return m, nil
			}
		case key.Matches(msg, m.keys.NextStep):
			if m.catalogEdit == nil && m.focusTabs {
				m.advanceStepKeepTabs()
				return m, nil
			}
		case key.Matches(msg, m.keys.Advance):
			if m.catalogEdit == nil && m.focusTabs && m.step != stepReview {
				m.advanceStep()
				return m, nil
			}
		}
	}

	m.confirmQuit = false
	m.errorText = ""

	switch m.step {
	case stepIntro:
		return m.updateIntro(msg)
	case stepGitHub:
		return m.updateGitHub(msg)
	case stepSync:
		return m.updateSync(msg)
	case stepAutomation:
		return m.updateAutomation(msg)
	case stepFixes:
		return m.updateFixes(msg)
	case stepCatalogs:
		return m.updateCatalogs(msg)
	case stepReview:
		return m.updateReview(msg)
	default:
		return m, nil
	}
}

func (m *configWizardModel) advanceStep() {
	if err := m.nextStep(); err != nil {
		m.errorText = err.Error()
	}
}

func (m *configWizardModel) advanceStepKeepTabs() {
	if err := m.nextStep(); err != nil {
		m.errorText = err.Error()
		return
	}
	m.focusTabs = true
	m.onStepChanged()
}

func (m *configWizardModel) prevStepKeepTabs() {
	if m.step == stepIntro {
		return
	}
	m.step--
	m.focusTabs = true
	m.onStepChanged()
}

func (m *configWizardModel) View() string {
	var b strings.Builder

	title := lipgloss.JoinHorizontal(lipgloss.Center,
		titleBadgeStyle.Render("bb"),
		" "+headerStyle.Render("config wizard"),
	)
	subtitle := hintStyle.Render("Interactive setup for local configuration files")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(subtitle)
	b.WriteString("\n\n")
	b.WriteString(m.stepsHeader())
	b.WriteString("\n")

	content := ""
	switch m.step {
	case stepIntro:
		content = m.viewIntro()
	case stepGitHub:
		content = m.viewGitHub()
	case stepSync:
		content = m.viewSync()
	case stepAutomation:
		content = m.viewAutomation()
	case stepFixes:
		content = m.viewFixes()
	case stepCatalogs:
		content = m.viewCatalogs()
	case stepReview:
		content = m.viewReview()
	}
	contentPanel := panelStyle
	if w := m.viewContentWidth(); w > 0 {
		contentPanel = contentPanel.Width(w)
	}
	b.WriteString(contentPanel.Render(content))

	b.WriteString("\n")
	if m.errorText != "" {
		b.WriteString(alertStyle.Render(errorStyle.Render(m.errorText)))
		b.WriteString("\n")
	}
	if m.confirmQuit {
		b.WriteString(alertStyle.Render(errorStyle.Render("Press Ctrl+C again to discard changes and quit.")))
		b.WriteString("\n")
	}

	helpView := m.help.View(m.keys)
	helpPanel := helpPanelStyle
	if w := m.viewContentWidth(); w > 0 {
		helpPanel = helpPanel.Width(w)
	}
	helpBlock := helpPanel.Render(helpView)

	body := b.String()
	spacer := ""
	if m.height > 0 {
		const pageVerticalPadding = 2
		const separatorLines = 2
		bodyHeight := lipgloss.Height(body)
		helpHeight := lipgloss.Height(helpBlock)
		total := bodyHeight + separatorLines + helpHeight + pageVerticalPadding
		if gap := m.height - total; gap > 0 {
			spacer = strings.Repeat("\n", gap)
		}
	}

	doc := body + "\n\n" + spacer + helpBlock
	return pageStyle.Render(doc) + "\n"
}

func (m *configWizardModel) updateIntro(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if key.Matches(keyMsg, m.keys.Advance) {
			m.advanceStep()
		}
	}
	return m, nil
}

func (m *configWizardModel) viewContentWidth() int {
	if m.width <= 0 {
		return 0
	}
	contentWidth := m.width - 8
	if contentWidth < 52 {
		return 0
	}
	return contentWidth
}

func (m *configWizardModel) updateGitHub(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up":
			if m.focusTabs {
				return m, nil
			}
			if m.githubFocus == 0 {
				m.focusTabs = true
				m.updateGitHubFocus()
				return m, nil
			}
			m.githubFocus--
			m.updateGitHubFocus()
			return m, nil
		case "down":
			if m.focusTabs {
				m.focusTabs = false
				m.githubFocus = 0
				m.updateGitHubFocus()
				return m, nil
			}
			if m.githubFocus < 2 {
				m.githubFocus++
			}
			m.updateGitHubFocus()
			return m, nil
		case "enter":
			if !m.focusTabs {
				m.advanceStep()
				return m, nil
			}
		case " ":
			if m.focusTabs {
				return m, nil
			}
			if m.githubFocus == 1 {
				m.config.GitHub.DefaultVisibility = shiftEnumValue(m.config.GitHub.DefaultVisibility, []string{"private", "public"}, +1)
				m.recomputeDirty()
				return m, nil
			}
			if m.githubFocus == 2 {
				m.config.GitHub.RemoteProtocol = shiftEnumValue(m.config.GitHub.RemoteProtocol, []string{"ssh", "https"}, +1)
				m.recomputeDirty()
				return m, nil
			}
		case "right":
			if !m.focusTabs && m.githubFocus == 1 {
				m.config.GitHub.DefaultVisibility = shiftEnumValue(m.config.GitHub.DefaultVisibility, []string{"private", "public"}, +1)
				m.recomputeDirty()
				return m, nil
			}
			if !m.focusTabs && m.githubFocus == 2 {
				m.config.GitHub.RemoteProtocol = shiftEnumValue(m.config.GitHub.RemoteProtocol, []string{"ssh", "https"}, +1)
				m.recomputeDirty()
				return m, nil
			}
		case "left":
			if !m.focusTabs && m.githubFocus == 1 {
				m.config.GitHub.DefaultVisibility = shiftEnumValue(m.config.GitHub.DefaultVisibility, []string{"private", "public"}, -1)
				m.recomputeDirty()
				return m, nil
			}
			if !m.focusTabs && m.githubFocus == 2 {
				m.config.GitHub.RemoteProtocol = shiftEnumValue(m.config.GitHub.RemoteProtocol, []string{"ssh", "https"}, -1)
				m.recomputeDirty()
				return m, nil
			}
		}
	}

	if m.focusTabs || m.githubFocus != 0 {
		return m, nil
	}

	var cmd tea.Cmd
	m.githubOwnerInput, cmd = m.githubOwnerInput.Update(msg)
	m.config.GitHub.Owner = strings.TrimSpace(m.githubOwnerInput.Value())
	m.recomputeDirty()
	return m, cmd
}

func (m *configWizardModel) updateSync(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "up":
		if m.focusTabs {
			return m, nil
		}
		if m.syncCursor == 0 {
			m.focusTabs = true
			return m, nil
		}
		m.syncCursor--
	case "down":
		if m.focusTabs {
			m.focusTabs = false
			m.syncCursor = 0
			return m, nil
		}
		if m.syncCursor < 5 {
			m.syncCursor++
		}
	case " ":
		if !m.focusTabs {
			m.toggleSyncOption(m.syncCursor)
			m.recomputeDirty()
		}
	case "enter":
		if !m.focusTabs {
			m.advanceStep()
		}
	case "k":
		if m.syncCursor > 0 {
			m.syncCursor--
		}
	case "j":
		if m.syncCursor < 5 {
			m.syncCursor++
		}
	}
	return m, nil
}

func (m *configWizardModel) updateAutomation(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up":
			if m.focusTabs {
				return m, nil
			}
			if m.automationFocus == automationFocusScheduler {
				m.focusTabs = true
				m.updateAutomationFocus()
				return m, nil
			}
			m.automationFocus--
			m.updateAutomationFocus()
			return m, nil
		case "down":
			if m.focusTabs {
				m.focusTabs = false
				m.automationFocus = automationFocusScheduler
				m.updateAutomationFocus()
				return m, nil
			}
			if m.automationFocus < automationFocusCount-1 {
				m.automationFocus++
			}
			m.updateAutomationFocus()
			return m, nil
		case " ":
			if m.focusTabs {
				return m, nil
			}
			if m.automationFocus == automationFocusNotifyEnabled {
				m.config.Notify.Enabled = !m.config.Notify.Enabled
				m.recomputeDirty()
				return m, nil
			}
			if m.automationFocus == automationFocusNotifyDedupe {
				m.config.Notify.Dedupe = !m.config.Notify.Dedupe
				m.recomputeDirty()
				return m, nil
			}
		case "enter":
			if !m.focusTabs {
				if v, err := strconv.Atoi(strings.TrimSpace(m.schedulerInterval.Value())); err == nil {
					m.config.Scheduler.IntervalMinutes = v
				}
				if v, err := strconv.Atoi(strings.TrimSpace(m.notifyThrottle.Value())); err == nil {
					m.config.Notify.ThrottleMinutes = v
				}
				m.advanceStep()
				return m, nil
			}
		}
	}

	if m.focusTabs {
		return m, nil
	}

	var cmd tea.Cmd
	switch m.automationFocus {
	case automationFocusScheduler:
		m.schedulerInterval, cmd = m.schedulerInterval.Update(msg)
		if v, err := strconv.Atoi(strings.TrimSpace(m.schedulerInterval.Value())); err == nil {
			m.config.Scheduler.IntervalMinutes = v
		}
	case automationFocusNotifyThrottle:
		m.notifyThrottle, cmd = m.notifyThrottle.Update(msg)
		if v, err := strconv.Atoi(strings.TrimSpace(m.notifyThrottle.Value())); err == nil {
			m.config.Notify.ThrottleMinutes = v
		}
	default:
		return m, nil
	}
	m.recomputeDirty()
	return m, cmd
}

func (m *configWizardModel) updateFixes(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up":
			if m.focusTabs {
				return m, nil
			}
			if m.fixesFocus == fixesFocusLumenAutoCommit {
				m.focusTabs = true
				m.updateFixesFocus()
				return m, nil
			}
			return m, nil
		case "down":
			if m.focusTabs {
				m.focusTabs = false
				m.fixesFocus = fixesFocusLumenAutoCommit
				m.updateFixesFocus()
				return m, nil
			}
			return m, nil
		case " ":
			if m.focusTabs || !m.lumenAvailable {
				return m, nil
			}
			if m.fixesFocus == fixesFocusLumenAutoCommit {
				m.config.Integrations.Lumen.AutoGenerateCommitMessageWhenEmpty = !m.config.Integrations.Lumen.AutoGenerateCommitMessageWhenEmpty
				m.recomputeDirty()
				return m, nil
			}
		case "enter":
			if !m.focusTabs {
				m.advanceStep()
				return m, nil
			}
		}
	}
	return m, nil
}

func (m *configWizardModel) updateCatalogs(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.catalogEdit != nil {
		return m.updateCatalogEditor(msg)
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up":
			if m.focusTabs {
				return m, nil
			}
			if len(m.catalogRows) == 0 {
				m.focusTabs = true
				m.catalogTable.Blur()
				m.applyCatalogTableStyles(false)
				return m, nil
			}
			cur := m.catalogTable.Cursor()
			if cur > 0 {
				m.catalogTable.SetCursor(cur - 1)
				m.normalizeCatalogButtonForSelection()
				return m, nil
			}
			m.focusTabs = true
			m.catalogTable.Blur()
			m.applyCatalogTableStyles(false)
			return m, nil
		case "down":
			if m.focusTabs {
				m.focusTabs = false
				if len(m.catalogRows) == 0 {
					m.catalogFocus = catalogFocusButtons
					m.catalogBtn = m.catalogButtonMin()
					m.catalogTable.Blur()
					m.applyCatalogTableStyles(false)
				} else {
					m.catalogFocus = catalogFocusButtons
					m.catalogBtn = m.catalogButtonMin()
					m.catalogTable.Focus()
					m.applyCatalogTableStyles(true)
				}
				return m, nil
			}
			if len(m.catalogRows) > 0 {
				cur := m.catalogTable.Cursor()
				if cur < len(m.catalogRows)-1 {
					m.catalogTable.SetCursor(cur + 1)
					m.normalizeCatalogButtonForSelection()
				}
				return m, nil
			}
		case " ":
			if !m.focusTabs && m.selectedLocalCatalogIndex() >= 0 {
				if err := m.setDefaultFromSelection(); err != nil {
					m.errorText = err.Error()
				} else {
					m.recomputeDirty()
				}
				return m, nil
			}
		case "right":
			if !m.focusTabs && m.catalogBtn < m.catalogButtonMax() {
				m.catalogBtn++
				return m, nil
			}
		case "left":
			if !m.focusTabs && m.catalogBtn > m.catalogButtonMin() {
				m.catalogBtn--
				return m, nil
			}
		case "enter":
			if !m.focusTabs {
				switch m.catalogBtn {
				case 0: // edit
					if m.selectedLocalCatalogIndex() < 0 {
						m.errorText = "select a catalog to edit"
						return m, nil
					}
					m.startCatalogEditRootEditor()
					return m, nil
				case 1: // add
					if row, ok := m.selectedCatalogTableRow(); ok && row.RemoteOnly {
						m.startCatalogAddEditorWithTemplate(row.Name, row.SuggestedRoots)
					} else {
						m.startCatalogAddEditor()
					}
					return m, nil
				case 2: // set default
					if m.selectedLocalCatalogIndex() < 0 {
						m.errorText = "select a catalog to set default"
						return m, nil
					}
					if err := m.setDefaultFromSelection(); err != nil {
						m.errorText = err.Error()
						return m, nil
					}
					m.recomputeDirty()
					return m, nil
				case 3: // toggle repo path layout depth
					if m.selectedLocalCatalogIndex() < 0 {
						m.errorText = "select a catalog to toggle layout"
						return m, nil
					}
					if err := m.toggleRepoPathDepthFromSelection(); err != nil {
						m.errorText = err.Error()
						return m, nil
					}
					m.recomputeDirty()
					return m, nil
				case 4: // toggle private default-branch auto-push
					if m.selectedLocalCatalogIndex() < 0 {
						m.errorText = "select a catalog to toggle private auto-push"
						return m, nil
					}
					if err := m.toggleDefaultBranchAutoPushFromSelection(domain.VisibilityPrivate); err != nil {
						m.errorText = err.Error()
						return m, nil
					}
					m.recomputeDirty()
					return m, nil
				case 5: // toggle public default-branch auto-push
					if m.selectedLocalCatalogIndex() < 0 {
						m.errorText = "select a catalog to toggle public auto-push"
						return m, nil
					}
					if err := m.toggleDefaultBranchAutoPushFromSelection(domain.VisibilityPublic); err != nil {
						m.errorText = err.Error()
						return m, nil
					}
					m.recomputeDirty()
					return m, nil
				case 6: // continue
					if len(m.machine.Catalogs) == 0 {
						m.errorText = "at least one catalog is required"
						return m, nil
					}
					m.advanceStep()
					return m, nil
				}
			}
		}

		if m.focusTabs {
			return m, nil
		}
	}
	return m, nil
}

func (m *configWizardModel) updateCatalogEditor(msg tea.Msg) (tea.Model, tea.Cmd) {
	editor := m.catalogEdit
	if editor == nil {
		return m, nil
	}
	actionStart := m.catalogEditorActionStart()
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.catalogEdit = nil
			m.updateCatalogFocus()
			return m, nil
		case "down":
			if editor.focus < m.catalogEditorFocusCount()-1 {
				editor.focus++
			}
			editor.confirmDelete = false
			m.updateCatalogEditorFocus()
			return m, nil
		case "up":
			if editor.focus == 0 {
				m.catalogEdit = nil
				m.focusTabs = true
				m.catalogTable.Blur()
				return m, nil
			}
			editor.focus--
			editor.confirmDelete = false
			m.updateCatalogEditorFocus()
			return m, nil
		case "right":
			if editor.mode == catalogEditorAdd && editor.focus == 1 {
				m.shiftCatalogAddRootSuggestion(+1)
				return m, nil
			}
			if editor.mode == catalogEditorEditRoot && editor.focus >= len(editor.inputs) && editor.focus < actionStart {
				if err := m.shiftCatalogEditorOption(editor.focus-len(editor.inputs), 1); err != nil {
					editor.err = err.Error()
					return m, nil
				}
				editor.confirmDelete = false
				editor.err = ""
				return m, nil
			}
			if editor.focus >= actionStart {
				last := m.catalogEditorFocusCount() - 1
				if editor.focus < last {
					editor.focus++
					editor.confirmDelete = false
					m.updateCatalogEditorFocus()
				}
				return m, nil
			}
		case "left":
			if editor.mode == catalogEditorAdd && editor.focus == 1 {
				m.shiftCatalogAddRootSuggestion(-1)
				return m, nil
			}
			if editor.mode == catalogEditorEditRoot && editor.focus >= len(editor.inputs) && editor.focus < actionStart {
				if err := m.shiftCatalogEditorOption(editor.focus-len(editor.inputs), -1); err != nil {
					editor.err = err.Error()
					return m, nil
				}
				editor.confirmDelete = false
				editor.err = ""
				return m, nil
			}
			if editor.focus > actionStart {
				editor.focus--
				editor.confirmDelete = false
				m.updateCatalogEditorFocus()
				return m, nil
			}
		case " ":
			if editor.mode == catalogEditorAdd && editor.focus == 1 {
				m.shiftCatalogAddRootSuggestion(+1)
				return m, nil
			}
			if editor.mode == catalogEditorEditRoot && editor.focus >= len(editor.inputs) && editor.focus < actionStart {
				if err := m.shiftCatalogEditorOption(editor.focus-len(editor.inputs), 1); err != nil {
					editor.err = err.Error()
					return m, nil
				}
				editor.confirmDelete = false
				editor.err = ""
				return m, nil
			}
		case "enter":
			editor.err = ""
			if editor.focus < len(editor.inputs) {
				if editor.focus < len(editor.inputs)-1 {
					editor.focus++
				} else {
					if actionStart > len(editor.inputs) {
						editor.focus = len(editor.inputs)
					} else {
						editor.focus = actionStart
					}
				}
				editor.confirmDelete = false
				m.updateCatalogEditorFocus()
				return m, nil
			}
			if editor.mode == catalogEditorEditRoot && editor.focus >= len(editor.inputs) && editor.focus < actionStart {
				if err := m.shiftCatalogEditorOption(editor.focus-len(editor.inputs), 1); err != nil {
					editor.err = err.Error()
					return m, nil
				}
				editor.confirmDelete = false
				editor.err = ""
				return m, nil
			}
			action := editor.focus - actionStart
			switch editor.mode {
			case catalogEditorAdd:
				switch action {
				case 0: // save
					if err := m.commitCatalogEditor(); err != nil {
						editor.err = err.Error()
						return m, nil
					}
					m.catalogEdit = nil
					m.recomputeDirty()
					m.updateCatalogFocus()
					return m, nil
				case 1: // cancel
					m.catalogEdit = nil
					m.updateCatalogFocus()
					return m, nil
				}
			case catalogEditorEditRoot:
				switch action {
				case 0: // save
					if err := m.commitCatalogEditor(); err != nil {
						editor.err = err.Error()
						return m, nil
					}
					m.catalogEdit = nil
					m.recomputeDirty()
					m.updateCatalogFocus()
					return m, nil
				case 1: // delete
					if !editor.confirmDelete {
						editor.confirmDelete = true
						editor.err = "Press Enter again to confirm delete."
						return m, nil
					}
					if err := m.deleteCatalogAt(editor.row); err != nil {
						editor.err = err.Error()
						return m, nil
					}
					m.catalogEdit = nil
					m.recomputeDirty()
					m.updateCatalogFocus()
					return m, nil
				case 2: // cancel
					m.catalogEdit = nil
					m.updateCatalogFocus()
					return m, nil
				}
			}
			return m, nil
		}
	}

	cmds := make([]tea.Cmd, len(editor.inputs))
	for i := range editor.inputs {
		editor.inputs[i], cmds[i] = editor.inputs[i].Update(msg)
	}
	editor.err = ""
	editor.confirmDelete = false
	return m, tea.Batch(cmds...)
}

func (m *configWizardModel) updateReview(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(keyMsg, m.keys.Toggle):
			if len(missingCatalogRoots(m.machine.Catalogs)) > 0 {
				m.createMissingRoots = !m.createMissingRoots
			}
			return m, nil
		case key.Matches(keyMsg, m.keys.Apply):
			if err := m.validateAll(); err != nil {
				m.errorText = err.Error()
				return m, nil
			}
			m.applied = true
			m.allowQuit = true
			return m, tea.Quit
		case key.Matches(keyMsg, m.keys.Advance):
			if err := m.validateAll(); err != nil {
				m.errorText = err.Error()
				return m, nil
			}
			m.applied = true
			m.allowQuit = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *configWizardModel) initGitHubInputs() {
	owner := textinput.New()
	owner.Prompt = ""
	owner.Placeholder = "GitHub owner"
	owner.SetValue(strings.TrimSpace(m.config.GitHub.Owner))
	owner.Validate = func(v string) error {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("github.owner is required")
		}
		return nil
	}
	owner.Focus()
	m.githubOwnerInput = owner
	if m.config.GitHub.DefaultVisibility != "private" && m.config.GitHub.DefaultVisibility != "public" {
		m.config.GitHub.DefaultVisibility = "private"
	}
	if m.config.GitHub.RemoteProtocol != "ssh" && m.config.GitHub.RemoteProtocol != "https" {
		m.config.GitHub.RemoteProtocol = "ssh"
	}
	m.updateGitHubFocus()
}

func (m *configWizardModel) initSchedulerInput() {
	interval := textinput.New()
	interval.Prompt = ""
	interval.Placeholder = "minutes (>= 1)"
	interval.SetValue(strconv.Itoa(m.config.Scheduler.IntervalMinutes))
	interval.Validate = func(v string) error {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("must be an integer")
		}
		if n < 1 {
			return fmt.Errorf("must be >= 1")
		}
		return nil
	}
	interval.Blur()
	m.schedulerInterval = interval
}

func (m *configWizardModel) initNotifyInput() {
	throttle := textinput.New()
	throttle.Prompt = ""
	throttle.Placeholder = "non-negative integer"
	throttle.SetValue(strconv.Itoa(m.config.Notify.ThrottleMinutes))
	throttle.Validate = func(v string) error {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("must be an integer")
		}
		if n < 0 {
			return fmt.Errorf("must be >= 0")
		}
		return nil
	}
	throttle.Blur()
	m.notifyThrottle = throttle
}

func (m *configWizardModel) initCatalogTable() {
	cols := []table.Column{
		{Title: "Name", Width: 16},
		{Title: "Root", Width: 36},
		{Title: "Preset", Width: 12},
		{Title: "Layout", Width: 10},
		{Title: "Default", Width: 8},
		{Title: "Private Push", Width: 12},
		{Title: "Public Push", Width: 11},
	}
	m.catalogTable = table.New(
		table.WithColumns(cols),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(8),
	)
	m.applyCatalogTableStyles(false)
	m.rebuildCatalogRows()
}

func (m *configWizardModel) applyCatalogTableStyles(active bool) {
	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		Foreground(textColor).
		Background(panelBgColor).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		BorderBottom(true).
		Bold(true)
	styles.Cell = styles.Cell.Foreground(textColor)
	if active {
		styles.Selected = styles.Selected.
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("33")).
			Bold(true)
	} else {
		styles.Selected = styles.Selected.
			Foreground(textColor).
			Background(panelBgColor).
			Bold(false)
	}
	m.catalogTable.SetStyles(styles)
}

func (m *configWizardModel) resizeCatalogTable() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	w := m.viewContentWidth()
	if w <= 0 {
		w = m.width - 8
	}
	w -= 8
	if w < 50 {
		w = 50
	}

	maxVisibleHeight := m.height - 18
	if maxVisibleHeight < 6 {
		maxVisibleHeight = 6
	}
	// Keep the table compact for small catalog counts instead of filling the viewport.
	preferredHeight := len(m.catalogRows) + 3
	if preferredHeight < 6 {
		preferredHeight = 6
	}
	h := preferredHeight
	if h > maxVisibleHeight {
		h = maxVisibleHeight
	}
	m.catalogTable.SetWidth(w)
	m.catalogTable.SetHeight(h)
}

func (m *configWizardModel) updateGitHubFocus() {
	if m.focusTabs || m.githubFocus != 0 {
		m.githubOwnerInput.Blur()
		return
	}
	m.githubOwnerInput.Focus()
}

func (m *configWizardModel) updateAutomationFocus() {
	if m.focusTabs || m.step != stepAutomation {
		m.schedulerInterval.Blur()
		m.notifyThrottle.Blur()
		return
	}
	if m.automationFocus == automationFocusScheduler {
		m.schedulerInterval.Focus()
		m.notifyThrottle.Blur()
		return
	}
	m.schedulerInterval.Blur()
	if m.automationFocus == automationFocusNotifyThrottle {
		m.notifyThrottle.Focus()
		return
	}
	m.notifyThrottle.Blur()
}

func (m *configWizardModel) updateFixesFocus() {
	// No text input focus to manage yet; reserved for future fixes settings.
}

func (m *configWizardModel) updateCatalogEditorFocus() {
	if m.catalogEdit == nil {
		return
	}
	for i := range m.catalogEdit.inputs {
		if i == m.catalogEdit.focus {
			m.catalogEdit.inputs[i].Focus()
			continue
		}
		m.catalogEdit.inputs[i].Blur()
	}
}

func (m *configWizardModel) catalogEditorFocusCount() int {
	if m.catalogEdit == nil {
		return 0
	}
	switch m.catalogEdit.mode {
	case catalogEditorAdd:
		return len(m.catalogEdit.inputs) + 2 // save, cancel
	case catalogEditorEditRoot:
		return len(m.catalogEdit.inputs) + m.catalogEditorToggleCount() + 3 // toggles + save, delete, cancel
	default:
		return len(m.catalogEdit.inputs)
	}
}

func (m *configWizardModel) catalogEditorToggleCount() int {
	if m.catalogEdit == nil {
		return 0
	}
	if m.catalogEdit.mode == catalogEditorEditRoot {
		return 5
	}
	return 0
}

func (m *configWizardModel) catalogEditorActionStart() int {
	if m.catalogEdit == nil {
		return 0
	}
	return len(m.catalogEdit.inputs) + m.catalogEditorToggleCount()
}

func (m *configWizardModel) toggleSyncOption(idx int) {
	switch idx {
	case 0:
		m.config.Sync.AutoDiscover = !m.config.Sync.AutoDiscover
	case 1:
		m.config.Sync.IncludeUntrackedAsDirty = !m.config.Sync.IncludeUntrackedAsDirty
	case 2:
		m.config.Sync.DefaultAutoPushPrivate = !m.config.Sync.DefaultAutoPushPrivate
	case 3:
		m.config.Sync.DefaultAutoPushPublic = !m.config.Sync.DefaultAutoPushPublic
	case 4:
		m.config.Sync.FetchPrune = !m.config.Sync.FetchPrune
	case 5:
		m.config.Sync.PullFFOnly = !m.config.Sync.PullFFOnly
	}
}

func (m *configWizardModel) stepCount() int {
	return int(stepReview) + 1
}

func (m *configWizardModel) nextStep() error {
	if err := m.validateCurrentStep(); err != nil {
		return err
	}
	if int(m.step) < m.stepCount()-1 {
		m.step++
		m.focusTabs = m.step == stepIntro || m.step == stepReview
		m.onStepChanged()
	}
	return nil
}

func (m *configWizardModel) prevStep() {
	if m.step == stepIntro {
		return
	}
	m.step--
	m.focusTabs = m.step == stepIntro || m.step == stepReview
	m.onStepChanged()
}

func (m *configWizardModel) onStepChanged() {
	m.errorText = ""
	m.confirmQuit = false
	m.updateGitHubFocus()
	m.updateAutomationFocus()
	m.updateFixesFocus()
	m.updateCatalogFocus()
}

func (m *configWizardModel) updateCatalogFocus() {
	if m.step != stepCatalogs || m.focusTabs || m.catalogEdit != nil {
		m.catalogTable.Blur()
		m.applyCatalogTableStyles(false)
		return
	}
	if len(m.catalogRows) == 0 {
		m.catalogFocus = catalogFocusButtons
		m.catalogBtn = m.catalogButtonMin()
		m.catalogTable.Blur()
		m.applyCatalogTableStyles(false)
		return
	}
	m.catalogFocus = catalogFocusButtons
	m.normalizeCatalogButtonForSelection()
	m.catalogTable.Focus()
	m.applyCatalogTableStyles(true)
}

func (m *configWizardModel) catalogButtonMin() int {
	if len(m.machine.Catalogs) == 0 {
		return 1 // Add
	}
	if m.selectedLocalCatalogIndex() < 0 {
		return 1 // Add only when remote-only row selected
	}
	return 0 // Edit
}

func (m *configWizardModel) catalogButtonMax() int {
	if len(m.machine.Catalogs) == 0 || m.selectedLocalCatalogIndex() < 0 {
		return 1 // Add
	}
	return 6 // Edit, Add, Set Default, Toggle Layout, Toggle Private, Toggle Public, Continue
}

func (m *configWizardModel) validateCurrentStep() error {
	switch m.step {
	case stepGitHub:
		if m.githubOwnerInput.Err != nil {
			return m.githubOwnerInput.Err
		}
		if strings.TrimSpace(m.githubOwnerInput.Value()) == "" {
			return fmt.Errorf("github.owner is required")
		}
		if m.config.GitHub.DefaultVisibility != "private" && m.config.GitHub.DefaultVisibility != "public" {
			return fmt.Errorf("default visibility must be private or public")
		}
		if m.config.GitHub.RemoteProtocol != "ssh" && m.config.GitHub.RemoteProtocol != "https" {
			return fmt.Errorf("remote protocol must be ssh or https")
		}
	case stepAutomation:
		if m.schedulerInterval.Err != nil {
			return m.schedulerInterval.Err
		}
		if m.notifyThrottle.Err != nil {
			return m.notifyThrottle.Err
		}
	case stepCatalogs:
		if err := validateMachineForSave(m.machine); err != nil {
			return err
		}
	}
	return nil
}

func (m *configWizardModel) validateAll() error {
	if err := m.validateCurrentStep(); err != nil {
		return err
	}
	if v, err := strconv.Atoi(strings.TrimSpace(m.schedulerInterval.Value())); err == nil {
		m.config.Scheduler.IntervalMinutes = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(m.notifyThrottle.Value())); err == nil {
		m.config.Notify.ThrottleMinutes = v
	}
	if err := validateConfigForSave(m.config); err != nil {
		return err
	}
	if err := validateMachineForSave(m.machine); err != nil {
		return err
	}
	return nil
}

func (m *configWizardModel) recomputeDirty() {
	catalogChanged := !reflect.DeepEqual(m.machine.Catalogs, m.originalMachine.Catalogs)
	defaultChanged := m.machine.DefaultCatalog != m.originalMachine.DefaultCatalog
	configChanged := !reflect.DeepEqual(m.config, m.originalConfig)
	m.dirty = configChanged || catalogChanged || defaultChanged
}

func (m *configWizardModel) stepsHeader() string {
	labels := []string{"Intro", "GitHub", "Sync", "Automation", "Fixes", "Catalogs", "Review"}
	parts := make([]string, 0, len(labels))
	for i, l := range labels {
		if i == int(m.step) {
			active := tabCurrentStyle
			if m.focusTabs {
				active = tabFocusedStyle
			}
			parts = append(parts, active.Render(l))
			continue
		}
		parts = append(parts, tabBaseStyle.Render(l))
	}
	row := lipgloss.JoinHorizontal(lipgloss.Bottom, parts...)
	if m.width <= 0 {
		return row
	}
	gap := m.width - lipgloss.Width(row) - 4
	if gap <= 0 {
		return row
	}
	return lipgloss.JoinHorizontal(lipgloss.Bottom, row, tabGapStyle.Render(strings.Repeat(" ", gap)))
}

func (m *configWizardModel) viewIntro() string {
	mode := "reconfigure"
	if len(m.machine.Catalogs) == 0 || strings.TrimSpace(m.originalConfig.GitHub.Owner) == "" {
		mode = "onboarding"
	}
	var b strings.Builder
	b.WriteString(labelStyle.Render("Welcome"))
	b.WriteString("\n")
	b.WriteString("This wizard guides setup and reconfiguration without manual file editing.")
	b.WriteString("\n\n")
	b.WriteString(labelStyle.Render("Session mode"))
	b.WriteString("\n")
	b.WriteString(renderStatusPill(mode))
	if prerequisite := githubCLIPrerequisiteMessage(m.input.GitHubCLIStatus); prerequisite != "" {
		b.WriteString("\n\n")
		b.WriteString(labelStyle.Render("GitHub CLI prerequisite"))
		b.WriteString("\n")
		b.WriteString(prerequisite)
	}
	b.WriteString("\n\n")
	b.WriteString(labelStyle.Render("Files that may be updated"))
	b.WriteString("\n")
	b.WriteString("- " + m.input.ConfigPath)
	b.WriteString("\n")
	b.WriteString("- " + m.input.MachinePath)
	return b.String()
}

func (m *configWizardModel) viewGitHub() string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("GitHub & Transport"))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Connect bb to your GitHub defaults for repository creation and origin configuration. GitHub operations require gh in PATH and an authenticated gh session."))
	if prerequisite := githubCLIPrerequisiteMessage(m.input.GitHubCLIStatus); prerequisite != "" {
		b.WriteString("\n")
		b.WriteString(prerequisite)
	}
	b.WriteString("\n\n")
	b.WriteString(renderFieldBlock(
		!m.focusTabs && m.githubFocus == 0,
		"Repository owner",
		"GitHub user or organization that should own repositories created by bb init.",
		renderInputContainer(m.githubOwnerInput.View(), !m.focusTabs && m.githubFocus == 0),
		errorText(m.githubOwnerInput.Err),
	))
	b.WriteString("\n\n")
	b.WriteString(renderFieldBlock(
		!m.focusTabs && m.githubFocus == 1,
		"Default repository visibility",
		"Visibility used when creating new repositories.",
		renderEnumLine(m.config.GitHub.DefaultVisibility, []string{"private", "public"}),
		"",
	))
	b.WriteString("\n\n")
	b.WriteString(renderFieldBlock(
		!m.focusTabs && m.githubFocus == 2,
		"Git remote protocol",
		"URL format used for origin remotes.",
		renderEnumLine(m.config.GitHub.RemoteProtocol, []string{"ssh", "https"}),
		"",
	))
	return b.String()
}

func (m *configWizardModel) viewSync() string {
	labels := []string{
		"Discover repositories automatically",
		"Treat untracked files as dirty state",
		"Allow automatic push for private repositories",
		"Allow automatic push for public repositories",
		"Run fetch --prune during sync",
		"Require fast-forward only pulls",
	}
	descriptions := []string{
		"Scans configured catalogs for git repos during sync operations.",
		"Marks repositories unsyncable when untracked files are present.",
		"Permits syncing to push ahead commits for private repositories.",
		"Permits syncing to push ahead commits for public repositories.",
		"Keeps remote tracking refs clean before reconciliation.",
		"Prevents merge commits during automated pull operations.",
	}
	values := []bool{
		m.config.Sync.AutoDiscover,
		m.config.Sync.IncludeUntrackedAsDirty,
		m.config.Sync.DefaultAutoPushPrivate,
		m.config.Sync.DefaultAutoPushPublic,
		m.config.Sync.FetchPrune,
		m.config.Sync.PullFFOnly,
	}
	var b strings.Builder
	b.WriteString(labelStyle.Render("Sync Settings"))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Define how bb discovers repositories and performs automated sync operations."))
	b.WriteString("\n\n")
	for i := range labels {
		b.WriteString(renderToggleField(
			!m.focusTabs && i == m.syncCursor,
			labels[i],
			descriptions[i],
			values[i],
		))
		if i < len(labels)-1 {
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

func (m *configWizardModel) viewAutomation() string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Automation & Notifications"))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Configure background sync cadence and unsyncable notification behavior."))
	b.WriteString("\n\n")
	b.WriteString(renderFieldBlock(
		!m.focusTabs && m.automationFocus == automationFocusScheduler,
		"Run interval (minutes)",
		"Scheduler launch interval for background `bb sync --notify`.",
		renderInputContainer(m.schedulerInterval.View(), !m.focusTabs && m.automationFocus == automationFocusScheduler),
		errorText(m.schedulerInterval.Err),
	))
	b.WriteString("\n\n")
	b.WriteString(renderToggleField(
		!m.focusTabs && m.automationFocus == automationFocusNotifyEnabled,
		"Enable notifications",
		"Emits notification output for unsyncable repositories when --notify is used.",
		m.config.Notify.Enabled,
	))
	b.WriteString("\n\n")
	b.WriteString(renderToggleField(
		!m.focusTabs && m.automationFocus == automationFocusNotifyDedupe,
		"Deduplicate notifications",
		"Suppresses repeated notifications for unchanged unsyncable states.",
		m.config.Notify.Dedupe,
	))
	b.WriteString("\n\n")
	b.WriteString(renderFieldBlock(
		!m.focusTabs && m.automationFocus == automationFocusNotifyThrottle,
		"Notification throttle (minutes)",
		"Minimum minutes between notifications for the same repository.",
		renderInputContainer(m.notifyThrottle.View(), !m.focusTabs && m.automationFocus == automationFocusNotifyThrottle),
		errorText(m.notifyThrottle.Err),
	))
	return b.String()
}

func (m *configWizardModel) viewFixes() string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Fixes"))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Configure commit-message defaults for `bb fix` commit-producing actions."))
	b.WriteString("\n\n")
	b.WriteString(renderToggleFieldWithAvailability(
		!m.focusTabs && m.fixesFocus == fixesFocusLumenAutoCommit,
		m.lumenAvailable,
		"Use Lumen for empty commit messages",
		"When enabled, `bb fix` runs `lumen draft` to use AI/LLM generation of commit messages from staged changes when message is empty or set to auto.",
		m.config.Integrations.Lumen.AutoGenerateCommitMessageWhenEmpty,
	))
	if !m.lumenAvailable {
		b.WriteString("\n\n")
		b.WriteString(renderFieldBlock(
			false,
			"Lumen required",
			"Install and configure Lumen to enable AI commit message generation.",
			hintStyle.Render(lumenInstallTipText),
			"",
		))
	}
	return b.String()
}

func (m *configWizardModel) viewCatalogs() string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Catalog Management"))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Catalogs define root folders where bb discovers repositories."))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Use Edit to configure root path, clone preset mapping, layout depth, default-branch auto-push policy, and sync clone behavior for the selected catalog."))
	if len(m.input.KnownCatalogRoots) > 0 {
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("Remote-known catalogs are listed as read-only rows; select one and choose Add to prefill local mapping."))
	}
	b.WriteString("\n\n")
	if m.catalogEdit != nil {
		b.WriteString(panelStyle.Render(m.viewCatalogEditor()))
		return b.String()
	}
	if len(m.catalogRows) == 0 {
		emptyState := strings.Join([]string{
			labelStyle.Render("No catalogs configured yet"),
			hintStyle.Render("Use Add to define your first catalog."),
			hintStyle.Render("Examples: /Volumes/Projects/Software or /Users/you/Code"),
		}, "\n")
		b.WriteString(panelStyle.
			Border(lipgloss.NormalBorder()).
			BorderForeground(borderColor).
			Background(panelBgColor).
			Padding(1, 2).
			Render(emptyState))
	} else {
		b.WriteString(m.catalogTable.View())
	}
	b.WriteString("\n\n")
	b.WriteString(renderCatalogActions(
		!m.focusTabs && m.catalogFocus == catalogFocusButtons && m.catalogBtn == 0,
		!m.focusTabs && m.catalogFocus == catalogFocusButtons && m.catalogBtn == 1,
		!m.focusTabs && m.catalogFocus == catalogFocusButtons && m.catalogBtn == 2,
		!m.focusTabs && m.catalogFocus == catalogFocusButtons && m.catalogBtn == 3,
		!m.focusTabs && m.catalogFocus == catalogFocusButtons && m.catalogBtn == 4,
		!m.focusTabs && m.catalogFocus == catalogFocusButtons && m.catalogBtn == 5,
		!m.focusTabs && m.catalogFocus == catalogFocusButtons && m.catalogBtn == 6,
		len(m.machine.Catalogs) > 0,
		m.selectedCatalogPolicySummary(),
	))
	return b.String()
}

func (m *configWizardModel) viewCatalogEditor() string {
	if m.catalogEdit == nil {
		return ""
	}
	var b strings.Builder
	if m.catalogEdit.mode == catalogEditorAdd {
		b.WriteString(labelStyle.Render("Add catalog"))
		b.WriteString("\n\n")
		b.WriteString(renderFieldBlock(
			m.catalogEdit.focus == 0,
			"Catalog name",
			"Human-friendly name used in bb catalog commands.",
			renderInputContainer(m.catalogEdit.inputs[0].View(), m.catalogEdit.focus == 0),
			"",
		))
		b.WriteString("\n")
		b.WriteString(renderFieldBlock(
			m.catalogEdit.focus == 1,
			"Catalog root path",
			"Absolute path where repositories should be discovered. Use Left/Right to cycle suggested roots when available.",
			renderInputContainer(m.catalogEdit.inputs[1].View(), m.catalogEdit.focus == 1),
			"",
		))
		if len(m.catalogEdit.rootSuggestions) > 0 {
			b.WriteString("\n")
			b.WriteString(hintStyle.Render("Suggested roots from other machines:"))
			b.WriteString("\n")
			b.WriteString(hintStyle.Render(strings.Join(m.catalogEdit.rootSuggestions, "  |  ")))
		}
		b.WriteString("\n\n")
		b.WriteString(renderEditorActions(
			false,
			m.catalogEdit.focus == 2,
			false,
			m.catalogEdit.focus == 3,
		))
	} else {
		b.WriteString(labelStyle.Render("Edit catalog"))
		b.WriteString("\n\n")
		b.WriteString(renderFieldBlock(
			m.catalogEdit.focus == 0,
			renderWarningTitle("Catalog root path"),
			"Absolute path where repositories should be discovered.\nChanging this does not move repositories on disk and can break bb sync until paths match.",
			renderInputContainer(m.catalogEdit.inputs[0].View(), m.catalogEdit.focus == 0),
			"",
		))
		b.WriteString("\n")
		b.WriteString(renderFieldBlock(
			m.catalogEdit.focus == 1,
			"Clone preset mapping",
			"Preset from clone.presets applied to this catalog.",
			renderEnumLine(m.catalogEdit.presetValue, m.catalogEdit.presetOptions),
			"",
		))
		b.WriteString("\n")
		b.WriteString(renderFieldBlock(
			m.catalogEdit.focus == 2,
			renderWarningTitle("Repository layout depth"),
			"Choose how repo paths are derived under this catalog root.\nChanging this does not move repositories on disk and can break bb sync until paths match.",
			renderEnumLine(strconv.Itoa(m.catalogEdit.repoPathDepth), []string{"1", "2"}),
			"",
		))
		b.WriteString("\n")
		b.WriteString(renderFieldBlock(
			m.catalogEdit.focus == 3,
			"Private default-branch auto-push",
			"Default auto-push behavior for private repositories in this catalog.",
			renderCheckbox(m.catalogEdit.privatePush),
			"",
		))
		b.WriteString("\n")
		b.WriteString(renderFieldBlock(
			m.catalogEdit.focus == 4,
			"Public default-branch auto-push",
			"Default auto-push behavior for public repositories in this catalog.",
			renderCheckbox(m.catalogEdit.publicPush),
			"",
		))
		b.WriteString("\n")
		b.WriteString(renderFieldBlock(
			m.catalogEdit.focus == 5,
			"Auto clone during sync",
			"When enabled, bb sync can clone missing local copies for this catalog.\nWhen disabled, missing copies are marked clone_required and should be handled via bb fix clone.",
			renderCheckbox(m.catalogEdit.autoCloneOnSync),
			"",
		))
		b.WriteString("\n\n")
		b.WriteString(renderEditorActions(
			true,
			m.catalogEdit.focus == 6,
			m.catalogEdit.focus == 7,
			m.catalogEdit.focus == 8,
		))
	}
	if m.catalogEdit.err != "" {
		b.WriteString("\n")
		b.WriteString(alertStyle.Render(errorStyle.Render(m.catalogEdit.err)))
	}
	return b.String()
}

func (m *configWizardModel) viewReview() string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Review"))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Inspect pending changes before writing configuration files."))
	b.WriteString("\n\n")
	lines := m.diffLines()
	if len(lines) == 0 {
		b.WriteString(renderFieldBlock(false, "Pending changes", "No differences from current configuration.", "", ""))
	} else {
		diff := strings.Builder{}
		for _, line := range lines {
			diff.WriteString("• ")
			diff.WriteString(line)
			diff.WriteString("\n")
		}
		b.WriteString(renderFieldBlock(false, "Pending changes", "", strings.TrimSpace(diff.String()), ""))
	}
	missing := missingCatalogRoots(m.machine.Catalogs)
	if len(missing) > 0 {
		b.WriteString("\n")
		roots := strings.Builder{}
		for _, root := range missing {
			roots.WriteString("• ")
			roots.WriteString(root)
			roots.WriteString("\n")
		}
		b.WriteString(renderFieldBlock(
			false,
			"Missing catalog roots",
			"These paths do not exist yet.",
			strings.TrimSpace(roots.String()),
			"",
		))
		b.WriteString("\n")
		b.WriteString(renderFieldBlock(
			false,
			"Create missing roots on apply",
			"When enabled, bb creates missing catalog root folders before saving.",
			renderCheckbox(m.createMissingRoots),
			"",
		))
	}
	return b.String()
}

func (m *configWizardModel) diffLines() []string {
	out := []string{}
	if m.originalConfig.GitHub.Owner != m.config.GitHub.Owner {
		out = append(out, fmt.Sprintf("github.owner: %q -> %q", m.originalConfig.GitHub.Owner, m.config.GitHub.Owner))
	}
	if m.originalConfig.GitHub.DefaultVisibility != m.config.GitHub.DefaultVisibility {
		out = append(out, fmt.Sprintf("github.default_visibility: %q -> %q", m.originalConfig.GitHub.DefaultVisibility, m.config.GitHub.DefaultVisibility))
	}
	if m.originalConfig.GitHub.RemoteProtocol != m.config.GitHub.RemoteProtocol {
		out = append(out, fmt.Sprintf("github.remote_protocol: %q -> %q", m.originalConfig.GitHub.RemoteProtocol, m.config.GitHub.RemoteProtocol))
	}
	if !reflect.DeepEqual(m.originalConfig.Clone, m.config.Clone) {
		out = append(out, "clone settings updated")
	}
	if m.originalConfig.Link != m.config.Link {
		out = append(out, "link settings updated")
	}
	if m.originalConfig.Sync != m.config.Sync {
		out = append(out, "sync settings updated")
	}
	if m.originalConfig.Scheduler != m.config.Scheduler {
		out = append(out, "scheduler settings updated")
	}
	if m.originalConfig.Notify != m.config.Notify {
		out = append(out, "notify settings updated")
	}
	if m.originalConfig.Integrations != m.config.Integrations {
		out = append(out, "integration settings updated")
	}
	if !reflect.DeepEqual(m.originalMachine.Catalogs, m.machine.Catalogs) {
		out = append(out, "catalog list updated")
	}
	if m.originalMachine.DefaultCatalog != m.machine.DefaultCatalog {
		out = append(out, fmt.Sprintf("default_catalog: %q -> %q", m.originalMachine.DefaultCatalog, m.machine.DefaultCatalog))
	}
	return out
}

func renderStatusPill(value string) string {
	return lipgloss.NewStyle().
		Foreground(textColor).
		Background(accentBgColor).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Padding(0, 1).
		Bold(true).
		Render(strings.ToUpper(value))
}

func githubCLIPrerequisiteMessage(status GitHubCLIStatus) string {
	if !status.Checked {
		return ""
	}
	if !status.Installed {
		return warnStyle.Render("GitHub operations require gh. Install with `brew install gh`, then run `gh auth login`.")
	}
	if !status.Authenticated {
		msg := "gh is installed but not authenticated. Run `gh auth login`, then verify with `gh auth status`."
		if details := strings.TrimSpace(status.AuthStatus); details != "" {
			msg = msg + " Current status: " + details
		}
		return warnStyle.Render(msg)
	}
	return hintStyle.Render("gh is installed and authenticated (`gh auth status`).")
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func renderWarningTitle(title string) string {
	return warnStyle.Render("⚠ ") + title
}

func renderInputContainer(input string, focused bool) string {
	style := inputStyle
	if focused {
		style = inputFocusStyle
	}
	return style.Render(input)
}

func renderCheckbox(v bool) string {
	if v {
		return switchOnStyle.Render("● ON")
	}
	return switchOffStyle.Render("○ OFF")
}

func renderFieldBlock(focused bool, title, description, value, err string) string {
	var b strings.Builder
	b.WriteString(labelStyle.Render(title))
	if description != "" {
		b.WriteString("\n")
		b.WriteString(hintStyle.Render(description))
	}
	if value != "" {
		b.WriteString("\n")
		b.WriteString(value)
	}
	if err != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(err))
	}
	style := fieldStyle
	if focused {
		style = fieldFocusStyle
	}
	return style.Render(b.String())
}

func renderToggleField(focused bool, title, description string, value bool) string {
	return renderToggleFieldWithAvailability(focused, true, title, description, value)
}

func renderToggleFieldWithAvailability(focused bool, available bool, title, description string, value bool) string {
	var b strings.Builder
	switch {
	case available:
		b.WriteString(renderCheckbox(value))
	default:
		if value {
			b.WriteString(switchOnStyle.Copy().Foreground(mutedTextColor).Background(lipgloss.AdaptiveColor{Light: "#F6F8FA", Dark: "#161B22"}).Render("● ON"))
		} else {
			b.WriteString(switchOffStyle.Copy().Foreground(mutedTextColor).Render("○ OFF"))
		}
	}
	b.WriteString(" ")
	titleStyle := labelStyle
	if !available {
		titleStyle = hintStyle.Copy()
	}
	b.WriteString(titleStyle.Render(title))
	if description != "" {
		b.WriteString("\n")
		b.WriteString(hintStyle.Render(description))
	}
	style := fieldStyle
	if !available {
		style = style.BorderForeground(mutedTextColor)
	} else if focused {
		style = fieldFocusStyle
	}
	return style.Render(b.String())
}

func renderButtonLabel(label string, focused bool) string {
	if focused {
		return "[" + label + "]"
	}
	return " " + label + " "
}

func renderCatalogActions(
	editFocused, addFocused, setDefaultFocused, toggleLayoutFocused, togglePrivateFocused, togglePublicFocused, continueFocused bool,
	hasCatalogs bool,
	policySummary string,
) string {
	editIsFocused := hasCatalogs && editFocused
	addIsFocused := addFocused
	setDefaultIsFocused := hasCatalogs && setDefaultFocused
	toggleLayoutIsFocused := hasCatalogs && toggleLayoutFocused
	togglePrivateIsFocused := hasCatalogs && togglePrivateFocused
	togglePublicIsFocused := hasCatalogs && togglePublicFocused
	continueIsFocused := hasCatalogs && continueFocused

	editStyle := buttonStyle
	if !hasCatalogs {
		editStyle = buttonDisabledStyle
	} else if editIsFocused {
		editStyle = buttonFocusStyle
	}
	addStyle := buttonStyle
	if addIsFocused {
		addStyle = buttonFocusStyle
	}
	setDefaultStyle := buttonStyle
	if !hasCatalogs {
		setDefaultStyle = buttonDisabledStyle
	} else if setDefaultIsFocused {
		setDefaultStyle = buttonFocusStyle
	}
	toggleLayoutStyle := buttonStyle
	if !hasCatalogs {
		toggleLayoutStyle = buttonDisabledStyle
	} else if toggleLayoutIsFocused {
		toggleLayoutStyle = buttonFocusStyle
	}
	togglePrivateStyle := buttonStyle
	if !hasCatalogs {
		togglePrivateStyle = buttonDisabledStyle
	} else if togglePrivateIsFocused {
		togglePrivateStyle = buttonFocusStyle
	}
	togglePublicStyle := buttonStyle
	if !hasCatalogs {
		togglePublicStyle = buttonDisabledStyle
	} else if togglePublicIsFocused {
		togglePublicStyle = buttonFocusStyle
	}
	continueStyle := buttonPrimaryStyle
	if !hasCatalogs {
		continueStyle = buttonDisabledStyle
	}
	if continueIsFocused {
		continueStyle = buttonPrimaryFocusStyle
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		editStyle.Render(renderButtonLabel("Edit", editIsFocused)),
		addStyle.Render(renderButtonLabel("Add", addIsFocused)),
		setDefaultStyle.Render(renderButtonLabel("Set Default", setDefaultIsFocused)),
		toggleLayoutStyle.Render(renderButtonLabel("Toggle Layout", toggleLayoutIsFocused)),
		togglePrivateStyle.Render(renderButtonLabel("Toggle Private", togglePrivateIsFocused)),
		togglePublicStyle.Render(renderButtonLabel("Toggle Public", togglePublicIsFocused)),
		continueStyle.Render(renderButtonLabel("Continue", continueIsFocused)),
	) + "\n" + hintStyle.Render(policySummary)
}

func renderEditorActions(includeDelete, saveFocused, deleteFocused, cancelFocused bool) string {
	saveStyle := buttonPrimaryStyle
	if saveFocused {
		saveStyle = buttonPrimaryFocusStyle
	}
	cancelStyle := buttonStyle
	if cancelFocused {
		cancelStyle = buttonFocusStyle
	}
	parts := []string{saveStyle.Render(renderButtonLabel("Save", saveFocused))}
	if includeDelete {
		deleteStyle := buttonDangerStyle
		if deleteFocused {
			deleteStyle = buttonDangerFocusStyle
		}
		parts = append(parts, deleteStyle.Render(renderButtonLabel("Delete", deleteFocused)))
	}
	parts = append(parts, cancelStyle.Render(renderButtonLabel("Cancel", cancelFocused)))
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func shiftEnumValue(current string, options []string, delta int) string {
	if len(options) == 0 {
		return current
	}
	for i, option := range options {
		if option == current {
			next := i + delta
			for next < 0 {
				next += len(options)
			}
			return options[next%len(options)]
		}
	}
	return options[0]
}

func renderEnumLine(current string, options []string) string {
	parts := make([]string, 0, len(options))
	for _, option := range options {
		label := "○ " + option
		style := enumOptionStyle
		if current == option {
			label = "● " + option
			style = enumOptionActiveStyle
		}
		parts = append(parts, style.Render(label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m *configWizardModel) rebuildCatalogRows() {
	m.catalogRows = make([]catalogTableRow, 0, len(m.machine.Catalogs))
	localCatalogNames := map[string]struct{}{}
	for _, c := range m.machine.Catalogs {
		def := ""
		if c.Name == m.machine.DefaultCatalog {
			def = "yes"
		}
		localCatalogNames[c.Name] = struct{}{}
		m.catalogRows = append(m.catalogRows, catalogTableRow{
			Name:        c.Name,
			Root:        c.Root,
			Preset:      strings.TrimSpace(m.config.Clone.CatalogPreset[c.Name]),
			Layout:      repoPathDepthLabel(c),
			Default:     def,
			PrivatePush: onOffLabel(c.AllowsDefaultBranchAutoPush(domain.VisibilityPrivate)),
			PublicPush:  onOffLabel(c.AllowsDefaultBranchAutoPush(domain.VisibilityPublic)),
		})
	}
	remoteCatalogNames := make([]string, 0, len(m.input.KnownCatalogRoots))
	for name := range m.input.KnownCatalogRoots {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if _, exists := localCatalogNames[trimmed]; exists {
			continue
		}
		remoteCatalogNames = append(remoteCatalogNames, trimmed)
	}
	sort.Strings(remoteCatalogNames)
	for _, name := range remoteCatalogNames {
		roots := sanitizeCatalogRootSuggestions(m.input.KnownCatalogRoots[name])
		rootDisplay := "(unknown root)"
		if len(roots) > 0 {
			rootDisplay = roots[0]
			if len(roots) > 1 {
				rootDisplay = fmt.Sprintf("%s (+%d alt)", roots[0], len(roots)-1)
			}
		}
		m.catalogRows = append(m.catalogRows, catalogTableRow{
			Name:           name,
			Root:           rootDisplay,
			Preset:         "-",
			Layout:         "-",
			Default:        "remote-only",
			PrivatePush:    "-",
			PublicPush:     "-",
			RemoteOnly:     true,
			SuggestedRoots: roots,
		})
	}
	rows := make([]table.Row, 0, len(m.catalogRows))
	for _, row := range m.catalogRows {
		name := row.Name
		if row.RemoteOnly {
			name = "remote: " + name
		}
		rows = append(rows, table.Row{
			name,
			row.Root,
			row.Preset,
			row.Layout,
			row.Default,
			row.PrivatePush,
			row.PublicPush,
		})
	}
	m.catalogTable.SetRows(rows)
	if cursor := m.catalogTable.Cursor(); cursor >= len(rows) && len(rows) > 0 {
		m.catalogTable.SetCursor(len(rows) - 1)
	}
	m.normalizeCatalogButtonForSelection()
	m.resizeCatalogTable()
}

func sanitizeCatalogRootSuggestions(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func onOffLabel(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func repoPathDepthLabel(c domain.Catalog) string {
	if domain.EffectiveRepoPathDepth(c) == 2 {
		return "2-level"
	}
	return "1-level"
}

func (m *configWizardModel) catalogPresetOptions() []string {
	seen := map[string]struct{}{}
	options := []string{"none"}
	seen["none"] = struct{}{}

	presets := make([]string, 0, len(m.config.Clone.Presets)+1)
	for name := range m.config.Clone.Presets {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" || trimmed == "none" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		presets = append(presets, trimmed)
	}
	if _, ok := seen["references"]; !ok {
		presets = append(presets, "references")
	}
	sort.Strings(presets)
	return append(options, presets...)
}

func builtinReferencesClonePreset() domain.ClonePreset {
	shallow := true
	filter := "blob:none"
	return domain.ClonePreset{
		Shallow: &shallow,
		Filter:  &filter,
	}
}

func (m *configWizardModel) startCatalogAddEditor() {
	m.startCatalogAddEditorWithTemplate("", nil)
}

func (m *configWizardModel) startCatalogAddEditorWithTemplate(nameValue string, rootSuggestions []string) {
	name := textinput.New()
	name.Prompt = ""
	name.Placeholder = "catalog name"
	name.SetValue(strings.TrimSpace(nameValue))
	if strings.TrimSpace(name.Value()) == "" {
		name.Focus()
	} else {
		name.Blur()
	}
	root := textinput.New()
	root.Prompt = ""
	root.Placeholder = "/absolute/path"
	rootSuggestions = sanitizeCatalogRootSuggestions(rootSuggestions)
	if len(rootSuggestions) > 0 {
		root.SetValue(rootSuggestions[0])
	}
	if strings.TrimSpace(name.Value()) == "" {
		root.Blur()
	} else {
		root.Focus()
	}
	initialFocus := 0
	if strings.TrimSpace(name.Value()) != "" {
		initialFocus = 1
	}
	m.catalogEdit = &catalogEditor{
		mode:            catalogEditorAdd,
		inputs:          []textinput.Model{name, root},
		rootSuggestions: rootSuggestions,
		rootSuggestion:  0,
		focus:           initialFocus,
		row:             -1,
	}
	m.catalogTable.Blur()
	m.updateCatalogEditorFocus()
}

func (m *configWizardModel) startCatalogEditRootEditor() {
	idx := m.selectedLocalCatalogIndex()
	if idx < 0 || idx >= len(m.machine.Catalogs) {
		m.errorText = "select a catalog first"
		return
	}
	catalog := m.machine.Catalogs[idx]
	root := textinput.New()
	root.Prompt = ""
	root.Placeholder = "/absolute/path"
	root.SetValue(catalog.Root)
	root.Focus()
	presetOptions := m.catalogPresetOptions()
	presetValue := strings.TrimSpace(m.config.Clone.CatalogPreset[catalog.Name])
	if presetValue == "" {
		presetValue = "none"
	}
	seenPreset := false
	for _, option := range presetOptions {
		if option == presetValue {
			seenPreset = true
			break
		}
	}
	if !seenPreset && presetValue != "" {
		presetOptions = append(presetOptions, presetValue)
		sort.Strings(presetOptions[1:])
	}
	m.catalogEdit = &catalogEditor{
		mode:            catalogEditorEditRoot,
		inputs:          []textinput.Model{root},
		presetOptions:   presetOptions,
		presetValue:     presetValue,
		focus:           0,
		row:             idx,
		repoPathDepth:   domain.EffectiveRepoPathDepth(catalog),
		privatePush:     catalog.AllowsDefaultBranchAutoPush(domain.VisibilityPrivate),
		publicPush:      catalog.AllowsDefaultBranchAutoPush(domain.VisibilityPublic),
		autoCloneOnSync: catalog.AllowsAutoCloneOnSync(),
	}
	m.catalogTable.Blur()
}

func (m *configWizardModel) commitCatalogEditor() error {
	editor := m.catalogEdit
	if editor == nil {
		return nil
	}
	switch editor.mode {
	case catalogEditorAdd:
		name := strings.TrimSpace(editor.inputs[0].Value())
		root := strings.TrimSpace(editor.inputs[1].Value())
		if name == "" {
			return fmt.Errorf("catalog name is required")
		}
		for _, c := range m.machine.Catalogs {
			if c.Name == name {
				return fmt.Errorf("catalog %q already exists", name)
			}
		}
		if root == "" {
			return fmt.Errorf("catalog root is required")
		}
		if !filepath.IsAbs(root) {
			return fmt.Errorf("catalog root must be absolute")
		}
		m.machine.Catalogs = append(m.machine.Catalogs, domain.Catalog{
			Name:          name,
			Root:          root,
			RepoPathDepth: domain.DefaultRepoPathDepth,
		})
		if strings.TrimSpace(m.machine.DefaultCatalog) == "" {
			m.machine.DefaultCatalog = name
		}
		m.rebuildCatalogRows()
		m.catalogTable.SetCursor(len(m.machine.Catalogs) - 1)
		return nil
	case catalogEditorEditRoot:
		if editor.row < 0 || editor.row >= len(m.machine.Catalogs) {
			return fmt.Errorf("invalid catalog selection")
		}
		root := strings.TrimSpace(editor.inputs[0].Value())
		if root == "" {
			return fmt.Errorf("catalog root is required")
		}
		if !filepath.IsAbs(root) {
			return fmt.Errorf("catalog root must be absolute")
		}
		preset := strings.TrimSpace(editor.presetValue)
		if preset == "none" {
			preset = ""
		}
		if preset != "" {
			if _, ok := m.config.Clone.Presets[preset]; !ok {
				if preset == "references" {
					m.config.Clone.Presets[preset] = builtinReferencesClonePreset()
				} else {
					return fmt.Errorf("clone preset %q is not defined", preset)
				}
			}
		}

		m.machine.Catalogs[editor.row].Root = root
		if editor.repoPathDepth == 2 {
			m.machine.Catalogs[editor.row].RepoPathDepth = 2
		} else {
			m.machine.Catalogs[editor.row].RepoPathDepth = 1
		}
		privatePush := editor.privatePush
		publicPush := editor.publicPush
		autoCloneOnSync := editor.autoCloneOnSync
		m.machine.Catalogs[editor.row].AllowAutoPushDefaultBranchPrivate = &privatePush
		m.machine.Catalogs[editor.row].AllowAutoPushDefaultBranchPublic = &publicPush
		m.machine.Catalogs[editor.row].AutoCloneOnSync = &autoCloneOnSync

		name := m.machine.Catalogs[editor.row].Name
		if m.config.Clone.CatalogPreset == nil {
			m.config.Clone.CatalogPreset = map[string]string{}
		}
		if preset == "" {
			delete(m.config.Clone.CatalogPreset, name)
		} else {
			m.config.Clone.CatalogPreset[name] = preset
		}
		m.rebuildCatalogRows()
		m.catalogTable.SetCursor(editor.row)
		return nil
	default:
		return nil
	}
}

func (m *configWizardModel) deleteSelectedCatalog() error {
	idx := m.selectedLocalCatalogIndex()
	return m.deleteCatalogAt(idx)
}

func (m *configWizardModel) deleteCatalogAt(idx int) error {
	if idx < 0 || idx >= len(m.machine.Catalogs) {
		return fmt.Errorf("select a catalog first")
	}
	name := m.machine.Catalogs[idx].Name
	out := make([]domain.Catalog, 0, len(m.machine.Catalogs)-1)
	out = append(out, m.machine.Catalogs[:idx]...)
	out = append(out, m.machine.Catalogs[idx+1:]...)
	m.machine.Catalogs = out
	if m.machine.DefaultCatalog == name {
		m.machine.DefaultCatalog = ""
		if len(m.machine.Catalogs) > 0 {
			m.machine.DefaultCatalog = m.machine.Catalogs[0].Name
		}
	}
	m.rebuildCatalogRows()
	if len(m.machine.Catalogs) == 0 {
		m.catalogTable.SetCursor(0)
		m.catalogFocus = catalogFocusButtons
		m.catalogBtn = m.catalogButtonMin()
		return nil
	}
	if idx >= len(m.machine.Catalogs) {
		idx = len(m.machine.Catalogs) - 1
	}
	m.catalogTable.SetCursor(idx)
	return nil
}

func (m *configWizardModel) setDefaultFromSelection() error {
	idx := m.selectedLocalCatalogIndex()
	if idx < 0 || idx >= len(m.machine.Catalogs) {
		return fmt.Errorf("select a catalog first")
	}
	m.machine.DefaultCatalog = m.machine.Catalogs[idx].Name
	m.rebuildCatalogRows()
	return nil
}

func (m *configWizardModel) toggleRepoPathDepthFromSelection() error {
	idx := m.selectedLocalCatalogIndex()
	if idx < 0 || idx >= len(m.machine.Catalogs) {
		return fmt.Errorf("select a catalog first")
	}
	current := domain.EffectiveRepoPathDepth(m.machine.Catalogs[idx])
	if current == 2 {
		m.machine.Catalogs[idx].RepoPathDepth = 1
	} else {
		m.machine.Catalogs[idx].RepoPathDepth = 2
	}
	m.rebuildCatalogRows()
	m.catalogTable.SetCursor(idx)
	return nil
}

func (m *configWizardModel) toggleDefaultBranchAutoPushFromSelection(visibility domain.Visibility) error {
	idx := m.selectedLocalCatalogIndex()
	if idx < 0 || idx >= len(m.machine.Catalogs) {
		return fmt.Errorf("select a catalog first")
	}
	current := m.machine.Catalogs[idx]
	switch visibility {
	case domain.VisibilityPrivate:
		next := !current.AllowsDefaultBranchAutoPush(domain.VisibilityPrivate)
		m.machine.Catalogs[idx].AllowAutoPushDefaultBranchPrivate = &next
	case domain.VisibilityPublic:
		next := !current.AllowsDefaultBranchAutoPush(domain.VisibilityPublic)
		m.machine.Catalogs[idx].AllowAutoPushDefaultBranchPublic = &next
	default:
		return fmt.Errorf("unsupported visibility %q", visibility)
	}
	m.rebuildCatalogRows()
	m.catalogTable.SetCursor(idx)
	return nil
}

func (m *configWizardModel) shiftCatalogEditorOption(toggleIndex int, delta int) error {
	editor := m.catalogEdit
	if editor == nil || editor.mode != catalogEditorEditRoot {
		return nil
	}
	switch toggleIndex {
	case 0:
		editor.presetValue = shiftEnumValue(editor.presetValue, editor.presetOptions, delta)
	case 1:
		if editor.repoPathDepth == 2 {
			editor.repoPathDepth = 1
		} else {
			editor.repoPathDepth = 2
		}
	case 2:
		editor.privatePush = !editor.privatePush
	case 3:
		editor.publicPush = !editor.publicPush
	case 4:
		editor.autoCloneOnSync = !editor.autoCloneOnSync
	default:
		return fmt.Errorf("invalid editor toggle selection")
	}
	return nil
}

func (m *configWizardModel) selectedCatalogPolicySummary() string {
	row, ok := m.selectedCatalogTableRow()
	if !ok {
		return "Select a catalog to change default-branch auto-push policy."
	}
	if row.RemoteOnly {
		suggestion := "(no known roots)"
		if len(row.SuggestedRoots) > 0 {
			suggestion = strings.Join(row.SuggestedRoots, ", ")
		}
		return fmt.Sprintf("Selected remote-only %q. Use Add to create local mapping. Suggested roots: %s", row.Name, suggestion)
	}
	idx := m.selectedLocalCatalogIndex()
	if idx < 0 || idx >= len(m.machine.Catalogs) {
		return "Select a catalog to change default-branch auto-push policy."
	}
	c := m.machine.Catalogs[idx]
	preset := strings.TrimSpace(m.config.Clone.CatalogPreset[c.Name])
	if preset == "" {
		preset = "-"
	}
	return fmt.Sprintf(
		"Selected %q: preset=%s, layout=%s, private=%s, public=%s, auto_clone_sync=%s",
		c.Name,
		preset,
		repoPathDepthLabel(c),
		onOffLabel(c.AllowsDefaultBranchAutoPush(domain.VisibilityPrivate)),
		onOffLabel(c.AllowsDefaultBranchAutoPush(domain.VisibilityPublic)),
		onOffLabel(c.AllowsAutoCloneOnSync()),
	)
}

func (m *configWizardModel) selectedCatalogTableRow() (catalogTableRow, bool) {
	idx := m.catalogTable.Cursor()
	if idx < 0 || idx >= len(m.catalogRows) {
		return catalogTableRow{}, false
	}
	return m.catalogRows[idx], true
}

func (m *configWizardModel) selectedLocalCatalogIndex() int {
	row, ok := m.selectedCatalogTableRow()
	if !ok || row.RemoteOnly {
		return -1
	}
	for i, catalog := range m.machine.Catalogs {
		if strings.TrimSpace(catalog.Name) == strings.TrimSpace(row.Name) {
			return i
		}
	}
	return -1
}

func (m *configWizardModel) normalizeCatalogButtonForSelection() {
	min := m.catalogButtonMin()
	max := m.catalogButtonMax()
	if m.catalogBtn < min || m.catalogBtn > max {
		m.catalogBtn = min
	}
}

func (m *configWizardModel) shiftCatalogAddRootSuggestion(delta int) {
	editor := m.catalogEdit
	if editor == nil || editor.mode != catalogEditorAdd || len(editor.rootSuggestions) == 0 {
		return
	}
	next := editor.rootSuggestion + delta
	for next < 0 {
		next += len(editor.rootSuggestions)
	}
	next = next % len(editor.rootSuggestions)
	editor.rootSuggestion = next
	editor.inputs[1].SetValue(editor.rootSuggestions[next])
}
