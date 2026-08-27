import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, ChevronRight, FolderOpen } from "lucide-react";
import { cn } from "@/lib/utils";
import { FileListItem } from "@/components/ui/FileListItem";
import { EmptyState } from "@/components/ui/EmptyState";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { CommitMessageBox } from "@/components/ui/CommitMessageBox";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { AiProviderSettings } from "@/components/AiProviderSettings";
import { formatAppError, generateCommitMessage, getWorkspace, type FileChange } from "@/lib/api";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";
import { modifierFromPointerEvent, nextFileSelection } from "@/lib/fileSelection";

export interface ChangesPanelProps {
  selectedPath: string | null;
  onSelectFile: (path: string) => void;
}

function FileSection({
  title,
  actionLabel,
  actionVariant,
  allActionLabel,
  files,
  emptyLabel,
  selectedPath,
  onSelectFile,
  onStageToggle,
  onBulkAction,
  onAllAction,
}: {
  title: string;
  actionLabel: string;
  actionVariant: "primary" | "secondary";
  allActionLabel?: string;
  files: FileChange[];
  emptyLabel: string;
  selectedPath: string | null;
  onSelectFile: (path: string) => void;
  onStageToggle: (file: FileChange) => void;
  onBulkAction: (paths: string[]) => void;
  onAllAction?: () => void;
}): React.JSX.Element {
  const [open, setOpen] = useState(true);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [anchor, setAnchor] = useState<string | null>(null);

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
    onSelectFile(path);
  };

  const handleBulk = (event: React.MouseEvent) => {
    event.stopPropagation();
    if (selectedInSection.length === 0) return;
    onBulkAction(selectedInSection);
    setSelected(new Set());
  };

  return (
    <div
      className={cn("flex flex-col min-h-0 border-b border-border-subtle", open ? "flex-1" : "shrink-0")}
      onKeyDown={(event) => {
        if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== "a") return;
        if (event.shiftKey || files.length === 0) return;
        event.preventDefault();
        setSelected(new Set(files.map((file) => file.path)));
      }}
    >
      <div className="shrink-0 flex items-center gap-1 pr-1">
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOpen((value) => !value)}
          className={cn(
            "flex-1 min-w-0 flex items-center gap-1.5 px-3 py-1.5",
            "text-xs font-medium text-text-muted uppercase tracking-wide",
            "hover:bg-bg-primary/60 hover:text-text-secondary",
            "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-inset",
          )}
        >
          {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          {title} ({files.length})
        </button>
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
          className="shrink-0 mx-1.5 min-w-[4.5rem]"
          disabled={selectedInSection.length === 0}
          onClick={handleBulk}
          title={`${actionLabel} selected files`}
        >
          {actionLabel}
        </Button>
      </div>
      {open ? (
        <div
          role="listbox"
          aria-label={title}
          aria-multiselectable="true"
          className="flex-1 min-h-0 overflow-auto px-1 pb-2 select-none"
        >
          {files.length === 0 ? (
            <p className="px-2 py-1 text-xs text-text-muted italic">{emptyLabel}</p>
          ) : (
            files.map((file) => (
              <FileListItem
                key={`${file.staged ? "s" : "u"}-${file.path}`}
                change={file}
                selected={selected.has(file.path) || selectedPath === file.path}
                onClick={(event) => handleFileClick(file.path, event)}
                onStageToggle={() => onStageToggle(file)}
              />
            ))
          )}
        </div>
      ) : null}
    </div>
  );
}

export function ChangesPanel({
  selectedPath,
  onSelectFile,
}: ChangesPanelProps): React.JSX.Element {
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
    commitMessage,
    commitPending,
  } = useWorkingCopy();
  const [message, setMessage] = useState("");
  const [aiBusy, setAiBusy] = useState(false);
  const [aiPromptOpen, setAiPromptOpen] = useState(false);
  const [aiSettingsOpen, setAiSettingsOpen] = useState(false);

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
      .then((msg) => setMessage(msg))
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
    actionError ?? (isError ? (error ? formatAppError(error) : "Failed to load working copy") : null);

  const commitBox = (
    <div className="shrink-0 border-t border-border-subtle p-3">
      <CommitMessageBox
        value={message}
        onChange={setMessage}
        onSubmit={() => {
          if (stagedFiles.length === 0 || !message.trim()) return;
          commitMessage(message, { onSuccess: () => setMessage("") });
        }}
        onAiGenerate={handleAiGenerate}
        disabled={stagedFiles.length === 0 || commitPending || aiBusy}
      />
    </div>
  );

  if (isLoading && unstagedFiles.length === 0 && stagedFiles.length === 0) {
    return (
      <div className="flex flex-col h-full min-h-0">
        <div className="flex-1 flex items-center justify-center">
          <span className="text-xs text-text-muted italic">Loading…</span>
        </div>
        {commitBox}
        {aiDialogs}
        <ErrorAlert message={wcAlert} onDismiss={() => setActionError(null)} />
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full min-h-0">
      <FileSection
        title="Unstaged"
        actionLabel="Stage"
        actionVariant="primary"
        allActionLabel="Stage All"
        files={unstagedFiles}
        emptyLabel="No unstaged changes"
        selectedPath={selectedPath}
        onSelectFile={onSelectFile}
        onStageToggle={(file) => stage([file.path])}
        onBulkAction={(paths) => stage(paths)}
        onAllAction={() => stage(unstagedFiles.map((file) => file.path))}
      />
      <FileSection
        title="Staged"
        actionLabel="Unstage"
        actionVariant="secondary"
        files={stagedFiles}
        emptyLabel="No staged changes"
        selectedPath={selectedPath}
        onSelectFile={onSelectFile}
        onStageToggle={(file) => unstage([file.path])}
        onBulkAction={(paths) => unstage(paths)}
      />
      {commitBox}
      {aiDialogs}
      <ErrorAlert message={wcAlert} onDismiss={() => setActionError(null)} />
    </div>
  );
}
