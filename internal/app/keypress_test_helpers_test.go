package app

import tea "charm.land/bubbletea/v2"

func testKeyPressCode(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func testKeyPressRunes(text string) tea.KeyPressMsg {
	runes := []rune(text)
	if len(runes) == 0 {
		return tea.KeyPressMsg{}
	}
	code := runes[0]
	if len(runes) > 1 {
		code = tea.KeyExtended
	}
	return tea.KeyPressMsg{
		Code: code,
		Text: text,
	}
}

func testKeyPressRunesAlt(text string) tea.KeyPressMsg {
	runes := []rune(text)
	if len(runes) == 0 {
		return tea.KeyPressMsg{Mod: tea.ModAlt}
	}
	return tea.KeyPressMsg{
		Code: runes[0],
		Mod:  tea.ModAlt,
	}
}

func testKeyPressCtrl(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{
		Code: code,
		Mod:  tea.ModCtrl,
	}
}

func viewContent(v tea.View) string {
	return v.Content
}
