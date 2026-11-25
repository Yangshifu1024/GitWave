package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"gitwave.work/internal/ai"
	"gitwave.work/internal/gitcli"
	"gitwave.work/internal/workspace"
)

const palettePrompt = `You convert a user request into exactly ONE GitWave UI action.
Respond with one JSON object with action, params and a short explanation. Allowed actions:
- explain_commit {"sha": string}
- locate_commit {"sha": string}
- create_branch {"name": string, "from": optional commit or branch}
- checkout_branch {"name": local branch name}
- create_tag {"name": string, "sha": optional commit}
- stash_changes {"message": optional string}
- fetch_remotes {}
- none {}
Use the provided recent commits and references to resolve the request. Mutating actions require user confirmation.
Use the exact action and parameter keys. Apply the reply language only to the explanation.
Never invent actions outside this list.`

type paletteAIState struct {
	loading, saving bool
	failure         string
	intent          *paletteIntent
}

func (s paletteAIState) busy() bool { return s.loading || s.saving }

type paletteIntent struct {
	action, explanation string
	name, revision      string
	message             string
	oid                 string
	confirm             bool
	source              gitcli.CheckoutTarget
}

func parsePaletteIntent(raw string) (paletteIntent, error) {
	var wire struct {
		Action      string                     `json:"action"`
		Params      map[string]json.RawMessage `json:"params"`
		Explanation string                     `json:"explanation"`
	}
	start, end := strings.IndexByte(raw, '{'), strings.LastIndexByte(raw, '}')
	if start < 0 || end <= start || json.Unmarshal([]byte(raw[start:end+1]), &wire) != nil {
		return paletteIntent{}, errors.New("AI did not return a valid JSON action")
	}
	value := func(key string, required bool) (string, error) {
		raw := wire.Params[key]
		var text string
		if len(raw) != 0 && json.Unmarshal(raw, &text) != nil {
			return "", fmt.Errorf("AI action parameter %q must be a string", key)
		}
		text = strings.TrimSpace(text)
		if required && text == "" {
			return "", fmt.Errorf("AI action is missing parameter %q", key)
		}
		return text, nil
	}
	result := paletteIntent{action: strings.ToLower(strings.TrimSpace(wire.Action)), explanation: strings.TrimSpace(wire.Explanation)}
	var err error
	switch result.action {
	case "explain_commit", "locate_commit":
		result.revision, err = value("sha", true)
	case "create_branch", "checkout_branch", "create_tag":
		result.confirm = true
		if result.name, err = value("name", true); err != nil {
			return paletteIntent{}, err
		}
		if result.action == "create_branch" {
			result.revision, err = value("from", false)
		} else if result.action == "create_tag" {
			result.revision, err = value("sha", false)
		}
	case "stash_changes":
		result.confirm = true
		result.message, err = value("message", false)
	case "fetch_remotes":
		result.confirm = true
	case "none":
	default:
		return paletteIntent{}, errors.New("AI suggested an unsupported action")
	}
	return result, err
}

func (f *commandPaletteForm) cancelAI() {
	if f.aiStop != nil {
		f.aiStop()
		f.aiStop = nil
	}
	f.aiID++
	f.ai.Set(paletteAIState{})
}

func (w *workbench) askPaletteAI(form *commandPaletteForm) {
	query := strings.TrimSpace(form.query.Get())
	if query == "" || !w.currentCommandPalette(form) || form.ai.Get().busy() {
		return
	}
	access := w.workspaceAI()
	if access.store == nil || access.id == "" {
		form.ai.Set(paletteAIState{failure: w.tr("No active workspace", "当前没有活动工作区")})
		return
	}
	if form.child == nil {
		form.ai.Set(paletteAIState{failure: w.tr("Open a repository in this Workspace first.", "请先在当前工作区打开仓库。")})
		return
	}
	form.cancelAI()
	form.ai.Set(paletteAIState{loading: true})
	id := form.aiID
	repos, repo := w.workspaceState.Get().Repos, form.child.repo
	ctx, cancel := context.WithCancel(w.repoCtx)
	form.aiStop = cancel
	go func() {
		defer cancel()
		intent, err := resolvePaletteIntent(ctx, access, repo, repos, query)
		w.cx.Post(func() {
			if !w.currentCommandPalette(form) || id != form.aiID {
				return
			}
			form.aiStop = nil
			if err != nil {
				failure := err.Error()
				if errors.Is(err, ai.ErrNotConfigured) {
					failure = w.tr("Configure a provider in Workspace AI settings.", "请先在工作区 AI 设置中配置服务商。")
				}
				form.ai.Set(paletteAIState{failure: failure})
				return
			}
			form.ai.Set(paletteAIState{intent: &intent})
		})
	}()
}

func resolvePaletteIntent(ctx context.Context, access workspaceAI, repo *gitcli.Repository, repos []workspace.Repo, query string) (paletteIntent, error) {
	config, err := access.configured(ctx)
	if err != nil {
		return paletteIntent{}, err
	}
	refs, err := repo.References(ctx)
	if err != nil {
		return paletteIntent{}, err
	}
	prompt, err := paletteContext(ctx, repo, refs, repos)
	if err != nil {
		return paletteIntent{}, err
	}
	outcome, err := access.generate(ctx, config, ai.Prompt{
		System: withAIRepoRules(palettePrompt, repo.AIRules(ctx)),
		User:   "Repository context:\n" + prompt + "\n\nUser request: " + query,
	})
	if err != nil {
		return paletteIntent{}, err
	}
	intent, err := parsePaletteIntent(outcome.Text)
	if err != nil {
		return paletteIntent{}, err
	}
	switch intent.action {
	case "stash_changes":
		intent.source = gitcli.CheckoutTarget{Ref: refs.HeadRef, OID: refs.Head}
	case "checkout_branch":
		for _, ref := range refs.Refs {
			if ref.Name == "refs/heads/"+intent.name && ref.Type == "commit" && ref.Symbolic == "" {
				intent.oid = ref.OID
				intent.source = gitcli.CheckoutTarget{Ref: refs.HeadRef, OID: refs.Head}
				return intent, nil
			}
		}
		return paletteIntent{}, fmt.Errorf("local branch %q was not found", intent.name)
	case "explain_commit", "locate_commit", "create_branch", "create_tag":
		if intent.revision != "" && intent.revision != "HEAD" {
			intent.oid, err = repo.ResolveCommit(ctx, intent.revision)
		} else if refs.Head == "" {
			err = errors.New("no base commit")
		} else {
			intent.oid = refs.Head
		}
	}
	return intent, err
}

func paletteContext(ctx context.Context, repo *gitcli.Repository, refs gitcli.References, repos []workspace.Repo) (string, error) {
	type namedRepo struct {
		Name string `json:"name"`
	}
	snapshot := struct {
		Current string                 `json:"current_branch"`
		Local   []string               `json:"local_branches"`
		Remote  []string               `json:"remote_branches"`
		Tags    []string               `json:"tags"`
		Repos   []namedRepo            `json:"repos"`
		Commits []gitcli.CommitSubject `json:"recent_commits"`
	}{Current: "HEAD (detached)", Local: []string{}, Remote: []string{}, Tags: []string{}, Repos: []namedRepo{}, Commits: []gitcli.CommitSubject{}}
	if refs.HeadRef != "" {
		snapshot.Current = ai.Scrub(strings.TrimPrefix(refs.HeadRef, "refs/heads/"))
	}
	for _, ref := range refs.Refs {
		name, kind := visibleBranch(ref)
		if kind == "local" && len(snapshot.Local) < 20 {
			snapshot.Local = append(snapshot.Local, ai.Scrub(name))
		} else if kind == "remote" && len(snapshot.Remote) < 15 && ref.Symbolic == "" {
			snapshot.Remote = append(snapshot.Remote, ai.Scrub(name))
		} else if name, ok := strings.CutPrefix(ref.Name, "refs/tags/"); ok && len(snapshot.Tags) < 20 {
			snapshot.Tags = append(snapshot.Tags, ai.Scrub(name))
		}
	}
	for _, entry := range repos[:min(len(repos), 20)] {
		name := entry.Nickname
		if name == "" {
			name = filepath.Base(entry.Path)
		}
		snapshot.Repos = append(snapshot.Repos, namedRepo{Name: ai.Scrub(name)})
	}
	if refs.Head != "" {
		commits, err := repo.RecentCommitSubjects(ctx, refs.Head)
		if err != nil {
			return "", err
		}
		for i := range commits {
			commits[i].Subject = ai.Scrub(commits[i].Subject)
		}
		snapshot.Commits = commits
	}
	prompt, err := json.MarshalIndent(snapshot, "", "  ")
	return string(prompt), err
}

func (w *workbench) runPaletteIntent(form *commandPaletteForm, intent *paletteIntent) {
	if !w.currentCommandPalette(form) || form.ai.Get().intent != intent || form.ai.Get().busy() {
		return
	}
	if intent.confirm && (!w.canSync() || w.fetch.prompt.Get() != nil) {
		form.ai.Set(paletteAIState{intent: intent, failure: w.tr("Wait for the current Git operation to finish.", "请等待当前 Git 操作完成。")})
		return
	}
	if intent.action == "stash_changes" {
		w.savePaletteStash(form, intent)
		return
	}
	w.closeCommandPalette()
	switch intent.action {
	case "explain_commit":
		w.openCommitExplanation(intent.oid, "")
	case "locate_commit":
		w.locatePaletteCommit(intent.oid)
	case "create_branch", "create_tag":
		kind := createBranchForm
		if intent.action == "create_tag" {
			kind = tagManagerForm
		}
		w.openRefForm(refDialogState{kind: kind, base: intent.oid, baseLabel: shortSHA(intent.oid)})
		if ref := w.refForm.Get(); ref != nil {
			w.refName.Set(intent.name)
			w.applyRefForm(ref, intent.name, "", "")
		}
	case "checkout_branch":
		w.requestCheckoutFrom(gitcli.CheckoutTarget{Ref: "refs/heads/" + intent.name, OID: intent.oid}, intent.name, &intent.source)
	case "fetch_remotes":
		w.startFetch("")
	case "none":
		message := intent.explanation
		if message == "" {
			message = w.tr("No matching action for that request", "没有匹配的操作")
		}
		w.refNotice.Set(referenceNotice{text: message})
	}
}

func (w *workbench) savePaletteStash(form *commandPaletteForm, intent *paletteIntent) {
	if !w.canSaveStash() {
		form.ai.Set(paletteAIState{intent: intent, failure: w.tr("This repository is not ready to save a stash.", "当前仓库暂时无法保存贮藏。")})
		return
	}
	form.ai.Set(paletteAIState{intent: intent, saving: true})
	stash := &stashSaveForm{child: form.child, key: form.key, source: &intent.source}
	w.saveStash(stash, intent.message, true, func(err error) {
		if !w.currentCommandPalette(form) || form.ai.Get().intent != intent {
			return
		}
		switch err {
		case nil:
			w.closeCommandPalette()
		case gitcli.ErrStashTargetChanged:
			form.ai.Set(paletteAIState{failure: w.tr("The repository state changed after the proposal. Ask again to save a stash.",
				"仓库状态在建议生成后已变更。请重新询问后再保存贮藏。")})
		default:
			form.ai.Set(paletteAIState{failure: changesWriteError(err)})
		}
	})
}
