// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Pane, ResizeHandle, Split } from "./Split";

beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe = vi.fn();
      disconnect = vi.fn();
    },
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("Split direction", () => {
  it("renders a vertical split's horizontal separator correctly on its first mount", () => {
    render(
      <Split direction="vertical">
        <Pane initialSize={200}>top</Pane>
        <ResizeHandle />
        <Pane initialSize={200}>bottom</Pane>
      </Split>,
    );
    const handle = screen.getByRole("separator");
    expect(handle.getAttribute("aria-orientation")).toBe("horizontal");
    expect(handle.style.cursor).toBe("row-resize");
    expect(handle.style.height).toBe("2px");
    expect(handle.style.width).toBe("");
  });

  it("uses the nearest split direction for nested separators", () => {
    render(
      <Split direction="vertical">
        <Pane initialSize={200}>
          <Split>
            <Pane initialSize={100}>left</Pane>
            <ResizeHandle />
            <Pane initialSize={100}>right</Pane>
          </Split>
        </Pane>
        <ResizeHandle />
        <Pane initialSize={200}>bottom</Pane>
      </Split>,
    );
    const [inner, outer] = screen.getAllByRole("separator");
    expect(inner?.getAttribute("aria-orientation")).toBe("vertical");
    expect(inner?.style.cursor).toBe("col-resize");
    expect(outer?.getAttribute("aria-orientation")).toBe("horizontal");
    expect(outer?.style.cursor).toBe("row-resize");
  });
});
