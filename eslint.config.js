// ESLint v9 flat config — https://eslint.org/docs/latest/use/configure/configuration-files
import js from "@eslint/js";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";

export default tseslint.config(
  {
    ignores: [
      "dist",
      "src-tauri/target",
      "src-tauri/gen",
      "node_modules",
      "src/bindings",
      "**/*.d.ts",
      // Build/tooling configs — not application source.
      "vite.config.ts",
      "eslint.config.js",
    ],
  },
  {
    files: ["**/*.{ts,tsx}"],
    extends: [js.configs.recommended, ...tseslint.configs.recommendedTypeChecked],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
      parserOptions: {
        project: ["./tsconfig.json", "./tsconfig.node.json"],
        tsconfigRootDir: import.meta.dirname,
      },
    },
    plugins: {
      "react-hooks": reactHooks,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      // eslint-plugin-react-hooks 5 → 7 turns `recommended` from 2 rules into
      // the full React Compiler lint set (16 rules, 13 of them errors). Three
      // of those flag pre-existing, working patterns (resetting state from an
      // effect when a prop/key changes, refs written during render); converging
      // them is a dedicated refactor task, not part of a dependency bump, so
      // they stay visible as warnings instead of failing CI. Everything else in
      // the new set — rules-of-hooks, purity, set-state-in-render, ... — is left
      // at its upstream severity.
      "react-hooks/set-state-in-effect": "warn",
      "react-hooks/refs": "warn",
      "react-hooks/immutability": "warn",
      "@typescript-eslint/consistent-type-imports": "error",
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
    },
  },
);
