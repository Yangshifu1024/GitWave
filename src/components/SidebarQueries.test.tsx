// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState, type ComponentProps, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { listRemoteDetails, listSubmodules, listWorktrees } from "@/lib/api";
import type { SidebarSectionProps } from "@/components/ui/SidebarSection";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { RemotesPanel } from "./RemotesPanel";
import { SubmodulesPanel } from "./SubmodulesPanel";
import { WorktreePanel } from "./WorktreePanel";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/lib/api", () => ({
  listRemoteDetails: vi.fn(),
  listSubmodules: vi.fn(),
  listWorktrees: vi.fn(),
  formatAppError: String,
}));
vi.mock("@heroui/react", () => ({ Menu: () => null, Popover: () => null }));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, disabled, onClick }: ComponentProps<"button">) => (
    <button disabled={disabled} onClick={onClick}>
      {children}
    </button>
  ),
}));
vi.mock("@/components/ui/Input", () => ({ Input: () => null }));
vi.mock("@/components/ui/Modal", () => ({ Modal: () => null }));
vi.mock("@/components/ui/ErrorAlert", () => ({ ErrorAlert: () => null }));
vi.mock("@/components/ui/ListItem", () => ({
  ListItem: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
vi.mock("@/components/ui/ContextMenu", () => ({
  ContextMenu: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  ContextMenuTrigger: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  ContextMenuContent: () => null,
  ContextMenuItem: () => null,
  ContextMenuLabel: () => null,
  ContextMenuSeparator: () => null,
}));
vi.mock("@/components/ui/SidebarSection", () => ({
  // Preserve the section's uncontrolled disclosure contract: remounting a
  // section applies defaultOpen again and loses the user's collapsed state.
  SidebarSection: function Section({
    children,
    defaultOpen = true,
    collapsible = true,
  }: SidebarSectionProps) {
    const [open, setOpen] = useState(defaultOpen);
    return (
      <section>
        {collapsible ? (
          <button aria-label="section" aria-expanded={open} onClick={() => setOpen(!open)}>
            toggle
          </button>
        ) : null}
        {collapsible && open ? children : null}
      </section>
    );
  },
}));

const rows = (prefix: string) =>
  [1, 2].map((index) => ({
    name: `${prefix}-${index}`,
    fetch_url: `https://example.test/${prefix}-${index}`,
    push_url: null,
    path: `/${prefix}-${index}`,
    branch: null,
    is_main: index === 1,
    is_locked: false,
    url: `https://example.test/${prefix}-${index}`,
    initialized: true,
    in_sync: true,
    head_sha: null,
  }));

function deferred() {
  let resolve!: (value: ReturnType<typeof rows>) => void;
  const promise = new Promise<ReturnType<typeof rows>>((yes) => {
    resolve = yes;
  });
  return { promise, resolve };
}

beforeEach(() => {
  vi.resetAllMocks();
  useWorkspaceUiStore.setState({ activeWorkspaceId: "ws", activeRepoId: "a", historyEpoch: 0 });
});
afterEach(cleanup);

describe.each([
  { name: "remotes", Component: RemotesPanel, request: vi.mocked(listRemoteDetails) },
  { name: "worktrees", Component: WorktreePanel, request: vi.mocked(listWorktrees) },
  { name: "submodules", Component: SubmodulesPanel, request: vi.mocked(listSubmodules) },
])("$name query transitions", ({ Component, request }) => {
  const mount = () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <Component />
      </QueryClientProvider>,
    );
  };

  it("preserves rows and user collapse state while the same repository refreshes", async () => {
    const refresh = deferred();
    request.mockResolvedValueOnce(rows("old")).mockReturnValueOnce(refresh.promise);
    mount();
    await waitFor(() => expect(screen.queryByText("old-1")).not.toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "section" }));
    expect(screen.getByRole("button", { name: "section" }).getAttribute("aria-expanded")).toBe(
      "false",
    );
    act(() => useWorkspaceUiStore.getState().bumpHistoryEpoch());
    await waitFor(() => expect(request).toHaveBeenCalledTimes(2));
    expect(screen.getByRole("button", { name: "section" }).getAttribute("aria-expanded")).toBe(
      "false",
    );
    fireEvent.click(screen.getByRole("button", { name: "section" }));
    expect(screen.queryByText("old-1")).not.toBeNull();
    await act(async () => {
      refresh.resolve(rows("fresh"));
      await refresh.promise;
    });
    await waitFor(() => expect(screen.queryByText("fresh-1")).not.toBeNull());
    expect(screen.queryByText("old-1")).toBeNull();
  });

  it.each(["repository", "workspace"])(
    "hides prior rows and ignores late results after switching %s",
    async (scope) => {
      const oldRefresh = deferred();
      const newRepo = deferred();
      request
        .mockResolvedValueOnce(rows("old"))
        .mockReturnValueOnce(oldRefresh.promise)
        .mockReturnValueOnce(newRepo.promise);
      mount();
      await waitFor(() => expect(screen.queryByText("old-1")).not.toBeNull());
      act(() => useWorkspaceUiStore.getState().bumpHistoryEpoch());
      await waitFor(() => expect(request).toHaveBeenCalledTimes(2));
      act(() =>
        useWorkspaceUiStore.setState(
          scope === "repository" ? { activeRepoId: "b" } : { activeWorkspaceId: "other-ws" },
        ),
      );
      await waitFor(() => expect(request).toHaveBeenCalledTimes(3));
      expect(screen.queryByText("old-1")).toBeNull();
      await act(async () => {
        oldRefresh.resolve(rows("stale"));
        await oldRefresh.promise;
      });
      expect(screen.queryByText("stale-1")).toBeNull();
      await act(async () => {
        newRepo.resolve(rows("new"));
        await newRepo.promise;
      });
      await waitFor(() => expect(screen.queryByText("new-1")).not.toBeNull());
      expect(screen.queryByText("old-1")).toBeNull();
    },
  );
});
