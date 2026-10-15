// @vitest-environment jsdom
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { getBlame, type BlameLine } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { BlameView } from "./BlameView";

vi.mock("@/lib/api", () => ({ getBlame: vi.fn(), formatAppError: (e: unknown) => String(e) }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key, i18n: { language: "en" } }),
}));
afterEach(() => {
  cleanup();
  vi.mocked(getBlame).mockReset();
});

it("keeps full attribution available and omits the duplicate path in a diff inspector", async () => {
  useWorkspaceUiStore.setState({ activeWorkspaceId: "ws", activeRepoId: "repo" });
  vi.mocked(getBlame).mockResolvedValue([
    {
      line_no: 1,
      sha: "abcdef0123456789",
      author: "A very long author name",
      author_email: "author@example.test",
      time: 0,
      content: "const value = 1;",
    },
  ]);
  render(<BlameView path="file.ts" hideHeader />);
  expect(await screen.findByText("const value = 1;")).toBeTruthy();
  const details = screen.getByTitle(/abcdef0123456789/);
  expect(details.title).toContain("A very long author name <author@example.test>");
  expect(screen.queryByText("file.ts")).toBeNull();
});

it("keeps the current file when an earlier blame finishes late", async () => {
  let resolveOld!: (lines: BlameLine[]) => void;
  let resolveNew!: (lines: BlameLine[]) => void;
  vi.mocked(getBlame)
    .mockReturnValueOnce(
      new Promise((resolve) => {
        resolveOld = resolve;
      }),
    )
    .mockReturnValueOnce(
      new Promise((resolve) => {
        resolveNew = resolve;
      }),
    );
  useWorkspaceUiStore.setState({ activeWorkspaceId: "ws", activeRepoId: "repo" });
  const { rerender } = render(<BlameView path="old.ts" />);
  rerender(<BlameView path="new.ts" />);
  const line = (content: string): BlameLine => ({
    line_no: 1,
    sha: "abc1234",
    author: "Test",
    author_email: "t@local",
    time: 0,
    content,
  });
  await act(async () => {
    await Promise.resolve();
    resolveNew([line("current content")]);
  });
  await act(async () => {
    await Promise.resolve();
    resolveOld([line("stale content")]);
  });
  expect(screen.queryByText("stale content")).toBeNull();
  expect(screen.getByText("current content")).toBeTruthy();
  expect(getBlame).toHaveBeenLastCalledWith("ws", "new.ts", "repo");
});
