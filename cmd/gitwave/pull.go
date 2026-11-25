package main

import (
	"slices"
	"strconv"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type pullUI struct {
	form    *core.Signal[*pullDialogState]
	options *core.Signal[gitcli.PullOptions]
	remotes *core.Signal[[]gitcli.PullRemote]
	err     *core.Signal[string]
}

type pullDialogState struct {
	child *backend
	key   changesWriteKey
	local branch
}

func newPullUI(cx *core.Scope) pullUI {
	return pullUI{
		form:    core.NewSignal(cx, (*pullDialogState)(nil)),
		options: core.NewSignal(cx, gitcli.PullOptions{}),
		remotes: core.NewSignalFunc(cx, []gitcli.PullRemote(nil), nil),
		err:     core.NewSignal(cx, ""),
	}
}

func (w *workbench) pullSourceBranch() branch {
	for _, candidate := range w.repo.Get().Branches {
		if candidate.Current && candidate.Kind == "local" {
			return candidate
		}
	}
	return branch{}
}

func (w *workbench) canOpenPull() bool {
	return w.canSync() && w.fetch.prompt.Get() == nil && w.pullSourceBranch().Name != ""
}

func (w *workbench) pullBehind() string {
	s := w.changes.Get()
	if !s.ready || s.bare {
		return ""
	}
	return strconv.Itoa(s.status.Behind)
}

func (w *workbench) openPull() {
	child := w.core
	if child == nil || w.repoCtx == nil || !w.canOpenPull() {
		return
	}
	local := w.pullSourceBranch()
	form := &pullDialogState{child: child, key: w.changesWriteKey(), local: local}
	w.pull.remotes.Set(nil)
	w.pull.err.Set("")
	w.pull.options.Set(gitcli.PullOptions{Local: gitcli.CheckoutTarget{Ref: "refs/heads/" + local.Name, OID: local.Tip}})
	w.pull.form.Set(form)
	ctx := w.repoCtx
	go func() {
		choices, err := child.repo.PullChoices(ctx, local.Name)
		w.cx.Post(func() {
			if ctx.Err() != nil || !w.samePullForm(form) {
				return
			}
			if err != nil {
				w.pull.err.Set(err.Error())
				return
			}
			if len(choices.Remotes) == 0 {
				w.pull.err.Set(w.tr("No remotes configured", "尚未配置远端"))
				return
			}
			remote, selected := choices.UpstreamRemote, choices.UpstreamBranch
			if remote == "" {
				remote = choices.Remotes[0].Name
				for _, candidate := range choices.Remotes {
					if candidate.Name == "origin" {
						remote = "origin"
						break
					}
				}
			}
			if selected == "" {
				selected = local.Name
			}
			options := w.pull.options.Get()
			options.Remote, options.Branch = remote, selected
			w.pull.options.Set(options)
			w.pull.remotes.Set(choices.Remotes)
		})
	}()
}

func (w *workbench) samePullForm(form *pullDialogState) bool {
	return w.pull.form.Get() == form && w.core == form.child && w.changesWriteKey() == form.key
}

func (w *workbench) pullBranchesForRemote(name string) []string {
	for _, remote := range w.pull.remotes.Get() {
		if remote.Name == name {
			return remote.Branches
		}
	}
	return nil
}

func (w *workbench) pullDialog(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.pull.form.Get() != nil }, Then: func(*core.Scope) core.Node {
		form := w.pull.form.Get()
		submit := func() {
			options := w.pull.options.Get()
			if !w.samePullForm(form) || options.Remote == "" || options.Branch == "" {
				return
			}
			w.pull.form.Set(nil)
			w.runSync(syncRequest{pull: &options, transfer: gitcli.FetchOptions{Remote: options.Remote}})
		}
		cancel := func() {
			if w.samePullForm(form) {
				w.pull.form.Set(nil)
			}
		}
		remoteValue := func() string { return w.pull.options.Get().Remote }
		branchValue := func() string { return w.pull.options.Get().Branch }
		remote := syncSelect(chrome, "pull-remote", w.tr("Remote", "远程"), remoteValue, func() []string {
			var names []string
			for _, remote := range w.pull.remotes.Get() {
				names = append(names, remote.Name)
			}
			return names
		}, func(name string) {
			options := w.pull.options.Get()
			if !w.samePullForm(form) || options.Remote == name {
				return
			}
			names := w.pullBranchesForRemote(name)
			options.Remote = name
			if !slices.Contains(names, options.Branch) {
				options.Branch = form.local.Name
				if len(names) != 0 && !slices.Contains(names, options.Branch) {
					options.Branch = names[0]
				}
			}
			w.pull.options.Set(options)
		})
		fromBranch := syncSelect(chrome, "pull-branch", w.tr("Branch", "分支"), func() string {
			if remote, selected := remoteValue(), branchValue(); remote != "" && selected != "" {
				return remote + "/" + selected
			}
			return ""
		}, func() []string {
			if remote := remoteValue(); remote != "" {
				names := w.pullBranchesForRemote(remote)
				selected := branchValue()
				options := make([]string, 0, len(names)+1)
				if selected != "" && !slices.Contains(names, selected) {
					options = append(options, remote+"/"+selected)
				}
				for _, name := range names {
					options = append(options, remote+"/"+name)
				}
				return options
			}
			return nil
		}, func(label string) {
			if name, ok := strings.CutPrefix(label, remoteValue()+"/"); ok && w.samePullForm(form) {
				options := w.pull.options.Get()
				options.Branch = name
				w.pull.options.Set(options)
			}
		})
		confirmLabel := w.tr("Pull", "拉取")
		confirm := toolbarAction(chrome, confirmLabel, submit, chrome.label(confirmLabel, chrome.text, 12))
		confirm.TestID = "pull-confirm"
		confirm.Disabled = func() bool { return remoteValue() == "" || branchValue() == "" }
		return ui.Dialog{TestID: "pull-dialog", Title: confirmLabel,
			Description: w.tr("Pull remote branches and merge them into your local branch", "拉取远程分支并合并到你的本地分支"),
			Open:        func() bool { return w.samePullForm(form) },
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					cancel()
				}
			},
			Children: []core.Node{
				remote, fromBranch,
				core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
					chrome.label(w.tr("Into", "合并到"), chrome.muted, 12), chrome.label(form.local.Name, chrome.text, 12),
				}},
				ui.Checkbox{TestID: "pull-rebase", Text: w.tr("Rebase instead of merge", "变基而不是合并"),
					Checked: func() bool { return w.pull.options.Get().Rebase },
					OnChange: func(enabled bool) {
						options := w.pull.options.Get()
						options.Rebase = enabled
						w.pull.options.Set(options)
					}},
				ui.Checkbox{TestID: "pull-stash", Text: w.tr("Stash and reapply local changes", "贮藏并重新应用本地变更"),
					Checked: func() bool { return w.pull.options.Get().Stash },
					OnChange: func(enabled bool) {
						options := w.pull.options.Get()
						options.Stash = enabled
						w.pull.options.Set(options)
					}},
				core.Text{ContentFunc: w.pull.err.Get, Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.danger}},
			},
			Footer: []core.Node{action(chrome, w.tr("Cancel", "取消"), cancel), confirm}}
	}}
}

func syncSelect(chrome palette, id, label string, value func() string, values func() []string, change func(string)) core.Node {
	selectControl := base.Select{
		TestID: id, Label: label,
		Disabled: func() bool { return len(values()) == 0 },
		Items: func() []base.SelectItem {
			names := values()
			items := make([]base.SelectItem, len(names))
			for i, name := range names {
				items[i] = base.SelectItem{Title: name, Value: name}
			}
			return items
		},
		Value: func() *string {
			selected := value()
			if selected == "" {
				return nil
			}
			return &selected
		},
		OnChange: func(selected *string) {
			if selected != nil {
				change(*selected)
			}
		},
		Style: core.Style{Height: core.Pt(29), BorderWidth: 1, BorderColor: chrome.border, CornerRadius: 4,
			Direction: core.Row, AlignItems: core.AlignCenter, Padding: core.Edges{Left: 8, Right: 8}, FlexGrow: 1},
		RootStyle: core.Style{FlexGrow: 1},
		PopupStyle: core.Style{Background: chrome.elevated, BorderWidth: 1, BorderColor: chrome.border,
			CornerRadius: 4, Padding: core.All(4), MaxHeight: core.Pt(220)},
		ItemStyle: core.Style{Padding: core.All(5), Hover: &core.Style{Background: chrome.hover},
			FocusVisible: &core.Style{Background: chrome.selected}},
		Trigger: core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6},
			Children: []core.Node{core.Text{ContentFunc: value, MaxLines: 1, Style: core.Style{FlexGrow: 1, FontSize: chrome.uiSize(12), Color: chrome.text}},
				core.Icon{SVG: lucide.ChevronDown(), Style: core.Style{FontSize: 13, Color: chrome.muted}}}},
		RenderItem: func(entry base.SelectEntry) core.Node { return chrome.label(entry.Value, chrome.text, 12) },
	}
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
		Children: []core.Node{core.Text{Content: label, Style: core.Style{Width: core.Pt(58), Color: chrome.muted, FontSize: chrome.uiSize(12)}}, selectControl}}
}
