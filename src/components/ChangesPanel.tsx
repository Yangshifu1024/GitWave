import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, ChevronRight, EyeOff, FolderOpen, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/ContextMenu";
import { FileListItem } from "@/components/ui/FileListItem";
import { EmptyState } from "@/components/ui/EmptyState";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { CommitMessageBox } from "@/components/ui/CommitMessageBox";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { AiProviderSettings } from "@/components/AiProviderSettings";
import { GitignoreEditor } from "@/components/GitignoreEditor";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { Radio, RadioGroup } from "@heroui/react";
import { formatAppError, generateCommitMessage, getWorkspace, type FileChange } from "@/lib/api";
import { commitDraftKey, getCommitDraft, useCommitDraftStore } from "@/stores/commitDraftStore";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";
import { deriveIgnorePatterns } from "@/lib/ignorePattern";
import { modifierFromPointerEvent, nextFileSelection } from "@/lib/fileSelection";

export interface ChangesPanelProps {
  selectedPath: string | null;
  selectedStaged: boolean | null;
  onSelectFile: (path: string, staged: boolean) => void;
  /** `bar` = horizontal 3-column grid; `modal` = single column inside WorkingCopyModal */
  layout?: "stacked" | "bar" | "modal";
  /** Fired after a successful commit (the hosting modal closes itself). */
  onCommitted?: () => void;
}

function FileSection({
  title,
  actionLabel,
  actionVariant,
  allActionLabel,
  allActionVariant = "secondary",
  dangerActionLabel,
  allDangerActionLabel,
  files,
  emptyLabel,
  selectedPath,
  selectedStaged,
  onSelectFile,
  onStageToggle,
  onBulkAction,
  onAllAction,
  onDangerAction,
  onAllDangerAction,
  onDiscardFile,
  onIgnoreFile,
  layout,
}: {
  title: string;
  actionLabel: string;
  actionVariant: "primary" | "secondary";
  allActionLabel?: string;
  allActionVariant?: "primary" | "secondary";
  /** Destructive bulk action on the selected files (with confirmation upstream). */
  dangerActionLabel?: string;
  onDangerAction?: (paths: string[]) => void;
  /** Destructive action on every file in the section (with confirmation upstream). */
  allDangerActionLabel?: string;
  onAllDangerAction?: () => void;
  files: FileChange[];
  emptyLabel: string;
  selectedPath: string | null;
  selectedStaged: boolean | null;
  onSelectFile: (path: string, staged: boolean) => void;
  onStageToggle: (file: FileChange) => void;
  onBulkAction: (paths: string[]) => void;
  onAllAction?: () => void;
  /** Present to enable the per-file context menu (unstaged section only). */
  onDiscardFile?: (file: FileChange) => void;
  onIgnoreFile?: (file: FileChange) => void;
  layout: "stacked" | "bar" | "modal";
}): React.JSX.Element {
  const bar = layout === "bar";
  const fixed = bar || layout === "modal";
  const { t } = useTranslation();
  const [open, setOpen] = useState(true);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [focusedPath, setFocusedPath] = useState<string | null>(null);
  const [anchor, setAnchor] = useState<string | null>(null);
  const sectionOpen = fixed || open;

  const orderedPaths = files.map((file) => file.path);
  const selectedInSection = orderedPaths.filter((path) => selected.has(path));

  const handleFileClick = (
    path: string,
    event: React.MouseEvent<HTMLDivElement> | React.KeyboardEvent<HTMLDivElement>,
  ) => {
    const next = nextFileSelection(
      orderedPaths,
      selected,
      path,
      "key" in event && event.key === " " && !event.shiftKey
        ? "toggle"
        : modifierFromPointerEvent(event),
      anchor,
    );
    setFocusedPath(path);
    setSelected(next.selected);
    setAnchor(next.anchor);
    const file = files.find((entry) => entry.path === path);
    onSelectFile(path, file?.staged ?? false);
  };

  const handleBulk = (event: React.MouseEvent) => {
    event.stopPropagation();
    if (selectedInSection.length === 0) return;
    onBulkAction(selectedInSection);
    setSelected(new Set());
  };

  const handleDangerBulk = (event: React.MouseEvent) => {
    event.stopPropagation();
    if (selectedInSection.length === 0 || !onDangerAction) return;
    onDangerAction(selectedInSection);
    setSelected(new Set());
  };

  return (
    <div
      className={cn(
        "flex flex-col min-h-0",
        bar
          ? "flex-1 min-w-0 border-r border-border-subtle last:border-r-0"
          : layout === "modal"
            ? "flex-1 min-h-0 border-b border-border-subtle last:border-b-0"
            : "border-b border-border-subtle",
        !fixed && open ? "flex-1" : !fixed ? "shrink-0" : undefined,
      )}
      onKeyDown={(event) => {
        if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== "a") return;
        if (event.shiftKey || files.length === 0) return;
        event.preventDefault();
        setSelected(new Set(files.map((file) => file.path)));
      }}
    >
      <div className={cn("shrink-0 flex items-center gap-1", fixed ? "px-2 py-1" : "pr-1")}>
        {fixed ? (
          <h3 className="flex-1 min-w-0 truncate px-1 text-[10px] font-semibold uppercase tracking-wide text-text-muted">
            {t("changes.fileSection.titleWithCount", { title, total: files.length })}
          </h3>
        ) : (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            aria-expanded={open}
            onClick={() => setOpen((value) => !value)}
            className={cn(
              "h-auto flex-1 min-w-0 flex items-center gap-1.5 px-3 py-1.5",
              "text-xs font-medium text-text-muted uppercase tracking-wide",
              "hover:bg-bg-primary/60 hover:text-text-secondary",
              "rounded-none border-0 shadow-none bg-transparent",
            )}
          >
            {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
            {t("changes.fileSection.titleWithCount", { title, total: files.length })}
          </Button>
        )}
        {allActionLabel && onAllAction ? (
          <Button
            type="button"
            variant={allActionVariant}
            size="sm"
            className="shrink-0"
            disabled={files.length === 0}
            onClick={(event) => {
              event.stopPropagation();
              onAllAction();
            }}
            title={t("changes.fileSection.allFilesTooltip", { action: allActionLabel })}
          >
            {allActionLabel}
          </Button>
        ) : null}
        <Button
          type="button"
          variant={actionVariant}
          size="sm"
          className={cn("shrink-0 min-w-[4.5rem]", bar ? "" : "mx-1.5")}
          disabled={selectedInSection.length === 0}
          onClick={handleBulk}
          title={t("changes.fileSection.selectedFilesTooltip", { action: actionLabel })}
        >
          {actionLabel}
        </Button>
        {allDangerActionLabel && onAllDangerAction ? (
          <Button
            type="button"
            variant="danger"
            size="sm"
            className="shrink-0"
            disabled={files.length === 0}
            onClick={(event) => {
              event.stopPropagation();
              onAllDangerAction();
            }}
            title={t("changes.fileSection.allFilesTooltip", { action: allDangerActionLabel })}
          >
            {allDangerActionLabel}
          </Button>
        ) : null}
        {dangerActionLabel && onDangerAction ? (
          <Button
            type="button"
            variant="danger-soft"
            size="sm"
            className="shrink-0"
            disabled={selectedInSection.length === 0}
            onClick={handleDangerBulk}
            title={t("changes.fileSection.selectedFilesTooltip", { action: dangerActionLabel })}
          >
            {dangerActionLabel}
          </Button>
        ) : null}
      </div>
      {sectionOpen ? (
        <div
          role="listbox"
          aria-label={title}
          aria-multiselectable="true"
          className={cn(
            "flex-1 min-h-0 overflow-auto select-none",
            fixed ? "px-1 pb-1" : "px-1 pb-2",
          )}
        >
          {files.length === 0 ? (
            <p className="px-2 py-1 text-xs text-text-muted italic">{emptyLabel}</p>
          ) : (
            files.map((file) => {
              const key = `${file.staged ? "s" : "u"}-${file.path}`;
              const row = (
                <FileListItem
                  key={key}
                  change={file}
                  tabIndex={
                    file.path ===
                    (orderedPaths.includes(focusedPath ?? "") ? focusedPath : orderedPaths[0])
                      ? 0
                      : -1
                  }
                  onFocus={() => setFocusedPath(file.path)}
                  onKeyDown={(event) => {
                    const index = orderedPaths.indexOf(file.path);
                    const nextIndex =
                      event.key === "ArrowDown"
                        ? Math.min(index + 1, files.length - 1)
                        : event.key === "ArrowUp"
                          ? Math.max(index - 1, 0)
                          : event.key === "Home"
                            ? 0
                            : event.key === "End"
                              ? files.length - 1
                              : null;
                    if (nextIndex === null) return;
                    event.preventDefault();
                    const path = orderedPaths[nextIndex];
                    if (!path) return;
                    const rows = event.currentTarget
                      .closest('[role="listbox"]')
                      ?.querySelectorAll<HTMLElement>('[role="option"]');
                    rows?.[nextIndex]?.focus();
                    if (!(event.ctrlKey || event.metaKey)) handleFileClick(path, event);
                  }}
                  selected={selected.has(file.path)}
                  active={selectedPath === file.path && selectedStaged === file.staged}
                  onClick={(event) => handleFileClick(file.path, event)}
                  onStageToggle={() => onStageToggle(file)}
                />
              );
              if (!onDiscardFile && !onIgnoreFile) return row;

              const canDiscard = file.kind !== "renamed";
              const canIgnore = file.kind === "untracked" && file.path !== ".gitignore";
              const ignoreDisabledReason =
                file.path === ".gitignore"
                  ? t("changes.ignore.ignoreGitignoreDisabled")
                  : t("changes.ignore.onlyUntrackedDisabled");
              return (
                <ContextMenu key={key}>
                  {/* Radix asChild clones this plain <div> to attach the
                      context-menu trigger props; wrapping FileListItem
                      directly would drop them (custom component contract). */}
                  <ContextMenuTrigger asChild>
                    <div role="presentation">{row}</div>
                  </ContextMenuTrigger>
                  <ContextMenuContent className="max-w-[260px]">
                    <ContextMenuLabel title={file.path}>{file.path}</ContextMenuLabel>
                    <ContextMenuSeparator />
                    <ContextMenuItem
                      destructive
                      disabled={!canDiscard}
                      title={canDiscard ? undefined : t("changes.discard.renamedDisabled")}
                      onSelect={() => file.kind !== "renamed" && onDiscardFile?.(file)}
                    >
                      <Trash2 size={14} />
                      {t("changes.discard.menu")}
                    </ContextMenuItem>
                    <ContextMenuItem
                      disabled={!canIgnore}
                      title={canIgnore ? undefined : ignoreDisabledReason}
                      onSelect={() =>
                        file.kind === "untracked" &&
                        file.path !== ".gitignore" &&
                        onIgnoreFile?.(file)
                      }
                    >
                      <EyeOff size={14} />
                      {t("changes.ignore.menu")}
                    </ContextMenuItem>
                  </ContextMenuContent>
                </ContextMenu>
              );
            })
          )}
        </div>
      ) : null}
    </div>
  );
}

export function ChangesPanel({
  selectedPath,
  selectedStaged,
  onSelectFile,
  layout = "stacked",
  onCommitted,
}: ChangesPanelProps): React.JSX.Element {
  const { t } = useTranslation();
  const bar = layout === "bar";
  const modal = layout === "modal";
  const {
    workspaceId,
    repoId,
    data,
    isLoading,
    isError,
    error,
    actionError,
    setActionError,
    unstagedFiles,
    stagedFiles,
    stage,
    unstage,
    discard,
    ignore,
    commitMessage,
    commitPending,
  } = useWorkingCopy();
  const draftKey = commitDraftKey(workspaceId, repoId);
  const draft = useCommitDraftStore((state) => getCommitDraft(state.drafts, draftKey));
  const updateDraft = useCommitDraftStore((state) => state.update);
  const clearDraft = useCommitDraftStore((state) => state.clear);
  const message = draft.message;
  const setMessage = (value: string) => updateDraft(draftKey, { message: value });
  const amendMode = Boolean(draft.amendHead && draft.amendHead === data?.sha);
  const scopeRef = useRef(draftKey);
  const aiRequestRef = useRef(0);
  if (scopeRef.current !== draftKey) aiRequestRef.current += 1;
  scopeRef.current = draftKey;
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);
  const [amendConfirmOpen, setAmendConfirmOpen] = useState(false);
  const headMessage = data?.head_message ?? null;
  const canAmend = headMessage != null && data?.branch !== "(detached)";
  const [aiBusy, setAiBusy] = useState(false);
  const [aiPromptOpen, setAiPromptOpen] = useState(false);
  const [aiSettingsOpen, setAiSettingsOpen] = useState(false);
  const [gitignoreOpen, setGitignoreOpen] = useState(false);
  const setStatus = useStatusAreaStore((s) => s.setStatus);
  /** Context-menu / bulk action awaiting confirmation (destructive ops only). */
  const [pendingAction, setPendingAction] = useState<
    { type: "discard"; files: FileChange[] } | { type: "ignore"; file: FileChange } | null
  >(null);

  /** Queue a discard confirmation for one or more unstaged files. */
  const requestDiscard = (files: FileChange[]) => {
    const discardable = files.filter((file) => file.kind !== "renamed");
    if (discardable.length === 0) return;
    setPendingAction({ type: "discard", files: discardable });
  };

  const { data: workspace } = useQuery({
    queryKey: ["workspace", workspaceId],
    queryFn: () => getWorkspace(workspaceId!),
    enabled: Boolean(workspaceId),
  });

  useEffect(() => {
    setAiBusy(false);
    setAiPromptOpen(false);
    setAmendConfirmOpen(false);
    setPendingAction(null);
  }, [draftKey]);

  const handleAiGenerate = () => {
    if (!workspaceId || aiBusy) return;
    const request = ++aiRequestRef.current;
    const isCurrent = () =>
      mountedRef.current && scopeRef.current === draftKey && aiRequestRef.current === request;
    setAiBusy(true);
    setActionError(null);
    void (async () => {
      try {
        const currentWorkspace = workspace ?? (await getWorkspace(workspaceId));
        if (!isCurrent()) return;
        if (!currentWorkspace.settings.ai_provider?.trim()) {
          setAiPromptOpen(true);
          return;
        }
        const res = await generateCommitMessage(workspaceId);
        if (!isCurrent()) return;
        updateDraft(draftKey, { message: res.text, previousMessage: message });
        if (res.used_fallback) {
          setStatus(t("changes.ai.fallbackNotice", { provider: res.provider_used }), "info");
        }
      } catch (e) {
        if (isCurrent()) setActionError(formatAppError(e));
      } finally {
        if (isCurrent()) setAiBusy(false);
      }
    })();
  };

  const aiDialogs = (
    <>
      <Modal
        open={aiPromptOpen}
        onOpenChange={setAiPromptOpen}
        title={t("changes.ai.notConfiguredTitle")}
        description={t("changes.ai.notConfiguredDescription")}
        size="sm"
        footer={
          <>
            <Button
              variant="secondary"
              size="sm"
              className="min-w-0 flex-[3]"
              onClick={() => setAiPromptOpen(false)}
            >
              {t("changes.action.notNow")}
            </Button>
            <Button
              variant="primary"
              size="sm"
              className="min-w-0 flex-[7]"
              onClick={() => {
                setAiPromptOpen(false);
                setAiSettingsOpen(true);
              }}
            >
              {t("changes.action.configure")}
            </Button>
          </>
        }
      />
      <GitignoreEditor open={gitignoreOpen} onClose={() => setGitignoreOpen(false)} />
      <AiProviderSettings
        workspaceId={workspaceId}
        workspaceName={workspace?.name}
        open={aiSettingsOpen}
        onOpenChange={setAiSettingsOpen}
      />
    </>
  );

  if (!repoId) {
    return (
      <EmptyState
        icon={<FolderOpen size={24} />}
        title={t("changes.noRepoTitle")}
        description={t("changes.noRepoDescription")}
        className="py-8"
      />
    );
  }

  const wcAlert =
    actionError ?? (isError ? (error ? formatAppError(error) : t("changes.loadFailed")) : null);

  const gitignoreEntry = !bar ? (
    <div className="flex justify-end px-2">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="h-auto px-1 py-0.5 text-[11px] text-text-muted"
        onClick={() => setGitignoreOpen(true)}
      >
        {t("changes.editGitignore")}
      </Button>
    </div>
  ) : null;

  const commitBox = (
    <div
      className={cn(
        "shrink-0",
        bar
          ? "flex flex-col min-h-0 p-2"
          : modal
            ? "border-t border-border-subtle p-2"
            : "border-t border-border-subtle p-3",
      )}
    >
      <CommitMessageBox
        value={message}
        onChange={setMessage}
        onSubmit={() => {
          if ((!amendMode && stagedFiles.length === 0) || !message.trim()) return;
          // Snapshot taken at submit time: unstaged work is not part of the
          // commit, so its presence means the modal should stay open.
          const hasLeftoverUnstaged = unstagedFiles.length > 0;
          commitMessage(message, {
            amend: amendMode,
            onSuccess: () => {
              if (
                getCommitDraft(useCommitDraftStore.getState().drafts, draftKey).message === message
              )
                clearDraft(draftKey);
              if (scopeRef.current !== draftKey || !mountedRef.current) return;
              if (!hasLeftoverUnstaged) onCommitted?.();
            },
          });
        }}
        onAiGenerate={handleAiGenerate}
        aiLoading={aiBusy}
        amendMessage={amendMode ? headMessage : null}
        onAmend={canAmend ? () => setAmendConfirmOpen(true) : undefined}
        disabled={commitPending || aiBusy}
        submitDisabled={!amendMode && stagedFiles.length === 0}
        aiDisabled={stagedFiles.length === 0}
        onRestoreDraft={
          draft.previousMessage !== undefined
            ? () =>
                updateDraft(draftKey, {
                  message: draft.previousMessage!,
                  previousMessage: undefined,
                })
            : undefined
        }
        className={bar ? "h-full" : undefined}
      />
    </div>
  );

  if (isLoading && unstagedFiles.length === 0 && stagedFiles.length === 0) {
    return (
      <div className={cn("flex flex-col h-full min-h-0", bar && "grid grid-cols-[1fr_1fr_280px]")}>
        <div
          className={cn(
            bar
              ? "col-span-2 flex items-center justify-center"
              : "flex-1 flex items-center justify-center",
          )}
        >
          <span className="text-xs text-text-muted italic">{t("changes.loading")}</span>
        </div>
        {gitignoreEntry}
        {commitBox}
        {aiDialogs}
        <ErrorAlert message={wcAlert} onDismiss={() => setActionError(null)} />
      </div>
    );
  }

  const unstagedSection = (
    <FileSection
      key={`${draftKey}-unstaged`}
      title={t("changes.section.unstaged")}
      actionLabel={t("changes.action.stage")}
      actionVariant="secondary"
      allActionLabel={t("changes.action.stageAll")}
      allActionVariant="primary"
      files={unstagedFiles}
      emptyLabel={t("changes.section.noUnstaged")}
      selectedPath={selectedPath}
      selectedStaged={selectedStaged}
      onSelectFile={onSelectFile}
      onStageToggle={(file) => stage([file.path])}
      onBulkAction={(paths) => stage(paths)}
      onAllAction={() => stage(unstagedFiles.map((file) => file.path))}
      onDiscardFile={(file) => setPendingAction({ type: "discard", files: [file] })}
      onIgnoreFile={(file) => setPendingAction({ type: "ignore", file })}
      {...(modal
        ? {
            allDangerActionLabel: t("changes.action.discardAll"),
            onAllDangerAction: () => requestDiscard(unstagedFiles),
            dangerActionLabel: t("changes.action.discard"),
            onDangerAction: (paths: string[]) =>
              requestDiscard(unstagedFiles.filter((file) => paths.includes(file.path))),
          }
        : {})}
      layout={layout}
    />
  );

  const stagedSection = (
    <FileSection
      key={`${draftKey}-staged`}
      title={t("changes.section.staged")}
      actionLabel={t("changes.action.unstage")}
      actionVariant="secondary"
      allActionLabel={t("changes.action.unstageAll")}
      allActionVariant="primary"
      files={stagedFiles}
      emptyLabel={t("changes.section.noStaged")}
      selectedPath={selectedPath}
      selectedStaged={selectedStaged}
      onSelectFile={onSelectFile}
      onStageToggle={(file) => unstage([file.path])}
      onBulkAction={(paths) => unstage(paths)}
      onAllAction={() => unstage(stagedFiles.map((file) => file.path))}
      layout={layout}
    />
  );

  const closePending = (): void => setPendingAction(null);

  const amendConfirm = amendConfirmOpen && headMessage != null && (
    <AmendConfirmModal
      hasUpstream={data?.upstream != null}
      onCancel={() => setAmendConfirmOpen(false)}
      onConfirm={() => {
        updateDraft(draftKey, { message: headMessage, amendHead: data?.sha });
        setAmendConfirmOpen(false);
      }}
    />
  );

  const confirmDialogs =
    pendingAction?.type === "discard" ? (
      <DiscardConfirmModal
        files={pendingAction.files}
        onCancel={closePending}
        onConfirm={() => {
          discard(pendingAction.files.map((file) => file.path));
          closePending();
        }}
      />
    ) : pendingAction?.type === "ignore" ? (
      <IgnoreScopeModal
        file={pendingAction.file}
        onCancel={closePending}
        onAdd={(pattern) => {
          ignore(pattern);
          closePending();
        }}
      />
    ) : null;

  return (
    <div className={cn("flex flex-col h-full min-h-0", bar && "grid grid-cols-[1fr_1fr_280px]")}>
      {unstagedSection}
      {stagedSection}
      {gitignoreEntry}
      {commitBox}
      {aiDialogs}
      {amendConfirm}
      {confirmDialogs}
      <div className={bar ? "col-span-3" : undefined}>
        <ErrorAlert message={wcAlert} onDismiss={() => setActionError(null)} />
      </div>
    </div>
  );
}

/** Confirmation before entering amend mode (rewrites history, §7.4). */
function AmendConfirmModal({
  hasUpstream,
  onCancel,
  onConfirm,
}: {
  hasUpstream: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}): React.JSX.Element {
  const { t } = useTranslation();

  return (
    <Modal
      open
      onOpenChange={(open) => !open && onCancel()}
      title={t("changes.amend.title")}
      description={t("changes.amend.description")}
      size="sm"
      footer={
        <>
          <Button variant="secondary" size="sm" className="min-w-0 flex-[3]" onClick={onCancel}>
            {t("changes.action.cancel")}
          </Button>
          <Button variant="primary" size="sm" className="min-w-0 flex-[7]" onClick={onConfirm}>
            {t("changes.amend.confirm")}
          </Button>
        </>
      }
    >
      {hasUpstream ? (
        <p className="rounded-md border border-warning/50 bg-warning/5 p-2 text-xs text-text-secondary">
          {t("changes.amend.pushedWarning")}
        </p>
      ) : null}
    </Modal>
  );
}

/** Confirmation for the destructive discard of one or more unstaged files. */
function DiscardConfirmModal({
  files,
  onCancel,
  onConfirm,
}: {
  files: FileChange[];
  onCancel: () => void;
  onConfirm: () => void;
}): React.JSX.Element {
  const { t } = useTranslation();
  const single = files.length === 1;
  const first = files[0];
  const untracked = single && first?.kind === "untracked";
  const hasUntracked = files.some((file) => file.kind === "untracked");

  let title: string;
  let description: string;
  if (single && first) {
    title = t("changes.discard.singleTitle", { path: first.path });
    description = untracked
      ? t("changes.discard.singleUntrackedDescription", { path: first.path })
      : t("changes.discard.singleTrackedDescription", { path: first.path });
  } else {
    title = t("changes.discard.multiTitle", { files: files.length });
    description = hasUntracked
      ? t("changes.discard.multiUntrackedDescription", { files: files.length })
      : t("changes.discard.multiTrackedDescription", { files: files.length });
  }

  return (
    <Modal
      open
      onOpenChange={(open) => !open && onCancel()}
      title={title}
      description={description}
      size="sm"
      footer={
        <>
          <Button variant="secondary" size="sm" className="min-w-0 flex-[3]" onClick={onCancel}>
            {t("changes.action.cancel")}
          </Button>
          <Button variant="danger" size="sm" className="min-w-0 flex-[7]" onClick={onConfirm}>
            {single
              ? untracked
                ? t("changes.discard.deleteFile")
                : t("changes.action.discard")
              : t("changes.discard.multiConfirm", { files: files.length })}
          </Button>
        </>
      }
    />
  );
}

/** Lets the user pick what to append to `.gitignore` for an untracked file. */
function IgnoreScopeModal({
  file,
  onCancel,
  onAdd,
}: {
  file: FileChange;
  onCancel: () => void;
  onAdd: (pattern: string) => void;
}): React.JSX.Element {
  const { t } = useTranslation();
  const patterns = deriveIgnorePatterns(file.path);
  const options = [
    { value: patterns.full, label: t("changes.ignore.filePath") },
    ...(patterns.dir ? [{ value: patterns.dir, label: t("changes.ignore.directory") }] : []),
    ...(patterns.ext ? [{ value: patterns.ext, label: t("changes.ignore.extension") }] : []),
  ];
  const [choice, setChoice] = useState(patterns.full);

  return (
    <Modal
      open
      onOpenChange={(open) => !open && onCancel()}
      title={t("changes.ignore.title")}
      description={t("changes.ignore.description")}
      size="sm"
      footer={
        <>
          <Button variant="secondary" size="sm" className="min-w-0 flex-[3]" onClick={onCancel}>
            {t("changes.action.cancel")}
          </Button>
          <Button
            variant="primary"
            size="sm"
            className="min-w-0 flex-[7]"
            onClick={() => onAdd(choice)}
          >
            {t("changes.action.add")}
          </Button>
        </>
      }
    >
      <div className="rounded-xl bg-bg-primary p-3">
        <RadioGroup
          value={choice}
          onChange={setChoice}
          aria-label={t("changes.ignore.scopeLabel")}
          className="flex w-full flex-col gap-1 [&_[data-slot=radio]]:mt-0"
        >
          {options.map((option) => (
            <Radio key={option.value} value={option.value} className="w-full">
              <Radio.Content
                className={cn(
                  "h-auto w-full flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-left transition-colors",
                  choice === option.value
                    ? "border-accent bg-accent/5 ring-accent/30 ring-1"
                    : "border-border-default hover:border-border-strong",
                )}
              >
                <span className="shrink-0 text-sm text-text-primary">{option.label}</span>
                <code className="min-w-0 truncate font-mono text-xs text-text-secondary">
                  {option.value}
                </code>
              </Radio.Content>
            </Radio>
          ))}
        </RadioGroup>
      </div>
    </Modal>
  );
}
