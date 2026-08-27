import { create } from "zustand";

interface LayoutStoreState {
  inspectorMaximized: boolean;
  toggleInspectorMaximized: () => void;
  setInspectorMaximized: (value: boolean) => void;
  /** Bottom working-copy bar collapsed to a single row (commit box hidden). */
  wcBarCollapsed: boolean;
  toggleWcBarCollapsed: () => void;
  setWcBarCollapsed: (value: boolean) => void;
  /** Bottom working-copy bar stretched to half the window height. */
  wcBarMaximized: boolean;
  toggleWcBarMaximized: () => void;
  setWcBarMaximized: (value: boolean) => void;
}

export const useLayoutStore = create<LayoutStoreState>((set) => ({
  inspectorMaximized: false,
  toggleInspectorMaximized: () => set((s) => ({ inspectorMaximized: !s.inspectorMaximized })),
  setInspectorMaximized: (value) => set({ inspectorMaximized: value }),

  wcBarCollapsed: false,
  toggleWcBarCollapsed: () =>
    set((s) => {
      const collapsed = !s.wcBarCollapsed;
      // A collapsed bar has no maximized body to restore.
      return { wcBarCollapsed: collapsed, wcBarMaximized: collapsed ? false : s.wcBarMaximized };
    }),
  setWcBarCollapsed: (value) =>
    set(value ? { wcBarCollapsed: true, wcBarMaximized: false } : { wcBarCollapsed: false }),

  wcBarMaximized: false,
  toggleWcBarMaximized: () => set((s) => ({ wcBarMaximized: !s.wcBarMaximized })),
  setWcBarMaximized: (value) => set({ wcBarMaximized: value }),
}));
