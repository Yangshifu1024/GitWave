// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { aiPaletteIntent, explainCommit, getCommitPage } from "@/lib/api";
import { useUiStore } from "@/stores/uiStore";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { CommandPalette } from "./CommandPalette";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/lib/api", () => ({
  aiPaletteIntent: vi.fn(),
  explainCommit: vi.fn(),
  getCommitPage: vi.fn(),
}));

function openPalette() {
  const locate = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <CommandPalette requestLocate={locate} />
    </QueryClientProvider>,
  );
  return { input: screen.getByRole("searchbox"), locate };
}

beforeEach(() => {
  vi.resetAllMocks();
  useUiStore.setState({ paletteOpen: true, settingsOpen: false, menuAction: null });
  useWorkspaceUiStore.setState({ activeWorkspaceId: "ws", activeRepoId: "repo" });
});
afterEach(cleanup);

it("runs the first matching local command when Enter is pressed", () => {
  const { input } = openPalette();
  fireEvent.change(input, { target: { value: "openSettings" } });
  fireEvent.keyDown(input, { key: "Enter", isComposing: true });
  expect(useUiStore.getState().settingsOpen).toBe(false);
  fireEvent.keyDown(input, { key: "Enter" });
  expect(useUiStore.getState().settingsOpen).toBe(true);
  expect(aiPaletteIntent).not.toHaveBeenCalled();
});

it("selects the first incoming commit and lets arrow keys change the Enter target", async () => {
  vi.mocked(getCommitPage).mockResolvedValue({
    next_cursor: null,
    has_more: false,
    snapshot_truncated: false,
    commits: ["aaaaaaa", "bbbbbbb"].map((sha) => ({
      sha,
      message_summary: "match",
      author: "author",
      author_email: "",
      time: 0,
      lane: 0,
      parents: [],
      refs: [],
    })),
    scanned: 2,
    snapshot_size: 2,
  });
  const { input, locate } = openPalette();
  fireEvent.change(input, { target: { value: "match" } });
  fireEvent.keyDown(input, { key: "Enter" });
  expect(locate).not.toHaveBeenCalled();
  const first = await screen.findByRole("button", { name: /aaaaaaa/ });
  expect(first.getAttribute("aria-current")).toBe("true");
  fireEvent.keyDown(input, { key: "ArrowDown" });
  fireEvent.keyDown(input, { key: "Enter" });
  expect(locate).toHaveBeenCalledWith("bbbbbbb");
  expect(aiPaletteIntent).not.toHaveBeenCalled();
});

it("keeps unmatched input local until Ask AI is activated", async () => {
  vi.mocked(getCommitPage).mockResolvedValue({
    next_cursor: null,
    has_more: false,
    snapshot_truncated: false,
    commits: [],
    scanned: 0,
    snapshot_size: 0,
  });
  vi.mocked(aiPaletteIntent).mockResolvedValue({
    action: "none",
    explanation: "",
    params: {},
    requires_confirm: false,
  });
  const { input } = openPalette();
  fireEvent.change(input, { target: { value: "unmatched" } });
  await screen.findByText("palette.askHint");
  fireEvent.keyDown(input, { key: "Enter" });
  expect((input as HTMLInputElement).value).toBe("unmatched");
  expect(aiPaletteIntent).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "palette.askAi" }));
  await waitFor(() => expect(aiPaletteIntent).toHaveBeenCalledWith("ws", "unmatched"));
});

it("drops a late AI action when the user edits the query", async () => {
  let finish!: (intent: Awaited<ReturnType<typeof aiPaletteIntent>>) => void;
  const pending = new Promise<Awaited<ReturnType<typeof aiPaletteIntent>>>((resolve) => {
    finish = resolve;
  });
  vi.mocked(aiPaletteIntent).mockReturnValue(pending);
  const { input } = openPalette();
  fireEvent.change(input, { target: { value: "create a branch" } });
  fireEvent.click(screen.getByRole("button", { name: "palette.askAi" }));
  await waitFor(() => expect(aiPaletteIntent).toHaveBeenCalledTimes(1));
  fireEvent.change(input, { target: { value: "another request" } });
  await act(async () => {
    finish({
      action: "create_branch",
      params: { name: "obsolete" },
      explanation: "obsolete suggestion",
      requires_confirm: true,
    });
    await pending;
  });
  expect(screen.queryByText("obsolete suggestion")).toBeNull();
  expect(screen.queryByRole("button", { name: "common.confirm" })).toBeNull();
});

it("does not send another explanation after reopening and dismissing the palette", async () => {
  vi.mocked(aiPaletteIntent).mockResolvedValue({
    action: "explain_commit",
    explanation: "",
    params: { sha: "abcdef0" },
    requires_confirm: false,
  });
  vi.mocked(explainCommit).mockResolvedValue({
    text: "Generated explanation",
    provider_used: "test-provider",
    used_fallback: false,
  });
  const { input } = openPalette();
  fireEvent.change(input, { target: { value: "explain this commit" } });
  fireEvent.click(screen.getByRole("button", { name: "palette.askAi" }));
  fireEvent.click(await screen.findByRole("button", { name: "palette.run" }));
  await screen.findByText("Generated explanation");

  fireEvent.keyDown(window, { key: "k", ctrlKey: true });
  fireEvent.keyDown(screen.getByRole("searchbox"), { key: "Escape" });

  expect(screen.queryByRole("dialog")).toBeNull();
  expect(explainCommit).toHaveBeenCalledExactlyOnceWith("ws", "abcdef0");
});

it.each([
  ["palette.command.fetchRemote", "repo:fetch"],
  ["menu.branch.pull.label", "branch:pull"],
  ["menu.branch.push.label", "branch:push"],
])("routes %s to the same command as the corresponding application menu", (label, action) => {
  const { input } = openPalette();
  fireEvent.change(input, { target: { value: label } });
  fireEvent.keyDown(input, { key: "Enter" });
  expect(useUiStore.getState().menuAction?.action).toBe(action);
  expect(useUiStore.getState().paletteOpen).toBe(false);
  expect(aiPaletteIntent).not.toHaveBeenCalled();
});
