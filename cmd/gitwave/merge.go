package main

import (
	"fmt"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func (w *workbench) canOpenMerge(source branch) bool {
	return source.Kind == "local" && source.Tip != "" && !source.Current && w.repo.Get().Head != "" &&
		w.canSync() && w.fetch.prompt.Get() == nil && w.changes.Get().status.Commit.MergeHeads == ""
}

func (w *workbench) openBranchMerge(source branch) {
	if !w.canOpenMerge(source) || w.core == nil {
		return
	}
	info := w.repo.Get()
	name := info.Branch
	target := gitcli.CheckoutTarget{OID: info.Head}
	for _, candidate := range info.Branches {
		if candidate.Current && candidate.Kind == "local" {
			target.Ref = "refs/heads/" + candidate.Name
			name = candidate.Name
			break
		}
	}
	if target.Ref == "" {
		name = w.tr("Detached HEAD", "分离的 HEAD")
	}
	w.mergeMode.Set(w.mergeAutoLabel())
	w.openRefForm(refDialogState{kind: mergeBranchForm, branch: source, baseLabel: name, checking: true})
	form := w.refForm.Get()
	if form == nil || form.kind != mergeBranchForm {
		return
	}
	ctx := w.repoCtx
	go func() {
		plan, err := form.child.repo.PrepareMerge(ctx, gitcli.CheckoutTarget{Ref: "refs/heads/" + source.Name, OID: source.Tip}, target)
		w.cx.Post(func() {
			if ctx.Err() != nil || w.refForm.Get() != form || w.core != form.child || w.changesWriteKey() != form.key {
				return
			}
			loaded := *form
			loaded.checking = false
			if err != nil {
				w.refFailure.Set(err.Error())
			} else {
				loaded.merge = plan
			}
			w.refForm.Set(&loaded)
		})
	}()
}

func (w *workbench) mergeAutoLabel() string {
	return w.tr("Auto — fast-forward when possible", "自动——尽可能快进 (Fast-forward)")
}

func (w *workbench) mergeFields(chrome palette, form *refDialogState) core.Node {
	label := w.tr("Option", "选项")
	options := []string{w.mergeAutoLabel(), w.tr("No fast-forward — always create a merge commit", "不快进——始终创建合并提交")}
	var status string
	color := chrome.muted
	switch {
	case form.checking:
		status = w.tr("Checking merge…", "正在检查合并…")
	case form.merge == nil:
	case form.merge.UpToDate:
		status = w.tr("Already up to date", "已是最新的")
	case form.merge.DirtyFiles != 0:
		status = fmt.Sprintf(w.tr("Working copy has %d uncommitted file(s) — commit or stash before merging", "工作副本中有 %d 个未提交文件——合并前请先提交或贮藏"), form.merge.DirtyFiles)
		color = chrome.danger
	case len(form.merge.Conflicts) != 0:
		status = fmt.Sprintf(w.tr("May conflict in %d file(s) (resolve after merge)", "可能在 %d 个文件中产生冲突（合并后解决）"), len(form.merge.Conflicts))
		color = chrome.danger
	case form.merge.Conflicted:
		status = w.tr("Review the staged changes before committing the merge", "提交合并前请检查暂存的变更")
		color = chrome.warning
	default:
		status = w.tr("Merge can be done without conflicts", "合并不会产生冲突")
	}
	return core.View{Style: core.Style{Gap: 7}, Children: []core.Node{
		pushDetail(chrome, w.tr("Merge", "合并"), func() string { return form.branch.Name }),
		pushDetail(chrome, w.tr("Into", "目标"), func() string { return form.baseLabel }),
		syncSelect(chrome, "merge-mode", label, w.mergeMode.Get, func() []string {
			if w.currentChangesWrite().pending {
				return nil
			}
			return options
		}, w.mergeMode.Set),
		core.Text{TestID: "merge-preview", Content: status, Role: core.RoleStatus, Style: core.Style{Color: color, FontSize: chrome.uiSize(12)}},
	}}
}

func (w *workbench) applyBranchMerge(form *refDialogState) {
	noFF := w.mergeMode.Get() != w.mergeAutoLabel()
	w.refFailure.Set("")
	w.beginChangesWrite(form.key)
	ctx := w.appCtx
	go func() {
		result, err := form.merge.Apply(ctx, noFF)
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			w.setChangesWrite(form.key, changesWrite{})
			if w.core != form.child || w.changesWriteKey() != form.key || w.refForm.Get() != form {
				return
			}
			if err != nil {
				w.refFailure.Set(err.Error())
				w.loadChanges()
				return
			}
			w.refForm.Set(nil)
			if !result.Conflicted {
				kind := w.tr("three way", "三方合并")
				if result.FastForward {
					kind = w.tr("fast forward", "快进")
				}
				w.refNotice.Set(referenceNotice{text: fmt.Sprintf(w.tr("Merged %s (%s)", "已合并 %s（%s）"), form.branch.Name, kind)})
			}
			w.loadChanges()
			w.refreshRepository(false)
		})
	}()
}

func (w *workbench) mergeBanner(chrome palette) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		changes := w.changes.Get()
		count := len(changes.conflicts)
		merging := changes.status.Commit.MergeHeads != ""
		if !merging && count == 0 {
			return core.View{Style: core.Style{Display: core.DisplayNone}}
		}
		var text string
		switch {
		case merging && count == 0:
			text = w.tr("Merge in progress — all conflicts resolved. Commit the merge from Working Copy to finish it.", "合并进行中，冲突已全部解决。请在工作副本中提交以完成合并。")
		case merging:
			text = fmt.Sprintf(w.tr("Merge in progress — %d conflicted file(s)", "合并进行中，%d 个文件存在冲突"), count)
		default:
			text = fmt.Sprintf(w.tr("The index has %d unresolved conflict file(s). Resolve them before pulling.", "index 中有 %d 个未解决的冲突文件。请先解决，再继续拉取。"), count)
		}
		message := chrome.label(text, chrome.muted, 12)
		message.Role, message.Style.FlexGrow, message.Style.FlexShrink = core.RoleStatus, 1, 1
		label := w.tr("Resolve", "解决冲突")
		openChanges := toolbarAction(chrome, label, func() {
			w.openConflicts("")
		}, chrome.label(label, chrome.text, 12))
		openChanges.TestID = "open-conflicts"
		openChanges.Disabled = func() bool { return count == 0 || w.currentChangesWrite().pending }
		banner := bar(chrome, 33, chrome.panel, core.Icon{SVG: lucide.TriangleAlert(), Style: core.Style{FontSize: 14, Color: chrome.warning}},
			message, openChanges)
		if merging {
			banner.Children = append(banner.Children, w.mergeAbortButton(chrome, "abort-merge"))
		}
		banner.TestID = "merge-banner"
		return banner
	})
}
