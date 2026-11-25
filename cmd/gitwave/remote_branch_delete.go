package main

import (
	"fmt"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

func (w *workbench) loadRemoteBranchDeletion(form *refDialogState) {
	prefix := "refs/heads/"
	if form.kind == deleteRemoteBranchForm {
		prefix = "refs/remotes/"
	}
	ctx := w.repoCtx
	go func() {
		plan, err := form.child.repo.PrepareRemoteBranchDelete(ctx, gitcli.CheckoutTarget{
			Ref: prefix + form.branch.Name, OID: form.branch.Tip,
		})
		w.cx.Post(func() {
			if ctx.Err() != nil || w.refForm.Get() != form || w.core != form.child || w.changesWriteKey() != form.key {
				return
			}
			loaded := *form
			loaded.checking = false
			if err != nil {
				w.refFailure.Set(w.refActionFailure(err))
			} else if len(plan.Destinations()) != 0 {
				loaded.remoteDelete = plan
			}
			w.refForm.Set(&loaded)
		})
	}()
}

func (w *workbench) remoteBranchDeleteFields(chrome palette, form *refDialogState, busy func() bool) core.Node {
	var fields []core.Node
	if form.kind == deleteBranchForm {
		fields = append(fields, ui.Checkbox{TestID: "also-delete-remote-branch",
			Text:    w.tr("Also delete matching remote branches", "同时删除对应的远端分支"),
			Checked: w.alsoDeleteRemote.Get, OnChange: w.alsoDeleteRemote.Set,
			Disabled: func() bool { return busy() || form.remoteDelete == nil }})
	}
	if form.checking {
		fields = append(fields, chrome.label(w.tr("Reading tracking references…", "正在读取跟踪引用…"), chrome.muted, 12))
	}
	if form.remoteDelete != nil {
		fields = append(fields, core.Show{When: func() bool {
			return form.kind == deleteRemoteBranchForm || w.alsoDeleteRemote.Get()
		}, Then: func(*core.Scope) core.Node {
			targets := form.remoteDelete.Destinations()
			return core.List{TestID: "remote-delete-destinations", EstimatedSize: 35,
				Style: core.Style{Height: core.Pt(core.Points(min(146, 35*len(targets))))},
				Count: func() int { return len(targets) },
				Row: func(_ *core.Scope, row core.ListRow) core.Node {
					return core.Dynamic(func(*core.Scope) core.Node {
						i := row.Index()
						if i < 0 || i >= len(targets) {
							return core.View{}
						}
						target := targets[i]
						endpoint := chrome.label(target.Destination, chrome.muted, 11)
						endpoint.MaxLines = 1
						return core.View{Style: core.Style{MinHeight: core.Pt(35)}, Children: []core.Node{
							chrome.label(target.Branch+" · "+shortSHA(target.Tip), chrome.text, 12), endpoint,
						}}
					})
				}}
		}})
	}
	return core.View{Style: core.Style{Gap: 6}, Children: fields}
}

func (w *workbench) remoteBranchDeleteSummary(result gitcli.RemoteBranchDeleteResult) string {
	var notes []string
	if result.Local != "" {
		notes = append(notes, fmt.Sprintf(w.tr("Deleted local branch %s", "已删除本地分支 %s"), result.Local))
	}
	switch len(result.Deleted) {
	case 0:
	case 1:
		notes = append(notes, fmt.Sprintf(w.tr("Deleted remote branch %s", "已删除远端分支 %s"), result.Deleted[0].Branch))
	default:
		notes = append(notes, fmt.Sprintf(w.tr("Deleted remote branches at %d destinations", "已删除 %d 处远端分支"), len(result.Deleted)))
	}
	return strings.Join(notes, " · ")
}

func (w *workbench) remoteDeleteNotice(result gitcli.RemoteBranchDeleteResult, message string) string {
	parts := []string{w.remoteBranchDeleteSummary(result), message}
	parts = append(parts, result.Warnings...)
	var visible []string
	for _, part := range parts {
		if part != "" {
			visible = append(visible, part)
		}
	}
	return strings.Join(visible, " · ")
}
