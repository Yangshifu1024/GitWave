package main

import (
	"context"
	"errors"
	"fmt"

	"gitwave.work/internal/gitcli"
	"gitwave.work/internal/workspace"
)

type refreshSummary struct{ ok, failed int }

func fetchWorkspaceRepos(ctx context.Context, repos []workspace.Repo, options gitcli.FetchOptions) (refreshSummary, error) {
	var summary refreshSummary
	for _, item := range repos {
		if item.Missing {
			continue
		}
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		repo, err := gitcli.OpenRoot(ctx, item.Path)
		if err == nil {
			_, err = repo.Fetch(ctx, options)
		}
		if ctx.Err() != nil {
			return summary, ctx.Err()
		}
		if err != nil {
			summary.failed++
		} else {
			summary.ok++
		}
	}
	return summary, nil
}

func (w *workbench) refreshFromRemotes(job refreshJob, state workspace.State) {
	reason := job.reason
	if reason < refreshAutomatic || !w.sameRefreshTarget(job) || w.repoCtx == nil || !w.canSync() || w.fetch.prompt.Get() != nil ||
		(reason == refreshAutomatic && (!w.refreshPrefs.Get().Enabled || w.refreshSettingsBusy.Get())) {
		return
	}
	path := w.path.Get()
	if path == "" {
		return
	}
	parent := w.repoCtx
	if reason == refreshAutomatic && job.ws != "" {
		parent = w.appCtx
	}
	ctx, cancel := context.WithCancel(parent)
	ctx = w.networkContext(ctx)
	action := &fetchOperation{key: w.changesWriteKey(), workspace: job.ws, kind: "refresh", cancel: cancel}
	w.fetch.progress.Set(fetchProgress{})
	w.fetch.notice.Set(nil)
	w.fetch.running.Set(action)
	w.setChangesWrite(action.key, changesWrite{pending: true})
	vault := w.remoteCredentials()
	go func() {
		defer cancel()
		var summary refreshSummary
		var err error
		options := gitcli.FetchOptions{Vault: vault, NonInteractive: reason == refreshAutomatic, Progress: func(name string, p gitcli.TransferProgress) {
			w.cx.Post(func() {
				if w.fetch.running.Get() == action && ctx.Err() == nil {
					w.fetch.progress.Set(fetchProgress{remote: name, TransferProgress: p})
				}
			})
		}}
		if reason == refreshAutomatic && state.Workspace.ID != "" {
			summary, err = fetchWorkspaceRepos(ctx, state.Repos, options)
		} else {
			open := gitcli.Open
			if state.Workspace.ID != "" {
				open = gitcli.OpenRoot
			}
			var repo *gitcli.Repository
			repo, err = open(ctx, path)
			if err == nil {
				_, err = repo.Fetch(ctx, options)
				if err == nil {
					summary.ok++
				}
			}
		}
		w.cx.Post(func() {
			if w.appCtx.Err() != nil || w.fetch.running.Get() != action {
				return
			}
			w.fetch.running.Set(nil)
			w.setChangesWrite(action.key, changesWrite{})
			if !action.inView(w) {
				return
			}
			w.refreshRepository(false)
			switch {
			case errors.Is(err, context.Canceled):
				w.showSyncNotice("refresh", w.tr("Cancelled", "已取消"), false)
			case err != nil:
				w.showSyncNotice("refresh", err.Error(), true)
			case reason == refreshRemoteManual:
				w.showSyncNotice("refresh", w.tr("Repository refreshed", "仓库已刷新"), false)
			case summary.failed != 0:
				message := w.tr("Refreshed %d/%d repositories (%d failed)", "已刷新 %d/%d 个仓库（%d 个失败）")
				w.showSyncNotice("refresh", fmt.Sprintf(message, summary.ok, summary.ok+summary.failed, summary.failed), false)
			default:
				w.showSyncNotice("refresh", fmt.Sprintf(w.tr("Refreshed %d repositories", "已刷新 %d 个仓库"), summary.ok), false)
			}
		})
	}()
}
