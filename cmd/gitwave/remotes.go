package main

import (
	"fmt"
	"slices"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
)

type remoteUI struct {
	remoteList      *core.Signal[[]gitcli.RemoteInfo]
	remoteOpen      *core.Signal[bool]
	remoteForm      *core.Signal[*remoteForm]
	remoteName      *core.Signal[string]
	remoteFetch     *core.Signal[string]
	remotePush      *core.Signal[string]
	remoteError     *core.Signal[string]
	remoteListError *core.Signal[string]
	remoteReadID    uint64
}

type remoteForm struct {
	kind  string
	child *backend
	key   changesWriteKey
	entry gitcli.RemoteInfo
}

func newRemoteUI(cx *core.Scope) remoteUI {
	return remoteUI{
		remoteList: core.NewSignalFunc(cx, []gitcli.RemoteInfo(nil), nil),
		remoteOpen: core.NewSignal(cx, false), remoteForm: core.NewSignal[*remoteForm](cx, nil),
		remoteName: core.NewSignal(cx, ""), remoteFetch: core.NewSignal(cx, ""), remotePush: core.NewSignal(cx, ""),
		remoteError: core.NewSignal(cx, ""), remoteListError: core.NewSignal(cx, ""),
	}
}

func (w *workbench) clearRemotes() {
	w.remoteReadID++
	w.remoteList.Set(nil)
	w.remoteOpen.Set(false)
	w.remoteListError.Set("")
	w.closeRemoteForm()
}

func (w *workbench) closeRemoteForm() {
	w.remoteForm.Set(nil)
	w.remoteName.Set("")
	w.remoteFetch.Set("")
	w.remotePush.Set("")
	w.remoteError.Set("")
}

func (w *workbench) loadRemotes() {
	child, ctx := w.core, w.repoCtx
	if child == nil {
		return
	}
	w.remoteReadID++
	id := w.remoteReadID
	go func() {
		items, err := child.repo.Remotes(ctx)
		w.cx.Post(func() {
			if ctx.Err() != nil || w.core != child || w.remoteReadID != id {
				return
			}
			if err != nil {
				w.remoteListError.Set(changesWriteError(err))
				return
			}
			w.remoteListError.Set("")
			previous := w.remoteList.Get()
			if len(items) == 0 || (len(previous) > 1) != (len(items) > 1) {
				w.remoteOpen.Set(len(items) > 1)
			}
			if !slices.EqualFunc(previous, items, gitcli.RemoteInfo.SameSelection) {
				w.remoteList.Set(items)
			}
		})
	}()
}

func (w *workbench) canManageRemotes() bool {
	return w.core != nil && w.canSync() && w.fetch.prompt.Get() == nil &&
		w.remoteForm.Get() == nil && w.worktreeForm.Get() == nil && w.worktreeRemove.Get() == nil
}

func (w *workbench) openRemoteForm(kind string, remote gitcli.RemoteInfo) {
	if !w.canManageRemotes() {
		return
	}
	w.remoteError.Set("")
	w.remoteName.Set(remote.Name)
	w.remoteFetch.Set(remote.FetchURL)
	w.remotePush.Set(remote.PushURL)
	w.remoteForm.Set(&remoteForm{kind: kind, child: w.core, key: w.changesWriteKey(), entry: remote})
}

func (w *workbench) canSubmitRemoteForm() bool {
	form := w.remoteForm.Get()
	if form == nil || form.child != w.core || form.key != w.changesWriteKey() || !w.canSync() {
		return false
	}
	return form.kind == "remove" ||
		strings.TrimSpace(w.remoteName.Get()) != "" && strings.TrimSpace(w.remoteFetch.Get()) != ""
}

func (w *workbench) submitRemoteForm() {
	if !w.canSubmitRemoteForm() {
		return
	}
	form := w.remoteForm.Get()
	name, fetch, push := strings.TrimSpace(w.remoteName.Get()), strings.TrimSpace(w.remoteFetch.Get()), strings.TrimSpace(w.remotePush.Get())
	w.remoteError.Set("")
	w.setChangesWrite(form.key, changesWrite{pending: true})
	ctx := w.appCtx
	go func() {
		var err error
		var message string
		switch form.kind {
		case "add":
			err = form.child.repo.AddRemote(ctx, name, fetch)
			message = fmt.Sprintf(w.tr("Remote %q added", "已添加远程「%s」"), name)
		case "edit":
			err = form.child.repo.EditRemote(ctx, form.entry, name, fetch, push)
			message = fmt.Sprintf(w.tr("Remote %q updated", "已更新远程「%s」"), name)
		case "remove":
			err = form.child.repo.RemoveRemote(ctx, form.entry)
			message = fmt.Sprintf(w.tr("Remote %q removed", "已移除远程「%s」"), form.entry.Name)
		}
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			w.setChangesWrite(form.key, changesWrite{})
			if w.core != form.child || w.changesWriteKey() != form.key {
				return
			}
			if err != nil {
				w.remoteError.Set(changesWriteError(err))
				w.loadRemotes()
				w.refreshRepository(false)
				return
			}
			w.closeRemoteForm()
			w.refNotice.Set(referenceNotice{text: message})
			w.refreshRepository(false)
		})
	}()
}

func (w *workbench) fetchSelectedRemote(remote gitcli.RemoteInfo) {
	if w.canManageRemotes() {
		w.runSync(syncRequest{transfer: gitcli.FetchOptions{Remote: remote.Name, Expected: &remote}})
	}
}
