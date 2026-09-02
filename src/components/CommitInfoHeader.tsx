import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Chip } from "@heroui/react";
import { Cherry, Sparkles, Tag as TagIcon, Undo2 } from "lucide-react";
import { useTranslation } from "react-i18next";

import {
  cherryPickCommit,
  formatAppError,
  getBranches,
  getCommitDetails,
  revertCommit,
} from "@/lib/api";
import type { CommitRef } from "@/lib/api";
import { useTags } from "@/hooks/useTags";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { CommitExplainModal } from "@/components/CommitExplainModal";
import { TagManagerModal } from "@/components/TagManagerModal";
import { RefBadge } from "@/components/RefBadge";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

function formatDateTime(time: number, locale: string): string {
  return new Date(time * 1000).toLocaleString(locale, {
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
  const { t, i18n } = useTranslation();
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
  const { data: tags = [], invalidate: invalidateTags } = useTags();

  // Branch tips (local + remote) so the refs row can show what points here.
  const { data: branches = [] } = useQuery({
    queryKey: ["branches", workspaceId],
    queryFn: () => getBranches(workspaceId),
  });

  if (isLoading) {
    return (
      <div className="shrink-0 border-b border-border-subtle bg-bg-elevated px-4 py-2.5 text-xs text-text-muted">
        {t("commits.header.loading")}
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

  // Everything pointing at this commit: branch tips whose tip sha matches
  // plus tags — the same shapes the history rows badge, so colors and icons
  // stay consistent. The current branch stands out via `emphasize` instead
  // of a literal HEAD badge.
  const commitRefs: Array<CommitRef & { emphasize?: boolean }> = [
    ...branches
      .filter((b) => b.last_commit_sha === data.sha)
      .map((b) => ({
        name: b.name,
        kind: b.kind === "remote" ? ("remote_branch" as const) : ("local_branch" as const),
        emphasize: b.is_current,
      })),
    ...tags
      .filter((tag) => tag.sha === data.sha)
      .map((tag) => ({ name: tag.name, kind: "tag" as const })),
  ];

  const run = (op: CommitAction): void => {
    setBusy(true);
    const request =
      op === "revert" ? revertCommit(workspaceId, sha) : cherryPickCommit(workspaceId, sha);
    request
      .then(() => {
        setStatus(
          op === "revert"
            ? t("commits.revert.done", { sha: shortSha })
            : t("commits.cherryPick.done", { sha: shortSha }),
        );
        void queryClient.invalidateQueries({ queryKey: ["working-copy"] });
        void queryClient.invalidateQueries({ queryKey: ["commit-details", workspaceId, sha] });
        void queryClient.invalidateQueries({ queryKey: ["branches", workspaceId] });
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
        <span>{formatDateTime(data.time, i18n.language)}</span>
      </div>
      {commitRefs.length > 0 ? (
        <div className="mt-2 flex flex-wrap items-center gap-1">
          {commitRefs.map((r) => (
            <RefBadge key={`${r.kind}:${r.name}`} r={r} truncate={false} emphasize={r.emphasize} />
          ))}
        </div>
      ) : null}
      <div className="mt-2 flex flex-wrap items-center gap-1 border-t border-border-subtle pt-1.5">
        <Button
          variant="ghost"
          size="sm"
          aria-label={t("commits.header.explainAria")}
          title={t("commits.header.explainTitle")}
          onClick={() => setExplainOpen(true)}
        >
          <Sparkles size={13} />
          {t("commits.header.explain")}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          aria-label={t("commits.header.tagAria")}
          title={t("commits.header.tagTitle")}
          onClick={() => setTagOpen(true)}
        >
          <TagIcon size={13} />
          {t("commits.header.tag")}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          aria-label={t("commits.header.cherryPickAria")}
          title={t("commits.header.cherryPickTitle")}
          onClick={() => setPending("cherry-pick")}
        >
          <Cherry size={13} />
          {t("commits.action.cherryPick")}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          aria-label={t("commits.header.revertAria")}
          title={t("commits.header.revertTitle")}
          onClick={() => setPending("revert")}
        >
          <Undo2 size={13} />
          {t("commits.action.revert")}
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
            invalidateTags();
            bumpHistory();
          }}
          onError={(message) => setStatus(message, "danger")}
          onCreated={(name) => setStatus(t("commits.tag.created", { sha: shortSha, name }))}
        />
      ) : null}

      {pending ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setPending(null);
          }}
          title={
            pending === "revert"
              ? t("commits.revert.confirmTitle", { sha: shortSha })
              : t("commits.cherryPick.confirmTitle", { sha: shortSha })
          }
          description={
            pending === "revert"
              ? t("commits.revert.confirmDescription", { subject })
              : t("commits.cherryPick.confirmDescription", { subject })
          }
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setPending(null)}
              >
                {t("commits.action.cancel")}
              </Button>
              <Button
                variant={pending === "revert" ? "danger" : "primary"}
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy}
                onClick={() => run(pending)}
              >
                {pending === "revert" ? t("commits.action.revert") : t("commits.action.cherryPick")}
              </Button>
            </>
          }
        />
      ) : null}
    </div>
  );
}
