package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type fixTUIKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	Apply    key.Binding
	ApplyAll key.Binding
	Refresh  key.Binding
	Ignore   key.Binding
	Unignore key.Binding
	Help     key.Binding
	Quit     key.Binding
	Cancel   key.Binding
}

func defaultFixTUIKeyMap() fixTUIKeyMap {
	return fixTUIKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "prev repo"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "next repo"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "prev fix"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "next fix"),
		),
		Apply: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "apply selected fix"),
		),
		ApplyAll: key.NewBinding(
			key.WithKeys("ctrl+a"),
			key.WithHelp("ctrl+a", "apply all selected"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh repos"),
		),
		Ignore: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "ignore repo"),
		),
		Unignore: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("u", "unignore repo"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel prompt"),
		),
	}
}

func (k fixTUIKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Left, k.Apply, k.ApplyAll, k.Refresh, k.Quit}
}

func (k fixTUIKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Apply, k.ApplyAll, k.Refresh, k.Ignore, k.Unignore},
		{k.Help, k.Cancel, k.Quit},
	}
}

type fixTUIModel struct {
	app             *App
	includeCatalogs []string
	repos           []fixRepoState
	visible         []fixRepoState
	ignored         map[string]bool
	selectedAction  map[string]int

	keys fixTUIKeyMap
	help help.Model

	repoList list.Model

	width  int
	height int

	status  string
	errText string

	messagePrompt bool
	messageInput  textinput.Model
	pendingPath   string
}

type fixListItem struct {
	Path     string
	Name     string
	Branch   string
	State    string
	Reasons  string
	Action   string
	Syncable bool
}

func (i fixListItem) FilterValue() string {
	return i.Name + " " + i.Path + " " + i.Branch + " " + i.Reasons + " " + i.Action
}

type fixColumnLayout struct {
	Repo    int
	Branch  int
	State   int
	Reasons int
	Action  int
}

type fixRepoDelegate struct{}

func (d fixRepoDelegate) Height() int {
	return 1
}

func (d fixRepoDelegate) Spacing() int {
	return 0
}

func (d fixRepoDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d fixRepoDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	row, ok := item.(fixListItem)
	if !ok {
		return
	}

	// Leave one guard column to avoid terminal auto-wrap when the rendered row
	// exactly matches viewport width.
	contentWidth := m.Width() - 3 // indicator + wrap guard
	if contentWidth < 50 {
		contentWidth = 50
	}
	layout := fixListColumnsForWidth(contentWidth)

	selected := index == m.Index()

	repoCell := renderFixColumnCell(row.Name, layout.Repo, fixRepoNameStyle.Copy().Bold(selected))
	branchCell := renderFixColumnCell(row.Branch, layout.Branch, fixBranchStyle)
	stateStyle := fixStateUnsyncableStyle
	if row.Syncable {
		stateStyle = fixStateSyncableStyle
	}
	if selected {
		stateStyle = stateStyle.Copy().Bold(true)
	}
	stateCell := renderFixColumnCell(row.State, layout.State, stateStyle)
	reasonsCell := renderFixColumnCell(row.Reasons, layout.Reasons, fixReasonsStyle)
	actionStyle := fixActionStyleFor(row.Action)
	if selected {
		actionStyle = actionStyle.Copy().Bold(true)
	}
	actionCell := renderFixColumnCell(row.Action, layout.Action, actionStyle)

	line := lipgloss.JoinHorizontal(lipgloss.Top,
		repoCell,
		fixListColumnGap,
		branchCell,
		fixListColumnGap,
		stateCell,
		fixListColumnGap,
		reasonsCell,
		fixListColumnGap,
		actionCell,
	)

	indicatorStyle := fixIndicatorStyle
	indicator := "  "
	if selected {
		indicatorStyle = fixIndicatorSelectedStyle
		indicator = "▸ "
		line = fixSelectedRowStyle.Render(line)
	}

	fmt.Fprint(w, indicatorStyle.Render(indicator)+line)
}

const fixNoAction = "-"

const (
	fixListDefaultWidth = 120
	fixListReservedCols = 8
)

var (
	fixNoActionStyle = lipgloss.NewStyle().
				Foreground(mutedTextColor)

	fixRepoNameStyle = lipgloss.NewStyle().
				Foreground(textColor)

	fixBranchStyle = lipgloss.NewStyle().
			Foreground(mutedTextColor)

	fixReasonsStyle = lipgloss.NewStyle().
			Foreground(mutedTextColor)

	fixStateSyncableStyle = lipgloss.NewStyle().
				Foreground(successColor)

	fixStateUnsyncableStyle = lipgloss.NewStyle().
				Foreground(errorFgColor)

	fixIndicatorStyle = lipgloss.NewStyle().
				Foreground(borderColor)

	fixIndicatorSelectedStyle = lipgloss.NewStyle().
					Foreground(accentColor).
					Bold(true)

	fixSelectedRowStyle = lipgloss.NewStyle().
				Background(accentBgColor)

	fixHeaderCellStyle = lipgloss.NewStyle().
				Foreground(textColor).
				Bold(true)

	fixActionPushStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("45"))

	fixActionStageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214"))

	fixActionPullStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("81"))

	fixActionUpstreamStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("177"))

	fixActionAutoPushStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("70"))

	fixActionAbortStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("203"))
)

const fixListColumnGap = "  "

func (a *App) runFixInteractive(includeCatalogs []string) (int, error) {
	model, err := newFixTUIModel(a, includeCatalogs)
	if err != nil {
		return 2, err
	}
	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return 2, err
	}
	return 0, nil
}

func newFixTUIModel(app *App, includeCatalogs []string) (*fixTUIModel, error) {
	repoList := newFixRepoListModel()

	m := &fixTUIModel{
		app:             app,
		includeCatalogs: append([]string(nil), includeCatalogs...),
		ignored:         map[string]bool{},
		selectedAction:  map[string]int{},
		keys:            defaultFixTUIKeyMap(),
		help:            help.New(),
		repoList:        repoList,
	}
	m.help.ShowAll = false
	if err := m.refreshRepos(); err != nil {
		return nil, err
	}
	return m, nil
}

func newFixRepoListModel() list.Model {
	delegate := fixRepoDelegate{}
	m := list.New([]list.Item{}, delegate, fixListDefaultWidth, 12)
	m.SetFilteringEnabled(false)
	m.SetShowFilter(false)
	m.SetShowTitle(false)
	m.SetShowStatusBar(false)
	m.SetShowPagination(false)
	m.SetShowHelp(false)
	m.DisableQuitKeybindings()
	m.KeyMap.PrevPage.SetEnabled(false)
	m.KeyMap.NextPage.SetEnabled(false)
	m.KeyMap.Filter.SetEnabled(false)
	m.KeyMap.ClearFilter.SetEnabled(false)
	m.KeyMap.ShowFullHelp.SetEnabled(false)
	m.KeyMap.CloseFullHelp.SetEnabled(false)
	m.KeyMap.GoToStart.SetEnabled(false)
	m.KeyMap.GoToEnd.SetEnabled(false)

	styles := list.DefaultStyles()
	styles.NoItems = hintStyle
	m.Styles = styles
	return m
}

func (m *fixTUIModel) Init() tea.Cmd {
	return nil
}

func (m *fixTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.resizeRepoList()
		return m, nil
	case tea.KeyMsg:
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
		if key.Matches(msg, m.keys.Help) {
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		}
		if m.messagePrompt {
			return m.updateMessagePrompt(msg)
		}
		switch {
		case key.Matches(msg, m.keys.Left):
			m.cycleCurrentAction(-1)
			return m, nil
		case key.Matches(msg, m.keys.Right):
			m.cycleCurrentAction(1)
			return m, nil
		case key.Matches(msg, m.keys.Apply):
			m.applyCurrentSelection()
			return m, nil
		case key.Matches(msg, m.keys.ApplyAll):
			m.applyAllSelections()
			return m, nil
		case key.Matches(msg, m.keys.Refresh):
			if err := m.refreshRepos(); err != nil {
				m.errText = err.Error()
			}
			return m, nil
		case key.Matches(msg, m.keys.Ignore):
			m.ignoreCurrentRepo()
			return m, nil
		case key.Matches(msg, m.keys.Unignore):
			m.unignoreCurrentRepo()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.repoList, cmd = m.repoList.Update(msg)
	return m, cmd
}

func (m *fixTUIModel) View() string {
	var b strings.Builder
	title := lipgloss.JoinHorizontal(lipgloss.Center,
		titleBadgeStyle.Render("bb"),
		" "+headerStyle.Render("fix"),
	)
	subtitle := hintStyle.Render("Interactive remediation for unsyncable repositories")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(subtitle)
	b.WriteString("\n\n")

	contentPanel := panelStyle
	if w := m.viewContentWidth(); w > 0 {
		contentPanel = contentPanel.Width(w)
	}
	b.WriteString(contentPanel.Render(m.viewMainContent()))

	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(hintStyle.Render(m.status))
	}
	if m.errText != "" {
		b.WriteString("\n")
		b.WriteString(alertStyle.Render(errorStyle.Render(m.errText)))
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

func (m *fixTUIModel) viewMainContent() string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Repository Fixes"))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Select per-repo fixes. Default selection is '-' (no action)."))
	b.WriteString("\n\n")
	b.WriteString(m.viewFixSummary())
	b.WriteString("\n\n")

	if len(m.visible) == 0 {
		b.WriteString(fieldStyle.Render("No repositories available for interactive fix right now."))
	} else {
		// Render list directly inside the main panel to avoid nested border/frame
		// width interactions that can trigger hard wrapping artifacts.
		b.WriteString(m.viewRepoList())
		if details := m.viewSelectedRepoDetails(); details != "" {
			b.WriteString("\n\n")
			b.WriteString(details)
		}
	}

	if m.messagePrompt {
		b.WriteString("\n\n")
		b.WriteString(renderFieldBlock(
			true,
			"Commit message",
			"Used for stage-commit-push. Leave default value to auto-generate.",
			renderInputContainer(m.messageInput.View(), true),
			"",
		))
	}
	return b.String()
}

func (m *fixTUIModel) viewRepoList() string {
	if len(m.visible) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.viewRepoHeader())
	b.WriteString("\n")
	b.WriteString(m.repoList.View())
	return b.String()
}

func (m *fixTUIModel) viewRepoHeader() string {
	listWidth := m.repoList.Width()
	if listWidth <= 0 {
		listWidth = fixListDefaultWidth
	}
	layout := fixListColumnsForWidth(max(50, listWidth-3))

	header := lipgloss.JoinHorizontal(lipgloss.Top,
		renderFixColumnCell("Repo", layout.Repo, fixHeaderCellStyle),
		fixListColumnGap,
		renderFixColumnCell("Branch", layout.Branch, fixHeaderCellStyle),
		fixListColumnGap,
		renderFixColumnCell("State", layout.State, fixHeaderCellStyle),
		fixListColumnGap,
		renderFixColumnCell("Reasons", layout.Reasons, fixHeaderCellStyle),
		fixListColumnGap,
		renderFixColumnCell("Selected Fix", layout.Action, fixHeaderCellStyle),
	)
	// Match row rendering width: two-char indicator plus one guard column.
	return "  " + header
}

func (m *fixTUIModel) viewSelectedRepoDetails() string {
	repo, ok := m.currentRepo()
	if !ok {
		return ""
	}
	actions := eligibleFixActions(repo.Record, repo.Meta)
	action := m.currentActionForRepo(repo.Record.Path, actions)

	reasonText := "none"
	if len(repo.Record.UnsyncableReasons) > 0 {
		parts := make([]string, 0, len(repo.Record.UnsyncableReasons))
		for _, r := range repo.Record.UnsyncableReasons {
			parts = append(parts, string(r))
		}
		sortStrings(parts)
		reasonText = strings.Join(parts, ", ")
	}

	state := "syncable"
	stateStyle := fixStateSyncableStyle
	if !repo.Record.Syncable {
		state = "unsyncable"
		stateStyle = fixStateUnsyncableStyle
	}

	var b strings.Builder
	b.WriteString(fieldStyle.Render(fmt.Sprintf("Selected: %s (%s)", repo.Record.Name, repo.Record.Path)))
	b.WriteString("\n")
	stateLabel := lipgloss.NewStyle().Foreground(mutedTextColor).Render("State:")
	actionLabel := lipgloss.NewStyle().Foreground(mutedTextColor).Render("Action:")
	branchLabel := lipgloss.NewStyle().Foreground(mutedTextColor).Render("Branch:")
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		stateLabel,
		" ",
		stateStyle.Render(state),
		"   ",
		actionLabel,
		" ",
		fixActionStyleFor(action).Render(action),
		"   ",
		branchLabel,
		" ",
		fixBranchStyle.Render(repo.Record.Branch),
	))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(mutedTextColor).Render("Reasons: ") + fixReasonsStyle.Render(reasonText))
	return b.String()
}

func (m *fixTUIModel) viewFixSummary() string {
	total := len(m.visible)
	pending := 0
	for _, repo := range m.visible {
		if m.selectedAction[repo.Record.Path] >= 0 {
			pending++
		}
	}
	totalStyle := renderStatusPill(fmt.Sprintf("%d repos", total))
	pendingStyle := renderStatusPill(fmt.Sprintf("%d selected", pending))
	noneStyle := fixNoActionStyle.Render("'-' means no action")
	return lipgloss.JoinHorizontal(lipgloss.Top, totalStyle, " ", pendingStyle, "  ", noneStyle)
}

func (m *fixTUIModel) viewContentWidth() int {
	if m.width <= 0 {
		return 0
	}
	contentWidth := m.width - 8
	if contentWidth < 52 {
		return 0
	}
	return contentWidth
}

func (m *fixTUIModel) resizeRepoList() {
	if m.height <= 0 {
		return
	}
	reserved := 18
	if m.messagePrompt {
		reserved += 5
	}
	height := m.height - reserved
	if height < 6 {
		height = 6
	}

	listWidth := fixListDefaultWidth
	if w := m.viewContentWidth(); w > 0 {
		listWidth = w - 10
	}
	if listWidth < 52 {
		listWidth = 52
	}
	m.repoList.SetSize(listWidth, height)
}

func (m *fixTUIModel) updateMessagePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Cancel) {
		m.messagePrompt = false
		m.pendingPath = ""
		m.status = "stage-commit-push cancelled"
		m.errText = ""
		m.resizeRepoList()
		return m, nil
	}
	if key.Matches(msg, m.keys.Apply) {
		raw := strings.TrimSpace(m.messageInput.Value())
		commitMessage := raw
		if raw == "" || raw == AutoFixCommitMessage {
			commitMessage = "auto"
		}
		if _, err := m.app.applyFixAction(m.includeCatalogs, m.pendingPath, FixActionStageCommitPush, commitMessage); err != nil {
			m.errText = err.Error()
		} else {
			m.status = fmt.Sprintf("applied %s", FixActionStageCommitPush)
			m.errText = ""
		}
		m.messagePrompt = false
		m.pendingPath = ""
		if err := m.refreshRepos(); err != nil {
			m.errText = err.Error()
		}
		m.resizeRepoList()
		return m, nil
	}

	var cmd tea.Cmd
	m.messageInput, cmd = m.messageInput.Update(msg)
	return m, cmd
}

func (m *fixTUIModel) refreshRepos() error {
	selectedPath := ""
	if current, ok := m.currentRepo(); ok {
		selectedPath = current.Record.Path
	}

	repos, err := m.app.loadFixRepos(m.includeCatalogs)
	if err != nil {
		return err
	}
	m.repos = repos
	m.rebuildList(selectedPath)
	m.errText = ""
	return nil
}

func (m *fixTUIModel) rebuildList(preferredPath string) {
	m.visible = m.visible[:0]
	items := make([]list.Item, 0, len(m.repos))

	for _, repo := range m.repos {
		path := repo.Record.Path
		if m.ignored[path] {
			continue
		}

		actions := eligibleFixActions(repo.Record, repo.Meta)
		if idx, ok := m.selectedAction[path]; !ok || idx < -1 || idx >= len(actions) {
			m.selectedAction[path] = -1
		}

		reasons := "none"
		if len(repo.Record.UnsyncableReasons) > 0 {
			parts := make([]string, 0, len(repo.Record.UnsyncableReasons))
			for _, r := range repo.Record.UnsyncableReasons {
				parts = append(parts, string(r))
			}
			sortStrings(parts)
			reasons = strings.Join(parts, ", ")
		}
		state := "syncable"
		if !repo.Record.Syncable {
			state = "unsyncable"
		}

		items = append(items, fixListItem{
			Path:     path,
			Name:     repo.Record.Name,
			Branch:   repo.Record.Branch,
			State:    state,
			Reasons:  reasons,
			Action:   m.currentActionForRepo(path, actions),
			Syncable: repo.Record.Syncable,
		})
		m.visible = append(m.visible, repo)
	}

	_ = m.repoList.SetItems(items)
	if len(items) == 0 {
		m.repoList.ResetSelected()
		return
	}

	cursor := 0
	if preferredPath != "" {
		for i, repo := range m.visible {
			if repo.Record.Path == preferredPath {
				cursor = i
				break
			}
		}
	}
	m.setCursor(cursor)
}

func (m *fixTUIModel) setCursor(target int) {
	if len(m.visible) == 0 {
		m.repoList.ResetSelected()
		return
	}
	if target < 0 {
		target = 0
	}
	if target > len(m.visible)-1 {
		target = len(m.visible) - 1
	}
	m.repoList.Select(target)
}

func (m *fixTUIModel) currentRepo() (fixRepoState, bool) {
	if len(m.visible) == 0 {
		return fixRepoState{}, false
	}
	idx := m.repoList.Index()
	if idx < 0 || idx >= len(m.visible) {
		return fixRepoState{}, false
	}
	return m.visible[idx], true
}

func (m *fixTUIModel) cycleCurrentAction(delta int) {
	repo, ok := m.currentRepo()
	if !ok {
		return
	}
	actions := eligibleFixActions(repo.Record, repo.Meta)
	key := repo.Record.Path
	optionCount := len(actions) + 1 // include "-" no-op option
	pos := m.selectedAction[key] + 1
	pos = (pos + delta) % optionCount
	if pos < 0 {
		pos += optionCount
	}
	m.selectedAction[key] = pos - 1
	selected := m.currentActionForRepo(key, actions)
	if selected == fixNoAction {
		m.status = fmt.Sprintf("no action selected for %s", repo.Record.Name)
	} else {
		m.status = fmt.Sprintf("%s selected for %s", selected, repo.Record.Name)
	}
	m.rebuildList(repo.Record.Path)
}

func (m *fixTUIModel) applyCurrentSelection() {
	repo, ok := m.currentRepo()
	if !ok {
		m.status = "no repository selected"
		return
	}
	actions := eligibleFixActions(repo.Record, repo.Meta)
	idx := m.selectedAction[repo.Record.Path]
	if idx < 0 {
		m.status = fmt.Sprintf("no action selected for %s", repo.Record.Name)
		return
	}
	if idx >= len(actions) {
		m.selectedAction[repo.Record.Path] = -1
		m.status = fmt.Sprintf("selection reset for %s; pick an eligible action", repo.Record.Name)
		m.rebuildList(repo.Record.Path)
		return
	}
	action := actions[idx]

	if action == FixActionStageCommitPush {
		ti := textinput.New()
		ti.Placeholder = "commit message"
		ti.SetValue(AutoFixCommitMessage)
		ti.Focus()
		m.messagePrompt = true
		m.messageInput = ti
		m.pendingPath = repo.Record.Path
		m.status = ""
		m.errText = ""
		m.resizeRepoList()
		return
	}

	if _, err := m.app.applyFixAction(m.includeCatalogs, repo.Record.Path, action, ""); err != nil {
		m.errText = err.Error()
		return
	}
	m.status = fmt.Sprintf("applied %s to %s", action, repo.Record.Name)
	m.errText = ""
	if err := m.refreshRepos(); err != nil {
		m.errText = err.Error()
	}
}

func (m *fixTUIModel) applyAllSelections() {
	if len(m.visible) == 0 {
		m.status = "no repositories available"
		return
	}

	applied := 0
	skipped := 0
	failures := make([]string, 0, 3)

	for _, repo := range m.visible {
		actions := eligibleFixActions(repo.Record, repo.Meta)
		idx := m.selectedAction[repo.Record.Path]
		if idx < 0 {
			skipped++
			continue
		}
		if idx >= len(actions) {
			skipped++
			m.selectedAction[repo.Record.Path] = -1
			continue
		}

		action := actions[idx]
		commitMessage := ""
		if action == FixActionStageCommitPush {
			commitMessage = "auto"
		}
		if _, err := m.app.applyFixAction(m.includeCatalogs, repo.Record.Path, action, commitMessage); err != nil {
			failures = append(failures, fmt.Sprintf("%s (%s)", repo.Record.Name, action))
			continue
		}
		applied++
	}

	if m.app != nil {
		if err := m.refreshRepos(); err != nil {
			m.errText = err.Error()
			return
		}
	} else if applied > 0 || len(failures) > 0 {
		m.errText = "internal: app is not configured for apply-all"
		return
	}

	if len(failures) > 0 {
		m.errText = fmt.Sprintf("failed: %s", strings.Join(failures, ", "))
	} else {
		m.errText = ""
	}
	m.status = fmt.Sprintf("applied %d, skipped %d, failed %d", applied, skipped, len(failures))
}

func (m *fixTUIModel) ignoreCurrentRepo() {
	repo, ok := m.currentRepo()
	if !ok {
		return
	}
	m.ignored[repo.Record.Path] = true
	m.status = fmt.Sprintf("ignored %s for this session", repo.Record.Name)
	m.rebuildList("")
}

func (m *fixTUIModel) unignoreCurrentRepo() {
	repo, ok := m.currentRepo()
	if ok {
		delete(m.ignored, repo.Record.Path)
	}
	if len(m.ignored) == 0 {
		m.status = "no ignored repositories"
		return
	}
	for path := range m.ignored {
		delete(m.ignored, path)
	}
	m.status = "cleared ignored repositories"
	m.rebuildList("")
}

func (m *fixTUIModel) currentActionForRepo(path string, actions []string) string {
	idx := m.selectedAction[path]
	if idx < 0 || idx >= len(actions) {
		return fixNoAction
	}
	return actions[idx]
}

func fixListColumnsForWidth(listWidth int) fixColumnLayout {
	const (
		repoMin    = 7
		branchMin  = 7
		stateMin   = 6
		reasonsMin = 12
		actionMin  = 10
	)

	layout := fixColumnLayout{
		Repo:    repoMin,
		Branch:  branchMin,
		State:   stateMin,
		Reasons: reasonsMin,
		Action:  actionMin,
	}

	minTotal := repoMin + branchMin + stateMin + reasonsMin + actionMin
	budget := listWidth - fixListReservedCols
	if budget < minTotal {
		budget = minTotal
	}
	extra := budget - minTotal
	if extra > 0 {
		repoExtra := extra * 18 / 100
		branchExtra := extra * 16 / 100
		stateExtra := extra * 10 / 100
		reasonsExtra := extra * 32 / 100
		actionExtra := extra - repoExtra - branchExtra - stateExtra - reasonsExtra
		layout.Repo += repoExtra
		layout.Branch += branchExtra
		layout.State += stateExtra
		layout.Reasons += reasonsExtra
		layout.Action += actionExtra
	}
	return layout
}

func renderFixColumnCell(value string, width int, style lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	return style.Width(width).MaxWidth(width).Render(ansi.Truncate(value, width, "…"))
}

func fixActionStyleFor(action string) lipgloss.Style {
	switch action {
	case fixNoAction:
		return fixNoActionStyle
	case FixActionPush:
		return fixActionPushStyle
	case FixActionStageCommitPush:
		return fixActionStageStyle
	case FixActionPullFFOnly:
		return fixActionPullStyle
	case FixActionSetUpstreamPush:
		return fixActionUpstreamStyle
	case FixActionEnableAutoPush:
		return fixActionAutoPushStyle
	case FixActionAbortOperation:
		return fixActionAbortStyle
	default:
		return lipgloss.NewStyle().Foreground(textColor)
	}
}

func sortStrings(in []string) {
	for i := 1; i < len(in); i++ {
		j := i
		for j > 0 && in[j] < in[j-1] {
			in[j], in[j-1] = in[j-1], in[j]
			j--
		}
	}
}
