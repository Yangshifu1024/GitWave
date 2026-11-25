package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitwave.work/internal/sshclient"

	"github.com/nsgo-dev/nsgo/core"
)

type sshForm string

const (
	sshAddForm   sshForm = "add"
	sshProbeForm sshForm = "probe"
)

type sshSettingsPanel struct {
	ctx          context.Context
	cancel, stop context.CancelFunc
	form         *core.Signal[sshForm]
	busy         *core.Signal[string]
	snapshot     *core.Signal[sshAgentSnapshot]
	path         *core.Signal[string]
	host, user   *core.Signal[string]
	failure      *core.Signal[string]
	probe        *core.Signal[*sshclient.ProbeResult]
}

type sshAgentSnapshot struct {
	listing sshclient.Listing
	err     error
}

type sshTaskResult struct {
	snapshot *sshAgentSnapshot
	probe    *sshclient.ProbeResult
}

func (w *workbench) openSSHSettings() {
	if w.sshSettings.Get() != nil {
		return
	}
	cx := w.cx
	ctx, cancel := context.WithCancel(w.appCtx)
	panel := &sshSettingsPanel{ctx: ctx, cancel: cancel,
		form: core.NewSignal(cx, sshForm("")), busy: core.NewSignal(cx, ""),
		snapshot: core.NewSignalFunc(cx, sshAgentSnapshot{}, nil),
		path:     core.NewSignal(cx, ""), host: core.NewSignal(cx, "github.com"), user: core.NewSignal(cx, "git"),
		failure: core.NewSignal(cx, ""), probe: core.NewSignal[*sshclient.ProbeResult](cx, nil)}
	w.sshSettings.Set(panel)
	w.refreshSSHAgent(panel)
}

func (w *workbench) closeSSHSettings() {
	if panel := w.sshSettings.Get(); panel != nil {
		panel.cancel()
		w.sshSettings.Set(nil)
	}
}

func loadSSHAgent(ctx context.Context) *sshAgentSnapshot {
	list, err := sshclient.List(ctx)
	return &sshAgentSnapshot{listing: list, err: err}
}

func (w *workbench) refreshSSHAgent(panel *sshSettingsPanel) {
	w.runSSHTask(panel, "load", func(ctx context.Context) (sshTaskResult, error) {
		return sshTaskResult{snapshot: loadSSHAgent(ctx)}, nil
	})
}

func (w *workbench) runSSHTask(panel *sshSettingsPanel, kind string, task func(context.Context) (sshTaskResult, error)) {
	if w.sshSettings.Get() != panel || panel.busy.Get() != "" {
		return
	}
	ctx, cancel := context.WithCancel(panel.ctx)
	panel.stop = cancel
	panel.busy.Set(kind)
	panel.failure.Set("")
	go func() {
		result, err := task(ctx)
		w.cx.Post(func() {
			cancelled := ctx.Err() != nil
			cancel()
			if panel.ctx.Err() != nil || w.sshSettings.Get() != panel {
				return
			}
			panel.stop = nil
			panel.busy.Set("")
			if cancelled {
				if kind == "add" {
					// A key may already have reached the agent before the child exited.
					w.refreshSSHAgent(panel)
				}
				return
			}
			if result.snapshot != nil {
				panel.snapshot.Set(*result.snapshot)
			}
			if err != nil {
				panel.failure.Set(w.sshSettingsError(err))
				return
			}
			if kind == "add" {
				w.closeSSHForm(panel)
			}
			if result.probe != nil {
				panel.probe.Set(result.probe)
			}
		})
	}()
}

func (w *workbench) closeSSHForm(panel *sshSettingsPanel) {
	if kind := panel.busy.Get(); (kind == "add" || kind == "probe") && panel.stop != nil {
		panel.stop()
	}
	panel.form.Set("")
	panel.path.Set("")
	panel.failure.Set("")
	panel.probe.Set(nil)
}

func (w *workbench) pickSSHKey(panel *sshSettingsPanel) {
	if panel.busy.Get() != "" || panel.form.Get() != sshAddForm {
		return
	}
	home, _ := os.UserHomeDir()
	var start string
	if home != "" {
		start = filepath.Join(home, ".ssh")
	}
	w.cx.PickFiles(core.PickFilesOptions{Title: w.tr("Choose a private SSH key", "选择 SSH 私钥"), Start: start}, func(files []core.File, err error) {
		if w.sshSettings.Get() != panel || panel.form.Get() != sshAddForm || panel.busy.Get() != "" {
			return
		}
		if err != nil {
			panel.failure.Set(w.sshSettingsError(err))
		} else if len(files) == 1 {
			panel.path.Set(files[0].Path)
		}
	})
}

func (w *workbench) addSSHKey(panel *sshSettingsPanel) {
	path := strings.TrimSpace(panel.path.Get())
	if path == "" || panel.form.Get() != sshAddForm {
		return
	}
	w.runSSHTask(panel, "add", func(ctx context.Context) (sshTaskResult, error) {
		if err := sshclient.Add(ctx, path); err != nil {
			return sshTaskResult{}, err
		}
		return sshTaskResult{snapshot: loadSSHAgent(ctx)}, nil
	})
}

func (w *workbench) removeSSHKey(panel *sshSettingsPanel, fingerprint string) {
	w.runSSHTask(panel, "remove", func(ctx context.Context) (sshTaskResult, error) {
		err := sshclient.Remove(ctx, fingerprint)
		return sshTaskResult{snapshot: loadSSHAgent(ctx)}, err
	})
}

func (w *workbench) testSSHConnection(panel *sshSettingsPanel) {
	if panel.form.Get() != sshProbeForm || panel.busy.Get() != "" {
		return
	}
	host, user := panel.host.Get(), panel.user.Get()
	panel.probe.Set(nil)
	w.runSSHTask(panel, "probe", func(ctx context.Context) (sshTaskResult, error) {
		result, err := sshclient.Probe(ctx, host, user)
		return sshTaskResult{probe: &result}, err
	})
}

func (w *workbench) startSSHAgent(panel *sshSettingsPanel) {
	w.runSSHTask(panel, "start", func(ctx context.Context) (sshTaskResult, error) {
		if err := sshclient.StartWindowsAgent(ctx); err != nil {
			return sshTaskResult{}, err
		}
		poll, cancel := context.WithTimeout(ctx, 16*time.Second)
		defer cancel()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			snapshot := loadSSHAgent(poll)
			if snapshot.listing.Running {
				return sshTaskResult{snapshot: snapshot}, nil
			}
			select {
			case <-poll.Done():
				return sshTaskResult{}, sshclient.ErrAgentUnavailable
			case <-ticker.C:
			}
		}
	})
}

func (w *workbench) sshSettingsError(err error) string {
	switch {
	case errors.Is(err, sshclient.ErrPassphrase):
		return w.tr("Load this passphrase-protected key in a terminal with ssh-add first.", "请先在终端用 ssh-add 加载这把带 passphrase 的密钥。")
	case errors.Is(err, sshclient.ErrPublicKey):
		return w.tr("Choose the private key without the .pub extension.", "请选择不带 .pub 后缀的私钥。")
	case errors.Is(err, sshclient.ErrKeyChanged):
		return w.tr("The selected key is no longer loaded in ssh-agent.", "选中的密钥已从 ssh-agent 移除。")
	case errors.Is(err, sshclient.ErrAgentUnavailable):
		return w.tr("ssh-agent is not running or cannot be reached.", "ssh-agent 未运行或无法连接。")
	case errors.Is(err, context.DeadlineExceeded):
		return w.tr("The OpenSSH command timed out.", "OpenSSH 命令超时。")
	default:
		return err.Error()
	}
}
