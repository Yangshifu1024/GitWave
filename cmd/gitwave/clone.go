package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"gitwave.work/internal/gitcli"
	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

type cloneUI struct {
	url         *core.Signal[string]
	progress    *core.Signal[gitcli.TransferProgress]
	replacement *core.Signal[*cloneReplacement]
	ready       *core.Signal[*cloneRegistration]
	auth        *core.Signal[*cloneAuthRequest]
	inlineAuth  inlineAuthUI
	cancel      context.CancelFunc
	suggested   string
}

type cloneReplacement struct {
	target      gitcli.CloneTarget
	source      string
	destination string
}

func (*cloneReplacement) Error() string { return "confirm replacement of the clone destination" }

type cloneRegistration struct {
	result gitcli.CloneResult
	err    error
}

func (r *cloneRegistration) Error() string { return r.err.Error() }
func (r *cloneRegistration) Unwrap() error { return r.err }

type cloneAuthRequest struct {
	challenge     *gitcli.RemoteAuthError
	selection     cloneReplacement
	gitAuth       *gitcli.HTTPSCredential
	storedFailure *gitcli.HTTPSCredential
}

func (*cloneAuthRequest) Error() string { return gitcli.ErrHTTPSAuthentication.Error() }

func newCloneUI(cx *core.Scope) cloneUI {
	return cloneUI{
		url:         core.NewSignal(cx, ""),
		progress:    core.NewSignal(cx, gitcli.TransferProgress{}),
		replacement: core.NewSignal(cx, (*cloneReplacement)(nil)),
		ready:       core.NewSignal(cx, (*cloneRegistration)(nil)),
		auth:        core.NewSignal(cx, (*cloneAuthRequest)(nil)),
		inlineAuth:  newInlineAuthUI(cx),
	}
}

func (w *workbench) cloneRepoMenuItem() core.MenuItem {
	ws := w.workspaceState.Get().Workspace
	return core.MenuItem{Label: w.tr("Clone remote repo", "克隆远程仓库"),
		Disabled: ws.ID == "" || w.workspaceBlocked(), OnSelect: func() {
			if w.workspaceModal.Get().kind != "" {
				return
			}
			w.clone.url.Set("")
			w.clone.progress.Set(gitcli.TransferProgress{})
			w.clone.replacement.Set(nil)
			w.clone.ready.Set(nil)
			w.clearCloneAuth()
			w.clone.suggested = ""
			w.showWorkspaceForm(workspaceDialog{kind: "clone", ws: ws}, "")
		},
	}
}

func (w *workbench) submitClone(dialog workspaceDialog) {
	if w.workspaceModal.Get() != dialog || w.workspaceBlocked() {
		return
	}
	remote, path := strings.TrimSpace(w.clone.url.Get()), w.workspaceInput.Get()
	if remote == "" || strings.TrimSpace(path) == "" {
		return
	}
	w.clone.progress.Set(gitcli.TransferProgress{})
	confirmed, ready := w.clone.replacement.Get(), w.clone.ready.Get()
	pending := w.clone.auth.Get()
	var gitAuth, mediaAuth *gitcli.HTTPSCredential
	var entered *gitcli.HTTPSCredential
	var challenge *gitcli.RemoteAuthError
	if pending != nil {
		if pending.selection.source != remote || pending.selection.destination != path {
			w.clearCloneAuth()
			return
		}
		entered = w.clone.inlineAuth.credentials()
		if entered == nil {
			return
		}
		challenge, gitAuth = pending.challenge, pending.gitAuth
		w.clearCloneAuth()
	}
	ctx, cancel := context.WithCancel(w.appCtx)
	ctx = w.networkContext(ctx)
	vault := w.remoteCredentials()
	w.clone.cancel = cancel
	action := w.workspaceAction + 1
	w.runWorkspace(func(appCtx context.Context) (workspaceResult, error) {
		defer cancel()
		currentWorkspace, err := w.workspaceStore.LoadWorkspace(ctx, dialog.ws.ID)
		if err != nil {
			return workspaceResult{}, err
		}
		if currentWorkspace.Workspace.ID == "" {
			return workspaceResult{}, errors.New(w.tr("The workspace no longer exists", "该工作区已不存在"))
		}
		var result gitcli.CloneResult
		if ready != nil {
			result = ready.result
			current, err := gitcli.OpenRoot(ctx, result.Repo.Path())
			if err != nil {
				return workspaceResult{}, &cloneRegistration{result: result, err: err}
			}
			if !result.Repo.SameCheckout(current) {
				return workspaceResult{}, errors.New(w.tr("The cloned repository changed before it could be added", "克隆的仓库在添加前发生了变化"))
			}
		} else {
			target, replace := gitcli.CloneTarget{}, false
			checkDestination := func(ctx context.Context, path string) error {
				return w.workspaceStore.CheckCloneDestination(ctx, dialog.ws.ID, path)
			}
			if pending != nil {
				target, replace = pending.selection.target, pending.selection.target.NeedsReplacement()
			} else if confirmed != nil && confirmed.source == remote && confirmed.destination == path {
				target, replace = confirmed.target, true
			} else {
				var err error
				target, err = gitcli.PrepareClone(ctx, path)
				if err != nil {
					return workspaceResult{}, err
				}
				if err := checkDestination(ctx, target.Path()); err != nil {
					return workspaceResult{}, err
				}
				if target.NeedsReplacement() {
					return workspaceResult{}, &cloneReplacement{target: target, source: remote, destination: path}
				}
			}
			credential, err := gitcli.ResolveHTTPSCredential(ctx, vault, challenge, entered)
			if errors.Is(err, gitcli.ErrSavedHTTPSCredential) {
				return workspaceResult{}, &cloneAuthRequest{challenge: challenge, gitAuth: gitAuth,
					selection: pending.selection, storedFailure: entered}
			}
			if err != nil {
				return workspaceResult{}, err
			}
			if credential != nil {
				if challenge.LFSLocation() == "" {
					gitAuth = credential
				} else {
					mediaAuth = credential
				}
			}
			result, err = target.Clone(ctx, remote, replace, gitcli.CloneOptions{
				Credentials: gitAuth, MediaCredentials: mediaAuth, Challenge: challenge, Vault: vault,
				BeforePublish: checkDestination, Progress: func(p gitcli.TransferProgress) {
					w.cx.Post(func() {
						if w.workspaceAction == action && w.workspaceBusy.Get() && ctx.Err() == nil {
							w.clone.progress.Set(p)
						}
					})
				}})
			if err != nil {
				var next *gitcli.RemoteAuthError
				if errors.As(err, &next) && !next.SameChallenge(challenge) {
					return workspaceResult{}, &cloneAuthRequest{challenge: next, gitAuth: gitAuth,
						selection: cloneReplacement{target: target, source: remote, destination: path}}
				}
				if result.Backup != "" {
					err = errors.Join(err, errors.New(w.cloneBackupNotice(result.Backup)))
				}
				return workspaceResult{}, err
			}
		}
		joined, err := w.registerWorkspaceRepo(appCtx, dialog.ws.ID, result.Repo)
		if err != nil {
			if result.Backup != "" {
				err = errors.Join(err, errors.New(w.cloneBackupNotice(result.Backup)))
			}
			return workspaceResult{}, &cloneRegistration{result: result, err: err}
		}
		if result.Backup != "" {
			joined.notice = w.cloneBackupNotice(result.Backup)
		}
		if result.Warning != "" {
			if joined.notice != "" {
				joined.notice += "\n"
			}
			joined.notice += result.Warning
		}
		return joined, nil
	})
}

func (w *workbench) cloneBackupNotice(path string) string {
	return fmt.Sprintf(w.tr("Original directory saved at %s", "原目录已保存在 %s"), path)
}

func (w *workbench) finishCloneAttempt(err error) bool {
	w.clone.cancel = nil
	w.clone.progress.Set(gitcli.TransferProgress{})
	w.clone.replacement.Set(nil)
	w.clone.ready.Set(nil)
	var replacement *cloneReplacement
	var ready *cloneRegistration
	var auth *cloneAuthRequest
	switch {
	case errors.As(err, &replacement):
		w.clone.replacement.Set(replacement)
		return true
	case errors.As(err, &ready):
		w.clone.ready.Set(ready)
	case errors.As(err, &auth):
		w.clearCloneAuth()
		if auth.storedFailure != nil {
			w.clone.inlineAuth.username.Set(auth.storedFailure.Username)
			w.clone.inlineAuth.remember.Set(auth.storedFailure.Remember)
			w.workspaceError.Set(w.savedRemoteCredentialUnavailable())
		}
		if auth.selection.target.NeedsReplacement() {
			w.clone.replacement.Set(&auth.selection)
		}
		w.clone.auth.Set(auth)
		return true
	case errors.Is(err, gitcli.ErrHTTPSAuthentication):
		w.workspaceError.Set(w.tr("HTTPS authentication failed", "HTTPS 认证失败"))
		return true
	case errors.Is(err, workspace.ErrCloneDestinationRegistered):
		w.workspaceError.Set(w.tr("The destination contains a repository registered in a workspace", "目标目录包含已登记到工作区的仓库"))
		return true
	case errors.Is(err, context.Canceled):
		w.workspaceError.Set(w.tr("Clone cancelled", "已取消克隆"))
		return true
	case errors.Is(err, gitcli.ErrCloneTargetChanged):
		w.workspaceError.Set(w.tr("The destination changed. Review it before retrying.", "目标目录已变化。请检查后重试。"))
		return true
	}
	return false
}

func (w *workbench) clearCloneAuth() {
	w.clone.auth.Set(nil)
	w.clone.inlineAuth.reset()
}

func (w *workbench) cloneDialog(chrome palette, dialog workspaceDialog) core.Node {
	title := w.tr("Clone remote repo", "克隆远程仓库")
	disabled := func() bool {
		return w.workspaceBusy.Get() || w.clone.ready.Get() != nil
	}
	change := func() {
		w.clone.replacement.Set(nil)
		w.workspaceError.Set("")
	}
	submit := func() { w.submitClone(dialog) }
	cancel := func() {
		if w.workspaceBusy.Get() {
			if w.clone.cancel != nil {
				w.clone.cancel()
			}
			return
		}
		if w.clone.auth.Get() != nil {
			w.clearCloneAuth()
			w.clone.replacement.Set(nil)
			w.workspaceError.Set(w.tr("Clone cancelled", "已取消克隆"))
			return
		}
		w.workspaceModal.Set(workspaceDialog{})
		w.workspaceError.Set("")
	}
	formStyle := core.Style{Height: core.Pt(32), Padding: core.All(6), BorderWidth: 1, BorderColor: chrome.border}
	source := core.Input{
		TestID: "clone-url", Label: w.tr("URL", "地址"), AutoFocus: true, Disabled: disabled,
		Placeholder: "https://github.com/user/repo.git",
		Value:       w.clone.url.Get,
		OnChange: func(value string) {
			change()
			w.clone.url.Set(value)
			if path := w.workspaceInput.Get(); path == "" || path == w.clone.suggested {
				w.clone.suggested = suggestedClonePath(value)
				w.workspaceInput.Set(w.clone.suggested)
			}
		},
		OnSubmit: func(string) { submit() }, Style: formStyle,
	}
	destination := core.Input{
		TestID: "clone-dest", Label: w.tr("Destination path", "目标路径"), Disabled: disabled,
		Placeholder: "." + string(filepath.Separator) + "repo", Value: w.workspaceInput.Get,
		OnChange: func(value string) {
			change()
			w.workspaceInput.Set(value)
		},
		OnSubmit: func(string) { submit() }, Style: formStyle.Merge(core.Style{FlexGrow: 1}),
	}
	label := func() string {
		switch {
		case w.workspaceBusy.Get():
			return w.tr("Cloning…", "正在克隆…")
		case w.clone.auth.Get() != nil:
			return w.tr("Save and retry", "保存并重试")
		case w.clone.replacement.Get() != nil:
			return w.tr("Replace and clone", "替换并克隆")
		case w.workspaceError.Get() != "":
			return w.tr("Retry", "重试")
		default:
			return w.tr("Clone", "克隆")
		}
	}
	confirm := toolbarAction(chrome, "", submit, core.Text{ContentFunc: label, Style: core.Style{Color: chrome.text, FontSize: chrome.uiSize(12)}})
	confirm.LabelFunc = label
	confirm.TestID = "clone-confirm"
	confirm.Disabled = func() bool {
		if w.clone.auth.Get() != nil {
			return w.clone.inlineAuth.incomplete()
		}
		return w.workspaceBusy.Get() || strings.TrimSpace(w.clone.url.Get()) == "" || strings.TrimSpace(w.workspaceInput.Get()) == ""
	}
	browse := toolbarAction(chrome, w.tr("Browse…", "浏览…"),
		func() { w.pickWorkspaceDirectory(dialog, title) }, chrome.label(w.tr("Browse…", "浏览…"), chrome.text, 12))
	browse.Disabled = disabled
	return ui.Dialog{
		TestID: "clone-dialog",
		TitleFunc: func() string {
			if w.clone.auth.Get() != nil {
				return w.tr("Authentication required", "需要认证")
			}
			return title
		},
		DescriptionFunc: func() string {
			if auth := w.clone.auth.Get(); auth != nil {
				return w.remoteAuthDescription(auth.challenge)
			}
			return w.tr("Copy a remote repository into a local folder.", "将远程仓库复制到本地文件夹。")
		},
		Open: func() bool { return w.workspaceModal.Get() == dialog },
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				cancel()
			}
		},
		Children: []core.Node{core.Show{When: func() bool { return w.clone.auth.Get() == nil }, Then: func(*core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
				chrome.label(source.Label, chrome.muted, 12), source,
				core.Text{ContentFunc: func() string {
					protocol := gitcli.CloneProtocol(w.clone.url.Get())
					if protocol == "" {
						protocol = "—"
					}
					return w.tr("Detected protocol: ", "检测到协议：") + protocol
				}, Style: core.Style{Color: chrome.muted, FontSize: chrome.uiSize(11)}},
				chrome.label(destination.Label, chrome.muted, 12),
				core.View{Style: core.Style{Direction: core.Row, Gap: 6}, Children: []core.Node{destination, browse}},
				core.Text{ContentFunc: func() string {
					choice := w.clone.replacement.Get()
					if choice != nil && choice.destination == w.workspaceInput.Get() {
						msg := w.tr("%s already contains files. The original directory will be kept as a backup.", "%s 已包含文件。原目录会保留为备份。")
						return fmt.Sprintf(msg, choice.target.Path())
					}
					return w.workspaceError.Get()
				}, Style: core.Style{Color: chrome.danger, FontSize: chrome.uiSize(12)}},
				w.cloneProgressView(chrome),
			}}
		}, Else: func(*core.Scope) core.Node {
			return w.inlineAuthInputs("clone", w.clone.inlineAuth, formStyle, submit)
		}}},
		Footer: []core.Node{action(chrome, w.tr("Cancel", "取消"), cancel), confirm},
	}
}

func (w *workbench) cloneProgressView(chrome palette) core.Node {
	return core.Show{When: w.workspaceBusy.Get, Then: func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
			ui.Progress{Label: w.tr("Clone progress", "克隆进度"),
				Value:         func() float32 { return float32(w.clone.progress.Get().Percent) },
				Indeterminate: func() bool { return w.clone.progress.Get().Total == 0 }},
			core.Text{ContentFunc: func() string {
				p := w.clone.progress.Get()
				var phase string
				switch p.Phase {
				case "Receiving objects":
					phase = w.tr("Receiving objects", "正在接收对象")
				case "Resolving deltas":
					phase = w.tr("Resolving deltas", "正在处理差异")
				case "Updating files", "Checking out files":
					phase = w.tr("Checking out files", "正在检出文件")
				case gitcli.TransferLFSDownload:
					return w.tr("Downloading Git LFS objects…", "正在下载 Git LFS 对象…")
				default:
					return w.tr("Starting clone…", "正在开始克隆…")
				}
				if p.Total == 0 {
					return phase
				}
				return fmt.Sprintf("%s %d/%d", phase, p.Completed, p.Total)
			}, Style: core.Style{Color: chrome.muted, FontSize: chrome.uiSize(11)}},
		}}
	}}
}

func suggestedClonePath(source string) string {
	source = strings.TrimRight(strings.TrimSpace(source), "/\\")
	if last := strings.LastIndexAny(source, "/\\:"); last >= 0 {
		source = source[last+1:]
	}
	name := strings.TrimSuffix(source, ".git")
	if name == "" || name == "." || name == ".." {
		return ""
	}
	return "." + string(filepath.Separator) + name
}
