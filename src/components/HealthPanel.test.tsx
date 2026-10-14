// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import { HealthPanel } from "./HealthPanel";
import { explainHealth, getHealth, type HealthReport } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

vi.mock("@/lib/api", () => ({
  getHealth: vi.fn(),
  explainHealth: vi.fn(),
  formatAppError: String,
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: { count?: number }) =>
      options?.count === undefined ? key : `${key} ${options.count}`,
  }),
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, disabled, onClick }: ComponentProps<"button">) => (
    <button disabled={disabled} onClick={onClick}>
      {children}
    </button>
  ),
}));
vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? <div role="dialog">{children}</div> : null,
}));

const report: HealthReport = {
  unpushed: null,
  conflict_residue: [],
  dirty_files: 22,
  stale_branches: [],
  large_files: [],
  branch_count: 5,
  tag_count: 34,
};
let client: QueryClient;
beforeEach(() => {
  vi.clearAllMocks();
  client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  useWorkspaceUiStore.setState({ activeWorkspaceId: "ws", activeRepoId: "repo" });
});
afterEach(() => {
  cleanup();
  client.clear();
});
const mount = () =>
  render(
    <QueryClientProvider client={client}>
      <HealthPanel />
    </QueryClientProvider>,
  );

it("defaults to a compact summary and keeps all metrics and AI summary accessible when expanded", async () => {
  vi.mocked(getHealth).mockResolvedValue(report);
  vi.mocked(explainHealth).mockResolvedValue("local metrics explanation");
  const view = mount();
  await screen.findByText("health.summary.changes 22");
  const disclosure = view.container.querySelector("details")!;
  expect(disclosure.open).toBe(false);
  fireEvent.click(view.container.querySelector("summary")!);
  expect(disclosure.open).toBe(true);
  expect(screen.getByText("health.values.noUpstream")).toBeTruthy();
  expect(screen.getByText("5 / 34")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "health.aiSummary" }));
  await screen.findByText("local metrics explanation");
  expect(explainHealth).toHaveBeenCalledExactlyOnceWith("ws");
  fireEvent.click(view.container.querySelector("summary")!);
  expect(disclosure.open).toBe(false);
});

it("shows operation residue ahead of working-copy changes without requiring expansion", async () => {
  vi.mocked(getHealth).mockResolvedValue({ ...report, conflict_residue: ["MERGE_HEAD"] });
  const view = mount();
  const summary = await screen.findByText("health.summary.residue");
  expect(summary.closest("summary")).toBeTruthy();
  expect(summary.getAttribute("title")).toBe("MERGE_HEAD");
  expect(view.container.querySelector("details")!.open).toBe(false);
  expect(screen.queryByText("health.summary.changes 22")).toBeNull();
});

it("renders a clean summary for a clean repository", async () => {
  vi.mocked(getHealth).mockResolvedValue({ ...report, dirty_files: 0 });
  mount();
  await screen.findByText("health.summary.clean");
});

it("does not query or show repository health with no active repository", () => {
  useWorkspaceUiStore.setState({ activeRepoId: null });
  const view = mount();
  expect(view.container.textContent).toBe("");
  expect(getHealth).not.toHaveBeenCalled();
});
