package main

import (
	"context"
	"errors"

	"gitwave.work/internal/externaltools"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func (w *workbench) externalTitleAction(chrome palette, width func() core.Points, id, label, title string, icon *core.SVG, invoke func()) core.View {
	button := w.titleAction(chrome, width, label, icon, true, nil, invoke)
	button.TestID = id
	button.Label = title
	button.Tooltip = chrome.label(title, chrome.text, 12)
	return button
}

func (w *workbench) editorTitleAction(chrome palette, width func() core.Points) core.Node {
	button := w.externalTitleAction(chrome, width, "open-editor", w.tr("Editor", "编辑器"),
		w.tr("Open in editor", "在编辑器中打开"), lucide.SquareCode(), nil)
	button.Children = append(button.Children, core.Icon{SVG: lucide.ChevronDown(),
		Style: core.Style{Color: chrome.muted, FontSize: 12}})
	return core.Show{When: func() bool { return w.path.Get() != "" },
		Else: func(*core.Scope) core.Node { return button },
		Then: func(*core.Scope) core.Node {
			return core.View{Desktop: button.Desktop, Children: []core.Node{base.Menu{
				TestID: button.TestID, Label: button.Label, Tooltip: button.Tooltip,
				Style: button.Style, Placement: core.PlaceBottom, Offset: 4, Margin: 8,
				Trigger: core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6},
					Children: button.Children},
				PopupStyle: core.Style{Background: chrome.elevated, BorderWidth: 1, BorderColor: chrome.border,
					Padding: core.All(4), CornerRadius: 8, MinWidth: core.Pt(180)},
				Children: []core.Node{core.Func(func(*core.Scope) core.Node {
					repo := w.path.Get()
					editors := externaltools.Editors()
					if len(editors) == 0 {
						return editorMenuItem(chrome, "", w.tr("No supported editor detected", "未检测到支持的编辑器"), nil)
					}
					items := make([]core.Node, 0, len(editors))
					for _, editor := range editors {
						items = append(items, editorMenuItem(chrome, editor.ID, editor.Name, func() {
							w.launchExternalTool(repo, func(ctx context.Context, dir string) error {
								return externaltools.OpenEditor(ctx, dir, editor.ID)
							})
						}))
					}
					return core.Group(items)
				})},
			}}}
		}}
}

func editorMenuItem(chrome palette, id, label string, selectEditor func()) core.Node {
	color := chrome.text
	if selectEditor == nil {
		color = chrome.muted
	}
	return base.MenuItem{TestID: "external-editor-" + id, Label: label, OnSelect: selectEditor,
		Disabled: func() bool { return selectEditor == nil },
		Style: core.Style{Height: core.Pt(28), AlignItems: core.AlignCenter, Direction: core.Row,
			Padding: core.Edges{Left: 8, Right: 8}, CornerRadius: 4,
			Hover: &core.Style{Background: chrome.hover}, FocusVisible: &core.Style{Background: chrome.hover}},
		Children: []core.Node{chrome.label(label, color, 12)}}
}

func (w *workbench) launchExternalTool(repo string, open func(context.Context, string) error) {
	ctx := w.repoCtx
	if ctx == nil || repo == "" || repo != w.path.Get() {
		return
	}
	go func() {
		if err := open(ctx, repo); err != nil {
			w.cx.Post(func() { w.showExternalToolError(ctx, repo, err) })
		}
	}()
}

func (w *workbench) revealRepository() {
	if repo, ctx := w.path.Get(), w.repoCtx; repo != "" && ctx != nil {
		w.cx.OpenFile(core.SharedFile{Path: repo}, func(opened bool, err error) {
			if !opened && err == nil {
				err = errors.New(w.tr("Could not reveal the repository folder", "无法打开仓库文件夹"))
			}
			w.showExternalToolError(ctx, repo, err)
		})
	}
}

func (w *workbench) showExternalToolError(ctx context.Context, repo string, err error) {
	if err != nil && ctx.Err() == nil && w.path.Get() == repo {
		w.refNotice.Set(referenceNotice{text: err.Error(), failed: true})
	}
}
