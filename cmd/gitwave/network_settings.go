package main

import (
	"context"
	"strings"
	"time"

	"gitwave.work/internal/proxy"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type networkSettingsUI struct {
	proxySettings *core.Signal[proxy.Settings]
	proxyTarget   *core.Signal[proxy.Settings]
	proxyDraft    *core.Signal[string]
	proxyError    *core.Signal[string]
	proxySaved    *core.Signal[bool]
	proxyBusy     bool
	stopSaved     func() bool
}

func newNetworkSettingsUI(cx *core.Scope) networkSettingsUI {
	return networkSettingsUI{
		proxySettings: core.NewSignal(cx, proxy.Settings{Mode: proxy.System}),
		proxyTarget:   core.NewSignal(cx, proxy.Settings{Mode: proxy.System}),
		proxyDraft:    core.NewSignal(cx, ""), proxyError: core.NewSignal(cx, ""), proxySaved: core.NewSignal(cx, false),
	}
}

func (w *workbench) loadProxySettings() bool {
	if w.workspaceStore == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(w.appCtx, time.Second)
	defer cancel()
	settings, err := w.workspaceStore.ProxySettings(ctx)
	w.proxySettings.Set(settings)
	w.proxyTarget.Set(settings)
	w.proxyDraft.Set(settings.ManualURL)
	if err != nil {
		w.proxyError.Set(w.tr("Could not read the saved proxy settings", "无法读取已保存的代理设置"))
	}
	return err == nil
}

func (w *workbench) networkContext(ctx context.Context) context.Context {
	return proxy.WithSettings(ctx, w.proxySettings.Get())
}

func (w *workbench) commitProxyAddress() {
	next := w.proxyTarget.Get()
	if next.Mode == proxy.Manual {
		next.ManualURL = strings.TrimSpace(w.proxyDraft.Get())
		w.saveProxySettings(next)
	}
}

func (w *workbench) selectProxyMode(mode proxy.Mode) {
	next := w.proxyTarget.Get()
	if next.Mode == proxy.Manual {
		withDraft := next
		withDraft.ManualURL = w.proxyDraft.Get()
		if normalized, err := withDraft.Normalize(); err == nil {
			next.ManualURL = normalized.ManualURL
		}
	}
	next.Mode = mode
	w.saveProxySettings(next)
}

func (w *workbench) saveProxySettings(candidate proxy.Settings) {
	next, err := candidate.Normalize()
	if err != nil {
		w.proxySaved.Set(false)
		w.proxyError.Set(w.tr("Enter a valid HTTP or HTTPS proxy address", "请输入有效的 HTTP 或 HTTPS 代理地址"))
		return
	}
	if next == w.proxyTarget.Get() {
		if !w.proxyBusy && next.Mode == proxy.Manual {
			w.proxyDraft.Set(next.ManualURL)
			w.proxyError.Set("")
		}
		return
	}
	w.proxyTarget.Set(next)
	if !w.proxyBusy {
		w.persistProxySettings(next)
	}
}

func (w *workbench) persistProxySettings(next proxy.Settings) {
	store := w.workspaceStore
	draft := w.proxyDraft.Get()
	w.proxyError.Set("")
	apply := func(err error) {
		if err != nil {
			w.proxySaved.Set(false)
			w.proxyError.Set(w.tr("Could not save the proxy settings", "无法保存代理设置"))
			return
		}
		w.proxySettings.Set(next)
		if w.proxyDraft.Get() == draft {
			w.proxyDraft.Set(next.ManualURL)
		}
		if w.stopSaved != nil {
			w.stopSaved()
		}
		w.proxySaved.Set(true)
		w.stopSaved = w.cx.AfterFunc(2*time.Second, func() { w.proxySaved.Set(false) })
	}
	if store == nil {
		apply(nil)
		return
	}
	w.proxyBusy = true
	go func() {
		err := store.SaveProxySettings(w.appCtx, next)
		w.cx.Post(func() {
			if w.appCtx.Err() != nil || w.workspaceStore != store {
				return
			}
			w.proxyBusy = false
			apply(err)
			if wanted := w.proxyTarget.Get(); wanted != next {
				w.persistProxySettings(wanted)
			} else if err != nil {
				w.proxyTarget.Set(w.proxySettings.Get())
			}
		})
	}()
}

func (w *workbench) networkSettings(chrome settingsTheme) core.Node {
	label := w.tr("Proxy", "代理")
	option := func(mode proxy.Mode, name string, icon *core.SVG) core.Node {
		checked := func() bool { return w.proxyTarget.Get().Mode == mode }
		return settingsCard(chrome, "settings-proxy-"+string(mode), name, checked,
			func() { w.selectProxyMode(mode) }, settingsCardIcon(chrome, icon, checked))
	}
	modeHint := func() string {
		switch w.proxyTarget.Get().Mode {
		case proxy.Manual:
			return w.tr("AI, Git and update checks use the address below.", "AI、Git 和更新检查使用下方地址。")
		case proxy.Off:
			return w.tr("No proxy is applied. Proxy variables from the launch environment keep working.",
				"不使用代理。启动环境中已设置的代理变量仍然有效。")
		default:
			return w.tr("Uses the proxy configured in the OS network settings. Linux follows proxy environment variables.",
				"使用操作系统网络设置中配置的代理。Linux 遵循代理环境变量。")
		}
	}
	return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
		chrome.label(label, chrome.muted, 12),
		ui.RadioGroup{Label: label, Horizontal: true, Style: core.Style{ColumnGap: 8}, Children: []core.Node{
			option(proxy.System, w.tr("Follow system", "跟随系统"), lucide.Globe()),
			option(proxy.Manual, w.tr("Manual", "手动"), lucide.Settings2()),
			option(proxy.Off, w.tr("Off", "关闭"), lucide.Monitor()),
		}},
		core.Text{ContentFunc: modeHint, Style: core.Style{FontSize: chrome.uiSize(11)}, StyleFunc: settingsTextColor(chrome.muted)},
		core.Show{When: func() bool { return w.proxyTarget.Get().Mode == proxy.Manual }, Then: func(*core.Scope) core.Node {
			caption := w.tr("Proxy address", "代理地址")
			return core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
				chrome.label(caption, chrome.text, 12),
				ui.Input{TestID: "settings-proxy-address", Label: caption, Placeholder: "http://127.0.0.1:7890",
					Value: w.proxyDraft.Get, OnChange: func(value string) {
						w.proxyDraft.Set(value)
						w.proxySaved.Set(false)
					}, OnBlur: w.commitProxyAddress, OnSubmit: func(string) { w.commitProxyAddress() }},
				chrome.hint(w.tr("The proxy GitWave applies always skips local addresses (localhost / 127.0.0.1).",
					"GitWave 应用的代理始终跳过本地地址（localhost / 127.0.0.1）。")),
			}}
		}},
		core.Show{When: func() bool { return w.proxyError.Get() != "" }, Then: func(*core.Scope) core.Node {
			return core.Text{Role: core.RoleStatus, TestID: "settings-proxy-error", ContentFunc: w.proxyError.Get,
				Style: core.Style{FontSize: chrome.uiSize(11)}, StyleFunc: settingsTextColor(chrome.danger)}
		}},
		core.Show{When: w.proxySaved.Get, Then: func(*core.Scope) core.Node {
			status := chrome.hint(w.tr("Saved — effective immediately", "已保存——立即生效"))
			status.TestID, status.Role = "settings-proxy-saved", core.RoleStatus
			return status
		}},
	}}
}
