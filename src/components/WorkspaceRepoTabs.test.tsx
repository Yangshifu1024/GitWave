// @vitest-environment jsdom
// Regression cover for "adding a repo lands on tab 1".
//
// The race: an add does two independent round trips — moving the selection
// (setActiveRepo, one sqlite write) and refetching the list (listRepos, which
// stats every repo path). The selection lands first, so for a moment the
// active id points at a repo the rendered list does not contain yet. The tab
// strip's liveness guard reads that as "the selection is dead" and falls back
// to the first repo, which is where the user ended up. Seeding the cache
// before moving the selection is what closes the window.
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { listRepos, setActiveRepo, type RepoRef } from "@/lib/api";
import { useSeedRepos } from "@/hooks/useSeedRepos";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { WorkspaceRepoTabs } from "./WorkspaceRepoTabs";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/lib/api", () => ({
  listRepos: vi.fn(),
  setActiveRepo: vi.fn(),
  relinkRepo: vi.fn(),
  removeRepo: vi.fn(),
  reorderRepos: vi.fn(),
  formatAppError: String,
}));
vi.mock("@heroui/react", () => {
  const Popover = Object.assign(({ children }: { children?: ReactNode }) => <>{children}</>, {
    Trigger: () => null,
    Content: () => null,
  });
  const Menu = Object.assign(({ children }: { children?: ReactNode }) => <>{children}</>, {
    Item: () => null,
  });
  return { Header: () => null, Separator: () => null, Popover, Menu };
});
// Plain markup: the guard under test reads the store, not the DOM, so the
// real React Aria tab machinery is only noise here.
vi.mock("@/components/ui/Tabs", () => ({
  Tabs: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  TabsList: ({ children }: { children?: ReactNode }) => <div role="tablist">{children}</div>,
  TabsTrigger: ({ value, children }: { value: string; children?: ReactNode }) => (
    <div role="tab" data-selected="false" data-value={value}>
      {children}
    </div>
  ),
}));
vi.mock("@/components/ui/Modal", () => ({ Modal: () => null }));
vi.mock("@/components/ui/PathInput", () => ({ PathInput: () => null }));
vi.mock("@/components/ui/ErrorAlert", () => ({ ErrorAlert: () => null }));

const WS = "ws-1";

function repo(id: string, status: RepoRef["status"] = "active"): RepoRef {
  return {
    id,
    workspace_id: WS,
    path: `C:/repos/${id}`,
    nickname: null,
    settings_override: null,
    status,
    missing_since: status === "missing" ? 1 : null,
    added_at: 1,
  };
}

/** Replays the order every add path uses: seed the list, then select. */
function AddHarness({ added }: { added: RepoRef[] }): React.JSX.Element {
  const seedRepos = useSeedRepos();
  return (
    <button
      onClick={() => {
        seedRepos(WS, added);
        const last = added[added.length - 1];
        if (last) useWorkspaceUiStore.getState().setActiveRepoId(last.id);
      }}
    >
      add
    </button>
  );
}

function mount(added: RepoRef[]): void {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <WorkspaceRepoTabs />
      <AddHarness added={added} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(setActiveRepo).mockResolvedValue(undefined);
  useWorkspaceUiStore.setState({ activeWorkspaceId: WS, activeRepoId: null, historyEpoch: 0 });
});
afterEach(cleanup);

describe("adding a repo", () => {
  it("activates the new repo instead of falling back to the first tab", async () => {
    const old = repo("r-a");
    const added = repo("r-b");
    // The refetch that is already in flight still carries the pre-add list.
    vi.mocked(listRepos).mockResolvedValue([old]);
    mount([added]);

    // No selection yet: the guard legitimately picks the only repo.
    await waitFor(() => expect(useWorkspaceUiStore.getState().activeRepoId).toBe("r-a"));
    vi.mocked(setActiveRepo).mockClear();

    fireEvent.click(screen.getByRole("button", { name: "add" }));

    await waitFor(() => expect(useWorkspaceUiStore.getState().activeRepoId).toBe("r-b"));
    expect(vi.mocked(setActiveRepo)).not.toHaveBeenCalledWith(WS, "r-a");
  });

  it("activates the last repo of a batch add and renders every new tab", async () => {
    const old = repo("r-a");
    const added = [repo("r-b"), repo("r-c")];
    vi.mocked(listRepos).mockResolvedValue([old]);
    mount(added);

    await waitFor(() => expect(useWorkspaceUiStore.getState().activeRepoId).toBe("r-a"));
    vi.mocked(setActiveRepo).mockClear();
    fireEvent.click(screen.getByRole("button", { name: "add" }));

    await waitFor(() => expect(useWorkspaceUiStore.getState().activeRepoId).toBe("r-c"));
    expect(vi.mocked(setActiveRepo)).not.toHaveBeenCalledWith(WS, "r-a");
    expect(screen.getAllByRole("tab").map((tab) => tab.getAttribute("data-value"))).toEqual([
      "r-a",
      "r-b",
      "r-c",
    ]);
  });

  it("still moves off a repo that went missing", async () => {
    // The guard must keep doing its job — seeding is not a blanket skip.
    const gone = repo("r-a", "missing");
    const alive = repo("r-b");
    vi.mocked(listRepos).mockResolvedValue([gone, alive]);
    useWorkspaceUiStore.setState({ activeWorkspaceId: WS, activeRepoId: "r-a" });

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <WorkspaceRepoTabs />
      </QueryClientProvider>,
    );

    await waitFor(() => expect(useWorkspaceUiStore.getState().activeRepoId).toBe("r-b"));
    expect(vi.mocked(setActiveRepo)).toHaveBeenCalledWith(WS, "r-b");
  });
});
