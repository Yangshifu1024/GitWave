// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { invoke } from "@tauri-apps/api/core";
import { getDiffPreview, type FileDiff } from "@/lib/api";
import { RevisionTools } from "./RevisionTools";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/lib/api", () => ({ getDiffPreview: vi.fn(), formatAppError: String }));
vi.mock("@tauri-apps/api/core", () => ({ invoke: vi.fn() }));
vi.mock("@/components/DiffText", () => ({
  DiffText: ({ file }: { file: FileDiff }) => (
    <output aria-label="visible diff">{file.path}</output>
  ),
}));
const base = "1".repeat(40),
  tip = "2".repeat(40);
let client: QueryClient;
beforeEach(() => {
  vi.resetAllMocks();
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
});
afterEach(() => {
  cleanup();
  client.clear();
});
const file = (path: string): FileDiff => ({
  path,
  old_sha: null,
  new_sha: null,
  additions: 0,
  deletions: 0,
  hunks: [],
});
function show(sha: string, onLocate: (oid: string) => void) {
  return (
    <QueryClientProvider client={client}>
      <RevisionTools
        workspaceId="ws"
        repoId="repo"
        sha={sha}
        path="nested/readme.md"
        onLocate={onLocate}
      />
    </QueryClientProvider>
  );
}

it("pins the old tree, compares it to the subsequently selected commit and allows selecting same-named files by path", async () => {
  vi.mocked(getDiffPreview).mockImplementation((_ws, _repo, req) =>
    Promise.resolve({
      truncated: false,
      too_large: false,
      diff: {
        total_additions: 0,
        total_deletions: 0,
        files: req.path
          ? [
              {
                ...file(req.path),
                hunks: [{ old_start: 1, old_lines: 0, new_start: 1, new_lines: 1, lines: [] }],
              },
            ]
          : [file("docs/readme.md"), file("nested/readme.md")],
      },
    }),
  );
  const locate = vi.fn();
  const panel = render(show(base, locate));
  fireEvent.click(screen.getByRole("button", { name: "app.revisions.setBase" }));
  expect(
    screen.getByRole<HTMLButtonElement>("button", { name: /app.revisions.compare/ }).disabled,
  ).toBe(true);
  panel.rerender(show(tip, locate));
  fireEvent.click(screen.getByRole("button", { name: /app.revisions.compare/ }));
  const selection = await screen.findByRole("combobox", { name: "app.revisions.file" });
  expect(getDiffPreview).toHaveBeenCalledWith("ws", "repo", { compare: [base, tip] });
  expect(screen.getByText(`${base.slice(0, 12)} → ${tip.slice(0, 12)}`)).toBeTruthy();
  expect((await screen.findByLabelText("visible diff")).textContent).toBe("docs/readme.md");
  fireEvent.change(selection, { target: { value: "nested/readme.md" } });
  expect((await screen.findByLabelText("visible diff")).textContent).toBe("nested/readme.md");
  expect(locate).not.toHaveBeenCalled();
});

it("pages beyond an empty exact-path scan and opens the matching commit by full OID", async () => {
  vi.mocked(invoke)
    .mockResolvedValueOnce({ entries: [], next: "unvisited-parent" })
    .mockRejectedValueOnce(new Error("read interrupted"))
    .mockResolvedValueOnce({ entries: [[base, "Moved file"]], next: null });
  const locate = vi.fn();
  render(show(tip, locate));
  fireEvent.click(screen.getByRole("button", { name: "app.revisions.path" }));
  const next = await screen.findByRole("button", { name: "palette.continueSearch" });
  expect(invoke).toHaveBeenCalledExactlyOnceWith("cmd_path_history", {
    workspaceId: "ws",
    repoId: "repo",
    start: tip,
    path: "nested/readme.md",
  });
  fireEvent.click(next);
  await screen.findByRole("alert");
  fireEvent.click(screen.getByRole("button", { name: "common.retry" }));
  const row = await screen.findByRole("button", { name: /Moved file/ });
  expect(vi.mocked(invoke).mock.calls[1]).toEqual(vi.mocked(invoke).mock.calls[2]);
  expect(invoke).toHaveBeenLastCalledWith("cmd_path_history", {
    workspaceId: "ws",
    repoId: "repo",
    start: "unvisited-parent",
    path: "nested/readme.md",
  });
  expect(locate).not.toHaveBeenCalled();
  fireEvent.click(row);
  expect(locate).toHaveBeenCalledExactlyOnceWith(base);
  expect(screen.queryByRole("dialog")).toBeNull();
});
