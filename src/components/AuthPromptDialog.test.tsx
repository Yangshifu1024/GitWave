// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ComponentProps, ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAuthPromptStore } from "@/stores/authPromptStore";
import { AuthPromptDialog } from "./AuthPromptDialog";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, disabled, onClick }: ComponentProps<"button">) => (
    <button disabled={disabled} onClick={onClick}>
      {children}
    </button>
  ),
}));
vi.mock("@/components/ui/Input", () => ({
  Input: ({
    value,
    onChange,
    placeholder,
    type,
  }: {
    value: string;
    onChange: (value: string) => void;
    placeholder: string;
    type?: string;
  }) => (
    <input
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      type={type}
    />
  ),
}));
vi.mock("@/components/ui/Checkbox", () => ({
  Checkbox: ({
    checked,
    onChange,
    children,
  }: {
    checked: boolean;
    onChange: (checked: boolean) => void;
    children: ReactNode;
  }) => (
    <label>
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      {children}
    </label>
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

const username = () => screen.getByPlaceholderText<HTMLInputElement>("common.authPrompt.username");
const password = () => screen.getByPlaceholderText<HTMLInputElement>("common.authPrompt.token");

beforeEach(() => useAuthPromptStore.getState().close());
afterEach(cleanup);

describe("authentication prompt session", () => {
  it("clears credentials and restores remember when the same remote reopens after cancellation", () => {
    const retry = vi.fn();
    const dismiss = vi.fn();
    render(<AuthPromptDialog />);
    act(() => useAuthPromptStore.getState().show("origin", retry, dismiss));
    fireEvent.change(username(), { target: { value: "alice" } });
    fireEvent.change(password(), { target: { value: "secret" } });
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "common.cancel" }));
    expect(dismiss).toHaveBeenCalledOnce();
    expect(screen.queryByRole("dialog")).toBeNull();
    act(() => useAuthPromptStore.getState().show("origin", retry));
    expect(username().value).toBe("");
    expect(password().value).toBe("");
    expect(screen.getByRole<HTMLInputElement>("checkbox").checked).toBe(true);
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "common.authPrompt.submit" }).disabled,
    ).toBe(true);
    expect(retry).not.toHaveBeenCalled();
  });

  it("submits once and starts with empty credentials on a later attempt", () => {
    const retry = vi.fn();
    render(<AuthPromptDialog />);
    act(() => useAuthPromptStore.getState().show("origin", retry));
    fireEvent.change(username(), { target: { value: " alice " } });
    fireEvent.change(password(), { target: { value: "secret" } });
    fireEvent.click(screen.getByRole("button", { name: "common.authPrompt.submit" }));
    expect(retry).toHaveBeenCalledExactlyOnceWith({
      username: "alice",
      password: "secret",
      remember: true,
    });
    act(() => useAuthPromptStore.getState().show("origin", retry));
    expect(username().value).toBe("");
    expect(password().value).toBe("");
  });
});
