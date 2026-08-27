import { describe, expect, it } from "vitest";
import { deriveIgnorePatterns } from "./ignorePattern";

describe("deriveIgnorePatterns", () => {
  it("offers full path, directory and extension in nested paths", () => {
    expect(deriveIgnorePatterns("src/foo/bar.ts")).toEqual({
      full: "src/foo/bar.ts",
      dir: "src/foo/",
      ext: "*.ts",
    });
  });

  it("omits the directory option at repo root", () => {
    expect(deriveIgnorePatterns("README.md")).toEqual({
      full: "README.md",
      ext: "*.md",
    });
  });

  it("omits the extension option for extensionless files", () => {
    expect(deriveIgnorePatterns("Makefile")).toEqual({ full: "Makefile" });
    expect(deriveIgnorePatterns("bin/tool")).toEqual({ full: "bin/tool", dir: "bin/" });
  });

  it("treats dotfiles as extensionless", () => {
    expect(deriveIgnorePatterns(".env.local")).toEqual({
      full: ".env.local",
      ext: "*.local",
    });
    expect(deriveIgnorePatterns(".gitignore")).toEqual({ full: ".gitignore" });
  });
});
