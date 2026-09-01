import React, { useMemo } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useRef, useState } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import type { CommitRef, CommitSummary } from "@/lib/api";
import { formatAppError, getCommitLog } from "@/lib/api";
import { remoteShortName } from "@/lib/branchNames";
import { resolveLocateIndex, type LocateRequest } from "@/lib/commitLocate";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Surface } from "@heroui/react";
import { cn } from "@/lib/utils";
import { EmptyState } from "@/components/ui/EmptyState";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { FolderOpen } from "lucide-react";
import { laneColor, RefBadge } from "@/components/RefBadge";
import { ContextMenu, ContextMenuContent, ContextMenuTrigger } from "@/components/ui/ContextMenu";
import {
  CommitMenuItems,
  useCommitMenuActions,
  type CommitMenuController,
} from "@/components/CommitContextMenu";
import { RefBadgeContextMenu } from "@/components/RefBadgeContextMenu";

const ROW_H = 28;
const INITIAL_LIMIT = 200;
const PAGE_SIZE = 300;
const LANE_GAP = 14;
const NODE_R = 3.2;

function formatTime(time: number, t: TFunction): string {
  const now = Math.floor(Date.now() / 1000);
  const diff = now - time;
  if (diff < 60) return t("branches.time.justNow");
  if (diff < 3600) return t("branches.time.minutesAgo", { n: Math.floor(diff / 60) });
  if (diff < 86400) return t("branches.time.hoursAgo", { n: Math.floor(diff / 3600) });
  if (diff < 604800) return t("branches.time.daysAgo", { n: Math.floor(diff / 86400) });
  return new Date(time * 1000).toLocaleDateString();
}

function shortSha(sha: string): string {
  return sha.slice(0, 7);
}

function laneX(lane: number): number {
  return LANE_GAP / 2 + lane * LANE_GAP;
}

interface RowArt {
  /** Lanes drawn as a full-height vertical line through this row. */
  verticals: number[];
  /** Lanes whose edge arrives into this row's node from the row top. */
  incoming: number[];
  /** Straight stub from the node down to the row bottom (first parent). */
  hasStub: boolean;
  /** Lanes curving away from the node to the row bottom (additional parents). */
  outCurves: number[];
}

const EMPTY_ROW_ART: RowArt = { verticals: [], incoming: [], hasStub: false, outCurves: [] };

/**
 * Edge routing à la Fork: every child→parent edge travels down the CHILD's
 * lane (additional parents depart sideways toward the parent's lane) and
 * curves into the parent's node in the parent's row. Tips are always solid
 * (stub from the node down), so parallel branch lanes render as long
 * continuous lines instead of per-row elbows.
 */
function computeRowArt(commits: CommitSummary[], shaToIndex: Map<string, number>): RowArt[] {
  const art: RowArt[] = commits.map(() => ({
    verticals: [],
    incoming: [],
    hasStub: false,
    outCurves: [],
  }));

  for (let i = 0; i < commits.length; i++) {
    const c = commits[i]!;
    // A dangling stub still hints at parents beyond the loaded window.
    if (c.parents.length > 0) art[i]!.hasStub = true;

    const parentRows = c.parents
      .map((p) => shaToIndex.get(p))
      .filter((v): v is number => v !== undefined);

    parentRows.forEach((p, k) => {
      const parent = commits[p]!;
      // First parent travels on the child's lane; additional parents depart
      // toward the parent's lane within the child's row.
      const travelLane = k === 0 ? c.lane : parent.lane;
      for (let r = i + 1; r < p; r++) {
        const row = art[r]!;
        if (!row.verticals.includes(travelLane)) row.verticals.push(travelLane);
      }
      const arrival = art[p]!;
      if (!arrival.incoming.includes(travelLane)) arrival.incoming.push(travelLane);
      if (k > 0 && !art[i]!.outCurves.includes(parent.lane)) {
        art[i]!.outCurves.push(parent.lane);
      }
    });
  }

  return art;
}

interface GraphRowProps {
  commit: CommitSummary;
  art: RowArt;
  maxLane: number;
  isHead: boolean;
}

/** Per-row SVG: through-lines, incoming/outgoing curves, commit node (newest-first). */
function GraphRow({ commit, art, maxLane, isHead }: GraphRowProps): React.JSX.Element {
  const width = laneX(maxLane) + LANE_GAP / 2;
  const cy = ROW_H / 2;
  const cx = laneX(commit.lane);
  const color = isHead ? "var(--color-branch-current)" : laneColor(commit.lane);

  return (
    <svg width={width} height={ROW_H} className="shrink-0 overflow-visible" aria-hidden>
      {art.verticals.map((lane) => (
        <line
          key={`thru-${lane}`}
          x1={laneX(lane)}
          y1={0}
          x2={laneX(lane)}
          y2={ROW_H}
          stroke={laneColor(lane)}
          strokeWidth={1.5}
        />
      ))}

      {/* Edges arriving into this node from above */}
      {art.incoming.map((lane) => {
        const x = laneX(lane);
        return x === cx ? (
          <line
            key={`in-${lane}`}
            x1={x}
            y1={0}
            x2={x}
            y2={cy - NODE_R}
            stroke={laneColor(lane)}
            strokeWidth={1.5}
          />
        ) : (
          <path
            key={`in-${lane}`}
            d={`M ${x} 0 C ${x} ${cy * 0.5}, ${cx} ${cy * 0.5}, ${cx} ${cy - NODE_R}`}
            fill="none"
            stroke={laneColor(lane)}
            strokeWidth={1.5}
          />
        );
      })}

      {/* First-parent stub straight down */}
      {art.hasStub ? (
        <line x1={cx} y1={cy + NODE_R} x2={cx} y2={ROW_H} stroke={color} strokeWidth={1.5} />
      ) : null}

      {/* Additional parents depart sideways toward their lane */}
      {art.outCurves.map((lane) => {
        const x = laneX(lane);
        return (
          <path
            key={`out-${lane}`}
            d={`M ${cx} ${cy + NODE_R} C ${cx} ${ROW_H * 0.7}, ${x} ${ROW_H * 0.7}, ${x} ${ROW_H}`}
            fill="none"
            stroke={color}
            strokeWidth={1.5}
          />
        );
      })}

      <circle
        cx={cx}
        cy={cy}
        r={isHead ? NODE_R + 1 : NODE_R}
        fill={color}
        stroke="var(--color-bg-panel)"
        strokeWidth={1.5}
      />
      {isHead ? (
        <circle
          cx={cx}
          cy={cy}
          r={NODE_R + 3}
          fill="none"
          stroke="var(--color-branch-current)"
          strokeWidth={1.5}
        />
      ) : commit.parents.length > 1 ? (
        <circle
          cx={cx}
          cy={cy}
          r={NODE_R + 2}
          fill="none"
          stroke={color}
          strokeWidth={1}
          opacity={0.5}
        />
      ) : null}
    </svg>
  );
}

interface CommitRowProps {
  commit: CommitSummary;
  art: RowArt;
  maxLane: number;
  onSelect: (sha: string) => void;
  isSelected: boolean;
  isHead: boolean;
  /** F011 row context menu controller (shared, modals live in CommitGraph). */
  menu: CommitMenuController;
}

type DisplayRef = CommitRef & { synced?: boolean };

/** Fork-style single badge per branch: a local branch and its tracked
 *  remote branch on the same commit fold into one synced badge (the remote
 *  name disappears, the badge icon becomes a check-in-circle). */
function mergeTrackedRefs(refs: CommitRef[]): DisplayRef[] {
  const locals = new Set(refs.filter((r) => r.kind === "local_branch").map((r) => r.name));
  const remoteShorts = new Set(
    refs.filter((r) => r.kind === "remote_branch").map((r) => remoteShortName(r.name)),
  );
  return refs
    .filter((r) => r.kind !== "remote_branch" || !locals.has(remoteShortName(r.name)))
    .map((r) =>
      r.kind === "local_branch" && remoteShorts.has(r.name) ? { ...r, synced: true } : r,
    );
}

function CommitRow({
  commit,
  art,
  maxLane,
  onSelect,
  isSelected,
  isHead,
  menu,
}: CommitRowProps): React.JSX.Element {
  const { t } = useTranslation();
  const refs = mergeTrackedRefs(commit.refs ?? []);

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <Surface
          variant="transparent"
          role="button"
          tabIndex={0}
          onClick={() => onSelect(commit.sha)}
          onKeyDown={(e) => e.key === "Enter" && onSelect(commit.sha)}
          onContextMenu={() => onSelect(commit.sha)}
          aria-current={isHead ? "true" : undefined}
          className={cn(
            "flex items-center gap-2 px-2 py-0 cursor-pointer rounded-none shadow-none",
            "transition-colors duration-fast border-l-2 border-l-transparent",
            !isSelected && !isHead && "hover:bg-bg-elevated",
            isHead && !isSelected && "bg-accent/10 border-l-accent hover:bg-accent/20",
            isSelected && "bg-accent/20 border-l-accent hover:bg-accent/30",
          )}
          style={{ height: `${ROW_H}px` }}
        >
          <GraphRow commit={commit} art={art} maxLane={maxLane} isHead={isHead} />

          <span className="font-mono text-[10px] text-text-muted w-14 shrink-0 tabular-nums">
            {shortSha(commit.sha)}
          </span>

          {refs.length > 0 ? (
            <span className="flex min-w-0 items-center gap-1 max-w-[55%] overflow-hidden">
              {refs.slice(0, 3).map((r) => (
                <RefBadgeContextMenu
                  key={`${r.kind}:${r.name}`}
                  r={r}
                  onSelect={() => onSelect(commit.sha)}
                >
                  <RefBadge
                    r={r}
                    lane={commit.lane}
                    emphasize={isHead && r.kind !== "remote_branch"}
                    synced={r.synced}
                  />
                </RefBadgeContextMenu>
              ))}
              {refs.length > 3 ? (
                <span className="text-xs text-text-muted shrink-0">+{refs.length - 3}</span>
              ) : null}
            </span>
          ) : null}

          <p
            className={cn(
              "flex-auto min-w-0 text-xs leading-none truncate text-text-primary",
              isHead ? "font-semibold" : "font-medium",
            )}
            title={commit.message_summary}
          >
            {commit.message_summary}
          </p>

          {/* Shrinks first and collapses to nothing before the message gives
              up its room: tight rows hide author/time instead of the refs. */}
          <span className="min-w-0 shrink-[999] overflow-hidden whitespace-nowrap text-[10px] text-text-muted tabular-nums">
            {commit.author} &middot; {formatTime(commit.time, t)}
          </span>
        </Surface>
      </ContextMenuTrigger>
      <ContextMenuContent className="max-w-[260px]">
        <CommitMenuItems
          commit={commit}
          onAction={menu.onAction}
          state={{ currentBranch: menu.currentBranch, headSha: menu.headSha }}
        />
      </ContextMenuContent>
    </ContextMenu>
  );
}

interface CommitGraphProps {
  onCommitSelect?: (sha: string) => void;
  selectedSha?: string | null;
  /** One-shot request to scroll a commit to the viewport center. */
  locateRequest?: LocateRequest | null;
}

export function CommitGraph({
  onCommitSelect,
  selectedSha: selectedShaProp,
  locateRequest,
}: CommitGraphProps): React.JSX.Element {
  const { t } = useTranslation();
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const historyEpoch = useWorkspaceUiStore((s) => s.historyEpoch);
  const [commits, setCommits] = useState<CommitSummary[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [localSelected, setLocalSelected] = useState<string | null>(null);
  const [limit, setLimit] = useState(INITIAL_LIMIT);
  const scrollRef = React.useRef<HTMLDivElement>(null);
  const [searchInput, setSearchInput] = useState("");
  const [filter, setFilter] = useState<string | null>(null);
  // F011: one shared row-menu controller; its modals render once below.
  const menu = useCommitMenuActions(activeWorkspaceId);

  // Debounce keystrokes so each character doesn't trigger a backend walk.
  useEffect(() => {
    const timer = window.setTimeout(() => {
      const needle = searchInput.trim();
      setFilter(needle ? needle : null);
    }, 300);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  const selectedSha = selectedShaProp !== undefined ? selectedShaProp : localSelected;

  // Backend returns newest-first — do not reverse.
  const shaToIndex = useMemo(() => {
    const m = new Map<string, number>();
    commits.forEach((c, i) => m.set(c.sha, i));
    return m;
  }, [commits]);

  const maxLane = useMemo(() => commits.reduce((m, c) => Math.max(m, c.lane), 0), [commits]);

  const rowArtByIndex = useMemo(() => computeRowArt(commits, shaToIndex), [commits, shaToIndex]);

  // Pagination key: context (workspace/repo/epoch) switch resets the window;
  // growing `limit` refetches a larger prefix of the same deterministic walk.
  const fetchKey = `${activeWorkspaceId ?? ""}|${activeRepoId ?? ""}|${historyEpoch}|${filter ?? ""}`;
  const prevFetchKeyRef = React.useRef<string | null>(null);

  useEffect(() => {
    const isNewContext = prevFetchKeyRef.current !== fetchKey;
    prevFetchKeyRef.current = fetchKey;
    if (!activeWorkspaceId || !activeRepoId) {
      setCommits([]);
      setError(null);
      return;
    }
    if (isNewContext && limit !== INITIAL_LIMIT) {
      // Repo / epoch switched: drop the previous window and let this state
      // change re-trigger the fetch with the initial size.
      setLimit(INITIAL_LIMIT);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    getCommitLog(activeWorkspaceId, limit, filter)
      .then(setCommits)
      .catch((e) => {
        if (!cancelled) {
          setCommits([]);
          setError(formatAppError(e));
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [activeWorkspaceId, activeRepoId, historyEpoch, limit, fetchKey, filter]);

  const virtualizer = useVirtualizer({
    count: commits.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_H,
    overscan: 10,
  });

  // "Load more" fires while scrolling near the bottom; a short page means the
  // walk reached the root.
  const hasMore = commits.length >= limit;
  const handleScroll = () => {
    const el = scrollRef.current;
    if (!el || loading || !hasMore) return;
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - 600) {
      setLimit((l) => l + PAGE_SIZE);
    }
  };

  // Locate request from the sidebar (branch click): center the commit in the
  // viewport. One-shot per seq — history refreshes must not yank the scroll
  // position back. A commit missing from the log window (async load still in
  // flight) stays unhandled and retries when shaToIndex updates.
  const handledLocateSeq = useRef(-1);
  useEffect(() => {
    if (!locateRequest) return;
    const index = resolveLocateIndex(
      locateRequest,
      handledLocateSeq.current,
      activeRepoId,
      shaToIndex,
    );
    if (index === null) return;
    handledLocateSeq.current = locateRequest.seq;
    virtualizer.scrollToIndex(index, { align: "center" });
  }, [locateRequest, activeRepoId, shaToIndex, virtualizer]);

  const handleSelect = (sha: string) => {
    setLocalSelected(sha);
    onCommitSelect?.(sha);
  };

  if (!activeWorkspaceId) {
    return (
      <div className="flex items-center justify-center h-full">
        <EmptyState
          icon={<FolderOpen size={22} />}
          title={t("branches.graph.selectWorkspaceTitle")}
          description={t("branches.graph.selectWorkspaceDesc")}
        />
      </div>
    );
  }

  if (!activeRepoId) {
    return (
      <div className="flex items-center justify-center h-full">
        <EmptyState
          icon={<FolderOpen size={22} />}
          title={t("branches.graph.selectRepoTitle")}
          description={t("branches.graph.selectRepoDesc")}
        />
      </div>
    );
  }

  // The search toolbar must stay mounted in every state: the filter lives in
  // this input, and a zero-hit search that replaced the whole panel used to
  // strand it (the input vanished along with the graph, no way to clear).
  const showGraph = !error && !(loading && commits.length === 0) && commits.length > 0;

  let stateContent: React.JSX.Element | null = null;
  if (!showGraph) {
    if (loading && commits.length === 0) {
      stateContent = (
        <div className="flex items-center justify-center h-full text-text-muted text-sm">
          {t("branches.graph.loading")}
        </div>
      );
    } else if (error) {
      stateContent = (
        <div className="flex items-center justify-center h-full text-danger text-sm px-4 text-center">
          {error}
        </div>
      );
    } else if (commits.length === 0) {
      stateContent = filter ? (
        <div className="flex flex-col items-center justify-center h-full gap-3 text-text-muted text-sm">
          <span>{t("branches.graph.noMatch", { filter })}</span>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setSearchInput("");
              setFilter(null);
            }}
          >
            {t("branches.graph.clearSearch")}
          </Button>
        </div>
      ) : (
        <div className="flex items-center justify-center h-full text-text-muted text-sm">
          {t("branches.graph.empty")}
        </div>
      );
    }
  }

  return (
    <div className="h-full flex flex-col min-h-0">
      <div className="shrink-0 border-b border-border-subtle px-3 py-1.5">
        <Input
          variant="search"
          value={searchInput}
          onChange={setSearchInput}
          placeholder={t("branches.graph.searchPlaceholder")}
          className="h-7 bg-bg-panel hover:bg-bg-panel focus-within:bg-bg-panel focus-visible:bg-bg-panel"
        />
      </div>
      {showGraph ? (
        <div ref={scrollRef} className="flex-1 min-h-0 overflow-auto" onScroll={handleScroll}>
          <div
            style={{
              height: `${virtualizer.getTotalSize()}px`,
              width: "100%",
              position: "relative",
            }}
          >
            {virtualizer.getVirtualItems().map((virtualRow) => {
              const commit = commits[virtualRow.index];
              if (!commit) return null;
              return (
                <div
                  key={commit.sha}
                  style={{
                    position: "absolute",
                    top: 0,
                    left: 0,
                    width: "100%",
                    height: `${virtualRow.size}px`,
                    transform: `translateY(${virtualRow.start}px)`,
                  }}
                >
                  <CommitRow
                    commit={commit}
                    art={rowArtByIndex[virtualRow.index] ?? EMPTY_ROW_ART}
                    maxLane={maxLane}
                    onSelect={handleSelect}
                    isSelected={selectedSha === commit.sha}
                    isHead={(commit.refs ?? []).some((r) => r.kind === "head")}
                    menu={menu}
                  />
                </div>
              );
            })}
          </div>

          {loading && commits.length > 0 ? (
            <div className="py-2 text-center text-[10px] text-text-muted">
              {t("branches.graph.loadingOlder")}
            </div>
          ) : !hasMore && commits.length > 0 ? (
            <div className="py-2 text-center text-[10px] text-text-muted">
              {t("branches.graph.endOfHistory", { total: commits.length })}
            </div>
          ) : null}
        </div>
      ) : (
        stateContent
      )}

      {menu.renderModals()}
    </div>
  );
}
