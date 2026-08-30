// Locale parity guard (docs/tasks/feat-i18n/plan.md):
//  1. en and zh-CN expose the same domain files and the same leaf keys.
//  2. Every error code constant in src-tauri/src/domain/error_codes/ has a
//     matching leaf under the merged "errors" subtree in both locales, and
//     every errors leaf maps back to a declared code (drift guard both ways).
// Runs in plain vitest (node fs), no i18next instance needed.

import { readFileSync, readdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const localesDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(localesDir, "../../..");
const errorCodesDir = path.join(repoRoot, "src-tauri/src/domain/error_codes");

function domainFiles(locale: string): string[] {
  return readdirSync(path.join(localesDir, locale)).filter((f) => f.endsWith(".json"));
}

function loadDomain(locale: string, file: string): Record<string, unknown> {
  return JSON.parse(readFileSync(path.join(localesDir, locale, file), "utf8")) as Record<
    string,
    unknown
  >;
}

/** Leaf key paths of a domain tree, relative to the domain root. */
function leafKeys(value: unknown, prefix = ""): Set<string> {
  const leaves = new Set<string>();
  if (value !== null && typeof value === "object" && !Array.isArray(value)) {
    const entries = Object.entries(value as Record<string, unknown>);
    if (entries.length === 0) leaves.add(prefix);
    for (const [key, child] of entries) {
      const next = prefix ? `${prefix}.${key}` : key;
      for (const leaf of leafKeys(child, next)) leaves.add(leaf);
    }
  } else {
    leaves.add(prefix);
  }
  return leaves;
}

/** Stable strings must exist in both locales, so empty-value leaves fail too. */
function assertNonEmptyStrings(tree: Record<string, unknown>, locale: string): void {
  for (const [key, value] of Object.entries(tree)) {
    if (value !== null && typeof value === "object") {
      assertNonEmptyStrings(value as Record<string, unknown>, locale);
    } else {
      expect(value, `${locale}: ${key} must be a non-empty string`).toBeTruthy();
    }
  }
}

/** {{placeholder}} names referenced by a translated string. */
function placeholders(text: string): Set<string> {
  return new Set([...text.matchAll(/\{\{(\w+)\}\}/g)].flatMap((m) => (m[1] ? [m[1]] : [])));
}

function deepMerge(target: Record<string, unknown>, source: Record<string, unknown>): void {
  for (const [key, value] of Object.entries(source)) {
    const existing = target[key];
    if (
      existing !== null &&
      typeof existing === "object" &&
      value !== null &&
      typeof value === "object"
    ) {
      deepMerge(existing as Record<string, unknown>, value as Record<string, unknown>);
    } else {
      target[key] = value;
    }
  }
}

/** Every `pub const X: &str = "…";` across the error_codes module files. */
function extractErrorCodes(): string[] {
  const codes: string[] = [];
  for (const file of readdirSync(errorCodesDir)) {
    if (!file.endsWith(".rs")) continue;
    const source = readFileSync(path.join(errorCodesDir, file), "utf8");
    for (const match of source.matchAll(/pub const [A-Z0-9_]+: &str = "([^"]+)";/g)) {
      if (match[1]) codes.push(match[1]);
    }
  }
  return codes;
}

/** All "errors" subtrees merged from every errors-*.json file of a locale. */
function mergedErrors(locale: string): Record<string, unknown> {
  const merged: Record<string, unknown> = {};
  for (const file of domainFiles(locale)) {
    if (!file.startsWith("errors")) continue;
    deepMerge(merged, loadDomain(locale, file)["errors"] as Record<string, unknown>);
  }
  return merged;
}

describe("locale parity", () => {
  it("en and zh-CN expose the same domain files", () => {
    expect(domainFiles("zh-CN").sort()).toEqual(domainFiles("en").sort());
  });

  it("en and zh-CN expose identical leaf keys per domain", () => {
    for (const file of domainFiles("en")) {
      const en = loadDomain("en", file);
      const zh = loadDomain("zh-CN", file);
      for (const [domain, subtree] of Object.entries(en)) {
        expect(zh, `${file}: zh-CN must define domain "${domain}"`).toHaveProperty(domain);
        const enLeaves = leafKeys(subtree);
        const zhLeaves = leafKeys(zh[domain]);
        expect(
          [...zhLeaves].filter((k) => !enLeaves.has(k)),
          `${file}/${domain}`,
        ).toEqual([]);
        expect(
          [...enLeaves].filter((k) => !zhLeaves.has(k)),
          `${file}/${domain}`,
        ).toEqual([]);
      }
      assertNonEmptyStrings(en, `en/${file}`);
      assertNonEmptyStrings(zh, `zh-CN/${file}`);
    }
  });

  it("en and zh-CN use identical {{placeholder}} sets per leaf", () => {
    for (const file of domainFiles("en")) {
      const en = loadDomain("en", file);
      const zh = loadDomain("zh-CN", file);
      const walk = (enNode: unknown, zhNode: unknown, prefix: string): void => {
        if (typeof enNode === "string" && typeof zhNode === "string") {
          const enPh = placeholders(enNode);
          const zhPh = placeholders(zhNode);
          expect(
            [...zhPh].filter((p) => !enPh.has(p)),
            `${file}: ${prefix} zh-CN has extra placeholders`,
          ).toEqual([]);
          expect(
            [...enPh].filter((p) => !zhPh.has(p)),
            `${file}: ${prefix} zh-CN misses placeholders`,
          ).toEqual([]);
          return;
        }
        if (
          enNode !== null &&
          typeof enNode === "object" &&
          zhNode !== null &&
          typeof zhNode === "object"
        ) {
          for (const [key, child] of Object.entries(enNode as Record<string, unknown>)) {
            walk(
              child,
              (zhNode as Record<string, unknown>)[key],
              prefix ? `${prefix}.${key}` : key,
            );
          }
        }
      };
      walk(en, zh, path.basename(file, ".json"));
    }
  });

  it("every Rust error code has a matching errors leaf in both locales", () => {
    const codes = extractErrorCodes();
    expect(codes.length).toBeGreaterThan(0);
    for (const locale of ["en", "zh-CN"]) {
      const errors = mergedErrors(locale);
      for (const code of codes) {
        let node: unknown = errors;
        for (const segment of code.split(".")) {
          expect(
            node !== null && typeof node === "object" && segment in node,
            `${locale}: errors.${code} missing`,
          ).toBe(true);
          node = (node as Record<string, unknown>)[segment];
        }
      }
    }
  });

  it("every locale errors leaf maps back to a declared Rust error code", () => {
    const declared = new Set(extractErrorCodes());
    for (const locale of ["en", "zh-CN"]) {
      const walk = (node: Record<string, unknown>, prefix: string): void => {
        for (const [key, value] of Object.entries(node)) {
          const code = prefix ? `${prefix}.${key}` : key;
          if (value !== null && typeof value === "object") {
            walk(value as Record<string, unknown>, code);
          } else {
            expect(
              declared.has(code),
              `${locale}: errors.${code} has no error_codes constant`,
            ).toBe(true);
          }
        }
      };
      walk(mergedErrors(locale), "");
    }
  });
});
