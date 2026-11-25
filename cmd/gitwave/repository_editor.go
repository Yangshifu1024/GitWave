package main

import (
	"fmt"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type repoEditorUI struct {
	repoEditor      *core.Signal[*repoEditorState]
	repoEditorDraft *core.Signal[string]
	repoEditorError *core.Signal[string]
	ignoreForm      *core.Signal[*ignoreForm]
	ignoreChoice    *core.Signal[gitcli.IgnoreScope]
}

type repoEditorState struct {
	child *backend
	key   changesWriteKey
	hooks bool
	list  []gitcli.HookInfo
	panel *gitcli.HooksEditor
	pick  gitcli.HookInfo
	hook  *gitcli.HookFile
	file  *gitcli.GitignoreEditor
}

type ignoreForm struct {
	child   *backend
	key     changesWriteKey
	path    string
	options []gitcli.IgnoreOption
}

func newRepoEditorUI(cx *core.Scope) repoEditorUI {
	return repoEditorUI{
		repoEditor: core.NewSignal[*repoEditorState](cx, nil), repoEditorDraft: core.NewSignal(cx, ""),
		repoEditorError: core.NewSignal(cx, ""), ignoreForm: core.NewSignal[*ignoreForm](cx, nil),
		ignoreChoice: core.NewSignal(cx, gitcli.IgnoreFile),
	}
}

func (w *workbench) closeRepoEditors() {
	w.repoEditor.Set(nil)
	w.repoEditorDraft.Set("")
	w.repoEditorError.Set("")
	w.ignoreForm.Set(nil)
}

func (w *workbench) canOpenRepoEditor() bool {
	return w.core != nil && w.core.repo.WorkTree != "" && w.canSync() && w.lfsPanel.Get() == nil
}

func (w *workbench) openRepoEditor(hooks bool) {
	if !w.canOpenRepoEditor() {
		return
	}
	state := &repoEditorState{child: w.core, key: w.changesWriteKey(), hooks: hooks}
	w.repoEditor.Set(state)
	w.repoEditorDraft.Set("")
	w.repoEditorError.Set("")
	ctx := w.repoCtx
	go func() {
		next := *state
		var err error
		if hooks {
			next.panel, err = state.child.repo.OpenHooksEditor(ctx)
			if err == nil {
				next.list, err = next.panel.List(ctx)
			}
		} else {
			next.file, err = state.child.repo.OpenGitignore(ctx)
		}
		w.cx.Post(func() {
			if ctx.Err() != nil || !w.currentRepoEditor(state) {
				return
			}
			if err != nil {
				w.repoEditorError.Set(changesWriteError(err))
				return
			}
			if next.file != nil {
				w.repoEditorDraft.Set(next.file.Text())
			}
			w.repoEditor.Set(&next)
		})
	}()
}

func (w *workbench) currentRepoEditor(state *repoEditorState) bool {
	return state != nil && w.repoEditor.Get() == state && w.core == state.child && w.changesWriteKey() == state.key
}

func (w *workbench) pickRepoHook(info gitcli.HookInfo) {
	current := w.repoEditor.Get()
	if !w.currentRepoEditor(current) || current.panel == nil || w.currentChangesWrite().pending {
		return
	}
	next := *current
	next.pick, next.hook = info, nil
	w.repoEditorDraft.Set("")
	w.repoEditorError.Set("")
	w.repoEditor.Set(&next)
	ctx := w.repoCtx
	go func() {
		hook, err := next.panel.Open(ctx, info.Name)
		w.cx.Post(func() {
			if ctx.Err() != nil || !w.currentRepoEditor(&next) {
				return
			}
			if err != nil {
				w.repoEditorError.Set(changesWriteError(err))
				return
			}
			loaded := next
			loaded.hook = hook
			w.repoEditorDraft.Set(hook.Text())
			w.repoEditor.Set(&loaded)
		})
	}()
}

func (w *workbench) canSaveRepoEditor() bool {
	state := w.repoEditor.Get()
	return w.currentRepoEditor(state) && !w.currentChangesWrite().pending &&
		!w.workspaceBlocked() && w.workspaceModal.Get().kind == "" &&
		(state.file != nil || state.hook != nil && state.hook.Text() != w.repoEditorDraft.Get())
}

func (w *workbench) saveRepoEditor() {
	if !w.canSaveRepoEditor() {
		return
	}
	state, draft := w.repoEditor.Get(), w.repoEditorDraft.Get()
	w.repoEditorError.Set("")
	w.beginChangesWrite(state.key)
	ctx := w.appCtx
	go func() {
		var err, reloadErr error
		next := *state
		if state.file != nil {
			err = state.file.Save(ctx, draft)
		} else {
			err = state.hook.Save(ctx, draft)
			if err == nil {
				next.list, reloadErr = state.panel.List(ctx)
				if reloadErr == nil {
					next.hook, reloadErr = state.panel.Open(ctx, state.pick.Name)
				}
			}
		}
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			w.setChangesWrite(state.key, changesWrite{})
			if !w.currentRepoEditor(state) {
				return
			}
			w.loadChanges()
			if err != nil {
				w.repoEditorError.Set(changesWriteError(err))
				return
			}
			if state.file != nil {
				w.closeRepoEditors()
				w.refNotice.Set(referenceNotice{text: w.tr(".gitignore saved", ".gitignore 已保存")})
				return
			}
			w.refNotice.Set(referenceNotice{text: fmt.Sprintf(w.tr("Hook %s saved", "已保存 hook %s"), state.pick.Name)})
			if reloadErr == nil {
				w.repoEditorDraft.Set(next.hook.Text())
			} else {
				next.hook = nil
				w.repoEditorError.Set(changesWriteError(reloadErr))
			}
			w.repoEditor.Set(&next)
		})
	}()
}

const samplePreCommit = `#!/bin/sh
# GitWave sample pre-commit hook. Non-zero exit aborts the commit.
# Example: block commits to main with TODO markers
# if git diff --cached | grep -q TODO; then
#   echo "TODO markers found in staged changes" >&2
#   exit 1
# fi
`

func (w *workbench) repositoryEditorDialog(chrome palette) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		state := w.repoEditor.Get()
		if !w.currentRepoEditor(state) {
			return core.View{}
		}
		title := w.tr("Edit .gitignore", "编辑 .gitignore")
		description := w.tr("One pattern per line, relative to the repository root.", "每行一条模式，相对于仓库根目录。")
		id, width := "gitignore-dialog", core.Pt(540)
		busy := func() bool { return w.currentChangesWrite().pending }
		close := func() {
			if !busy() {
				w.closeRepoEditors()
			}
		}
		save := func() string {
			if busy() {
				return w.tr("Saving…", "保存中…")
			}
			if state.hooks && state.pick.Name != "" {
				return fmt.Sprintf(w.tr("Save %s", "保存 %s"), state.pick.Name)
			}
			return w.tr("Save", "保存")
		}
		saveButton := toolbarAction(chrome, "", w.saveRepoEditor, core.Text{ContentFunc: save})
		saveButton.LabelFunc, saveButton.Disabled = save, func() bool { return !w.canSaveRepoEditor() }
		saveButton.TestID = "repo-editor-save"
		footer := []core.Node{action(chrome, w.tr("Cancel", "取消"), close), saveButton}
		body := w.repoEditorInput(chrome, state, busy)
		if state.hooks {
			title, id, width = "Git hooks", "hooks-dialog", core.Pt(700)
			description = w.tr("Edit scripts and see their paths in the repository’s configured hooks directory.", "编辑脚本并查看其在仓库实际 hook 目录中的路径。")
			body = w.repoHooksBody(chrome, state, body, busy)
			footer = []core.Node{saveButton}
		}
		x := iconAction(chrome, w.tr("Close", "关闭"), lucide.X(), close)
		x.TestID, x.Disabled = "repo-editor-close", busy
		header := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 12}, Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 4},
				Children: []core.Node{
					core.Text{Content: title, Role: core.RoleHeader, Style: ui.TypeTitle},
					core.Text{Content: description, Style: core.Style{FontSize: chrome.uiSize(13), Color: chrome.muted}},
				}}, x,
		}}
		return ui.Dialog{TestID: id, Label: title,
			Open: func() bool { return w.currentRepoEditor(state) },
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open && w.currentRepoEditor(state) {
					close()
				}
			},
			Style: core.Style{Width: width},
			Children: []core.Node{header, body,
				core.Text{TestID: "repo-editor-error", ContentFunc: w.repoEditorError.Get, Role: core.RoleAlert,
					Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.danger}}},
			Footer: footer}
	})
}

func (w *workbench) repoEditorInput(chrome palette, state *repoEditorState, busy func() bool) core.Node {
	id, label := "gitignore-text", ".gitignore"
	ready := state.file != nil
	placeholder := ""
	if state.hooks {
		id, label, ready = "hook-text", state.pick.Name, state.hook != nil
		placeholder = "#!/bin/sh\n# " + state.pick.Name
	}
	if !ready {
		return core.Text{ContentFunc: func() string {
			if w.repoEditorError.Get() != "" {
				return ""
			}
			return w.tr("Loading…", "加载中…")
		}, Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.muted}}
	}
	return core.Input{TestID: id, Label: label, Placeholder: placeholder, Multiline: true, NoAutoCorrect: true,
		Value: w.repoEditorDraft.Get, OnChange: w.repoEditorDraft.Set, Disabled: busy,
		Style: core.Style{MinWidth: core.Pt(0), Height: core.Pt(280), Padding: core.All(10),
			FontFamily: chrome.fonts.code, FontSize: chrome.codeSize(12), BorderWidth: 1, BorderColor: chrome.border}}
}

func (w *workbench) repoHooksBody(chrome palette, state *repoEditorState, input core.Node, busy func() bool) core.Node {
	choices := make([]core.Node, 0, len(state.list))
	for _, info := range state.list {
		status := chrome.border
		if info.Exists {
			status = core.RGB(0x2e8c4c)
		}
		row := toolbarAction(chrome, info.Name, func() { w.pickRepoHook(info) },
			core.View{Style: core.Style{Width: core.Pt(6), Height: core.Pt(6), CornerRadius: 3, Background: status}},
			core.Text{Content: info.Name, Style: core.Style{FontFamily: chrome.fonts.code, FontSize: chrome.codeSize(12)}})
		row.TestID = "hook-" + info.Name
		row.Selected = func() bool { return state.pick.Name == info.Name }
		row.Disabled = busy
		row.Style.JustifyContent = core.JustifyStart
		if state.pick.Name == info.Name {
			row.Style.Background = chrome.selected
		}
		choices = append(choices, row)
	}
	content := []core.Node{chrome.label(w.tr("Select a hook on the left to edit it.", "请在左侧选择要编辑的 hook。"), chrome.muted, 12)}
	if state.panel == nil && len(state.list) == 0 {
		content = []core.Node{input}
	} else if state.pick.Name != "" {
		hookPath := state.pick.Path
		if state.hook != nil {
			hookPath = state.hook.Path()
		}
		path := fmt.Sprintf(w.tr("Editing %s", "正在编辑 %s"), hookPath)
		content = []core.Node{core.View{Tooltip: chrome.label(hookPath, chrome.text, 12),
			Children: []core.Node{core.Text{Content: path, MaxLines: 2, Style: core.Style{
				MinWidth: core.Pt(0), FlexShrink: 1, FontFamily: chrome.fonts.code, FontSize: chrome.codeSize(11), Color: chrome.muted}}}}, input}
		if state.pick.Name == "pre-commit" && state.hook != nil {
			content = append(content, core.Show{When: func() bool { return strings.TrimSpace(w.repoEditorDraft.Get()) == "" },
				Then: func(*core.Scope) core.Node {
					label := w.tr("Insert sample", "插入示例")
					button := toolbarAction(chrome, label, func() { w.repoEditorDraft.Set(samplePreCommit) }, chrome.label(label, chrome.text, 12))
					button.TestID, button.Disabled = "hook-insert-sample", busy
					return button
				}})
		}
	}
	return core.View{Style: core.Style{Direction: core.Row, Gap: 12}, Children: []core.Node{
		core.View{Style: core.Style{Width: core.Pt(176), FlexShrink: 0, Gap: 2}, Children: choices},
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), Gap: 8}, Children: content},
	}}
}

func (w *workbench) openIgnoreForm(path string) {
	options := gitcli.GitignoreOptions(path)
	if !w.canOpenRepoEditor() || len(options) == 0 {
		return
	}
	w.repoEditorError.Set("")
	w.ignoreChoice.Set(gitcli.IgnoreFile)
	w.ignoreForm.Set(&ignoreForm{child: w.core, key: w.changesWriteKey(), path: path, options: options})
}

func (w *workbench) currentIgnoreForm(form *ignoreForm) bool {
	return form != nil && w.ignoreForm.Get() == form && w.core == form.child && w.changesWriteKey() == form.key
}

func (w *workbench) addIgnoreRule(form *ignoreForm) {
	if !w.currentIgnoreForm(form) || w.currentChangesWrite().pending ||
		w.workspaceBlocked() || w.workspaceModal.Get().kind != "" {
		return
	}
	scope := w.ignoreChoice.Get()
	w.repoEditorError.Set("")
	w.beginChangesWrite(form.key)
	ctx := w.appCtx
	go func() {
		added, err := form.child.repo.IgnoreUntracked(ctx, form.path, scope)
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			w.setChangesWrite(form.key, changesWrite{})
			if !w.currentIgnoreForm(form) {
				return
			}
			w.loadChanges()
			if err != nil {
				w.repoEditorError.Set(changesWriteError(err))
				return
			}
			w.closeRepoEditors()
			message := w.tr("File already ignored", "文件已被忽略")
			if added {
				message = w.tr("Pattern added to .gitignore", "已将模式添加到 .gitignore")
			}
			w.refNotice.Set(referenceNotice{text: message})
		})
	}()
}

func (w *workbench) ignoreDialog(chrome palette) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		form := w.ignoreForm.Get()
		if !w.currentIgnoreForm(form) {
			return core.View{}
		}
		busy := func() bool { return w.currentChangesWrite().pending }
		close := func() {
			if !busy() && w.currentIgnoreForm(form) {
				w.closeRepoEditors()
			}
		}
		items := make([]core.Node, 0, len(form.options))
		for _, opt := range form.options {
			label, id := w.tr("File path", "文件路径"), "ignore-file"
			switch opt.Scope {
			case gitcli.IgnoreDirectory:
				label, id = w.tr("Directory", "目录"), "ignore-directory"
			case gitcli.IgnoreExtension:
				label, id = w.tr("Extension", "扩展名"), "ignore-extension"
			}
			items = append(items, ui.Radio{TestID: id, Label: label, Disabled: busy,
				Checked:  func() bool { return w.ignoreChoice.Get() == opt.Scope },
				OnChange: func(bool) { w.ignoreChoice.Set(opt.Scope) },
				Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12},
					Children: []core.Node{
						chrome.label(label, chrome.text, 12),
						core.Text{Content: opt.Pattern, MaxLines: 1, Style: core.Style{
							FlexShrink: 1, MinWidth: core.Pt(0), FontFamily: chrome.fonts.code, FontSize: chrome.codeSize(12), Color: chrome.muted}},
					}}},
			})
		}
		label := w.tr("Add", "添加")
		add := toolbarAction(chrome, label, func() { w.addIgnoreRule(form) }, chrome.label(label, chrome.text, 12))
		add.TestID, add.Disabled = "ignore-add", busy
		return ui.Dialog{TestID: "ignore-dialog", Title: w.tr("Add to .gitignore", "添加到 .gitignore"),
			Description: w.tr("Append a rule to the repository’s .gitignore and keep the file on disk.", "向仓库的 .gitignore 追加一条规则，文件仍保留在磁盘上。"),
			Open:        func() bool { return w.currentIgnoreForm(form) },
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					close()
				}
			},
			Style: core.Style{Width: core.Pt(440)},
			Children: []core.Node{ui.RadioGroup{Label: w.tr("Ignore scope", "忽略范围"), Children: items},
				core.Text{TestID: "ignore-error", ContentFunc: w.repoEditorError.Get, Role: core.RoleAlert,
					Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.danger}}},
			Footer: []core.Node{action(chrome, w.tr("Cancel", "取消"), close), add}}
	})
}
