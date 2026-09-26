// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { PropsWithChildren } from "react";
import { getHook, listHooks } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { HooksPanel } from "./HooksPanel";

vi.mock("@/lib/api", () => ({
  getHook: vi.fn(),
  listHooks: vi.fn(),
  saveHook: vi.fn(),
  formatAppError: (e: unknown) => String(e),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, params?: { path?: string }) => params?.path ?? key }),
}));
vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ children }: PropsWithChildren) => <div>{children}</div>,
}));
vi.mock("@/components/ui/Textarea", () => ({
  Textarea: ({ value }: { value: string }) => <textarea readOnly value={value} />,
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children }: PropsWithChildren) => <button>{children}</button>,
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

it("shows the actual configured hook path and refreshes it on a repository switch", async () => {
  useWorkspaceUiStore.setState({ activeWorkspaceId: "ws", activeRepoId: "a" });
  vi.mocked(listHooks)
    .mockResolvedValueOnce([
      {
        name: "pre-commit",
        actual_path: "/shared/hooks/pre-commit",
        exists: true,
        executable: true,
      },
    ])
    .mockResolvedValueOnce([
      {
        name: "pre-commit",
        actual_path: "/linked/custom/pre-commit",
        exists: true,
        executable: true,
      },
    ]);
  vi.mocked(getHook).mockResolvedValue("echo hook");
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <HooksPanel workspaceId="ws" open onClose={() => {}} />
    </QueryClientProvider>,
  );
  fireEvent.click(await screen.findByText("pre-commit"));
  expect(screen.getByText("/shared/hooks/pre-commit")).toBeTruthy();
  act(() => {
    useWorkspaceUiStore.setState({ activeRepoId: "b" });
  });
  await waitFor(() => {
    expect(listHooks).toHaveBeenCalledTimes(2);
  });
  fireEvent.click(await screen.findByText("pre-commit"));
  expect(screen.getByText("/linked/custom/pre-commit")).toBeTruthy();
  expect(screen.queryByText("/shared/hooks/pre-commit")).toBeNull();
  client.clear();
});
