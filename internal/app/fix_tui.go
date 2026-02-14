package app

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"bb-project/internal/domain"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
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
	Skip     key.Binding
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
		Skip: key.NewBinding(
			key.WithKeys("ctrl+x"),
			key.WithHelp("ctrl+x", "skip in wizard"),
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
	return []key.Binding{k.Up, k.Left, k.Apply, k.Skip, k.ApplyAll, k.Quit}
}

func (k fixTUIKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Apply, k.Skip, k.ApplyAll, k.Refresh, k.Ignore, k.Unignore},
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

	viewMode fixViewMode
	wizard   fixWizardState

	summaryResults []fixSummaryResult
}

type fixListItem struct {
	Path      string
	Name      string
	Branch    string
	State     string
	Reasons   string
	Action    string
	ActionKey string
	Tier      fixRepoTier
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

type fixRepoTier int

const (
	fixRepoTierAutofixable fixRepoTier = iota
	fixRepoTierUnsyncableBlocked
	fixRepoTierSyncable
)

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
	stateStyle := fixStateSyncableStyle
	switch row.Tier {
	case fixRepoTierAutofixable:
		stateStyle = fixStateAutofixableStyle
	case fixRepoTierUnsyncableBlocked:
		stateStyle = fixStateUnsyncableStyle
	}
	if selected {
		stateStyle = stateStyle.Copy().Bold(true)
	}
	stateCell := renderFixColumnCell(row.State, layout.State, stateStyle)
	reasonsCell := renderFixColumnCell(row.Reasons, layout.Reasons, fixReasonsStyle)
	actionStyle := fixActionStyleFor(row.ActionKey)
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
const fixAllActions = "__all__"

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

	fixStateAutofixableStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("214"))

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

	fixDetailsLabelStyle = lipgloss.NewStyle().
				Foreground(accentColor).
				Bold(true)

	fixDetailsValueStyle = lipgloss.NewStyle().
				Foreground(textColor).
				Bold(true)

	fixDetailsPathStyle = lipgloss.NewStyle().
				Foreground(mutedTextColor)

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

	fixPageStyle = lipgloss.NewStyle().Padding(0, 2)
)

const fixListColumnGap = "  "

func (a *App) runFixInteractive(includeCatalogs []string, noRefresh bool) (int, error) {
	model, err := newFixTUIModel(a, includeCatalogs, noRefresh)
	if err != nil {
		return 2, err
	}
	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return 2, err
	}
	return 0, nil
}

func newFixTUIModel(app *App, includeCatalogs []string, noRefresh bool) (*fixTUIModel, error) {
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
	if err := m.refreshRepos(!noRefresh); err != nil {
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
		if m.viewMode == fixViewWizard {
			return m.updateWizard(msg)
		}
		if m.viewMode == fixViewSummary {
			return m.updateSummary(msg)
		}
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
		if key.Matches(msg, m.keys.Help) {
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
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
			if err := m.refreshRepos(true); err != nil {
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
	m.resizeRepoList()

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
	content := m.viewMainContent()
	switch m.viewMode {
	case fixViewWizard:
		content = m.viewWizardContent()
	case fixViewSummary:
		content = m.viewSummaryContent()
	}
	b.WriteString(contentPanel.Render(content))

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
		available := m.height - fixPageStyle.GetVerticalFrameSize()
		if available < 0 {
			available = 0
		}
		const separatorLines = 1
		used := lipgloss.Height(body) + separatorLines + lipgloss.Height(helpBlock)
		if gap := available - used; gap > 0 {
			spacer = strings.Repeat("\n", gap)
		}
	}

	doc := body + "\n" + spacer + helpBlock
	return fixPageStyle.Render(doc)
}

func (m *fixTUIModel) viewMainContent() string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Repository Fixes"))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Select per-repo fixes. Default selection is '-' (no action)."))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("List order: fixable unsyncable, unsyncable manual, then syncable."))
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
	actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
		Interactive: true,
		Risk:        repo.Risk,
	})
	action := m.currentActionForRepo(repo.Record.Path, actions)
	actionLabelValue := fixActionLabel(action)

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
	tier := classifyFixRepo(repo, actions)
	switch tier {
	case fixRepoTierAutofixable:
		state = "unsyncable (fixable)"
		stateStyle = fixStateAutofixableStyle
	case fixRepoTierUnsyncableBlocked:
		state = "unsyncable (manual)"
		stateStyle = fixStateUnsyncableStyle
	}

	var b strings.Builder
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		fixDetailsLabelStyle.Render("Selected:"),
		" ",
		fixDetailsValueStyle.Render(repo.Record.Name),
		" ",
		fixDetailsPathStyle.Render("("+repo.Record.Path+")"),
	))
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
		fixActionStyleFor(action).Render(actionLabelValue),
		"   ",
		branchLabel,
		" ",
		fixBranchStyle.Render(repo.Record.Branch),
	))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(mutedTextColor).Render("Reasons: ") + fixReasonsStyle.Render(reasonText))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(mutedTextColor).Render("Action help: ") + hintStyle.Render(fixActionDescription(action)))
	return b.String()
}

func (m *fixTUIModel) viewFixSummary() string {
	total := len(m.visible)
	pending := 0
	fixable := 0
	blocked := 0
	syncable := 0
	for _, repo := range m.visible {
		if m.selectedAction[repo.Record.Path] >= 0 {
			pending++
		}
		tier := classifyFixRepo(repo, eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
			Interactive: true,
			Risk:        repo.Risk,
		}))
		switch tier {
		case fixRepoTierAutofixable:
			fixable++
		case fixRepoTierUnsyncableBlocked:
			blocked++
		case fixRepoTierSyncable:
			syncable++
		}
	}
	totalStyle := renderStatusPill(fmt.Sprintf("%d repos", total))
	pendingStyle := renderStatusPill(fmt.Sprintf("%d selected", pending))
	autoStyle := renderFixSummaryPill(fmt.Sprintf("%d fixable", fixable), lipgloss.Color("214"))
	blockedStyle := renderFixSummaryPill(fmt.Sprintf("%d unsyncable manual", blocked), errorFgColor)
	syncStyle := renderFixSummaryPill(fmt.Sprintf("%d syncable", syncable), successColor)
	return lipgloss.JoinHorizontal(lipgloss.Top, totalStyle, " ", pendingStyle, "  ", autoStyle, " ", blockedStyle, " ", syncStyle)
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
	helpPanel := helpPanelStyle
	if w := m.viewContentWidth(); w > 0 {
		helpPanel = helpPanel.Width(w)
	}
	helpHeight := lipgloss.Height(helpPanel.Render(m.help.View(m.keys)))

	reserved := 0
	reserved += fixPageStyle.GetVerticalFrameSize()
	reserved += 3  // title + subtitle + one spacer line
	reserved += 14 // main panel border and non-list content
	reserved += 1  // single spacer line between body and footer help
	reserved += helpHeight
	if m.status != "" {
		reserved++
	}
	if m.errText != "" {
		reserved++
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

func (m *fixTUIModel) refreshRepos(refresh bool) error {
	selectedPath := ""
	if current, ok := m.currentRepo(); ok {
		selectedPath = current.Record.Path
	}

	repos, err := m.app.loadFixRepos(m.includeCatalogs, refresh)
	if err != nil {
		return err
	}
	m.repos = repos
	m.rebuildList(selectedPath)
	m.errText = ""
	return nil
}

func (m *fixTUIModel) rebuildList(preferredPath string) {
	type fixVisibleEntry struct {
		repo    fixRepoState
		actions []string
		tier    fixRepoTier
	}

	m.visible = m.visible[:0]
	items := make([]list.Item, 0, len(m.repos))
	entries := make([]fixVisibleEntry, 0, len(m.repos))

	for _, repo := range m.repos {
		path := repo.Record.Path
		if m.ignored[path] {
			continue
		}

		actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
			Interactive: true,
			Risk:        repo.Risk,
		})
		options := selectableFixActions(actions)
		if idx, ok := m.selectedAction[path]; !ok || idx < -1 || idx >= len(options) {
			m.selectedAction[path] = -1
		}
		entries = append(entries, fixVisibleEntry{
			repo:    repo,
			actions: actions,
			tier:    classifyFixRepo(repo, actions),
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].tier != entries[j].tier {
			return entries[i].tier < entries[j].tier
		}
		leftName := strings.ToLower(entries[i].repo.Record.Name)
		rightName := strings.ToLower(entries[j].repo.Record.Name)
		if leftName == rightName {
			return entries[i].repo.Record.Path < entries[j].repo.Record.Path
		}
		return leftName < rightName
	})

	for _, entry := range entries {
		repo := entry.repo
		path := repo.Record.Path
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
		switch entry.tier {
		case fixRepoTierAutofixable:
			state = "unsyncable (fixable)"
		case fixRepoTierUnsyncableBlocked:
			state = "unsyncable (manual)"
		}

		selected := m.currentActionForRepo(path, entry.actions)
		items = append(items, fixListItem{
			Path:      path,
			Name:      repo.Record.Name,
			Branch:    repo.Record.Branch,
			State:     state,
			Reasons:   reasons,
			Action:    fixActionLabel(selected),
			ActionKey: selected,
			Tier:      entry.tier,
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
	actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
		Interactive: true,
		Risk:        repo.Risk,
	})
	options := selectableFixActions(actions)
	key := repo.Record.Path
	optionCount := len(options) + 1 // include "-" no-op option
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
		m.status = fmt.Sprintf("%s selected for %s (%s)", fixActionLabel(selected), repo.Record.Name, fixActionDescription(selected))
	}
	m.rebuildList(repo.Record.Path)
}

func (m *fixTUIModel) applyCurrentSelection() {
	repo, ok := m.currentRepo()
	if !ok {
		m.status = "no repository selected"
		return
	}
	actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
		Interactive: true,
		Risk:        repo.Risk,
	})
	options := selectableFixActions(actions)
	idx := m.selectedAction[repo.Record.Path]
	if idx < 0 {
		m.status = fmt.Sprintf("no action selected for %s", repo.Record.Name)
		return
	}
	if idx >= len(options) {
		m.selectedAction[repo.Record.Path] = -1
		m.status = fmt.Sprintf("selection reset for %s; pick an eligible action", repo.Record.Name)
		m.rebuildList(repo.Record.Path)
		return
	}
	action := options[idx]
	m.summaryResults = nil

	queue := make([]fixWizardDecision, 0, 2)
	selections := []string{action}
	if action == fixAllActions {
		selections = actions
	}

	applied := 0
	failed := 0
	for _, selection := range selections {
		if isRiskyFixAction(selection) {
			queue = append(queue, fixWizardDecision{
				RepoPath: repo.Record.Path,
				Action:   selection,
			})
			continue
		}
		if err := m.applyImmediateAction(repo, selection); err != nil {
			m.summaryResults = append(m.summaryResults, fixSummaryResult{
				RepoName: repo.Record.Name,
				Action:   fixActionLabel(selection),
				Status:   "failed",
				Detail:   err.Error(),
			})
			failed++
			continue
		}
		m.summaryResults = append(m.summaryResults, fixSummaryResult{
			RepoName: repo.Record.Name,
			Action:   fixActionLabel(selection),
			Status:   "applied",
		})
		applied++
	}

	if len(queue) > 0 {
		m.startWizardQueue(queue)
		return
	}
	if m.app != nil && applied > 0 {
		if err := m.refreshRepos(true); err != nil {
			m.errText = err.Error()
			return
		}
	}
	if failed > 0 {
		m.errText = "one or more fixes failed"
	} else {
		m.errText = ""
	}
	m.status = fmt.Sprintf("applied %d, skipped %d, failed %d", applied, 0, failed)
}

func (m *fixTUIModel) applyAllSelections() {
	if len(m.visible) == 0 {
		m.status = "no repositories available"
		return
	}

	m.summaryResults = nil
	applied := 0
	skipped := 0
	failures := make([]string, 0, 3)
	queue := make([]fixWizardDecision, 0, 8)

	for _, repo := range m.visible {
		actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
			Interactive: true,
			Risk:        repo.Risk,
		})
		options := selectableFixActions(actions)
		idx := m.selectedAction[repo.Record.Path]
		if idx < 0 {
			skipped++
			continue
		}
		if idx >= len(options) {
			skipped++
			m.selectedAction[repo.Record.Path] = -1
			continue
		}

		selection := options[idx]
		selections := []string{selection}
		if selection == fixAllActions {
			selections = actions
		}
		for _, selected := range selections {
			if isRiskyFixAction(selected) {
				queue = append(queue, fixWizardDecision{
					RepoPath: repo.Record.Path,
					Action:   selected,
				})
				continue
			}
			if err := m.applyImmediateAction(repo, selected); err != nil {
				failures = append(failures, fmt.Sprintf("%s (%s)", repo.Record.Name, fixActionLabel(selected)))
				m.summaryResults = append(m.summaryResults, fixSummaryResult{
					RepoName: repo.Record.Name,
					Action:   fixActionLabel(selected),
					Status:   "failed",
					Detail:   err.Error(),
				})
				continue
			}
			m.summaryResults = append(m.summaryResults, fixSummaryResult{
				RepoName: repo.Record.Name,
				Action:   fixActionLabel(selected),
				Status:   "applied",
			})
			applied++
		}
	}

	if m.app != nil {
		if err := m.refreshRepos(true); err != nil {
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
	if len(queue) > 0 {
		m.startWizardQueue(queue)
		return
	}
	m.status = fmt.Sprintf("applied %d, skipped %d, failed %d", applied, skipped, len(failures))
}

func (m *fixTUIModel) applyImmediateAction(repo fixRepoState, action string) error {
	if m.app == nil {
		return errors.New("internal: app is not configured")
	}
	_, err := m.app.applyFixAction(m.includeCatalogs, repo.Record.Path, action, fixApplyOptions{
		Interactive: true,
	})
	return err
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
	options := selectableFixActions(actions)
	idx := m.selectedAction[path]
	if idx < 0 || idx >= len(options) {
		return fixNoAction
	}
	return options[idx]
}

func selectableFixActions(actions []string) []string {
	if len(actions) <= 1 {
		return actions
	}
	out := make([]string, 0, len(actions)+1)
	out = append(out, actions...)
	out = append(out, fixAllActions)
	return out
}

func fixListColumnsForWidth(listWidth int) fixColumnLayout {
	const (
		repoMin    = 7
		branchMin  = 7
		stateMin   = 18
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
		repoExtra := extra * 16 / 100
		branchExtra := extra * 14 / 100
		stateExtra := extra * 10 / 100
		reasonsExtra := extra * 30 / 100
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

func renderFixSummaryPill(value string, fg lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().
		Foreground(fg).
		Background(accentBgColor).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Padding(0, 1).
		Bold(true).
		Render(strings.ToUpper(value))
}

func fixActionStyleFor(action string) lipgloss.Style {
	switch action {
	case fixNoAction:
		return fixNoActionStyle
	case fixAllActions:
		return fixActionAutoPushStyle.Copy().Bold(true)
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

func fixActionLabel(action string) string {
	switch action {
	case fixNoAction:
		return fixNoAction
	case fixAllActions:
		return "All fixes"
	case FixActionAbortOperation:
		return "Abort operation"
	case FixActionPush:
		return "Push commits"
	case FixActionStageCommitPush:
		return "Stage, commit & push"
	case FixActionPullFFOnly:
		return "Pull (ff-only)"
	case FixActionSetUpstreamPush:
		return "Set upstream & push"
	case FixActionEnableAutoPush:
		return "Allow auto-push in sync"
	default:
		return action
	}
}

func fixActionDescription(action string) string {
	switch action {
	case fixNoAction:
		return "Do nothing for this repository in the current run."
	case fixAllActions:
		return "Run all currently eligible fixes for this repository."
	case FixActionAbortOperation:
		return "Cancel the active git operation (merge, rebase, cherry-pick, or bisect)."
	case FixActionPush:
		return "Push local commits that are ahead of upstream."
	case FixActionStageCommitPush:
		return "Stage all local changes, create a commit, then push."
	case FixActionPullFFOnly:
		return "Fast-forward your branch to upstream without creating a merge commit."
	case FixActionSetUpstreamPush:
		return "Set this branch's upstream tracking target and push."
	case FixActionEnableAutoPush:
		return "Allow future bb sync runs to auto-push this repo by enabling its auto-push policy."
	default:
		return "Action has no help text yet."
	}
}

func classifyFixRepo(repo fixRepoState, actions []string) fixRepoTier {
	if repo.Record.Syncable {
		return fixRepoTierSyncable
	}
	if unsyncableReasonsFullyCoverable(repo.Record.UnsyncableReasons, actions) {
		return fixRepoTierAutofixable
	}
	if !repo.Record.Syncable {
		return fixRepoTierUnsyncableBlocked
	}
	return fixRepoTierSyncable
}

func unsyncableReasonsFullyCoverable(reasons []domain.UnsyncableReason, actions []string) bool {
	if len(reasons) == 0 {
		return false
	}
	has := map[string]bool{}
	for _, action := range actions {
		has[action] = true
	}

	covers := func(reason domain.UnsyncableReason) bool {
		switch reason {
		case domain.ReasonOperationInProgress:
			return has[FixActionAbortOperation]
		case domain.ReasonDirtyTracked, domain.ReasonDirtyUntracked:
			return has[FixActionStageCommitPush]
		case domain.ReasonMissingUpstream:
			return has[FixActionSetUpstreamPush] || has[FixActionStageCommitPush]
		case domain.ReasonPushPolicyBlocked:
			return has[FixActionPush] || has[FixActionStageCommitPush] || has[FixActionSetUpstreamPush] || has[FixActionEnableAutoPush]
		case domain.ReasonPushFailed:
			return has[FixActionPush] || has[FixActionStageCommitPush] || has[FixActionSetUpstreamPush]
		case domain.ReasonPullFailed:
			return has[FixActionPullFFOnly]
		default:
			return false
		}
	}

	for _, reason := range reasons {
		if !covers(reason) {
			return false
		}
	}
	return true
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
