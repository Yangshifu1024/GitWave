// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import { DiffViewer } from "./DiffViewer";
import { getDiffPreview, type DiffPreview, type FileDiff } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

vi.mock("@/lib/api", () => ({
  getDiffPreview: vi.fn(),
  getImageContent: vi.fn(),
  isImageTooLargeError: () => false,
  formatAppError: String,
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/components/BlameView", () => ({ BlameView: () => null }));
vi.mock("@/components/DiffText", () => ({
  DiffText: ({ file }: { file: FileDiff }) => (
    <div>{file.hunks.flatMap((h) => h.lines.map((l) => l.content)).join("|")}</div>
  ),
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, disabled, "aria-label": label }: ComponentProps<"button">) => (
    <button onClick={onClick} disabled={disabled} aria-label={label}>
      {children}
    </button>
  ),
}));
vi.mock("@heroui/react", () => ({
  Chip: Object.assign(({ children }: ComponentProps<"span">) => <span>{children}</span>, {
    Label: ({ children }: ComponentProps<"span">) => <span>{children}</span>,
  }),
}));

function preview(content: string): DiffPreview {
  return {
    truncated: false,
    too_large: false,
    diff: {
      total_additions: 1,
      total_deletions: 0,
      files: [
        {
          path: "a.txt",
          old_sha: null,
          new_sha: null,
          staged: false,
          additions: 1,
          deletions: 0,
          hunks: content
            ? [
                {
                  old_start: 1,
                  old_lines: 0,
                  new_start: 1,
                  new_lines: 1,
                  lines: [{ kind: "added", content, old_line_no: null, new_line_no: 1 }],
                },
              ]
            : [],
        },
      ],
    },
  };
}
let client: QueryClient;
beforeEach(() => {
  vi.clearAllMocks();
  client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  useWorkspaceUiStore.setState({ activeWorkspaceId: "ws", activeRepoId: "a" });
});
afterEach(() => {
  cleanup();
  client.clear();
});
const wrap = (view: React.ReactNode) => (
  <QueryClientProvider client={client}>{view}</QueryClientProvider>
);

it("loads commit metadata first and only reads an expanded file", async () => {
  vi.mocked(getDiffPreview)
    .mockResolvedValueOnce(preview(""))
    .mockResolvedValue(preview("expanded content"));
  render(wrap(<DiffViewer commitOid="sha" />));
  await screen.findByText("a.txt");
  expect(screen.queryByText("+0")).toBeNull();
  expect(screen.queryByText("-0")).toBeNull();
  expect(getDiffPreview).toHaveBeenCalledTimes(1);
  expect(vi.mocked(getDiffPreview).mock.calls[0]?.[2].path).toBeUndefined();
  fireEvent.click(screen.getByLabelText("diff.file.expand"));
  await screen.findByText("expanded content");
  expect(vi.mocked(getDiffPreview).mock.calls[1]?.[2].path).toBe("a.txt");
});

it("explains oversized content without claiming there are no changes", async () => {
  const large = preview("");
  large.too_large = true;
  large.truncated = true;
  vi.mocked(getDiffPreview).mockResolvedValue(large);
  render(wrap(<DiffViewer workdir path="a.txt" staged={false} />));
  await screen.findByText("diff.preview.tooLarge");
  expect(screen.queryByText("diff.file.noChanges")).toBeNull();
  expect(screen.queryByText("+0")).toBeNull();
});

it("refreshes selected content even when status metadata is unchanged", async () => {
  vi.mocked(getDiffPreview)
    .mockResolvedValueOnce(preview("before edit"))
    .mockResolvedValue(preview("after edit"));
  render(wrap(<DiffViewer workdir path="a.txt" staged={false} />));
  await screen.findByText("before edit");
  await act(async () => {
    await client.invalidateQueries({ queryKey: ["diff-preview"] });
  });
  await screen.findByText("after edit");
  expect(
    vi
      .mocked(getDiffPreview)
      .mock.calls.every((args) => args[2].path === "a.txt" && args[2].staged === false),
  ).toBe(true);
});

it("does not render a previous repository's delayed response", async () => {
  let resolveOld!: (value: DiffPreview) => void;
  vi.mocked(getDiffPreview)
    .mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveOld = resolve;
        }),
    )
    .mockResolvedValue(preview("repository B"));
  render(wrap(<DiffViewer workdir path="a.txt" staged={false} />));
  await waitFor(() => expect(getDiffPreview).toHaveBeenCalledTimes(1));
  act(() => useWorkspaceUiStore.setState({ activeRepoId: "b" }));
  await screen.findByText("repository B");
  await act(async () => {
    resolveOld(preview("repository A"));
    await Promise.resolve();
  });
  expect(screen.queryByText("repository A")).toBeNull();
});
