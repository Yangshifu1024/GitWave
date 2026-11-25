package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
)

type stashUI struct {
	stashSave      *core.Signal[*stashSaveForm]
	stashApply     *core.Signal[*stashApplyPrompt]
	stashDrop      *core.Signal[*stashDropPrompt]
	stashMessage   *core.Signal[string]
	stashUntracked *core.Signal[bool]
	stashError     *core.Signal[string]
	stashList      *core.Signal[stashListState]
	stashOpen      *core.Signal[bool]
	stashPreview   *core.Signal[stashPreviewState]
	stashFiles     *core.Signal[[]fileDiff]
	stashCollapsed *core.Signal[bool]
	stashReadID    uint64
	stopStashDiff  context.CancelFunc
}

type stashSaveForm struct {
	child  *backend
	key    changesWriteKey
	shown  gitcli.Status
	source *gitcli.CheckoutTarget
}

type stashApplyPrompt struct {
	child    *backend
	key      changesWriteKey
	entry    gitcli.StashEntry
	shown    gitcli.Status
	checking bool
	pop      bool
}

type stashDropPrompt struct {
	child   *backend
	key     changesWriteKey
	entry   gitcli.StashEntry
	loading bool
	files   []fileDiff
}

type stashListState struct {
	entries []gitcli.StashEntry
	ready   bool
	err     string
}

type stashPreviewState struct {
	entry    gitcli.StashEntry
	path     string
	diff     preview
	loading  bool
	expanded bool
	err      string
}

func newStashUI(cx *core.Scope) stashUI {
	return stashUI{
		stashSave:      core.NewSignal[*stashSaveForm](cx, nil),
		stashApply:     core.NewSignal[*stashApplyPrompt](cx, nil),
		stashDrop:      core.NewSignal[*stashDropPrompt](cx, nil),
		stashMessage:   core.NewSignal(cx, ""),
		stashUntracked: core.NewSignal(cx, true),
		stashError:     core.NewSignal(cx, ""),
		stashList:      core.NewSignalFunc(cx, stashListState{}, nil),
		stashOpen:      core.NewSignal(cx, true),
		stashPreview:   core.NewSignalFunc(cx, stashPreviewState{}, nil),
		stashFiles:     core.NewSignalFunc(cx, []fileDiff(nil), nil),
		stashCollapsed: core.NewSignal(cx, false),
	}
}

func (w *workbench) clearStashes() {
	w.stashSave.Set(nil)
	w.stashApply.Set(nil)
	w.stashDrop.Set(nil)
	w.stashMessage.Set("")
	w.stashError.Set("")
	w.stashList.Set(stashListState{})
	w.stashOpen.Set(true)
	w.stashReadID++
	w.closeStashPreview()
}

func (w *workbench) loadStashes() {
	child := w.core
	if child == nil {
		return
	}
	w.stashReadID++
	id, ctx := w.stashReadID, w.repoCtx
	go func() {
		entries, err := child.repo.Stashes(ctx)
		w.cx.Post(func() {
			if ctx.Err() != nil || child != w.core || id != w.stashReadID {
				return
			}
			old := w.stashList.Get()
			result := stashListState{entries: entries, ready: true}
			if err != nil {
				result.err = changesWriteError(err)
			}
			if len(old.entries) == 0 && old.err == "" && (len(entries) != 0 || result.err != "") {
				w.stashOpen.Set(true)
			}
			if old.ready != result.ready || old.err != result.err || !slices.EqualFunc(old.entries, result.entries, gitcli.StashEntry.SameSelection) {
				w.stashList.Set(result)
			}
		})
	}()
}

func (w *workbench) canSaveStash() bool {
	current := w.changes.Get()
	return w.canManageStash() && current.ready && !current.bare
}

func (w *workbench) openStashSave() {
	if !w.canSaveStash() {
		return
	}
	w.stashError.Set("")
	w.stashSave.Set(&stashSaveForm{child: w.core, key: w.changesWriteKey(), shown: w.changes.Get().status})
}

func (w *workbench) submitStashSave() {
	form := w.stashSave.Get()
	if form == nil || w.core != form.child || w.changesWriteKey() != form.key || !w.canSync() {
		return
	}
	w.stashError.Set("")
	w.saveStash(form, w.stashMessage.Get(), w.stashUntracked.Get(), func(err error) {
		if w.stashSave.Get() != form {
			return
		}
		if err != nil {
			w.stashError.Set(err.Error())
			return
		}
		w.stashSave.Set(nil)
		w.stashMessage.Set("")
	})
}

func (w *workbench) saveStash(form *stashSaveForm, message string, include bool, complete func(error)) {
	w.beginChangesWrite(form.key)
	ctx := w.appCtx
	go func() {
		repo := form.child.repo
		shown := form.shown
		var err error
		if source := form.source; source != nil {
			shown, err = repo.Status(ctx)
			if err == nil && (shown.HeadRef != source.Ref || shown.HeadOID != source.OID) {
				err = gitcli.ErrStashTargetChanged
			}
		}
		if err == nil {
			_, err = repo.SaveStash(ctx, shown, message, include)
		}
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			result := changesWrite{}
			if err != nil && form.source == nil {
				result.err = err.Error()
			}
			w.setChangesWrite(form.key, result)
			if w.changesWriteKey() != form.key {
				return
			}
			w.loadStashes()
			w.loadChanges()
			complete(err)
			if err == nil {
				w.refNotice.Set(referenceNotice{text: w.tr("Stash saved", "已贮藏变更")})
			}
		})
	}()
}

func (w *workbench) stashTitle(entry gitcli.StashEntry) string {
	message := entry.Message
	if strings.TrimSpace(message) == "" {
		message = w.tr("No message", "无消息")
	}
	return fmt.Sprintf("stash@{%d} · %s", entry.Index, message)
}

func (w *workbench) openStashPreview(entry gitcli.StashEntry) {
	if w.core == nil || !slices.ContainsFunc(w.stashList.Get().entries, entry.SameSelection) {
		return
	}
	w.stashCollapsed.Set(false)
	w.stashError.Set("")
	w.stashFiles.Set(nil)
	w.loadStashPreview(stashPreviewState{entry: entry})
}

func (w *workbench) closeStashPreview() {
	if w.stopStashDiff != nil {
		w.stopStashDiff()
		w.stopStashDiff = nil
	}
	w.stashPreview.Set(stashPreviewState{})
	w.stashFiles.Set(nil)
}

func (w *workbench) loadStashPreview(state stashPreviewState) {
	if w.core == nil {
		return
	}
	if w.stopStashDiff != nil {
		w.stopStashDiff()
	}
	child := w.core
	ctx, cancel := context.WithCancel(w.repoCtx)
	w.stopStashDiff = cancel
	state.loading, state.err, state.diff = true, "", preview{}
	w.stashPreview.Set(state)
	go func() {
		result, err := child.repo.StashDiff(ctx, state.entry.OID, state.path, state.expanded)
		w.cx.Post(func() {
			if ctx.Err() != nil || child != w.core {
				return
			}
			cancel()
			w.stopStashDiff = nil
			state.loading = false
			if err != nil {
				state.err = changesWriteError(err)
			} else if state.path == "" {
				w.stashFiles.Set(result.Files)
				if len(result.Files) != 0 {
					state.path = result.Files[0].Path
					w.loadStashPreview(state)
					return
				}
			} else {
				state.diff = result
			}
			w.stashPreview.Set(state)
		})
	}()
}

func (w *workbench) canApplyStash() bool {
	return w.canManageStash() && w.core.repo.WorkTree != ""
}

func (w *workbench) canManageStash() bool {
	return w.core != nil && w.canSync() && w.stashApply.Get() == nil && w.stashSave.Get() == nil && w.stashDrop.Get() == nil
}

func (w *workbench) requestStashApply(entry gitcli.StashEntry, pop bool) {
	if !w.canApplyStash() {
		return
	}
	if !slices.ContainsFunc(w.stashList.Get().entries, entry.SameSelection) {
		w.showStashActionError(w.staleStashMessage())
		return
	}
	w.stashError.Set("")
	prompt := &stashApplyPrompt{child: w.core, key: w.changesWriteKey(), entry: entry, checking: true, pop: pop}
	w.stashApply.Set(prompt)
	ctx := w.repoCtx
	go func() {
		shown, err := prompt.child.repo.Status(ctx)
		w.cx.Post(func() {
			if ctx.Err() != nil || !w.currentStashApply(prompt) {
				return
			}
			if err != nil {
				w.stashApply.Set(nil)
				w.showStashActionError(err.Error())
				return
			}
			loaded := *prompt
			loaded.checking, loaded.shown = false, shown
			w.stashApply.Set(&loaded)
			if len(shown.Changes) == 0 {
				if !w.canSync() {
					w.stashApply.Set(nil)
					w.showStashActionError(w.tr("The repository is busy. Try applying the stash again.", "仓库正忙，请重试应用贮藏。"))
					return
				}
				w.submitStashApply()
			}
		})
	}()
}

func (w *workbench) currentStashApply(prompt *stashApplyPrompt) bool {
	return prompt != nil && w.stashApply.Get() == prompt && w.core == prompt.child && w.changesWriteKey() == prompt.key
}

func (w *workbench) submitStashApply() {
	prompt := w.stashApply.Get()
	if !w.currentStashApply(prompt) || prompt.checking || !w.canSync() {
		return
	}
	w.stashApply.Set(nil)
	w.beginChangesWrite(prompt.key)
	ctx := w.appCtx
	go func() {
		apply := prompt.child.repo.ApplyStash
		if prompt.pop {
			apply = prompt.child.repo.PopStash
		}
		result, err := apply(ctx, prompt.entry, prompt.shown)
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			w.setChangesWrite(prompt.key, changesWrite{})
			if w.changesWriteKey() != prompt.key || w.core != prompt.child {
				return
			}
			w.loadStashes()
			w.loadChanges()
			switch {
			case err != nil:
				w.showStashActionError(err.Error())
			case len(result.Conflicts) != 0:
				message := w.tr("Stash kept. Conflicts in", "已保留贮藏，冲突文件") + " · " + strings.Join(result.Conflicts, ", ")
				if result.Warning != nil {
					message += " · " + result.Warning.Error()
				}
				w.showChanges(true)
				w.showStashActionError(message)
			case result.Warning != nil:
				w.showStashActionError(result.Warning.Error())
			case result.Dropped:
				w.closePreviewForStash(prompt.entry)
				w.refNotice.Set(referenceNotice{text: w.tr("Stash applied and deleted", "已应用并删除 stash")})
			default:
				w.refNotice.Set(referenceNotice{text: w.tr("Stash applied", "已应用 stash")})
			}
		})
	}()
}

func (w *workbench) showStashActionError(message string) {
	w.stashError.Set(message)
	w.refNotice.Set(referenceNotice{text: message, failed: true})
}

func (w *workbench) staleStashMessage() string {
	return w.tr("The stash list changed. Reopen this entry to continue.", "贮藏列表已变化，请重新打开该条目后继续。")
}

func (w *workbench) requestStashDrop(entry gitcli.StashEntry) {
	if !w.canManageStash() {
		return
	}
	if !slices.ContainsFunc(w.stashList.Get().entries, entry.SameSelection) {
		w.showStashActionError(w.staleStashMessage())
		return
	}
	w.stashError.Set("")
	prompt := &stashDropPrompt{child: w.core, key: w.changesWriteKey(), entry: entry, loading: true}
	w.stashDrop.Set(prompt)
	ctx := w.repoCtx
	go func() {
		diff, err := prompt.child.repo.StashDiff(ctx, entry.OID, "", false)
		w.cx.Post(func() {
			if ctx.Err() != nil || w.stashDrop.Get() != prompt {
				return
			}
			loaded := *prompt
			loaded.loading = false
			if err == nil {
				loaded.files = diff.Files
			}
			w.stashDrop.Set(&loaded)
		})
	}()
}

func (w *workbench) submitStashDrop() {
	prompt := w.stashDrop.Get()
	if prompt == nil || w.core != prompt.child || w.changesWriteKey() != prompt.key || !w.canSync() {
		return
	}
	w.stashDrop.Set(nil)
	w.beginChangesWrite(prompt.key)
	ctx := w.appCtx
	go func() {
		err := prompt.child.repo.DropStash(ctx, prompt.entry)
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			w.setChangesWrite(prompt.key, changesWrite{})
			if w.changesWriteKey() != prompt.key || w.core != prompt.child {
				return
			}
			w.loadStashes()
			w.loadChanges()
			if err != nil {
				w.showStashActionError(err.Error())
				return
			}
			w.closePreviewForStash(prompt.entry)
			w.refNotice.Set(referenceNotice{text: w.tr("Stash deleted", "已删除 stash")})
		})
	}()
}

func (w *workbench) closePreviewForStash(entry gitcli.StashEntry) {
	if w.stashPreview.Get().entry.SameSelection(entry) {
		w.closeStashPreview()
	}
}
