// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { listConflicts, mergeInProgress, type ConflictFile } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { useMergeConflicts } from "./useMergeConflicts";

vi.mock("@/lib/api", () => ({
  abortMerge: vi.fn(),
  listConflicts: vi.fn(),
  mergeInProgress: vi.fn(),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((yes) => {
    resolve = yes;
  });
  return { promise, resolve };
}

const conflicts: ConflictFile[] = [
  { path: "a.ts", has_ours: true, has_theirs: true, has_base: true },
];

describe("useMergeConflicts repository ownership", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useWorkspaceUiStore.setState({ activeWorkspaceId: "ws", activeRepoId: "a" });
    vi.mocked(mergeInProgress).mockResolvedValue(true);
    vi.mocked(listConflicts).mockResolvedValue(conflicts);
  });
  afterEach(cleanup);

  it("clears a snapshot when leaving its repository and keeps it clear on return", async () => {
    const { result } = renderHook(() => useMergeConflicts());
    await act(async () => {
      await Promise.resolve();
    });
    expect(result.current.files).toEqual(conflicts);
    act(() => useWorkspaceUiStore.setState({ activeRepoId: null }));
    expect(result.current.active).toBe(false);
    expect(result.current.files).toEqual([]);
    const fresh = deferred<ConflictFile[]>();
    vi.mocked(listConflicts).mockReturnValueOnce(fresh.promise);
    act(() => useWorkspaceUiStore.setState({ activeRepoId: "a" }));
    expect(result.current.files).toEqual([]);
    await act(async () => {
      fresh.resolve([]);
      await fresh.promise;
    });
    expect(result.current.files).toEqual([]);
    expect(result.current.mergeInProgress).toBe(true);
  });

  it("ignores late conflicts after A → B → A switches", async () => {
    const oldA = deferred<ConflictFile[]>();
    const b = deferred<ConflictFile[]>();
    const newA = deferred<ConflictFile[]>();
    vi.mocked(listConflicts)
      .mockReturnValueOnce(oldA.promise)
      .mockReturnValueOnce(b.promise)
      .mockReturnValueOnce(newA.promise);
    const { result } = renderHook(() => useMergeConflicts());
    act(() => useWorkspaceUiStore.setState({ activeRepoId: "b" }));
    act(() => useWorkspaceUiStore.setState({ activeRepoId: "a" }));
    await act(async () => {
      oldA.resolve(conflicts);
      b.resolve(conflicts);
      await Promise.all([oldA.promise, b.promise]);
    });
    expect(result.current.files).toEqual([]);
    expect(result.current.active).toBe(false);
    await act(async () => {
      newA.resolve([]);
      await newA.promise;
    });
    expect(result.current.files).toEqual([]);
    expect(result.current.mergeInProgress).toBe(true);
  });

  it("does not fetch when no repository is active", async () => {
    useWorkspaceUiStore.setState({ activeRepoId: null });
    const { result } = renderHook(() => useMergeConflicts());
    await act(async () => result.current.refresh());
    expect(listConflicts).not.toHaveBeenCalled();
    expect(mergeInProgress).not.toHaveBeenCalled();
    expect(result.current.files).toEqual([]);
    expect(result.current.active).toBe(false);
  });
});
