export const PALETTES = ["native-blue", "tide"] as const;

export type Palette = (typeof PALETTES)[number];

export const DEFAULT_PALETTE: Palette = "native-blue";

export interface PaletteSwatch {
  canvas: string;
  sidebar: string;
  accent: string;
  lanes: [string, string, string];
}

export interface PaletteMeta {
  id: Palette;
  name: string;
  description: string;
  /** Light-mode preview colors for the settings picker. */
  swatch: PaletteSwatch;
}

export const PALETTE_META: Record<Palette, PaletteMeta> = {
  "native-blue": {
    id: "native-blue",
    name: "Native Blue",
    description: "macOS system look with a system blue accent",
    swatch: {
      canvas: "#ececec",
      sidebar: "#dfdfdf",
      accent: "#007aff",
      lanes: ["#32ade6", "#5856d6", "#af52de"],
    },
  },
  tide: {
    id: "tide",
    name: "Tide Studio",
    description: "GitWave's teal signature on cool gray",
    swatch: {
      canvas: "#f4f6f8",
      sidebar: "#e6ebef",
      accent: "#1a8f8a",
      // Matches shipped --color-lane-1..3 (see src/styles/tokens.css).
      lanes: ["#1a8f8a", "#3d6b9a", "#4a5fa8"],
    },
  },
};

/** Unknown or missing values fall back to the default palette. */
export function normalizePalette(value: string | null | undefined): Palette {
  return (PALETTES as readonly string[]).includes(value ?? "")
    ? (value as Palette)
    : DEFAULT_PALETTE;
}

const STORAGE_KEY = "gitwave-palette";

/** Current preference from localStorage (default when unset or corrupt). */
export function readStoredPalette(): Palette {
  if (typeof localStorage === "undefined") return DEFAULT_PALETTE;
  return normalizePalette(localStorage.getItem(STORAGE_KEY));
}

function applyPalette(palette: Palette): void {
  document.documentElement.dataset.palette = palette;
}

/**
 * Reads the persisted preference and applies it to <html>. Runs before
 * React mounts to avoid FOUC — see applyInitialPreferences() in main.tsx.
 */
export function applyInitialPalette(): void {
  applyPalette(readStoredPalette());
}

/**
 * Persists the choice and applies it immediately.
 */
export function storePalette(palette: Palette): void {
  localStorage.setItem(STORAGE_KEY, palette);
  applyPalette(palette);
}
