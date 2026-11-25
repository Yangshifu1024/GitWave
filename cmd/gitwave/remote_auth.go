package main

import (
	"fmt"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

type inlineAuthUI struct {
	username *core.Signal[string]
	password *core.Signal[string]
	remember *core.Signal[bool]
}

func newInlineAuthUI(cx *core.Scope) inlineAuthUI {
	return inlineAuthUI{core.NewSignal(cx, ""), core.NewSignal(cx, ""), core.NewSignal(cx, true)}
}

func (a inlineAuthUI) reset() {
	a.username.Set("")
	a.password.Set("")
	a.remember.Set(true)
}

func (a inlineAuthUI) incomplete() bool {
	return strings.TrimSpace(a.username.Get()) == ""
}

func (w *workbench) savedRemoteCredentialUnavailable() string {
	return w.tr("The saved credential for this account is unavailable. Enter an access token to continue.",
		"此账号已保存的凭证不可用。请输入访问令牌后继续。")
}

func (w *workbench) remoteAuthDescription(challenge *gitcli.RemoteAuthError) string {
	if challenge.LFSLocation() != "" {
		return fmt.Sprintf(w.tr("Git LFS at %q requires authentication.", "Git LFS 地址「%s」需要认证。"), challenge.Location())
	}
	return fmt.Sprintf(w.tr("The remote at %q requires authentication.", "远端「%s」需要认证。"), challenge.Location())
}

func (a inlineAuthUI) credentials() *gitcli.HTTPSCredential {
	if a.incomplete() {
		return nil
	}
	return &gitcli.HTTPSCredential{
		Username: strings.TrimSpace(a.username.Get()), Password: a.password.Get(), Remember: a.remember.Get(),
	}
}

func (w *workbench) inlineAuthInputs(prefix string, auth inlineAuthUI, style core.Style, submit func()) core.Node {
	user := w.tr("Username", "用户名")
	token := w.tr("Access token (PAT)", "访问令牌（PAT）")
	return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
		core.Input{TestID: prefix + "-auth-username", Label: user, Placeholder: user, AutoFocus: true,
			Value: auth.username.Get, OnChange: auth.username.Set, OnSubmit: func(string) { submit() }, Style: style},
		core.Input{TestID: prefix + "-auth-password", Label: token, Masked: true,
			Placeholder: w.tr("Enter a token or use the stored credential", "输入令牌，或留空使用已保存的凭证"),
			Value:       auth.password.Get, OnChange: auth.password.Set, OnSubmit: func(string) { submit() }, Style: style},
		ui.Checkbox{TestID: prefix + "-auth-remember",
			Text:    w.tr("Remember credentials", "记住凭证"),
			Checked: auth.remember.Get, OnChange: auth.remember.Set},
	}}
}
