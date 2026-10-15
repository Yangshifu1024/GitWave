// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { WorkbenchSplit } from "./WorkbenchSplit";
afterEach(() => {
  cleanup();
  localStorage.clear();
});
it("supports directional keyboard resizing, bounds and reset without losing pane state", () => {
  render(
    <WorkbenchSplit
      id="test"
      label="History size"
      direction="vertical"
      initial={42}
      min={22}
      max={72}
      first={<input defaultValue="draft" />}
      second={<p>diff</p>}
    />,
  );
  const separator = screen.getByRole("separator");
  fireEvent.keyDown(separator, { key: "ArrowDown", shiftKey: true });
  expect(separator.getAttribute("aria-valuenow")).toBe("52");
  fireEvent.keyDown(separator, { key: "End" });
  fireEvent.keyDown(separator, { key: "ArrowDown" });
  expect(separator.getAttribute("aria-valuenow")).toBe("72");
  fireEvent.keyDown(separator, { key: "Home" });
  expect(separator.getAttribute("aria-valuenow")).toBe("42");
  expect(localStorage.getItem("gitwave.split.test")).toBe("42");
  expect(screen.getByRole<HTMLInputElement>("textbox").value).toBe("draft");
});
