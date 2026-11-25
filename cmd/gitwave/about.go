package main

import (
	_ "embed"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
	"go.yaml.in/yaml/v3"
)

const projectURL = "https://github.com/Yangshifu1024/GitWave"

//go:embed assets/app-icon.png
var appIcon []byte

//go:embed nsgo.yaml
var appConfiguration []byte

type aboutUI struct {
	aboutOpen   *core.Signal[bool]
	aboutError  *core.Signal[string]
	aboutAction uint64
}

func newAboutUI(cx *core.Scope) aboutUI {
	return aboutUI{aboutOpen: core.NewSignal(cx, false), aboutError: core.NewSignal(cx, "")}
}

func appVersion() string {
	if version := core.App().Version; version != "" {
		return version
	}
	var config struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal(appConfiguration, &config); err != nil || config.Version == "" {
		return "?.?.?"
	}
	return config.Version
}

func appVersionLabel() string {
	label := "v" + appVersion()
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				label += " (" + setting.Value[:min(7, len(setting.Value))] + ")"
				break
			}
		}
	}
	return label
}

func (w *workbench) openAbout() {
	if !w.refreshBlocked() {
		w.aboutError.Set("")
		w.aboutOpen.Set(true)
	}
}

func (w *workbench) beginAboutAction() func(string) {
	w.aboutAction++
	id := w.aboutAction
	w.aboutError.Set("")
	return func(message string) {
		if w.appCtx.Err() == nil && w.aboutAction == id && w.aboutOpen.Get() {
			w.aboutError.Set(message)
		}
	}
}

func (w *workbench) aboutDataDirectory() {
	result := w.beginAboutAction()
	var directory string
	if w.workspaceStore != nil {
		directory = w.workspaceStore.DataDirectory()
	} else {
		path, err := workspacePathForApp(core.App())
		if err != nil {
			result(err.Error())
			return
		}
		directory = filepath.Dir(path)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		result(err.Error())
		return
	}
	w.cx.OpenFile(core.SharedFile{Path: directory}, func(opened bool, err error) {
		if err != nil {
			result(err.Error())
		} else if !opened {
			result(w.tr("Could not open the app data directory.", "无法打开应用数据目录。"))
		}
	})
}

func (w *workbench) aboutGitHub() {
	result := w.beginAboutAction()
	w.cx.OpenURL(projectURL, func(opened bool) {
		if !opened {
			result(w.tr("Could not open the GitHub project.", "无法打开 GitHub 项目。"))
		}
	})
}

func (w *workbench) aboutDialog(chrome palette) core.Node {
	return core.Show{When: w.aboutOpen.Get, Then: func(*core.Scope) core.Node {
		closeDialog := func() {
			w.aboutAction++
			w.aboutOpen.Set(false)
		}
		dataLabel := w.tr("App Data", "应用数据")
		data := toolbarAction(chrome, dataLabel, w.aboutDataDirectory,
			core.Icon{SVG: lucide.FolderOpen(), Style: core.Style{FontSize: 14, Color: chrome.text}},
			chrome.label(dataLabel, chrome.text, 12))
		data.TestID = "about-app-data"
		github := toolbarAction(chrome, "GitHub", w.aboutGitHub,
			core.Icon{SVG: lucide.ExternalLink(), Style: core.Style{FontSize: 14, Color: chrome.text}},
			chrome.label("GitHub", chrome.text, 12))
		github.TestID = "about-github"
		return ui.Dialog{TestID: "about-dialog", Title: w.tr("About GitWave", "关于 GitWave"), Open: w.aboutOpen.Get,
			Style: core.Style{Width: core.Pt(388)},
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					closeDialog()
				}
			}, Children: []core.Node{
				core.View{Style: core.Style{AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
					core.Image{TestID: "about-app-icon", Label: w.tr("GitWave app icon", "GitWave 应用图标"), Source: core.ImageSource{Data: appIcon},
						Style: core.Style{Width: core.Pt(80), Height: core.Pt(80), CornerRadius: 12}},
					core.Text{Content: "GitWave", Style: core.Style{FontSize: chrome.uiSize(18), FontWeight: 600, Color: chrome.text}},
					core.Text{Content: appVersionLabel(), TestID: "about-version", Style: core.Style{FontSize: chrome.uiSize(13), Color: chrome.muted}},
					core.Text{Content: w.tr("Local-first Git client with AI collaboration.", "本地优先、支持 AI 协作的 Git 客户端。"),
						Style: core.Style{TextAlign: core.TextAlignCenter, FontSize: chrome.uiSize(13), Color: chrome.text}},
					core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{data, github}},
					core.Show{When: func() bool { return w.aboutError.Get() != "" }, Then: func(*core.Scope) core.Node {
						return core.Text{Role: core.RoleStatus, TestID: "about-error", ContentFunc: w.aboutError.Get,
							Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.danger}}
					}},
				}},
			}, Footer: []core.Node{action(chrome, w.tr("Close", "关闭"), closeDialog)}}
	}}
}
