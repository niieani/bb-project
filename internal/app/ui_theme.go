package app

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

var uiThemeIsDark = true

func themeColor(isDark bool, light, dark string) color.Color {
	return lipgloss.LightDark(isDark)(
		lipgloss.Color(light),
		lipgloss.Color(dark),
	)
}

func applyGlobalTheme(isDark bool) {
	uiThemeIsDark = isDark
	applyConfigWizardTheme(isDark)
	applyFixTUITheme()
}

func init() {
	applyGlobalTheme(true)
}
