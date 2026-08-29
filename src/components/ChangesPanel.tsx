import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, ChevronRight, EyeOff, FolderOpen, Trash2 } from "lucide-react";
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
import { useToast } from "@/components/ui/Toast";
import { Radio, RadioGroup } from "@heroui/react";
import { formatAppError, generateCommitMessage, getWorkspace, type FileChange } from "@/lib/api";
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
  const [open, setOpen] = useState(true);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
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
      modifierFromPointerEvent(event),
      anchor,
    );
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
            {title} ({files.length})
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
            {title} ({files.length})
          </Button>
        )}
        {allActionLabel && onAllAction ? (
          <Button
            type="button"
            variant="secondary"
            size="sm"
            className="shrink-0"
            disabled={files.length === 0}
            onClick={(event) => {
              event.stopPropagation();
              onAllAction();
            }}
            title={`${allActionLabel} files`}
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
          title={`${actionLabel} selected files`}
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
            title={`${allDangerActionLabel} files`}
          >
            {allDangerActionLabel}
          </Button>
        ) : null}
        {dangerActionLabel && onDangerAction ? (
          <Button
            type="button"
            variant="danger"
            size="sm"
            className="shrink-0"
            disabled={selectedInSection.length === 0}
            onClick={handleDangerBulk}
            title={`${dangerActionLabel} selected files`}
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
                  change={file}
                  selected={
                    selected.has(file.path) ||
                    (selectedPath === file.path && selectedStaged === file.staged)
                  }
                  onClick={(event) => handleFileClick(file.path, event)}
                  onStageToggle={() => onStageToggle(file)}
                />
              );
              if (!onDiscardFile && !onIgnoreFile) return row;

              const canDiscard = file.kind !== "renamed";
              const canIgnore = file.kind === "untracked" && file.path !== ".gitignore";
              const ignoreDisabledReason =
                file.path === ".gitignore"
                  ? "Ignoring .gitignore itself is not allowed"
                  : "Only untracked files can be ignored";
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
                      title={canDiscard ? undefined : "Renamed files cannot be discarded yet"}
                      onSelect={() => file.kind !== "renamed" && onDiscardFile?.(file)}
                    >
                      <Trash2 size={14} />
                      Discard changes…
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
                      Add to .gitignore…
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
  const bar = layout === "bar";
  const modal = layout === "modal";
  const {
    workspaceId,
    repoId,
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
  const [message, setMessage] = useState("");
  const [aiBusy, setAiBusy] = useState(false);
  const [aiPromptOpen, setAiPromptOpen] = useState(false);
  const [aiSettingsOpen, setAiSettingsOpen] = useState(false);
  const [gitignoreOpen, setGitignoreOpen] = useState(false);
  const { toast } = useToast();
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

  const startGenerate = (provider: string | null | undefined) => {
    if (!workspaceId) return;
    if (!provider?.trim()) {
      setAiPromptOpen(true);
      return;
    }
    setAiBusy(true);
    setActionError(null);
    generateCommitMessage(workspaceId)
      .then((res) => {
        setMessage(res.text);
        if (res.used_fallback) {
          toast({
            title: `Primary AI provider failed — response served by ${res.provider_used}`,
            variant: "info",
          });
        }
      })
      .catch((e) => setActionError(formatAppError(e)))
      .finally(() => setAiBusy(false));
  };

  const handleAiGenerate = () => {
    if (!workspaceId || aiBusy) return;
    if (workspace) {
      startGenerate(workspace.settings.ai_provider);
      return;
    }
    getWorkspace(workspaceId)
      .then((ws) => startGenerate(ws.settings.ai_provider))
      .catch((e) => setActionError(formatAppError(e)));
  };

  const aiDialogs = (
    <>
      <Modal
        open={aiPromptOpen}
        onOpenChange={setAiPromptOpen}
        title="AI provider not configured"
        description="This workspace has no AI provider. Would you like to configure one now?"
        size="sm"
      >
        <div className="flex justify-end gap-2">
          <Button variant="secondary" size="sm" onClick={() => setAiPromptOpen(false)}>
            Not now
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={() => {
              setAiPromptOpen(false);
              setAiSettingsOpen(true);
            }}
          >
            Configure
          </Button>
        </div>
      </Modal>
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
        title="No repository selected"
        description="Select a repository to see uncommitted changes."
        className="py-8"
      />
    );
  }

  const wcAlert =
    actionError ??
    (isError ? (error ? formatAppError(error) : "Failed to load working copy") : null);

  const gitignoreEntry = !bar ? (
    <div className="flex justify-end px-2">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="h-auto px-1 py-0.5 text-[11px] text-text-muted"
        onClick={() => setGitignoreOpen(true)}
      >
        Edit .gitignore
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
          if (stagedFiles.length === 0 || !message.trim()) return;
          // Snapshot taken at submit time: unstaged work is not part of the
          // commit, so its presence means the modal should stay open.
          const hasLeftoverUnstaged = unstagedFiles.length > 0;
          commitMessage(message, {
            onSuccess: () => {
              setMessage("");
              if (!hasLeftoverUnstaged) onCommitted?.();
            },
          });
        }}
        onAiGenerate={handleAiGenerate}
        disabled={stagedFiles.length === 0 || commitPending || aiBusy}
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
          <span className="text-xs text-text-muted italic">Loading…</span>
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
      title="Unstaged"
      actionLabel="Stage"
      actionVariant="primary"
      allActionLabel="Stage All"
      files={unstagedFiles}
      emptyLabel="No unstaged changes"
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
            allDangerActionLabel: "Discard All",
            onAllDangerAction: () => requestDiscard(unstagedFiles),
            dangerActionLabel: "Discard",
            onDangerAction: (paths: string[]) =>
              requestDiscard(unstagedFiles.filter((file) => paths.includes(file.path))),
          }
        : {})}
      layout={layout}
    />
  );

  const stagedSection = (
    <FileSection
      title="Staged"
      actionLabel="Unstage"
      actionVariant="secondary"
      allActionLabel="Unstage All"
      files={stagedFiles}
      emptyLabel="No staged changes"
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
      {confirmDialogs}
      <div className={bar ? "col-span-3" : undefined}>
        <ErrorAlert message={wcAlert} onDismiss={() => setActionError(null)} />
      </div>
    </div>
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
  const single = files.length === 1;
  const first = files[0];
  const untracked = single && first?.kind === "untracked";
  const hasUntracked = files.some((file) => file.kind === "untracked");

  let title: string;
  let description: string;
  if (single && first) {
    title = `Discard changes in "${first.path}"?`;
    description = untracked
      ? `"${first.path}" is not tracked by git — discarding deletes the file from disk permanently.`
      : `Uncommitted changes in "${first.path}" will be restored to the last staged version. This cannot be undone.`;
  } else {
    title = `Discard changes in ${files.length} files?`;
    description = hasUntracked
      ? `${files.length} files will be discarded: tracked files are restored to their last staged version and untracked files are deleted from disk permanently. This cannot be undone.`
      : `Uncommitted changes in ${files.length} files will be restored to their last staged version. This cannot be undone.`;
  }

  return (
    <Modal
      open
      onOpenChange={(open) => !open && onCancel()}
      title={title}
      description={description}
      size="sm"
    >
      <div className="flex justify-end gap-2">
        <Button variant="secondary" size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button variant="danger" size="sm" onClick={onConfirm}>
          {single ? (untracked ? "Delete file" : "Discard") : `Discard ${files.length} files`}
        </Button>
      </div>
    </Modal>
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
  const patterns = deriveIgnorePatterns(file.path);
  const options = [
    { value: patterns.full, label: "File path" },
    ...(patterns.dir ? [{ value: patterns.dir, label: "Directory" }] : []),
    ...(patterns.ext ? [{ value: patterns.ext, label: "Extension" }] : []),
  ];
  const [choice, setChoice] = useState(patterns.full);

  return (
    <Modal
      open
      onOpenChange={(open) => !open && onCancel()}
      title="Add to .gitignore"
      description={`Appends the chosen pattern to the repo-root .gitignore (shared with collaborators). The file stays on disk but leaves the change list if untracked.`}
      size="sm"
    >
      <RadioGroup
        value={choice}
        onChange={setChoice}
        aria-label="Ignore scope"
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
      <div className="flex justify-end gap-2">
        <Button variant="secondary" size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button variant="primary" size="sm" onClick={() => onAdd(choice)}>
          Add
        </Button>
      </div>
    </Modal>
  );
}
