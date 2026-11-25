package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"gitwave.work/internal/gitcli"
	"gitwave.work/internal/gittool"
	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

func transferFilename(name string) string {
	const suffix = ".gitwave-workspace.json"
	base := strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '-'
		}
		return r
	}, name)
	base = strings.Trim(base, " .")
	if base == "" || !filepath.IsLocal(base+suffix) {
		base = "workspace"
	}
	return base + suffix
}

func importWorkspace(ctx context.Context, store *workspace.Store, path string) (workspace.Summary, int, error) {
	transfer, err := workspace.ReadTransfer(path)
	if err != nil {
		return workspace.Summary{}, 0, err
	}
	if len(transfer.Repos) != 0 {
		if _, err := gittool.Git(); err != nil {
			return workspace.Summary{}, 0, fmt.Errorf("find Git for workspace import: %w", err)
		}
	}
	repos := make([]string, 0, len(transfer.Repos))
	seen := make(map[string]bool, len(transfer.Repos))
	for _, path := range transfer.Repos {
		if ctx.Err() != nil {
			return workspace.Summary{}, 0, ctx.Err()
		}
		if path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		repo, err := gitcli.OpenRoot(ctx, abs)
		if err != nil {
			continue
		}
		key := repo.Path()
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			repos = append(repos, repo.Path())
			seen[key] = true
		}
	}
	ws, err := store.CreateWithRepos(ctx, transfer.Name, repos)
	if err != nil {
		return workspace.Summary{}, 0, err
	}
	return ws, len(transfer.Repos) - len(repos), nil
}

func (w *workbench) workspaceTransferDialog(chrome palette, dialog workspaceDialog) core.Node {
	export := dialog.kind == "export"
	title := w.tr("Import workspace", "导入工作区")
	label := w.tr("Import", "导入")
	placeholder := w.tr("source .gitwave-workspace.json path", "源 .gitwave-workspace.json 路径")
	description := w.tr("Reads a .gitwave-workspace.json, creates a new workspace and re-adds repos that exist on disk.",
		"读取 .gitwave-workspace.json，创建新工作区并重新添加磁盘上仍存在的仓库。")
	if export {
		title = w.tr("Export workspace", "导出工作区")
		label = w.tr("Export", "导出")
		placeholder = w.tr("destination .json path", "目标 .json 路径")
		description = w.tr("Writes a .gitwave-workspace.json with the workspace name and repo paths. API keys are never included.",
			"写入一个 .gitwave-workspace.json，包含工作区名称与仓库路径。绝不包含 API 密钥。")
	}
	disabled := func() bool { return w.workspaceBusy.Get() || strings.TrimSpace(w.workspaceInput.Get()) == "" }
	confirm := func() {
		if disabled() || w.workspaceModal.Get() != dialog {
			return
		}
		path := strings.TrimSpace(w.workspaceInput.Get())
		replace := w.workspaceOverwrite.Get()
		store := w.workspaceStore
		w.runWorkspaceAction(func(ctx context.Context) (workspaceResult, error) {
			if export {
				err := store.Export(ctx, dialog.ws.ID, path, replace)
				return workspaceResult{notice: fmt.Sprintf(w.tr("Workspace exported to %s", "工作区已导出到 %s"), path)}, err
			}
			ws, skipped, err := importWorkspace(ctx, store, path)
			if err != nil {
				return workspaceResult{}, err
			}
			message := fmt.Sprintf(w.tr("Workspace %q imported", "工作区「%s」已导入"), ws.Name)
			if skipped != 0 {
				message += fmt.Sprintf(w.tr(" · Skipped %d unavailable or duplicate paths", " · 跳过 %d 个不可用或重复的路径"), skipped)
			}
			return workspaceResult{notice: message, workspaceID: ws.ID, repoID: ws.LastRepoID}, nil
		}, !export)
	}
	cancel := func() {
		if !w.workspaceBusy.Get() {
			w.workspaceModal.Set(workspaceDialog{})
			w.workspaceOverwrite.Set(nil)
			w.workspaceError.Set("")
		}
	}
	return ui.Dialog{
		TestID: "workspace-transfer-dialog", Title: title, Description: description,
		Open: func() bool { return w.workspaceModal.Get() == dialog },
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				cancel()
			}
		},
		Children: []core.Node{
			core.Input{
				TestID: "workspace-transfer-path", Value: w.workspaceInput.Get, Placeholder: placeholder, AutoFocus: true,
				OnChange: func(path string) {
					w.workspaceInput.Set(path)
					w.workspaceOverwrite.Set(nil)
					w.workspaceError.Set("")
				},
				OnSubmit: func(string) {
					if w.workspaceOverwrite.Get() == nil {
						confirm()
					}
				},
				Style: core.Style{Height: core.Pt(32), Padding: core.All(6), BorderWidth: 1, BorderColor: chrome.border},
			},
			core.Text{ContentFunc: w.workspaceError.Get, Style: core.Style{Color: chrome.danger, FontSize: chrome.uiSize(12)}},
		},
		Footer: []core.Node{
			action(chrome, w.tr("Cancel", "取消"), cancel),
			core.Dynamic(func(*core.Scope) core.Node {
				text, color := label, chrome.text
				if w.workspaceOverwrite.Get() != nil {
					text, color = w.tr("Replace", "覆盖"), chrome.danger
				}
				button := toolbarAction(chrome, text, confirm, chrome.label(text, color, 12))
				button.Disabled = disabled
				return button
			}),
		},
	}
}
