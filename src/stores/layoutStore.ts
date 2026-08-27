import { create } from "zustand";

interface LayoutStoreState {
  inspectorMaximized: boolean;
  toggleInspectorMaximized: () => void;
  setInspectorMaximized: (value: boolean) => void;
}

export const useLayoutStore = create<LayoutStoreState>((set) => ({
  inspectorMaximized: false,
  toggleInspectorMaximized: () => set((s) => ({ inspectorMaximized: !s.inspectorMaximized })),
  setInspectorMaximized: (value) => set({ inspectorMaximized: value }),
}));
