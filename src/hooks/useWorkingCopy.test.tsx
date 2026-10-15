// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useWorkingCopyErrorStore } from "@/stores/workingCopyErrorStore";
import { useWorkingCopy } from "./useWorkingCopy";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { commit, stageFiles } from "@/lib/api";
vi.mock("@/hooks/useRemoteSync", () => ({
  useRemoteSync: () => ({ syncPending: {}, isSyncBusy: false }),
}));
vi.mock("@/lib/api", () => ({
  getWorkingCopy: vi.fn(() => Promise.resolve({ files: [] })),
  stageFiles: vi.fn(async () => {}),
  unstageFiles: vi.fn(),
  discardChanges: vi.fn(),
  ignorePath: vi.fn(),
  amendCommit: vi.fn(),
  commit: vi.fn(),
  formatAppError: String,
}));
beforeEach(() => {
  vi.clearAllMocks();
  useWorkingCopyErrorStore.setState({ errors: {} });
  useWorkspaceUiStore.setState({ activeWorkspaceId: "ws", activeRepoId: "a" });
});
afterEach(cleanup);
function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const invalidate = vi.spyOn(client, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return { ...renderHook(() => useWorkingCopy(), { wrapper }), invalidate };
}
it("pins a staged operation before a repository switch", async () => {
  const hook = setup();
  act(() => {
    hook.result.current.stage(["file.ts"]);
    useWorkspaceUiStore.getState().setActiveRepoId("b");
  });
  await waitFor(() => expect(stageFiles).toHaveBeenCalledWith("ws", ["file.ts"], "a"));
  await waitFor(() =>
    expect(hook.invalidate).toHaveBeenCalledWith({ queryKey: ["working-copy", "ws", "a"] }),
  );
});
it("finishes the original draft callback even when the view unmounts", async () => {
  let finish!: (sha: string) => void;
  vi.mocked(commit).mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const onSuccess = vi.fn();
  const hook = setup();
  act(() => hook.result.current.commitMessage("original", { onSuccess }));
  await waitFor(() => expect(commit).toHaveBeenCalledWith("ws", "original", "a"));
  hook.unmount();
  act(() => useWorkspaceUiStore.getState().setActiveRepoId("b"));
  await act(async () => {
    finish("sha");
    await Promise.resolve();
  });
  expect(onSuccess).toHaveBeenCalledOnce();
  expect(hook.invalidate).toHaveBeenCalledWith({ queryKey: ["working-copy", "ws", "a"] });
});

it("retains a late write failure for the original repo after unmount", async () => {
  let fail!: (error: Error) => void;
  vi.mocked(commit).mockImplementation(
    () =>
      new Promise((_resolve, reject) => {
        fail = reject;
      }),
  );
  const original = setup();
  act(() => original.result.current.commitMessage("draft"));
  await waitFor(() => expect(commit).toHaveBeenCalled());
  original.unmount();
  act(() => useWorkspaceUiStore.getState().setActiveRepoId("b"));
  await act(async () => {
    fail(new Error("commit failed"));
    await Promise.resolve();
  });
  const other = setup();
  expect(other.result.current.actionError).toBeNull();
  act(() => useWorkspaceUiStore.getState().setActiveRepoId("a"));
  expect(other.result.current.actionError).toBe("Error: commit failed");
  act(() => other.result.current.setActionError(null));
  expect(other.result.current.actionError).toBeNull();
});
