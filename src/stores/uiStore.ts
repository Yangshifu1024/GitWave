// Global UI surface state shared across top-level components: the settings
// modal (rendered by Toolbar) and the command palette (rendered by App).
// A store lets the palette and keyboard shortcuts open either one without
// prop-drilling through the layout.

import { create } from "zustand";

interface UiState {
  settingsOpen: boolean;
  setSettingsOpen: (open: boolean) => void;
  paletteOpen: boolean;
  setPaletteOpen: (open: boolean) => void;
}

export const useUiStore = create<UiState>()((set) => ({
  settingsOpen: false,
  setSettingsOpen: (settingsOpen) => set({ settingsOpen }),
  paletteOpen: false,
  setPaletteOpen: (paletteOpen) => set({ paletteOpen }),
}));
