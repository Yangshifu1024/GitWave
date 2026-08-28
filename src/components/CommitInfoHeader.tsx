import { useQuery } from "@tanstack/react-query";
import { Chip } from "@heroui/react";

import { formatAppError, getCommitDetails } from "@/lib/api";

function formatDateTime(time: number): string {
  return new Date(time * 1000).toLocaleString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * Header card above the inspector diff for the selected commit: full
 * message, author, date and short sha.
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

  return (
    <div className="shrink-0 select-text border-b border-border-subtle bg-bg-elevated px-4 py-2.5">
      <div className="flex items-start justify-between gap-3">
        <p className="min-w-0 text-sm font-semibold break-words text-text-primary">{subject}</p>
        <Chip
          size="sm"
          className="shrink-0 rounded-sm bg-bg-primary px-1.5 py-0.5 font-mono text-xs text-text-muted tabular-nums shadow-none"
          title={data.sha}
        >
          <Chip.Label>{data.sha.slice(0, 7)}</Chip.Label>
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
    </div>
  );
}
