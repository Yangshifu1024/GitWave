// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  executeInteractiveRebase,
  planInteractiveRebase,
  type InteractiveRebaseTodo,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { InteractiveRebaseDialog } from "./InteractiveRebaseDialog";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/lib/api", () => ({
  planInteractiveRebase: vi.fn(),
  executeInteractiveRebase: vi.fn(),
  formatAppError: String,
  getWorkingCopy: vi.fn(() => Promise.resolve({ files: [] })),
}));
afterEach(cleanup);

it("reorders todo rows with Alt+Arrow and submits the order only when started", async () => {
  const todos: InteractiveRebaseTodo[] = ["First", "Second", "Third"].map((summary, i) => ({
    oid: String(i + 1).repeat(40),
    summary,
    action: i === 1 ? "reword" : "pick",
    message: i === 1 ? "edited" : null,
  }));
  vi.mocked(planInteractiveRebase).mockResolvedValue(todos);
  vi.mocked(executeInteractiveRebase).mockResolvedValue({
    kind: "clean",
    conflicts: [],
    new_head: todos[1]!.oid,
  });
  useWorkspaceUiStore.setState({ activeWorkspaceId: "ws", activeRepoId: "repo" });
  render(
    <QueryClientProvider client={new QueryClient()}>
      <InteractiveRebaseDialog
        open
        workspaceId="ws"
        upstream="upstream"
        onClose={vi.fn()}
        onDone={vi.fn()}
      />
    </QueryClientProvider>,
  );
  await screen.findByText("First");
  const rows = () => screen.getAllByRole("listitem");
  const [first, second, third] = rows() as [HTMLElement, HTMLElement, HTMLElement];
  expect(second.tabIndex).toBe(0);
  fireEvent.keyDown(first, { key: "ArrowUp", altKey: true });
  fireEvent.keyDown(third, { key: "ArrowDown", altKey: true });
  fireEvent.keyDown(second, { key: "ArrowDown" });
  fireEvent.keyDown(within(second).getByRole("textbox"), { key: "ArrowUp", altKey: true });
  expect(rows()).toEqual([first, second, third]);
  second.focus();
  fireEvent.keyDown(second, { key: "ArrowUp", altKey: true });
  expect(rows()).toEqual([second, first, third]);
  expect(document.activeElement).toBe(second);
  fireEvent.keyDown(second, { key: "ArrowDown", altKey: true });
  fireEvent.keyDown(second, { key: "ArrowDown", altKey: true });
  expect(rows()).toEqual([first, third, second]);
  expect(executeInteractiveRebase).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "branches.irebase.start" }));
  await waitFor(() =>
    expect(executeInteractiveRebase).toHaveBeenCalledExactlyOnceWith("ws", "upstream", [
      todos[0],
      todos[2],
      todos[1],
    ]),
  );
});
