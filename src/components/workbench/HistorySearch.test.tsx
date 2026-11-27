// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { aiPaletteIntent, getCommitPage, type CommitPage } from "@/lib/api";
import { HistorySearch } from "./HistorySearch";

vi.mock("@/lib/api", () => ({
  getCommitPage: vi.fn(),
  aiPaletteIntent: vi.fn(),
  formatAppError: String,
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: { scanned?: number; total?: number; count?: number }) =>
      opts?.scanned !== undefined
        ? `${key} ${opts.scanned}/${opts.total}`
        : opts?.count !== undefined
          ? `${key} ${opts.count}`
          : key,
  }),
}));

const fullSha = "abcdef0123456789abcdef0123456789abcdef01";
function page(title?: string, next: string | null = null): CommitPage {
  return {
    commits: title
      ? [
          {
            sha: fullSha,
            author: "Reviewer",
            author_email: "test@example.org",
            time: 0,
            message_summary: title,
            lane: 0,
            parents: [],
            refs: [],
          },
        ]
      : [],
    next_cursor: next,
    has_more: Boolean(next),
    scanned: next ? 2000 : 3000,
    snapshot_size: 3000,
    snapshot_truncated: false,
  };
}
const open = () => fireEvent.click(screen.getByRole("button", { name: "app.workbench.search" }));
const input = () => screen.getByRole("searchbox", { name: "app.workbench.search" });
const search = (value: string) => fireEvent.change(input(), { target: { value } });
const more = () => screen.getByRole("button", { name: "palette.continueSearch" });

beforeEach(() => vi.resetAllMocks());
afterEach(cleanup);

it("continues past an empty local page and locates the full SHA only after selecting a result", async () => {
  const locate = vi.fn();
  vi.mocked(getCommitPage)
    .mockResolvedValueOnce(page(undefined, "opaque:3"))
    .mockResolvedValueOnce(page("Fix search scope"));
  render(<HistorySearch workspaceId="ws" repoId="r" onLocate={locate} />);
  expect(screen.queryByRole("dialog")).toBeNull();
  open();
  expect(screen.getByRole("dialog", { name: "app.workbench.search" })).toBeTruthy();
  expect(getCommitPage).not.toHaveBeenCalled();
  search("  search scope  ");
  const next = await screen.findByRole("button", { name: "palette.continueSearch" });
  expect(getCommitPage).toHaveBeenCalledExactlyOnceWith(
    "ws",
    "r",
    expect.any(Number),
    "search scope",
    null,
  );
  expect(screen.getByText("palette.searchProgress 2000/3000")).toBeTruthy();
  expect(locate).not.toHaveBeenCalled();
  fireEvent.click(next);
  const row = await screen.findByRole("button", { name: /Fix search scope/ });
  expect(row.textContent).toContain(fullSha.slice(0, 7));
  expect(getCommitPage).toHaveBeenLastCalledWith(
    "ws",
    "r",
    expect.any(Number),
    "search scope",
    "opaque:3",
  );
  fireEvent.click(row);
  expect(locate).toHaveBeenCalledExactlyOnceWith(fullSha);
  expect(screen.queryByRole("dialog")).toBeNull();
  open();
  expect((input() as HTMLInputElement).value).toBe("");
  expect(screen.queryByText("Fix search scope")).toBeNull();
  expect(aiPaletteIntent).not.toHaveBeenCalled();
});

it("reports errors locally and retries the same query and cursor without discarding earlier hits", async () => {
  vi.mocked(getCommitPage)
    .mockResolvedValueOnce(page("First result", "cursor:old"))
    .mockRejectedValueOnce(new Error("failed to read commit history"))
    .mockResolvedValueOnce({ ...page(), snapshot_truncated: true });
  render(<HistorySearch workspaceId="ws" repoId="r" onLocate={vi.fn()} />);
  open();
  search("a@example.org");
  await screen.findByRole("button", { name: /First result/ });
  fireEvent.click(more());
  const error = await screen.findByRole("alert");
  expect(error.textContent).toContain("failed to read commit history");
  fireEvent.click(screen.getByRole("button", { name: "common.retry" }));
  await screen.findByText("app.workbench.searchLimit 3000");
  expect(screen.getByRole("button", { name: /First result/ })).toBeTruthy();
  expect(vi.mocked(getCommitPage).mock.calls.map((call) => [call[3], call[4]])).toEqual([
    ["a@example.org", null],
    ["a@example.org", "cursor:old"],
    ["a@example.org", "cursor:old"],
  ]);
  expect(screen.queryByRole("alert")).toBeNull();
  expect(aiPaletteIntent).not.toHaveBeenCalled();
});

it.each(["query", "workspace", "repo"])(
  "ignores late results and cursors after the %s changes",
  async (change) => {
    let finish!: (page: CommitPage) => void;
    const old = new Promise<CommitPage>((resolve) => {
      finish = resolve;
    });
    vi.mocked(getCommitPage).mockReturnValueOnce(old).mockResolvedValueOnce(page("Current result"));
    const locate = vi.fn();
    const show = (workspaceId: string, repoId: string) => (
      <HistorySearch workspaceId={workspaceId} repoId={repoId} onLocate={locate} />
    );
    const panel = render(show("ws", "repo"));
    open();
    search("needle");
    await waitFor(() => expect(getCommitPage).toHaveBeenCalledTimes(1));
    const ws = change === "workspace" ? "other-ws" : "ws";
    const repo = change === "repo" ? "other-repo" : "repo";
    const filter = change === "query" ? "new query" : "needle";
    if (change === "query") search(filter);
    else panel.rerender(show(ws, repo));
    await screen.findByRole("button", { name: /Current result/ });
    expect(getCommitPage).toHaveBeenLastCalledWith(ws, repo, expect.any(Number), filter, null);
    await act(async () => {
      finish(page("Outdated result", "old:next"));
      await old;
    });
    expect(screen.queryByText("Outdated result")).toBeNull();
    expect(screen.queryByRole("button", { name: "palette.continueSearch" })).toBeNull();
    expect(locate).not.toHaveBeenCalled();
  },
);
