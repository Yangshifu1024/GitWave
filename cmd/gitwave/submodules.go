package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
)

type submoduleUI struct {
	submodules         *core.Signal[[]gitcli.SubmoduleInfo]
	submodulesOpen     *core.Signal[bool]
	submodulesReady    *core.Signal[bool]
	submoduleError     *core.Signal[string]
	submoduleListError *core.Signal[string]
	submoduleForm      *core.Signal[*submoduleForm]
	submoduleURL       *core.Signal[string]
	submodulePath      *core.Signal[string]
	submoduleCancel    *core.Signal[context.CancelFunc]
	submoduleAuth      inlineAuthUI
	submoduleReadID    uint64
}

type submoduleForm struct {
	entry     gitcli.SubmoduleInfo
	plan      *gitcli.SubmoduleUpdatePlan
	adding    bool
	add       *gitcli.SubmoduleAddPlan
	challenge *gitcli.RemoteAuthError
}

func newSubmoduleUI(cx *core.Scope) submoduleUI {
	return submoduleUI{
		submodules:         core.NewSignalFunc(cx, []gitcli.SubmoduleInfo(nil), nil),
		submodulesOpen:     core.NewSignal(cx, true),
		submodulesReady:    core.NewSignal(cx, false),
		submoduleError:     core.NewSignal(cx, ""),
		submoduleListError: core.NewSignal(cx, ""),
		submoduleForm:      core.NewSignal[*submoduleForm](cx, nil),
		submoduleURL:       core.NewSignal(cx, ""),
		submodulePath:      core.NewSignal(cx, ""),
		submoduleCancel:    core.NewSignalFunc(cx, context.CancelFunc(nil), nil),
		submoduleAuth:      newInlineAuthUI(cx),
	}
}

func (w *workbench) clearSubmodules() {
	if cancel := w.submoduleCancel.Get(); cancel != nil {
		cancel()
		w.submoduleCancel.Set(nil)
	}
	w.submoduleReadID++
	w.submodules.Set(nil)
	w.submodulesOpen.Set(true)
	w.submodulesReady.Set(false)
	w.submoduleError.Set("")
	w.submoduleListError.Set("")
	w.closeSubmoduleForm()
}

func (w *workbench) closeSubmoduleForm() {
	w.submoduleForm.Set(nil)
	w.submoduleURL.Set("")
	w.submodulePath.Set("")
	w.submoduleAuth.reset()
}

func (w *workbench) loadSubmodules() {
	child, ctx := w.core, w.repoCtx
	if child == nil {
		return
	}
	w.submoduleReadID++
	id := w.submoduleReadID
	go func() {
		items, err := child.repo.Submodules(ctx)
		w.cx.Post(func() {
			if ctx.Err() != nil || w.core != child || w.submoduleReadID != id {
				return
			}
			w.submodulesReady.Set(true)
			if err != nil {
				w.submoduleListError.Set(changesWriteError(err))
				return
			}
			w.submoduleListError.Set("")
			if !slices.Equal(items, w.submodules.Get()) {
				if len(items) == 0 {
					w.submodulesOpen.Set(true)
				}
				w.submodules.Set(items)
			}
		})
	}()
}

func (w *workbench) hasSubmoduleWorktree() bool {
	return w.submodulesReady.Get() && w.core != nil && w.core.repo.WorkTree != ""
}

func (w *workbench) canEditSubmodules() bool {
	return w.hasSubmoduleWorktree() && w.canSync() && w.fetch.prompt.Get() == nil
}

func (w *workbench) currentSubmodule(previous gitcli.SubmoduleInfo) (gitcli.SubmoduleInfo, bool) {
	for _, item := range w.submodules.Get() {
		if item.Name == previous.Name && item.Path == previous.Path {
			return item, true
		}
	}
	return gitcli.SubmoduleInfo{}, false
}

func (w *workbench) runSubmodule(entry gitcli.SubmoduleInfo, action string) {
	if !w.canEditSubmodules() {
		return
	}
	form := w.submoduleForm.Get()
	if action == "init" || action == "update" {
		current, found := w.currentSubmodule(entry)
		if !found {
			return
		}
		entry = current
	}
	transfer := gitcli.FetchOptions{Vault: w.remoteCredentials()}
	var updatePlan *gitcli.SubmoduleUpdatePlan
	var addPlan *gitcli.SubmoduleAddPlan
	if action == "retry" {
		if form == nil || form.challenge == nil || w.submoduleAuth.incomplete() {
			return
		}
		transfer.Challenge, transfer.Credentials = form.challenge, w.submoduleAuth.credentials()
		updatePlan, addPlan = form.plan, form.add
	}
	adding := action == "add" || addPlan != nil
	source, relative := strings.TrimSpace(w.submoduleURL.Get()), strings.TrimSpace(w.submodulePath.Get())
	if action == "add" && (form == nil || !form.adding || form.challenge != nil || source == "" || relative == "") {
		return
	}
	child, key := w.core, w.changesWriteKey()
	ctx, cancel := context.WithCancel(w.appCtx)
	ctx = w.networkContext(ctx)
	if adding || action == "update" || action == "retry" {
		w.submoduleCancel.Set(cancel)
	}
	w.submoduleError.Set("")
	w.setChangesWrite(key, changesWrite{pending: true})
	go func() {
		defer cancel()
		entered := transfer.Credentials
		credential, err := gitcli.ResolveHTTPSCredential(ctx, transfer.Vault, transfer.Challenge, entered)
		transfer.Credentials = credential
		var result gitcli.SubmoduleUpdateResult
		var message string
		switch {
		case err != nil:
		case action == "init":
			err = child.repo.InitSubmodule(ctx, entry)
			message = fmt.Sprintf(w.tr("Initialized submodule %q", "已初始化子模块「%s」"), entry.Name)
		case action == "deinit":
			err = child.repo.DeinitSubmodule(ctx, entry)
			message = fmt.Sprintf(w.tr("Deactivated submodule %q", "已取消注册子模块「%s」"), entry.Name)
		case action == "add" || action == "update" || action == "retry":
			if adding {
				if addPlan == nil {
					addPlan, err = child.repo.PrepareSubmoduleAdd(ctx, source, relative)
				}
				if err == nil {
					result.Warning, err = addPlan.Apply(ctx, transfer)
				}
				message = fmt.Sprintf(w.tr("Added submodule %q", "已添加子模块「%s」"), relative)
			} else {
				if updatePlan == nil {
					updatePlan, err = child.repo.PrepareSubmoduleUpdate(ctx, entry)
				}
				if err == nil {
					result, err = updatePlan.Apply(ctx, transfer)
				}
				message = fmt.Sprintf(w.tr("Updated submodule %q", "已更新子模块「%s」"), entry.Name)
			}
		}
		items, readErr := child.repo.Submodules(w.appCtx)
		w.cx.Post(func() {
			if w.appCtx.Err() != nil {
				return
			}
			w.setChangesWrite(key, changesWrite{})
			if w.core != child || w.changesWriteKey() != key {
				return
			}
			w.submoduleCancel.Set(nil)
			if readErr == nil {
				w.submoduleReadID++
				w.submoduleListError.Set("")
				w.submodules.Set(items)
			}
			if errors.Is(err, context.Canceled) {
				w.closeSubmoduleForm()
				w.workspaceNotice.Set(w.tr("Submodule operation canceled", "已取消子模块操作"))
			} else if errors.Is(err, gitcli.ErrSavedHTTPSCredential) {
				w.submoduleAuth.username.Set(entered.Username)
				w.submoduleAuth.remember.Set(entered.Remember)
				w.submoduleError.Set(w.savedRemoteCredentialUnavailable())
			} else if err != nil {
				var challenge *gitcli.RemoteAuthError
				if errors.As(err, &challenge) {
					w.submoduleAuth.reset()
					w.submoduleForm.Set(&submoduleForm{entry: entry, plan: updatePlan, adding: adding,
						add: addPlan, challenge: challenge})
				} else {
					if form != nil && form.challenge != nil {
						w.closeSubmoduleForm()
						if adding {
							w.submoduleForm.Set(&submoduleForm{adding: true})
							w.submoduleURL.Set(source)
							w.submodulePath.Set(relative)
						}
					}
					w.submoduleError.Set(changesWriteError(err))
				}
			} else {
				w.closeSubmoduleForm()
				if result.Warning != "" {
					message += " · " + result.Warning
				}
				w.workspaceNotice.Set(message)
			}
			w.refreshRepository(false)
		})
	}()
}
