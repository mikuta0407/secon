package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// compactTheme は標準テーマの余白・行間を詰めて、一覧の情報密度を上げる。
type compactTheme struct{ fyne.Theme }

func newCompactTheme() fyne.Theme { return compactTheme{theme.DefaultTheme()} }

func (t compactTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	return t.Theme.Color(n, v)
}

func (t compactTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNamePadding: // 既定 6
		return 3
	case theme.SizeNameInnerPadding: // 既定 8
		return 4
	case theme.SizeNameLineSpacing: // 既定 4
		return 2
	case theme.SizeNameText: // 既定 14
		return 13
	}
	return t.Theme.Size(n)
}
