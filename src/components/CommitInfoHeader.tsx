import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Chip } from "@heroui/react";
import { Cherry, Sparkles, Tag as TagIcon, Trash2, Undo2 } from "lucide-react";

import {
  cherryPickCommit,
  createTag,
  deleteTag,
  formatAppError,
  getCommitDetails,
  listTags,
  revertCommit,
} from "@/lib/api";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Textarea } from "@/components/ui/Textarea";
import { CommitExplainModal } from "@/components/CommitExplainModal";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

function formatDateTime(time: number): string {
  return new Date(time * 1000).toLocaleString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

type CommitAction = "revert" | "cherry-pick";

/**
 * Header card above the inspector diff for the selected commit: full
 * message, author, date, short sha and row actions (revert / cherry-pick).
 */
export function CommitInfoHeader({
  workspaceId,
  sha,
}: {
  workspaceId: string;
  sha: string;
}): React.JSX.Element | null {
  const { data, isLoading, error } = useQuery({
    queryKey: ["commit-details", workspaceId, sha],
    queryFn: () => getCommitDetails(workspaceId, sha),
  });
  const queryClient = useQueryClient();
  const setStatus = useStatusAreaStore((s) => s.setStatus);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const [pending, setPending] = useState<CommitAction | null>(null);
  const [busy, setBusy] = useState(false);
  const [tagOpen, setTagOpen] = useState(false);
  const [explainOpen, setExplainOpen] = useState(false);

  // Tags on this commit (listed inside the tag manager modal).
  const { data: tags = [], refetch: refetchTags } = useQuery({
    queryKey: ["tags", workspaceId],
    queryFn: () => listTags(workspaceId),
  });

  if (isLoading) {
    return (
      <div className="shrink-0 border-b border-border-subtle bg-bg-elevated px-4 py-2.5 text-xs text-text-muted">
        Loading commit…
      </div>
    );
  }
  if (error) {
    return (
      <div className="shrink-0 border-b border-border-subtle bg-bg-elevated px-4 py-2.5 text-xs text-danger">
        {formatAppError(error)}
      </div>
    );
  }
  if (!data) return null;

  const [subject = "", ...bodyLines] = data.message_full.split("\n");
  const body = bodyLines.join("\n").trim();
  const shortSha = data.sha.slice(0, 7);

  const run = (op: CommitAction): void => {
    setBusy(true);
    const request =
      op === "revert" ? revertCommit(workspaceId, sha) : cherryPickCommit(workspaceId, sha);
    request
      .then(() => {
        setStatus(
          op === "revert"
            ? `Reverted ${shortSha}`
            : `Cherry-picked ${shortSha} onto the current branch`,
        );
        void queryClient.invalidateQueries({ queryKey: ["working-copy"] });
        void queryClient.invalidateQueries({ queryKey: ["commit-details", workspaceId, sha] });
        bumpHistory();
      })
      .catch((e) => setStatus(formatAppError(e), "danger"))
      .finally(() => {
        setBusy(false);
        setPending(null);
      });
  };

  return (
    <div className="shrink-0 select-text border-b border-border-subtle bg-bg-elevated px-4 py-2.5">
      <div className="flex items-start justify-between gap-3">
        <p className="min-w-0 text-sm font-semibold break-words text-text-primary">{subject}</p>
        <Chip
          size="sm"
          className="shrink-0 rounded-sm bg-bg-primary px-1.5 py-0.5 font-mono text-xs text-text-muted tabular-nums shadow-none"
          title={data.sha}
        >
          <Chip.Label>{shortSha}</Chip.Label>
        </Chip>
      </div>
      {body ? (
        <p className="mt-1 text-xs leading-5 whitespace-pre-wrap break-words text-text-secondary">
          {body}
        </p>
      ) : null}
      <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-text-muted">
        <span className="font-medium text-text-secondary">{data.author}</span>
        <span className="truncate">{data.author_email}</span>
        <span aria-hidden="true">·</span>
        <span>{formatDateTime(data.time)}</span>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-1 border-t border-border-subtle pt-1.5">
        <Button
          variant="ghost"
          size="sm"
          aria-label="Explain this commit with AI"
          title="Explain with AI"
          onClick={() => setExplainOpen(true)}
        >
          <Sparkles size={13} />
          Explain
        </Button>
        <Button
          variant="ghost"
          size="sm"
          aria-label="Manage tags on this commit"
          title="Manage tags"
          onClick={() => setTagOpen(true)}
        >
          <TagIcon size={13} />
          Tag
        </Button>
        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          aria-label="Cherry-pick this commit onto the current branch"
          title="Cherry-pick onto current branch"
          onClick={() => setPending("cherry-pick")}
        >
          <Cherry size={13} />
          Cherry-pick
        </Button>
        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          aria-label="Revert this commit"
          title="Revert this commit"
          onClick={() => setPending("revert")}
        >
          <Undo2 size={13} />
          Revert
        </Button>
      </div>

      {explainOpen ? (
        <CommitExplainModal
          workspaceId={workspaceId}
          sha={data.sha}
          subject={subject}
          open
          onClose={() => setExplainOpen(false)}
        />
      ) : null}

      {tagOpen ? (
        <TagManagerModal
          workspaceId={workspaceId}
          sha={data.sha}
          tags={tags}
          onClose={() => setTagOpen(false)}
          onChanged={() => {
            void refetchTags();
            bumpHistory();
          }}
          onError={(message) => setStatus(message, "danger")}
          onCreated={(name) => setStatus(`Tagged ${shortSha} as ${name}`)}
        />
      ) : null}

      {pending ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setPending(null);
          }}
          title={pending === "revert" ? `Revert ${shortSha}?` : `Cherry-pick ${shortSha}?`}
          description={
            pending === "revert"
              ? `Creates a commit that undoes "${subject}" on the current branch. The working copy must be clean.`
              : `Applies "${subject}" onto the current branch as a new commit (original author preserved). The working copy must be clean.`
          }
          size="sm"
        >
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={() => setPending(null)}>
              Cancel
            </Button>
            <Button
              variant={pending === "revert" ? "danger" : "primary"}
              size="sm"
              disabled={busy}
              onClick={() => run(pending)}
            >
              {pending === "revert" ? "Revert" : "Cherry-pick"}
            </Button>
          </div>
        </Modal>
      ) : null}
    </div>
  );
}

/** Create (lightweight or annotated) and delete tags on the selected commit. */
function TagManagerModal({
  workspaceId,
  sha,
  tags,
  onClose,
  onChanged,
  onError,
  onCreated,
}: {
  workspaceId: string;
  sha: string;
  tags: { name: string; sha: string; annotation: string | null }[];
  onClose: () => void;
  onChanged: () => void;
  onError: (message: string) => void;
  onCreated: (name: string) => void;
}): React.JSX.Element {
  const [name, setName] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);

  const here = tags.filter((t) => t.sha === sha);

  const create = (): void => {
    if (!name.trim() || busy) return;
    setBusy(true);
    createTag(workspaceId, name.trim(), sha, message.trim() || null)
      .then(() => {
        onCreated(name.trim());
        setName("");
        setMessage("");
        onChanged();
        onClose();
      })
      .catch((e) => onError(formatAppError(e)))
      .finally(() => setBusy(false));
  };

  const remove = (tagName: string): void => {
    if (busy) return;
    setBusy(true);
    deleteTag(workspaceId, tagName)
      .then(() => onChanged())
      .catch((e) => onError(formatAppError(e)))
      .finally(() => setBusy(false));
  };

  return (
    <Modal
      open
      onOpenChange={(open) => !open && onClose()}
      title={`Tags on ${sha.slice(0, 7)}`}
      size="sm"
    >
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-1.5">
          {here.length === 0 ? (
            <p className="text-xs text-text-muted italic">No tags on this commit.</p>
          ) : (
            here.map((t) => (
              <div
                key={t.name}
                className="flex items-center gap-2 rounded-md border border-border-subtle px-2 py-1.5"
              >
                <TagIcon size={13} className="shrink-0 text-text-muted" />
                <span className="min-w-0 flex-1 truncate text-xs font-medium text-text-primary">
                  {t.name}
                  {t.annotation ? (
                    <span className="ml-1 font-normal text-text-muted" title={t.annotation}>
                      (annotated)
                    </span>
                  ) : null}
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  className="p-1 text-text-muted hover:text-danger"
                  aria-label={`Delete tag ${t.name}`}
                  title={`Delete ${t.name}`}
                  disabled={busy}
                  onClick={() => remove(t.name)}
                >
                  <Trash2 size={13} />
                </Button>
              </div>
            ))
          )}
        </div>
        <div className="flex flex-col gap-2 border-t border-border-subtle pt-3">
          <Input
            value={name}
            onChange={setName}
            onKeyDown={(e) => {
              if (e.key === "Enter") create();
            }}
            placeholder="tag name (e.g. v1.2.0)"
            autoFocus
          />
          <Textarea
            value={message}
            onChange={setMessage}
            placeholder="Annotation (optional — leave empty for a lightweight tag)"
            rows={2}
          />
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={onClose}>
              Close
            </Button>
            <Button variant="primary" size="sm" disabled={busy || !name.trim()} onClick={create}>
              Create tag
            </Button>
          </div>
        </div>
      </div>
    </Modal>
  );
}
