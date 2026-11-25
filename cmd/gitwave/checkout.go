package main

import (
	"fmt"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

type checkoutPrompt struct {
	name, failure string
	dirty         int
	plan          *gitcli.CheckoutPlan
	child         *backend
	key           changesWriteKey
}

func (w *workbench) checkoutBranch(b branch) {
	if b.Current || b.Tip == "" {
		return
	}
	prefix := "refs/heads/"
	if b.Kind == "remote" {
		prefix = "refs/remotes/"
	}
	w.requestCheckout(gitcli.CheckoutTarget{Ref: prefix + b.Name, OID: b.Tip}, b.Name)
}

func (w *workbench) checkoutTag(tag tagRef) {
	if tag.Tip != "" {
		w.requestCheckout(gitcli.CheckoutTarget{Ref: "refs/tags/" + tag.Name, OID: tag.Object}, tag.Name)
	}
}

func (w *workbench) checkoutCommit(sha string) {
	if sha != "" {
		w.requestCheckout(gitcli.CheckoutTarget{OID: sha}, shortSHA(sha))
	}
}

func (w *workbench) requestCheckout(target gitcli.CheckoutTarget, name string) {
	w.requestCheckoutFrom(target, name, nil)
}

func (w *workbench) requestCheckoutFrom(target gitcli.CheckoutTarget, name string, source *gitcli.CheckoutTarget) {
	child, key := w.core, w.changesWriteKey()
	if child == nil || w.currentChangesWrite().pending || w.refForm.Get() != nil || w.checkoutPrompt.Get() != (checkoutPrompt{}) {
		return
	}
	readCtx, applyCtx := w.repoCtx, w.appCtx
	w.beginChangesWrite(key)
	w.workspaceNotice.Set("")
	w.refNotice.Set(referenceNotice{})
	go func() {
		plan, err := child.repo.PrepareCheckout(readCtx, target)
		w.cx.Post(func() {
			if applyCtx.Err() != nil {
				return
			}
			if readCtx.Err() != nil || w.core != child || w.changesWriteKey() != key {
				w.setChangesWrite(key, changesWrite{})
				return
			}
			switch {
			case err != nil:
				w.setChangesWrite(key, changesWrite{})
				w.checkoutPrompt.Set(checkoutPrompt{name: name, failure: changesWriteError(err)})
				w.loadChanges()
			case source != nil && !plan.StartsAt(*source):
				w.setChangesWrite(key, changesWrite{})
				w.refNotice.Set(referenceNotice{failed: true, text: w.tr("The checked-out branch or commit changed after the proposal.", "当前检出的分支或提交在建议生成后已变更。")})
				w.loadChanges()
			case plan.Current:
				w.setChangesWrite(key, changesWrite{})
				w.loadChanges()
			case plan.DirtyFiles != 0:
				w.setChangesWrite(key, changesWrite{})
				if source == nil {
					w.checkoutPrompt.Set(checkoutPrompt{name: plan.Name(), dirty: plan.DirtyFiles, child: child, key: key, plan: plan})
				} else {
					w.refNotice.Set(referenceNotice{failed: true, text: fmt.Sprintf(w.tr("Working copy has %d uncommitted file(s).", "工作区有 %d 个未提交的文件。"), plan.DirtyFiles)})
				}
				w.loadChanges()
			default:
				w.applyCheckout(child, key, plan, gitcli.CheckoutClean)
			}
		})
	}()
}

func (w *workbench) confirmDirtyCheckout(mode gitcli.CheckoutMode) {
	prompt := w.checkoutPrompt.Get()
	if prompt.plan == nil || prompt.child != w.core || prompt.key != w.changesWriteKey() || w.currentChangesWrite().pending {
		return
	}
	w.checkoutPrompt.Set(checkoutPrompt{})
	w.beginChangesWrite(prompt.key)
	w.applyCheckout(prompt.child, prompt.key, prompt.plan, mode)
}

func (w *workbench) applyCheckout(child *backend, key changesWriteKey, plan *gitcli.CheckoutPlan, mode gitcli.CheckoutMode) {
	ctx := w.appCtx
	w.refNotice.Set(referenceNotice{text: fmt.Sprintf(w.tr("Checking out %s…", "正在检出 %s…"), plan.Name())})
	note := fmt.Sprintf(w.tr("switch to %s", "切换到 %s"), plan.Name())
	go func() {
		var result gitcli.CheckoutResult
		var err error
		if mode == gitcli.CheckoutClean {
			result, err = plan.Apply(ctx)
		} else {
			result, err = plan.ApplyDirty(ctx, mode, note)
		}
		var info repository
		var loadErr error
		if err == nil {
			info, loadErr = child.loadRepository(ctx)
		}
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			w.setChangesWrite(key, changesWrite{})
			if w.changesWriteKey() != key || w.core != child {
				return
			}
			if err != nil {
				w.refNotice.Set(referenceNotice{})
				message := err.Error()
				if detail := changesWriteError(err); detail != message {
					message += "\n" + detail
				}
				w.checkoutPrompt.Set(checkoutPrompt{name: plan.Name(), failure: message})
			} else {
				if loadErr == nil {
					w.repo.Set(info)
					w.revealSidebarBranch(info)
				}
				message := fmt.Sprintf(w.tr("Checked out %s", "已检出 %s"), result.Name)
				if result.Created {
					message = fmt.Sprintf(w.tr("Created and switched to %s", "已创建并切换到 %s"), result.Name)
				}
				if result.Reapplied {
					message = fmt.Sprintf(w.tr("Checked out %s and re-applied stash", "已检出 %s 并重新应用贮藏"), result.Name)
				}
				if result.Warning != "" {
					w.checkoutPrompt.Set(checkoutPrompt{name: result.Name, failure: message + "\n" + result.Warning})
				}
				w.refNotice.Set(referenceNotice{text: message})
				if !w.changesMode.Get() {
					w.locate = ""
					w.selectCommit(result.OID)
				}
			}
			w.refreshRepository(false)
		})
	}()
}

func (w *workbench) checkoutDialog(chrome palette) core.Node {
	open := func() bool { return w.checkoutPrompt.Get() != (checkoutPrompt{}) }
	close := func() { w.checkoutPrompt.Set(checkoutPrompt{}) }
	return ui.Dialog{TestID: "checkout-dialog", Role: core.RoleAlertDialog, Open: open,
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				close()
			}
		},
		TitleFunc: func() string {
			prompt := w.checkoutPrompt.Get()
			if prompt.dirty != 0 {
				return fmt.Sprintf(w.tr("Switch to %s?", "切换到 %s？"), prompt.name)
			}
			return fmt.Sprintf(w.tr("Checkout %s", "检出 %s"), prompt.name)
		},
		DescriptionFunc: func() string {
			prompt := w.checkoutPrompt.Get()
			if prompt.dirty == 1 {
				return w.tr("The working copy has one uncommitted file. Discard it, or stash, switch, and re-apply the stash on the destination.",
					"工作副本中有一个未提交文件。可以丢弃改动，也可以先贮藏，切换后再重新应用。")
			}
			if prompt.dirty > 1 {
				return fmt.Sprintf(w.tr("The working copy has %d uncommitted files. Discard them, or stash, switch, and re-apply the stash on the destination.",
					"工作副本中有 %d 个未提交文件。可以丢弃改动，也可以先贮藏，切换后再重新应用。"), prompt.dirty)
			}
			return prompt.failure
		},
		Footer: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			if w.checkoutPrompt.Get().dirty != 0 {
				discardLabel := w.tr("Discard", "丢弃")
				discard := toolbarAction(chrome, discardLabel, func() { w.confirmDirtyCheckout(gitcli.CheckoutDiscard) },
					chrome.label(discardLabel, core.RGB(0xffffff), 12))
				discard.TestID = "checkout-discard"
				discard.Style.Background = chrome.danger
				discard.Style.Hover = &core.Style{Background: chrome.danger, Opacity: core.Alpha(.88)}
				stashLabel := w.tr("Stash & switch", "贮藏并切换")
				stash := toolbarAction(chrome, stashLabel, func() { w.confirmDirtyCheckout(gitcli.CheckoutStash) },
					chrome.label(stashLabel, core.RGB(0xffffff), 12))
				stash.TestID = "checkout-stash"
				stash.Style.Background = chrome.accent
				stash.Style.Hover = &core.Style{Background: chrome.accent, Opacity: core.Alpha(.88)}
				return core.View{Style: core.Style{Direction: core.Row, Gap: 6}, Children: []core.Node{
					action(chrome, w.tr("Cancel", "取消"), close),
					discard, stash,
				}}
			}
			return action(chrome, w.tr("OK", "确定"), close)
		})},
	}
}
