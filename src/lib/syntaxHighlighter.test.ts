import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";
import { highlightLines } from "./syntaxHighlighter";

beforeAll(async () => {
  const wasm = await readFile(fileURLToPath(import.meta.resolve("shiki/onig.wasm")));
  vi.stubGlobal(
    "fetch",
    vi.fn(() => Promise.resolve(new Response(new Uint8Array(wasm).buffer))),
  );
});
afterAll(() => vi.unstubAllGlobals());

const samples = {
  typescript: "const answer: number = 42;",
  tsx: "const view = <div>Hello</div>;",
  javascript: "const answer = 42;",
  jsx: "const view = <div>Hello</div>;",
  rust: "fn main() { let answer = 42; }",
  python: "def hello(): return 42",
  go: "package main\nfunc main() {}",
  java: "public class Hello {}",
  c: "int main() { return 42; }",
  cpp: "#include <vector>\nstd::vector<int> values;",
  json: '{"answer": 42}',
  markdown: "# Hello\n```ts\nconst answer = 42;\n```",
  toml: 'name = "GitWave"',
  yaml: "name: GitWave",
  bash: 'echo "Hello"',
};

describe("bounded syntax highlighter", () => {
  for (const [lang, code] of Object.entries(samples)) {
    it(`loads ${lang} and its dependencies in both themes`, async () => {
      for (const theme of ["github-light", "github-dark"]) {
        const result = await highlightLines(code, lang, theme);
        expect(
          result.tokens.map((line) => line.map((token) => token.content).join("")).join("\n"),
        ).toBe(code);
        expect(result.tokens.flat().some((token) => token.color)).toBe(true);
      }
    });
  }
  it("shares concurrent grammar initialization", async () => {
    const outputs = await Promise.all([
      highlightLines("const a = 1", "typescript", "github-light"),
      highlightLines("const b = 2", "typescript", "github-dark"),
    ]);
    expect(outputs[0].tokens[0]!.map((token) => token.content).join("")).toBe("const a = 1");
    expect(outputs[1].tokens[0]!.map((token) => token.content).join("")).toBe("const b = 2");
  });
});
