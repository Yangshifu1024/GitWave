// Global UI surface state shared across top-level components: the settings
// modal (rendered by Toolbar) and the command palette (rendered by App).
// A store lets the palette and keyboard shortcuts open either one without
// prop-drilling through the layout.

import { create } from "zustand";

/** Actions fired from the Toolbar menu bar; ActionBar owns the handlers. */
export type AppMenuAction =
  | "workspace:new"
  | "workspace:rename"
  | "workspace:ai"
  | "workspace:export"
  | "workspace:import"
  | "workspace:delete"
  | "repo:init"
  | "repo:clone"
  | "repo:add"
  | "repo:fetch"
  | "repo:lfs"
  | "repo:hooks"
  | "branch:new"
  | "branch:pull"
  | "branch:push"
  | "branch:pr";

interface MenuActionRequest {
  /** Monotonic id so repeating the same action re-triggers the consumer. */
  id: number;
  action: AppMenuAction;
}

interface UiState {
  settingsOpen: boolean;
  setSettingsOpen: (open: boolean) => void;
  paletteOpen: boolean;
  setPaletteOpen: (open: boolean) => void;
  menuAction: MenuActionRequest | null;
  requestMenuAction: (action: AppMenuAction) => void;
  clearMenuAction: (id: number) => void;
}

let nextMenuActionId = 1;

export const useUiStore = create<UiState>()((set) => ({
  settingsOpen: false,
  setSettingsOpen: (settingsOpen) => set({ settingsOpen }),
  paletteOpen: false,
  setPaletteOpen: (paletteOpen) => set({ paletteOpen }),
  menuAction: null,
  requestMenuAction: (action) => set({ menuAction: { id: nextMenuActionId++, action } }),
  clearMenuAction: (id) =>
    set((state) => (state.menuAction?.id === id ? { menuAction: null } : {})),
}));
