package main

import (
	"maps"
	"strings"
	"unicode"
)

type commitDraft struct {
	message             string
	amendHead           string
	amendRef            string
	amendUpstream       string
	original            string
	previous            string
	beforeSuggestion    string
	hasBeforeSuggestion bool
}

type amendConfirmation struct {
	key      changesWriteKey
	head     string
	ref      string
	upstream string
	message  string
	loading  bool
	err      string
}

func (w *workbench) currentCommitDraft() commitDraft {
	return w.commitDrafts.Get()[w.changesWriteKey()]
}

func (w *workbench) setCommitDraft(key changesWriteKey, draft commitDraft) {
	drafts := maps.Clone(w.commitDrafts.Get())
	if draft == (commitDraft{}) {
		delete(drafts, key)
	} else {
		drafts[key] = draft
	}
	w.commitDrafts.Set(drafts)
}

func (w *workbench) editCommitMessage(message string) {
	if w.currentChangesWrite().pending {
		return
	}
	draft := w.currentCommitDraft()
	draft.message = message
	w.setCommitDraft(w.changesWriteKey(), draft)
}

func (s workingChanges) canAmend() bool {
	return s.ready && !s.bare && s.status.HeadOID != "" && s.status.HeadOID != "(initial)" &&
		strings.HasPrefix(s.status.HeadRef, "refs/heads/") && s.status.Commit.MergeHeads == "" &&
		(s.status.Commit.Continuation == "" || s.status.Commit.CanAmendInteractiveRebase()) && len(s.conflicts) == 0
}

func (s workingChanges) canCommit(draft commitDraft) bool {
	if !s.ready || s.bare || len(s.conflicts) != 0 ||
		strings.TrimSpace(draft.message) == "" || strings.ContainsRune(draft.message, 0) {
		return false
	}
	if draft.amendHead != "" {
		return s.amendMatches(draft)
	}
	return s.status.Commit.Continuation == "" && (len(s.staged) != 0 || s.status.Commit.MergeHeads != "")
}

func (s workingChanges) amendMatches(draft commitDraft) bool {
	return s.canAmend() && draft.amendHead == s.status.HeadOID && draft.amendRef == s.status.HeadRef &&
		draft.amendUpstream == s.status.Upstream
}

func (w *workbench) openAmend() {
	shown := w.changes.Get()
	child := w.core
	if child == nil || !shown.canAmend() || w.currentChangesWrite().pending {
		return
	}
	status := shown.status
	prompt := &amendConfirmation{key: w.changesWriteKey(), head: status.HeadOID,
		ref: status.HeadRef, upstream: status.Upstream, loading: true}
	w.amendPrompt.Set(prompt)
	ctx := w.repoCtx
	go func() {
		commits, err := child.repo.Commits(ctx, []string{prompt.head})
		w.cx.Post(func() {
			if ctx.Err() != nil || w.amendPrompt.Get() != prompt {
				return
			}
			loaded := *prompt
			loaded.loading = false
			if err != nil {
				loaded.err = err.Error()
			} else {
				loaded.message = strings.TrimRightFunc(commits[0].Message, unicode.IsSpace)
			}
			w.amendPrompt.Set(&loaded)
		})
	}()
}

func (w *workbench) canConfirmAmend(prompt *amendConfirmation) bool {
	shown := w.changes.Get()
	return prompt != nil && !prompt.loading && prompt.err == "" && w.changesWriteKey() == prompt.key &&
		shown.canAmend() && shown.status.HeadOID == prompt.head && shown.status.HeadRef == prompt.ref &&
		shown.status.Upstream == prompt.upstream && !w.currentChangesWrite().pending
}

func (w *workbench) confirmAmend() {
	prompt := w.amendPrompt.Get()
	if !w.canConfirmAmend(prompt) {
		return
	}
	draft := w.currentCommitDraft()
	previous := draft.message
	if draft.amendHead != "" {
		previous = draft.previous
	}
	w.setCommitDraft(prompt.key, commitDraft{
		message: prompt.message, amendHead: prompt.head, amendRef: prompt.ref, amendUpstream: prompt.upstream,
		original: prompt.message, previous: previous,
	})
	w.amendPrompt.Set(nil)
}

func (w *workbench) writeCommit() {
	child, shown, key := w.core, w.changes.Get(), w.changesWriteKey()
	draft := w.currentCommitDraft()
	if child == nil || !w.changesMode.Get() || !shown.canCommit(draft) || w.currentChangesWrite().pending {
		return
	}
	w.beginChangesWrite(key)
	ctx := w.appCtx
	go func() {
		commit, err := child.repo.Commit(ctx, shown.status, draft.message, draft.amendHead)
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			result := changesWrite{commit: commit.SHA, warning: commit.Warning}
			if err != nil {
				result.err = changesWriteError(err)
			} else if w.commitDrafts.Get()[key] == draft {
				w.setCommitDraft(key, commitDraft{})
			}
			w.setChangesWrite(key, result)
			if w.changesWriteKey() == key {
				w.loadChanges()
				if err == nil {
					w.refreshRepository(false)
				}
			}
		})
	}()
}
