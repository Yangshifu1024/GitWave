import { beforeEach, expect, it, vi } from "vitest";
import { invoke } from "@tauri-apps/api/core";
import { getBlame, getDiffPreview, getImageContent, getWorkingCopy } from "./api";
vi.mock("@tauri-apps/api/core", () => ({ invoke: vi.fn().mockResolvedValue({}) }));
beforeEach(() => vi.clearAllMocks());
it("binds reads to the requested repository before IPC scheduling", async () => {
  await getWorkingCopy("ws", "repo-a");
  await getBlame("ws", "file.txt", "repo-a");
  await getImageContent("ws", "image.png", undefined, "repo-a");
  await getDiffPreview("ws", "repo-a", { path: "file.txt", staged: false });
  for (const call of vi.mocked(invoke).mock.calls)
    expect(call[1]).toMatchObject({ workspaceId: "ws", repoId: "repo-a" });
});
