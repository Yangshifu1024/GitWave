// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ComponentProps, ReactNode } from "react";
import { ConflictPanel } from "./ConflictPanel";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { getConflictSides, explainConflict, resolveConflict } from "@/lib/api";
import type { ConflictSides } from "@/lib/api";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/lib/api", () => ({
  getConflictSides: vi.fn(),
  explainConflict: vi.fn(),
  resolveConflict: vi.fn(),
  formatAppError: (error: unknown) => String(error),
}));
vi.mock("@/components/ui/Button", () => ({
  Button: (props: ComponentProps<"button">) => (
    <button onClick={props.onClick} disabled={props.disabled} aria-label={props["aria-label"]}>
      {props.children}
    </button>
  ),
}));
vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children }: { open: boolean; children: ReactNode }) =>
    open ? <div role="alertdialog">{children}</div> : null,
}));

const sides = (path: string): ConflictSides => ({
  path,
  ours: `ours ${path}`,
  theirs: `theirs ${path}`,
  base: null,
  working: `working ${path}`,
});
const merge = {
  active: true,
  mergeInProgress: false,
  files: ["a.ts", "b.ts"].map((path) => ({
    path,
    has_ours: true,
    has_theirs: true,
    has_base: false,
  })),
  refresh: vi.fn().mockResolvedValue(undefined),
  abort: vi.fn().mockResolvedValue(undefined),
};
const editor = () => screen.getByRole<HTMLTextAreaElement>("textbox");
async function openFile(path: string) {
  await flushAct(() => {
    fireEvent.click(screen.getByRole("option", { name: path }));
  });
}
beforeEach(() => {
  vi.clearAllMocks();
  useWorkspaceUiStore.setState({ activeWorkspaceId: "ws", activeRepoId: "repo" });
  vi.mocked(getConflictSides).mockImplementation((_, path) => Promise.resolve(sides(path)));
});
afterEach(cleanup);

describe("conflict editor input and response ownership", () => {
  it("preserves A's edited buffer through A → B → A and confirms discarding any dirty file", async () => {
    const close = vi.fn();
    render(<ConflictPanel open merge={merge} onClose={close} />);
    await openFile("a.ts");
    fireEvent.change(editor(), { target: { value: "edited A" } });
    await openFile("b.ts");
    fireEvent.click(screen.getByRole("button", { name: "conflicts.close" }));
    expect(screen.getByText("conflicts.discard.confirm")).toBeTruthy();
    expect(close).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText("conflicts.discard.keepEditing"));
    await openFile("a.ts");
    expect(editor().value).toBe("edited A");
  });

  it("keeps Use Ours dirty and retains the buffer when save fails", async () => {
    vi.mocked(resolveConflict).mockRejectedValue(new Error("save failed"));
    const close = vi.fn();
    render(<ConflictPanel open merge={merge} onClose={close} />);
    await openFile("a.ts");
    fireEvent.click(screen.getByText("conflicts.useOurs"));
    await flushAct(() => {
      fireEvent.click(screen.getByText("conflicts.markResolved"));
    });
    expect(editor().value).toBe("ours a.ts");
    fireEvent.click(screen.getByRole("button", { name: "conflicts.close" }));
    expect(screen.getByText("conflicts.discard.confirm")).toBeTruthy();
    expect(close).not.toHaveBeenCalled();
  });

  it("clears the old editor during loading and ignores out-of-order file responses", async () => {
    let finishA!: (value: ConflictSides) => void;
    vi.mocked(getConflictSides).mockImplementation(async (_, path) =>
      path === "a.ts"
        ? new Promise((resolve) => {
            finishA = resolve;
          })
        : sides(path),
    );
    render(<ConflictPanel open merge={merge} onClose={vi.fn()} />);
    await openFile("a.ts");
    expect(screen.queryByRole("textbox")).toBeNull();
    await openFile("b.ts");
    await flushAct(() => finishA(sides("a.ts")));
    expect(editor().value).toBe("working b.ts");
  });

  it("ignores an explanation that arrives after the repo changes", async () => {
    let finish!: (value: Awaited<ReturnType<typeof explainConflict>>) => void;
    vi.mocked(explainConflict).mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    render(<ConflictPanel open merge={merge} onClose={vi.fn()} />);
    await openFile("a.ts");
    fireEvent.click(screen.getByText("conflicts.explain"));
    act(() => useWorkspaceUiStore.setState({ activeRepoId: "other" }));
    await flushAct(() =>
      finish({ text: "old repo explanation", used_fallback: false, provider_used: "test" }),
    );
    expect(screen.queryByText("old repo explanation")).toBeNull();
  });
});

async function flushAct(action: () => void) {
  await act(async () => {
    action();
    await Promise.resolve();
  });
}
