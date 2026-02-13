package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type fixTUIKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	Apply    key.Binding
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
	return []key.Binding{k.Up, k.Left, k.Apply, k.Refresh, k.Ignore, k.Quit}
}

func (k fixTUIKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Apply, k.Refresh, k.Ignore, k.Unignore},
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

	table table.Model

	width  int
	height int

	showHelp bool
	status   string
	errText  string

	messagePrompt bool
	messageInput  textinput.Model
	pendingPath   string
}

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
	columns := []table.Column{
		{Title: "Repo", Width: 24},
		{Title: "Branch", Width: 20},
		{Title: "State", Width: 12},
		{Title: "Reasons", Width: 32},
		{Title: "Selected Fix", Width: 22},
	}
	t := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		Foreground(textColor).
		Background(panelBgColor).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		BorderBottom(true).
		Bold(true)
	styles.Cell = styles.Cell.Foreground(textColor)
	styles.Selected = styles.Selected.
		Foreground(textColor).
		Background(accentBgColor).
		Bold(true)
	t.SetStyles(styles)

	m := &fixTUIModel{
		app:             app,
		includeCatalogs: append([]string(nil), includeCatalogs...),
		ignored:         map[string]bool{},
		selectedAction:  map[string]int{},
		keys:            defaultFixTUIKeyMap(),
		help:            help.New(),
		table:           t,
	}
	m.help.ShowAll = false
	if err := m.refreshRepos(); err != nil {
		return nil, err
	}
	return m, nil
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
		m.resizeTable()
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
	m.table, cmd = m.table.Update(msg)
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
	b.WriteString(hintStyle.Render("Select a repository row, choose an action, and apply."))
	b.WriteString("\n\n")
	if len(m.table.Rows()) == 0 {
		b.WriteString(fieldStyle.Render("No repositories available for interactive fix right now."))
	} else {
		tablePanel := panelStyle.
			Border(lipgloss.NormalBorder()).
			BorderForeground(borderColor).
			Background(panelBgColor).
			Padding(0, 1)
		if w := m.viewContentWidth(); w > 0 {
			tablePanel = tablePanel.Width(w - 4)
		}
		b.WriteString(tablePanel.Render(m.table.View()))
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

func (m *fixTUIModel) resizeTable() {
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
	m.table.SetHeight(height)
	if w := m.viewContentWidth(); w > 0 {
		tableWidth := w - 8
		if tableWidth < 60 {
			tableWidth = 60
		}
		m.table.SetWidth(tableWidth)
	}
}

func (m *fixTUIModel) updateMessagePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Cancel) {
		m.messagePrompt = false
		m.pendingPath = ""
		m.status = "stage-commit-push cancelled"
		m.errText = ""
		m.resizeTable()
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
		m.resizeTable()
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
	m.rebuildTable(selectedPath)
	m.errText = ""
	return nil
}

func (m *fixTUIModel) rebuildTable(preferredPath string) {
	m.visible = m.visible[:0]
	rows := make([]table.Row, 0, len(m.repos))
	for _, repo := range m.repos {
		key := repo.Record.Path
		if m.ignored[key] {
			continue
		}
		actions := eligibleFixActions(repo.Record, repo.Meta)
		if len(actions) == 0 {
			delete(m.selectedAction, key)
		} else {
			if idx, ok := m.selectedAction[key]; !ok || idx < 0 || idx >= len(actions) {
				m.selectedAction[key] = 0
			}
		}

		selectedFix := "-"
		if len(actions) > 0 {
			selectedFix = actions[m.selectedAction[key]]
		}
		reasons := "none"
		if len(repo.Record.UnsyncableReasons) > 0 {
			parts := make([]string, 0, len(repo.Record.UnsyncableReasons))
			for _, r := range repo.Record.UnsyncableReasons {
				parts = append(parts, string(r))
			}
			sortStrings(parts)
			reasons = strings.Join(parts, ",")
		}
		state := "syncable"
		if !repo.Record.Syncable {
			state = "unsyncable"
		}

		rows = append(rows, table.Row{
			repo.Record.Name,
			repo.Record.Branch,
			state,
			reasons,
			selectedFix,
		})
		m.visible = append(m.visible, repo)
	}
	m.table.SetRows(rows)
	if len(rows) == 0 {
		m.table.SetCursor(0)
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
	m.table.SetCursor(cursor)
}

func (m *fixTUIModel) currentRepo() (fixRepoState, bool) {
	if len(m.visible) == 0 {
		return fixRepoState{}, false
	}
	cursor := m.table.Cursor()
	if cursor < 0 || cursor >= len(m.visible) {
		return fixRepoState{}, false
	}
	return m.visible[cursor], true
}

func (m *fixTUIModel) cycleCurrentAction(delta int) {
	repo, ok := m.currentRepo()
	if !ok {
		return
	}
	actions := eligibleFixActions(repo.Record, repo.Meta)
	if len(actions) == 0 {
		m.status = "no eligible actions for selected repository"
		return
	}
	key := repo.Record.Path
	idx := m.selectedAction[key]
	idx = (idx + delta) % len(actions)
	if idx < 0 {
		idx += len(actions)
	}
	m.selectedAction[key] = idx
	m.status = fmt.Sprintf("%s selected for %s", actions[idx], repo.Record.Name)
	m.rebuildTable(repo.Record.Path)
}

func (m *fixTUIModel) applyCurrentSelection() {
	repo, ok := m.currentRepo()
	if !ok {
		m.status = "no repository selected"
		return
	}
	actions := eligibleFixActions(repo.Record, repo.Meta)
	if len(actions) == 0 {
		m.status = "no eligible actions for selected repository"
		return
	}
	idx := m.selectedAction[repo.Record.Path]
	if idx < 0 || idx >= len(actions) {
		idx = 0
		m.selectedAction[repo.Record.Path] = idx
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
		m.resizeTable()
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

func (m *fixTUIModel) ignoreCurrentRepo() {
	repo, ok := m.currentRepo()
	if !ok {
		return
	}
	m.ignored[repo.Record.Path] = true
	m.status = fmt.Sprintf("ignored %s for this session", repo.Record.Name)
	m.rebuildTable("")
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
	m.rebuildTable("")
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
