// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getCommitPage, type CommitPage, type CommitSummary } from "@/lib/api";
import { useCommitPages } from "./useCommitPages";

vi.mock("@/lib/api", () => ({ getCommitPage: vi.fn(), formatAppError: (e: unknown) => String(e) }));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (value: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
function page(sha: string | null, cursor: string | null = null): CommitPage {
  const commit: CommitSummary = {
    sha: sha ?? "",
    author: "Test",
    author_email: "test@local",
    time: 0,
    message_summary: sha ?? "",
    lane: 0,
    parents: [],
    refs: [],
  };
  return {
    commits: sha ? [commit] : [],
    next_cursor: cursor,
    has_more: !!cursor,
    scanned: 2000,
    snapshot_size: 3000,
    snapshot_truncated: false,
  };
}
async function start() {
  await act(async () => {
    await Promise.resolve();
    await vi.advanceTimersByTimeAsync(0);
  });
}

describe("useCommitPages request lifecycle", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.mocked(getCommitPage).mockReset();
  });
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("ignores a late result after changing repo, including A→B→A", async () => {
    const oldA = deferred<CommitPage>();
    const b = deferred<CommitPage>();
    const newA = deferred<CommitPage>();
    vi.mocked(getCommitPage)
      .mockReturnValueOnce(oldA.promise)
      .mockReturnValueOnce(b.promise)
      .mockReturnValueOnce(newA.promise);
    const { result, rerender } = renderHook(
      ({ repoId }) => useCommitPages({ workspaceId: "ws", repoId }),
      { initialProps: { repoId: "a" } },
    );
    await start();
    rerender({ repoId: "b" });
    await start();
    rerender({ repoId: "a" });
    await start();
    await act(async () => {
      await Promise.resolve();
      oldA.resolve(page("stale-a"));
      b.reject(new Error("stale error"));
    });
    expect(result.current.loading).toBe(true);
    expect(result.current.error).toBeNull();
    expect(result.current.commits).toEqual([]);
    await act(async () => {
      await Promise.resolve();
      newA.resolve(page("fresh-a"));
    });
    expect(result.current.commits.map((c) => c.sha)).toEqual(["fresh-a"]);
    expect(result.current.loading).toBe(false);
  });

  it("appends exactly one cursor page and suppresses double load-more", async () => {
    const second = deferred<CommitPage>();
    vi.mocked(getCommitPage)
      .mockResolvedValueOnce(page("first", "1:1"))
      .mockReturnValueOnce(second.promise);
    const { result } = renderHook(() =>
      useCommitPages({ workspaceId: "ws", repoId: "repo", pageSize: 10 }),
    );
    await start();
    act(() => {
      result.current.loadMore();
      result.current.loadMore();
    });
    expect(getCommitPage).toHaveBeenCalledTimes(2);
    expect(getCommitPage).toHaveBeenLastCalledWith("ws", "repo", 10, null, "1:1");
    await act(async () => {
      await Promise.resolve();
      second.resolve(page("second"));
    });
    expect(result.current.commits.map((c) => c.sha)).toEqual(["first", "second"]);
    expect(result.current.hasMore).toBe(false);
  });

  it("continues an empty bounded search page and exposes its scan count", async () => {
    vi.mocked(getCommitPage)
      .mockResolvedValueOnce(page(null, "2:2000"))
      .mockResolvedValueOnce(page("match"));
    const { result } = renderHook(() =>
      useCommitPages({ workspaceId: "ws", repoId: "repo", filter: "needle", pageSize: 10 }),
    );
    await start();
    expect(result.current.commits).toEqual([]);
    expect(result.current.hasMore).toBe(true);
    expect(result.current.scanned).toBe(2000);
    await act(async () => {
      await Promise.resolve();
      result.current.loadMore();
    });
    expect(result.current.commits[0]?.sha).toBe("match");
    expect(getCommitPage).toHaveBeenLastCalledWith("ws", "repo", 10, "needle", "2:2000");
  });

  it("restarts an expired snapshot without mixing old and new commits", async () => {
    vi.mocked(getCommitPage)
      .mockResolvedValueOnce(page("old", "3:1"))
      .mockRejectedValueOnce({ code: "git.history_cursor_expired" })
      .mockResolvedValueOnce(page("new"));
    const { result } = renderHook(() => useCommitPages({ workspaceId: "ws", repoId: "repo" }));
    await start();
    await act(async () => {
      await Promise.resolve();
      result.current.loadMore();
    });
    expect(result.current.commits.map((c) => c.sha)).toEqual(["new"]);
    expect(getCommitPage).toHaveBeenLastCalledWith("ws", "repo", 200, null, null);
  });

  it("cancels both the debounce and pending result when disabled", async () => {
    const pending = deferred<CommitPage>();
    vi.mocked(getCommitPage).mockReturnValue(pending.promise);
    const { result, rerender } = renderHook(
      ({ enabled }) =>
        useCommitPages({ workspaceId: "ws", repoId: "repo", enabled, debounceMs: 300 }),
      { initialProps: { enabled: true } },
    );
    rerender({ enabled: false });
    await act(async () => {
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(300);
    });
    expect(getCommitPage).not.toHaveBeenCalled();
    rerender({ enabled: true });
    await act(async () => {
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(300);
    });
    rerender({ enabled: false });
    await act(async () => {
      await Promise.resolve();
      pending.resolve(page("hidden"));
    });
    expect(result.current.commits).toEqual([]);
    expect(result.current.loading).toBe(false);
  });
});
