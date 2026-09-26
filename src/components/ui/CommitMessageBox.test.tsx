// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CommitMessageBox } from "./CommitMessageBox";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
afterEach(cleanup);

describe("commit message controls", () => {
  it("accepts a long body and separates editing/amend from submit and AI availability", () => {
    const submit = vi.fn();
    const value = "fix: concise subject\n\n" + "Detailed explanation. ".repeat(50);
    render(
      <CommitMessageBox
        value={value}
        onChange={vi.fn()}
        onSubmit={submit}
        onAmend={vi.fn()}
        onAiGenerate={vi.fn()}
        submitDisabled
        aiDisabled
      />,
    );
    const textarea = screen.getByRole<HTMLTextAreaElement>("textbox");
    expect(textarea.value).toBe(value);
    expect(textarea.hasAttribute("maxlength")).toBe(false);
    expect(textarea.disabled).toBe(false);
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "changes.amend.button" }).disabled,
    ).toBe(false);
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "changes.messageBox.commit" }).disabled,
    ).toBe(true);
    fireEvent.keyDown(textarea, { key: "Enter", ctrlKey: true });
    expect(submit).not.toHaveBeenCalled();
  });
});
