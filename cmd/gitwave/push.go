package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

type pushUI struct {
	form    *core.Signal[*pushDialogState]
	review  *core.Signal[*pushReviewState]
	remotes *core.Signal[[]gitcli.PushRemote]
	remote  *core.Signal[string]
	tags    *core.Signal[bool]
	force   *core.Signal[bool]
	err     *core.Signal[string]
}

type pushDialogState struct {
	child       *backend
	key         changesWriteKey
	source      branch
	fixedRemote string
	tags        []gitcli.CheckoutTarget
}

type pushReviewState struct {
	key     changesWriteKey
	child   *backend
	request syncRequest
}

func newPushUI(cx *core.Scope) pushUI {
	return pushUI{
		form:    core.NewSignal(cx, (*pushDialogState)(nil)),
		review:  core.NewSignal(cx, (*pushReviewState)(nil)),
		remotes: core.NewSignalFunc(cx, []gitcli.PushRemote(nil), nil),
		remote:  core.NewSignal(cx, ""),
		tags:    core.NewSignal(cx, false),
		force:   core.NewSignal(cx, false),
		err:     core.NewSignal(cx, ""),
	}
}

func (w *workbench) pushAhead() string {
	status := w.changes.Get()
	if !status.ready || status.bare {
		return ""
	}
	return strconv.Itoa(status.status.Ahead)
}

func (w *workbench) canPushFrom(source branch) bool {
	return w.canSync() && w.fetch.prompt.Get() == nil && source.Kind == "local" && source.Tip != ""
}

func (w *workbench) openPush() {
	shown := w.pullSourceBranch()
	child, key, ctx := w.core, w.changesWriteKey(), w.repoCtx
	if child == nil || ctx == nil || !w.canPushFrom(shown) {
		return
	}
	go func() {
		fresh, err := child.loadRepository(ctx)
		w.cx.Post(func() {
			if ctx.Err() != nil || w.core != child || w.changesWriteKey() != key || w.push.form.Get() != nil || !w.canSync() {
				return
			}
			if err != nil {
				w.refNotice.Set(referenceNotice{text: err.Error(), failed: true})
				return
			}
			for _, current := range fresh.Branches {
				if current.Kind == "local" && current.Current && current.Name == shown.Name {
					w.openPushFrom(current, "", fresh.Tags)
					return
				}
			}
			w.refNotice.Set(referenceNotice{text: w.tr("The checked-out branch changed before pushing", "当前分支在推送前发生变化"), failed: true})
		})
	}()
}

func (w *workbench) openPushFrom(source branch, fixedRemote string, tags []tagRef) {
	child := w.core
	if child == nil || w.repoCtx == nil || !w.canPushFrom(source) {
		return
	}
	form := &pushDialogState{child: child, key: w.changesWriteKey(), source: source, fixedRemote: fixedRemote}
	for _, tag := range tags {
		if tag.Tip == source.Tip {
			form.tags = append(form.tags, gitcli.CheckoutTarget{Ref: "refs/tags/" + tag.Name, OID: tag.Object})
		}
	}
	w.push.remotes.Set(nil)
	w.push.remote.Set("")
	w.push.tags.Set(false)
	w.push.force.Set(false)
	w.push.err.Set("")
	w.push.form.Set(form)
	ctx := w.repoCtx
	go func() {
		remotes, err := child.repo.PushRemotes(ctx)
		w.cx.Post(func() {
			if ctx.Err() != nil || !w.samePushForm(form) {
				return
			}
			if err != nil {
				w.push.err.Set(err.Error())
				return
			}
			if len(remotes) == 0 {
				w.push.err.Set(w.tr("No remotes configured", "尚未配置远端"))
				return
			}
			if fixedRemote != "" && !slices.ContainsFunc(remotes, func(remote gitcli.PushRemote) bool { return remote.Name == fixedRemote }) {
				w.push.err.Set(w.tr("The selected remote is no longer configured", "所选远端已不存在"))
				return
			}
			name := fixedRemote
			if name == "" {
				name = preferredPushRemote(form.source.UpstreamRef, remotes)
			}
			w.push.remotes.Set(remotes)
			w.push.remote.Set(name)
		})
	}()
}

func preferredPushRemote(upstream string, remotes []gitcli.PushRemote) string {
	fallback := remotes[0].Name
	best := ""
	for _, remote := range remotes {
		if remote.Name == "origin" {
			fallback = remote.Name
		}
		if strings.HasPrefix(upstream, "refs/remotes/"+remote.Name+"/") && len(remote.Name) > len(best) {
			best = remote.Name
		}
	}
	if best != "" {
		return best
	}
	return fallback
}

func (w *workbench) samePushForm(form *pushDialogState) bool {
	return w.push.form.Get() == form && w.core == form.child && w.changesWriteKey() == form.key
}

func (w *workbench) submitPush(form *pushDialogState) {
	if !w.samePushForm(form) {
		return
	}
	selected := w.push.remote.Get()
	for _, remote := range w.push.remotes.Get() {
		if remote.Name != selected {
			continue
		}
		options := gitcli.PushOptions{Remote: remote,
			Branch:         gitcli.CheckoutTarget{Ref: "refs/heads/" + form.source.Name, OID: form.source.Tip},
			RequireCurrent: form.fixedRemote == "",
			Force:          form.fixedRemote == "" && w.push.force.Get(),
		}
		if form.fixedRemote == "" && w.push.tags.Get() {
			options.Tags = form.tags
		}
		w.push.form.Set(nil)
		w.runSync(syncRequest{push: &options, transfer: gitcli.FetchOptions{Remote: remote.Name}})
		return
	}
}

func (w *workbench) pushDialog(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.push.form.Get() != nil }, Then: func(*core.Scope) core.Node {
		form := w.push.form.Get()
		cancel := func() {
			if w.samePushForm(form) {
				w.push.form.Set(nil)
			}
		}
		title := w.tr("Push", "推送")
		submitLabel := func() string {
			if w.push.force.Get() {
				return w.tr("Review force push", "检查强制推送")
			}
			return title
		}
		confirm := toolbarAction(chrome, "", func() { w.submitPush(form) },
			core.Text{ContentFunc: submitLabel, Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.text}})
		confirm.LabelFunc = submitLabel
		confirm.TestID = "push-confirm"
		confirm.Disabled = func() bool { return w.push.remote.Get() == "" }
		var body []core.Node
		description := w.tr("Push your local changes to remote repository", "将你的本地变更推送到远程仓库")
		if form.fixedRemote != "" {
			description = fmt.Sprintf(w.tr("Push branch %s to %s?", "将分支 %s 推送到 %s？"), form.source.Name, form.fixedRemote)
		} else {
			remote := syncSelect(chrome, "push-remote", w.tr("Remote", "远程"), w.push.remote.Get, func() []string {
				var names []string
				for _, remote := range w.push.remotes.Get() {
					names = append(names, remote.Name)
				}
				return names
			}, func(name string) {
				if w.samePushForm(form) {
					w.push.remote.Set(name)
				}
			})
			tagLabel := w.tr("Push tags on this commit", "推送此提交上的标签")
			if len(form.tags) != 0 {
				names := make([]string, 0, len(form.tags))
				for _, tag := range form.tags {
					names = append(names, strings.TrimPrefix(tag.Ref, "refs/tags/"))
				}
				tagLabel = fmt.Sprintf(w.tr("Push tags on this commit (%s)", "推送此提交上的标签(%s)"), strings.Join(names, ", "))
			}
			body = append(body,
				remote,
				pushDetail(chrome, w.tr("Branch", "分支"), func() string { return form.source.Name }),
				pushDetail(chrome, w.tr("To", "推送到"), func() string {
					if name := w.push.remote.Get(); name != "" {
						return name + "/" + form.source.Name
					}
					return ""
				}),
				ui.Checkbox{TestID: "push-tags", Text: tagLabel, Checked: w.push.tags.Get, OnChange: w.push.tags.Set,
					Disabled: func() bool { return len(form.tags) == 0 }},
				ui.Checkbox{TestID: "push-force", Text: w.tr("Force push", "强制推送"),
					Checked: w.push.force.Get, OnChange: w.push.force.Set},
			)
		}
		selectedRemote := func() gitcli.PushRemote {
			selected := w.push.remote.Get()
			for _, remote := range w.push.remotes.Get() {
				if remote.Name == selected {
					return remote
				}
			}
			return gitcli.PushRemote{}
		}
		body = append(body, core.Show{When: func() bool { return selectedRemote().DestinationCount() > 1 }, Then: func(*core.Scope) core.Node {
			return core.Text{
				ContentFunc: func() string {
					return fmt.Sprintf(w.tr("This remote has %d push destinations.", "此远端配置了 %d 个推送目的地。"), selectedRemote().DestinationCount())
				},
				Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.muted},
			}
		}})
		body = append(body, core.Show{When: func() bool { return selectedRemote().LFSUploadURL() != "" }, Then: func(*core.Scope) core.Node {
			return core.Text{TestID: "push-lfs-destination", ContentFunc: func() string {
				return fmt.Sprintf(w.tr("LFS objects are uploaded to %s", "LFS 对象将上传到 %s"), selectedRemote().LFSUploadURL())
			}, Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.muted}}
		}})
		body = append(body, core.Text{ContentFunc: w.push.err.Get, Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.danger}})
		return ui.Dialog{TestID: "push-dialog", Title: title, Description: description,
			Open: func() bool { return w.samePushForm(form) },
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					cancel()
				}
			},
			Children: body, Footer: []core.Node{action(chrome, w.tr("Cancel", "取消"), cancel), confirm}}
	}}
}

func pushDetail(chrome palette, label string, value func() string) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
		core.Text{Content: label, Style: core.Style{Width: core.Pt(58), FontSize: chrome.uiSize(12), Color: chrome.muted}},
		core.Text{ContentFunc: value, MaxLines: 1, Style: core.Style{FlexShrink: 1, FontSize: chrome.uiSize(12), Color: chrome.text}},
	}}
}

func (w *workbench) pushReviewDialog(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.push.review.Get() != nil }, Then: func(*core.Scope) core.Node {
		review := w.push.review.Get()
		valid := func() bool {
			return w.push.review.Get() == review && w.core == review.child && w.changesWriteKey() == review.key
		}
		cancel := func() {
			if valid() {
				w.push.review.Set(nil)
			}
		}
		submit := func() {
			if valid() {
				w.push.review.Set(nil)
				w.runSync(review.request)
			}
		}
		var rows []core.Node
		endpoint := ""
		for _, change := range review.request.review.Changes {
			if endpoint != change.Destination {
				endpoint = change.Destination
				rows = append(rows, chrome.label(endpoint, chrome.text, 12))
			}
			previous := change.Before
			if previous == "" {
				previous = w.tr("missing", "不存在")
			}
			rows = append(rows, chrome.label(change.Ref, chrome.text, 11),
				chrome.label(previous+" → "+change.After, chrome.muted, 11))
		}
		label := w.tr("Force push", "强制推送")
		confirm := toolbarAction(chrome, label, submit, chrome.label(label, chrome.danger, 12))
		confirm.TestID = "push-force-confirm"
		return ui.Dialog{TestID: "push-review-dialog", Title: w.tr("Confirm force push", "确认强制推送"),
			Description: w.tr("The remote references will be replaced only if they still match the reviewed versions.",
				"仅当远端引用仍与以下版本一致时，才会替换它们。"),
			Open: valid,
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					cancel()
				}
			},
			Children: []core.Node{core.Scroll{Style: core.Style{MaxHeight: core.Pt(260), Gap: 4}, Children: rows}},
			Footer:   []core.Node{action(chrome, w.tr("Cancel", "取消"), cancel), confirm}}
	}}
}

func (w *workbench) branchPushItem(source branch, names []string) core.MenuItem {
	label := w.tr("Push", "推送")
	item := core.MenuItem{Label: label, Disabled: !w.canPushFrom(source) || len(names) == 0}
	if len(names) == 1 {
		name := names[0]
		item.Label = fmt.Sprintf(w.tr("Push to %s", "推送到 %s"), name)
		item.OnSelect = w.scopedRefAction(func() { w.openPushFrom(source, name, w.repo.Get().Tags) })
	}
	if len(names) > 1 {
		for _, name := range names {
			item.Items = append(item.Items, core.MenuItem{Label: name,
				OnSelect: w.scopedRefAction(func() { w.openPushFrom(source, name, w.repo.Get().Tags) })})
		}
	}
	return item
}
