// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useFixedVirtualizer } from "./useFixedVirtualizer";

const observers: MockResizeObserver[] = [];
class MockResizeObserver {
  readonly notify: () => void;
  observe = vi.fn();
  disconnect = vi.fn();
  constructor(notify: () => void) {
    this.notify = notify;
    observers.push(this);
  }
}

function viewport(height = 100, offset = 0) {
  const element = document.createElement("div");
  let currentHeight = height;
  Object.defineProperty(element, "clientHeight", { get: () => currentHeight });
  element.scrollTop = offset;
  const scrollTo = vi.fn((optionsOrX?: ScrollToOptions | number, y?: number) => {
    element.scrollTop = typeof optionsOrX === "number" ? (y ?? 0) : (optionsOrX?.top ?? 0);
    element.dispatchEvent(new Event("scroll"));
  });
  element.scrollTo = scrollTo;
  return {
    element,
    scrollTo,
    resize: (height: number) => {
      currentHeight = height;
    },
    scroll: (offset: number) => {
      element.scrollTop = offset;
      element.dispatchEvent(new Event("scroll"));
    },
  };
}

beforeEach(() => {
  observers.length = 0;
  vi.stubGlobal("ResizeObserver", MockResizeObserver);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("useFixedVirtualizer", () => {
  it("waits for a scroll element and exposes rows as soon as the node arrives", () => {
    const view = viewport();
    const { result, rerender } = renderHook(
      ({ element }) => useFixedVirtualizer({ element, count: 100, rowHeight: 20, overscan: 2 }),
      { initialProps: { element: null as HTMLDivElement | null } },
    );
    expect(result.current.items).toEqual([]);
    expect(result.current.totalSize).toBe(2000);
    act(() => result.current.scrollToIndex(10, { align: "center" }));
    expect(view.scrollTo).not.toHaveBeenCalled();
    rerender({ element: view.element });
    expect(result.current.items.map((item) => item.index)).toEqual([0, 1, 2, 3, 4, 5, 6]);
    act(() => result.current.scrollToIndex(10, { align: "center" }));
    expect(view.scrollTo).toHaveBeenCalledExactlyOnceWith({ top: 160 });
  });

  it("covers partially visible rows with bounded overscan when scrolling", () => {
    const view = viewport();
    const { result } = renderHook(() =>
      useFixedVirtualizer({ element: view.element, count: 100, rowHeight: 20, overscan: 2 }),
    );
    const initialItems = result.current.items;
    act(() => view.scroll(75));
    expect(result.current.items.map((item) => item.index)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);
    expect(result.current.items[0]).toMatchObject({ start: 20, size: 20 });
    expect(initialItems.map((item) => item.index)).toEqual([0, 1, 2, 3, 4, 5, 6]);
    act(() => view.scroll(1900));
    expect(result.current.items.map((item) => item.index)).toEqual([93, 94, 95, 96, 97, 98, 99]);
    expect(result.current.items[result.current.items.length - 1]?.start).toBe(1980);
  });

  it("updates the window when a hidden panel becomes visible or changes height", () => {
    const view = viewport(0);
    const { result } = renderHook(() =>
      useFixedVirtualizer({ element: view.element, count: 100, rowHeight: 20, overscan: 2 }),
    );
    expect(result.current.items).toEqual([]);
    act(() => {
      view.resize(100);
      observers[0]?.notify();
    });
    expect(result.current.items.map((item) => item.index)).toEqual([0, 1, 2, 3, 4, 5, 6]);
    act(() => {
      view.resize(200);
      observers[0]?.notify();
    });
    expect(result.current.items).toHaveLength(12);
    expect(result.current.items[result.current.items.length - 1]?.index).toBe(11);
  });

  it("adapts when pages append, the list shrinks, or becomes empty", () => {
    const view = viewport(100, 40);
    const { result, rerender } = renderHook(
      ({ count }) =>
        useFixedVirtualizer({ element: view.element, count, rowHeight: 20, overscan: 2 }),
      { initialProps: { count: 3 } },
    );
    expect(result.current.items.map((item) => item.index)).toEqual([0, 1, 2]);
    rerender({ count: 100 });
    expect(result.current.totalSize).toBe(2000);
    expect(result.current.items.map((item) => item.index)).toEqual([0, 1, 2, 3, 4, 5, 6, 7, 8]);
    act(() => view.scroll(1900));
    rerender({ count: 3 });
    expect(result.current.totalSize).toBe(60);
    expect(result.current.items.every((item) => item.index >= 0 && item.index < 3)).toBe(true);
    rerender({ count: 0 });
    expect(result.current.items).toEqual([]);
    expect(result.current.totalSize).toBe(0);
    act(() => result.current.scrollToIndex(0));
    expect(view.scrollTo).not.toHaveBeenCalled();
  });

  it("centers located rows and clamps both ends to the actual scrollable range", () => {
    const view = viewport();
    const { result } = renderHook(() =>
      useFixedVirtualizer({ element: view.element, count: 100, rowHeight: 20, overscan: 2 }),
    );
    act(() => result.current.scrollToIndex(10, { align: "center" }));
    expect(view.scrollTo).toHaveBeenLastCalledWith({ top: 160 });
    expect(result.current.items.some((item) => item.index === 10)).toBe(true);
    act(() => result.current.scrollToIndex(-10, { align: "center" }));
    expect(view.scrollTo).toHaveBeenLastCalledWith({ top: 0 });
    act(() => result.current.scrollToIndex(1000, { align: "center" }));
    expect(view.scrollTo).toHaveBeenLastCalledWith({ top: 1900 });
    act(() => result.current.scrollToIndex(10));
    expect(view.scrollTo).toHaveBeenLastCalledWith({ top: 200 });
  });

  it("keeps short lists at the top when locating a row", () => {
    const view = viewport();
    const { result } = renderHook(() =>
      useFixedVirtualizer({ element: view.element, count: 3, rowHeight: 20, overscan: 2 }),
    );
    act(() => result.current.scrollToIndex(2, { align: "center" }));
    expect(view.scrollTo).toHaveBeenLastCalledWith({ top: 0 });
  });

  it("rebinds to a replacement scroll node and detaches listeners on unmount", () => {
    const first = viewport();
    const second = viewport(100, 400);
    const removeFirst = vi.spyOn(first.element, "removeEventListener");
    const removeSecond = vi.spyOn(second.element, "removeEventListener");
    const { result, rerender, unmount } = renderHook(
      ({ element }) => useFixedVirtualizer({ element, count: 100, rowHeight: 20, overscan: 2 }),
      { initialProps: { element: first.element } },
    );
    const oldObserver = observers[0]!;
    expect(oldObserver.observe).toHaveBeenCalledWith(first.element);
    rerender({ element: second.element });
    expect(oldObserver.disconnect).toHaveBeenCalledOnce();
    expect(removeFirst).toHaveBeenCalledWith("scroll", expect.any(Function));
    expect(result.current.items[0]?.index).toBe(18);
    act(() => first.scroll(800));
    expect(result.current.items[0]?.index).toBe(18);
    act(() => second.scroll(600));
    expect(result.current.items[0]?.index).toBe(28);
    const currentObserver = observers[observers.length - 1]!;
    expect(currentObserver.observe).toHaveBeenCalledWith(second.element);
    unmount();
    expect(currentObserver.disconnect).toHaveBeenCalledOnce();
    expect(removeSecond).toHaveBeenCalledWith("scroll", expect.any(Function));
  });
});
