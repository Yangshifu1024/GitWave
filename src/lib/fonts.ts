export interface FontPreferences {
  sans: string;
  mono: string;
}

export type FontSlot = keyof FontPreferences;

/**
 * Placeholder copy for the settings inputs — what renders when unset. Display
 * only; kept in sync by hand with the leading faces of the default chains in
 * src/styles/tokens.css (--font-*-fallback).
 */
export const DEFAULT_FONT_LEADS: Record<FontSlot, string> = {
  sans: "System",
  mono: "Roboto Mono",
};

const STORAGE_KEYS: Record<FontSlot, string> = {
  sans: "gitwave-font-sans",
  mono: "gitwave-font-mono",
};

/** The token each slot overrides on <html> and the fallback chain it leads. */
const FONT_PROPERTIES: Record<FontSlot, { override: string; fallback: string }> = {
  sans: { override: "--font-sans", fallback: "--font-sans-fallback" },
  mono: { override: "--font-mono", fallback: "--font-mono-fallback" },
};

// Strips anything that could break out of the double-quoted font-family value
// built by buildFontOverride, plus Unicode "other" characters (control, zero-
// width format, surrogate, unassigned). Non-ASCII names (CJK etc.) are kept.
const FORBIDDEN_CHARS = /["'\\;{}<>]|\p{C}/gu;

/**
 * Normalizes raw user input into a safe font list: splits on commas, strips
 * quote/backslash/control characters per segment, collapses internal
 * whitespace runs (quoted names keep them and would fail to match), drops
 * empty segments. An empty result means "use the default chain".
 */
export function sanitizeFontList(input: string): string {
  return input
    .split(",")
    .map((segment) =>
      segment
        .replace(FORBIDDEN_CHARS, "")
        .replace(/\s+/g, " ")
        .trim(),
    )
    .filter((segment) => segment !== "")
    .join(", ");
}

/** Turns a sanitized list into the CSS value that leads the fallback chain. */
export function buildFontOverride(sanitized: string, slot: FontSlot): string {
  if (sanitized === "") return "";
  const { fallback } = FONT_PROPERTIES[slot];
  const leading = sanitized
    .split(",")
    .map((name) => `"${name.trim()}"`)
    .join(", ");
  return `${leading}, var(${fallback})`;
}

/**
 * Font-family value for settings previews: the draft chain, or the default
 * chain when the draft is blank (inherit would show the applied font instead).
 */
export function previewFontFamily(sanitized: string, slot: FontSlot): string {
  return buildFontOverride(sanitized, slot) || `var(${FONT_PROPERTIES[slot].fallback})`;
}

function applyFont(slot: FontSlot, sanitized: string): void {
  const { override } = FONT_PROPERTIES[slot];
  const value = buildFontOverride(sanitized, slot);
  if (value === "") {
    document.documentElement.style.removeProperty(override);
  } else {
    document.documentElement.style.setProperty(override, value);
  }
}

function readStoredFont(slot: FontSlot): string {
  if (typeof localStorage === "undefined") return "";
  return localStorage.getItem(STORAGE_KEYS[slot]) ?? "";
}

/** Current preferences from localStorage ("" = default chain). */
export function readStoredFonts(): FontPreferences {
  return { sans: readStoredFont("sans"), mono: readStoredFont("mono") };
}

/**
 * Reads the persisted preferences and applies them to <html>. Runs before
 * React mounts to avoid FOUC — see applyInitialPreferences() in main.tsx.
 */
export function applyInitialFonts(): void {
  const stored = readStoredFonts();
  applyFont("sans", sanitizeFontList(stored.sans));
  applyFont("mono", sanitizeFontList(stored.mono));
}

/**
 * Persists the given preferences (sanitized here) and applies them
 * immediately. Returns the sanitized values so callers can realign drafts.
 */
export function storeFonts(prefs: FontPreferences): FontPreferences {
  const saved: FontPreferences = {
    sans: sanitizeFontList(prefs.sans),
    mono: sanitizeFontList(prefs.mono),
  };
  if (typeof localStorage !== "undefined") {
    for (const slot of ["sans", "mono"] as const) {
      if (saved[slot] === "") localStorage.removeItem(STORAGE_KEYS[slot]);
      else localStorage.setItem(STORAGE_KEYS[slot], saved[slot]);
    }
  }
  applyFont("sans", saved.sans);
  applyFont("mono", saved.mono);
  return saved;
}
