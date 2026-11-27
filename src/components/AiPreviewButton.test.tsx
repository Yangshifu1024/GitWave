// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ComponentProps } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { discardAiPreview, previewAi } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { AiPreviewButton } from "./AiPreviewButton";
import type { ModalProps } from "./ui/Modal";

vi.mock("@/lib/api", () => ({
  previewAi: vi.fn(),
  discardAiPreview: vi.fn(),
  formatAppError: String,
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/components/ui/Button", () => ({
  Button: ({
    children,
    disabled,
    onClick,
    title,
    "aria-label": label,
  }: ComponentProps<"button">) => (
    <button disabled={disabled} onClick={onClick} title={title} aria-label={label}>
      {children}
    </button>
  ),
}));
vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, onOpenChange, children, footer }: ModalProps) =>
    open ? (
      <div role="dialog">
        <button onClick={() => onOpenChange(false)}>Dismiss</button>
        {children}
        {footer}
      </div>
    ) : null,
}));

const snapshot = {
  id: 17,
  prompt: {
    system: "Ask [EMAIL].",
    user: "Call [PHONE].",
    counts: { secret_lines: 0, emails: 1, phones: 1 },
  },
};
const task = { kind: "health" } as const;
const show = (onSend: (id: number) => void, workspaceId = "ws") => (
  <AiPreviewButton workspaceId={workspaceId} task={task} onSend={onSend} />
);

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(discardAiPreview).mockResolvedValue(undefined);
  useWorkspaceUiStore.setState({ activeWorkspaceId: "ws", activeRepoId: "repo" });
});
afterEach(cleanup);

it("displays the prepared messages locally and discards them on dismissal", async () => {
  const send = vi.fn();
  vi.mocked(previewAi).mockResolvedValue(snapshot);
  render(show(send));
  expect(previewAi).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "ai.preview.open" }));
  await screen.findByText(snapshot.prompt.system);
  expect(screen.getByText(snapshot.prompt.user)).toBeTruthy();
  expect(send).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
  await waitFor(() => expect(discardAiPreview).toHaveBeenCalledExactlyOnceWith(snapshot.id));
  expect(send).not.toHaveBeenCalled();
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("sends the displayed snapshot once on explicit confirmation", async () => {
  const send = vi.fn();
  vi.mocked(previewAi).mockResolvedValue(snapshot);
  render(show(send));
  fireEvent.click(screen.getByRole("button", { name: "ai.preview.open" }));
  await screen.findByText(snapshot.prompt.user);
  fireEvent.click(screen.getByRole("button", { name: "ai.preview.send" }));
  expect(send).toHaveBeenCalledExactlyOnceWith(snapshot.id);
  expect(previewAi).toHaveBeenCalledTimes(1);
  expect(discardAiPreview).not.toHaveBeenCalled();
});

it("discards a delayed preview after a workspace change without presenting or sending it", async () => {
  const send = vi.fn();
  let release!: (value: typeof snapshot) => void;
  const pending = new Promise<typeof snapshot>((resolve) => {
    release = resolve;
  });
  vi.mocked(previewAi).mockReturnValue(pending);
  const view = render(show(send));
  fireEvent.click(screen.getByRole("button", { name: "ai.preview.open" }));
  await waitFor(() => expect(previewAi).toHaveBeenCalledTimes(1));
  view.rerender(show(send, "other-workspace"));
  await act(async () => {
    release(snapshot);
    await pending;
  });
  await waitFor(() => expect(discardAiPreview).toHaveBeenCalledExactlyOnceWith(snapshot.id));
  expect(screen.queryByText(snapshot.prompt.user)).toBeNull();
  expect(send).not.toHaveBeenCalled();
});
