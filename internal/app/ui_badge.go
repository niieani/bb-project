package app

import (
	"strings"

	"charm.land/lipgloss/v2"
)

type badgeTone int

const (
	badgeToneNeutral badgeTone = iota
	badgeToneInfo
	badgeToneSuccess
	badgeToneWarning
	badgeToneDanger
)

func renderBadge(label string, tone badgeTone) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	base := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	switch tone {
	case badgeToneInfo:
		return base.
			Foreground(themeColor(uiThemeIsDark, "#0550AE", "#C9D1D9")).
			Background(themeColor(uiThemeIsDark, "#DDF4FF", "#13233A")).
			Render(label)
	case badgeToneSuccess:
		return base.
			Foreground(themeColor(uiThemeIsDark, "#0F5132", "#0D1117")).
			Background(themeColor(uiThemeIsDark, "#D1FADF", "#3FB950")).
			Render(label)
	case badgeToneWarning:
		return base.
			Foreground(themeColor(uiThemeIsDark, "#663C00", "#161B22")).
			Background(themeColor(uiThemeIsDark, "#F8D66D", "#D29922")).
			Render(label)
	case badgeToneDanger:
		return base.
			Foreground(themeColor(uiThemeIsDark, "#FFFFFF", "#FFFFFF")).
			Background(themeColor(uiThemeIsDark, "#CF222E", "#F85149")).
			Render(label)
	default:
		return base.
			Foreground(mutedTextColor).
			Background(themeColor(uiThemeIsDark, "#F6F8FA", "#161B22")).
			Render(label)
	}
}
