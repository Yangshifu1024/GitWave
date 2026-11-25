package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

type fetchUI struct {
	running  *core.Signal[*fetchOperation]
	prompt   *core.Signal[*fetchPrompt]
	progress *core.Signal[fetchProgress]
	notice   *core.Signal[*fetchNotice]
	auth     inlineAuthUI
}

type fetchOperation struct {
	key       changesWriteKey
	workspace string
	kind      string
	reviewing bool
	cancel    context.CancelFunc
}

func (action *fetchOperation) inView(w *workbench) bool {
	if action.workspace != "" {
		return action.workspace == w.workspaceState.Get().Workspace.ID
	}
	return action.key == w.changesWriteKey()
}

type syncRequest struct {
	transfer     gitcli.FetchOptions
	pull         *gitcli.PullOptions
	plan         *gitcli.PullPlan
	push         *gitcli.PushOptions
	pushPlan     *gitcli.PushPlan
	review       *gitcli.PushReview
	remoteDelete *gitcli.RemoteBranchDeletePlan
}

const remoteDeleteSync = "delete-remote-branch"

func (r syncRequest) kind() string {
	if r.remoteDelete != nil {
		return remoteDeleteSync
	}
	if r.push != nil {
		return "push"
	}
	if r.pull != nil {
		return "pull"
	}
	return "fetch"
}

type fetchPrompt struct {
	key       changesWriteKey
	child     *backend
	challenge *gitcli.RemoteAuthError
	request   syncRequest
	pushed    int
	deleted   gitcli.RemoteBranchDeleteResult
	failure   string
}

type fetchProgress struct {
	remote string
	gitcli.TransferProgress
}

type fetchNotice struct {
	key     changesWriteKey
	kind    string
	message string
	failed  bool
}

func newFetchUI(cx *core.Scope) fetchUI {
	return fetchUI{
		running:  core.NewSignal(cx, (*fetchOperation)(nil)),
		prompt:   core.NewSignal(cx, (*fetchPrompt)(nil)),
		progress: core.NewSignal(cx, fetchProgress{}),
		notice:   core.NewSignal(cx, (*fetchNotice)(nil)),
		auth:     newInlineAuthUI(cx),
	}
}

func (w *workbench) canSync() bool {
	return w.refForm.Get() == nil && w.canSyncRepository()
}

func (w *workbench) canSyncRepository() bool {
	return w.path.Get() != "" && !w.workspaceBlocked() && w.workspaceModal.Get().kind == "" &&
		w.repoEditor.Get() == nil && w.ignoreForm.Get() == nil &&
		w.checkoutPrompt.Get() == (checkoutPrompt{}) && w.pull.form.Get() == nil && w.conflictPanel.Get() == nil &&
		w.push.form.Get() == nil && w.push.review.Get() == nil &&
		!w.currentChangesWrite().pending && w.fetch.running.Get() == nil
}

func (w *workbench) startFetch(remote string) {
	if w.fetch.prompt.Get() == nil {
		w.runSync(syncRequest{transfer: gitcli.FetchOptions{Remote: remote}})
	}
}

func (w *workbench) runSync(request syncRequest) {
	child := w.core
	if child == nil || w.repoCtx == nil || !w.canSync() {
		return
	}
	ctx, cancel := context.WithCancel(w.repoCtx)
	ctx = w.networkContext(ctx)
	request.transfer.Vault = w.remoteCredentials()
	previousPrompt := w.fetch.prompt.Get()
	kind := request.kind()
	action := &fetchOperation{key: w.changesWriteKey(), kind: kind, cancel: cancel}
	action.reviewing = request.push != nil && request.push.Force && request.review == nil
	w.fetch.prompt.Set(nil)
	w.fetch.auth.reset()
	w.fetch.notice.Set(nil)
	w.fetch.progress.Set(fetchProgress{remote: request.transfer.Remote})
	w.fetch.running.Set(action)
	if kind == "pull" {
		w.beginChangesWrite(action.key)
	} else {
		w.setChangesWrite(action.key, changesWrite{pending: true})
	}
	request.transfer.Progress = func(remote string, p gitcli.TransferProgress) {
		w.cx.Post(func() {
			if w.fetch.running.Get() == action && ctx.Err() == nil {
				w.fetch.progress.Set(fetchProgress{remote: remote, TransferProgress: p})
			}
		})
	}
	go func() {
		defer cancel()
		var fetched gitcli.FetchResult
		var pulled gitcli.PullResult
		var pushed gitcli.PushResult
		var deleted gitcli.RemoteBranchDeleteResult
		var review *gitcli.PushReview
		entered := request.transfer.Credentials
		credential, err := gitcli.ResolveHTTPSCredential(ctx, request.transfer.Vault, request.transfer.Challenge, entered)
		request.transfer.Credentials = credential
		switch {
		case err != nil:
		case kind == remoteDeleteSync:
			deleted, err = request.remoteDelete.Apply(ctx, request.transfer)
		case kind == "push":
			if request.pushPlan == nil {
				request.pushPlan, err = child.repo.PreparePush(ctx, *request.push)
			}
			if err == nil && request.push.Force && request.review == nil {
				review, err = request.pushPlan.Review(ctx, request.transfer)
			} else if err == nil {
				pushed, err = request.pushPlan.Apply(ctx, request.transfer, request.review)
			}
		case kind == "pull":
			if request.plan == nil {
				request.plan, err = child.repo.PreparePull(ctx, *request.pull)
			}
			if err == nil {
				pulled, err = request.plan.Apply(ctx, request.transfer)
			}
		default:
			fetched, err = child.repo.Fetch(ctx, request.transfer)
		}
		w.cx.Post(func() {
			if w.appCtx.Err() != nil || w.fetch.running.Get() != action {
				return
			}
			w.fetch.running.Set(nil)
			w.setChangesWrite(action.key, changesWrite{})
			if w.core != child || w.changesWriteKey() != action.key {
				return
			}
			if len(fetched.Completed) > 0 || pulled.Fetched || pushed.Branches > 0 || deleted.Local != "" || len(deleted.Deleted) != 0 {
				w.refreshRepository(false)
			} else if kind == "pull" && w.changesMode.Get() {
				w.loadChanges()
			}
			var challenge *gitcli.RemoteAuthError
			switch {
			case errors.Is(err, gitcli.ErrSavedHTTPSCredential):
				request.transfer.Progress = nil
				w.fetch.auth.username.Set(entered.Username)
				w.fetch.auth.remember.Set(entered.Remember)
				prompt := &fetchPrompt{key: action.key, child: child, challenge: request.transfer.Challenge,
					request: request, failure: w.savedRemoteCredentialUnavailable()}
				if previousPrompt != nil && previousPrompt.challenge.SameChallenge(prompt.challenge) {
					prompt.pushed, prompt.deleted = previousPrompt.pushed, previousPrompt.deleted
				}
				w.fetch.prompt.Set(prompt)
			case review != nil:
				request.review = review
				request.transfer = gitcli.FetchOptions{Remote: request.push.Remote.Name}
				w.push.review.Set(&pushReviewState{key: action.key, child: child, request: request})
			case errors.Is(err, context.Canceled):
				message := w.tr("Cancelled", "已取消")
				if kind == "push" && pushed.Attempted {
					message = w.tr("Push cancelled. The remote may have applied updates.", "已取消推送。远端可能已应用部分更新。")
				}
				if kind == remoteDeleteSync {
					if deleted.Attempted {
						message = w.tr("Cancelled. Check the remote for any unconfirmed deletion.", "已取消。请核对远端是否执行了尚未确认的删除。")
					}
					message = w.remoteDeleteNotice(deleted, message)
				}
				w.showSyncNotice(kind, message, false)
			case errors.As(err, &challenge) && (request.transfer.Credentials == nil || !challenge.SameChallenge(request.transfer.Challenge)):
				request.transfer.Progress = nil
				w.fetch.prompt.Set(&fetchPrompt{key: action.key, child: child, challenge: challenge, request: request,
					pushed: pushed.Branches, deleted: deleted})
			case kind == remoteDeleteSync:
				message := ""
				if err != nil {
					message = w.tr("Remote branch deletion failed", "删除远端分支失败") + " · " + err.Error()
				}
				w.showSyncNotice(kind, w.remoteDeleteNotice(deleted, message), err != nil || len(deleted.Warnings) != 0)
			case err != nil:
				message := w.tr("Fetch failed", "获取失败")
				if kind == "push" {
					message = w.tr("Push failed", "推送失败")
					if pushed.Branches > 0 {
						message = fmt.Sprintf(w.tr("Pushed the branch to %d destinations; later updates failed", "分支已推送到 %d 个目的地，后续更新失败"), pushed.Branches)
					}
				} else if kind == "pull" {
					message = w.tr("Pull failed", "拉取失败")
				} else if len(fetched.Completed) != 0 {
					message = fmt.Sprintf(w.tr("Fetched %d remotes before an error", "已获取 %d 个远端，部分失败"), len(fetched.Completed))
				}
				w.showSyncNotice(kind, message+": "+err.Error(), true)
			case kind == "push":
				message := fmt.Sprintf(w.tr("Pushed to %s", "已推送至 %s"), request.push.Remote.Name)
				if len(pushed.SkippedTags) != 0 {
					pattern := w.tr("Pushed to %s; skipped tags that differ on the remote: %s", "已推送至 %s，跳过与远端不同的标签：%s")
					message = fmt.Sprintf(pattern, request.push.Remote.Name, strings.Join(pushed.SkippedTags, ", "))
				}
				if len(pushed.Warnings) != 0 {
					message += " · " + strings.Join(pushed.Warnings, " · ")
				}
				w.showSyncNotice(kind, message, len(pushed.Warnings) != 0)
			case kind == "pull":
				message := fmt.Sprintf(w.tr("Pulled from %s", "已从 %s 拉取"), request.pull.Remote)
				if pulled.UpToDate {
					message = w.tr("Already up to date", "已经是最新状态")
				}
				if pulled.Warning != "" {
					message += " · " + pulled.Warning
				}
				w.showSyncNotice(kind, message, pulled.Warning != "")
			case len(fetched.Completed) == 0:
				w.showSyncNotice(kind, w.tr("No remotes configured", "尚未配置远端"), false)
			default:
				message := w.tr("Fetched", "已获取")
				if request.transfer.Remote != "" && len(fetched.Completed) == 1 {
					message = fmt.Sprintf(w.tr("Fetched from %s", "已从 %s 获取"), fetched.Completed[0])
				}
				if len(fetched.Warnings) != 0 {
					message += " · " + strings.Join(fetched.Warnings, " · ")
				}
				w.showSyncNotice(kind, message, false)
			}
		})
	}()
}

func (w *workbench) showSyncNotice(kind, message string, failed bool) {
	notice := &fetchNotice{key: w.changesWriteKey(), kind: kind, message: message, failed: failed}
	w.fetch.notice.Set(notice)
	w.cx.AfterFunc(15*time.Second, func() {
		if w.fetch.notice.Get() == notice {
			w.fetch.notice.Set(nil)
		}
	})
}

func (w *workbench) fetchStatusView(chrome palette) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		if action := w.fetch.running.Get(); action != nil && action.inView(w) {
			label := func() string {
				p := w.fetch.progress.Get()
				if action.reviewing {
					return w.tr("Reading remote references…", "正在读取远端引用…")
				}
				if action.kind == "refresh" {
					return w.tr("Refreshing…", "正在刷新…")
				}
				if action.kind == "push" {
					if p.Total != 0 {
						return fmt.Sprintf(w.tr("Pushing to %s… %d%%", "正在推送至 %s… %d%%"), p.remote, p.Percent)
					}
					return fmt.Sprintf(w.tr("Pushing to %s…", "正在推送至 %s…"), p.remote)
				}
				if action.kind == "pull" {
					if p.Phase == gitcli.TransferLFSDownload {
						return w.tr("Downloading Git LFS objects…", "正在下载 Git LFS 对象…")
					}
					if p.remote != "" && p.Total != 0 {
						return fmt.Sprintf(w.tr("Pulling from %s… %d%%", "正在从 %s 拉取… %d%%"), p.remote, p.Percent)
					}
					return w.tr("Pulling changes…", "正在拉取变更…")
				}
				if action.kind == remoteDeleteSync {
					if p.remote != "" {
						return fmt.Sprintf(w.tr("Deleting branch on %s…", "正在删除 %s 上的分支…"), p.remote)
					}
					return w.tr("Deleting branch…", "正在删除分支…")
				}
				if p.remote != "" && p.Total != 0 {
					return fmt.Sprintf(w.tr("Fetching %s… %d%%", "正在获取 %s… %d%%"), p.remote, p.Percent)
				}
				return w.tr("Fetching…", "正在获取…")
			}
			cancel := toolbarAction(chrome, w.tr("Cancel", "取消"), action.cancel, chrome.label(w.tr("Cancel", "取消"), chrome.muted, 11))
			cancel.TestID = action.kind + "-cancel"
			progress := w.tr("Fetch progress", "获取进度")
			if action.kind == "pull" {
				progress = w.tr("Pull progress", "拉取进度")
			} else if action.kind == "push" {
				progress = w.tr("Push progress", "推送进度")
			} else if action.kind == remoteDeleteSync {
				progress = w.tr("Remote branch deletion progress", "删除远端分支进度")
			}
			return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
				Children: []core.Node{
					core.Text{ContentFunc: label, Role: core.RoleStatus, TestID: action.kind + "-status",
						Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.accent}},
					ui.Progress{Label: progress,
						Value:         func() float32 { return float32(w.fetch.progress.Get().Percent) },
						Indeterminate: func() bool { return w.fetch.progress.Get().Total == 0 },
						Style:         core.Style{Width: core.Pt(60)}},
					cancel,
				}}
		}
		if notice := w.fetch.notice.Get(); notice != nil && notice.key == w.changesWriteKey() {
			color := chrome.muted
			if notice.failed {
				color = chrome.danger
			}
			return core.Text{Content: notice.message, Role: core.RoleStatus, TestID: notice.kind + "-status", MaxLines: 1,
				Style: core.Style{FontSize: chrome.uiSize(11), Color: color, FlexShrink: 1}}
		}
		return core.View{}
	})
}

func (w *workbench) fetchAuthDialog(chrome palette) core.Node {
	return core.Show{When: func() bool {
		return w.fetch.prompt.Get() != nil && w.workspaceModal.Get().kind == "" &&
			w.checkoutPrompt.Get() == (checkoutPrompt{}) && w.refForm.Get() == nil && w.pull.form.Get() == nil &&
			w.push.form.Get() == nil && w.push.review.Get() == nil
	}, Then: func(*core.Scope) core.Node {
		prompt := w.fetch.prompt.Get()
		valid := func() bool {
			return w.fetch.prompt.Get() == prompt && w.core == prompt.child && w.changesWriteKey() == prompt.key
		}
		submit := func() {
			if auth := w.fetch.auth.credentials(); valid() && auth != nil {
				request := prompt.request
				request.transfer.Remote = prompt.challenge.Remote
				request.transfer.Challenge = prompt.challenge
				request.transfer.Credentials = auth
				w.runSync(request)
			}
		}
		cancel := func() {
			if valid() {
				w.fetch.prompt.Set(nil)
				w.fetch.auth.reset()
				message := w.tr("Cancelled", "已取消")
				if prompt.pushed > 0 {
					message = fmt.Sprintf(w.tr("Cancelled after pushing the branch to %d of the selected destinations",
						"分支已推送到 %d 个目的地，后续操作已取消"), prompt.pushed)
				}
				if prompt.request.kind() == remoteDeleteSync {
					message = w.remoteDeleteNotice(prompt.deleted, message)
				}
				w.showSyncNotice(prompt.request.kind(), message, false)
			}
		}
		retryLabel := w.tr("Save and retry", "保存并重试")
		retry := toolbarAction(chrome, retryLabel, submit, chrome.label(retryLabel, chrome.text, 12))
		retry.Disabled = w.fetch.auth.incomplete
		description := w.remoteAuthDescription(prompt.challenge)
		if prompt.pushed > 0 {
			description = fmt.Sprintf(w.tr("The branch was already pushed to %d of the selected destinations.", "分支已推送到 %d 个目的地。"),
				prompt.pushed) + " " + description
		}
		if prompt.failure != "" {
			description += "\n" + prompt.failure
		}
		if prompt.request.kind() == remoteDeleteSync {
			if summary := w.remoteBranchDeleteSummary(prompt.deleted); summary != "" {
				description = summary + " · " + description
			}
		}
		style := core.Style{Height: core.Pt(32), Padding: core.All(6), BorderWidth: 1, BorderColor: chrome.border}
		prefix := prompt.request.kind()
		return ui.Dialog{TestID: prefix + "-auth-dialog", Title: w.tr("Authentication required", "需要认证"), Description: description,
			Open: func() bool { return w.fetch.prompt.Get() == prompt },
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					cancel()
				}
			},
			Children: []core.Node{w.inlineAuthInputs(prefix, w.fetch.auth, style, submit)},
			Footer:   []core.Node{action(chrome, w.tr("Cancel", "取消"), cancel), retry}}
	}}
}
