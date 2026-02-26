package app

import (
	"bytes"
	"image/color"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"bb-project/internal/domain"
)

func TestLipglossV2BubbleTeaDownsamplesByProgramProfile(t *testing.T) {
	trueColor := runLipglossProfileProbe(t, colorprofile.TrueColor)
	ansi256 := runLipglossProfileProbe(t, colorprofile.ANSI256)

	if !strings.Contains(trueColor, "[38;2;") || !strings.Contains(trueColor, "48;2;") {
		t.Fatalf("truecolor output missing 24-bit ANSI sequences: %q", trueColor)
	}
	if strings.Contains(ansi256, "[38;2;") || strings.Contains(ansi256, "48;2;") {
		t.Fatalf("ansi256 output should not contain 24-bit ANSI sequences: %q", ansi256)
	}
	if !strings.Contains(ansi256, "[38;5;") || !strings.Contains(ansi256, "48;5;") {
		t.Fatalf("ansi256 output missing downgraded 256-color ANSI sequences: %q", ansi256)
	}
}

func TestLipglossV2FixTUIListTokensFollowBackgroundColor(t *testing.T) {
	m := newFixTUIModelForTest([]fixRepoState{
		testLipglossRepoState(),
	})

	_, _ = m.Update(tea.BackgroundColorMsg{color.RGBA{R: 0, G: 0, B: 0, A: 0xFF}})
	darkChip := renderCurrentChoiceChip(FixActionPullFFOnly, false)
	darkChipBackground := fixChoiceChipStyle.GetBackground()

	_, _ = m.Update(tea.BackgroundColorMsg{color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}})
	lightChip := renderCurrentChoiceChip(FixActionPullFFOnly, false)
	lightChipBackground := fixChoiceChipStyle.GetBackground()

	if !reflect.DeepEqual(darkChipBackground, themeColor(true, "#F6F8FA", "#30363D")) {
		t.Fatalf("dark list chip background = %#v, want %#v", darkChipBackground, themeColor(true, "#F6F8FA", "#30363D"))
	}
	if !reflect.DeepEqual(lightChipBackground, themeColor(false, "#F6F8FA", "#30363D")) {
		t.Fatalf("light list chip background = %#v, want %#v", lightChipBackground, themeColor(false, "#F6F8FA", "#30363D"))
	}
	if darkChip == lightChip {
		t.Fatalf("expected list chip render to differ between dark/light themes, both=%q", darkChip)
	}
}

func TestLipglossV2FixTUIWizardTokensFollowBackgroundColor(t *testing.T) {
	m := newFixTUIModelForTest([]fixRepoState{
		testLipglossRepoState(),
	})

	_, _ = m.Update(tea.BackgroundColorMsg{color.RGBA{R: 0, G: 0, B: 0, A: 0xFF}})
	darkHint := m.renderWizardVisualDiffHint()
	darkShortcut := lipgloss.NewStyle().
		Foreground(textColor).
		Background(themeColor(true, "#ECF3FF", "#1C2738")).
		Padding(0, 1).
		Render(m.visualDiffShortcutDisplayLabel())
	if !strings.Contains(darkHint, darkShortcut) {
		t.Fatalf("dark wizard hint missing dark shortcut styling: %q", darkHint)
	}

	_, _ = m.Update(tea.BackgroundColorMsg{color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}})
	lightHint := m.renderWizardVisualDiffHint()
	lightShortcut := lipgloss.NewStyle().
		Foreground(textColor).
		Background(themeColor(false, "#ECF3FF", "#1C2738")).
		Padding(0, 1).
		Render(m.visualDiffShortcutDisplayLabel())
	if !strings.Contains(lightHint, lightShortcut) {
		t.Fatalf("light wizard hint missing light shortcut styling: %q", lightHint)
	}

	if darkHint == lightHint {
		t.Fatalf("expected wizard hint render to differ between dark/light themes, both=%q", darkHint)
	}
}

func runLipglossProfileProbe(t *testing.T, profile colorprofile.Profile) string {
	t.Helper()

	probeView := tea.NewView(
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("#58A6FF")).
			Background(lipgloss.Color("#1C2738")).
			Render("profile-probe"),
	)

	var out bytes.Buffer
	var in bytes.Buffer
	withProfile := tea.WithColorProfile
	p := tea.NewProgram(
		lipglossProfileProbeModel{view: probeView},
		tea.WithWindowSize(80, 24),
		withProfile(profile),
		tea.WithEnvironment([]string{"TERM=xterm-256color"}),
		tea.WithInput(&in),
		tea.WithOutput(&out),
	)
	if _, err := p.Run(); err != nil {
		t.Fatalf("bubble tea run failed for profile %v: %v", profile, err)
	}
	return out.String()
}

type lipglossProfileProbeModel struct {
	view tea.View
}

func (lipglossProfileProbeModel) Init() tea.Cmd {
	return tea.Quit
}

func (m lipglossProfileProbeModel) Update(_ tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (m lipglossProfileProbeModel) View() tea.View {
	return m.view
}

func testLipglossRepoState() fixRepoState {
	return fixRepoState{
		Record: domain.MachineRepoRecord{
			Name:      "api",
			Path:      "/repos/api",
			OriginURL: "git@github.com:you/api.git",
			Upstream:  "origin/main",
			Behind:    1,
		},
		Meta: &domain.RepoMetadataFile{
			OriginURL: "https://github.com/you/api.git",
			AutoPush:  domain.AutoPushModeDisabled,
		},
	}
}
