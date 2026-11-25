package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"gitwave.work/internal/gitcli"
)

func (w *workbench) canRebaseBranch(upstream branch, checkout repository) bool {
	return upstream.Kind == "local" && upstream.Tip != "" && !upstream.Current && checkout.Head != "" &&
		slices.ContainsFunc(checkout.Branches, func(b branch) bool { return b.Kind == "local" && b.Current }) &&
		w.changes.Get().status.Commit == (gitcli.CommitState{}) && w.canSync() && w.fetch.prompt.Get() == nil
}

func (w *workbench) startBranchRebase(upstream branch, checkout repository) {
	child, key := w.core, w.changesWriteKey()
	if child == nil || !w.canRebaseBranch(upstream, checkout) {
		return
	}
	current := checkout.Branches[slices.IndexFunc(checkout.Branches, func(b branch) bool {
		return b.Kind == "local" && b.Current
	})]
	local := gitcli.CheckoutTarget{Ref: "refs/heads/" + current.Name, OID: checkout.Head}
	onto := gitcli.CheckoutTarget{Ref: "refs/heads/" + upstream.Name, OID: upstream.Tip}
	w.beginChangesWrite(key)
	w.workspaceNotice.Set("")
	w.refNotice.Set(referenceNotice{text: fmt.Sprintf(w.tr("Rebasing onto %s…", "正在变基到 %s…"), upstream.Name)})
	ctx := w.appCtx
	go func() {
		result, err := child.repo.RebaseBranch(ctx, onto, local)
		moved := err == nil && (result.Kind == gitcli.RebaseFastForward || result.Kind == gitcli.RebaseClean)
		var info repository
		if moved {
			info, _ = child.loadRepository(ctx)
		}
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			active := w.core == child && w.changesWriteKey() == key
			if active && info.Head != "" {
				w.repo.Set(info)
			}
			w.setChangesWrite(key, changesWrite{})
			if !active {
				return
			}
			w.refNotice.Set(w.branchRebaseNotice(upstream.Name, result, err))
			w.loadChanges()
			if moved {
				w.refreshRepository(false)
			}
		})
	}()
}

func (w *workbench) branchRebaseNotice(name string, result gitcli.RebaseResult, err error) referenceNotice {
	if err != nil {
		var dirty *gitcli.RebaseDirtyError
		if errors.As(err, &dirty) {
			return referenceNotice{failed: true, text: fmt.Sprintf(w.tr(
				"Working copy has %d uncommitted file(s). Commit or stash changes before rebasing.",
				"工作副本中有 %d 个未提交文件。请先提交或贮藏改动，再变基。"), dirty.Files)}
		}
		return referenceNotice{failed: true, text: changesWriteError(err)}
	}
	var kind string
	switch result.Kind {
	case gitcli.RebaseUpToDate:
		kind = w.tr("already up to date", "已是最新")
	case gitcli.RebaseFastForward:
		kind = w.tr("fast forward", "快进")
	case gitcli.RebaseClean:
		kind = w.tr("clean", "无冲突")
	case gitcli.RebaseConflicts:
		return referenceNotice{failed: true, text: fmt.Sprintf(w.tr("Rebase onto %s hit conflicts: %s", "变基到 %s 时遇到冲突：%s"),
			name, strings.Join(result.Conflicts, ", "))}
	}
	return referenceNotice{text: fmt.Sprintf(w.tr("Rebased onto %s (%s)", "已变基到 %s（%s）"), name, kind)}
}
