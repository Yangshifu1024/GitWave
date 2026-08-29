import { useCallback, useState } from "react";
import { readStoredFonts, storeFonts, type FontPreferences } from "@/lib/fonts";

export interface UseFontsReturn {
  fonts: FontPreferences;
  /**
   * Persists and applies the preferences at once. Resolves to the sanitized
   * values so callers can realign their drafts with what was saved.
   */
  saveFonts: (prefs: FontPreferences) => FontPreferences;
}

/**
 * Hook for reading and writing the user font preferences. Mirrors usePalette:
 * the initial application happens before React mounts (applyInitialFonts()
 * in main.tsx); this hook only keeps component state in sync after that.
 */
export function useFonts(): UseFontsReturn {
  const [fonts, setFonts] = useState<FontPreferences>(readStoredFonts);

  const saveFonts = useCallback((prefs: FontPreferences) => {
    const saved = storeFonts(prefs);
    setFonts(saved);
    return saved;
  }, []);

  return { fonts, saveFonts };
}
