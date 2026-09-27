/// <reference types="vitest/config" />
import { defineConfig, type Plugin } from "vite";
import { execSync } from "node:child_process";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

// Shiki grammars are large immutable JSON datasets. Emit them as lazy data
// assets in production; dev serves the same data through the virtual module.
function syntaxGrammarAssets(): Plugin {
  const prefix = "virtual:syntax-grammar/";
  let production = false;
  return {
    name: "gitwave-syntax-grammar-assets",
    configResolved(config) {
      production = config.command === "build";
    },
    resolveId(id) {
      if (id.startsWith(prefix)) return `\0${id}`;
    },
    async load(id) {
      if (!id.startsWith(`\0${prefix}`)) return;
      const language = id.slice(prefix.length + 1);
      if (!/^[a-z]+$/.test(language)) throw new Error("Invalid syntax grammar name");
      const grammar = await import(`shiki/langs/${language}.mjs`);
      const source = JSON.stringify(grammar.default);
      if (!production) return `export default async () => (${source});`;
      const asset = this.emitFile({ type: "asset", name: `${language}.grammar.json`, source });
      return `export default async () => {
        const response = await fetch(import.meta.ROLLUP_FILE_URL_${asset});
        if (!response.ok) throw new Error("Could not load syntax grammar: ${language}");
        return response.json();
      };`;
    },
  };
}

const host = process.env["TAURI_DEV_HOST"];

// Short commit SHA baked into the frontend bundle at build time (About dialog).
// Falls back to an empty string outside a git checkout (e.g. source archives).
function gitShortSha(): string {
  try {
    return execSync("git rev-parse --short HEAD", { stdio: ["ignore", "pipe", "ignore"] })
      .toString()
      .trim();
  } catch {
    return "";
  }
}

// https://vite.dev/config/
export default defineConfig(async () => ({
  // Vitest runs in a bare node environment on purpose: store tests stub
  // window.localStorage themselves (see autoRefreshStore.test.ts).
  test: { environment: "node" },

  plugins: [react(), tailwindcss(), syntaxGrammarAssets()],

  define: {
    __GIT_SHA__: JSON.stringify(gitShortSha()),
  },

  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },

  build: {
    rolldownOptions: {
      output: {
        codeSplitting: {
          includeDependenciesRecursively: false,
          groups: [
            { name: "runtime", test: /(?:\0vite|vite\/preload-helper)/, priority: 20 },
            {
              test: /node_modules/,
              name: (id) => {
                const parts = id.split("/node_modules/").at(-1)!.split("/");
                const name = parts[0]!.startsWith("@") ? parts.slice(0, 2).join("-") : parts[0]!;
                return `vendor-${name.replace("@", "")}`;
              },
            },
          ],
        },
      },
    },
  },

  // Vite options tailored for Tauri development and only applied in `tauri dev` or `tauri build`
  //
  // 1. prevent Vite from obscuring rust errors
  clearScreen: false,
  // 2. tauri expects a fixed port, fail if that port is not available
  server: {
    port: 1420,
    strictPort: true,
    // Pin IPv4: Node ≥17 resolves `localhost` to ::1 first, Vite then binds
    // IPv6 only while the Tauri CLI probes 127.0.0.1 and waits forever.
    host: host || "127.0.0.1",
    hmr: host
      ? {
          protocol: "ws",
          host,
          port: 1421,
        }
      : undefined,
    watch: {
      // 3. tell Vite to ignore watching `src-tauri`
      ignored: ["**/src-tauri/**"],
    },
  },
}));
