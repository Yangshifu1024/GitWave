// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ComponentProps, ReactNode } from "react";
import { ChangesPanel } from "./ChangesPanel";
import type { CommitMessageBoxProps } from "./ui/CommitMessageBox";
import { commitDraftKey, useCommitDraftStore } from "@/stores/commitDraftStore";
import type { FileChange } from "@/lib/api";
import { generateCommitMessage } from "@/lib/api";

const fixtures = vi.hoisted(() => ({
  wc: {
    workspaceId: "ws",
    repoId: "a",
    data: { head_message: "original HEAD", sha: "head", branch: "main", upstream: null },
    isLoading: false,
    isError: false,
    error: null,
    actionError: null,
    setActionError: vi.fn(),
    unstagedFiles: [] as FileChange[],
    stagedFiles: [] as FileChange[],
    stage: vi.fn(),
    unstage: vi.fn(),
    discard: vi.fn(),
    ignore: vi.fn(),
    commitMessage:
      vi.fn<(message: string, options: { onSuccess?: () => void; amend?: boolean }) => void>(),
    commitPending: false,
  },
}));
vi.mock("@/hooks/useWorkingCopy", () => ({ useWorkingCopy: () => fixtures.wc }));
vi.mock("@tanstack/react-query", () => ({
  useQuery: () => ({ data: { settings: { ai_provider: "test" } } }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/lib/api", () => ({
  generateCommitMessage: vi.fn(),
  getWorkspace: vi.fn(),
  formatAppError: String,
}));
vi.mock("@heroui/react", () => ({
  Radio: () => null,
  RadioGroup: () => null,
  Surface: (props: ComponentProps<"div">) => <div {...props} />,
}));
vi.mock("@/components/ui/StatusIcon", () => ({ StatusIcon: () => null }));
vi.mock("@/components/AiProviderSettings", () => ({ AiProviderSettings: () => null }));
vi.mock("@/components/GitignoreEditor", () => ({ GitignoreEditor: () => null }));
vi.mock("@/components/ui/CommitMessageBox", () => ({
  CommitMessageBox: (props: CommitMessageBoxProps) => (
    <div>
      <textarea
        aria-label="draft"
        value={props.value}
        onChange={(e) => props.onChange(e.target.value)}
      />
      <button disabled={props.disabled || props.submitDisabled} onClick={props.onSubmit}>
        submit
      </button>
      <button disabled={props.disabled} onClick={props.onAmend}>
        amend
      </button>
      <button onClick={props.onAiGenerate}>generate</button>
      <button onClick={props.onRestoreDraft}>restore</button>
    </div>
  ),
}));
vi.mock("@/components/ui/Button", () => ({
  Button: (props: ComponentProps<"button">) => (
    <button disabled={props.disabled} onClick={props.onClick}>
      {props.children}
    </button>
  ),
}));
vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children, footer }: { open: boolean; children: ReactNode; footer: ReactNode }) =>
    open ? (
      <div>
        {children}
        {footer}
      </div>
    ) : null,
}));

const props = { selectedPath: null, selectedStaged: null, onSelectFile: vi.fn() };
const editor = () => screen.getByLabelText<HTMLTextAreaElement>("draft");
beforeEach(() => {
  vi.clearAllMocks();
  fixtures.wc.repoId = "a";
  fixtures.wc.stagedFiles = [];
  useCommitDraftStore.setState({ drafts: {} });
});
afterEach(cleanup);

describe("commit draft workflow", () => {
  it("restores separate drafts after modal remount and A → B → A", () => {
    let view = render(<ChangesPanel {...props} />);
    fireEvent.change(editor(), { target: { value: "draft A" } });
    view.unmount();
    view = render(<ChangesPanel {...props} />);
    expect(editor().value).toBe("draft A");
    fixtures.wc.repoId = "b";
    view.rerender(<ChangesPanel {...props} />);
    expect(editor().value).toBe("");
    fireEvent.change(editor(), { target: { value: "draft B" } });
    fixtures.wc.repoId = "a";
    view.rerender(<ChangesPanel {...props} />);
    expect(editor().value).toBe("draft A");
  });

  it("allows message-only amend and clears only the successful repository draft", () => {
    useCommitDraftStore.getState().update(commitDraftKey("ws", "b"), { message: "draft B" });
    render(<ChangesPanel {...props} />);
    expect(screen.getByText<HTMLButtonElement>("submit").disabled).toBe(true);
    fireEvent.click(screen.getByText("amend"));
    fireEvent.click(screen.getByText("changes.amend.confirm"));
    expect(editor().value).toBe("original HEAD");
    fireEvent.change(editor(), { target: { value: "corrected HEAD" } });
    fireEvent.click(screen.getByText("submit"));
    expect(fixtures.wc.commitMessage).toHaveBeenCalledWith(
      "corrected HEAD",
      expect.objectContaining({ amend: true }),
    );
    expect(editor().value).toBe("corrected HEAD");
    act(() => {
      fixtures.wc.commitMessage.mock.calls[0]?.[1].onSuccess?.();
    });
    expect(editor().value).toBe("");
    expect(useCommitDraftStore.getState().drafts[commitDraftKey("ws", "b")]?.message).toBe(
      "draft B",
    );
  });

  it("restores the draft from before AI generation", async () => {
    vi.mocked(generateCommitMessage).mockResolvedValue({
      text: "AI draft",
      used_fallback: false,
      provider_used: "test",
    });
    render(<ChangesPanel {...props} />);
    fireEvent.change(editor(), { target: { value: "my draft" } });
    await flushAct(() => {
      fireEvent.click(screen.getByText("generate"));
    });
    expect(editor().value).toBe("AI draft");
    fireEvent.click(screen.getByText("restore"));
    expect(editor().value).toBe("my draft");
  });

  it("rejects late AI output even after switching A → B → A", async () => {
    let finish!: (value: Awaited<ReturnType<typeof generateCommitMessage>>) => void;
    vi.mocked(generateCommitMessage).mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const view = render(<ChangesPanel {...props} />);
    fireEvent.change(editor(), { target: { value: "my draft" } });
    fireEvent.click(screen.getByText("generate"));
    fixtures.wc.repoId = "b";
    view.rerender(<ChangesPanel {...props} />);
    fixtures.wc.repoId = "a";
    view.rerender(<ChangesPanel {...props} />);
    await flushAct(() => finish({ text: "stale AI", used_fallback: false, provider_used: "test" }));
    expect(editor().value).toBe("my draft");
  });
});

it("supports a single keyboard entry point, Shift-arrow range selection and Space toggle", () => {
  fixtures.wc.stagedFiles = ["a.ts", "b.ts", "c.ts"].map((path) => ({
    path,
    kind: "modified",
    staged: true,
    additions: 1,
    deletions: 0,
  }));
  render(<ChangesPanel {...props} />);
  const rows = screen.getAllByRole("option");
  expect(rows.map((row) => row.tabIndex)).toEqual([0, -1, -1]);
  fireEvent.keyDown(rows[0]!, { key: "Enter" });
  fireEvent.keyDown(rows[0]!, { key: "ArrowDown", shiftKey: true });
  expect(document.activeElement).toBe(rows[1]);
  expect(rows.map((row) => row.getAttribute("aria-selected"))).toEqual(["true", "true", "false"]);
  fireEvent.keyDown(rows[1]!, { key: " " });
  expect(rows[1]?.getAttribute("aria-selected")).toBe("false");
  expect(rows.map((row) => row.tabIndex)).toEqual([-1, 0, -1]);
});

async function flushAct(action: () => void) {
  await act(async () => {
    action();
    await Promise.resolve();
  });
}
