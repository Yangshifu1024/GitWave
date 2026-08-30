// i18next bootstrap: UI language resolution, persistence and application.
//
// Resolution order: explicit user choice (localStorage) → system language
// (navigator) → English. Switching (Settings → General) persists and applies
// immediately: react-i18next re-renders every useTranslation consumer, the
// html lang attribute follows, and the native app menu rebuilds because
// useNativeAppMenu keys its effect on i18n.language.

import i18next, { type i18n as I18nInstance } from "i18next";
import { initReactI18next } from "react-i18next";

import { resources } from "./resources";

export const UI_LANGUAGES = ["en", "zh-CN"] as const;
export type UiLanguage = (typeof UI_LANGUAGES)[number];

export const LANGUAGE_STORAGE_KEY = "gitwave.language";

const SUPPORTED = new Set<string>(UI_LANGUAGES);

/** Explicit user choice from localStorage, or null when unset/invalid. */
export function readStoredLanguage(): UiLanguage | null {
  try {
    const raw = localStorage.getItem(LANGUAGE_STORAGE_KEY);
    return raw !== null && SUPPORTED.has(raw) ? (raw as UiLanguage) : null;
  } catch {
    return null;
  }
}

/** System language mapped onto a supported locale (zh* → zh-CN, else en). */
export function languageFromNavigator(): UiLanguage {
  // Range decision (F010): every zh* variant, including zh-TW / zh-HK,
  // maps onto the single simplified-Chinese locale the app ships.
  const candidates =
    typeof navigator !== "undefined" && navigator.languages?.length
      ? navigator.languages
      : typeof navigator !== "undefined" && navigator.language
        ? [navigator.language]
        : [];
  for (const tag of candidates) {
    if (tag.toLowerCase().startsWith("zh")) return "zh-CN";
    if (SUPPORTED.has(tag)) return tag as UiLanguage;
  }
  return "en";
}

function applyDocumentLang(lang: string): void {
  // Guard for non-DOM contexts (plain-node unit tests).
  if (typeof document !== "undefined") document.documentElement.lang = lang;
}

export function initI18n(): I18nInstance {
  void i18next.use(initReactI18next).init({
    resources,
    lng: readStoredLanguage() ?? languageFromNavigator(),
    fallbackLng: "en",
    // React escapes rendered text; leaving i18next escaping on would
    // double-escape interpolated values.
    interpolation: { escapeValue: false },
    returnNull: false,
  });
  applyDocumentLang(i18next.language);
  i18next.on("languageChanged", applyDocumentLang);
  return i18next;
}

/** Persist and apply a UI language switch (Settings → General). */
export function changeUiLanguage(lang: UiLanguage): void {
  try {
    localStorage.setItem(LANGUAGE_STORAGE_KEY, lang);
  } catch {
    // Persistence is best-effort; the in-session switch still applies.
  }
  void i18next.changeLanguage(lang);
}
