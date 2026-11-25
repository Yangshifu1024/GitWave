package main

import (
	"strconv"

	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
	"golang.org/x/text/width"
)

type fontMetrics struct {
	ui, code           string
	uiScale, codeScale core.Points
}

const (
	defaultUIFontSize   = 16
	defaultCodeFontSize = 12
)

func desktopFonts(fonts workspace.DesktopFonts) fontMetrics {
	family := func(value, fallback string) string {
		if value == "" {
			return fallback
		}
		if fallback == "" {
			fallback = "sans-serif"
		}
		return value + ", " + fallback
	}
	return fontMetrics{
		ui:        family(fonts.UI, ""),
		code:      family(fonts.Code, core.FontMonospace),
		uiScale:   fontScale(fonts.UISize, defaultUIFontSize),
		codeScale: fontScale(fonts.CodeSize, defaultCodeFontSize),
	}
}

func fontScale(value string, standard int) core.Points {
	size, err := strconv.Atoi(value)
	if err != nil {
		return 1
	}
	return core.Points(size) / core.Points(standard)
}

func (p palette) uiSize(size core.Points) core.Points {
	if p.fonts.uiScale == 0 {
		return size
	}
	return size * p.fonts.uiScale
}

func (p palette) codeSize(size core.Points) core.Points {
	if p.fonts.codeScale == 0 {
		return size
	}
	return size * p.fonts.codeScale
}

func (p palette) codeCellWidth(cx *core.Scope) core.Points {
	font := core.Font{Family: p.fonts.code, Size: p.codeSize(defaultCodeFontSize)}
	advance := p.codeSize(7.3)
	for _, sample := range []string{"0000", "WWWW", "MMMM", "@@@@"} {
		advance = max(advance, core.Points(cx.MeasureText(sample, font).Width)/core.Points(len(sample)))
	}
	return advance
}

func codeColumns(text string) int {
	columns := 0
	for _, r := range text {
		switch {
		case r == '\t':
			columns += 4
		case r < 128:
			columns++
		default:
			switch width.LookupRune(r).Kind() {
			case width.EastAsianFullwidth, width.EastAsianWide:
				columns += 2
			default:
				columns++
			}
		}
	}
	return columns
}

func (w *workbench) commitFontSettings() {
	draft, baseline := w.fontDraft.Get(), w.fontBaseline
	if draft == baseline {
		return
	}
	if w.desktopPrefsBusy.Get() {
		w.fontCommitQueued = true
		return
	}
	fields := []struct {
		key             workspace.DesktopPreference
		applied, edited string
	}{
		{workspace.UIFontPreference, baseline.UI, workspace.NormalizeFontFamilies(draft.UI)},
		{workspace.UIFontSizePreference, baseline.UISize, workspace.NormalizeFontSize(draft.UISize, 12)},
		{workspace.CodeFontPreference, baseline.Code, workspace.NormalizeFontFamilies(draft.Code)},
		{workspace.CodeSizePreference, baseline.CodeSize, workspace.NormalizeFontSize(draft.CodeSize, 10)},
	}
	changes := make(map[workspace.DesktopPreference]string)
	next := w.desktopPrefs.Get()
	for _, field := range fields {
		if field.edited != field.applied {
			changes[field.key] = field.edited
			next, _ = next.With(field.key, field.edited)
		}
	}
	if len(changes) == 0 {
		w.fontDraft.Set(baseline)
		return
	}
	w.saveDesktopPreferences(next, changes, func(err error) {
		if err != nil {
			return
		}
		current := w.fontDraft.Get()
		saved := w.desktopPrefs.Get().Fonts
		w.fontBaseline = saved
		if current.UI == draft.UI {
			current.UI = saved.UI
		}
		if current.UISize == draft.UISize {
			current.UISize = saved.UISize
		}
		if current.Code == draft.Code {
			current.Code = saved.Code
		}
		if current.CodeSize == draft.CodeSize {
			current.CodeSize = saved.CodeSize
		}
		w.fontDraft.Set(current)
	})
}

func (w *workbench) fontSettings(chrome settingsTheme) core.Node {
	return core.View{Style: core.Style{Gap: 10}, Children: []core.Node{
		chrome.label(w.tr("Fonts", "字体"), chrome.muted, 12),
		w.fontSettingsRow(chrome, "ui", w.tr("Interface font", "界面字体"),
			w.tr("Use comma-separated font families.", "用逗号分隔多个字体族。"),
			w.tr("System (default)", "系统字体（默认）"), defaultUIFontSize, 12,
			func(d *workspace.DesktopFonts) *string { return &d.UI },
			func(d *workspace.DesktopFonts) *string { return &d.UISize }),
		w.fontSettingsRow(chrome, "code", w.tr("Monospace font", "等宽字体"),
			w.tr("Used in diffs, blame and code.", "用于 diff、blame 和代码。"),
			w.tr("Monospace (default)", "默认等宽字体"), defaultCodeFontSize, 10,
			func(d *workspace.DesktopFonts) *string { return &d.Code },
			func(d *workspace.DesktopFonts) *string { return &d.CodeSize }),
	}}
}

func (w *workbench) fontSettingsRow(chrome settingsTheme, id, label, hint, placeholder string, defaultSize, minimum int,
	family, size func(*workspace.DesktopFonts) *string) core.Node {
	read := func(field func(*workspace.DesktopFonts) *string) string {
		draft := w.fontDraft.Get()
		return *field(&draft)
	}
	edit := func(field func(*workspace.DesktopFonts) *string, value string) {
		draft := w.fontDraft.Get()
		*field(&draft) = value
		w.fontDraft.Set(draft)
	}
	input := func(name, caption, placeholder string, field func(*workspace.DesktopFonts) *string, keyboard core.KeyboardType) core.Node {
		reset := w.tr("Reset "+label, "重置"+label)
		column := core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), Gap: 4}
		if name == "size" {
			reset = w.tr("Reset size for "+label, "重置"+label+"的字号")
			column = core.Style{Width: core.Pt(124), Gap: 4}
		}
		testID := "settings-font-" + id + "-" + name
		return core.View{Style: column, Children: []core.Node{
			chrome.label(caption, chrome.text, 12),
			core.View{Style: core.Style{Direction: core.Row, Gap: 4}, Children: []core.Node{
				ui.Input{TestID: testID, Label: caption, Placeholder: placeholder,
					Value: func() string { return read(field) }, OnChange: func(value string) { edit(field, value) },
					OnBlur: w.commitFontSettings, OnSubmit: func(string) { w.commitFontSettings() },
					KeyboardType: keyboard, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0),
						MinWidth: core.Pt(0), FontSize: chrome.uiSize(13)}},
				core.Show{When: func() bool { return read(field) != "" }, Then: func(*core.Scope) core.Node {
					button := chrome.iconAction(reset, lucide.RotateCcw(), func() {
						edit(field, "")
						w.commitFontSettings()
					})
					button.TestID = testID + "-reset"
					return button
				}},
			}},
		}}
	}
	return core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, Gap: 10}, Children: []core.Node{
			input("family", label, placeholder, family, core.KeyboardDefault),
			input("size", w.tr("Size (px)", "字号（px）"), strconv.Itoa(defaultSize), size, core.KeyboardNumber),
		}},
		chrome.hint(hint),
		core.Text{TestID: "settings-font-" + id + "-preview", MaxLines: 1,
			Content: "Aa Bb 0123 — The quick brown fox · 中文字体预览 0O1lI",
			StyleFunc: func() core.Style {
				draft := w.fontDraft.Get()
				value := workspace.NormalizeFontFamilies(*family(&draft))
				fontSize := core.Points(defaultSize) * fontScale(workspace.NormalizeFontSize(*size(&draft), minimum), defaultSize)
				if id == "code" {
					return core.Style{FontFamily: desktopFonts(workspace.DesktopFonts{Code: value}).code, FontSize: fontSize, Color: chrome.muted()}
				}
				family := desktopFonts(workspace.DesktopFonts{UI: value}).ui
				if family == "" {
					family = "sans-serif"
				}
				return core.Style{FontFamily: family, FontSize: fontSize, Color: chrome.muted()}
			}},
	}}
}
