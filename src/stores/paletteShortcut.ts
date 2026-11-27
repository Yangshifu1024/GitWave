import { isMacOS } from "@/lib/platform";
import { create } from "zustand";
import { persist } from "zustand/middleware";
export const paletteChords = ["Mod+K", "Mod+Shift+K", "Mod+Alt+K"] as const;
type Chord = (typeof paletteChords)[number];
export const usePaletteShortcut = create<{ chord: Chord; set: (chord: Chord) => void }>()(
  persist((set) => ({ chord: "Mod+K", set: (chord) => set({ chord }) }), {
    name: "gitwave.palette-shortcut",
    merge: (stored, current) => {
      const value = (stored as { chord?: Chord } | null)?.chord;
      return { ...current, chord: value && paletteChords.includes(value) ? value : "Mod+K" };
    },
  }),
);
export function matchesPaletteShortcut(e: KeyboardEvent, chord: Chord) {
  return (
    !e.isComposing &&
    !e.repeat &&
    (isMacOS() ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey) &&
    (e.key.toLowerCase() === "k" || (e.altKey && e.code === "KeyK")) &&
    e.shiftKey === (chord === "Mod+Shift+K") &&
    e.altKey === (chord === "Mod+Alt+K")
  );
}
