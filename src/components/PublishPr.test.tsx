// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { invoke } from "@tauri-apps/api/core";
import { openUrl } from "@tauri-apps/plugin-opener";
import { generatePrDescription } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { PrDescriptionModal } from "./PrDescriptionModal";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@tauri-apps/api/core", () => ({ invoke: vi.fn() }));
vi.mock("@tauri-apps/plugin-opener", () => ({ openUrl: vi.fn() }));
vi.mock("@/lib/api", () => ({ generatePrDescription: vi.fn(), formatAppError: String }));
const target = { repository: "sample/project", head: "topic", oid: "a".repeat(40), base: "main" };
const url = "https://github.com/sample/project/pull/42";
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const show = (open: boolean) => (
    <QueryClientProvider client={client}>
      <PrDescriptionModal open={open} workspaceId="ws" onClose={vi.fn()} />
    </QueryClientProvider>
  );
  const view = render(show(true));
  return () => view.rerender(show(false));
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(generatePrDescription).mockResolvedValue({
    title: "Generated title",
    body: "Generated body",
    provider_used: "local",
    used_fallback: false,
  });
  useWorkspaceUiStore.setState({ activeWorkspaceId: "ws", activeRepoId: "repo" });
});
afterEach(cleanup);

it("publishes only after review, preserves edited multiline content on failure, and opens the confirmed PR on request", async () => {
  vi.mocked(invoke)
    .mockResolvedValueOnce(target)
    .mockRejectedValueOnce(new Error("request rejected"))
    .mockResolvedValueOnce(url);
  mount();
  const title = await screen.findByDisplayValue("Generated title");
  const body = screen.getByDisplayValue("Generated body");
  fireEvent.change(title, { target: { value: "Reviewed PR title" } });
  fireEvent.change(body, { target: { value: "First paragraph\n\n- Detail" } });
  expect(invoke).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "commits.pr.review" }));
  const base = await screen.findByDisplayValue("main");
  expect(screen.getByText(/sample\/project.*topic/)).toBeTruthy();
  expect(invoke).toHaveBeenCalledExactlyOnceWith("cmd_github_pr_target", {
    workspaceId: "ws",
    repoId: "repo",
  });
  fireEvent.change(base, { target: { value: "topic" } });
  const publish = screen.getByRole<HTMLButtonElement>("button", { name: "commits.pr.publish" });
  expect(publish.disabled).toBe(true);
  fireEvent.change(base, { target: { value: "release" } });
  fireEvent.click(publish);
  await screen.findByRole("alert");
  expect(invoke).toHaveBeenLastCalledWith("cmd_create_github_pr", {
    workspaceId: "ws",
    repoId: "repo",
    draft: {
      target,
      base: "release",
      title: "Reviewed PR title",
      body: "First paragraph\n\n- Detail",
    },
  });
  expect((title as HTMLInputElement).value).toBe("Reviewed PR title");
  expect((body as HTMLTextAreaElement).value).toBe("First paragraph\n\n- Detail");
  fireEvent.change(body, { target: { value: "Final body" } });
  fireEvent.click(publish);
  const result = await screen.findByRole("button", { name: url });
  expect(invoke).toHaveBeenLastCalledWith("cmd_create_github_pr", {
    workspaceId: "ws",
    repoId: "repo",
    draft: { target, base: "release", title: "Reviewed PR title", body: "Final body" },
  });
  expect(openUrl).not.toHaveBeenCalled();
  vi.mocked(openUrl).mockRejectedValueOnce(new Error("Browser unavailable"));
  fireEvent.click(result);
  expect(openUrl).toHaveBeenCalledExactlyOnceWith(url);
  await screen.findByRole("alert");
  expect(screen.getByText(/Browser unavailable/)).toBeTruthy();
  expect(screen.getByRole("button", { name: url })).toBeTruthy();
});

it("dismissing the reviewed draft never requests PR creation", async () => {
  vi.mocked(invoke).mockResolvedValueOnce(target);
  const close = mount();
  await screen.findByDisplayValue("Generated title");
  fireEvent.click(screen.getByRole("button", { name: "commits.pr.review" }));
  await screen.findByDisplayValue("main");
  close();
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(invoke).toHaveBeenCalledExactlyOnceWith("cmd_github_pr_target", {
    workspaceId: "ws",
    repoId: "repo",
  });
});
