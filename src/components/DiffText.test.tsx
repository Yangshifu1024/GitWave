// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { DiffText } from "./DiffText";
import type { FileDiff } from "@/lib/api";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it.each(["unified", "split"] as const)(
  "virtualizes a resizable full-height %s viewport",
  (mode) => {
    let height = 820;
    let resized = () => {};
    vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockImplementation(() => height);
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(notify: () => void) {
          resized = notify;
        }
        observe() {}
        disconnect() {}
      },
    );
    const file: FileDiff = {
      path: "large.txt",
      old_sha: null,
      new_sha: null,
      additions: 3000,
      deletions: 0,
      hunks: [
        {
          old_start: 1,
          old_lines: 0,
          new_start: 1,
          new_lines: 3000,
          lines: Array.from({ length: 3000 }, (_, i) => ({
            kind: "added",
            content: `row ${i + 1}`,
            old_line_no: null,
            new_line_no: i + 1,
          })),
        },
      ],
    };
    const { container } = render(<DiffText file={file} mode={mode} fillViewport />);
    const viewport = container.firstElementChild as HTMLDivElement;
    expect(viewport.style.height).toBe("");
    expect(screen.getByText("row 40")).toBeTruthy();
    expect(screen.queryByText("row 3000")).toBeNull();
    expect(screen.getAllByText(/^row /).length).toBeLessThan(70);
    height = 300;
    act(() => resized());
    expect(screen.getAllByText(/^row /).length).toBeLessThan(30);
    fireEvent.scroll(viewport, { target: { scrollTop: 3001 * 20 - height } });
    expect(screen.getByText("row 3000")).toBeTruthy();
    expect(screen.queryByText("row 1")).toBeNull();
    expect(screen.getAllByText(/^row /).length).toBeLessThan(30);
  },
);
