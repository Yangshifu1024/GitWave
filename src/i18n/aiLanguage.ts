// AI reply language (F010): an app-global preference, stored in localStorage
// and passed to every prose-producing AI command over IPC. Rust sanitizes the
// value and appends a "respond in …" directive to the system prompt. The
// prompt body stays English; only the output language is governed here.

export const AI_LANGUAGES = ["en", "zh", "ja", "ko"] as const;
export type AiLanguage = (typeof AI_LANGUAGES)[number];

export const AI_LANGUAGE_STORAGE_KEY = "gitwave.aiLanguage";

/** Native names — language-invariant labels, deliberately not translated. */
export const AI_LANGUAGE_NATIVE_NAMES: Record<AiLanguage, string> = {
  en: "English",
  zh: "中文",
  ja: "日本語",
  ko: "한국어",
};

export function readStoredAiLanguage(): AiLanguage {
  try {
    const raw = localStorage.getItem(AI_LANGUAGE_STORAGE_KEY);
    if (raw !== null && (AI_LANGUAGES as readonly string[]).includes(raw)) {
      return raw as AiLanguage;
    }
  } catch {
    // Fall through to the default below.
  }
  return "en";
}

export function setAiLanguage(lang: AiLanguage): void {
  try {
    localStorage.setItem(AI_LANGUAGE_STORAGE_KEY, lang);
  } catch {
    // Persistence is best-effort; the in-session value is still used.
  }
}
