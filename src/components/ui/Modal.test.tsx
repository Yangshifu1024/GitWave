// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Modal } from "./Modal";

afterEach(cleanup);

it("opens a controlled dialog, closes it and opens it again", async () => {
  const onOpenChange = vi.fn();
  const show = (open: boolean) => (
    <Modal open={open} onOpenChange={onOpenChange} title="Settings">
      <input aria-label="Repository path" />
    </Modal>
  );
  const view = render(show(false));
  expect(screen.queryByRole("dialog")).toBeNull();
  view.rerender(show(true));
  expect(await screen.findByRole("dialog", { name: "Settings" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Close" }));
  expect(onOpenChange).toHaveBeenCalledWith(false);
  view.rerender(show(false));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  view.rerender(show(true));
  expect(await screen.findByRole("dialog", { name: "Settings" })).toBeTruthy();
});
