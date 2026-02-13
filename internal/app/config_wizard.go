package app

import (
	"fmt"
	"path/filepath"
	"reflect"
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
	stepNotify
	stepCatalogs
	stepReview
)

type catalogEditorMode int

const (
	catalogEditorAdd catalogEditorMode = iota
	catalogEditorEditRoot
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
		{k.CatalogAdd, k.CatalogEdit, k.CatalogDelete, k.CatalogDefault},
	}
}

func defaultConfigWizardKeyMap() configWizardKeyMap {
	return configWizardKeyMap{
		NextStep: key.NewBinding(
			key.WithKeys("right"),
			key.WithHelp("right", "next step (tabs focus)"),
		),
		PrevStep: key.NewBinding(
			key.WithKeys("left"),
			key.WithHelp("left", "prev step (tabs focus)"),
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
			key.WithHelp("space", "toggle"),
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
			key.WithHelp("e", "edit root"),
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
	mode   catalogEditorMode
	inputs []textinput.Model
	focus  int
	row    int
	err    string
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

	notifyThrottle textinput.Model
	notifyFocus    int

	catalogTable table.Model
	catalogEdit  *catalogEditor

	createMissingRoots bool
}

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	labelStyle  = lipgloss.NewStyle().Bold(true)
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	hintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	focusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	tabStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
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
		createMissingRoots: true,
	}
	if m.machine.Catalogs == nil {
		m.machine.Catalogs = []domain.Catalog{}
	}
	if m.config.StateTransport.Mode != "external" {
		m.config.StateTransport.Mode = "external"
	}
	m.initGitHubInputs()
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
	case stepNotify:
		return m.updateNotify(msg)
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

	b.WriteString(headerStyle.Render("bb config"))
	b.WriteString("\n")
	b.WriteString(m.stepsHeader())
	b.WriteString("\n\n")

	switch m.step {
	case stepIntro:
		b.WriteString(m.viewIntro())
	case stepGitHub:
		b.WriteString(m.viewGitHub())
	case stepSync:
		b.WriteString(m.viewSync())
	case stepNotify:
		b.WriteString(m.viewNotify())
	case stepCatalogs:
		b.WriteString(m.viewCatalogs())
	case stepReview:
		b.WriteString(m.viewReview())
	}

	b.WriteString("\n")
	if m.errorText != "" {
		b.WriteString(errorStyle.Render(m.errorText))
		b.WriteString("\n")
	}
	if m.confirmQuit {
		b.WriteString(errorStyle.Render("Press Ctrl+C again to discard changes and quit."))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.help.View(m.keys))
	b.WriteString("\n")
	return b.String()
}

func (m *configWizardModel) updateIntro(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if key.Matches(keyMsg, m.keys.Advance) {
			m.advanceStep()
		}
	}
	return m, nil
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

func (m *configWizardModel) updateNotify(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up":
			if m.focusTabs {
				return m, nil
			}
			if m.notifyFocus == 0 {
				m.focusTabs = true
				m.updateNotifyFocus()
				return m, nil
			}
			m.notifyFocus--
			m.updateNotifyFocus()
			return m, nil
		case "down":
			if m.focusTabs {
				m.focusTabs = false
				m.notifyFocus = 0
				m.updateNotifyFocus()
				return m, nil
			}
			if m.notifyFocus < 2 {
				m.notifyFocus++
			}
			m.updateNotifyFocus()
			return m, nil
		case " ":
			if m.focusTabs {
				return m, nil
			}
			if m.notifyFocus == 0 {
				m.config.Notify.Enabled = !m.config.Notify.Enabled
				m.recomputeDirty()
				return m, nil
			}
			if m.notifyFocus == 1 {
				m.config.Notify.Dedupe = !m.config.Notify.Dedupe
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

	if m.focusTabs || m.notifyFocus != 2 {
		return m, nil
	}

	var cmd tea.Cmd
	m.notifyThrottle, cmd = m.notifyThrottle.Update(msg)
	if v, err := strconv.Atoi(strings.TrimSpace(m.notifyThrottle.Value())); err == nil {
		m.config.Notify.ThrottleMinutes = v
	}
	m.recomputeDirty()
	return m, cmd
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
			if len(m.machine.Catalogs) == 0 || m.catalogTable.Cursor() <= 0 {
				m.focusTabs = true
				m.catalogTable.Blur()
				return m, nil
			}
		case "down":
			if m.focusTabs {
				m.focusTabs = false
				m.catalogTable.Focus()
				if len(m.machine.Catalogs) == 0 && m.catalogEdit == nil {
					m.startCatalogAddEditor()
				}
				return m, nil
			}
		case " ":
			if !m.focusTabs && len(m.machine.Catalogs) > 0 {
				if err := m.setDefaultFromSelection(); err != nil {
					m.errorText = err.Error()
				} else {
					m.recomputeDirty()
				}
				return m, nil
			}
		case "enter":
			if !m.focusTabs {
				m.advanceStep()
				return m, nil
			}
		}

		if m.focusTabs {
			return m, nil
		}

		switch {
		case key.Matches(keyMsg, m.keys.CatalogAdd):
			m.startCatalogAddEditor()
			return m, nil
		case key.Matches(keyMsg, m.keys.CatalogEdit):
			m.startCatalogEditRootEditor()
			return m, nil
		case key.Matches(keyMsg, m.keys.CatalogDelete):
			if err := m.deleteSelectedCatalog(); err != nil {
				m.errorText = err.Error()
			} else {
				m.recomputeDirty()
			}
			return m, nil
		case key.Matches(keyMsg, m.keys.CatalogDefault):
			if err := m.setDefaultFromSelection(); err != nil {
				m.errorText = err.Error()
			} else {
				m.recomputeDirty()
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.catalogTable, cmd = m.catalogTable.Update(msg)
	return m, cmd
}

func (m *configWizardModel) updateCatalogEditor(msg tea.Msg) (tea.Model, tea.Cmd) {
	editor := m.catalogEdit
	if editor == nil {
		return m, nil
	}
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.catalogEdit = nil
			return m, nil
		case "down":
			editor.focus = (editor.focus + 1) % len(editor.inputs)
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
			m.updateCatalogEditorFocus()
			return m, nil
		case "enter":
			if err := m.commitCatalogEditor(); err != nil {
				editor.err = err.Error()
				return m, nil
			}
			m.catalogEdit = nil
			m.recomputeDirty()
			return m, nil
		}
	}

	cmds := make([]tea.Cmd, len(editor.inputs))
	for i := range editor.inputs {
		editor.inputs[i], cmds[i] = editor.inputs[i].Update(msg)
	}
	editor.err = ""
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
		{Title: "Name", Width: 18},
		{Title: "Root", Width: 56},
		{Title: "Default", Width: 8},
	}
	m.catalogTable = table.New(
		table.WithColumns(cols),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(8),
	)
	styles := table.DefaultStyles()
	styles.Selected = styles.Selected.Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Bold(false)
	m.catalogTable.SetStyles(styles)
	m.rebuildCatalogRows()
}

func (m *configWizardModel) resizeCatalogTable() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	w := m.width - 4
	if w < 30 {
		w = 30
	}
	h := m.height - 14
	if h < 6 {
		h = 6
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

func (m *configWizardModel) updateNotifyFocus() {
	if m.focusTabs {
		m.notifyThrottle.Blur()
		return
	}
	if m.notifyFocus == 2 {
		m.notifyThrottle.Focus()
		return
	}
	m.notifyThrottle.Blur()
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
	m.updateNotifyFocus()
	if m.step == stepCatalogs && !m.focusTabs {
		m.catalogTable.Focus()
		if len(m.machine.Catalogs) == 0 && m.catalogEdit == nil {
			m.startCatalogAddEditor()
		}
	} else {
		m.catalogTable.Blur()
	}
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
	case stepNotify:
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
	labels := []string{"Intro", "GitHub", "Sync", "Notify", "Catalogs", "Review"}
	parts := make([]string, 0, len(labels))
	for i, l := range labels {
		if i == int(m.step) {
			active := focusStyle
			if m.focusTabs {
				active = tabStyle
			}
			parts = append(parts, active.Render("["+l+"]"))
			continue
		}
		parts = append(parts, " "+l+" ")
	}
	return strings.Join(parts, " -> ")
}

func (m *configWizardModel) viewIntro() string {
	mode := "reconfigure"
	if len(m.machine.Catalogs) == 0 || strings.TrimSpace(m.originalConfig.GitHub.Owner) == "" {
		mode = "onboarding"
	}
	return strings.Join([]string{
		"Interactive configuration wizard.",
		"",
		"Mode: " + mode,
		"",
		"Files that may be updated:",
		"- " + m.input.ConfigPath,
		"- " + m.input.MachinePath,
		"",
		hintStyle.Render("Use Right Arrow to continue. Use Esc to go back."),
		hintStyle.Render("Use Up from the first field to focus tabs, then Left/Right to switch steps."),
	}, "\n")
}

func (m *configWizardModel) viewGitHub() string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("GitHub & Transport"))
	b.WriteString("\n\n")
	b.WriteString("State transport mode is fixed to external in v1.\n\n")

	ownerCursor := " "
	if !m.focusTabs && m.githubFocus == 0 {
		ownerCursor = ">"
	}
	b.WriteString(ownerCursor + " GitHub Owner (required)\n")
	b.WriteString(hintStyle.Render("GitHub user or organization that will own new repositories created by bb init."))
	b.WriteString("\n")
	b.WriteString(m.githubOwnerInput.View())
	if m.githubOwnerInput.Err != nil {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.githubOwnerInput.Err.Error()))
	}
	b.WriteString("\n\n")

	visibilityCursor := " "
	if !m.focusTabs && m.githubFocus == 1 {
		visibilityCursor = ">"
	}
	b.WriteString(visibilityCursor + " Default Repository Visibility\n")
	b.WriteString(hintStyle.Render("Default visibility for newly created repositories."))
	b.WriteString("\n")
	b.WriteString(renderEnumLine(m.config.GitHub.DefaultVisibility, []string{"private", "public"}))
	b.WriteString("\n\n")

	protocolCursor := " "
	if !m.focusTabs && m.githubFocus == 2 {
		protocolCursor = ">"
	}
	b.WriteString(protocolCursor + " Git Remote Protocol\n")
	b.WriteString(hintStyle.Render("Remote URL format used when configuring origin."))
	b.WriteString("\n")
	b.WriteString(renderEnumLine(m.config.GitHub.RemoteProtocol, []string{"ssh", "https"}))
	b.WriteString("\n\n")

	b.WriteString(hintStyle.Render("Up/Down switches fields. Space cycles enum options. Enter continues."))
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
	b.WriteString("\n\n")
	for i := range labels {
		cursor := " "
		if !m.focusTabs && i == m.syncCursor {
			cursor = ">"
		}
		mark := "[ ]"
		if values[i] {
			mark = "[x]"
		}
		b.WriteString(fmt.Sprintf("%s %s %s\n", cursor, mark, labels[i]))
		b.WriteString("    " + hintStyle.Render(descriptions[i]) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Up/Down switches fields. Space toggles. Enter continues."))
	return b.String()
}

func (m *configWizardModel) viewNotify() string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Notify Settings"))
	b.WriteString("\n\n")
	enabledCursor := " "
	dedupeCursor := " "
	throttleCursor := " "
	if !m.focusTabs && m.notifyFocus == 0 {
		enabledCursor = ">"
	}
	if !m.focusTabs && m.notifyFocus == 1 {
		dedupeCursor = ">"
	}
	if !m.focusTabs && m.notifyFocus == 2 {
		throttleCursor = ">"
	}
	b.WriteString(fmt.Sprintf("%s [%s] Enable notifications\n", enabledCursor, boolMarker(m.config.Notify.Enabled)))
	b.WriteString("    " + hintStyle.Render("Emits notification output for unsyncable repositories when --notify is used.") + "\n")
	b.WriteString(fmt.Sprintf("%s [%s] Deduplicate notifications\n", dedupeCursor, boolMarker(m.config.Notify.Dedupe)))
	b.WriteString("    " + hintStyle.Render("Suppresses repeated notifications for unchanged unsyncable states.") + "\n")
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%s Notification throttle (minutes)\n", throttleCursor))
	b.WriteString(hintStyle.Render("Minimum minutes between notifications for the same repository."))
	b.WriteString("\n")
	b.WriteString(m.notifyThrottle.View())
	if m.notifyThrottle.Err != nil {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.notifyThrottle.Err.Error()))
	}
	b.WriteString("\n\n")
	b.WriteString(hintStyle.Render("Up/Down switches fields. Space toggles booleans. Enter continues."))
	return b.String()
}

func (m *configWizardModel) viewCatalogs() string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Catalog Management"))
	b.WriteString("\n\n")
	if len(m.machine.Catalogs) == 0 {
		b.WriteString("No catalogs configured yet.\n\n")
		b.WriteString(hintStyle.Render("Start by creating your first catalog below."))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("A catalog is a named root folder where bb discovers repositories."))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("Examples: /Volumes/Projects/Software or /Users/you/Code"))
		b.WriteString("\n")
	} else {
		b.WriteString(m.catalogTable.View())
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("a:add  e:edit root  d:delete  space/s:set default"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("Use Up on first row to focus tabs, then Left/Right to switch steps."))
		b.WriteString("\n")
	}

	if m.catalogEdit != nil {
		b.WriteString("\n")
		if m.catalogEdit.mode == catalogEditorAdd {
			b.WriteString(labelStyle.Render("Add Catalog"))
			b.WriteString("\n")
			b.WriteString("name\n")
			b.WriteString(m.catalogEdit.inputs[0].View())
			b.WriteString("\nroot (absolute path)\n")
			b.WriteString(m.catalogEdit.inputs[1].View())
		} else {
			b.WriteString(labelStyle.Render("Edit Catalog Root"))
			b.WriteString("\n")
			b.WriteString("root (absolute path)\n")
			b.WriteString(m.catalogEdit.inputs[0].View())
		}
		if m.catalogEdit.err != "" {
			b.WriteString("\n")
			b.WriteString(errorStyle.Render(m.catalogEdit.err))
		}
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("down: next field  up: previous field (or tabs from first)  enter: save  esc: cancel"))
	}
	return b.String()
}

func (m *configWizardModel) viewReview() string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Review"))
	b.WriteString("\n\n")
	lines := m.diffLines()
	if len(lines) == 0 {
		b.WriteString("No changes\n")
	} else {
		for _, line := range lines {
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	missing := missingCatalogRoots(m.machine.Catalogs)
	if len(missing) > 0 {
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("Missing catalog roots"))
		b.WriteString("\n")
		for _, root := range missing {
			b.WriteString("- ")
			b.WriteString(root)
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("\ncreate missing roots on apply: [%s] (press Space to toggle)\n", boolMarker(m.createMissingRoots)))
	}
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Press Enter or Ctrl+S to apply changes."))
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
	if m.originalConfig.Sync != m.config.Sync {
		out = append(out, "sync settings updated")
	}
	if m.originalConfig.Notify != m.config.Notify {
		out = append(out, "notify settings updated")
	}
	if !reflect.DeepEqual(m.originalMachine.Catalogs, m.machine.Catalogs) {
		out = append(out, "catalog list updated")
	}
	if m.originalMachine.DefaultCatalog != m.machine.DefaultCatalog {
		out = append(out, fmt.Sprintf("default_catalog: %q -> %q", m.originalMachine.DefaultCatalog, m.machine.DefaultCatalog))
	}
	return out
}

func boolMarker(v bool) string {
	if v {
		return "x"
	}
	return " "
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
		mark := "( )"
		if current == option {
			mark = "(x)"
		}
		parts = append(parts, fmt.Sprintf("%s %s", mark, option))
	}
	return strings.Join(parts, "   ")
}

func (m *configWizardModel) rebuildCatalogRows() {
	rows := make([]table.Row, 0, len(m.machine.Catalogs))
	for _, c := range m.machine.Catalogs {
		def := ""
		if c.Name == m.machine.DefaultCatalog {
			def = "yes"
		}
		rows = append(rows, table.Row{c.Name, c.Root, def})
	}
	m.catalogTable.SetRows(rows)
}

func (m *configWizardModel) startCatalogAddEditor() {
	name := textinput.New()
	name.Prompt = ""
	name.Placeholder = "catalog name"
	name.Focus()
	root := textinput.New()
	root.Prompt = ""
	root.Placeholder = "/absolute/path"
	root.Blur()
	m.catalogEdit = &catalogEditor{
		mode:   catalogEditorAdd,
		inputs: []textinput.Model{name, root},
		focus:  0,
		row:    -1,
	}
}

func (m *configWizardModel) startCatalogEditRootEditor() {
	idx := m.catalogTable.Cursor()
	if idx < 0 || idx >= len(m.machine.Catalogs) {
		m.errorText = "select a catalog first"
		return
	}
	root := textinput.New()
	root.Prompt = ""
	root.Placeholder = "/absolute/path"
	root.SetValue(m.machine.Catalogs[idx].Root)
	root.Focus()
	m.catalogEdit = &catalogEditor{
		mode:   catalogEditorEditRoot,
		inputs: []textinput.Model{root},
		focus:  0,
		row:    idx,
	}
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
		m.machine.Catalogs = append(m.machine.Catalogs, domain.Catalog{Name: name, Root: root})
		if strings.TrimSpace(m.machine.DefaultCatalog) == "" {
			m.machine.DefaultCatalog = name
		}
		m.rebuildCatalogRows()
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
		m.machine.Catalogs[editor.row].Root = root
		m.rebuildCatalogRows()
		return nil
	default:
		return nil
	}
}

func (m *configWizardModel) deleteSelectedCatalog() error {
	idx := m.catalogTable.Cursor()
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
	return nil
}

func (m *configWizardModel) setDefaultFromSelection() error {
	idx := m.catalogTable.Cursor()
	if idx < 0 || idx >= len(m.machine.Catalogs) {
		return fmt.Errorf("select a catalog first")
	}
	m.machine.DefaultCatalog = m.machine.Catalogs[idx].Name
	m.rebuildCatalogRows()
	return nil
}
