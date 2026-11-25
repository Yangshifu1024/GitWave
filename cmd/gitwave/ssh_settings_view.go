package main

import (
	"runtime"
	"strings"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func (w *workbench) sshSettingsView(chrome settingsTheme, panel *sshSettingsPanel) core.Node {
	toggle := func(id, label string, icon *core.SVG, mode sshForm) core.Node {
		button := chrome.iconAction(label, icon, func() {
			w.closeSSHForm(panel)
			panel.form.Set(mode)
		})
		button.TestID = id
		button.Disabled = func() bool { return panel.busy.Get() != "" }
		return button
	}
	return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifyEnd, Gap: 4}, Children: []core.Node{
			toggle("ssh-test-open", w.tr("Test SSH connection", "测试 SSH 连接"), lucide.Wifi(), sshProbeForm),
			toggle("ssh-add-open", w.tr("Add SSH key", "添加 SSH 密钥"), lucide.Plus(), sshAddForm),
		}},
		core.Show{When: func() bool { return panel.form.Get() == sshAddForm }, Then: func(cx *core.Scope) core.Node {
			cx.OnKeyWith("escape", core.KeyOptions{When: core.KeyAlways}, func() { w.closeSSHForm(panel) })
			return w.sshAddFields(chrome, panel)
		}},
		core.Show{When: func() bool { return panel.form.Get() == sshProbeForm }, Then: func(cx *core.Scope) core.Node {
			cx.OnKeyWith("escape", core.KeyOptions{When: core.KeyAlways}, func() { w.closeSSHForm(panel) })
			return w.sshProbeFields(chrome, panel)
		}},
		core.Show{When: func() bool { return panel.failure.Get() != "" && panel.form.Get() == "" }, Then: func(*core.Scope) core.Node {
			return sshFailure(chrome, panel)
		}},
		core.Dynamic(func(*core.Scope) core.Node { return w.sshKeyList(chrome, panel) }),
	}}
}

func sshFailure(chrome settingsTheme, panel *sshSettingsPanel) core.Node {
	return core.Text{TestID: "ssh-action-error", Role: core.RoleStatus, ContentFunc: panel.failure.Get,
		Style: core.Style{FontSize: chrome.uiSize(12)}, StyleFunc: settingsTextColor(chrome.danger)}
}

func (w *workbench) sshAddFields(chrome settingsTheme, panel *sshSettingsPanel) core.Node {
	label := w.tr("Key path", "密钥路径")
	busy := func() bool { return panel.busy.Get() != "" }
	pick := chrome.iconAction(w.tr("Browse for a key", "浏览密钥文件"), lucide.FolderOpen(), func() { w.pickSSHKey(panel) })
	pick.TestID, pick.Disabled = "ssh-pick-key", busy
	add := chrome.action(w.tr("Add", "添加"), func() { w.addSSHKey(panel) })
	add.TestID = "ssh-add-apply"
	add.Disabled = func() bool { return busy() || strings.TrimSpace(panel.path.Get()) == "" }
	return core.View{TestID: "ssh-add-form", Style: core.Style{Gap: 6, Padding: core.All(10), CornerRadius: 8},
		StyleFunc: func() core.Style { return core.Style{Background: chrome.colors().elevated} }, Children: []core.Node{
			chrome.label(label, chrome.text, 12),
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: []core.Node{
				ui.Input{TestID: "ssh-key-path", Label: label, Placeholder: "~/.ssh/id_ed25519", AutoFocus: true,
					Value: panel.path.Get, OnChange: panel.path.Set, OnSubmit: func(string) { w.addSSHKey(panel) }, ReadonlyFunc: busy,
					Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}}, pick,
			}},
			chrome.hint(w.tr("Adds a private key to ssh-agent. Load passphrase-protected keys in a terminal with ssh-add first.",
				"将私钥添加到 ssh-agent。带 passphrase 的密钥请先在终端用 ssh-add 加载。")),
			sshFailure(chrome, panel),
			core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifyEnd, Gap: 6}, Children: []core.Node{
				chrome.action(w.tr("Cancel", "取消"), func() { w.closeSSHForm(panel) }), add,
			}},
		}}
}

func (w *workbench) sshProbeFields(chrome settingsTheme, panel *sshSettingsPanel) core.Node {
	busy := func() bool { return panel.busy.Get() != "" }
	input := func(id, label, placeholder string, value *core.Signal[string]) core.Node {
		return core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
			chrome.label(label, chrome.text, 12),
			ui.Input{TestID: id, Label: label, Placeholder: placeholder, Value: value.Get, ReadonlyFunc: busy,
				OnChange: func(text string) { value.Set(text); panel.probe.Set(nil); panel.failure.Set("") },
				OnSubmit: func(string) { w.testSSHConnection(panel) }},
		}}
	}
	runLabel := func() string {
		if panel.busy.Get() == "probe" {
			return w.tr("Testing…", "测试中…")
		}
		return w.tr("Run test", "运行测试")
	}
	run := chrome.button(w.tr("Run test", "运行测试"), func() { w.testSSHConnection(panel) },
		core.Text{ContentFunc: runLabel, Style: core.Style{FontSize: chrome.uiSize(12)}, StyleFunc: settingsTextColor(chrome.text)})
	run.TestID, run.Label, run.LabelFunc = "ssh-test-run", "", runLabel
	run.Disabled = func() bool {
		return busy() || strings.TrimSpace(panel.host.Get()) == "" || strings.TrimSpace(panel.user.Get()) == ""
	}
	return core.View{TestID: "ssh-test-form", Style: core.Style{Gap: 8, Padding: core.All(10),
		CornerRadius: 6, BorderWidth: 1}, StyleFunc: func() core.Style { return core.Style{BorderColor: chrome.colors().border} }, Children: []core.Node{
		input("ssh-test-host", w.tr("Host", "主机"), "github.com", panel.host),
		input("ssh-test-user", w.tr("User", "用户"), "git", panel.user),
		sshFailure(chrome, panel),
		core.Dynamic(func(*core.Scope) core.Node {
			result := panel.probe.Get()
			if result == nil {
				return core.View{}
			}
			label, color := w.tr("Failed", "失败"), chrome.danger
			if result.Success {
				label, color = w.tr("Success", "成功"), chrome.text
			}
			return core.View{TestID: "ssh-test-result", Style: core.Style{Gap: 4, Padding: core.All(8), CornerRadius: 5, BorderWidth: 1},
				StyleFunc: func() core.Style {
					p := chrome.colors()
					background := p.remove
					if result.Success {
						background = p.add
					}
					return core.Style{BorderColor: color(), Background: background}
				}, Children: []core.Node{
					core.Text{Role: core.RoleStatus, Content: label, Style: core.Style{FontSize: chrome.uiSize(13)}, StyleFunc: settingsTextColor(color)},
					core.Text{Content: result.Message, Style: core.Style{FontSize: chrome.codeSize(11), FontFamily: chrome.fonts.code}, StyleFunc: settingsTextColor(chrome.muted)},
				}}
		}),
		core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceBetween}, Children: []core.Node{
			run, chrome.action(w.tr("Close", "关闭"), func() { w.closeSSHForm(panel) }),
		}},
	}}
}

func (w *workbench) sshKeyList(chrome settingsTheme, panel *sshSettingsPanel) core.Node {
	snapshot := panel.snapshot.Get()
	if panel.busy.Get() == "load" {
		return chrome.hint(w.tr("Loading keys…", "正在加载密钥…"))
	}
	if snapshot.err != nil {
		return core.Text{TestID: "ssh-load-error", Role: core.RoleStatus, Content: w.sshSettingsError(snapshot.err),
			Style: core.Style{FontSize: chrome.uiSize(12)}, StyleFunc: settingsTextColor(chrome.danger)}
	}
	if !snapshot.listing.Running {
		info := core.View{TestID: "ssh-agent-down", Style: core.Style{Gap: 8, Padding: core.All(12), AlignItems: core.AlignCenter},
			Children: []core.Node{chrome.label(w.tr("ssh-agent is not running", "ssh-agent 未运行"), chrome.text, 13),
				core.Text{Content: w.tr("Start ssh-agent to manage loaded keys.", "启动 ssh-agent 后即可管理已加载的密钥。"),
					Style: core.Style{FontSize: chrome.uiSize(12), TextAlign: core.TextAlignCenter}, StyleFunc: settingsTextColor(chrome.muted)}}}
		if runtime.GOOS == "windows" {
			caption := func() string {
				if panel.busy.Get() == "start" {
					return w.tr("Waiting for permission…", "等待授权…")
				}
				return w.tr("Start ssh-agent service", "启动 ssh-agent 服务")
			}
			button := chrome.button(caption(), func() { w.startSSHAgent(panel) },
				core.Icon{SVG: lucide.Power(), Style: core.Style{FontSize: 14}, StyleFunc: settingsTextColor(chrome.text)},
				core.Text{ContentFunc: caption, Style: core.Style{FontSize: chrome.uiSize(12)}, StyleFunc: settingsTextColor(chrome.text)})
			button.TestID, button.Label, button.LabelFunc = "ssh-agent-start", "", caption
			button.Disabled = func() bool { return panel.busy.Get() != "" }
			info.Children = append(info.Children, button)
		}
		return info
	}
	if len(snapshot.listing.Keys) == 0 {
		return core.View{TestID: "ssh-key-list-empty", Style: core.Style{Gap: 4, Padding: core.All(12), AlignItems: core.AlignCenter},
			Children: []core.Node{chrome.label(w.tr("No keys in ssh-agent", "ssh-agent 中没有密钥"), chrome.text, 13),
				chrome.hint(w.tr("Add a key to enable SSH clone and push.", "添加密钥以启用 SSH 克隆和推送。"))}}
	}
	rows := make([]core.Node, 0, len(snapshot.listing.Keys))
	for _, key := range snapshot.listing.Keys {
		identity := key.Comment
		if identity == "" {
			identity = key.Fingerprint
		}
		remove := chrome.iconAction(w.tr("Remove ", "移除 ")+identity, lucide.Trash(), func() { w.removeSSHKey(panel, key.Fingerprint) })
		remove.TestID = "ssh-key-remove"
		remove.Disabled = func() bool { return panel.busy.Get() != "" }
		rows = append(rows, core.View{TestID: "ssh-key-row", Style: core.Style{Direction: core.Row,
			AlignItems: core.AlignCenter, Gap: 8, MinHeight: core.Pt(38), Padding: core.Edges{Left: 8, Right: 4}}, Children: []core.Node{
			core.Icon{SVG: lucide.Key(), Style: core.Style{FontSize: 14}, StyleFunc: settingsTextColor(chrome.muted)},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 3}, Children: []core.Node{
				chrome.label(identity, chrome.text, 12),
				core.Text{Content: key.Fingerprint, MaxLines: 1, Style: core.Style{FontFamily: chrome.fonts.code,
					FontSize: chrome.codeSize(11)}, StyleFunc: settingsTextColor(chrome.muted)},
			}}, remove,
		}})
	}
	return core.View{TestID: "ssh-key-list", Children: rows}
}
