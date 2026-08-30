export interface FontPreferences {
  sans: string;
  mono: string;
  /** UI font size in px as raw input ("" = app default 16). */
  sansSize: string;
  /** Mono font size in px as raw input ("" = app default 12). */
  monoSize: string;
}

export type FontSlot = "sans" | "mono";

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

/** App-default sizes (px) and clamp bounds for the two size settings. The mono
 * default is the leading mono size (text-xs = 12px) that --font-mono-scale
 * normalizes against. */
export const FONT_SIZE_DEFAULTS: Record<FontSlot, number> = { sans: 16, mono: 12 };
export const FONT_SIZE_BOUNDS: Record<FontSlot, { min: number; max: number }> = {
  sans: { min: 12, max: 20 },
  mono: { min: 10, max: 20 },
};

const SIZE_STORAGE_KEYS: Record<FontSlot, string> = {
  sans: "gitwave-font-sans-size",
  mono: "gitwave-font-mono-size",
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
    .map((segment) => segment.replace(FORBIDDEN_CHARS, "").replace(/\s+/g, " ").trim())
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
 * Normalizes raw size input into a clamped px value string: leading integer
 * parse ("16.7" → 16, "016" → 16), clamped into [min, max]. Empty or
 * non-numeric input means "use the app default".
 */
export function sanitizeSizeInput(input: string, min: number, max: number): string {
  const value = Number.parseInt(input, 10);
  if (Number.isNaN(value)) return "";
  return String(Math.min(max, Math.max(min, value)));
}

/** Slot-aware wrapper that applies the built-in clamp bounds. */
export function sanitizeSlotSize(slot: FontSlot, input: string): string {
  const { min, max } = FONT_SIZE_BOUNDS[slot];
  return sanitizeSizeInput(input, min, max);
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

/**
 * Applies one size setting on <html>. Sans size scales the whole rem-based
 * interface via the root font-size; mono size only scales mono text via
 * --font-mono-scale (see the .font-mono rules at the end of tokens.css).
 */
function applySize(slot: FontSlot, size: string): void {
  const rootStyle = document.documentElement.style;
  if (size === "") {
    rootStyle.removeProperty(slot === "sans" ? "font-size" : "--font-mono-scale");
  } else if (slot === "sans") {
    rootStyle.setProperty("font-size", `${size}px`);
  } else {
    rootStyle.setProperty("--font-mono-scale", String(Number(size) / FONT_SIZE_DEFAULTS.mono));
  }
}

function readStoredFont(slot: FontSlot): string {
  if (typeof localStorage === "undefined") return "";
  return localStorage.getItem(STORAGE_KEYS[slot]) ?? "";
}

function readStoredSize(slot: FontSlot): string {
  if (typeof localStorage === "undefined") return "";
  return localStorage.getItem(SIZE_STORAGE_KEYS[slot]) ?? "";
}

/** Current preferences from localStorage ("" = default chain / default size). */
export function readStoredFonts(): FontPreferences {
  return {
    sans: readStoredFont("sans"),
    mono: readStoredFont("mono"),
    sansSize: readStoredSize("sans"),
    monoSize: readStoredSize("mono"),
  };
}

/**
 * Reads the persisted preferences and applies them to <html>. Runs before
 * React mounts to avoid FOUC — see applyInitialPreferences() in main.tsx.
 */
export function applyInitialFonts(): void {
  const stored = readStoredFonts();
  applyFont("sans", sanitizeFontList(stored.sans));
  applyFont("mono", sanitizeFontList(stored.mono));
  applySize("sans", sanitizeSlotSize("sans", stored.sansSize));
  applySize("mono", sanitizeSlotSize("mono", stored.monoSize));
}

/**
 * Persists the given preferences (sanitized here) and applies them
 * immediately. Returns the sanitized values so callers can realign drafts.
 */
export function storeFonts(prefs: FontPreferences): FontPreferences {
  const saved: FontPreferences = {
    sans: sanitizeFontList(prefs.sans),
    mono: sanitizeFontList(prefs.mono),
    sansSize: sanitizeSlotSize("sans", prefs.sansSize),
    monoSize: sanitizeSlotSize("mono", prefs.monoSize),
  };
  if (typeof localStorage !== "undefined") {
    for (const slot of ["sans", "mono"] as const) {
      if (saved[slot] === "") localStorage.removeItem(STORAGE_KEYS[slot]);
      else localStorage.setItem(STORAGE_KEYS[slot], saved[slot]);
      const size = slot === "sans" ? saved.sansSize : saved.monoSize;
      if (size === "") localStorage.removeItem(SIZE_STORAGE_KEYS[slot]);
      else localStorage.setItem(SIZE_STORAGE_KEYS[slot], size);
    }
  }
  applyFont("sans", saved.sans);
  applyFont("mono", saved.mono);
  applySize("sans", saved.sansSize);
  applySize("mono", saved.monoSize);
  return saved;
}
