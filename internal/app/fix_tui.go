package app

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"bb-project/internal/domain"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
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
	Setting  key.Binding
	Skip     key.Binding
	ApplyAll key.Binding
	Refresh  key.Binding
	Ignore   key.Binding
	Unignore key.Binding
	Help     key.Binding
	Quit     key.Binding
	Cancel   key.Binding
}

type fixTUIHelpMap struct {
	short []key.Binding
	full  [][]key.Binding
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
		Setting: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "cycle auto-push"),
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
			key.WithHelp("r", "revalidate state"),
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
	return []key.Binding{k.Up, k.Left, k.Apply, k.Setting, k.ApplyAll, k.Quit}
}

func (k fixTUIKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Apply, k.Setting, k.Skip, k.ApplyAll, k.Refresh, k.Ignore, k.Unignore},
		{k.Help, k.Cancel, k.Quit},
	}
}

func (m fixTUIHelpMap) ShortHelp() []key.Binding {
	return m.short
}

func (m fixTUIHelpMap) FullHelp() [][]key.Binding {
	return m.full
}

func newHelpBinding(keys []string, keyLabel string, desc string) key.Binding {
	if len(keys) == 0 {
		keys = []string{keyLabel}
	}
	return key.NewBinding(
		key.WithKeys(keys...),
		key.WithHelp(keyLabel, desc),
	)
}

func (m *fixTUIModel) contextualHelpMap() fixTUIHelpMap {
	switch m.viewMode {
	case fixViewWizard:
		return m.wizardHelpMap()
	case fixViewSummary:
		return m.summaryHelpMap()
	default:
		return m.listHelpMap()
	}
}

func (m *fixTUIModel) listHelpMap() fixTUIHelpMap {
	short := make([]key.Binding, 0, 9)
	primary := make([]key.Binding, 0, 4)
	secondary := make([]key.Binding, 0, 5)
	tertiary := make([]key.Binding, 0, 2)

	repo, hasRepo := m.currentRepo()
	options := []string(nil)
	if hasRepo {
		if !m.ignored[repo.Record.Path] {
			actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
				Interactive:     true,
				Risk:            repo.Risk,
				SyncStrategy:    FixSyncStrategyRebase,
				SyncFeasibility: repo.SyncFeasibility,
			})
			options = selectableFixActions(fixActionsForSelection(actions))
		}
	}

	canApplySelected := false
	if hasRepo {
		idx := m.selectedAction[repo.Record.Path]
		canApplySelected = idx >= 0 && idx < len(options)
	}
	canApplyAll := m.hasAnySelectedFixes()
	canMoveRepo := len(m.visible) > 1
	canChangeFix := len(options) > 0
	canToggleAutoPush := hasRepo && !m.ignored[repo.Record.Path] && repoMetaAllowsAutoPush(repo.Meta)
	canIgnoreRepo := hasRepo && !m.ignored[repo.Record.Path]
	canUnignoreRepo := hasRepo && m.ignored[repo.Record.Path]

	if canApplySelected {
		b := newHelpBinding([]string{"enter"}, "enter", "apply selected")
		short = append(short, b)
		primary = append(primary, b)
	}
	if canApplyAll {
		b := newHelpBinding([]string{"ctrl+a"}, "ctrl+a", "apply all selected")
		short = append(short, b)
		primary = append(primary, b)
	}
	if canMoveRepo {
		b := newHelpBinding([]string{"up", "down"}, "↑/↓", "move repo")
		short = append(short, b)
		primary = append(primary, b)
	}
	if canChangeFix {
		b := newHelpBinding([]string{"left", "right"}, "←/→", "change fix")
		short = append(short, b)
		primary = append(primary, b)
	}
	if canToggleAutoPush {
		b := newHelpBinding([]string{"s"}, "s", "cycle auto-push")
		short = append(short, b)
		secondary = append(secondary, b)
	}
	if canIgnoreRepo {
		b := newHelpBinding([]string{"i"}, "i", "ignore repo")
		short = append(short, b)
		secondary = append(secondary, b)
	}
	if canUnignoreRepo {
		b := newHelpBinding([]string{"u"}, "u", "unignore repo")
		short = append(short, b)
		secondary = append(secondary, b)
	}
	refresh := newHelpBinding([]string{"r"}, "r", "revalidate state")
	short = append(short, refresh)
	secondary = append(secondary, refresh)

	quit := newHelpBinding([]string{"q", "ctrl+c"}, "q", "quit")
	short = append(short, quit)
	tertiary = append(tertiary, quit)

	helpToggle := newHelpBinding([]string{"?"}, "?", "more keys")
	tertiary = append(tertiary, helpToggle)

	rows := make([][]key.Binding, 0, 3)
	if len(primary) > 0 {
		rows = append(rows, primary)
	}
	if len(secondary) > 0 {
		rows = append(rows, secondary)
	}
	if len(tertiary) > 0 {
		rows = append(rows, tertiary)
	}
	return fixTUIHelpMap{short: short, full: rows}
}

func (m *fixTUIModel) wizardHelpMap() fixTUIHelpMap {
	short := make([]key.Binding, 0, 8)
	primary := make([]key.Binding, 0, 5)
	secondary := make([]key.Binding, 0, 3)
	tertiary := make([]key.Binding, 0, 2)

	enterDesc := "activate"
	if m.wizard.FocusArea != fixWizardFocusActions {
		enterDesc = "next field"
	}
	enter := newHelpBinding([]string{"enter"}, "enter", enterDesc)
	short = append(short, enter)
	primary = append(primary, enter)

	switch m.wizard.FocusArea {
	case fixWizardFocusVisibility:
		b := newHelpBinding([]string{"left", "right"}, "←/→", "change visibility")
		short = append(short, b)
		primary = append(primary, b)
	case fixWizardFocusActions:
		b := newHelpBinding([]string{"left", "right"}, "←/→", "select button")
		short = append(short, b)
		primary = append(primary, b)
	case fixWizardFocusGitignore:
		b := newHelpBinding([]string{" "}, "space", "toggle")
		short = append(short, b)
		primary = append(primary, b)
	}

	cancel := newHelpBinding([]string{"esc"}, "esc", "cancel wizard")
	short = append(short, cancel)
	primary = append(primary, cancel)

	skip := newHelpBinding([]string{"ctrl+x"}, "ctrl+x", "skip repo")
	short = append(short, skip)
	primary = append(primary, skip)

	move := newHelpBinding([]string{"up", "down"}, "↑/↓", "move focus/scroll")
	short = append(short, move)
	secondary = append(secondary, move)

	focus := newHelpBinding([]string{"tab", "shift+tab"}, "tab/shift+tab", "cycle focus")
	short = append(short, focus)
	secondary = append(secondary, focus)

	helpToggle := newHelpBinding([]string{"?"}, "?", "more keys")
	quit := newHelpBinding([]string{"q", "ctrl+c"}, "q", "quit")
	tertiary = append(tertiary, helpToggle, quit)

	rows := [][]key.Binding{primary, secondary}
	if m.wizardHasContextOverflow() {
		page := newHelpBinding([]string{"pgup", "pgdown"}, "pgup/pgdn", "page scroll")
		short = append(short, page)
		rows = append(rows, []key.Binding{page})
	}
	rows = append(rows, tertiary)
	return fixTUIHelpMap{short: short, full: rows}
}

func (m *fixTUIModel) summaryHelpMap() fixTUIHelpMap {
	candidates := m.summaryFollowUpCandidates()
	m.syncSummaryFollowUpState(candidates)
	selectedCount := m.summarySelectedFollowUpCount(candidates)

	enterDesc := "back to list"
	if selectedCount > 0 {
		enterDesc = "run selected fixes"
	}
	back := newHelpBinding([]string{"enter"}, "enter", enterDesc)
	cancel := newHelpBinding([]string{"esc"}, "esc", "back to list")
	skip := newHelpBinding([]string{"ctrl+x"}, "ctrl+x", "back to list")
	quit := newHelpBinding([]string{"q", "ctrl+c"}, "q", "quit")
	helpToggle := newHelpBinding([]string{"?"}, "?", "more keys")
	move := newHelpBinding([]string{"up", "down"}, "↑/↓", "move follow-up")
	toggle := newHelpBinding([]string{"space"}, "space", "toggle follow-up")

	short := []key.Binding{back, cancel, quit}
	rows := [][]key.Binding{}
	if len(candidates) > 0 {
		short = []key.Binding{back, move, toggle, cancel, quit}
		rows = append(rows, []key.Binding{back, move, toggle})
	}
	rows = append(rows, []key.Binding{cancel, skip})
	rows = append(rows, []key.Binding{quit, helpToggle})

	return fixTUIHelpMap{
		short: short,
		full:  rows,
	}
}

func (m *fixTUIModel) hasAnySelectedFixes() bool {
	for _, repo := range m.visible {
		if m.ignored[repo.Record.Path] {
			continue
		}
		idx := m.selectedAction[repo.Record.Path]
		if idx < 0 {
			continue
		}
		actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
			Interactive:     true,
			Risk:            repo.Risk,
			SyncStrategy:    FixSyncStrategyRebase,
			SyncFeasibility: repo.SyncFeasibility,
		})
		options := selectableFixActions(fixActionsForSelection(actions))
		if idx < len(options) {
			return true
		}
	}
	return false
}

func (m *fixTUIModel) footerHelpInnerWidth(helpPanel lipgloss.Style) int {
	if w := helpPanel.GetWidth(); w > 0 {
		inner := w - helpPanel.GetHorizontalFrameSize()
		if inner > 0 {
			return inner
		}
	}
	if w := m.viewContentWidth(); w > 0 {
		inner := w - helpPanelStyle.GetHorizontalFrameSize()
		if inner > 0 {
			return inner
		}
	}
	if m.width > 0 {
		inner := (m.width - fixPageStyle.GetHorizontalFrameSize()) - helpPanelStyle.GetHorizontalFrameSize()
		if inner > 0 {
			return inner
		}
	}
	return 0
}

func (m *fixTUIModel) footerHelpView(helpPanel lipgloss.Style) string {
	helpMap := m.contextualHelpMap()
	helpModel := m.help
	innerWidth := m.footerHelpInnerWidth(helpPanel)
	if innerWidth > 0 {
		helpModel.Width = innerWidth
	}
	if helpModel.ShowAll {
		return helpModel.View(helpMap)
	}
	return renderSingleLineShortHelp(helpModel, helpMap.ShortHelp(), innerWidth)
}

func renderSingleLineShortHelp(helpModel help.Model, bindings []key.Binding, width int) string {
	separatorText := helpModel.ShortSeparator
	if separatorText == "" {
		separatorText = " • "
	}
	separator := helpModel.Styles.ShortSeparator.Render(separatorText)
	ellipsisText := helpModel.Ellipsis
	if ellipsisText == "" {
		ellipsisText = "…"
	}
	ellipsis := helpModel.Styles.Ellipsis.Render(ellipsisText)

	entries := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if !binding.Enabled() {
			continue
		}
		item := binding.Help()
		keyLabel := strings.TrimSpace(item.Key)
		desc := strings.TrimSpace(item.Desc)
		if keyLabel == "" || desc == "" {
			continue
		}
		entries = append(entries, helpModel.Styles.ShortKey.Render(keyLabel)+" "+helpModel.Styles.ShortDesc.Render(desc))
	}
	if len(entries) == 0 {
		return ""
	}
	line := strings.Join(entries, separator)
	line = strings.ReplaceAll(line, "\n", " ")
	if width > 0 {
		line = ansi.Truncate(line, width, ellipsis)
	}
	return line
}

type fixTUIModel struct {
	app             *App
	includeCatalogs []string
	loadReposFn     func(includeCatalogs []string, refreshMode scanRefreshMode) ([]fixRepoState, error)
	repos           []fixRepoState
	visible         []fixRepoState
	ignored         map[string]bool
	selectedAction  map[string]int
	rowRepoIndex    []int
	repoIndexToRow  []int

	keys fixTUIKeyMap
	help help.Model

	repoList list.Model

	width  int
	height int

	status  string
	errText string

	revalidating          bool
	revalidateSpinner     spinner.Model
	revalidatePath        string
	immediateApplying     bool
	immediateApplySpinner spinner.Model
	immediatePhase        string
	immediateStep         string
	immediateEvents       <-chan tea.Msg
	pendingCmd            tea.Cmd

	viewMode fixViewMode
	wizard   fixWizardState

	summaryResults           []fixSummaryResult
	summaryCursor            int
	summarySelectedFollowUps map[string]bool
}

type fixListItem struct {
	Kind              fixListItemKind
	Catalog           string
	NamePlain         string
	Path              string
	Name              string
	Branch            string
	State             string
	AutoPushMode      domain.AutoPushMode
	AutoPushAvailable bool
	Reasons           string
	Action            string
	ActionKey         string
	Tier              fixRepoTier
	Ignored           bool
}

type fixListItemKind int

const (
	fixListItemRepo fixListItemKind = iota
	fixListItemCatalogHeader
	fixListItemCatalogBreak
)

func (i fixListItem) FilterValue() string {
	return i.NamePlain + " " + i.Path + " " + i.Branch + " " + i.Reasons + " " + i.Action + " " + i.Catalog
}

type fixColumnLayout struct {
	Repo    int
	Branch  int
	State   int
	Auto    int
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

	switch row.Kind {
	case fixListItemCatalogBreak:
		fmt.Fprint(w, fixCatalogBreakStyle.Render(strings.Repeat("─", max(12, m.Width()-2))))
		return
	case fixListItemCatalogHeader:
		fmt.Fprint(w, fixCatalogHeaderStyle.Render(row.Name))
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

	repoStyle := fixRepoNameStyle.Copy().Bold(selected)
	branchStyle := fixBranchStyle
	reasonsStyle := fixReasonsStyle
	stateStyle := fixStateSyncableStyle
	switch row.Tier {
	case fixRepoTierAutofixable:
		stateStyle = fixStateAutofixableStyle
	case fixRepoTierUnsyncableBlocked:
		stateStyle = fixStateUnsyncableStyle
	}
	if row.Ignored {
		repoStyle = repoStyle.Copy().Foreground(mutedTextColor).Faint(true)
		branchStyle = branchStyle.Copy().Faint(true)
		reasonsStyle = reasonsStyle.Copy().Faint(true)
		stateStyle = fixStateIgnoredStyle
	}
	if selected {
		stateStyle = stateStyle.Copy().Bold(true)
	}
	repoCell := renderFixColumnCell(row.Name, layout.Repo, repoStyle)
	branchCell := renderFixColumnCell(row.Branch, layout.Branch, branchStyle)
	stateCell := renderFixColumnCell(row.State, layout.State, stateStyle)
	autoPushText := autoPushModeDisplayLabel(row.AutoPushMode)
	autoPushStyle := fixAutoPushOffStyle
	if !row.AutoPushAvailable {
		autoPushText = "n/a"
		autoPushStyle = fixNoActionStyle
	} else if row.AutoPushMode == domain.AutoPushModeIncludeDefaultBranch {
		autoPushStyle = fixAutoPushIncludeDefaultStyle
	} else if row.AutoPushMode == domain.AutoPushModeEnabled {
		autoPushStyle = fixAutoPushOnStyle
	}
	if row.Ignored {
		autoPushStyle = autoPushStyle.Copy().Foreground(mutedTextColor).Faint(true)
	}
	if selected {
		autoPushStyle = autoPushStyle.Copy().Bold(true)
	}
	autoPushCell := renderFixColumnCell(autoPushText, layout.Auto, autoPushStyle)
	reasonsCell := renderFixColumnCell(row.Reasons, layout.Reasons, reasonsStyle)
	actionStyle := fixActionStyleFor(row.ActionKey)
	if row.Ignored {
		actionStyle = fixNoActionStyle.Copy().Faint(true)
	}
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
		autoPushCell,
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
	fixListReservedCols = 10
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

	fixStateIgnoredStyle = lipgloss.NewStyle().
				Foreground(mutedTextColor).
				Faint(true)

	fixIndicatorStyle = lipgloss.NewStyle().
				Foreground(borderColor)

	fixIndicatorSelectedStyle = lipgloss.NewStyle().
					Foreground(accentColor).
					Bold(true)

	fixSelectedRowStyle = lipgloss.NewStyle().
				Background(accentBgColor)

	fixCatalogHeaderStyle = lipgloss.NewStyle().
				Foreground(accentColor).
				Bold(true)

	fixCatalogBreakStyle = lipgloss.NewStyle().
				Foreground(borderColor)

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

	fixActionCreateProjectStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("113"))

	fixActionForkStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("99"))

	fixActionSyncStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("111"))

	fixActionAutoPushStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("70"))

	fixActionAbortStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("203"))

	fixAutoPushOnStyle = lipgloss.NewStyle().
				Foreground(successColor)

	fixAutoPushOffStyle = lipgloss.NewStyle().
				Foreground(mutedTextColor)

	fixAutoPushIncludeDefaultStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("81"))

	fixPageStyle = lipgloss.NewStyle().Padding(0, 2)
)

const fixListColumnGap = "  "

func (a *App) runFixInteractive(includeCatalogs []string, noRefresh bool) (int, error) {
	model := newFixTUIBootModel(a, includeCatalogs, noRefresh)
	program := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := program.Run()
	if err != nil {
		return 2, err
	}
	if boot, ok := finalModel.(*fixTUIBootModel); ok && boot.loadErr != nil {
		return 2, boot.loadErr
	}
	return 0, nil
}

type fixTUIBootModel struct {
	app             *App
	includeCatalogs []string
	noRefresh       bool

	width  int
	height int

	spin    spinner.Model
	loadFn  func() (*fixTUIModel, error)
	loadErr error

	progressMu   sync.RWMutex
	progressLine string
}

type fixTUILoadedMsg struct {
	model *fixTUIModel
	err   error
}

type fixTUIRevalidatedMsg struct {
	repos         []fixRepoState
	err           error
	preferredPath string
}

type fixImmediateActionTask struct {
	RepoPath string
	RepoName string
	Action   string
}

type fixTUIImmediateApplyTaskStartedMsg struct {
	Task fixImmediateActionTask
}

type fixTUIImmediateApplyProgressMsg struct {
	Task  fixImmediateActionTask
	Event fixApplyStepEvent
}

type fixTUIImmediateApplyCompletedMsg struct {
	Results       []fixSummaryResult
	Applied       int
	Failed        int
	Skipped       int
	Queue         []fixWizardDecision
	Repos         []fixRepoState
	RefreshErr    error
	PreferredPath string
}

type fixTUIImmediateApplyChannelClosedMsg struct{}

func newFixProgressSpinner() spinner.Model {
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	spin.Style = lipgloss.NewStyle().Foreground(accentColor)
	return spin
}

func newFixTUIBootModel(app *App, includeCatalogs []string, noRefresh bool) *fixTUIBootModel {
	spin := newFixProgressSpinner()

	return &fixTUIBootModel{
		app:             app,
		includeCatalogs: append([]string(nil), includeCatalogs...),
		noRefresh:       noRefresh,
		spin:            spin,
		progressLine:    "Preparing interactive fix startup",
	}
}

func (m *fixTUIBootModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.loadReposCmd())
}

func (m *fixTUIBootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case fixTUILoadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err
			return m, tea.Quit
		}
		if msg.model == nil {
			m.loadErr = errors.New("internal: fix tui model loader returned nil")
			return m, tea.Quit
		}
		m.transferWindowSize(msg.model)
		return msg.model, nil
	case tea.KeyMsg:
		if key.Matches(msg, defaultFixTUIKeyMap().Quit) {
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m *fixTUIBootModel) View() string {
	title := lipgloss.JoinHorizontal(lipgloss.Center,
		titleBadgeStyle.Render("bb"),
		" "+headerStyle.Render("fix"),
	)
	subtitle := hintStyle.Render("Interactive remediation for unsyncable repositories")

	message := fmt.Sprintf("%s %s", m.spin.View(), "Loading repositories and risk checks for interactive fix...")
	detail := hintStyle.Render("Interactive UI opens automatically when startup checks complete.")

	contentPanel := panelStyle
	if w := m.viewContentWidth(); w > 0 {
		contentPanel = contentPanel.Width(w)
	}

	var b strings.Builder
	b.WriteString(labelStyle.Render(m.currentProgress()))
	b.WriteString("\n")
	b.WriteString(message)
	b.WriteString("\n")
	b.WriteString(detail)

	body := title + "\n" + subtitle + "\n\n" + contentPanel.Render(b.String())
	return fixPageStyle.Render(body)
}

func (m *fixTUIBootModel) loadReposCmd() tea.Cmd {
	return func() tea.Msg {
		load := m.loadFn
		if load == nil {
			load = func() (*fixTUIModel, error) {
				return newFixTUIModel(m.app, m.includeCatalogs, m.noRefresh)
			}
		}

		restoreLogObserver := func() {}
		if m.app != nil {
			restoreLogObserver = m.app.setLogObserver(m.setProgress)
		}
		defer restoreLogObserver()

		model, err := load()
		return fixTUILoadedMsg{model: model, err: err}
	}
}

func (m *fixTUIBootModel) transferWindowSize(target *fixTUIModel) {
	target.width = m.width
	target.height = m.height
	target.help.Width = m.width
	target.resizeRepoList()
}

func (m *fixTUIBootModel) viewContentWidth() int {
	if m.width <= 0 {
		return 0
	}
	contentWidth := m.width - 8
	if contentWidth < 52 {
		return 0
	}
	return contentWidth
}

func (m *fixTUIBootModel) setProgress(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	m.progressMu.Lock()
	m.progressLine = line
	m.progressMu.Unlock()
}

func (m *fixTUIBootModel) currentProgress() string {
	m.progressMu.RLock()
	defer m.progressMu.RUnlock()
	if strings.TrimSpace(m.progressLine) == "" {
		return "Preparing interactive fix startup"
	}
	return m.progressLine
}

func newFixTUIModel(app *App, includeCatalogs []string, noRefresh bool) (*fixTUIModel, error) {
	repoList := newFixRepoListModel()

	m := &fixTUIModel{
		app:                      app,
		includeCatalogs:          append([]string(nil), includeCatalogs...),
		loadReposFn:              app.loadFixRepos,
		ignored:                  map[string]bool{},
		selectedAction:           map[string]int{},
		summaryCursor:            0,
		summarySelectedFollowUps: map[string]bool{},
		keys:                     defaultFixTUIKeyMap(),
		help:                     help.New(),
		repoList:                 repoList,
		revalidateSpinner:        newFixProgressSpinner(),
		immediateApplySpinner:    newFixProgressSpinner(),
	}
	m.help.ShowAll = false
	initialRefreshMode := scanRefreshIfStale
	if noRefresh {
		initialRefreshMode = scanRefreshNever
	}
	if err := m.refreshRepos(initialRefreshMode); err != nil {
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
		if m.viewMode == fixViewWizard {
			m.syncWizardViewport()
		}
		return m, nil
	case spinner.TickMsg:
		if m.viewMode == fixViewWizard && m.wizard.Applying {
			var cmd tea.Cmd
			m.wizard.ApplySpinner, cmd = m.wizard.ApplySpinner.Update(msg)
			return m, cmd
		}
		if m.immediateApplying {
			var cmd tea.Cmd
			m.immediateApplySpinner, cmd = m.immediateApplySpinner.Update(msg)
			return m, cmd
		}
		if m.revalidating {
			var cmd tea.Cmd
			m.revalidateSpinner, cmd = m.revalidateSpinner.Update(msg)
			return m, cmd
		}
		return m, nil
	case fixTUIRevalidatedMsg:
		m.revalidating = false
		m.revalidatePath = ""
		if msg.err != nil {
			m.errText = msg.err.Error()
			m.status = "revalidation failed"
			return m, nil
		}
		m.repos = msg.repos
		m.rebuildList(msg.preferredPath)
		m.errText = ""
		m.status = fmt.Sprintf("revalidated %d repos", len(m.visible))
		return m, nil
	case fixWizardApplyProgressMsg:
		return m, m.handleWizardApplyProgress(msg)
	case fixWizardApplyCompletedMsg:
		return m, m.handleWizardApplyCompleted(msg)
	case fixWizardApplyChannelClosedMsg:
		if m.viewMode == fixViewWizard && m.wizard.Applying {
			m.wizard.Applying = false
			m.wizard.ApplyPhase = ""
			m.wizard.ApplyEvents = nil
			if m.errText == "" {
				m.errText = "internal: apply stream closed unexpectedly"
			}
		}
		return m, nil
	case fixTUIImmediateApplyTaskStartedMsg:
		m.immediatePhase = fixWizardApplyPhasePreparing
		m.immediateStep = fmt.Sprintf("%s (%s)", msg.Task.RepoName, fixActionLabel(msg.Task.Action))
		m.status = m.immediateApplyStatusLine()
		if m.immediateEvents != nil {
			return m, waitImmediateApplyMsg(m.immediateEvents)
		}
		return m, nil
	case fixTUIImmediateApplyProgressMsg:
		return m, m.handleImmediateApplyProgress(msg)
	case fixTUIImmediateApplyCompletedMsg:
		return m, m.handleImmediateApplyCompleted(msg)
	case fixTUIImmediateApplyChannelClosedMsg:
		if m.immediateApplying {
			m.immediateApplying = false
			m.immediatePhase = ""
			m.immediateStep = ""
			m.immediateEvents = nil
			if m.errText == "" {
				m.errText = "internal: apply stream closed unexpectedly"
			}
		}
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
		if m.revalidating || m.immediateApplying {
			return m, nil
		}
		if key.Matches(msg, m.keys.Up) {
			m.moveRepoCursor(-1)
			return m, nil
		}
		if key.Matches(msg, m.keys.Down) {
			m.moveRepoCursor(1)
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
			return m, m.takePendingCmd()
		case key.Matches(msg, m.keys.Setting):
			m.toggleCurrentRepoAutoPush()
			return m, nil
		case key.Matches(msg, m.keys.ApplyAll):
			m.applyAllSelections()
			return m, m.takePendingCmd()
		case key.Matches(msg, m.keys.Refresh):
			return m, m.beginRevalidate()
		case key.Matches(msg, m.keys.Ignore):
			m.ignoreCurrentRepo()
			return m, nil
		case key.Matches(msg, m.keys.Unignore):
			m.unignoreCurrentRepo()
			return m, nil
		}
	}

	if m.viewMode == fixViewWizard {
		var cmd tea.Cmd
		m.wizard.BodyViewport, cmd = m.wizard.BodyViewport.Update(msg)
		return m, cmd
	}
	if m.viewMode == fixViewSummary {
		return m, nil
	}

	var cmd tea.Cmd
	m.repoList, cmd = m.repoList.Update(msg)
	return m, cmd
}

func (m *fixTUIModel) View() string {
	m.resizeRepoList()

	var b strings.Builder
	if m.viewMode == fixViewWizard {
		b.WriteString(m.viewWizardTopLine())
		b.WriteString("\n")
	} else {
		title := lipgloss.JoinHorizontal(lipgloss.Center,
			titleBadgeStyle.Render("bb"),
			" "+headerStyle.Render("fix"),
		)
		subtitle := hintStyle.Render("Interactive remediation for unsyncable repositories")
		b.WriteString(title)
		b.WriteString("\n")
		b.WriteString(subtitle)
		b.WriteString("\n\n")
	}

	contentPanel := m.mainContentPanelStyle()
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

	helpPanel := helpPanelStyle
	if w := m.viewContentWidth(); w > 0 {
		helpPanel = helpPanel.Width(w)
	}
	helpView := m.footerHelpView(helpPanel)
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

func (m *fixTUIModel) mainContentPanelStyle() lipgloss.Style {
	contentPanel := panelStyle
	if (m.viewMode == fixViewWizard && m.wizard.Applying) || m.revalidating || m.immediateApplying {
		contentPanel = contentPanel.BorderForeground(accentColor)
	}
	if w := m.viewContentWidth(); w > 0 {
		contentPanel = contentPanel.Width(w)
	}
	return contentPanel
}

func (m *fixTUIModel) viewMainContent() string {
	var b strings.Builder
	title := "Repository Fixes"
	if m.immediateApplying {
		title += " " + m.immediateApplySpinner.View()
	} else if m.revalidating {
		title += " " + m.revalidateSpinner.View()
	}
	b.WriteString(labelStyle.Render(title))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Select per-repo fixes. Default selection is '-' (no action)."))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Grouped by catalog (default catalog first), then fixable, unsyncable, syncable, and ignored."))
	if m.immediateApplying {
		b.WriteString("\n")
		b.WriteString(hintStyle.Render(m.immediateApplyStatusLine()))
	}
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
		renderFixColumnCell("Auto-push", layout.Auto, fixHeaderCellStyle),
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
		Interactive:     true,
		Risk:            repo.Risk,
		SyncStrategy:    FixSyncStrategyRebase,
		SyncFeasibility: repo.SyncFeasibility,
	})
	action := m.currentActionForRepo(repo.Record.Path, fixActionsForSelection(actions))
	ignored := m.ignored[repo.Record.Path]
	if ignored {
		action = fixNoAction
	}
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
	if ignored {
		state = "ignored"
		stateStyle = fixStateIgnoredStyle
	} else {
		tier := classifyFixRepo(repo, actions)
		switch tier {
		case fixRepoTierAutofixable:
			state = "fixable"
			stateStyle = fixStateAutofixableStyle
		case fixRepoTierUnsyncableBlocked:
			state = "unsyncable"
			stateStyle = fixStateUnsyncableStyle
		}
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
	autoPushLabel := lipgloss.NewStyle().Foreground(mutedTextColor).Render("Auto-push:")
	actionLabel := lipgloss.NewStyle().Foreground(mutedTextColor).Render("Action:")
	branchLabel := lipgloss.NewStyle().Foreground(mutedTextColor).Render("Branch:")
	autoPushValue := autoPushModeDisplayLabel(repoMetaAutoPushMode(repo.Meta))
	autoPushValueStyle := fixAutoPushOffStyle
	if !repoMetaAllowsAutoPush(repo.Meta) {
		autoPushValue = "n/a"
		autoPushValueStyle = fixNoActionStyle
	} else if repoMetaAutoPushMode(repo.Meta) == domain.AutoPushModeIncludeDefaultBranch {
		autoPushValueStyle = fixAutoPushIncludeDefaultStyle
	} else if repoMetaAutoPushMode(repo.Meta) == domain.AutoPushModeEnabled {
		autoPushValueStyle = fixAutoPushOnStyle
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
		stateLabel,
		" ",
		stateStyle.Render(state),
		"   ",
		autoPushLabel,
		" ",
		autoPushValueStyle.Render(autoPushValue),
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
	ignored := 0
	for _, repo := range m.visible {
		if !m.ignored[repo.Record.Path] && m.selectedAction[repo.Record.Path] >= 0 {
			pending++
		}
		if m.ignored[repo.Record.Path] {
			ignored++
			continue
		}
		tier := classifyFixRepo(repo, eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
			Interactive:     true,
			Risk:            repo.Risk,
			SyncStrategy:    FixSyncStrategyRebase,
			SyncFeasibility: repo.SyncFeasibility,
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
	blockedStyle := renderFixSummaryPill(fmt.Sprintf("%d unsyncable", blocked), errorFgColor)
	syncStyle := renderFixSummaryPill(fmt.Sprintf("%d syncable", syncable), successColor)
	ignoredStyle := renderFixSummaryPill(fmt.Sprintf("%d ignored", ignored), mutedTextColor)
	return lipgloss.JoinHorizontal(lipgloss.Top, totalStyle, " ", pendingStyle, "  ", autoStyle, " ", blockedStyle, " ", syncStyle, " ", ignoredStyle)
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
	helpHeight := lipgloss.Height(helpPanel.Render(m.footerHelpView(helpPanel)))

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

func (m *fixTUIModel) refreshRepos(refreshMode scanRefreshMode) error {
	selectedPath := ""
	if current, ok := m.currentRepo(); ok {
		selectedPath = current.Record.Path
	}

	loadRepos := m.loadReposFn
	if loadRepos == nil {
		if m.app == nil {
			return errors.New("internal: app is not configured")
		}
		loadRepos = m.app.loadFixRepos
	}
	repos, err := loadRepos(m.includeCatalogs, refreshMode)
	if err != nil {
		return err
	}
	m.repos = repos
	m.rebuildList(selectedPath)
	m.errText = ""
	return nil
}

func (m *fixTUIModel) beginRevalidate() tea.Cmd {
	if m.revalidating {
		return nil
	}
	m.revalidating = true
	m.errText = ""
	m.status = "revalidating repository states..."
	m.revalidatePath = ""
	if current, ok := m.currentRepo(); ok {
		m.revalidatePath = current.Record.Path
	}
	return tea.Batch(m.revalidateSpinner.Tick, m.revalidateReposCmd(m.revalidatePath))
}

func (m *fixTUIModel) beginImmediateApply(tasks []fixImmediateActionTask, queue []fixWizardDecision, skipped int) {
	if m.immediateApplying {
		return
	}
	m.immediateApplying = true
	m.immediatePhase = fixWizardApplyPhasePreparing
	m.immediateStep = ""
	m.errText = ""
	m.status = m.immediateApplyStatusLine()

	progress := make(chan tea.Msg, max(8, len(tasks)*4+4))
	m.immediateEvents = progress
	includeCatalogs := append([]string(nil), m.includeCatalogs...)
	loadRepos := m.loadReposFn
	app := m.app
	preferredPath := ""
	if current, ok := m.currentRepo(); ok {
		preferredPath = current.Record.Path
	}

	go func() {
		results := make([]fixSummaryResult, 0, len(tasks))
		applied := 0
		failed := 0

		for _, task := range tasks {
			progress <- fixTUIImmediateApplyTaskStartedMsg{Task: task}
			if app == nil {
				results = append(results, fixSummaryResult{
					RepoName: task.RepoName,
					RepoPath: task.RepoPath,
					Action:   fixActionLabel(task.Action),
					Status:   "failed",
					Detail:   "internal: app is not configured",
				})
				failed++
				continue
			}

			_, err := app.applyFixActionWithObserver(includeCatalogs, task.RepoPath, task.Action, fixApplyOptions{
				Interactive:  true,
				SyncStrategy: FixSyncStrategyRebase,
			}, func(event fixApplyStepEvent) {
				progress <- fixTUIImmediateApplyProgressMsg{Task: task, Event: event}
			})
			if err != nil {
				results = append(results, fixSummaryResult{
					RepoName: task.RepoName,
					RepoPath: task.RepoPath,
					Action:   fixActionLabel(task.Action),
					Status:   "failed",
					Detail:   err.Error(),
				})
				failed++
				continue
			}

			results = append(results, fixSummaryResult{
				RepoName: task.RepoName,
				RepoPath: task.RepoPath,
				Action:   fixActionLabel(task.Action),
				Status:   "applied",
			})
			applied++
		}

		var repos []fixRepoState
		var refreshErr error
		if loadRepos != nil && (applied > 0 || failed > 0) {
			repos, refreshErr = loadRepos(includeCatalogs, scanRefreshAlways)
		}

		progress <- fixTUIImmediateApplyCompletedMsg{
			Results:       results,
			Applied:       applied,
			Failed:        failed,
			Skipped:       skipped,
			Queue:         append([]fixWizardDecision(nil), queue...),
			Repos:         repos,
			RefreshErr:    refreshErr,
			PreferredPath: preferredPath,
		}
		close(progress)
	}()

	m.pendingCmd = tea.Batch(m.immediateApplySpinner.Tick, waitImmediateApplyMsg(progress))
}

func waitImmediateApplyMsg(progress <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-progress
		if !ok {
			return fixTUIImmediateApplyChannelClosedMsg{}
		}
		return msg
	}
}

func (m *fixTUIModel) immediateApplyStatusLine() string {
	phase := strings.TrimSpace(m.immediatePhase)
	if phase == "" {
		phase = fixWizardApplyPhasePreparing
	}
	line := fmt.Sprintf("%s %s... controls are locked until execution completes.", m.immediateApplySpinner.View(), phase)
	if step := strings.TrimSpace(m.immediateStep); step != "" {
		line += " Current step: " + step
	}
	return line
}

func (m *fixTUIModel) handleImmediateApplyProgress(msg fixTUIImmediateApplyProgressMsg) tea.Cmd {
	entrySummary := strings.TrimSpace(msg.Event.Entry.Summary)
	if entrySummary != "" {
		m.immediateStep = entrySummary
	}
	if msg.Event.Status == fixApplyStepRunning {
		m.immediatePhase = m.wizardApplyPhaseForEntry(msg.Event.Entry)
	}
	m.status = m.immediateApplyStatusLine()
	if m.immediateEvents != nil {
		return waitImmediateApplyMsg(m.immediateEvents)
	}
	return nil
}

func (m *fixTUIModel) handleImmediateApplyCompleted(msg fixTUIImmediateApplyCompletedMsg) tea.Cmd {
	m.immediateApplying = false
	m.immediatePhase = ""
	m.immediateStep = ""
	m.immediateEvents = nil

	m.summaryResults = append(m.summaryResults, msg.Results...)

	if msg.RefreshErr != nil {
		m.errText = msg.RefreshErr.Error()
		m.status = "failed to refresh repository state after apply"
		return nil
	}
	if len(msg.Repos) > 0 {
		m.repos = msg.Repos
		m.rebuildList(msg.PreferredPath)
	}

	if msg.Failed > 0 {
		m.errText = "one or more fixes failed"
	} else {
		m.errText = ""
	}

	if len(msg.Queue) > 0 {
		m.startWizardQueue(msg.Queue)
		return nil
	}

	m.status = fmt.Sprintf("applied %d, skipped %d, failed %d", msg.Applied, msg.Skipped, msg.Failed)
	return nil
}

func (m *fixTUIModel) takePendingCmd() tea.Cmd {
	cmd := m.pendingCmd
	m.pendingCmd = nil
	return cmd
}

func (m *fixTUIModel) revalidateReposCmd(preferredPath string) tea.Cmd {
	loadRepos := m.loadReposFn
	includeCatalogs := append([]string(nil), m.includeCatalogs...)
	return func() tea.Msg {
		if loadRepos == nil {
			if m.app == nil {
				return fixTUIRevalidatedMsg{
					err:           errors.New("internal: app is not configured"),
					preferredPath: preferredPath,
				}
			}
			loadRepos = m.app.loadFixRepos
		}
		repos, err := loadRepos(includeCatalogs, scanRefreshAlways)
		return fixTUIRevalidatedMsg{
			repos:         repos,
			err:           err,
			preferredPath: preferredPath,
		}
	}
}

func (m *fixTUIModel) rebuildList(preferredPath string) {
	type fixVisibleEntry struct {
		repo    fixRepoState
		actions []string
		tier    fixRepoTier
	}

	m.visible = m.visible[:0]
	m.rowRepoIndex = m.rowRepoIndex[:0]
	m.repoIndexToRow = m.repoIndexToRow[:0]
	items := make([]list.Item, 0, len(m.repos)*2)
	entries := make([]fixVisibleEntry, 0, len(m.repos))

	for _, repo := range m.repos {
		path := repo.Record.Path

		actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
			Interactive:     true,
			Risk:            repo.Risk,
			SyncStrategy:    FixSyncStrategyRebase,
			SyncFeasibility: repo.SyncFeasibility,
		})
		options := selectableFixActions(fixActionsForSelection(actions))
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
		if entries[i].repo.IsDefaultCatalog != entries[j].repo.IsDefaultCatalog {
			return entries[i].repo.IsDefaultCatalog
		}
		if entries[i].repo.Record.Catalog != entries[j].repo.Record.Catalog {
			return strings.ToLower(entries[i].repo.Record.Catalog) < strings.ToLower(entries[j].repo.Record.Catalog)
		}
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

	currentCatalog := ""
	rowIndex := 0
	appendCatalogBreak := func() {
		items = append(items, fixListItem{Kind: fixListItemCatalogBreak})
		m.rowRepoIndex = append(m.rowRepoIndex, -1)
		rowIndex++
	}
	appendCatalogHeader := func(catalog string, isDefault bool) {
		label := fmt.Sprintf("Catalog: %s", catalog)
		if isDefault {
			label += " (default)"
		}
		items = append(items, fixListItem{
			Kind:      fixListItemCatalogHeader,
			Catalog:   catalog,
			Name:      label,
			NamePlain: label,
		})
		m.rowRepoIndex = append(m.rowRepoIndex, -1)
		rowIndex++
	}

	for _, entry := range entries {
		repo := entry.repo
		path := repo.Record.Path
		if repo.Record.Catalog != currentCatalog {
			if currentCatalog != "" {
				appendCatalogBreak()
			}
			currentCatalog = repo.Record.Catalog
			appendCatalogHeader(currentCatalog, repo.IsDefaultCatalog)
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
		switch entry.tier {
		case fixRepoTierAutofixable:
			state = "fixable"
		case fixRepoTierUnsyncableBlocked:
			state = "unsyncable"
		}
		ignored := m.ignored[path]
		if ignored {
			state = "ignored"
		}

		selected := m.currentActionForRepo(path, fixActionsForSelection(entry.actions))
		if ignored {
			selected = fixNoAction
		}
		repoIndex := len(m.visible)
		items = append(items, fixListItem{
			Kind:              fixListItemRepo,
			Catalog:           repo.Record.Catalog,
			NamePlain:         repo.Record.Name,
			Path:              path,
			Name:              formatRepoDisplayName(repo),
			Branch:            repo.Record.Branch,
			State:             state,
			AutoPushMode:      repoMetaAutoPushMode(repo.Meta),
			AutoPushAvailable: repoMetaAllowsAutoPush(repo.Meta),
			Reasons:           reasons,
			Action:            fixActionLabel(selected),
			ActionKey:         selected,
			Tier:              entry.tier,
			Ignored:           ignored,
		})
		m.rowRepoIndex = append(m.rowRepoIndex, repoIndex)
		m.repoIndexToRow = append(m.repoIndexToRow, rowIndex)
		rowIndex++
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
	if target >= len(m.repoIndexToRow) {
		target = len(m.repoIndexToRow) - 1
	}
	row := m.repoIndexToRow[target]
	m.repoList.Select(row)
}

func (m *fixTUIModel) currentRepo() (fixRepoState, bool) {
	if len(m.visible) == 0 {
		return fixRepoState{}, false
	}
	row := m.repoList.Index()
	if row < 0 || row >= len(m.rowRepoIndex) {
		return fixRepoState{}, false
	}
	idx := m.rowRepoIndex[row]
	if idx < 0 || idx >= len(m.visible) {
		return fixRepoState{}, false
	}
	return m.visible[idx], true
}

func (m *fixTUIModel) moveRepoCursor(delta int) {
	if len(m.visible) == 0 || len(m.rowRepoIndex) == 0 || delta == 0 {
		return
	}
	row := m.repoList.Index()
	if row < 0 || row >= len(m.rowRepoIndex) {
		row = 0
	}
	next := row
	for {
		next += delta
		if next < 0 || next >= len(m.rowRepoIndex) {
			return
		}
		if m.rowRepoIndex[next] >= 0 {
			m.repoList.Select(next)
			return
		}
	}
}

func (m *fixTUIModel) cycleCurrentAction(delta int) {
	repo, ok := m.currentRepo()
	if !ok {
		return
	}
	if m.ignored[repo.Record.Path] {
		m.status = fmt.Sprintf("%s is ignored for this session; press u to unignore", repo.Record.Name)
		return
	}
	actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
		Interactive:     true,
		Risk:            repo.Risk,
		SyncStrategy:    FixSyncStrategyRebase,
		SyncFeasibility: repo.SyncFeasibility,
	})
	options := selectableFixActions(fixActionsForSelection(actions))
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
	if m.ignored[repo.Record.Path] {
		m.status = fmt.Sprintf("%s is ignored for this session; press u to unignore", repo.Record.Name)
		return
	}
	actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
		Interactive:     true,
		Risk:            repo.Risk,
		SyncStrategy:    FixSyncStrategyRebase,
		SyncFeasibility: repo.SyncFeasibility,
	})
	selectionActions := fixActionsForSelection(actions)
	options := selectableFixActions(selectionActions)
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
	m.resetSummaryFollowUpState()
	m.summaryResults = nil

	queue := make([]fixWizardDecision, 0, 2)
	immediateTasks := make([]fixImmediateActionTask, 0, 2)
	selections := []string{action}
	if action == fixAllActions {
		selections = fixActionsForAllExecution(selectionActions)
	}
	for _, selection := range selections {
		if isRiskyFixAction(selection) {
			queue = append(queue, fixWizardDecision{
				RepoPath: repo.Record.Path,
				Action:   selection,
			})
			continue
		}
		immediateTasks = append(immediateTasks, fixImmediateActionTask{
			RepoPath: repo.Record.Path,
			RepoName: repo.Record.Name,
			Action:   selection,
		})
	}

	if len(immediateTasks) > 0 {
		m.beginImmediateApply(immediateTasks, queue, 0)
		return
	}
	if len(queue) > 0 {
		m.startWizardQueue(queue)
		return
	}
	m.errText = ""
	m.status = "no applicable actions selected"
}

func (m *fixTUIModel) applyAllSelections() {
	if len(m.visible) == 0 {
		m.status = "no repositories available"
		return
	}

	m.resetSummaryFollowUpState()
	m.summaryResults = nil
	applied := 0
	skipped := 0
	queue := make([]fixWizardDecision, 0, 8)
	immediateTasks := make([]fixImmediateActionTask, 0, len(m.visible))

	for _, repo := range m.visible {
		if m.ignored[repo.Record.Path] {
			skipped++
			continue
		}
		actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
			Interactive:     true,
			Risk:            repo.Risk,
			SyncStrategy:    FixSyncStrategyRebase,
			SyncFeasibility: repo.SyncFeasibility,
		})
		selectionActions := fixActionsForSelection(actions)
		options := selectableFixActions(selectionActions)
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
			selections = fixActionsForAllExecution(selectionActions)
		}
		for _, selected := range selections {
			if isRiskyFixAction(selected) {
				queue = append(queue, fixWizardDecision{
					RepoPath: repo.Record.Path,
					Action:   selected,
				})
				continue
			}
			immediateTasks = append(immediateTasks, fixImmediateActionTask{
				RepoPath: repo.Record.Path,
				RepoName: repo.Record.Name,
				Action:   selected,
			})
			applied++
		}
	}

	if len(immediateTasks) > 0 {
		m.beginImmediateApply(immediateTasks, queue, skipped)
		return
	}

	if len(queue) > 0 {
		m.startWizardQueue(queue)
		return
	}
	m.errText = ""
	m.status = fmt.Sprintf("applied %d, skipped %d, failed %d", applied, skipped, 0)
}

func nextAutoPushMode(mode domain.AutoPushMode) domain.AutoPushMode {
	switch domain.NormalizeAutoPushMode(mode) {
	case domain.AutoPushModeDisabled:
		return domain.AutoPushModeEnabled
	case domain.AutoPushModeEnabled:
		return domain.AutoPushModeIncludeDefaultBranch
	default:
		return domain.AutoPushModeDisabled
	}
}

func autoPushModeDisplayLabel(mode domain.AutoPushMode) string {
	switch domain.NormalizeAutoPushMode(mode) {
	case domain.AutoPushModeEnabled:
		return "true"
	case domain.AutoPushModeIncludeDefaultBranch:
		return "include-default-branch"
	default:
		return "false"
	}
}

func (m *fixTUIModel) toggleCurrentRepoAutoPush() {
	repo, ok := m.currentRepo()
	if !ok {
		m.status = "no repository selected"
		return
	}
	if m.ignored[repo.Record.Path] {
		m.status = fmt.Sprintf("%s is ignored for this session; press u to unignore", repo.Record.Name)
		return
	}
	repoKey := strings.TrimSpace(repo.Record.RepoKey)
	if repoKey == "" {
		m.errText = fmt.Sprintf("%s has no repo_key; cannot toggle auto-push", repo.Record.Name)
		return
	}
	if !repoMetaAllowsAutoPush(repo.Meta) {
		m.errText = fmt.Sprintf("auto-push is n/a for %s (read-only remote)", repo.Record.Name)
		return
	}
	next := nextAutoPushMode(repoMetaAutoPushMode(repo.Meta))

	if m.app != nil {
		code, err := m.app.RunRepoPolicy(repoKey, next)
		if err != nil {
			m.errText = err.Error()
			return
		}
		if code != 0 {
			m.errText = fmt.Sprintf("repo policy returned non-zero exit code %d", code)
			return
		}
	}
	m.setLocalRepoAutoPushMode(repo.Record.Path, next)

	m.errText = ""
	m.status = fmt.Sprintf("auto-push %s for %s", autoPushModeDisplayLabel(next), repo.Record.Name)
}

func (m *fixTUIModel) setLocalRepoAutoPushMode(path string, mode domain.AutoPushMode) {
	for i := range m.repos {
		if m.repos[i].Record.Path != path {
			continue
		}
		if m.repos[i].Meta == nil {
			m.repos[i].Meta = &domain.RepoMetadataFile{RepoKey: m.repos[i].Record.RepoKey}
		}
		m.repos[i].Meta.AutoPush = domain.NormalizeAutoPushMode(mode)
	}
	m.rebuildList(path)
}

func (m *fixTUIModel) ignoreCurrentRepo() {
	repo, ok := m.currentRepo()
	if !ok {
		return
	}
	if m.ignored[repo.Record.Path] {
		m.status = fmt.Sprintf("%s is already ignored for this session", repo.Record.Name)
		return
	}
	m.ignored[repo.Record.Path] = true
	m.status = fmt.Sprintf("ignored %s for this session", repo.Record.Name)
	m.rebuildList(repo.Record.Path)
}

func (m *fixTUIModel) unignoreCurrentRepo() {
	repo, ok := m.currentRepo()
	if ok && m.ignored[repo.Record.Path] {
		delete(m.ignored, repo.Record.Path)
		m.status = fmt.Sprintf("unignored %s", repo.Record.Name)
		m.rebuildList(repo.Record.Path)
		return
	}
	if len(m.ignored) == 0 {
		m.status = "no ignored repositories"
		return
	}
	if ok {
		m.status = fmt.Sprintf("%s is not ignored", repo.Record.Name)
		return
	}
	m.status = "select an ignored repo to unignore"
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

func fixActionsForSelection(actions []string) []string {
	if len(actions) == 0 {
		return nil
	}
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		if action == FixActionEnableAutoPush {
			continue
		}
		out = append(out, action)
	}
	sort.SliceStable(out, func(i, j int) bool {
		li := fixActionSelectionPriority(out[i])
		lj := fixActionSelectionPriority(out[j])
		if li != lj {
			return li < lj
		}
		return out[i] < out[j]
	})
	return out
}

func fixActionsForAllExecution(actions []string) []string {
	if len(actions) == 0 {
		return nil
	}
	stageCommitSelected := containsAction(actions, FixActionStageCommitPush)
	seen := make(map[string]struct{}, len(actions))
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		if stageCommitSelected && (action == FixActionPush || action == FixActionSetUpstreamPush) {
			continue
		}
		out = append(out, action)
	}
	return out
}

func fixActionSelectionPriority(action string) int {
	switch action {
	case FixActionAbortOperation:
		return 10
	case FixActionSyncWithUpstream:
		return 20
	case FixActionPullFFOnly:
		return 30
	case FixActionStageCommitPush:
		return 40
	case FixActionCreateProject:
		return 50
	case FixActionSetUpstreamPush:
		return 60
	case FixActionPush:
		return 70
	case FixActionForkAndRetarget:
		return 80
	default:
		return 100
	}
}

func fixListColumnsForWidth(listWidth int) fixColumnLayout {
	const (
		repoMin    = 7
		branchMin  = 7
		stateMin   = 10
		autoMin    = 9
		reasonsMin = 12
		actionMin  = 10
	)

	layout := fixColumnLayout{
		Repo:    repoMin,
		Branch:  branchMin,
		State:   stateMin,
		Auto:    autoMin,
		Reasons: reasonsMin,
		Action:  actionMin,
	}

	minTotal := repoMin + branchMin + stateMin + autoMin + reasonsMin + actionMin
	budget := listWidth - fixListReservedCols
	if budget < minTotal {
		budget = minTotal
	}
	extra := budget - minTotal
	if extra > 0 {
		repoExtra := extra * 14 / 100
		branchExtra := extra * 14 / 100
		stateExtra := extra * 8 / 100
		autoExtra := extra * 8 / 100
		reasonsExtra := extra * 28 / 100
		actionExtra := extra - repoExtra - branchExtra - stateExtra - autoExtra - reasonsExtra
		layout.Repo += repoExtra
		layout.Branch += branchExtra
		layout.State += stateExtra
		layout.Auto += autoExtra
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
	case FixActionCreateProject:
		return fixActionCreateProjectStyle
	case FixActionForkAndRetarget:
		return fixActionForkStyle
	case FixActionSyncWithUpstream:
		return fixActionSyncStyle
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
	}
	if spec, ok := fixActionSpecFor(action); ok {
		return spec.Label
	}
	return action
}

func fixActionDescription(action string) string {
	switch action {
	case fixNoAction:
		return "Do nothing for this repository in the current run."
	case fixAllActions:
		return "Run all currently eligible fixes for this repository."
	}
	if spec, ok := fixActionSpecFor(action); ok {
		return spec.Description
	}
	return "Action has no help text yet."
}

func formatRepoDisplayName(repo fixRepoState) string {
	name := repo.Record.Name
	if name == "" {
		name = filepathBaseFallback(repo.Record.Path)
	}
	if url := githubRepoURLForRecord(repo.Record); url != "" {
		return osc8Link(name, url)
	}
	return name
}

func githubRepoURLForRecord(rec domain.MachineRepoRecord) string {
	originURL := strings.TrimSpace(rec.OriginURL)
	if originURL == "" {
		return ""
	}
	originIdentity, err := domain.NormalizeOriginIdentity(originURL)
	if err != nil {
		return ""
	}
	host, path, ok := strings.Cut(originIdentity, "/")
	if !ok || strings.TrimSpace(path) == "" {
		return ""
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if strings.HasSuffix(host, ".github.com") {
		host = "github.com"
	}
	if host != "github.com" {
		return ""
	}
	return "https://" + host + "/" + path
}

func osc8Link(label, target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return label
	}
	const esc = "\x1b"
	return esc + "]8;;" + target + esc + "\\" + label + esc + "]8;;" + esc + "\\"
}

func filepathBaseFallback(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "repo"
	}
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return path
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" {
		return path
	}
	return last
}

func repoMetaAutoPushMode(meta *domain.RepoMetadataFile) domain.AutoPushMode {
	if meta == nil {
		return domain.AutoPushModeDisabled
	}
	return domain.NormalizeAutoPushMode(meta.AutoPush)
}

func repoMetaAllowsAutoPush(meta *domain.RepoMetadataFile) bool {
	if meta == nil {
		return true
	}
	return pushAccessAllowsAutoPush(meta.PushAccess)
}

func classifyFixRepo(repo fixRepoState, actions []string) fixRepoTier {
	if repo.Record.Syncable {
		return fixRepoTierSyncable
	}
	if unsyncableReasonsFullyCoverable(repo.Record.UnsyncableReasons, fixActionsForSelection(actions)) {
		return fixRepoTierAutofixable
	}
	return fixRepoTierUnsyncableBlocked
}

func unsyncableReasonsFullyCoverable(reasons []domain.UnsyncableReason, actions []string) bool {
	if len(reasons) == 0 || len(actions) == 0 {
		return false
	}
	has := fixActionSet(actions)
	for _, reason := range reasons {
		if !fixReasonCoveredByActions(reason, has) {
			return false
		}
	}
	return true
}

func uncoveredUnsyncableReasons(reasons []domain.UnsyncableReason, actions []string) []domain.UnsyncableReason {
	if len(reasons) == 0 {
		return nil
	}
	has := fixActionSet(actions)
	uncovered := make([]domain.UnsyncableReason, 0, len(reasons))
	for _, reason := range reasons {
		if fixReasonCoveredByActions(reason, has) {
			continue
		}
		uncovered = append(uncovered, reason)
	}
	return uncovered
}

func fixActionSet(actions []string) map[string]bool {
	has := map[string]bool{}
	for _, action := range actions {
		has[action] = true
	}
	return has
}

func fixReasonCoveredByActions(reason domain.UnsyncableReason, has map[string]bool) bool {
	switch reason {
	case domain.ReasonMissingOrigin:
		return has[FixActionCreateProject]
	case domain.ReasonOperationInProgress:
		return has[FixActionAbortOperation]
	case domain.ReasonDirtyTracked, domain.ReasonDirtyUntracked:
		return has[FixActionStageCommitPush]
	case domain.ReasonMissingUpstream:
		return has[FixActionSetUpstreamPush] || has[FixActionStageCommitPush] || has[FixActionCreateProject]
	case domain.ReasonDiverged:
		return has[FixActionSyncWithUpstream]
	case domain.ReasonPushPolicyBlocked:
		return has[FixActionPush] || has[FixActionStageCommitPush] || has[FixActionSetUpstreamPush] || has[FixActionCreateProject]
	case domain.ReasonPushFailed:
		return has[FixActionPush] || has[FixActionStageCommitPush] || has[FixActionSetUpstreamPush] || has[FixActionCreateProject]
	case domain.ReasonSyncConflict:
		return false
	case domain.ReasonSyncProbeFailed:
		return false
	case domain.ReasonPushAccessBlocked:
		return has[FixActionForkAndRetarget]
	case domain.ReasonPullFailed:
		return has[FixActionPullFFOnly]
	default:
		return false
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
