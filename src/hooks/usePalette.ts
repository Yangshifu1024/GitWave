import { useCallback, useState } from "react";
import { readStoredPalette, storePalette, type Palette } from "@/lib/palette";

export interface UsePaletteReturn {
  palette: Palette;
  setPalette: (p: Palette) => void;
}

/**
 * Hook for reading and writing the user color-palette preference.
 * - `palette` is the current preference (defaults to "native-blue")
 * - `setPalette` persists to localStorage AND applies it to <html> at once
 *
 * The initial application happens before React mounts (see
 * applyInitialPalette() in main.tsx); this hook only keeps component state
 * in sync after that.
 */
export function usePalette(): UsePaletteReturn {
  const [palette, setPaletteState] = useState<Palette>(readStoredPalette);

  const setPalette = useCallback((p: Palette) => {
    storePalette(p);
    setPaletteState(p);
  }, []);

  return { palette, setPalette };
}
