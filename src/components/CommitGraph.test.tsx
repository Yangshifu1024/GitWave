// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ComponentProps, ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CommitGraph } from "./CommitGraph";
import { useCommitPages } from "@/hooks/useCommitPages";
import type { CommitRef, CommitSummary } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

vi.mock("@/hooks/useCommitPages", () => ({ useCommitPages: vi.fn() }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: { count?: number }) =>
      options?.count === undefined ? key : `${key} ${options.count}`,
  }),
}));
vi.mock("@heroui/react", () => ({
  Surface: ({ children, variant, ...props }: ComponentProps<"div"> & { variant?: string }) => (
    <div {...props} data-variant={variant}>
      {children}
    </div>
  ),
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, "aria-pressed": pressed }: ComponentProps<"button">) => (
    <button onClick={onClick} aria-pressed={pressed}>
      {children}
    </button>
  ),
}));
vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, title, children }: { open: boolean; title: string; children: ReactNode }) =>
    open ? (
      <div role="dialog" aria-label={title}>
        {children}
      </div>
    ) : null,
}));
vi.mock("@/components/ui/ContextMenu", () => ({
  ContextMenu: ({ children }: { children: ReactNode }) => <>{children}</>,
  ContextMenuTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  ContextMenuContent: () => null,
}));
vi.mock("@/components/CommitContextMenu", () => ({
  CommitMenuItems: () => null,
  useCommitMenuActions: () => ({
    currentBranch: "main",
    headSha: "sha-0",
    onAction: vi.fn(),
    renderModals: () => null,
  }),
}));
vi.mock("@/components/RefBadgeContextMenu", () => ({
  RefBadgeContextMenu: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
vi.mock("@/components/RefBadge", () => ({
  laneColor: () => "gray",
  RefBadge: ({ r, synced }: { r: CommitRef; synced?: boolean }) => (
    <span aria-label={synced ? `synced ${r.name}` : r.name}>{r.name}</span>
  ),
}));

let commits: CommitSummary[];
beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe = vi.fn();
      disconnect = vi.fn();
    },
  );
  vi.stubGlobal("requestAnimationFrame", vi.fn());
  vi.stubGlobal("cancelAnimationFrame", vi.fn());
  vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(280);
  commits = Array.from({ length: 300 }, (_, index) => ({
    sha: `sha-${index}`,
    author: "Author",
    author_email: "author@example.test",
    time: 1700000000,
    message_summary: `Commit ${index}`,
    lane: index % 40,
    parents: index < 299 ? [`sha-${index + 1}`] : [],
    refs: [],
  }));
  vi.mocked(useCommitPages).mockImplementation(() => ({
    commits,
    loading: false,
    error: null,
    hasMore: false,
    snapshotTruncated: false,
    snapshotSize: commits.length,
    scanned: commits.length,
    loadMore: vi.fn(),
    retry: vi.fn(),
  }));
  useWorkspaceUiStore.setState({ activeWorkspaceId: "ws", activeRepoId: "repo", historyEpoch: 0 });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function commitRow(index: number) {
  return screen.getByText(`Commit ${index}`).closest('[role="button"]')!;
}
function scrollContainer(row: Element): HTMLDivElement {
  return row.parentElement!.parentElement!.parentElement as HTMLDivElement;
}

it("switches to full topology without losing the scroll node or selected commit", () => {
  const select = vi.fn();
  render(<CommitGraph onCommitSelect={select} />);
  const viewport = scrollContainer(commitRow(0));
  viewport.scrollTo = vi.fn((optionsOrX?: ScrollToOptions | number, y?: number) => {
    if (typeof optionsOrX === "number") {
      viewport.scrollLeft = optionsOrX;
      viewport.scrollTop = y ?? 0;
    } else {
      viewport.scrollLeft = optionsOrX?.left ?? viewport.scrollLeft;
      viewport.scrollTop = optionsOrX?.top ?? viewport.scrollTop;
    }
  });
  fireEvent.scroll(viewport, { target: { scrollTop: 1680 } });
  const selectedRow = commitRow(60);
  fireEvent.click(selectedRow);
  expect(select).toHaveBeenCalledExactlyOnceWith("sha-60");
  expect(selectedRow.getAttribute("aria-pressed")).toBe("true");
  const compactSvg = selectedRow.querySelector("svg")!;
  expect(Number(compactSvg.getAttribute("width"))).toBeLessThanOrEqual(112);
  // All original topology coordinates remain represented in the compact graph.
  expect(Number(compactSvg.getAttribute("viewBox")!.split(" ")[2])).toBeGreaterThan(112);
  fireEvent.click(screen.getByRole("button", { name: "branches.graph.fullGraph" }));
  expect(
    screen
      .getByRole("button", { name: "branches.graph.compactGraph" })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  expect(scrollContainer(commitRow(60))).toBe(viewport);
  expect(viewport.scrollTop).toBe(1680);
  expect(commitRow(60)).toBe(selectedRow);
  expect(selectedRow.getAttribute("aria-pressed")).toBe("true");
  expect(Number(selectedRow.querySelector("svg")!.getAttribute("width"))).toBeGreaterThan(112);
  expect(select).toHaveBeenCalledTimes(1);
  viewport.scrollLeft = 120;
  fireEvent.click(screen.getByRole("button", { name: "branches.graph.compactGraph" }));
  expect(viewport.scrollTop).toBe(1680);
  expect(viewport.scrollLeft).toBe(0);
});

it("folds tracked refs before counting and opens all remaining refs without selecting the row", () => {
  commits[0]!.refs = [
    { name: "origin/main", kind: "remote_branch" },
    { name: "v0.10.2", kind: "tag" },
    { name: "main", kind: "local_branch" },
    { name: "feature/a-very-long-branch-name", kind: "local_branch" },
  ];
  const select = vi.fn();
  render(<CommitGraph onCommitSelect={select} />);
  const row = commitRow(0);
  expect(within(row as HTMLElement).getByLabelText("synced main")).toBeTruthy();
  expect(within(row as HTMLElement).queryByText("origin/main")).toBeNull();
  const more = within(row as HTMLElement).getByRole("button", { name: "branches.graph.allRefs 3" });
  expect(more.textContent).toBe("+2");
  fireEvent.click(more);
  expect(select).not.toHaveBeenCalled();
  const dialog = screen.getByRole("dialog", { name: "branches.graph.allRefs 3" });
  expect(within(dialog).getByText("main")).toBeTruthy();
  expect(within(dialog).getByText("v0.10.2")).toBeTruthy();
  expect(within(dialog).getByText("feature/a-very-long-branch-name")).toBeTruthy();
});

it("keeps rendered history bounded while scrolling a large list", () => {
  render(<CommitGraph />);
  const viewport = scrollContainer(commitRow(0));
  expect(screen.getAllByText(/^Commit \d+$/).length).toBeLessThan(50);
  fireEvent.scroll(viewport, { target: { scrollTop: 5600 } });
  expect(screen.getByText("Commit 200")).toBeTruthy();
  expect(screen.queryByText("Commit 0")).toBeNull();
  expect(screen.getAllByText(/^Commit \d+$/).length).toBeLessThan(50);
});
