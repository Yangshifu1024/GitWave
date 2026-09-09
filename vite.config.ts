/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import { execSync } from "node:child_process";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

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

  plugins: [react(), tailwindcss()],

  define: {
    __GIT_SHA__: JSON.stringify(gitShortSha()),
  },

  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
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
