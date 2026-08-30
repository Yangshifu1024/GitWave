// Domain-scoped translation files per locale. Each file wraps its own
// top-level domain key (e.g. changes.json → { "changes": { … } }) and all
// domains merge into a single i18next "translation" namespace, so keys read
// t("changes.panel.stageAll"). The error domain is split across several
// errors-*.json files that deep-merge into one "errors" subtree; other
// domains must keep their top-level key unique per file. Adding a file
// needs no resources.ts change (import.meta.glob picks it up).

import type { Resource } from "i18next";

interface DomainModule {
  default: Record<string, unknown>;
}

const enModules = import.meta.glob<DomainModule>("./locales/en/*.json", { eager: true });
const zhCnModules = import.meta.glob<DomainModule>("./locales/zh-CN/*.json", { eager: true });

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function deepMerge(target: Record<string, unknown>, source: Record<string, unknown>): void {
  for (const [key, value] of Object.entries(source)) {
    const existing = target[key];
    if (isPlainObject(existing) && isPlainObject(value)) {
      deepMerge(existing, value);
    } else {
      target[key] = value;
    }
  }
}

function mergeDomains(modules: Record<string, DomainModule>): Record<string, unknown> {
  const merged: Record<string, unknown> = {};
  for (const mod of Object.values(modules)) deepMerge(merged, mod.default);
  return merged;
}

export const resources: Resource = {
  en: { translation: mergeDomains(enModules) },
  "zh-CN": { translation: mergeDomains(zhCnModules) },
};
