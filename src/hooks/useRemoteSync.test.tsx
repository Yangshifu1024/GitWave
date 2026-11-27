// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { fetchRemote } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { useAuthPromptStore } from "@/stores/authPromptStore";
import { useSyncStore } from "@/stores/syncStore";
import { useOperationHistory } from "@/stores/operationHistory";
import { useRemoteSync } from "./useRemoteSync";
vi.mock("@tauri-apps/api/event", () => ({ listen: () => Promise.resolve(() => {}) }));
vi.mock("@/lib/api", async () => ({
  ...(await vi.importActual("@/lib/api")),
  fetchRemote: vi.fn(),
}));
beforeEach(() => {
  vi.resetAllMocks();
  useSyncStore.setState({ activeOp: null, fading: false });
  useOperationHistory.setState({ entries: [] });
  useWorkspaceUiStore.setState({ activeWorkspaceId: "one", activeRepoId: "r1" });
});
afterEach(cleanup);
it.each(["cancelled", "failed"] as const)(
  "records a %s fetch exactly once for the repo captured on submission",
  async (outcome) => {
    const client = new QueryClient();
    client.setQueryData(["repos", "one"], [{ id: "r1", nickname: "First repo", path: "/first" }]);
    let fail!: (e: unknown) => void;
    vi.mocked(fetchRemote).mockReturnValueOnce(
      new Promise((_res, reject) => {
        fail = reject;
      }),
    );
    const hook = renderHook(() => useRemoteSync(), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    });
    act(() => hook.result.current.fetch({ remote: "origin" }));
    await waitFor(() => expect(fetchRemote).toHaveBeenCalledTimes(1));
    act(() => useWorkspaceUiStore.setState({ activeWorkspaceId: "two", activeRepoId: "r2" }));
    act(() =>
      fail(
        outcome === "cancelled"
          ? { code: "git.sync_cancelled" }
          : new Error("provider detail to omit"),
      ),
    );
    await waitFor(() => expect(useOperationHistory.getState().entries).toHaveLength(1));
    const [entry] = useOperationHistory.getState().entries;
    expect(entry).toMatchObject({
      workspaceId: "one",
      repoId: "r1",
      label: "First repo",
      operation: "fetch",
      outcome,
    });
    expect(JSON.stringify(entry)).not.toContain("provider detail");
  },
);

it.each(["retry", "dismiss", "switch"])(
  "records only the final outcome after authentication: %s",
  async (action) => {
    const client = new QueryClient();
    vi.mocked(fetchRemote)
      .mockRejectedValueOnce({ code: "git.fetch_auth_failed" })
      .mockResolvedValueOnce(undefined);
    const hook = renderHook(() => useRemoteSync(), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    });
    act(() => hook.result.current.fetch());
    await waitFor(() => expect(useAuthPromptStore.getState().retry).toBeTruthy());
    expect(useOperationHistory.getState().entries).toHaveLength(0);
    act(() => {
      if (action === "dismiss") {
        useAuthPromptStore.getState().cancel();
        return;
      }
      if (action === "switch") useWorkspaceUiStore.setState({ activeRepoId: "other" });
      const prompt = useAuthPromptStore.getState();
      prompt.close();
      prompt.retry?.({ username: "user", password: "test", remember: false });
    });
    await waitFor(() => expect(useOperationHistory.getState().entries).toHaveLength(1));
    expect(useOperationHistory.getState().entries[0]).toMatchObject({
      repoId: "r1",
      outcome: action === "retry" ? "success" : "cancelled",
    });
    expect(fetchRemote).toHaveBeenCalledTimes(action === "retry" ? 2 : 1);
  },
);
