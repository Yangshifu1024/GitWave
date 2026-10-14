// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ThreePaneLayout } from "./ThreePaneLayout";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

let notifyResize: ResizeObserverCallback;
beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: ResizeObserverCallback) {
        notifyResize = callback;
      }
      observe = vi.fn();
      disconnect = vi.fn();
    },
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function resize(width: number) {
  act(() =>
    notifyResize([{ contentRect: { width } } as ResizeObserverEntry], {} as ResizeObserver),
  );
}
function mount() {
  return render(<ThreePaneLayout sidebar="sidebar" main="history" inspector="inspector" />);
}
const widthOf = (name: string) =>
  Number(screen.getByRole("separator", { name }).getAttribute("aria-valuenow"));

describe("history-first pane resizing", () => {
  it("supports keyboard resizing in each direction and Home resets", () => {
    mount();
    resize(1440);
    const sidebar = screen.getByRole("separator", { name: "app.layout.resizeSidebar" });
    const inspector = screen.getByRole("separator", { name: "app.layout.resizeInspector" });
    expect(widthOf("app.layout.resizeSidebar")).toBe(240);
    expect(widthOf("app.layout.resizeInspector")).toBe(360);
    fireEvent.keyDown(sidebar, { key: "ArrowRight" });
    fireEvent.keyDown(inspector, { key: "ArrowLeft", shiftKey: true });
    expect(widthOf("app.layout.resizeSidebar")).toBe(250);
    expect(widthOf("app.layout.resizeInspector")).toBe(400);
    fireEvent.keyDown(sidebar, { key: "Home" });
    fireEvent.keyDown(inspector, { key: "Home" });
    expect(widthOf("app.layout.resizeSidebar")).toBe(240);
    expect(widthOf("app.layout.resizeInspector")).toBe(360);
  });

  it("preserves history space when the window narrows and restores preferences", () => {
    mount();
    resize(1440);
    const inspector = screen.getByRole("separator", { name: "app.layout.resizeInspector" });
    for (let i = 0; i < 9; i++) fireEvent.keyDown(inspector, { key: "ArrowLeft", shiftKey: true });
    expect(widthOf("app.layout.resizeInspector")).toBe(720);
    resize(1024);
    expect(
      widthOf("app.layout.resizeSidebar") + widthOf("app.layout.resizeInspector") + 361,
    ).toBeLessThanOrEqual(1024);
    resize(1440);
    expect(widthOf("app.layout.resizeInspector")).toBe(720);
    for (let i = 0; i < 20; i++)
      fireEvent.keyDown(inspector, { key: "ArrowRight", shiftKey: true });
    expect(widthOf("app.layout.resizeInspector")).toBe(300);
  });

  it("accumulates mouse movements and cleans up an interrupted drag", () => {
    const view = mount();
    resize(1440);
    document.body.style.cursor = "crosshair";
    document.body.style.userSelect = "text";
    fireEvent.mouseDown(screen.getByRole("separator", { name: "app.layout.resizeSidebar" }), {
      clientX: 240,
    });
    fireEvent.mouseMove(document, { clientX: 260 });
    fireEvent.mouseMove(document, { clientX: 280 });
    expect(widthOf("app.layout.resizeSidebar")).toBe(280);
    view.unmount();
    expect(document.body.style.cursor).toBe("crosshair");
    expect(document.body.style.userSelect).toBe("text");
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  });
});
