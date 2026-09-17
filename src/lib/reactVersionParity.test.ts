// React runtime parity guard (docs/tasks/fix-react-dom-version-mismatch/plan.md):
// react and react-dom must resolve to the exact same version. A mismatch throws
// React error #527 before the first render, and because the Tauri main window
// starts hidden (`visible: false` in tauri.conf.json) the app never reveals
// itself — it looks like the binary refuses to start.

import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

function installedVersion(name: string): string {
  const manifest = path.join(repoRoot, "node_modules", name, "package.json");
  return (JSON.parse(readFileSync(manifest, "utf8")) as { version: string }).version;
}

describe("react version parity", () => {
  it("react and react-dom resolve to the same version", () => {
    expect(installedVersion("react-dom")).toBe(installedVersion("react"));
  });
});
