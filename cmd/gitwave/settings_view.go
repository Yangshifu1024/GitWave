package main

import (
	"strings"

	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type settingsSection string

const (
	settingsGeneral    settingsSection = "general"
	settingsAppearance settingsSection = "appearance"
	settingsNetwork    settingsSection = "network"
	settingsSSH        settingsSection = "ssh"
)

// Settings binds colors independently of the workbench's render so a change
// of theme keeps the open dialog and its native fields mounted.
type settingsTheme struct {
	colors func() palette
	fonts  fontMetrics
}

func (s settingsTheme) uiSize(size core.Points) core.Points {
	return (palette{fonts: s.fonts}).uiSize(size)
}
func (s settingsTheme) codeSize(size core.Points) core.Points {
	return (palette{fonts: s.fonts}).codeSize(size)
}

func (s settingsTheme) text() core.Color   { return s.colors().text }
func (s settingsTheme) muted() core.Color  { return s.colors().muted }
func (s settingsTheme) danger() core.Color { return s.colors().danger }

func settingsTextColor(color func() core.Color) func() core.Style {
	return func() core.Style { return core.Style{Color: color()} }
}

func (s settingsTheme) label(value string, color func() core.Color, size core.Points) core.Text {
	return core.Text{Content: value, MaxLines: 1, Style: core.Style{FontSize: s.uiSize(size)}, StyleFunc: settingsTextColor(color)}
}

func (s settingsTheme) hint(value string) core.Text {
	return core.Text{Content: value, Style: core.Style{FontSize: s.uiSize(11)}, StyleFunc: settingsTextColor(s.muted)}
}

func (s settingsTheme) buttonColors() core.Style {
	p := s.colors()
	return core.Style{Hover: &core.Style{Background: p.hover}, FocusVisible: &core.Style{OutlineWidth: 1, OutlineColor: p.accent}}
}

func (s settingsTheme) button(label string, invoke func(), children ...core.Node) core.View {
	button := toolbarAction(palette{}, label, invoke, children...)
	button.Style.Hover, button.Style.FocusVisible = nil, nil
	button.StyleFunc = s.buttonColors
	return button
}

func (s settingsTheme) action(label string, invoke func()) core.View {
	return s.button(label, invoke, s.label(label, s.text, 12))
}

func (s settingsTheme) iconAction(label string, icon *core.SVG, invoke func()) core.View {
	button := s.button(label, invoke, core.Icon{SVG: icon, Style: core.Style{FontSize: 14}, StyleFunc: settingsTextColor(s.muted)})
	button.Tooltip = s.label(label, s.text, 12)
	return button
}

func (w *workbench) settingsDialog(colors func() palette, fonts func() workspace.DesktopFonts) core.Node {
	return core.Show{When: w.refreshSettingsOpen.Get, Then: func(cx *core.Scope) core.Node {
		chrome := settingsTheme{colors: colors}
		// Keep the settings type sizes steady while its font fields are edited.
		cx.Untrack(func() { chrome.fonts = desktopFonts(fonts()) })
		closeDialog := func() {
			w.commitRefreshInterval()
			w.commitFontSettings()
			w.commitProxyAddress()
			w.closeSSHSettings()
			w.refreshSettingsOpen.Set(false)
		}
		title := w.tr("Settings", "设置")
		heading := chrome.label(title, chrome.text, 18)
		heading.Role, heading.Style.FontWeight = core.RoleHeader, 600
		closeButton := chrome.iconAction(w.tr("Close", "关闭"), lucide.X(), closeDialog)
		closeButton.TestID = "settings-close"
		closeButton.Style.Width = core.Pt(28)
		return base.Dialog{TestID: "settings-dialog", Label: title, Open: w.refreshSettingsOpen.Get,
			BackdropTestID: "settings-backdrop",
			BackdropStyleFunc: func() core.Style {
				backdrop := core.RGB(0x000000).Faded(.38)
				if cx.Env().Dark() {
					backdrop = core.RGB(0x000000).Faded(.55)
				} else if w.desktopPrefs.Get().Palette == workspace.TideStudio {
					backdrop = core.RGB(0x1b2228).Faded(.4)
				}
				style := core.Style{Background: backdrop}
				if cx.Env().OS() == core.OSMacOS {
					style.BackdropFilters = []core.Filter{core.Blur(8)}
				}
				return style
			},
			PopupStyle: core.Style{FontFamily: chrome.fonts.ui, Gap: 12, Padding: core.All(16), CornerRadius: 12,
				BorderWidth: 1, FlexShrink: 0},
			PopupStyleFunc: func() core.Style {
				p := colors()
				return core.Style{Width: core.Pt(min(800, cx.Env().Window().Width*.9)), Background: p.elevated, BorderColor: p.border}
			},
			After: []core.Node{core.View{Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOn},
				Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Height: core.Pt(titleBarHeight)}}},
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					closeDialog()
				}
			},
			Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceBetween, AlignItems: core.AlignCenter},
					Children: []core.Node{heading, closeButton}},
				core.View{Style: core.Style{Direction: core.Row, MinHeight: core.Pt(0), Gap: 20},
					StyleFunc: func() core.Style { return core.Style{Height: core.Pt(cx.Env().Window().Height * .6)} },
					Children: []core.Node{
						core.View{TestID: "settings-nav", Style: core.Style{Width: core.Pt(176), FlexShrink: 0, Gap: 2, Padding: core.Edges{Right: 8},
							Borders: core.Edges{Right: 1}}, StyleFunc: func() core.Style { return core.Style{BorderColor: colors().border} }, Children: []core.Node{
							w.settingsTab(chrome, settingsGeneral, w.tr("General", "通用"), lucide.Settings2()),
							w.settingsTab(chrome, settingsAppearance, w.tr("Appearance", "外观"), lucide.Palette()),
							w.settingsTab(chrome, settingsNetwork, w.tr("Network", "网络"), lucide.Globe()),
							w.settingsTab(chrome, settingsSSH, w.tr("SSH Keys", "SSH 密钥"), lucide.KeyRound()),
						}},
						core.Dynamic(func(*core.Scope) core.Node {
							scroll := core.Scroll{TestID: "settings-body", Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), MinHeight: core.Pt(0)}}
							switch w.settingsSection.Get() {
							case settingsAppearance:
								scroll.Children = []core.Node{w.appearanceSettings(chrome)}
								scroll.ContentOffset = w.appearanceOffset
								scroll.OnScroll = func(event core.ScrollEvent) {
									if !event.Stale {
										w.appearanceOffset = event.Offset
									}
								}
							case settingsNetwork:
								scroll.Children = []core.Node{w.networkSettings(chrome)}
							case settingsSSH:
								if panel := w.sshSettings.Get(); panel != nil {
									scroll.Children = []core.Node{w.sshSettingsView(chrome, panel)}
								}
							default:
								scroll.Children = []core.Node{w.generalSettings(chrome)}
							}
							return scroll
						}),
					}},
				core.Show{When: func() bool { return w.desktopPrefsErr.Get() != "" }, Then: func(*core.Scope) core.Node {
					return core.Text{Role: core.RoleStatus, TestID: "settings-save-error", ContentFunc: w.desktopPrefsErr.Get,
						StyleFunc: settingsTextColor(chrome.danger)}
				}},
			}}
	}}
}

func (w *workbench) settingsTab(chrome settingsTheme, section settingsSection, label string, icon *core.SVG) core.Node {
	selected := func() bool { return w.settingsSection.Get() == section }
	color := func() core.Style {
		if selected() {
			return core.Style{Color: chrome.colors().accent, FontWeight: 500}
		}
		return core.Style{Color: chrome.muted()}
	}
	caption := chrome.label(label, chrome.muted, 14)
	caption.StyleFunc = color
	button := chrome.button(label, func() {
		if !selected() {
			w.commitRefreshInterval()
			w.commitFontSettings()
			w.commitProxyAddress()
			w.closeSSHSettings()
			w.settingsSection.Set(section)
			if section == settingsSSH {
				w.openSSHSettings()
			}
		}
	}, core.Icon{SVG: icon, Style: core.Style{FontSize: 15}, StyleFunc: color}, caption)
	button.TestID = "settings-" + string(section)
	button.Selected = selected
	button.Style.JustifyContent, button.Style.Height = core.JustifyStart, core.Pt(36)
	button.Style.Gap, button.Style.Padding = 8, core.Edges{Left: 12, Right: 12}
	button.StyleFunc = func() core.Style {
		style, accent := chrome.buttonColors(), chrome.colors().accent
		style.Hover = &core.Style{Background: accent.Faded(.05)}
		if selected() {
			style.Background = accent.Faded(.1)
		}
		return style
	}
	return button
}

func (w *workbench) generalSettings(chrome settingsTheme) core.Node {
	return core.View{Style: core.Style{Gap: 20}, Children: []core.Node{
		core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
			w.settingsLanguage(chrome, "settings-ui-language", w.tr("UI language", "界面语言"),
				w.tr("Applies immediately to the interface and menus.", "立即更新界面和菜单的显示语言。"),
				workspace.UILanguagePreference, func() string {
					if strings.HasPrefix(strings.ToLower(w.cx.Env().Locale()), "zh") {
						return "zh-CN"
					}
					return "en"
				}, []ui.Option[string]{{Value: "en", Label: "English"}, {Value: "zh-CN", Label: "中文"}}),
			w.settingsLanguage(chrome, "settings-ai-language", w.tr("AI language", "AI 语言"),
				w.tr("Language for generated replies, commit messages and PR descriptions.", "AI 回复、commit message 和 PR 描述使用的语言。"),
				workspace.AILanguagePreference, func() string { return w.desktopPrefs.Get().AILanguage },
				[]ui.Option[string]{{Value: "en", Label: "English"}, {Value: "zh", Label: "中文"},
					{Value: "ja", Label: "日本語"}, {Value: "ko", Label: "한국어"}}),
		}},
		w.refreshSettingsControls(chrome),
		w.updateSettings(chrome),
	}}
}

func (w *workbench) settingsLanguage(chrome settingsTheme, id, label, hint string, key workspace.DesktopPreference,
	value func() string, options []ui.Option[string]) core.Node {
	return core.View{Style: core.Style{Gap: 5}, Children: []core.Node{
		chrome.label(label, chrome.text, 12),
		ui.Select[string]{TestID: id, Label: label, Options: options, Value: value,
			Style:    core.Style{Width: core.Pct(100)},
			OnChange: func(value string) { w.saveDesktopPreference(key, value) }},
		chrome.hint(hint),
	}}
}

func settingsHint(chrome palette, hint string) core.Text {
	return core.Text{Content: hint, Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.muted}}
}

func (w *workbench) appearanceSettings(chrome settingsTheme) core.Node {
	themeLabel, paletteLabel := w.tr("Theme", "主题"), w.tr("Color palette", "配色方案")
	theme := func(value, label string, icon *core.SVG) core.Node {
		checked := func() bool { return w.desktopPrefs.Get().Theme == value }
		return settingsCard(chrome, "settings-theme-"+value, label, checked,
			func() { w.saveDesktopPreference(workspace.ThemePreference, value) }, settingsCardIcon(chrome, icon, checked))
	}
	color := func(value, label string) core.Node {
		return settingsCard(chrome, "settings-palette-"+value, label, func() bool { return w.desktopPrefs.Get().Palette == value },
			func() { w.saveDesktopPreference(workspace.PalettePreference, value) }, paletteSample(value))
	}
	return core.View{Style: core.Style{Gap: 20}, Children: []core.Node{
		core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
			chrome.label(themeLabel, chrome.muted, 12),
			ui.RadioGroup{Label: themeLabel, Horizontal: true, Style: core.Style{ColumnGap: 8}, Children: []core.Node{
				theme(workspace.LightTheme, w.tr("Light", "浅色"), lucide.Sun()),
				theme(workspace.DarkTheme, w.tr("Dark", "深色"), lucide.Moon()),
				theme(workspace.SystemTheme, w.tr("System", "跟随系统"), lucide.Monitor()),
			}},
		}},
		core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
			chrome.label(paletteLabel, chrome.muted, 12),
			ui.RadioGroup{Label: paletteLabel, Horizontal: true, Style: core.Style{ColumnGap: 8}, Children: []core.Node{
				color(workspace.NativeBlue, "Native Blue"),
				color(workspace.TideStudio, "Tide Studio"),
			}},
		}},
		w.fontSettings(chrome),
	}}
}

func settingsCard(chrome settingsTheme, id, label string, checked func() bool, selectOption func(), preview core.Node) base.Radio {
	caption := chrome.label(label, chrome.muted, 14)
	caption.Style.FlexShrink, caption.Style.MinWidth = 1, core.Pt(0)
	caption.StyleFunc = func() core.Style {
		if checked() {
			return core.Style{Color: chrome.text(), FontWeight: 500}
		}
		return core.Style{Color: chrome.muted()}
	}
	return base.Radio{TestID: id, Label: label, Checked: checked, OnChange: func(bool) { selectOption() },
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0),
			MinHeight: core.Pt(38), Padding: core.All(8), Gap: 8, CornerRadius: 8, BorderWidth: 1},
		StyleFunc: func() core.Style {
			p := chrome.colors()
			style := core.Style{BorderColor: p.border, Hover: &core.Style{BorderColor: p.muted},
				FocusVisible: &core.Style{OutlineWidth: 1, OutlineColor: p.accent}}
			if checked() {
				style.Background, style.BorderColor, style.Hover = p.accent.Faded(.05), p.accent, nil
			}
			return style
		}, Children: []core.Node{preview, caption}}
}

func settingsCardIcon(chrome settingsTheme, icon *core.SVG, checked func() bool) core.Node {
	return core.Icon{SVG: icon, Style: core.Style{FontSize: 18, FlexShrink: 0},
		StyleFunc: func() core.Style {
			if checked() {
				return core.Style{Color: chrome.colors().accent}
			}
			return core.Style{Color: chrome.muted()}
		}}
}

func paletteSample(name string) core.Node {
	rgb := core.RGB
	canvas, sidebar, accent := rgb(0xececec), rgb(0xdfdfdf), rgb(0x007aff)
	lanes := []core.Color{rgb(0x007aff), rgb(0x06b7db), rgb(0x7828c8)}
	if name == workspace.TideStudio {
		canvas, sidebar, accent = rgb(0xf4f6f8), rgb(0xe6ebef), rgb(0x1a8f8a)
		lanes = []core.Color{rgb(0x12a594), rgb(0x0090ff), rgb(0x8e4ec6)}
	}
	var bars []core.Node
	for _, lane := range lanes {
		bars = append(bars, core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Height: core.Pt(4), CornerRadius: 2, Background: lane}})
	}
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, Padding: core.All(4),
		Width: core.Pt(80), Height: core.Pt(28), FlexShrink: 0, CornerRadius: 6, Background: canvas,
		BorderWidth: 1, BorderColor: core.RGB(0x000000).Faded(.08)}, Children: []core.Node{
		core.View{Style: core.Style{Width: core.Pt(12), Height: core.Pct(100), CornerRadius: 4, Background: sidebar}},
		core.View{Style: core.Style{Direction: core.Row, Gap: 4, FlexGrow: 1, FlexBasis: core.Pt(0)}, Children: bars},
		core.View{Style: core.Style{Width: core.Pt(12), Height: core.Pt(12), CornerRadius: 6, Background: accent}},
	}}
}
