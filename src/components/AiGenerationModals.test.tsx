// @vitest-environment jsdom
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ComponentProps, ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { explainCommit, generatePrDescription } from "@/lib/api";
import type { InputProps } from "@/components/ui/Input";
import type { TextareaProps } from "@/components/ui/Textarea";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { CommitExplainModal } from "./CommitExplainModal";
import { PrDescriptionModal } from "./PrDescriptionModal";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/lib/api", () => ({
  explainCommit: vi.fn(),
  generatePrDescription: vi.fn(),
  formatAppError: String,
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, disabled, onClick }: ComponentProps<"button">) => (
    <button disabled={disabled} onClick={onClick}>
      {children}
    </button>
  ),
}));
vi.mock("@/components/ui/Input", () => ({
  Input: ({ value, onChange, placeholder, disabled }: InputProps) => (
    <input
      value={value}
      onChange={(e) => onChange?.(e.target.value)}
      placeholder={placeholder}
      disabled={disabled}
    />
  ),
}));
vi.mock("@/components/ui/Textarea", () => ({
  Textarea: ({ value, onChange, placeholder, disabled }: TextareaProps) => (
    <textarea
      value={value}
      onChange={(e) => onChange?.(e.target.value)}
      placeholder={placeholder}
      disabled={disabled}
    />
  ),
}));
vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children, footer }: { open: boolean; children: ReactNode; footer: ReactNode }) =>
    open ? (
      <div role="dialog">
        {children}
        {footer}
      </div>
    ) : null,
}));

const outcome = (text: string, used_fallback = false) => ({
  text,
  title: text,
  body: `${text} body`,
  provider_used: "test-provider",
  used_fallback,
});

function deferred() {
  let resolve!: (value: ReturnType<typeof outcome>) => void;
  const promise = new Promise<ReturnType<typeof outcome>>((yes) => {
    resolve = yes;
  });
  return { promise, resolve };
}

beforeEach(() => {
  vi.resetAllMocks();
  useWorkspaceUiStore.setState({ activeWorkspaceId: "ws", activeRepoId: "a" });
  useStatusAreaStore.getState().clearStatus();
});
afterEach(() => {
  cleanup();
  useStatusAreaStore.getState().clearStatus();
});

describe.each(["explanation", "pr"] as const)("%s generation session", (kind) => {
  const show = (open: boolean) =>
    kind === "explanation" ? (
      <CommitExplainModal workspaceId="ws" sha="commit" open={open} onClose={() => undefined} />
    ) : (
      <PrDescriptionModal workspaceId="ws" open={open} onClose={() => undefined} />
    );
  const contents = () =>
    kind === "explanation"
      ? (screen.queryByText("fresh")?.textContent ?? "")
      : screen.getByPlaceholderText<HTMLInputElement>("commits.pr.titlePlaceholder").value;
  const request = () =>
    kind === "explanation" ? vi.mocked(explainCommit) : vi.mocked(generatePrDescription);
  const mount = () => {
    const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
    return render(show(true), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    });
  };

  it.each(["reopen", "repository switch"])("ignores stale results after %s", async (transition) => {
    const old = deferred();
    const fresh = deferred();
    request().mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise);
    const view = mount();
    await waitFor(() => expect(request()).toHaveBeenCalledTimes(1));
    if (transition === "reopen") {
      view.rerender(show(false));
      expect(screen.queryByRole("dialog")).toBeNull();
      view.rerender(show(true));
    } else {
      act(() => useWorkspaceUiStore.setState({ activeRepoId: "b" }));
    }
    await waitFor(() => expect(request()).toHaveBeenCalledTimes(2));
    expect(contents()).toBe("");
    await act(async () => {
      fresh.resolve(outcome("fresh"));
      await fresh.promise;
    });
    await waitFor(() => expect(contents()).toBe("fresh"));
    await act(async () => {
      old.resolve(outcome("stale", true));
      await old.promise;
    });
    expect(contents()).toBe("fresh");
    expect(screen.queryByText("stale")).toBeNull();
    expect(useStatusAreaStore.getState().status).toBeNull();
  });

  it("clears completed output while a reopened generation is pending", async () => {
    const fresh = deferred();
    request().mockResolvedValueOnce(outcome("fresh")).mockReturnValueOnce(fresh.promise);
    const view = mount();
    await waitFor(() => expect(contents()).toBe("fresh"));
    view.rerender(show(false));
    view.rerender(show(true));
    await waitFor(() => expect(request()).toHaveBeenCalledTimes(2));
    expect(contents()).toBe("");
    await act(async () => {
      fresh.resolve(outcome("fresh"));
      await fresh.promise;
    });
    await waitFor(() => expect(contents()).toBe("fresh"));
  });

  it("does not publish fallback status if closed before the mutation starts", async () => {
    const old = deferred();
    request().mockReturnValueOnce(old.promise);
    const view = mount();
    view.rerender(show(false));
    await waitFor(() => expect(request()).toHaveBeenCalledTimes(1));
    await act(async () => {
      old.resolve(outcome("stale", true));
      await old.promise;
    });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(useStatusAreaStore.getState().status).toBeNull();
  });
});
