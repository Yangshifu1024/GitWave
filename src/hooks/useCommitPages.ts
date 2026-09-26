import { useCallback, useEffect, useRef, useState } from "react";
import { formatAppError, getCommitPage, type CommitPage, type CommitSummary } from "@/lib/api";

/** Cursor requests belong to one repository/query generation, including errors
 * and loading flags. Load-more never refetches or replaces the earlier prefix. */
export function useCommitPages({
  workspaceId,
  repoId,
  filter = null,
  epoch = 0,
  enabled = true,
  pageSize = 200,
  debounceMs = 0,
}: {
  workspaceId: string | null;
  repoId: string | null;
  filter?: string | null;
  epoch?: number;
  enabled?: boolean;
  pageSize?: number;
  debounceMs?: number;
}) {
  const [commits, setCommits] = useState<CommitSummary[]>([]);
  const [page, setPage] = useState<CommitPage | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const generation = useRef(0);
  const busy = useRef(false);
  const fetchPage = useCallback(
    async (cursor: string | null, mine: number) => {
      if (!workspaceId || !repoId || mine !== generation.current || busy.current) return;
      busy.current = true;
      setLoading(true);
      setError(null);
      let replace = cursor === null;
      try {
        let result: CommitPage;
        try {
          result = await getCommitPage(workspaceId, repoId, pageSize, filter, cursor);
        } catch (cause) {
          if (mine !== generation.current) return;
          if (
            !cursor ||
            !(
              typeof cause === "object" &&
              cause !== null &&
              "code" in cause &&
              cause.code === "git.history_cursor_expired"
            )
          )
            throw cause;
          replace = true;
          result = await getCommitPage(workspaceId, repoId, pageSize, filter, null);
        }
        if (mine !== generation.current) return;
        setCommits((previous) => (replace ? result.commits : [...previous, ...result.commits]));
        setPage(result);
      } catch (cause) {
        if (mine === generation.current) setError(formatAppError(cause));
      } finally {
        if (mine === generation.current) {
          busy.current = false;
          setLoading(false);
        }
      }
    },
    [workspaceId, repoId, pageSize, filter],
  );

  useEffect(() => {
    const mine = ++generation.current;
    busy.current = false;
    setCommits([]);
    setPage(null);
    setError(null);
    setLoading(enabled && !!workspaceId && !!repoId);
    if (!enabled || !workspaceId || !repoId) return;
    const timer = window.setTimeout(() => {
      void fetchPage(null, mine);
    }, debounceMs);
    return () => {
      generation.current += 1;
      busy.current = false;
      window.clearTimeout(timer);
    };
  }, [workspaceId, repoId, epoch, enabled, debounceMs, fetchPage]);

  const loadMore = useCallback(() => {
    if (!enabled || !page?.next_cursor || error) return;
    void fetchPage(page.next_cursor, generation.current);
  }, [enabled, page, error, fetchPage]);
  const retry = useCallback(() => {
    void fetchPage(page?.next_cursor ?? null, generation.current);
  }, [page, fetchPage]);
  return {
    commits,
    loading,
    error,
    hasMore: page?.has_more ?? false,
    scanned: page?.scanned ?? 0,
    snapshotSize: page?.snapshot_size ?? 0,
    snapshotTruncated: page?.snapshot_truncated ?? false,
    loadMore,
    retry,
  };
}
