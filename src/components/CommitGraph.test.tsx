// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
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
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(640);
  HTMLElement.prototype.scrollTo = vi.fn(function (
    this: HTMLElement,
    optionsOrX?: ScrollToOptions | number,
    y?: number,
  ) {
    if (typeof optionsOrX === "number") {
      this.scrollLeft = optionsOrX;
      this.scrollTop = y ?? 0;
    } else {
      this.scrollLeft = optionsOrX?.left ?? this.scrollLeft;
      this.scrollTop = optionsOrX?.top ?? this.scrollTop;
    }
    this.dispatchEvent(new Event("scroll"));
  });
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
  delete (HTMLElement.prototype as Partial<HTMLElement>).scrollTo;
});

function commitRow(index: number) {
  return screen.getByText(`Commit ${index}`).closest('[role="button"]')!;
}
function scrollContainer(row: Element): HTMLDivElement {
  return row.parentElement!.parentElement!.parentElement as HTMLDivElement;
}

function expectUndistortedGraph(svg: SVGSVGElement) {
  // Each SVG unit must occupy one CSS pixel on both axes. A bounded outer
  // graph viewport may scroll, but it must never squeeze the SVG itself.
  expect(svg.hasAttribute("viewBox")).toBe(false);
  expect(svg.hasAttribute("preserveAspectRatio")).toBe(false);
  expect(svg.querySelector("circle")).toBeTruthy();
}

function nodeX(row: Element) {
  return Number(row.querySelector("circle")!.getAttribute("cx"));
}

function expectNodeVisible(row: Element, viewport: HTMLElement) {
  expect(nodeX(row) - viewport.scrollLeft).toBeGreaterThanOrEqual(0);
  expect(nodeX(row) - viewport.scrollLeft).toBeLessThanOrEqual(viewport.clientWidth);
}

it("always renders full topology without the redundant heading or mode switch", () => {
  const select = vi.fn();
  render(<CommitGraph onCommitSelect={select} />);
  expect(screen.queryByText("branches.graph.title")).toBeNull();
  expect(screen.queryByRole("button", { name: "branches.graph.fullGraph" })).toBeNull();
  expect(screen.queryByRole("button", { name: "branches.graph.compactGraph" })).toBeNull();
  const viewport = scrollContainer(commitRow(0));
  fireEvent.scroll(viewport, { target: { scrollTop: 1680 } });
  const selectedRow = commitRow(60);
  fireEvent.click(selectedRow);
  expect(select).toHaveBeenCalledExactlyOnceWith("sha-60");
  expect(selectedRow.getAttribute("aria-pressed")).toBe("true");
  expectUndistortedGraph(selectedRow.querySelector("svg")!);
  expect(nodeX(commitRow(61)) - nodeX(selectedRow)).toBe(18);
  expectNodeVisible(selectedRow, viewport);
  expect(viewport.scrollTop).toBe(1680);
});

it("does not pull a manually browsed graph back to the selection when history pages append or reload", () => {
  const view = render(<CommitGraph selectedSha="sha-39" />);
  const viewport = scrollContainer(commitRow(0));
  expect(viewport.scrollLeft).toBeGreaterThan(0);
  fireEvent.scroll(viewport, { target: { scrollLeft: 45, scrollTop: 280 } });
  commits = [...commits, { ...commits[299]!, sha: "older", lane: 50 }];
  view.rerender(<CommitGraph selectedSha="sha-39" />);
  expect(viewport.scrollLeft).toBe(45);
  expect(viewport.scrollTop).toBe(280);
  commits = commits.map((commit) => ({ ...commit }));
  view.rerender(<CommitGraph selectedSha="sha-39" />);
  expect(viewport.scrollLeft).toBe(45);
  expect(viewport.scrollTop).toBe(280);
  act(() => useWorkspaceUiStore.getState().bumpHistoryEpoch());
  expect(viewport.scrollLeft).toBe(45);
});

it("reveals the same selected commit again after the user pans away and selects it again", () => {
  const select = vi.fn();
  render(<CommitGraph onCommitSelect={select} />);
  const viewport = scrollContainer(commitRow(0));
  fireEvent.scroll(viewport, { target: { scrollTop: 2184 } });
  const row = commitRow(79);
  fireEvent.click(row);
  expectNodeVisible(row, viewport);
  expect(row.getAttribute("aria-pressed")).toBe("true");
  fireEvent.scroll(viewport, { target: { scrollLeft: 0 } });
  expect(nodeX(row) - viewport.scrollLeft).toBeGreaterThan(viewport.clientWidth);
  fireEvent.click(row);
  expectNodeVisible(row, viewport);
  expect(viewport.scrollTop).toBe(2184);
  expect(row.getAttribute("aria-pressed")).toBe("true");
  expect(select).toHaveBeenNthCalledWith(1, "sha-79");
  expect(select).toHaveBeenNthCalledWith(2, "sha-79");
});

it("routes a merge curve to its parent lane at full spacing", () => {
  commits[0]!.parents = ["sha-1", "sha-2"];
  render(<CommitGraph />);
  const mergeRow = commitRow(0);
  const curve = mergeRow
    .querySelector("path")!
    .getAttribute("d")!
    .match(/\d+(?:\.\d+)?/g)!
    .map(Number);
  expect(curve[0]).toBe(nodeX(mergeRow));
  expect(curve[6]).toBe(nodeX(commitRow(2)));
  expect(curve[6]! - curve[0]!).toBe(36);
  expectUndistortedGraph(mergeRow.querySelector("svg")!);
});

it("reveals a located high-lane node and handles each locate request once", () => {
  const view = render(<CommitGraph />);
  const viewport = scrollContainer(commitRow(0));
  view.rerender(
    <CommitGraph
      selectedSha="sha-239"
      locateRequest={{ repoId: "repo", sha: "sha-239", seq: 1 }}
    />,
  );
  expectNodeVisible(commitRow(239), viewport);
  expect(viewport.scrollLeft).toBeGreaterThan(0);
  expect(viewport.scrollTop).toBeGreaterThan(0);
  const locatedTop = viewport.scrollTop;
  fireEvent.scroll(viewport, { target: { scrollLeft: 40 } });
  commits = commits.map((commit) => ({ ...commit }));
  view.rerender(
    <CommitGraph
      selectedSha="sha-239"
      locateRequest={{ repoId: "repo", sha: "sha-239", seq: 1 }}
    />,
  );
  expect(viewport.scrollLeft).toBe(40);
  expect(viewport.scrollTop).toBe(locatedTop);
  view.rerender(
    <CommitGraph
      selectedSha="sha-239"
      locateRequest={{ repoId: "repo", sha: "sha-239", seq: 2 }}
    />,
  );
  expectNodeVisible(commitRow(239), viewport);
  expect(viewport.scrollTop).toBe(locatedTop);
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
  expect(within(row as HTMLElement).getByText("v0.10.2")).toBeTruthy();
  expect(within(row as HTMLElement).queryByText("main")).toBeNull();
  expect(within(row as HTMLElement).queryByText("origin/main")).toBeNull();
  const more = within(row as HTMLElement).getByRole("button", { name: "branches.graph.allRefs 3" });
  expect(more.textContent).toBe("+2");
  fireEvent.click(more);
  expect(select).not.toHaveBeenCalled();
  const dialog = screen.getByRole("dialog", { name: "branches.graph.allRefs 3" });
  expect(within(dialog).getByText("main")).toBeTruthy();
  expect(within(dialog).getByLabelText("synced main")).toBeTruthy();
  expect(within(dialog).getByText("v0.10.2")).toBeTruthy();
  expect(within(dialog).getByText("feature/a-very-long-branch-name")).toBeTruthy();
  expect(Array.from(dialog.querySelectorAll("span[aria-label]")).map((e) => e.textContent)).toEqual(
    ["v0.10.2", "main", "feature/a-very-long-branch-name"],
  );
});

it("prefers a tag over a remote branch on the same commit", () => {
  commits[0]!.refs = [
    { name: "origin/main", kind: "remote_branch" },
    { name: "v0.10.4", kind: "tag" },
  ];
  render(<CommitGraph />);
  const row = commitRow(0) as HTMLElement;
  expect(within(row).getByText("v0.10.4")).toBeTruthy();
  expect(within(row).queryByText("origin/main")).toBeNull();
  expect(within(row).getByRole("button", { name: "branches.graph.allRefs 2" }).textContent).toBe(
    "+1",
  );
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
