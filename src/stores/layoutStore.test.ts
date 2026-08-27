import { beforeEach, describe, expect, it } from "vitest";
import { useLayoutStore } from "./layoutStore";

describe("useLayoutStore working-copy bar state", () => {
  beforeEach(() => {
    const { setWcBarCollapsed, setWcBarMaximized } = useLayoutStore.getState();
    setWcBarCollapsed(false);
    setWcBarMaximized(false);
  });

  it("starts expanded and not maximized", () => {
    const s = useLayoutStore.getState();
    expect(s.wcBarCollapsed).toBe(false);
    expect(s.wcBarMaximized).toBe(false);
  });

  it("toggles maximized without touching collapsed state", () => {
    const { toggleWcBarMaximized } = useLayoutStore.getState();
    toggleWcBarMaximized();
    expect(useLayoutStore.getState().wcBarMaximized).toBe(true);
    expect(useLayoutStore.getState().wcBarCollapsed).toBe(false);

    toggleWcBarMaximized();
    expect(useLayoutStore.getState().wcBarMaximized).toBe(false);
  });

  it("clears maximized when collapsing the bar", () => {
    useLayoutStore.getState().toggleWcBarMaximized();
    useLayoutStore.getState().toggleWcBarCollapsed();

    expect(useLayoutStore.getState().wcBarCollapsed).toBe(true);
    expect(useLayoutStore.getState().wcBarMaximized).toBe(false);
  });

  it("keeps maximized cleared after expanding again", () => {
    useLayoutStore.getState().toggleWcBarMaximized();
    useLayoutStore.getState().toggleWcBarCollapsed(); // collapse also clears maximize
    useLayoutStore.getState().toggleWcBarCollapsed(); // expand

    expect(useLayoutStore.getState().wcBarCollapsed).toBe(false);
    expect(useLayoutStore.getState().wcBarMaximized).toBe(false);
  });

  it("collapses via setter and stays collapsed on repeat set", () => {
    useLayoutStore.getState().setWcBarCollapsed(true);
    useLayoutStore.getState().setWcBarCollapsed(true);
    expect(useLayoutStore.getState().wcBarCollapsed).toBe(true);
    expect(useLayoutStore.getState().wcBarMaximized).toBe(false);
  });

  it("expanding via setter does not clear a later maximized state", () => {
    useLayoutStore.getState().setWcBarCollapsed(false);
    useLayoutStore.getState().setWcBarMaximized(true);
    useLayoutStore.getState().setWcBarCollapsed(false); // expand again

    expect(useLayoutStore.getState().wcBarMaximized).toBe(true);
  });
});
