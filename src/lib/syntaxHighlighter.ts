import { createHighlighterCore, type LanguageInput } from "shiki/core";
import { createOnigurumaEngine } from "shiki/engine/oniguruma";
import wasmUrl from "shiki/onig.wasm?url";

// Grammars are static data assets rather than executable JavaScript bundles.
// Each loader includes the grammar's embedded-language dependencies.
const grammars: Record<string, () => Promise<{ default: () => Promise<LanguageInput[]> }>> = {
  typescript: () => import("virtual:syntax-grammar/typescript"),
  tsx: () => import("virtual:syntax-grammar/tsx"),
  javascript: () => import("virtual:syntax-grammar/javascript"),
  jsx: () => import("virtual:syntax-grammar/jsx"),
  rust: () => import("virtual:syntax-grammar/rust"),
  python: () => import("virtual:syntax-grammar/python"),
  go: () => import("virtual:syntax-grammar/go"),
  java: () => import("virtual:syntax-grammar/java"),
  c: () => import("virtual:syntax-grammar/c"),
  cpp: () => import("virtual:syntax-grammar/cpp"),
  json: () => import("virtual:syntax-grammar/json"),
  markdown: () => import("virtual:syntax-grammar/markdown"),
  toml: () => import("virtual:syntax-grammar/toml"),
  yaml: () => import("virtual:syntax-grammar/yaml"),
  bash: () => import("virtual:syntax-grammar/bash"),
};

let highlighter: ReturnType<typeof createHighlighterCore> | undefined;
const pendingLanguages = new Map<string, Promise<void>>();

export async function highlightLines(code: string, lang: string, theme: string) {
  highlighter ??= createHighlighterCore({
    langs: [],
    themes: [import("shiki/themes/github-dark.mjs"), import("shiki/themes/github-light.mjs")],
    engine: createOnigurumaEngine(
      fetch(wasmUrl).then((response) => {
        if (!response.ok) throw new Error("Could not load syntax engine");
        return response.arrayBuffer();
      }),
    ),
  }).catch((error: unknown) => {
    highlighter = undefined;
    throw error;
  });
  const instance = await highlighter;
  const loader = grammars[lang];
  if (!loader) throw new Error(`Unsupported syntax language: ${lang}`);
  if (!instance.getLoadedLanguages().includes(lang)) {
    let pending = pendingLanguages.get(lang);
    if (!pending) {
      pending = loader()
        .then((module) => module.default())
        .then((languages) => instance.loadLanguage(...languages))
        .finally(() => pendingLanguages.delete(lang));
      pendingLanguages.set(lang, pending);
    }
    await pending;
  }
  return instance.codeToTokens(code, { lang, theme });
}
