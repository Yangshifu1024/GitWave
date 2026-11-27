// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { getDiffPreview, type DiffPreview } from "@/lib/api";
import { useWorkbenchStore, workbenchKey } from "@/stores/workbenchStore";
import { HistoryDetails } from "./HistoryDetails";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/lib/api", () => ({ getDiffPreview: vi.fn(), formatAppError: String }));
vi.mock("@/components/CommitInfoHeader", () => ({ CommitInfoHeader: () => null }));
vi.mock("@/components/DiffViewer", () => ({
  DiffViewer: ({ commitOid, path }: { commitOid: string; path: string }) => (
    <output aria-label="Selected diff">
      {commitOid} / {path}
    </output>
  ),
}));

function summary(...paths: string[]): DiffPreview {
  return {
    truncated: false,
    too_large: false,
    diff: {
      total_additions: 0,
      total_deletions: 0,
      files: paths.map((path) => ({
        path,
        old_sha: null,
        new_sha: null,
        additions: 0,
        deletions: 0,
        hunks: [],
      })),
    },
  };
}

let client: QueryClient;
beforeEach(() => {
  vi.resetAllMocks();
  useWorkbenchStore.setState({ sessions: {} });
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
});
afterEach(() => {
  cleanup();
  client.clear();
});

function view(sha: string | null = "abc", repoId = "r", workspaceId = "ws") {
  return (
    <QueryClientProvider client={client}>
      <HistoryDetails
        workspaceId={workspaceId}
        repoId={repoId}
        scope={workbenchKey(workspaceId, repoId)}
        sha={sha}
      />
    </QueryClientProvider>
  );
}
const selectedDiff = () => screen.getByLabelText("Selected diff").textContent;
const expandAll = () =>
  fireEvent.click(screen.getByRole("button", { name: "app.workbench.expandFolders" }));

it("selects the first sorted leaf when the changed-file summary arrives asynchronously", async () => {
  let resolve!: (data: DiffPreview) => void;
  vi.mocked(getDiffPreview).mockReturnValueOnce(
    new Promise((done) => {
      resolve = done;
    }),
  );
  render(view());
  expect(screen.queryByLabelText("Selected diff")).toBeNull();
  await waitFor(() =>
    expect(getDiffPreview).toHaveBeenCalledExactlyOnceWith("ws", "r", {
      commit_oid: "abc",
      expanded: false,
    }),
  );
  act(() =>
    resolve(summary("AAA.md", "src/z.ts", "docs/guide.md", "docs/api/z.md", "docs/api/a.md")),
  );
  await screen.findByRole("button", { name: "docs/api/a.md", pressed: true });
  expect(selectedDiff()).toBe("abc / docs/api/a.md");
  expect(screen.getByRole("button", { name: "src", expanded: false })).toBeTruthy();
  expect(screen.getByText("5")).toBeTruthy();
  expandAll();
  const leaves = screen.getAllByRole("button").filter((el) => el.hasAttribute("aria-pressed"));
  expect(leaves.map((el) => el.getAttribute("aria-label"))).toEqual([
    "docs/api/a.md",
    "docs/api/z.md",
    "docs/guide.md",
    "src/z.ts",
    "AAA.md",
  ]);
});

it("leaves the selected diff intact when its folder or the entire tree is collapsed", async () => {
  vi.mocked(getDiffPreview).mockResolvedValue(
    summary("a.txt", "docs/index.ts", "src/core/index.ts"),
  );
  render(view());
  await screen.findByLabelText("Selected diff");
  expandAll();
  const path = "src/core/index.ts";
  fireEvent.click(screen.getByRole("button", { name: path }));
  expect(screen.getByRole("button", { name: path, pressed: true })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "src", expanded: true }));
  expect(screen.queryByRole("button", { name: path })).toBeNull();
  expect(selectedDiff()).toBe(`abc / ${path}`);
  await act(async () => {
    await client.invalidateQueries({ queryKey: ["diff-preview"] });
  });
  expect(screen.getByRole("button", { name: "src", expanded: false })).toBeTruthy();
  expect(screen.queryByRole("button", { name: path })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "src", expanded: false }));
  expect(screen.getByRole("button", { name: path, pressed: true })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "app.workbench.collapseFolders" }));
  expect(screen.queryByRole("button", { name: path })).toBeNull();
  expect(selectedDiff()).toBe(`abc / ${path}`);
  expandAll();
  expect(screen.getByRole("button", { name: path, pressed: true })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "docs/index.ts" }));
  expect(selectedDiff()).toBe("abc / docs/index.ts");
  expect(getDiffPreview).toHaveBeenCalledTimes(2);
});

it("keeps a file and a directory both addressable at the same path in a type-changing commit", async () => {
  vi.mocked(getDiffPreview).mockResolvedValue(summary("entry", "entry/new.txt"));
  render(view());
  const nested = await screen.findByRole("button", { name: "entry/new.txt", pressed: true });
  const file = screen.getByRole("button", { name: "entry", pressed: false });
  fireEvent.click(file);
  expect(selectedDiff()).toBe("abc / entry");
  fireEvent.click(nested);
  expect(selectedDiff()).toBe("abc / entry/new.txt");
  fireEvent.click(screen.getByRole("button", { name: "entry", expanded: true }));
  expect(screen.queryByRole("button", { name: "entry/new.txt" })).toBeNull();
  expect(screen.getByRole("button", { name: "entry", pressed: false })).toBeTruthy();
});

it("restores the chosen file within its workspace and repo and ignores it for a different commit", async () => {
  vi.mocked(getDiffPreview).mockResolvedValue(summary("root.md", "core/a.ts", "core/b.ts"));
  const panel = render(view());
  await screen.findByRole("button", { name: "core/a.ts", pressed: true });
  fireEvent.click(screen.getByRole("button", { name: "core/b.ts" }));
  panel.rerender(view());
  expect(selectedDiff()).toBe("abc / core/b.ts");
  for (const [repo, ws] of [
    ["other-repo", "ws"],
    ["r", "other-ws"],
  ]) {
    panel.rerender(view("abc", repo, ws));
    await screen.findByRole("button", { name: "core/a.ts", pressed: true });
    expect(selectedDiff()).toBe("abc / core/a.ts");
  }
  panel.rerender(view("def"));
  await screen.findByRole("button", { name: "core/a.ts", pressed: true });
  expect(selectedDiff()).toBe("def / core/a.ts");
  panel.rerender(view());
  await screen.findByRole("button", { name: "core/b.ts", pressed: true });
  expect(selectedDiff()).toBe("abc / core/b.ts");
});

it("falls back to the first sorted leaf when the remembered path disappears on a refetch", async () => {
  vi.mocked(getDiffPreview)
    .mockResolvedValueOnce(summary("README", "src/a.ts", "src/b.ts"))
    .mockResolvedValueOnce(summary("README", "src/c.ts"));
  render(view());
  await screen.findByRole("button", { name: "src/a.ts", pressed: true });
  fireEvent.click(screen.getByRole("button", { name: "src/b.ts" }));
  await act(async () => {
    await client.invalidateQueries({ queryKey: ["diff-preview"] });
  });
  await screen.findByRole("button", { name: "src/c.ts", pressed: true });
  expect(selectedDiff()).toBe("abc / src/c.ts");
});

it("retries a failed summary and shows the empty commit without selecting a file", async () => {
  vi.mocked(getDiffPreview)
    .mockRejectedValueOnce(new Error("read failed"))
    .mockResolvedValueOnce(summary());
  render(view());
  const error = await screen.findByRole("alert");
  expect(error.textContent).toContain("read failed");
  fireEvent.click(screen.getByRole("button", { name: "common.retry" }));
  await screen.findByText("app.workbench.noFiles");
  expect(screen.queryByLabelText("Selected diff")).toBeNull();
  expect(screen.queryByRole("button", { name: "app.workbench.expandFolders" })).toBeNull();
  expect(getDiffPreview).toHaveBeenCalledTimes(2);
});
