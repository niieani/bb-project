package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
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
			Foreground(lipgloss.AdaptiveColor{Light: "#0550AE", Dark: "#C9D1D9"}).
			Background(lipgloss.AdaptiveColor{Light: "#DDF4FF", Dark: "#13233A"}).
			Render(label)
	case badgeToneSuccess:
		return base.
			Foreground(lipgloss.AdaptiveColor{Light: "#0F5132", Dark: "#0D1117"}).
			Background(lipgloss.AdaptiveColor{Light: "#D1FADF", Dark: "#3FB950"}).
			Render(label)
	case badgeToneWarning:
		return base.
			Foreground(lipgloss.AdaptiveColor{Light: "#663C00", Dark: "#161B22"}).
			Background(lipgloss.AdaptiveColor{Light: "#F8D66D", Dark: "#D29922"}).
			Render(label)
	case badgeToneDanger:
		return base.
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"}).
			Render(label)
	default:
		return base.
			Foreground(mutedTextColor).
			Background(lipgloss.AdaptiveColor{Light: "#F6F8FA", Dark: "#161B22"}).
			Render(label)
	}
}
