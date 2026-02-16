package app

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
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
	Toggle   key.Binding
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
		Toggle: key.NewBinding(
			key.WithKeys(" ", "space"),
			key.WithHelp("space", "select fix"),
		),
		Apply: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "run selected"),
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
			key.WithHelp("ctrl+a", "run all selected"),
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
	return []key.Binding{k.Up, k.Left, k.Toggle, k.Apply, k.ApplyAll, k.Quit}
}

func (k fixTUIKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right, k.Toggle},
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
		canApplySelected = len(m.scheduledActionsForRepo(repo.Record.Path, options)) > 0
	}
	canApplyCurrent := hasRepo && !m.ignored[repo.Record.Path] && len(options) > 0
	canApplyAll := m.hasAnySelectedFixes()
	canMoveRepo := len(m.visible) > 1
	canChangeFix := len(options) > 0
	canToggleSchedule := hasRepo && !m.ignored[repo.Record.Path] && len(options) > 0
	canToggleAutoPush := hasRepo && !m.ignored[repo.Record.Path] && repoMetaAllowsAutoPush(repo.Meta)
	canIgnoreRepo := hasRepo && !m.ignored[repo.Record.Path]
	canUnignoreRepo := hasRepo && m.ignored[repo.Record.Path]

	if canApplyCurrent {
		desc := "run current"
		if canApplySelected {
			desc = "run selected/current"
		}
		b := newHelpBinding([]string{"enter"}, "enter", desc)
		short = append(short, b)
		primary = append(primary, b)
	}
	if canApplyAll {
		b := newHelpBinding([]string{"ctrl+a"}, "ctrl+a", "run all selected")
		short = append(short, b)
		primary = append(primary, b)
	}
	if canMoveRepo {
		b := newHelpBinding([]string{"up", "down"}, "↑/↓", "move repo")
		short = append(short, b)
		primary = append(primary, b)
	}
	if canChangeFix {
		b := newHelpBinding([]string{"left", "right"}, "←/→", "choose fix")
		short = append(short, b)
		primary = append(primary, b)
	}
	if canToggleSchedule {
		b := newHelpBinding([]string{"space"}, "space", "select/unselect")
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
	if m.wizard.FocusArea == fixWizardFocusCommit && m.wizard.CommitButtonFocused {
		enterDesc = "generate message"
	}
	if m.wizard.FocusArea == fixWizardFocusActions && m.wizard.ActionFocus == fixWizardActionApply && m.wizardNeedsReviewBeforeApply() {
		enterDesc = "review first"
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
	case fixWizardFocusCommit:
		b := newHelpBinding([]string{"left", "right"}, "←/→", "input/generate")
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
	if m.wizardHasVisualDiffButton() {
		diff := newHelpBinding([]string{"alt+v"}, m.visualDiffShortcutDisplayLabel(), "visual diff")
		short = append(short, diff)
		secondary = append(secondary, diff)
	}

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
		actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
			Interactive:     true,
			Risk:            repo.Risk,
			SyncStrategy:    FixSyncStrategyRebase,
			SyncFeasibility: repo.SyncFeasibility,
		})
		options := selectableFixActions(fixActionsForSelection(actions))
		if len(m.scheduledActionsForRepo(repo.Record.Path, options)) > 0 {
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
	app                     *App
	includeCatalogs         []string
	loadReposFn             func(includeCatalogs []string, refreshMode scanRefreshMode) ([]fixRepoState, error)
	execProcessFn           func(c *exec.Cmd, fn tea.ExecCallback) tea.Cmd
	generateCommitMessageFn func(repoPath string) (string, error)
	prepareVisualDiffCmdFn  func(repoPath string, args []string) (*exec.Cmd, func(error) error, error)
	repos                   []fixRepoState
	visible                 []fixRepoState
	ignored                 map[string]bool
	actionCursor            map[string]int
	scheduled               map[string][]string
	rowRepoIndex            []int
	repoIndexToRow          []int

	keys fixTUIKeyMap
	help help.Model

	repoList list.Model

	width  int
	height int

	status  string
	errText string

	listDetailsTargetHeight int

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
	Kind               fixListItemKind
	Catalog            string
	NamePlain          string
	Path               string
	Name               string
	Branch             string
	State              string
	AutoPushMode       domain.AutoPushMode
	AutoPushAvailable  bool
	Reasons            string
	CurrentAction      string
	HasMultipleChoices bool
	HasLeftChoice      bool
	HasRightChoice     bool
	ScheduledActions   []string
	Tier               fixRepoTier
	Ignored            bool
}

type fixListItemKind int

const (
	fixListItemRepo fixListItemKind = iota
	fixListItemCatalogHeader
	fixListItemCatalogBreak
)

func (i fixListItem) FilterValue() string {
	return i.NamePlain + " " + i.Path + " " + i.Branch + " " + i.Reasons + " " + fixScheduledPlainText(i.ScheduledActions) + " " + fixActionLabel(i.CurrentAction) + " " + i.Catalog
}

type fixColumnLayout struct {
	Repo        int
	Branch      int
	State       int
	Auto        int
	Reasons     int
	SelectFixes int
}

type fixRepoTier int

const (
	fixRepoTierAutofixable fixRepoTier = iota
	fixRepoTierUnsyncableBlocked
	fixRepoTierNotCloned
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
	contentWidth := fixListContentWidth(m.Width())
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
	case fixRepoTierNotCloned:
		stateStyle = fixStateNotClonedStyle
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
	selectFixesCell := renderFixSelectFixesCell(
		row.ScheduledActions,
		row.CurrentAction,
		row.HasMultipleChoices,
		row.HasLeftChoice,
		row.HasRightChoice,
		layout.SelectFixes,
		selected,
		row.Ignored,
	)

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
		selectFixesCell,
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
const fixTUISubtitle = "Interactive remediation for unsyncable repositories"

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

	fixStateNotClonedStyle = lipgloss.NewStyle().
				Foreground(warningColor)

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
				Bold(true).
				Align(lipgloss.Left)

	fixDetailsLabelStyle = lipgloss.NewStyle().
				Foreground(accentColor).
				Bold(true)

	fixDetailsValueStyle = lipgloss.NewStyle().
				Foreground(textColor).
				Bold(true)

	fixDetailsPathStyle = lipgloss.NewStyle().
				Foreground(mutedTextColor)

	fixDetailsStateKeyStyle = lipgloss.NewStyle().
				Foreground(accentColor).
				Bold(true)

	fixDetailsAutoPushKeyStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("81")).
					Bold(true)

	fixDetailsBranchKeyStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("111")).
					Bold(true)

	fixDetailsReasonsKeyStyle = lipgloss.NewStyle().
					Foreground(warningColor).
					Bold(true)

	fixDetailsSelectedKeyStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("177")).
					Bold(true)

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

	fixActionCloneStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("120"))

	fixActionForkStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("99"))

	fixActionSyncStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("111"))

	fixActionAutoPushStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("70"))

	fixActionAbortStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("203"))

	fixChoiceArrowAvailableStyle = lipgloss.NewStyle().
					Foreground(textColor).
					Bold(true)

	fixChoiceArrowDimStyle = lipgloss.NewStyle().
				Foreground(mutedTextColor).
				Faint(true)

	fixChoiceChipStyle = lipgloss.NewStyle().
				Background(lipgloss.AdaptiveColor{Light: "#F6F8FA", Dark: "#30363D"}).
				Padding(0, 1)

	fixAutoPushOnStyle = lipgloss.NewStyle().
				Foreground(successColor)

	fixAutoPushOffStyle = lipgloss.NewStyle().
				Foreground(mutedTextColor)

	fixAutoPushIncludeDefaultStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("81"))

	fixPageStyle = lipgloss.NewStyle().Padding(0, 2)

	fixTopBorderStyle = lipgloss.NewStyle().
				Foreground(borderColor)
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
	subtitle := hintStyle.Render(fixTUISubtitle)

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
	contentWidth := m.width - fixPageStyle.GetHorizontalFrameSize()
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
		loadReposFn:              app.loadFixReposForInteractive,
		execProcessFn:            tea.ExecProcess,
		generateCommitMessageFn:  app.generateLumenCommitMessage,
		prepareVisualDiffCmdFn:   app.prepareLumenDiffExecCommand,
		ignored:                  map[string]bool{},
		actionCursor:             map[string]int{},
		scheduled:                map[string][]string{},
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
		if m.viewMode == fixViewWizard && m.wizard.CommitGenerating {
			var cmd tea.Cmd
			m.wizard.CommitGenerateSpinner, cmd = m.wizard.CommitGenerateSpinner.Update(msg)
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
	case fixWizardCommitGeneratedMsg:
		return m, m.handleWizardCommitGenerated(msg)
	case fixWizardDiffCompletedMsg:
		return m, m.handleWizardDiffCompleted(msg)
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
		case key.Matches(msg, m.keys.Toggle):
			m.toggleCurrentActionScheduled()
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

	helpPanel := helpPanelStyle
	if w := m.viewContentWidth(); w > 0 {
		helpPanel = helpPanel.Width(w)
	}
	helpView := m.footerHelpView(helpPanel)
	helpBlock := helpPanel.Render(helpView)

	body := m.viewBodyForMode(m.viewMode)
	return fixPageStyle.Render(body + "\n" + helpBlock)
}

func (m *fixTUIModel) viewBodyForMode(mode fixViewMode) string {
	var b strings.Builder
	topChrome := m.viewTopChromeForMode(mode)
	if topChrome != "" {
		b.WriteString(topChrome)
	}
	contentPanel := m.mainContentPanelStyle()
	content := m.viewContentForMode(mode)
	if mode == fixViewList {
		b.WriteString(renderPanelWithTopTitle(contentPanel, m.viewListPanelTitle(), content))
	} else {
		b.WriteString(contentPanel.Render(content))
	}
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(hintStyle.Render(m.status))
	}
	if m.errText != "" {
		b.WriteString("\n")
		b.WriteString(alertStyle.Render(errorStyle.Render(m.errText)))
	}
	return b.String()
}

func (m *fixTUIModel) viewTopChromeForMode(mode fixViewMode) string {
	var b strings.Builder
	if mode == fixViewWizard {
		b.WriteString(m.viewWizardTopLine())
		b.WriteString("\n")
		return b.String()
	}
	if mode == fixViewList {
		return ""
	}

	title := lipgloss.JoinHorizontal(lipgloss.Center,
		titleBadgeStyle.Render("bb"),
		" "+headerStyle.Render("fix"),
	)
	subtitle := hintStyle.Render(fixTUISubtitle)
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(subtitle)
	b.WriteString("\n\n")
	return b.String()
}

func (m *fixTUIModel) viewContentForMode(mode fixViewMode) string {
	switch mode {
	case fixViewWizard:
		return m.viewWizardContent()
	case fixViewSummary:
		return m.viewSummaryContent()
	default:
		return m.viewMainContent()
	}
}

func (m *fixTUIModel) viewListPanelTitle() string {
	title := lipgloss.JoinHorizontal(lipgloss.Center,
		titleBadgeStyle.Render("bb"),
		" "+headerStyle.Render("fix"),
	)
	subtitle := hintStyle.Render("·")
	line := lipgloss.JoinHorizontal(lipgloss.Center,
		title,
		" ",
		subtitle,
		" ",
		hintStyle.Render(fixTUISubtitle),
	)
	if m.immediateApplying {
		line = lipgloss.JoinHorizontal(lipgloss.Center, line, " ", m.immediateApplySpinner.View())
	} else if m.revalidating {
		line = lipgloss.JoinHorizontal(lipgloss.Center, line, " ", m.revalidateSpinner.View())
	}
	return line
}

func renderPanelWithTopTitle(panel lipgloss.Style, title string, content string) string {
	withoutTop := panel.Border(lipgloss.RoundedBorder(), false, true, true, true)
	rendered := withoutTop.Render(content)
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 {
		return rendered
	}
	totalWidth := lipgloss.Width(lines[0])
	if totalWidth < 2 {
		return rendered
	}

	border := lipgloss.RoundedBorder()
	available := totalWidth - 2
	plainTitle := strings.TrimSpace(title)
	if plainTitle == "" {
		plainTitle = "bb fix"
	}
	titleText := " " + ansi.Truncate(plainTitle, max(1, available-2), "…") + " "
	titleWidth := lipgloss.Width(titleText)

	prefix := border.Top
	prefixWidth := lipgloss.Width(prefix)
	if prefixWidth+titleWidth > available {
		prefix = ""
		prefixWidth = 0
		titleText = ansi.Truncate(strings.TrimSpace(titleText), max(1, available), "…")
		titleWidth = lipgloss.Width(titleText)
	}

	fillWidth := available - prefixWidth - titleWidth
	if fillWidth < 0 {
		fillWidth = 0
	}
	topLine := fixTopBorderStyle.Render(border.TopLeft) +
		fixTopBorderStyle.Render(prefix) +
		titleText +
		fixTopBorderStyle.Render(strings.Repeat(border.Top, fillWidth)) +
		fixTopBorderStyle.Render(border.TopRight)

	lines[0] = topLine
	return strings.Join(lines, "\n")
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
	content := m.viewMainContentWithoutDetails()
	if len(m.visible) == 0 {
		return content
	}
	if details := m.viewSelectedRepoDetails(); details != "" {
		return content + "\n" + m.padSelectedDetails(details)
	}
	return content
}

func (m *fixTUIModel) padSelectedDetails(details string) string {
	target := m.listDetailsTargetHeight
	if target <= 0 {
		return details
	}
	lines := lipgloss.Height(details)
	if width := m.repoDetailsLineWidth(); width > 0 {
		lines = lipgloss.Height(lipgloss.NewStyle().Width(width).Render(details))
	}
	if lines >= target {
		return details
	}
	return details + strings.Repeat("\n", target-lines)
}

func (m *fixTUIModel) viewMainContentWithoutDetails() string {
	var b strings.Builder
	b.WriteString(hintStyle.Render("Choose eligible fixes with ←/→, press space to select or unselect."))
	if m.immediateApplying {
		b.WriteString("\n")
		b.WriteString(hintStyle.Render(m.immediateApplyStatusLine()))
	}
	b.WriteString("\n")
	b.WriteString(m.viewFixSummary())
	b.WriteString("\n")

	if len(m.visible) == 0 {
		b.WriteString(fieldStyle.Render("No repositories available for interactive fix right now."))
	} else {
		// Render list directly inside the main panel to avoid nested border/frame
		// width interactions that can trigger hard wrapping artifacts.
		b.WriteString(m.viewRepoList())
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
	if sticky := m.viewStickyCatalogHeader(); sticky != "" {
		b.WriteString(sticky)
		b.WriteString("\n")
	}
	b.WriteString(m.repoList.View())
	return b.String()
}

func (m *fixTUIModel) viewStickyCatalogHeader() string {
	label, ok := m.stickyCatalogLabelForViewport()
	if !ok || strings.TrimSpace(label) == "" {
		return ""
	}
	return fixCatalogHeaderStyle.Render(label)
}

func (m *fixTUIModel) stickyCatalogLabelForViewport() (string, bool) {
	items := m.repoList.Items()
	if len(items) == 0 {
		return "", false
	}
	start, end := m.repoList.Paginator.GetSliceBounds(len(items))
	if start < 0 || start >= len(items) || end <= start {
		return "", false
	}
	if end > len(items) {
		end = len(items)
	}

	firstRepo := -1
	firstHeader := -1
	for i := start; i < end; i++ {
		row, ok := items[i].(fixListItem)
		if !ok {
			continue
		}
		if row.Kind == fixListItemCatalogHeader && firstHeader == -1 {
			firstHeader = i
		}
		if row.Kind == fixListItemRepo {
			firstRepo = i
			break
		}
	}
	if firstRepo < 0 {
		return "", false
	}
	// When a catalog header is already visible before the first repo row on this
	// page, do not render an extra sticky line.
	if firstHeader >= 0 && firstHeader < firstRepo {
		return "", false
	}

	for i := firstRepo; i >= 0; i-- {
		row, ok := items[i].(fixListItem)
		if !ok {
			continue
		}
		if row.Kind != fixListItemCatalogHeader {
			continue
		}
		name := strings.TrimSpace(row.Name)
		if name != "" {
			return name, true
		}
		break
	}

	row, ok := items[firstRepo].(fixListItem)
	if !ok {
		return "", false
	}
	catalog := strings.TrimSpace(row.Catalog)
	if catalog == "" {
		return "", false
	}
	return fmt.Sprintf("Catalog: %s", catalog), true
}

func (m *fixTUIModel) viewRepoHeader() string {
	listWidth := m.repoList.Width()
	if listWidth <= 0 {
		listWidth = fixListDefaultWidth
	}
	layout := fixListColumnsForWidth(fixListContentWidth(listWidth))

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
		renderFixColumnCell("Select Fixes", layout.SelectFixes, fixHeaderCellStyle),
	)
	// Match row rendering width: two-char indicator plus one guard column.
	return "  " + header
}

func (m *fixTUIModel) viewSelectedRepoDetails() string {
	repo, ok := m.currentRepo()
	if !ok {
		return ""
	}
	return m.viewRepoDetails(repo)
}

func (m *fixTUIModel) viewRepoDetails(repo fixRepoState) string {
	actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
		Interactive:     true,
		Risk:            repo.Risk,
		SyncStrategy:    FixSyncStrategyRebase,
		SyncFeasibility: repo.SyncFeasibility,
	})
	options := selectableFixActions(fixActionsForSelection(actions))
	action := m.currentActionForRepo(repo.Record.Path, options)
	scheduled := m.scheduledActionsForRepo(repo.Record.Path, options)
	ignored := m.ignored[repo.Record.Path]

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
		switch classifyFixRepo(repo, actions) {
		case fixRepoTierAutofixable:
			state = "fixable"
			stateStyle = fixStateAutofixableStyle
		case fixRepoTierUnsyncableBlocked:
			state = "unsyncable"
			stateStyle = fixStateUnsyncableStyle
		case fixRepoTierNotCloned:
			state = "not cloned"
			stateStyle = fixStateNotClonedStyle
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
	separator := hintStyle.Render(" · ")
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
	segments := []string{
		fixDetailsStateKeyStyle.Render("State:") + " " + stateStyle.Render(state),
		fixDetailsAutoPushKeyStyle.Render("Auto-push:") + " " + autoPushValueStyle.Render(autoPushValue),
		fixDetailsBranchKeyStyle.Render("Branch:") + " " + fixBranchStyle.Render(repo.Record.Branch),
		fixDetailsReasonsKeyStyle.Render("Reasons:") + " " + fixReasonsStyle.Render(reasonText),
		fixDetailsSelectedKeyStyle.Render("Selected fixes:") + " " + renderScheduledDetails(scheduled, ignored),
	}
	for i, line := range wrapStyledSegments(segments, separator, m.repoDetailsLineWidth()) {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(line)
	}
	b.WriteString("\n")
	b.WriteString(fixDetailsLabelStyle.Render("Action: ") + hintStyle.Render(fixActionDescription(action)))
	return b.String()
}

func (m *fixTUIModel) viewFixSummary() string {
	total := len(m.visible)
	pending := 0
	fixable := 0
	blocked := 0
	notCloned := 0
	syncable := 0
	ignored := 0
	for _, repo := range m.visible {
		actions := eligibleFixActions(repo.Record, repo.Meta, fixEligibilityContext{
			Interactive:     true,
			Risk:            repo.Risk,
			SyncStrategy:    FixSyncStrategyRebase,
			SyncFeasibility: repo.SyncFeasibility,
		})
		options := selectableFixActions(fixActionsForSelection(actions))
		if !m.ignored[repo.Record.Path] {
			pending += len(m.scheduledActionsForRepo(repo.Record.Path, options))
		}
		if m.ignored[repo.Record.Path] {
			ignored++
			continue
		}
		tier := classifyFixRepo(repo, actions)
		switch tier {
		case fixRepoTierAutofixable:
			fixable++
		case fixRepoTierUnsyncableBlocked:
			blocked++
		case fixRepoTierNotCloned:
			notCloned++
		case fixRepoTierSyncable:
			syncable++
		}
	}
	totalStyle := renderStatusPill(fmt.Sprintf("%d repos", total))
	pendingStyle := renderStatusPill(fmt.Sprintf("%d selected", pending))
	autoStyle := renderFixSummaryPill(fmt.Sprintf("%d fixable", fixable), lipgloss.Color("214"))
	blockedStyle := renderFixSummaryPill(fmt.Sprintf("%d unsyncable", blocked), errorFgColor)
	notClonedStyle := renderFixSummaryPill(fmt.Sprintf("%d not cloned", notCloned), warningColor)
	syncStyle := renderFixSummaryPill(fmt.Sprintf("%d syncable", syncable), successColor)
	ignoredStyle := renderFixSummaryPill(fmt.Sprintf("%d ignored", ignored), mutedTextColor)
	pills := lipgloss.JoinHorizontal(lipgloss.Top, totalStyle, " ", pendingStyle, "  ", autoStyle, " ", blockedStyle, " ", notClonedStyle, " ", syncStyle, " ", ignoredStyle)
	summaryWidth := m.repoDetailsLineWidth()
	if summaryWidth <= 0 || lipgloss.Width(pills) <= summaryWidth {
		return pills
	}
	return m.viewFixSummaryCompactText(total, pending, fixable, blocked, notCloned, syncable, ignored, summaryWidth)
}

func (m *fixTUIModel) viewFixSummaryCompactText(total int, pending int, fixable int, blocked int, notCloned int, syncable int, ignored int, width int) string {
	segments := []string{
		fmt.Sprintf("repos: %d", total),
		fmt.Sprintf("selected: %d", pending),
		fmt.Sprintf("fixable: %d", fixable),
		fmt.Sprintf("unsyncable: %d", blocked),
		fmt.Sprintf("not cloned: %d", notCloned),
		fmt.Sprintf("syncable: %d", syncable),
		fmt.Sprintf("ignored: %d", ignored),
	}
	separator := hintStyle.Render(" · ")
	lines := wrapStyledSegments(segments, separator, width)
	return strings.Join(lines, "\n")
}

func (m *fixTUIModel) viewContentWidth() int {
	if m.width <= 0 {
		return 0
	}
	contentWidth := m.width - fixPageStyle.GetHorizontalFrameSize()
	if contentWidth < 52 {
		return 0
	}
	return contentWidth
}

func (m *fixTUIModel) repoDetailsLineWidth() int {
	if w := m.viewContentWidth(); w > 0 {
		inner := w - panelStyle.GetHorizontalFrameSize()
		if inner > 0 {
			return inner
		}
	}
	return 0
}

func wrapStyledSegments(segments []string, separator string, width int) []string {
	if len(segments) == 0 {
		return nil
	}
	if width <= 0 {
		return []string{strings.Join(segments, separator)}
	}

	lines := make([]string, 0, len(segments))
	current := ""
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		if current == "" {
			current = segment
			continue
		}
		candidate := current + separator + segment
		if ansi.StringWidth(candidate) <= width {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = segment
	}
	if strings.TrimSpace(current) != "" {
		lines = append(lines, current)
	}
	return lines
}

func (m *fixTUIModel) maxRepoDetailsHeight(innerWidth int) int {
	if innerWidth <= 0 {
		return 0
	}
	renderer := lipgloss.NewStyle().Width(innerWidth)
	maxHeight := 0
	for _, repo := range m.visible {
		details := m.viewRepoDetails(repo)
		rendered := lipgloss.Height(renderer.Render(details))
		if rendered > maxHeight {
			maxHeight = rendered
		}
	}
	return maxHeight
}

func (m *fixTUIModel) listViewRenderedHeight(helpBlock string) int {
	body := m.viewBodyForMode(fixViewList)
	return lipgloss.Height(fixPageStyle.Render(body + "\n" + helpBlock))
}

func (m *fixTUIModel) resizeRepoList() {
	if m.height <= 0 {
		return
	}
	m.listDetailsTargetHeight = 0

	listWidth := fixListDefaultWidth
	if w := m.viewContentWidth(); w > 0 {
		listWidth = w - panelStyle.GetHorizontalFrameSize()
	}
	if listWidth < 52 {
		listWidth = 52
	}
	// Keep width in sync for all view modes; list height only matters in list mode.
	if m.viewMode != fixViewList {
		height := m.repoList.Height()
		if height < 6 {
			height = 6
		}
		m.repoList.SetSize(listWidth, height)
		return
	}

	helpPanel := helpPanelStyle
	if w := m.viewContentWidth(); w > 0 {
		helpPanel = helpPanel.Width(w)
	}
	helpBlock := helpPanel.Render(m.footerHelpView(helpPanel))

	const (
		preferredMinListHeight = 6
		hardMinListHeight      = 1
	)

	detailsTargetHeight := 0
	if len(m.visible) > 0 {
		detailsWidth := m.repoDetailsLineWidth()
		detailsTargetHeight = m.maxRepoDetailsHeight(detailsWidth)
	}
	m.listDetailsTargetHeight = detailsTargetHeight

	minListHeight := preferredMinListHeight
	if detailsTargetHeight > 0 {
		minListHeight = hardMinListHeight
	}

	maxListHeight := m.height
	if maxListHeight < minListHeight {
		maxListHeight = minListHeight
	}

	chosenHeight := minListHeight
	for height := maxListHeight; height >= minListHeight; height-- {
		m.repoList.SetSize(listWidth, height)
		if m.listViewRenderedHeight(helpBlock) <= m.height {
			chosenHeight = height
			break
		}
	}
	m.repoList.SetSize(listWidth, chosenHeight)

	if len(m.visible) > 0 {
		if gap := m.height - m.listViewRenderedHeight(helpBlock); gap > 0 {
			m.listDetailsTargetHeight = detailsTargetHeight + gap
		}
	}
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
		if len(options) == 0 {
			m.actionCursor[path] = -1
			delete(m.scheduled, path)
		} else {
			if idx, ok := m.actionCursor[path]; !ok || idx < 0 || idx >= len(options) {
				m.actionCursor[path] = 0
			}
			normalized := normalizeScheduledFixes(options, m.scheduled[path])
			if len(normalized) == 0 {
				delete(m.scheduled, path)
			} else {
				m.scheduled[path] = normalized
			}
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
		case fixRepoTierNotCloned:
			state = "not cloned"
		}
		ignored := m.ignored[path]
		if ignored {
			state = "ignored"
		}

		options := selectableFixActions(fixActionsForSelection(entry.actions))
		currentAction := m.currentActionForRepo(path, options)
		cursor := m.actionCursor[path]
		if cursor < 0 || cursor >= len(options) {
			cursor = 0
		}
		scheduled := m.scheduledActionsForRepo(path, options)
		repoIndex := len(m.visible)
		items = append(items, fixListItem{
			Kind:               fixListItemRepo,
			Catalog:            repo.Record.Catalog,
			NamePlain:          repo.Record.Name,
			Path:               path,
			Name:               formatRepoDisplayName(repo),
			Branch:             repo.Record.Branch,
			State:              state,
			AutoPushMode:       repoMetaAutoPushMode(repo.Meta),
			AutoPushAvailable:  repoMetaAllowsAutoPush(repo.Meta),
			Reasons:            reasons,
			CurrentAction:      currentAction,
			HasMultipleChoices: len(options) > 1,
			HasLeftChoice:      len(options) > 1 && cursor > 0,
			HasRightChoice:     len(options) > 1 && cursor < len(options)-1,
			ScheduledActions:   append([]string(nil), scheduled...),
			Tier:               entry.tier,
			Ignored:            ignored,
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
	if len(options) == 0 {
		m.status = fmt.Sprintf("no eligible fixes available for %s", repo.Record.Name)
		return
	}
	key := repo.Record.Path
	pos := m.actionCursor[key]
	if pos < 0 || pos >= len(options) {
		pos = 0
	}
	pos = (pos + delta) % len(options)
	if pos < 0 {
		pos += len(options)
	}
	m.actionCursor[key] = pos
	selected := options[pos]
	m.status = fmt.Sprintf("current fix for %s: %s (space to select)", repo.Record.Name, fixActionLabel(selected))
	m.rebuildList(repo.Record.Path)
}

func (m *fixTUIModel) toggleCurrentActionScheduled() {
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
	options := selectableFixActions(fixActionsForSelection(actions))
	if len(options) == 0 {
		m.status = fmt.Sprintf("no eligible fixes available for %s", repo.Record.Name)
		return
	}
	current := m.currentActionForRepo(repo.Record.Path, options)
	if current == fixNoAction {
		m.status = fmt.Sprintf("no eligible fixes available for %s", repo.Record.Name)
		return
	}
	scheduled := append([]string(nil), m.scheduledActionsForRepo(repo.Record.Path, options)...)
	wasScheduled := containsAction(scheduled, current)
	if wasScheduled {
		scheduled = removeAction(scheduled, current)
	} else {
		scheduled = append(scheduled, current)
	}
	scheduled = normalizeScheduledFixes(options, scheduled)
	if len(scheduled) == 0 {
		delete(m.scheduled, repo.Record.Path)
	} else {
		m.scheduled[repo.Record.Path] = scheduled
	}
	if containsAction(scheduled, current) {
		m.status = fmt.Sprintf("selected %s for %s", fixActionLabel(current), repo.Record.Name)
	} else if wasScheduled {
		m.status = fmt.Sprintf("unselected %s for %s", fixActionLabel(current), repo.Record.Name)
	} else if len(scheduled) > 0 {
		m.status = fmt.Sprintf("%s superseded by %s for %s", fixActionLabel(current), fixActionLabel(scheduled[0]), repo.Record.Name)
	} else {
		m.status = fmt.Sprintf("no fixes selected for %s", repo.Record.Name)
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
	options := selectableFixActions(fixActionsForSelection(actions))
	selections := m.scheduledActionsForRepo(repo.Record.Path, options)
	if len(selections) == 0 {
		current := m.currentActionForRepo(repo.Record.Path, options)
		if current == fixNoAction {
			m.status = fmt.Sprintf("no eligible fixes available for %s", repo.Record.Name)
			return
		}
		selections = []string{current}
	}
	m.resetSummaryFollowUpState()
	m.summaryResults = nil

	queue := make([]fixWizardDecision, 0, 2)
	immediateTasks := make([]fixImmediateActionTask, 0, 2)
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
		options := selectableFixActions(fixActionsForSelection(actions))
		selections := m.scheduledActionsForRepo(repo.Record.Path, options)
		if len(selections) == 0 {
			skipped++
			continue
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
	if len(options) == 0 {
		return fixNoAction
	}
	idx := m.actionCursor[path]
	if idx < 0 || idx >= len(options) {
		idx = 0
	}
	return options[idx]
}

func selectableFixActions(actions []string) []string {
	return append([]string(nil), actions...)
}

func (m *fixTUIModel) scheduledActionsForRepo(path string, eligible []string) []string {
	if len(eligible) == 0 {
		return nil
	}
	return normalizeScheduledFixes(eligible, m.scheduled[path])
}

func normalizeScheduledFixes(eligible []string, scheduled []string) []string {
	if len(eligible) == 0 || len(scheduled) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(eligible))
	for _, action := range eligible {
		allowed[action] = struct{}{}
	}
	filtered := make([]string, 0, len(scheduled))
	seen := make(map[string]struct{}, len(scheduled))
	for _, action := range scheduled {
		if _, ok := allowed[action]; !ok {
			continue
		}
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		filtered = append(filtered, action)
	}
	if len(filtered) == 0 {
		return nil
	}
	return fixActionsForAllExecution(fixActionsForSelection(filtered))
}

func removeAction(actions []string, target string) []string {
	if len(actions) == 0 {
		return nil
	}
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		if action == target {
			continue
		}
		out = append(out, action)
	}
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
	publishNewBranchSelected := containsAction(actions, FixActionPublishNewBranch)
	checkpointThenSyncSelected := containsAction(actions, FixActionCheckpointThenSync)
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
		if publishNewBranchSelected && (action == FixActionPush || action == FixActionSetUpstreamPush || action == FixActionStageCommitPush) {
			continue
		}
		if checkpointThenSyncSelected && (action == FixActionPush || action == FixActionSetUpstreamPush || action == FixActionSyncWithUpstream || action == FixActionStageCommitPush) {
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
	case FixActionCheckpointThenSync:
		return 35
	case FixActionPublishNewBranch:
		return 38
	case FixActionStageCommitPush:
		return 40
	case FixActionClone:
		return 45
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

func fixListContentWidth(listWidth int) int {
	contentWidth := listWidth - 5 // indicator + wrap guard
	if contentWidth < 50 {
		contentWidth = 50
	}
	return contentWidth
}

func fixListColumnsForWidth(listWidth int) fixColumnLayout {
	const (
		repoMin        = 7
		branchMin      = 7
		stateMin       = 10
		autoMin        = 9
		reasonsMin     = 12
		selectFixesMin = 20
	)

	layout := fixColumnLayout{
		Repo:        repoMin,
		Branch:      branchMin,
		State:       stateMin,
		Auto:        autoMin,
		Reasons:     reasonsMin,
		SelectFixes: selectFixesMin,
	}

	minTotal := repoMin + branchMin + stateMin + autoMin + reasonsMin + selectFixesMin
	budget := listWidth - fixListReservedCols
	if budget < minTotal {
		budget = minTotal
	}
	extra := budget - minTotal
	if extra > 0 {
		repoExtra := extra * 12 / 100
		branchExtra := extra * 12 / 100
		stateExtra := extra * 7 / 100
		autoExtra := extra * 7 / 100
		reasonsExtra := extra * 24 / 100
		selectFixesExtra := extra - repoExtra - branchExtra - stateExtra - autoExtra - reasonsExtra
		layout.Repo += repoExtra
		layout.Branch += branchExtra
		layout.State += stateExtra
		layout.Auto += autoExtra
		layout.Reasons += reasonsExtra
		layout.SelectFixes += selectFixesExtra
	}
	return layout
}

func renderFixColumnCell(value string, width int, style lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	return style.Width(width).MaxWidth(width).Render(ansi.Truncate(value, width, "…"))
}

func renderFixSelectFixesCell(actions []string, current string, hasChoices bool, hasLeft bool, hasRight bool, width int, selected bool, ignored bool) string {
	squares := renderScheduledSquares(actions, ignored, selected)
	if !selected {
		return renderFixColumnCell(squares, width, lipgloss.NewStyle())
	}
	chip := renderCurrentChoiceChip(current, ignored)
	parts := []string{squares, chip}
	if hasChoices {
		left := fixChoiceArrowStyle(hasLeft).Render("←")
		right := fixChoiceArrowStyle(hasRight).Render("→")
		parts = []string{squares, left, chip, right}
	}
	cell := strings.TrimSpace(strings.Join(parts, " "))
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(ansi.Truncate(cell, width, "…"))
}

func renderScheduledSquares(actions []string, ignored bool, selectedRow bool) string {
	if len(actions) == 0 {
		style := fixNoActionStyle
		if ignored {
			style = style.Copy().Faint(true)
		}
		if selectedRow {
			style = style.Copy().Bold(true)
		}
		return style.Render(fixNoAction)
	}
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		style := fixActionStyleFor(action)
		if ignored {
			style = style.Copy().Foreground(mutedTextColor).Faint(true)
		}
		if selectedRow {
			style = style.Copy().Bold(true)
		}
		parts = append(parts, style.Render("■"))
	}
	return strings.Join(parts, " ")
}

func renderCurrentChoiceChip(action string, ignored bool) string {
	label := fixActionLabel(action)
	base := fixChoiceChipStyle
	if action == fixNoAction {
		base = base.Inherit(fixNoActionStyle)
	} else {
		base = base.Inherit(fixActionStyleFor(action))
	}
	if ignored {
		base = base.Foreground(mutedTextColor).Faint(true)
	}
	return base.Render(label)
}

func fixChoiceArrowStyle(available bool) lipgloss.Style {
	if available {
		return fixChoiceArrowAvailableStyle
	}
	return fixChoiceArrowDimStyle
}

func renderScheduledDetails(actions []string, ignored bool) string {
	if len(actions) == 0 {
		style := fixNoActionStyle
		if ignored {
			style = style.Copy().Faint(true)
		}
		return style.Render(fixNoAction)
	}
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		style := fixActionStyleFor(action)
		if ignored {
			style = style.Copy().Foreground(mutedTextColor).Faint(true)
		}
		token := style.Render("■")
		parts = append(parts, token+" "+style.Render(fixActionLabel(action)))
	}
	return strings.Join(parts, "  ")
}

func fixScheduledPlainText(actions []string) string {
	if len(actions) == 0 {
		return fixNoAction
	}
	labels := make([]string, 0, len(actions))
	for _, action := range actions {
		labels = append(labels, fixActionLabel(action))
	}
	return strings.Join(labels, " ")
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
	case FixActionPush:
		return fixActionPushStyle
	case FixActionStageCommitPush:
		return fixActionStageStyle
	case FixActionPublishNewBranch:
		return fixActionStageStyle
	case FixActionCheckpointThenSync:
		return fixActionSyncStyle
	case FixActionPullFFOnly:
		return fixActionPullStyle
	case FixActionSetUpstreamPush:
		return fixActionUpstreamStyle
	case FixActionCreateProject:
		return fixActionCreateProjectStyle
	case FixActionClone:
		return fixActionCloneStyle
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
	}
	if spec, ok := fixActionSpecFor(action); ok {
		return spec.Label
	}
	return action
}

func fixActionDescription(action string) string {
	switch action {
	case fixNoAction:
		return "No eligible fix is currently available for selection."
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
	if hasUnsyncableReason(repo.Record.UnsyncableReasons, domain.ReasonCloneRequired) {
		return fixRepoTierNotCloned
	}
	if unsyncableReasonsFullyCoverable(repo.Record.UnsyncableReasons, fixActionsForSelection(actions)) {
		return fixRepoTierAutofixable
	}
	return fixRepoTierUnsyncableBlocked
}

func hasUnsyncableReason(reasons []domain.UnsyncableReason, target domain.UnsyncableReason) bool {
	for _, reason := range reasons {
		if reason == target {
			return true
		}
	}
	return false
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
		return has[FixActionStageCommitPush] || has[FixActionCheckpointThenSync]
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
	case domain.ReasonCloneRequired:
		return has[FixActionClone]
	case domain.ReasonCatalogNotMapped:
		return false
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
